package services

import (
	"context"
	"fmt"
	"strings"
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

type PhoneMappingService struct {
	logger *logging.SafeLogger
}

func NewPhoneMappingService(logger *logging.SafeLogger) *PhoneMappingService {
	return &PhoneMappingService{
		logger: logger,
	}
}

// GetPhoneStatus checks the status of a phone number including quarantine status
func (s *PhoneMappingService) GetPhoneStatus(ctx context.Context, phoneNumber string) (*models.PhoneStatusResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	// Find the phone mapping
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": storagePhone},
	).Decode(&mapping)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Phone number not found
			return &models.PhoneStatusResponse{
				PhoneNumber:     phoneNumber,
				Found:           false,
				Quarantined:     false,
				OptedOut:        false,
				BetaWhitelisted: false,
			}, nil
		}
		s.logger.Error("failed to get phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to get phone mapping: %w", err)
	}

	// Check if quarantined (computed on-demand)
	now := time.Now()
	quarantined := mapping.QuarantineUntil != nil && mapping.QuarantineUntil.After(now)

	// Check if opted out (status is blocked)
	optedOut := mapping.Status == models.MappingStatusBlocked

	response := &models.PhoneStatusResponse{
		PhoneNumber:     phoneNumber,
		Found:           true,
		Quarantined:     quarantined,
		OptedOut:        optedOut,
		QuarantineUntil: mapping.QuarantineUntil,
	}

	// If not quarantined and has CPF, get citizen data
	if !quarantined && mapping.CPF != "" {
		var citizen models.Citizen
		err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(
			ctx,
			bson.M{"cpf": mapping.CPF},
		).Decode(&citizen)

		if err == nil {
			response.CPF = utils.MaskCPF(citizen.CPF)
			if citizen.Nome != nil {
				response.Name = utils.MaskName(*citizen.Nome)
			}
		}
	}

	// Add beta group information
	response.BetaWhitelisted = mapping.BetaGroupID != ""
	response.BetaGroupID = mapping.BetaGroupID

	// Get beta group name if whitelisted
	if mapping.BetaGroupID != "" {
		betaGroupCollection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
		var betaGroup models.BetaGroup
		err = betaGroupCollection.FindOne(ctx, bson.M{"_id": mapping.BetaGroupID}).Decode(&betaGroup)
		if err == nil {
			response.BetaGroupName = betaGroup.Name
		}
	}

	return response, nil
}

