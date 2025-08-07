package services

import (
	"context"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// PhoneMappingService handles phone-CPF mappings and opt-in/opt-out flows
type PhoneMappingService struct {
	logger               *logging.SafeLogger
	registrationValidator RegistrationValidator
}

// NewPhoneMappingService creates a new PhoneMappingService
func NewPhoneMappingService(logger *logging.SafeLogger, validator RegistrationValidator) *PhoneMappingService {
	return &PhoneMappingService{
		logger:               logger,
		registrationValidator: validator,
	}
}

// FindCPFByPhone finds a CPF by phone number and returns masked data
func (s *PhoneMappingService) FindCPFByPhone(ctx context.Context, phoneNumber string) (*models.PhoneCitizenResponse, error) {
	// Parse phone number
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}

	// Format for storage lookup
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	// Find active mapping
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{
			"phone_number": storagePhone,
			"status":       models.PhoneMappingStatusActive,
		},
	).Decode(&mapping)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &models.PhoneCitizenResponse{Found: false}, nil
		}
		return nil, fmt.Errorf("failed to query phone mapping: %w", err)
	}

	// Get citizen data to extract name
	var citizen models.Citizen
	err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(
		ctx,
		bson.M{"cpf": mapping.CPF},
	).Decode(&citizen)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Phone mapping exists but citizen doesn't - this is an invalid state
			// Return not found instead of error
			s.logger.Warn("phone mapping exists but citizen not found", 
				zap.String("phone_number", storagePhone), 
				zap.String("cpf", mapping.CPF))
			return &models.PhoneCitizenResponse{Found: false}, nil
		}
		s.logger.Error("failed to get citizen data", zap.Error(err), zap.String("cpf", mapping.CPF))
		return nil, fmt.Errorf("failed to get citizen data: %w", err)
	}

	// Extract name and first name
	var fullName, firstName string
	if citizen.Nome != nil {
		fullName = *citizen.Nome
		firstName = utils.ExtractFirstName(fullName)
	}

	// Mask sensitive data
	maskedName := utils.MaskName(fullName)
	maskedCPF := utils.MaskCPF(mapping.CPF)

	return &models.PhoneCitizenResponse{
		Found:     true,
		CPF:       maskedCPF,
		Name:      maskedName,
		FirstName: firstName,
	}, nil
}

// ValidateRegistration validates registration data
func (s *PhoneMappingService) ValidateRegistration(ctx context.Context, phoneNumber string, req *models.ValidateRegistrationRequest) (*models.ValidateRegistrationResponse, error) {
	// Parse phone number
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}

	// Validate registration using the validator service
	valid, matchedCPF, matchedName, err := s.registrationValidator.ValidateRegistration(
		ctx, req.Name, req.CPF, req.BirthDate)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Record validation attempt
	s.recordValidationAttempt(ctx, components, req.CPF, valid, req.Channel)

	return &models.ValidateRegistrationResponse{
		Valid:       valid,
		MatchedCPF:  matchedCPF,
		MatchedName: matchedName,
	}, nil
}

