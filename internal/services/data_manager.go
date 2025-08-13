package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// DataOperation represents a generic data operation
type DataOperation interface {
	GetKey() string
	GetCollection() string
	GetData() interface{}
	GetTTL() time.Duration
	GetType() string
}

// DataManager handles Redis/MongoDB operations with multi-level caching
type DataManager struct {
	redis  *redisclient.Client
	mongo  *mongo.Database
	logger *logging.SafeLogger
}

// NewDataManager creates a new data manager instance
func NewDataManager(redis *redisclient.Client, mongo *mongo.Database, logger *logging.SafeLogger) *DataManager {
	return &DataManager{
		redis:  redis,
		mongo:  mongo,
		logger: logger,
	}
}

// Write writes data to Redis write buffer and queues for MongoDB sync
func (dm *DataManager) Write(ctx context.Context, op DataOperation) error {
	// 1. Write to Redis write buffer
	writeKey := fmt.Sprintf("%s:write:%s", op.GetType(), op.GetKey())
	dataBytes, err := json.Marshal(op.GetData())
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Write to Redis with TTL (24 hours for write buffer)
	err = dm.redis.Set(ctx, writeKey, string(dataBytes), 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to write to Redis buffer: %w", err)
	}

	// 2. Queue sync job
	syncJob := SyncJob{
		ID:         utils.GenerateUUID(),
		Type:       op.GetType(),
		Key:        op.GetKey(),
		Collection: op.GetCollection(),
		Data:       op.GetData(),
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	jobBytes, err := json.Marshal(syncJob)
	if err != nil {
		return fmt.Errorf("failed to marshal sync job: %w", err)
	}

	// Push to Redis queue
	queueKey := fmt.Sprintf("sync:queue:%s", op.GetType())
	err = dm.redis.LPush(ctx, queueKey, string(jobBytes)).Err()
	if err != nil {
		return fmt.Errorf("failed to queue sync job: %w", err)
	}

	dm.logger.Info("data written to cache and queued for sync",
		zap.String("type", op.GetType()),
		zap.String("key", op.GetKey()),
		zap.String("collection", op.GetCollection()))

	return nil
}

// Read reads data from cache layers, falling back to MongoDB
func (dm *DataManager) Read(ctx context.Context, key string, collection string, dataType string, result interface{}) error {
	// 1. Check Redis write buffer first (most recent data)
	writeKey := fmt.Sprintf("%s:write:%s", dataType, key)
	if data, err := dm.redis.Get(ctx, writeKey).Result(); err == nil {
		if err := json.Unmarshal([]byte(data), result); err == nil {
			dm.logger.Debug("data read from write buffer",
				zap.String("type", dataType),
				zap.String("key", key))
			return nil
		}
	}

	// 2. Check Redis read cache
	cacheKey := fmt.Sprintf("%s:cache:%s", dataType, key)
	if data, err := dm.redis.Get(ctx, cacheKey).Result(); err == nil {
		if err := json.Unmarshal([]byte(data), result); err == nil {
			dm.logger.Debug("data read from cache",
				zap.String("type", dataType),
				zap.String("key", key))
			return nil
		}
	}

	// 3. Fall back to MongoDB
	err := dm.mongo.Collection(collection).FindOne(ctx, bson.M{"_id": key}).Decode(result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("document not found: %s", key)
		}
		return fmt.Errorf("failed to read from MongoDB: %w", err)
	}

	// 4. Cache in Redis for future reads
	dataBytes, err := json.Marshal(result)
	if err == nil {
		// Cache with TTL (1 hour for read cache)
		cacheKey := fmt.Sprintf("%s:cache:%s", dataType, key)
		dm.redis.Set(ctx, cacheKey, string(dataBytes), 1*time.Hour)
	}

	dm.logger.Debug("data read from MongoDB and cached",
		zap.String("type", dataType),
		zap.String("key", key),
		zap.String("collection", collection))

	return nil
}

// Delete removes data from all cache layers and MongoDB
func (dm *DataManager) Delete(ctx context.Context, key string, collection string, dataType string) error {
	// 1. Remove from Redis write buffer
	writeKey := fmt.Sprintf("%s:write:%s", dataType, key)
	dm.redis.Del(ctx, writeKey)

	// 2. Remove from Redis read cache
	cacheKey := fmt.Sprintf("%s:cache:%s", dataType, key)
	dm.redis.Del(ctx, cacheKey)

	// 3. Delete from MongoDB
	_, err := dm.mongo.Collection(collection).DeleteOne(ctx, bson.M{"_id": key})
	if err != nil {
		return fmt.Errorf("failed to delete from MongoDB: %w", err)
	}

	dm.logger.Info("data deleted from all layers",
		zap.String("type", dataType),
		zap.String("key", key),
		zap.String("collection", collection))

	return nil
}

// CleanupWriteBuffer removes data from write buffer after successful MongoDB sync
func (dm *DataManager) CleanupWriteBuffer(ctx context.Context, dataType string, key string) error {
	writeKey := fmt.Sprintf("%s:write:%s", dataType, key)
	err := dm.redis.Del(ctx, writeKey).Err()
	if err != nil {
		dm.logger.Warn("failed to cleanup write buffer",
			zap.String("type", dataType),
			zap.String("key", key),
			zap.Error(err))
		return err
	}

	dm.logger.Debug("write buffer cleaned up",
		zap.String("type", dataType),
		zap.String("key", key))

	return nil
}

// UpdateReadCache updates the read cache with fresh data
func (dm *DataManager) UpdateReadCache(ctx context.Context, dataType string, key string, data interface{}) error {
	cacheKey := fmt.Sprintf("%s:cache:%s", dataType, key)
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data for cache: %w", err)
	}

	// Update cache with TTL (1 hour)
	err = dm.redis.Set(ctx, cacheKey, string(dataBytes), 1*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to update read cache: %w", err)
	}

	dm.logger.Debug("read cache updated",
		zap.String("type", dataType),
		zap.String("key", key))

	return nil
}
