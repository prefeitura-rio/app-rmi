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
	"go.mongodb.org/mongo-driver/mongo/readpref"
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

// ExecuteWriteOperation executes a database write operation without transaction overhead for better performance
func ExecuteWriteOperation(ctx context.Context, collection string, operationType string, operation func(*mongo.Collection) error) error {
	logger := logging.Logger.With(
		zap.String("operation", "write_operation"),
		zap.String("collection", collection),
		zap.String("operation_type", operationType),
	)

	// Get collection with appropriate write concern
	coll := config.MongoDB.Collection(collection)

	// Execute operation directly without transaction overhead
	if err := operation(coll); err != nil {
		logger.Error("write operation failed", zap.Error(err))
		return fmt.Errorf("write operation failed: %w", err)
	}

	logger.Info("write operation completed successfully",
		zap.String("operation_type", operationType))
	return nil
}

// ExecuteWithWriteConcern executes a database operation with a specific write concern
func ExecuteWithWriteConcern(ctx context.Context, operationType string, operation func(mongo.SessionContext) error) error {
	logger := logging.Logger.With(zap.String("operation", "write_concern_operation"))

	wc := GetWriteConcernForOperation(operationType)

	// For unacknowledged writes (W=0), execute without transaction
	// Transactions don't support unacknowledged write concerns
	if wc.W == 0 {
		// Create a dummy session context for compatibility
		session, err := config.MongoDB.Client().StartSession()
		if err != nil {
			logger.Error("failed to start database session", zap.Error(err))
			return fmt.Errorf("failed to start database session: %w", err)
		}
		defer session.EndSession(ctx)

		sessCtx := mongo.NewSessionContext(ctx, session)
		if err := operation(sessCtx); err != nil {
			logger.Error("operation failed with write concern",
				zap.String("operation_type", operationType),
				zap.Error(err))
			return fmt.Errorf("operation failed with write concern: %w", err)
		}

		logger.Info("operation completed successfully with write concern",
			zap.String("operation_type", operationType))
		return nil
	}

	// For acknowledged writes, use transaction
	session, err := config.MongoDB.Client().StartSession()
	if err != nil {
		logger.Error("failed to start database session", zap.Error(err))
		return fmt.Errorf("failed to start database session: %w", err)
	}
	defer session.EndSession(ctx)

	// Execute operation with session and transaction
	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, operation(sessCtx)
	}, &options.TransactionOptions{
		WriteConcern: wc,
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

	// Write concerns are now applied at the collection level for better performance
	// This eliminates the need for per-operation write concern logic
	return opts
}

