package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// OptimisticLockError represents an optimistic locking conflict
type OptimisticLockError struct {
	Resource string
	Message  string
}

func (e OptimisticLockError) Error() string {
	return fmt.Sprintf("optimistic lock conflict for %s: %s", e.Resource, e.Message)
}

// VersionedDocument represents a document with version control
type VersionedDocument struct {
	Version   int32     `bson:"version" json:"version"`
	UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}

// OptimisticUpdateResult represents the result of an optimistic update
type OptimisticUpdateResult struct {
	ModifiedCount int64
	Version       int32
	UpdatedAt     time.Time
}

// UpdateWithOptimisticLock performs an update with optimistic locking
func UpdateWithOptimisticLock(ctx context.Context, collection string, filter bson.M, update bson.M, expectedVersion int32) (*OptimisticUpdateResult, error) {
	logger := logging.Logger.With(
		zap.String("collection", collection),
		zap.Int32("expected_version", expectedVersion),
	)

	// Add version check to filter
	filter["version"] = expectedVersion

	// Add version increment to update
	newVersion := expectedVersion + 1
	now := time.Now()

	// Ensure update is a $set operation
	if update["$set"] == nil {
		update["$set"] = bson.M{}
	}

	update["$set"].(bson.M)["version"] = newVersion
	update["$set"].(bson.M)["updated_at"] = now

	// Perform the update
	result, err := config.MongoDB.Collection(collection).UpdateOne(ctx, filter, update)
	if err != nil {
		logger.Error("failed to perform optimistic update", zap.Error(err))
		return nil, fmt.Errorf("failed to perform optimistic update: %w", err)
	}

	// Check if document was modified
	if result.ModifiedCount == 0 {
		// Build filter without version to check if document exists
		checkFilter := bson.M{}
		for k, v := range filter {
			if k != "version" {
				checkFilter[k] = v
			}
		}

		// Check if document exists with different version
		var existingDoc bson.M
		err := config.MongoDB.Collection(collection).FindOne(ctx, checkFilter).Decode(&existingDoc)
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("document not found")
		}
		if err != nil {
			logger.Error("failed to check existing document", zap.Error(err))
			return nil, fmt.Errorf("failed to check existing document: %w", err)
		}

		// Document exists but version doesn't match
		existingVersion, _ := existingDoc["version"].(int32)
		logger.Warn("optimistic lock conflict detected",
			zap.Int32("expected_version", expectedVersion),
			zap.Int32("actual_version", existingVersion))

		return nil, OptimisticLockError{
			Resource: collection,
			Message:  fmt.Sprintf("expected version %d, but document has version %d", expectedVersion, existingVersion),
		}
	}

	logger.Info("optimistic update successful",
		zap.Int64("modified_count", result.ModifiedCount),
		zap.Int32("new_version", newVersion))

	return &OptimisticUpdateResult{
		ModifiedCount: result.ModifiedCount,
		Version:       newVersion,
		UpdatedAt:     now,
	}, nil
}

// UpdateSelfDeclaredWithOptimisticLock updates self-declared data with optimistic locking
func UpdateSelfDeclaredWithOptimisticLock(ctx context.Context, cpf string, update bson.M, expectedVersion int32) (*OptimisticUpdateResult, error) {
	filter := bson.M{"cpf": cpf}
	return UpdateWithOptimisticLock(ctx, config.AppConfig.SelfDeclaredCollection, filter, update, expectedVersion)
}

// UpdateUserConfigWithOptimisticLock updates user config with optimistic locking
func UpdateUserConfigWithOptimisticLock(ctx context.Context, cpf string, update bson.M, expectedVersion int32) (*OptimisticUpdateResult, error) {
	filter := bson.M{"cpf": cpf}
	return UpdateWithOptimisticLock(ctx, config.AppConfig.UserConfigCollection, filter, update, expectedVersion)
}

// GetDocumentVersion gets the current version of a document
func GetDocumentVersion(ctx context.Context, collection string, filter bson.M) (int32, error) {
	var doc bson.M
	err := config.MongoDB.Collection(collection).FindOne(ctx, filter, options.FindOne().SetProjection(bson.M{"version": 1})).Decode(&doc)
	if err != nil {
		return 0, err
	}

	version, ok := doc["version"].(int32)
	if !ok {
		return 0, fmt.Errorf("version field not found or invalid type")
	}

	return version, nil
}

// GetSelfDeclaredVersion gets the current version of self-declared data
func GetSelfDeclaredVersion(ctx context.Context, cpf string) (int32, error) {
	filter := bson.M{"cpf": cpf}
	return GetDocumentVersion(ctx, config.AppConfig.SelfDeclaredCollection, filter)
}

// GetUserConfigVersion gets the current version of user config
func GetUserConfigVersion(ctx context.Context, cpf string) (int32, error) {
	filter := bson.M{"cpf": cpf}
	return GetDocumentVersion(ctx, config.AppConfig.UserConfigCollection, filter)
}

// RetryWithOptimisticLock retries an operation with exponential backoff on optimistic lock conflicts
func RetryWithOptimisticLock(ctx context.Context, maxRetries int, operation func() error) error {
	logger := logging.Logger.With(zap.String("operation", "retry_with_optimistic_lock"))

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		// Check if it's an optimistic lock error
		if _, ok := err.(OptimisticLockError); ok {
			if attempt == maxRetries {
				logger.Error("max retries reached for optimistic lock",
					zap.Int("attempts", attempt+1),
					zap.Error(err))
				return err
			}

			// Exponential backoff: wait 2^attempt * 100ms
			backoff := time.Duration(1<<attempt) * 100 * time.Millisecond
			logger.Info("optimistic lock conflict, retrying",
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff))

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}

		// If it's not an optimistic lock error, return immediately
		return err
	}

	return fmt.Errorf("max retries exceeded")
}

// InitializeDocumentVersion initializes version field for existing documents
func InitializeDocumentVersion(ctx context.Context, collection string, filter bson.M) error {
	logger := logging.Logger.With(zap.String("collection", collection))

	// Find documents without version field
	noVersionFilter := bson.M{}
	for k, v := range filter {
		noVersionFilter[k] = v
	}
	noVersionFilter["version"] = bson.M{"$exists": false}

	// Update them to have version 1
	update := bson.M{
		"$set": bson.M{
			"version":    1,
			"updated_at": time.Now(),
		},
	}

	result, err := config.MongoDB.Collection(collection).UpdateMany(ctx, noVersionFilter, update)
	if err != nil {
		logger.Error("failed to initialize document versions", zap.Error(err))
		return fmt.Errorf("failed to initialize document versions: %w", err)
	}

	if result.ModifiedCount > 0 {
		logger.Info("initialized document versions",
			zap.Int64("modified_count", result.ModifiedCount))
	}

	return nil
}