// QuarantinePhone quarantines a phone number
func (s *PhoneMappingService) QuarantinePhone(ctx context.Context, phoneNumber string) (*models.QuarantineResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)
	now := time.Now()

	// Calculate quarantine end date
	quarantineTTL := config.AppConfig.PhoneQuarantineTTL

	quarantineUntil := now.Add(quarantineTTL)

	// Check if phone mapping exists
	var existingMapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": storagePhone},
	).Decode(&existingMapping)

	if err == mongo.ErrNoDocuments {
		// Create new quarantine record without CPF
		newMapping := models.PhoneCPFMapping{
			PhoneNumber:     storagePhone,
			Status:          models.MappingStatusQuarantined,
			QuarantineUntil: &quarantineUntil,
			QuarantineHistory: []models.QuarantineEvent{
				{
					QuarantinedAt:   now,
					QuarantineUntil: quarantineUntil,
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, newMapping)
		if err != nil {
			s.logger.Error("failed to create quarantine record", zap.Error(err), zap.String("phone_number", storagePhone))
			return nil, fmt.Errorf("failed to create quarantine record: %w", err)
		}

		return &models.QuarantineResponse{
			Status:          "quarantined",
			PhoneNumber:     phoneNumber,
			QuarantineUntil: quarantineUntil,
			Message:         "Phone number quarantined for 6 months",
		}, nil
	}

	if err != nil {
		s.logger.Error("failed to check existing phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to check existing phone mapping: %w", err)
	}

	// Extend existing quarantine
	quarantineEvent := models.QuarantineEvent{
		QuarantinedAt:   now,
		QuarantineUntil: quarantineUntil,
	}

	update := bson.M{
		"$set": bson.M{
			"status":           models.MappingStatusQuarantined,
			"quarantine_until": quarantineUntil,
			"updated_at":       now,
		},
		"$push": bson.M{
			"quarantine_history": quarantineEvent,
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{"phone_number": storagePhone},
		update,
	)
	if err != nil {
		s.logger.Error("failed to extend quarantine", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to extend quarantine: %w", err)
	}

	return &models.QuarantineResponse{
		Status:          "quarantined",
		PhoneNumber:     phoneNumber,
		QuarantineUntil: quarantineUntil,
		Message:         "Phone number quarantine extended for 6 months",
	}, nil
}

// ReleaseQuarantine releases a phone number from quarantine
func (s *PhoneMappingService) ReleaseQuarantine(ctx context.Context, phoneNumber string) (*models.QuarantineResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)
	now := time.Now()

	// Find the phone mapping
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": storagePhone},
	).Decode(&mapping)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("phone number not found")
		}
		s.logger.Error("failed to get phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to get phone mapping: %w", err)
	}

	// Update the last quarantine event with release time
	if len(mapping.QuarantineHistory) > 0 {
		lastEvent := mapping.QuarantineHistory[len(mapping.QuarantineHistory)-1]
		lastEvent.ReleasedAt = &now
		mapping.QuarantineHistory[len(mapping.QuarantineHistory)-1] = lastEvent
	}

	// Determine new status based on whether CPF exists
	newStatus := models.MappingStatusActive
	if mapping.CPF == "" {
		// If no CPF, remove the record entirely
		_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).DeleteOne(
			ctx,
			bson.M{"phone_number": storagePhone},
		)
		if err != nil {
			s.logger.Error("failed to delete quarantine record", zap.Error(err), zap.String("phone_number", storagePhone))
			return nil, fmt.Errorf("failed to delete quarantine record: %w", err)
		}

		return &models.QuarantineResponse{
			Status:          "released",
			PhoneNumber:     phoneNumber,
			QuarantineUntil: now,
			Message:         "Phone number released from quarantine and removed",
		}, nil
	}

	// Update the mapping
	update := bson.M{
		"$set": bson.M{
			"status":             newStatus,
			"quarantine_until":   nil,
			"updated_at":         now,
			"quarantine_history": mapping.QuarantineHistory,
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{"phone_number": storagePhone},
		update,
	)
	if err != nil {
		s.logger.Error("failed to release quarantine", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to release quarantine: %w", err)
	}

	return &models.QuarantineResponse{
		Status:          "released",
		PhoneNumber:     phoneNumber,
		QuarantineUntil: now,
		Message:         "Phone number released from quarantine",
	}, nil
}

// BindPhoneToCPF binds a phone number to a CPF without setting opt-in
func (s *PhoneMappingService) BindPhoneToCPF(ctx context.Context, phoneNumber, cpf, channel string) (*models.BindResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)
	now := time.Now()

	// Validate CPF
	if !utils.ValidateCPF(cpf) {
		return nil, fmt.Errorf("invalid CPF format")
	}

	// Check if phone mapping exists
	var existingMapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": storagePhone},
	).Decode(&existingMapping)

	if err == mongo.ErrNoDocuments {
		// Create new mapping
		newMapping := models.PhoneCPFMapping{
			PhoneNumber: storagePhone,
			CPF:         cpf,
			Status:      models.MappingStatusActive,
			Channel:     channel,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, newMapping)
		if err != nil {
			s.logger.Error("failed to create phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
			return nil, fmt.Errorf("failed to create phone mapping: %w", err)
		}

		return &models.BindResponse{
			Status:      "bound",
			PhoneNumber: phoneNumber,
			CPF:         cpf,
			OptIn:       false,
			Message:     "Phone number bound to CPF without opt-in",
		}, nil
	}

	if err != nil {
		s.logger.Error("failed to check existing phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to check existing phone mapping: %w", err)
	}

	// Update existing mapping
	update := bson.M{
		"$set": bson.M{
			"cpf":        cpf,
			"status":     models.MappingStatusActive,
			"channel":    channel,
			"updated_at": now,
		},
	}

	// If was quarantined, release it
	if existingMapping.QuarantineUntil != nil {
		update["$set"].(bson.M)["quarantine_until"] = nil

		// Add release time to last quarantine event
		if len(existingMapping.QuarantineHistory) > 0 {
			lastEvent := existingMapping.QuarantineHistory[len(existingMapping.QuarantineHistory)-1]
			lastEvent.ReleasedAt = &now
			existingMapping.QuarantineHistory[len(existingMapping.QuarantineHistory)-1] = lastEvent
			update["$set"].(bson.M)["quarantine_history"] = existingMapping.QuarantineHistory
		}
	}

	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{"phone_number": storagePhone},
		update,
	)
	if err != nil {
		s.logger.Error("failed to update phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to update phone mapping: %w", err)
	}

	return &models.BindResponse{
		Status:      "bound",
		PhoneNumber: phoneNumber,
		CPF:         cpf,
		OptIn:       false,
		Message:     "Phone number bound to CPF without opt-in",
	}, nil
}