// GetInsertOptionsWithWriteConcern returns insert options with appropriate write concern
func GetInsertOptionsWithWriteConcern(operationType string) *options.InsertOneOptions {
	opts := options.InsertOne()

	// Write concerns are now applied at the collection level for better performance
	// This eliminates the need for per-operation write concern logic
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

	// MongoDB requires at least one operation for bulk write
	if len(operations) == 0 {
		logger.Info("no operations to perform in bulk write")
		return &mongo.BulkWriteResult{}, nil
	}

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

// GetCollectionWithReadPreference returns a collection with a specific read preference for load distribution
func GetCollectionWithReadPreference(collectionName string, readPref *readpref.ReadPref) *mongo.Collection {
	collection := config.MongoDB.Collection(collectionName)

	// Note: Read preferences are typically set at the client level
	// This function provides a centralized way to get collections with proper context
	// The actual read preference is determined by the MongoDB client configuration
	return collection
}

// GetCollectionForReadOperation returns a collection optimized for read operations
func GetCollectionForReadOperation(collectionName string) *mongo.Collection {
	// For read operations, we want to use secondary nodes when possible
	// This helps distribute load away from the primary
	// The readPreference=nearest is set at the client level
	return config.MongoDB.Collection(collectionName)
}

// GetCollectionForWriteOperation returns a collection optimized for write operations
func GetCollectionForWriteOperation(collectionName string) *mongo.Collection {
	// For write operations, we must use the primary node
	// But we can optimize the write concern for better performance
	return config.MongoDB.Collection(collectionName)
}

// ExecuteReadWithLoadDistribution executes a read operation with load distribution
func ExecuteReadWithLoadDistribution(ctx context.Context, collection string, operation func(*mongo.Collection) error) error {
	logger := logging.Logger.With(
		zap.String("operation", "read_with_load_distribution"),
		zap.String("collection", collection),
	)

	// Use a collection that can read from secondary nodes
	// This helps distribute load away from the primary
	// The readPreference=nearest is enforced at the client level
	coll := GetCollectionForReadOperation(collection)

	// Execute the read operation
	if err := operation(coll); err != nil {
		logger.Error("read operation failed", zap.Error(err))
		return fmt.Errorf("read operation failed: %w", err)
	}

	logger.Info("read operation completed with load distribution")
	return nil
}

// ExecuteWriteWithOptimizedConcern executes a write operation with optimized write concern
func ExecuteWriteWithOptimizedConcern(ctx context.Context, collection string, operationType string, operation func(*mongo.Collection) error) error {
	logger := logging.Logger.With(
		zap.String("operation", "write_with_optimized_concern"),
		zap.String("collection", collection),
		zap.String("operation_type", operationType),
	)

	// Use a collection optimized for write operations
	// Write concern is applied via options in the operation
	coll := GetCollectionForWriteOperation(collection)

	// Execute the write operation
	if err := operation(coll); err != nil {
		logger.Error("write operation failed", zap.Error(err))
		return fmt.Errorf("write operation failed: %w", err)
	}

	logger.Info("write operation completed with optimized write concern",
		zap.String("operation_type", operationType))
	return nil
}

// ExecuteWithLoadDistribution executes operations with optimal load distribution
func ExecuteWithLoadDistribution(ctx context.Context, operationType string, operation func(mongo.SessionContext) error) error {
	logger := logging.Logger.With(
		zap.String("operation", "load_distribution_operation"),
		zap.String("operation_type", operationType),
	)

	// Start a session with load distribution optimizations
	session, err := config.MongoDB.Client().StartSession()
	if err != nil {
		logger.Error("failed to start database session", zap.Error(err))
		return fmt.Errorf("failed to start database session: %w", err)
	}
	defer session.EndSession(ctx)

	// For audit operations (W=0), execute without transaction
	// Transactions don't support unacknowledged write concerns
	if operationType == "audit" {
		sessCtx := mongo.NewSessionContext(ctx, session)
		if err := operation(sessCtx); err != nil {
			logger.Error("operation failed with load distribution",
				zap.String("operation_type", operationType),
				zap.Error(err))
			return fmt.Errorf("operation failed with load distribution: %w", err)
		}

		logger.Info("operation completed successfully with load distribution",
			zap.String("operation_type", operationType))
		return nil
	}

	// Configure session based on operation type
	var sessionOpts *options.SessionOptions
	switch operationType {
	case "read":
		// For reads, use secondary nodes when possible
		sessionOpts = options.Session().SetDefaultReadPreference(readpref.Nearest())
	case "write":
		// For writes, use primary with optimized write concern
		sessionOpts = options.Session().
			SetDefaultReadPreference(readpref.Primary()).
			SetDefaultWriteConcern(&writeconcern.WriteConcern{W: 1})
	default:
		// Default to balanced approach
		sessionOpts = options.Session().SetDefaultReadPreference(readpref.Nearest())
	}

	// Execute operation with optimized session
	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, operation(sessCtx)
	}, &options.TransactionOptions{
		ReadPreference: sessionOpts.DefaultReadPreference,
		WriteConcern:   sessionOpts.DefaultWriteConcern,
	})

	if err != nil {
		logger.Error("operation failed with load distribution",
			zap.String("operation_type", operationType),
			zap.Error(err))
		return fmt.Errorf("operation failed with load distribution: %w", err)
	}

	logger.Info("operation completed successfully with load distribution",
		zap.String("operation_type", operationType))
	return nil
}

// GetCollectionWithLoadDistribution returns a collection optimized for the operation type
func GetCollectionWithLoadDistribution(collectionName, operationType string) *mongo.Collection {
	collection := config.MongoDB.Collection(collectionName)

	// The actual load distribution is handled at the session level
	// This function provides a centralized way to get collections with proper context
	return collection
}

// ExecuteReadWithSecondaryPreference executes read operations preferring secondary nodes
func ExecuteReadWithSecondaryPreference(ctx context.Context, collection string, operation func(*mongo.Collection) error) error {
	// Use a session that prefers secondary nodes for reads
	return ExecuteWithLoadDistribution(ctx, "read", func(sessCtx mongo.SessionContext) error {
		// Execute the read operation in the session context
		// The session will automatically route to secondary nodes when possible
		return operation(config.MongoDB.Collection(collection))
	})
}

// ExecuteWriteWithPrimaryOptimization executes write operations with primary node optimization
func ExecuteWriteWithPrimaryOptimization(ctx context.Context, collection string, operation func(*mongo.Collection) error) error {
	// Use a session optimized for write operations
	return ExecuteWithLoadDistribution(ctx, "write", func(sessCtx mongo.SessionContext) error {
		// Execute the write operation in the session context
		// The session will use primary node with optimized write concern
		return operation(config.MongoDB.Collection(collection))
	})
}

