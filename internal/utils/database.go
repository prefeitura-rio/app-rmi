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
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
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

// ExecuteWithWriteConcern executes a database operation with a specific write concern
func ExecuteWithWriteConcern(ctx context.Context, operationType string, operation func(mongo.SessionContext) error) error {
	logger := logging.Logger.With(zap.String("operation", "write_concern_operation"))

	// Start a session
	session, err := config.MongoDB.Client().StartSession()
	if err != nil {
		logger.Error("failed to start database session", zap.Error(err))
		return fmt.Errorf("failed to start database session: %w", err)
	}
	defer session.EndSession(ctx)

	// Execute operation with session
	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, operation(sessCtx)
	}, &options.TransactionOptions{
		WriteConcern: GetWriteConcernForOperation(operationType),
	})

	if err != nil {
		logger.Error("operation failed with write concern",
			zap.String("operation_type", operationType),
			zap.Error(err))
		return fmt.Errorf("operation failed with write concern: %w", err)
	}

	logger.Info("operation completed successfully with write concern",
		zap.String("operation_type", operationType))
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

// GetWriteConcernForOperation returns the appropriate write concern for different operation types
func GetWriteConcernForOperation(operationType string) *writeconcern.WriteConcern {
	switch operationType {
	case "audit":
		// Fire-and-forget for audit logs - highest performance
		return &writeconcern.WriteConcern{W: 0}
	case "user_data":
		// Acknowledged but not majority for user data updates - good performance
		return &writeconcern.WriteConcern{W: 1}
	case "critical":
		// Majority acknowledgment for critical operations - highest durability
		return &writeconcern.WriteConcern{W: "majority"}
	default:
		// Default to W(1) for most operations
		return &writeconcern.WriteConcern{W: 1}
	}
}

// GetUpdateOptionsWithWriteConcern returns update options with appropriate write concern
func GetUpdateOptionsWithWriteConcern(operationType string, upsert bool) *options.UpdateOptions {
	opts := options.Update().SetUpsert(upsert)

	// Note: Write concerns are applied at the collection level or via URI
	// This function provides the appropriate options structure for reference
	// The actual write concern is determined by the MongoDB URI configuration
	return opts
}

// GetInsertOptionsWithWriteConcern returns insert options with appropriate write concern
func GetInsertOptionsWithWriteConcern(operationType string) *options.InsertOneOptions {
	opts := options.InsertOne()

	// Note: Write concerns are applied at the collection level or via URI
	// This function provides the appropriate options structure for reference
	// The actual write concern is determined by the MongoDB URI configuration
	return opts
}

// GetCollectionWithWriteConcern returns a collection with the specified write concern
func GetCollectionWithWriteConcern(collectionName, operationType string) *mongo.Collection {
	collection := config.MongoDB.Collection(collectionName)

	// Note: Write concerns are now applied at the operation level via options
	// This function provides a centralized way to get collections with proper context
	return collection
}

// BulkWriteWithWriteConcern executes multiple write operations in a single batch
func BulkWriteWithWriteConcern(ctx context.Context, collection string, operations []mongo.WriteModel, operationType string) (*mongo.BulkWriteResult, error) {
	logger := logging.Logger.With(
		zap.String("collection", collection),
		zap.String("operation_type", operationType),
		zap.Int("operations_count", len(operations)),
	)

	// Configure bulk write options with appropriate write concern
	opts := options.BulkWrite().
		SetOrdered(false). // Allow parallel execution for better performance
		SetBypassDocumentValidation(false)

	// Execute bulk write operation
	result, err := config.MongoDB.Collection(collection).BulkWrite(ctx, operations, opts)
	if err != nil {
		logger.Error("bulk write operation failed", zap.Error(err))
		return nil, fmt.Errorf("bulk write operation failed: %w", err)
	}

	logger.Info("bulk write operation completed successfully",
		zap.Int64("inserted", result.InsertedCount),
		zap.Int64("modified", result.ModifiedCount),
		zap.Int64("deleted", result.DeletedCount),
		zap.Int64("upserted", result.UpsertedCount))

	return result, nil
}

// CreateBulkUpdateModels creates bulk update models for efficient batch processing
func CreateBulkUpdateModels(updates []BulkUpdateRequest) []mongo.WriteModel {
	var models []mongo.WriteModel

	for _, update := range updates {
		model := mongo.NewUpdateOneModel().
			SetFilter(update.Filter).
			SetUpdate(update.Update).
			SetUpsert(update.Upsert)

		models = append(models, model)
	}

	return models
}

// BulkUpdateRequest represents a single update operation for bulk processing
type BulkUpdateRequest struct {
	Filter bson.M
	Update bson.M
	Upsert bool
}