// GetQuarantinedPhones returns a paginated list of quarantined phone numbers
func (s *PhoneMappingService) GetQuarantinedPhones(ctx context.Context, page, perPage int, expired bool) (*models.QuarantinedListResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	skip := int64((page - 1) * perPage)
	perPage64 := int64(perPage)
	now := time.Now()

	// Build filter
	filter := bson.M{"quarantine_until": bson.M{"$exists": true, "$ne": nil}}
	if expired {
		filter["quarantine_until"] = bson.M{"$lte": now}
	} else {
		filter["quarantine_until"] = bson.M{"$gt": now}
	}

	// Count total
	total, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(ctx, filter)
	if err != nil {
		s.logger.Error("failed to count quarantined phones", zap.Error(err))
		return nil, fmt.Errorf("failed to count quarantined phones: %w", err)
	}

	// Find quarantined phones
	cursor, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).Find(
		ctx,
		filter,
		&options.FindOptions{
			Skip:  &skip,
			Limit: &perPage64,
			Sort:  bson.M{"quarantine_until": 1},
		},
	)
	if err != nil {
		s.logger.Error("failed to find quarantined phones", zap.Error(err))
		return nil, fmt.Errorf("failed to find quarantined phones: %w", err)
	}
	defer cursor.Close(ctx)

	var quarantinedPhones []models.QuarantinedPhone
	for cursor.Next(ctx) {
		var mapping models.PhoneCPFMapping
		if err := cursor.Decode(&mapping); err != nil {
			s.logger.Error("failed to decode phone mapping", zap.Error(err))
			continue
		}

		quarantinedPhone := models.QuarantinedPhone{
			PhoneNumber:     utils.ExtractPhoneFromComponents("55", mapping.PhoneNumber[:2], mapping.PhoneNumber[2:]),
			QuarantineUntil: *mapping.QuarantineUntil,
			Expired:         mapping.QuarantineUntil.Before(now),
		}

		if mapping.CPF != "" {
			quarantinedPhone.CPF = utils.MaskCPF(mapping.CPF)
		}

		quarantinedPhones = append(quarantinedPhones, quarantinedPhone)
	}

	totalPages := (int(total) + perPage - 1) / perPage

	return &models.QuarantinedListResponse{
		Data: quarantinedPhones,
		Pagination: models.PaginationInfo{
			Page:       page,
			PerPage:    perPage,
			Total:      int(total),
			TotalPages: totalPages,
		},
	}, nil
}