// OptIn processes opt-in for a phone number
func (s *PhoneMappingService) OptIn(ctx context.Context, phoneNumber string, req *models.OptInRequest) (*models.OptInResponse, error) {
	// Parse phone number
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}

	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	// Check if there's already an active mapping for this phone
	var existingMapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{
			"phone_number": storagePhone,
			"status":       models.PhoneMappingStatusActive,
		},
	).Decode(&existingMapping)

	if err == nil {
		// Active mapping exists - check if it's for the same CPF
		if existingMapping.CPF == req.CPF {
			// Phone is already mapped to this CPF - return success
			s.logger.Info("phone already mapped to this CPF", 
				zap.String("phone_number", storagePhone), 
				zap.String("cpf", req.CPF))
			return &models.OptInResponse{
				Status:         "already_opted_in",
				PhoneMappingID: existingMapping.ID.Hex(),
			}, nil
		}
		// Different CPF - block the old mapping
		s.blockPhoneMapping(ctx, storagePhone, existingMapping.CPF, "new_opt_in")
	} else if err != mongo.ErrNoDocuments {
		// Some other error occurred
		return nil, fmt.Errorf("failed to check existing mapping: %w", err)
	}

	// Check if there's any mapping for this phone-CPF combination (regardless of status)
	var anyMapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{
			"phone_number": storagePhone,
			"cpf":         req.CPF,
		},
	).Decode(&anyMapping)

	if err == nil {
		// Mapping exists for this phone-CPF combination
		if anyMapping.Status == models.PhoneMappingStatusBlocked {
			// Reactivate the blocked mapping by updating it
			s.logger.Info("reactivating blocked mapping", 
				zap.String("phone_number", storagePhone), 
				zap.String("cpf", req.CPF))
			
			// Update the existing mapping to active status
			_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
				ctx,
				bson.M{"_id": anyMapping.ID},
				bson.M{
					"$set": bson.M{
						"status":         models.PhoneMappingStatusActive,
						"channel":        req.Channel,
						"updated_at":     time.Now(),
					},
				},
			)
			if err != nil {
				return nil, fmt.Errorf("failed to reactivate blocked mapping: %w", err)
			}

			// Record opt-in history
			s.recordOptInHistory(ctx, storagePhone, req.CPF, models.OptInActionOptIn, req.Channel, nil, req.ValidationResult)

			// Update self-declared data if this is a validated registration
			if req.ValidationResult != nil && req.ValidationResult.Valid {
				s.updateSelfDeclaredOptIn(ctx, req.CPF, req.Channel, storagePhone)
			}

			return &models.OptInResponse{
				Status:         "opted_in",
				PhoneMappingID: anyMapping.ID.Hex(),
			}, nil
		}
		// If mapping exists but is not blocked, continue with normal flow
	} else if err != mongo.ErrNoDocuments {
		// Some other error occurred
		return nil, fmt.Errorf("failed to check existing mapping: %w", err)
	}

	// Create new mapping (this should only happen for new phone-CPF combinations)
	mapping := models.PhoneCPFMapping{
		PhoneNumber:    storagePhone,
		CPF:           req.CPF,
		Status:        models.PhoneMappingStatusActive,
		IsSelfDeclared: req.ValidationResult != nil && req.ValidationResult.Valid,
		Channel:       req.Channel,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Add validation attempt if provided
	if req.ValidationResult != nil {
		mapping.ValidationAttempts = []models.ValidationAttempt{
			{
				Timestamp: time.Now(),
				Valid:     req.ValidationResult.Valid,
				Channel:   req.Channel,
			},
		}
	}

	// Insert new mapping
	result, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create phone mapping: %w", err)
	}

	// Record opt-in history
	s.recordOptInHistory(ctx, storagePhone, req.CPF, models.OptInActionOptIn, req.Channel, nil, req.ValidationResult)

	// Update self-declared data if this is a validated registration
	if req.ValidationResult != nil && req.ValidationResult.Valid {
		s.updateSelfDeclaredOptIn(ctx, req.CPF, req.Channel, storagePhone)
	}

	return &models.OptInResponse{
		Status:         "opted_in",
		PhoneMappingID: result.InsertedID.(string),
	}, nil
}

// OptOut processes opt-out for a phone number
func (s *PhoneMappingService) OptOut(ctx context.Context, phoneNumber string, req *models.OptOutRequest) (*models.OptOutResponse, error) {
	// Parse phone number
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}

	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	// Find any mapping for this phone number (not just active)
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{
			"phone_number": storagePhone,
		},
	).Decode(&mapping)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("no mapping found for phone number")
		}
		return nil, fmt.Errorf("failed to find phone mapping: %w", err)
	}

	// Check if mapping is already blocked
	if mapping.Status == models.PhoneMappingStatusBlocked {
		s.logger.Info("phone mapping already blocked", 
			zap.String("phone_number", storagePhone), 
			zap.String("cpf", mapping.CPF))
		// Still record the opt-out in history
		s.recordOptInHistory(ctx, storagePhone, mapping.CPF, models.OptInActionOptOut, req.Channel, &req.Reason, nil)
		return &models.OptOutResponse{
			Status: "already_opted_out",
		}, nil
	}

	// Only block the mapping if the reason is "incorrect_person" (Mensagem era engano)
	if req.Reason == models.OptOutReasonIncorrectPerson {
		_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
			ctx,
			bson.M{"_id": mapping.ID},
			bson.M{
				"$set": bson.M{
					"status":     models.PhoneMappingStatusBlocked,
					"updated_at": time.Now(),
				},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to block phone mapping: %w", err)
		}
		s.logger.Info("phone mapping blocked due to incorrect person opt-out", 
			zap.String("phone_number", storagePhone), 
			zap.String("cpf", mapping.CPF))
	} else {
		s.logger.Info("opt-out recorded without blocking phone mapping", 
			zap.String("phone_number", storagePhone), 
			zap.String("cpf", mapping.CPF), 
			zap.String("reason", req.Reason))
	}

	// Record opt-out history
	s.recordOptInHistory(ctx, storagePhone, mapping.CPF, models.OptInActionOptOut, req.Channel, &req.Reason, nil)

	// Update self-declared data
	s.updateSelfDeclaredOptOut(ctx, mapping.CPF)

	return &models.OptOutResponse{
		Status: "opted_out",
	}, nil
}

