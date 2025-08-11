package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// DatabaseOperation represents a database operation that can be rolled back
type DatabaseOperation struct {
	Operation func() error
	Rollback  func() error
}

// ExecuteWithTransaction executes multiple database operations within a transaction
func ExecuteWithTransaction(ctx context.Context, operations []DatabaseOperation) error {
	logger := logging.Logger.With(zap.String("operation", "database_transaction"))

	// Start a session
	session, err := config.MongoDB.Client().StartSession()
	if err != nil {
		logger.Error("failed to start database session", zap.Error(err))
		return fmt.Errorf("failed to start database session: %w", err)
	}
	defer session.EndSession(ctx)

	// Start a transaction
	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		// Execute all operations
		for i, op := range operations {
			if err := op.Operation(); err != nil {
				logger.Error("operation failed, rolling back",
					zap.Int("operation_index", i),
					zap.Error(err))

				// Rollback all previous operations
				for j := i - 1; j >= 0; j-- {
					if rollbackErr := operations[j].Rollback(); rollbackErr != nil {
						logger.Error("rollback operation failed",
							zap.Int("rollback_index", j),
							zap.Error(rollbackErr))
					}
				}
				return nil, err
			}
		}
		return nil, nil
	})

	if err != nil {
		logger.Error("transaction failed", zap.Error(err))
		return fmt.Errorf("transaction failed: %w", err)
	}

	logger.Info("transaction completed successfully")
	return nil
}

// PhoneVerificationData represents the data needed for phone verification
type PhoneVerificationData struct {
	CPF         string
	DDI         string
	DDD         string
	Valor       string
	PhoneNumber string
	Code        string
	ExpiresAt   time.Time
}

// CreatePhoneVerification creates a phone verification record with proper error handling
func CreatePhoneVerification(ctx context.Context, data PhoneVerificationData) error {
	logger := logging.Logger.With(
		zap.String("cpf", data.CPF),
		zap.String("phone", data.PhoneNumber),
	)

	// First, try to send WhatsApp message
	if data.DDI != "" && data.Valor != "" {
		phone := fmt.Sprintf("%s%s%s", data.DDI, data.DDD, data.Valor)
		if err := SendVerificationCode(ctx, phone, data.Code); err != nil {
			logger.Error("failed to send WhatsApp message", zap.Error(err))
			return fmt.Errorf("failed to send verification code: %w", err)
		}
		logger.Info("WhatsApp verification code sent successfully")
	}

	// Then create the verification record
	verification := models.PhoneVerification{
		CPF: data.CPF,
		Telefone: &models.Telefone{
			Indicador: BoolPtr(false),
			Principal: &models.TelefonePrincipal{
				DDD:       &data.DDD,
				DDI:       &data.DDI,
				Valor:     &data.Valor,
				UpdatedAt: &data.ExpiresAt,
			},
		},
		PhoneNumber: data.PhoneNumber,
		Code:        data.Code,
		CreatedAt:   time.Now(),
		ExpiresAt:   data.ExpiresAt,
	}

	_, err := config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).InsertOne(ctx, verification)
	if err != nil {
		logger.Error("failed to create phone verification record", zap.Error(err))
		return fmt.Errorf("failed to create verification record: %w", err)
	}

	logger.Info("phone verification record created successfully")
	return nil
}

// UpdateSelfDeclaredPendingPhone updates the pending phone in self-declared collection
func UpdateSelfDeclaredPendingPhone(ctx context.Context, cpf string, data PhoneVerificationData) error {
	logger := logging.Logger.With(zap.String("cpf", cpf))

	pendingPhone := &models.Telefone{
		Indicador: BoolPtr(false),
		Principal: &models.TelefonePrincipal{
			DDD:       &data.DDD,
			DDI:       &data.DDI,
			Valor:     &data.Valor,
			UpdatedAt: &data.ExpiresAt,
		},
	}

	update := bson.M{
		"$set": bson.M{
			"telefone_pending": pendingPhone,
			"updated_at":       time.Now(),
		},
	}

	_, err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		logger.Error("failed to update pending phone", zap.Error(err))
		return fmt.Errorf("failed to update pending phone: %w", err)
	}

	logger.Info("pending phone updated successfully")
	return nil
}

// InvalidateCitizenCache invalidates all cache entries related to a citizen
func InvalidateCitizenCache(ctx context.Context, cpf string) error {
	logger := logging.Logger.With(zap.String("cpf", cpf))

	// Invalidate main citizen cache
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		logger.Warn("failed to invalidate citizen cache", zap.Error(err))
		return fmt.Errorf("failed to invalidate citizen cache: %w", err)
	}

	// Invalidate wallet cache
	walletCacheKey := fmt.Sprintf("citizen_wallet:%s", cpf)
	if err := config.Redis.Del(ctx, walletCacheKey).Err(); err != nil {
		logger.Warn("failed to invalidate wallet cache", zap.Error(err))
		// Don't return error for wallet cache invalidation failure
	}

	// Invalidate maintenance requests cache
	maintenanceCacheKey := fmt.Sprintf("maintenance_requests:%s", cpf)
	if err := config.Redis.Del(ctx, maintenanceCacheKey).Err(); err != nil {
		logger.Warn("failed to invalidate maintenance requests cache", zap.Error(err))
		// Don't return error for maintenance cache invalidation failure
	}

	logger.Info("citizen cache invalidated successfully")
	return nil
}