// GetQuarantineStats returns quarantine statistics
func (s *PhoneMappingService) GetQuarantineStats(ctx context.Context) (*models.QuarantineStats, error) {
	now := time.Now()

	// Total quarantined (with quarantine_until field)
	totalQuarantined, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(
		ctx,
		bson.M{"quarantine_until": bson.M{"$exists": true, "$ne": nil}},
	)
	if err != nil {
		s.logger.Error("failed to count total quarantined", zap.Error(err))
		return nil, fmt.Errorf("failed to count total quarantined: %w", err)
	}

	// Expired quarantines
	expiredQuarantines, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(
		ctx,
		bson.M{"quarantine_until": bson.M{"$lte": now}},
	)
	if err != nil {
		s.logger.Error("failed to count expired quarantines", zap.Error(err))
		return nil, fmt.Errorf("failed to count expired quarantines: %w", err)
	}

	// Active quarantines
	activeQuarantines, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(
		ctx,
		bson.M{"quarantine_until": bson.M{"$gt": now}},
	)
	if err != nil {
		s.logger.Error("failed to count active quarantines", zap.Error(err))
		return nil, fmt.Errorf("failed to count active quarantines: %w", err)
	}

	// Quarantines with CPF
	quarantinesWithCPF, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(
		ctx,
		bson.M{
			"quarantine_until": bson.M{"$exists": true, "$ne": nil},
			"cpf":              bson.M{"$exists": true, "$ne": ""},
		},
	)
	if err != nil {
		s.logger.Error("failed to count quarantines with CPF", zap.Error(err))
		return nil, fmt.Errorf("failed to count quarantines with CPF: %w", err)
	}

	// Quarantines without CPF
	quarantinesWithoutCPF, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(
		ctx,
		bson.M{
			"quarantine_until": bson.M{"$exists": true, "$ne": nil},
			"$or": []bson.M{
				{"cpf": bson.M{"$exists": false}},
				{"cpf": ""},
			},
		},
	)
	if err != nil {
		s.logger.Error("failed to count quarantines without CPF", zap.Error(err))
		return nil, fmt.Errorf("failed to count quarantines without CPF: %w", err)
	}

	// Total quarantine history events
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"quarantine_history": bson.M{"$exists": true}}}},
		{{Key: "$unwind", Value: "$quarantine_history"}},
		{{Key: "$count", Value: "total"}},
	}

	cursor, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).Aggregate(ctx, pipeline)
	if err != nil {
		s.logger.Error("failed to count quarantine history", zap.Error(err))
		return nil, fmt.Errorf("failed to count quarantine history: %w", err)
	}
	defer cursor.Close(ctx)

	var historyResult []bson.M
	if err := cursor.All(ctx, &historyResult); err != nil {
		s.logger.Error("failed to decode quarantine history count", zap.Error(err))
		return nil, fmt.Errorf("failed to decode quarantine history count: %w", err)
	}

	quarantineHistoryTotal := 0
	if len(historyResult) > 0 {
		if total, ok := historyResult[0]["total"].(int32); ok {
			quarantineHistoryTotal = int(total)
		}
	}

	return &models.QuarantineStats{
		TotalQuarantined:       int(totalQuarantined),
		ExpiredQuarantines:     int(expiredQuarantines),
		ActiveQuarantines:      int(activeQuarantines),
		QuarantinesWithCPF:     int(quarantinesWithCPF),
		QuarantinesWithoutCPF:  int(quarantinesWithoutCPF),
		QuarantineHistoryTotal: quarantineHistoryTotal,
	}, nil
}

// FindCPFByPhone finds a CPF by phone number (existing method, updated for new model)
func (s *PhoneMappingService) FindCPFByPhone(ctx context.Context, phoneNumber string) (*models.PhoneCitizenResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": storagePhone},
	).Decode(&mapping)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &models.PhoneCitizenResponse{Found: false}, nil
		}
		s.logger.Error("failed to get phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to get phone mapping: %w", err)
	}

	// Check if quarantined
	now := time.Now()
	if mapping.QuarantineUntil != nil && mapping.QuarantineUntil.After(now) {
		return &models.PhoneCitizenResponse{Found: false}, nil
	}

	if mapping.CPF == "" {
		return &models.PhoneCitizenResponse{Found: false}, nil
	}

	// Get citizen data
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

	return &models.PhoneCitizenResponse{
		Found: true,
		CPF:   utils.MaskCPF(citizen.CPF),
		Name: func() string {
			if citizen.Nome != nil {
				return utils.MaskName(*citizen.Nome)
			}
			return ""
		}(),
	}, nil
}