// RejectRegistration rejects a registration and blocks the phone-CPF mapping
func (s *PhoneMappingService) RejectRegistration(ctx context.Context, phoneNumber string, req *models.RejectRegistrationRequest) (*models.RejectRegistrationResponse, error) {
	// Parse phone number
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}

	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	// Block the mapping
	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{
			"phone_number": storagePhone,
			"cpf":         req.CPF,
		},
		bson.M{
			"$set": bson.M{
				"status":     models.PhoneMappingStatusBlocked,
				"updated_at": time.Now(),
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to block phone mapping: %w", err)
	}

	// Record rejection in opt-in history
	s.recordOptInHistory(ctx, storagePhone, req.CPF, models.OptInActionOptOut, req.Channel, &req.Reason, nil)

	return &models.RejectRegistrationResponse{
		Status: "rejected",
	}, nil
}

// Helper methods

func (s *PhoneMappingService) recordValidationAttempt(ctx context.Context, components *utils.PhoneComponents, cpf string, valid bool, channel string) {
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	attempt := models.ValidationAttempt{
		Timestamp: time.Now(),
		Valid:     valid,
		Channel:   channel,
	}

	_, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{"phone_number": storagePhone},
		bson.M{
			"$push": bson.M{
				"validation_attempts": attempt,
			},
			"$set": bson.M{
				"updated_at": time.Now(),
			},
		},
		options.Update().SetUpsert(true),
	)

	if err != nil {
		s.logger.Error("failed to record validation attempt", zap.Error(err))
	}
}

func (s *PhoneMappingService) recordOptInHistory(ctx context.Context, phoneNumber, cpf, action, channel string, reason *string, validationResult *models.ValidationResult) {
	history := models.OptInHistory{
		PhoneNumber:      phoneNumber,
		CPF:              cpf,
		Action:           action,
		Channel:          channel,
		Reason:           reason,
		ValidationResult: validationResult,
		Timestamp:        time.Now(),
	}

	_, err := config.MongoDB.Collection(config.AppConfig.OptInHistoryCollection).InsertOne(ctx, history)
	if err != nil {
		s.logger.Error("failed to record opt-in history", zap.Error(err))
	}
}

func (s *PhoneMappingService) blockPhoneMapping(ctx context.Context, phoneNumber, cpf, reason string) {
	_, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{
			"phone_number": phoneNumber,
			"cpf":         cpf,
		},
		bson.M{
			"$set": bson.M{
				"status":     models.PhoneMappingStatusBlocked,
				"updated_at": time.Now(),
			},
		},
	)
	if err != nil {
		s.logger.Error("failed to block phone mapping", zap.Error(err))
	}
}

func (s *PhoneMappingService) updateSelfDeclaredOptIn(ctx context.Context, cpf, channel, phoneNumber string) {
	// Update self-declared data with opt-in details
	update := bson.M{
		"$set": bson.M{
			"opt_in_details": bson.M{
				"status":       true,
				"channel":      channel,
				"timestamp":    time.Now(),
				"phone_number": phoneNumber,
				"validated":    true,
			},
			"updated_at": time.Now(),
		},
	}

	_, err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		s.logger.Error("failed to update self-declared opt-in", zap.Error(err))
	}
}

func (s *PhoneMappingService) updateSelfDeclaredOptOut(ctx context.Context, cpf string) {
	// Update self-declared data to opt out
	update := bson.M{
		"$set": bson.M{
			"opt_in_details.status": false,
			"opt_in_details.timestamp": time.Now(),
			"updated_at": time.Now(),
		},
	}

	_, err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
	)
	if err != nil {
		s.logger.Error("failed to update self-declared opt-out", zap.Error(err))
	}
} 