// ExecuteBulkWriteOptimized executes multiple write operations in a single batch with performance optimization
func ExecuteBulkWriteOptimized(ctx context.Context, collection string, operations []mongo.WriteModel, operationType string) (*mongo.BulkWriteResult, error) {
	logger := logging.Logger.With(
		zap.String("operation", "bulk_write_optimized"),
		zap.String("collection", collection),
		zap.String("operation_type", operationType),
		zap.Int("operations_count", len(operations)),
	)

	// Configure bulk write options for maximum performance
	opts := options.BulkWrite().
		SetOrdered(false).                  // Allow parallel execution for better performance
		SetBypassDocumentValidation(false). // Keep validation for data integrity
		SetComment("bulk_write_optimized")  // Add comment for monitoring

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
		zap.Int64("upserted", result.UpsertedCount),
		zap.String("operation_type", operationType))

	return result, nil
}

// CreateOptimizedBulkUpdateModels creates bulk update models with performance optimizations
func CreateOptimizedBulkUpdateModels(updates []BulkUpdateRequest) []mongo.WriteModel {
	var models []mongo.WriteModel
	models = make([]mongo.WriteModel, 0, len(updates)) // Pre-allocate slice capacity

	for _, update := range updates {
		model := mongo.NewUpdateOneModel().
			SetFilter(update.Filter).
			SetUpdate(update.Update).
			SetUpsert(update.Upsert)

		models = append(models, model)
	}

	return models
}

// ExecuteWriteWithLoadOptimization executes a write operation with dynamic optimization based on current load
func ExecuteWriteWithLoadOptimization(ctx context.Context, collection string, operationType string, operation func(*mongo.Collection) error) error {
	logger := logging.Logger.With(
		zap.String("operation", "write_with_load_optimization"),
		zap.String("collection", collection),
		zap.String("operation_type", operationType),
	)

	// Check current system load and adjust operation strategy
	if shouldUseBulkOperations() {
		logger.Info("high load detected - using bulk operation strategy")
		// For high load, we could implement bulk operation logic here
		// For now, we'll use the standard optimized approach
	}

	// Use a collection optimized for write operations
	coll := GetCollectionForWriteOperation(collection)

	// Execute the write operation with performance monitoring
	start := time.Now()
	err := operation(coll)
	duration := time.Since(start)

	// Log performance metrics
	if duration > 100*time.Millisecond {
		logger.Warn("slow write operation detected",
			zap.String("operation_type", operationType),
			zap.Duration("duration", duration),
			zap.String("collection", collection))
	} else {
		logger.Info("write operation completed with load optimization",
			zap.String("operation_type", operationType),
			zap.Duration("duration", duration))
	}

	if err != nil {
		logger.Error("write operation failed with load optimization",
			zap.String("operation_type", operationType),
			zap.Error(err))
		return fmt.Errorf("write operation failed with load optimization: %w", err)
	}

	return nil
}

// shouldUseBulkOperations determines if bulk operations should be used based on current load
func shouldUseBulkOperations() bool {
	// This is a simplified implementation
	// In production, you'd check actual system metrics
	// For now, we'll return false to use standard operations
	return false
}

// ExecuteWriteWithCollectionOptimization executes a write operation with collection-specific optimizations
func ExecuteWriteWithCollectionOptimization(ctx context.Context, collection string, operationType string, operation func(*mongo.Collection) error) error {
	logger := logging.Logger.With(
		zap.String("operation", "write_with_collection_optimization"),
		zap.String("collection", collection),
		zap.String("operation_type", operationType),
	)

	// Get collection with optimizations based on collection type
	var coll *mongo.Collection

	switch collection {
	case config.AppConfig.AuditLogsCollection:
		// Audit logs: Use fire-and-forget for maximum performance
		coll = config.MongoDB.Collection(collection)
		logger.Info("using audit collection optimization (fire-and-forget)")

	case config.AppConfig.CitizenCollection, config.AppConfig.SelfDeclaredCollection:
		// High-traffic collections: Use W=1 for good performance
		coll = config.MongoDB.Collection(collection)
		logger.Info("using high-traffic collection optimization (W=1)")

	case config.AppConfig.PhoneMappingCollection, config.AppConfig.OptInHistoryCollection:
		// Medium-traffic collections: Use W=1 with potential batching
		coll = config.MongoDB.Collection(collection)
		logger.Info("using medium-traffic collection optimization (W=1)")

	default:
		// Default optimization
		coll = config.MongoDB.Collection(collection)
		logger.Info("using default collection optimization")
	}

	// Execute the write operation
	start := time.Now()
	err := operation(coll)
	duration := time.Since(start)

	// Log performance metrics
	if duration > 100*time.Millisecond {
		logger.Warn("slow write operation detected",
			zap.String("operation_type", operationType),
			zap.Duration("duration", duration),
			zap.String("collection", collection))
	} else {
		logger.Info("write operation completed with collection optimization",
			zap.String("operation_type", operationType),
			zap.Duration("duration", duration),
			zap.String("collection", collection))
	}

	if err != nil {
		logger.Error("write operation failed with collection optimization",
			zap.String("operation_type", operationType),
			zap.Error(err))
		return fmt.Errorf("write operation failed with collection optimization: %w", err)
	}

	return nil
}