// ValidateRegistration validates registration data against base data
func (s *PhoneMappingService) ValidateRegistration(ctx context.Context, phoneNumber string, name, cpf, birthDate string) (*models.ValidateRegistrationResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)
	now := time.Now()

	// Validate CPF
	if !utils.ValidateCPF(cpf) {
		return &models.ValidateRegistrationResponse{
			Valid: false,
		}, nil
	}

	// Get citizen data from base collection
	var citizen models.Citizen
	err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(
		ctx,
		bson.M{"cpf": cpf},
	).Decode(&citizen)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &models.ValidateRegistrationResponse{
				Valid: false,
			}, nil
		}
		s.logger.Error("failed to get citizen data", zap.Error(err), zap.String("cpf", cpf))
		return nil, fmt.Errorf("failed to get citizen data: %w", err)
	}

	// Validate name (case-insensitive)
	validName := false
	if citizen.Nome != nil {
		validName = strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(*citizen.Nome))
	}

	// Validate birth date
	validBirthDate := false
	if citizen.Nascimento != nil && citizen.Nascimento.Data != nil {
		expectedDate := citizen.Nascimento.Data.Format("2006-01-02")
		validBirthDate = birthDate == expectedDate
	}

	// Record validation attempt
	validationAttempt := models.ValidationAttempt{
		AttemptedAt: now,
		Valid:       validName && validBirthDate,
		Channel:     "whatsapp",
	}

	// Update or create phone mapping with validation attempt
	update := bson.M{
		"$set": bson.M{
			"validation_attempt": validationAttempt,
			"updated_at":         now,
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{"phone_number": storagePhone},
		update,
		&options.UpdateOptions{Upsert: &[]bool{true}[0]},
	)
	if err != nil {
		s.logger.Error("failed to update validation attempt", zap.Error(err), zap.String("phone_number", storagePhone))
		// Don't fail the validation for this error
	}

	if validName && validBirthDate {
		return &models.ValidateRegistrationResponse{
			Valid:      true,
			MatchedCPF: citizen.CPF,
			MatchedName: func() string {
				if citizen.Nome != nil {
					return *citizen.Nome
				}
				return ""
			}(),
		}, nil
	}

	return &models.ValidateRegistrationResponse{
		Valid: false,
	}, nil
}

// OptIn processes opt-in for a phone number
func (s *PhoneMappingService) OptIn(ctx context.Context, phoneNumber, cpf, channel string) (*models.OptInResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)
	now := time.Now()

	// Validate CPF
	if !utils.ValidateCPF(cpf) {
		return nil, fmt.Errorf("invalid CPF format")
	}

	// Check if phone mapping exists
	var existingMapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": storagePhone},
	).Decode(&existingMapping)

	if err == mongo.ErrNoDocuments {
		// Create new mapping
		newMapping := models.PhoneCPFMapping{
			PhoneNumber: storagePhone,
			CPF:         cpf,
			Status:      models.MappingStatusActive,
			Channel:     channel,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, newMapping)
		if err != nil {
			s.logger.Error("failed to create phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
			return nil, fmt.Errorf("failed to create phone mapping: %w", err)
		}

		// Record opt-in history
		s.recordOptInHistory(ctx, phoneNumber, cpf, "opt_in", channel, "")

		return &models.OptInResponse{
			Status: "opted_in",
		}, nil
	}

	if err != nil {
		s.logger.Error("failed to check existing phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to check existing phone mapping: %w", err)
	}

	// Check if already opted in for the same CPF
	if existingMapping.CPF == cpf && existingMapping.Status == models.MappingStatusActive {
		return &models.OptInResponse{
			Status: "already_opted_in",
		}, nil
	}

	// Update existing mapping
	update := bson.M{
		"$set": bson.M{
			"cpf":        cpf,
			"status":     models.MappingStatusActive,
			"channel":    channel,
			"updated_at": now,
		},
	}

	// If was quarantined, release it
	if existingMapping.QuarantineUntil != nil {
		update["$set"].(bson.M)["quarantine_until"] = nil

		// Add release time to last quarantine event
		if len(existingMapping.QuarantineHistory) > 0 {
			lastEvent := existingMapping.QuarantineHistory[len(existingMapping.QuarantineHistory)-1]
			lastEvent.ReleasedAt = &now
			existingMapping.QuarantineHistory[len(existingMapping.QuarantineHistory)-1] = lastEvent
			update["$set"].(bson.M)["quarantine_history"] = existingMapping.QuarantineHistory
		}
	}

	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{"phone_number": storagePhone},
		update,
	)
	if err != nil {
		s.logger.Error("failed to update phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to update phone mapping: %w", err)
	}

	// Record opt-in history
	s.recordOptInHistory(ctx, phoneNumber, cpf, "opt_in", channel, "")

	return &models.OptInResponse{
		Status: "opted_in",
	}, nil
}

// OptOut processes opt-out for a phone number
func (s *PhoneMappingService) OptOut(ctx context.Context, phoneNumber, reason, channel string) (*models.OptOutResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)
	now := time.Now()

	// Find existing mapping
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": storagePhone},
	).Decode(&mapping)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Phone number not found - create a new mapping record to track the opt-out preference
			s.logger.Info("creating opt-out record for unknown phone number", zap.String("phone_number", storagePhone))
			
			// Create new mapping with blocked status to record the opt-out preference
			newMapping := models.PhoneCPFMapping{
				PhoneNumber: storagePhone,
				// CPF is empty since this phone is not bound to any CPF
				Status:    models.MappingStatusBlocked,
				Channel:   channel,
				CreatedAt: now,
				UpdatedAt: now,
			}
			
			_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, newMapping)
			if err != nil {
				s.logger.Error("failed to create opt-out record", zap.Error(err), zap.String("phone_number", storagePhone))
				return nil, fmt.Errorf("failed to create opt-out record: %w", err)
			}
			
			// Record opt-out history (with empty CPF since phone is not bound)
			s.recordOptInHistory(ctx, phoneNumber, "", "opt_out", channel, reason)
			
			return &models.OptOutResponse{
				Status: "opted_out",
			}, nil
		}
		s.logger.Error("failed to get phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to get phone mapping: %w", err)
	}

	// Record opt-out history
	s.recordOptInHistory(ctx, phoneNumber, mapping.CPF, "opt_out", channel, reason)

	// Update mapping status
	update := bson.M{
		"$set": bson.M{
			"status":     models.MappingStatusBlocked,
			"updated_at": now,
		},
	}

	// Only block the mapping if reason is "Mensagem era engano"
	if reason == "Mensagem era engano" {
		update["$set"].(bson.M)["status"] = models.MappingStatusBlocked
	}

	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{"phone_number": storagePhone},
		update,
	)
	if err != nil {
		s.logger.Error("failed to update phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to update phone mapping: %w", err)
	}

	return &models.OptOutResponse{
		Status: "opted_out",
	}, nil
}

// RejectRegistration rejects a registration and blocks the phone-CPF mapping
func (s *PhoneMappingService) RejectRegistration(ctx context.Context, phoneNumber, cpf string) (*models.RejectRegistrationResponse, error) {
	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("invalid phone number: %w", err)
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)
	now := time.Now()

	// Validate CPF
	if !utils.ValidateCPF(cpf) {
		return nil, fmt.Errorf("invalid CPF format")
	}

	// Find existing mapping
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": storagePhone},
	).Decode(&mapping)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &models.RejectRegistrationResponse{
				Status: "not_found",
			}, nil
		}
		s.logger.Error("failed to get phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to get phone mapping: %w", err)
	}

	// Record rejection in history
	s.recordOptInHistory(ctx, phoneNumber, cpf, "rejected", "whatsapp", "Registro rejeitado pelo usuário")

	// Block the mapping
	update := bson.M{
		"$set": bson.M{
			"status":     models.MappingStatusBlocked,
			"updated_at": now,
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(
		ctx,
		bson.M{"phone_number": storagePhone},
		update,
	)
	if err != nil {
		s.logger.Error("failed to block phone mapping", zap.Error(err), zap.String("phone_number", storagePhone))
		return nil, fmt.Errorf("failed to block phone mapping: %w", err)
	}

	return &models.RejectRegistrationResponse{
		Status: "rejected",
	}, nil
}

// recordOptInHistory records opt-in/opt-out history
func (s *PhoneMappingService) recordOptInHistory(ctx context.Context, phoneNumber, cpf, action, channel, reason string) {
	now := time.Now()

	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		s.logger.Error("failed to parse phone number for history", zap.Error(err), zap.String("phone_number", phoneNumber))
		return
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	history := models.OptInHistory{
		PhoneNumber: storagePhone,
		CPF:         cpf,
		Action:      action,
		Channel:     channel,
		Reason:      &reason,
		Timestamp:   now,
	}

	_, err = config.MongoDB.Collection(config.AppConfig.OptInHistoryCollection).InsertOne(ctx, history)
	if err != nil {
		s.logger.Error("failed to record opt-in history", zap.Error(err), zap.String("phone_number", phoneNumber))
		// Don't fail the main operation for this error
	}
}

// BulkUpdatePhoneStatuses updates multiple phone statuses in a single bulk operation
func (s *PhoneMappingService) BulkUpdatePhoneStatuses(ctx context.Context, updates []PhoneStatusUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	var operations []mongo.WriteModel

	for _, update := range updates {
		operation := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"phone_number": update.PhoneNumber}).
			SetUpdate(bson.M{"$set": update.Fields})
		operations = append(operations, operation)
	}

	// Use W=0 for maximum performance (phone mappings are W=0)
	opts := options.BulkWrite().SetOrdered(false)
	result, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).
		BulkWrite(ctx, operations, opts)

	if err != nil {
		s.logger.Error("failed to bulk update phone statuses",
			zap.Error(err),
			zap.Int("updates_count", len(updates)))
		return fmt.Errorf("failed to bulk update phone statuses: %w", err)
	}

	s.logger.Info("bulk phone status update completed",
		zap.Int64("matched", result.MatchedCount),
		zap.Int64("modified", result.ModifiedCount),
		zap.Int("updates_count", len(updates)))

	return nil
}

// BulkInsertPhoneMappings inserts multiple phone mappings in a single bulk operation
func (s *PhoneMappingService) BulkInsertPhoneMappings(ctx context.Context, mappings []models.PhoneCPFMapping) error {
	if len(mappings) == 0 {
		return nil
	}

	var operations []mongo.WriteModel

	for _, mapping := range mappings {
		operation := mongo.NewInsertOneModel().SetDocument(mapping)
		operations = append(operations, operation)
	}

	// Use W=0 for maximum performance (phone mappings are W=0)
	opts := options.BulkWrite().SetOrdered(false)
	result, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).
		BulkWrite(ctx, operations, opts)

	if err != nil {
		s.logger.Error("failed to bulk insert phone mappings",
			zap.Error(err),
			zap.Int("mappings_count", len(mappings)))
		return fmt.Errorf("failed to bulk insert phone mappings: %w", err)
	}

	s.logger.Info("bulk phone mapping insert completed",
		zap.Int64("inserted", result.InsertedCount),
		zap.Int("mappings_count", len(mappings)))

	return nil
}

// PhoneStatusUpdate represents an update to a phone status
type PhoneStatusUpdate struct {
	PhoneNumber string                 `json:"phone_number"`
	Fields      map[string]interface{} `json:"fields"`
}
