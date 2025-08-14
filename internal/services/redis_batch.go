package services

import (
	"context"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisBatch provides efficient batching for Redis operations
type RedisBatch struct {
	operations []func(redis.Pipeliner)
	keys       []string
	logger     *zap.Logger
}

// NewRedisBatch creates a new Redis batch operation helper
func NewRedisBatch(logger *zap.Logger) *RedisBatch {
	return &RedisBatch{
		operations: make([]func(redis.Pipeliner), 0, 100),
		keys:       make([]string, 0, 100),
		logger:     logger,
	}
}

// AddGet adds a GET operation to the batch
func (rb *RedisBatch) AddGet(key string) {
	rb.keys = append(rb.keys, key)
	rb.operations = append(rb.operations, func(pipe redis.Pipeliner) {
		pipe.Get(context.Background(), key)
	})
}

// AddSet adds a SET operation to the batch
func (rb *RedisBatch) AddSet(key string, value interface{}, expiration time.Duration) {
	rb.keys = append(rb.keys, key)
	rb.operations = append(rb.operations, func(pipe redis.Pipeliner) {
		pipe.Set(context.Background(), key, value, expiration)
	})
}

// AddDel adds a DEL operation to the batch
func (rb *RedisBatch) AddDel(keys ...string) {
	rb.keys = append(rb.keys, keys...)
	rb.operations = append(rb.operations, func(pipe redis.Pipeliner) {
		pipe.Del(context.Background(), keys...)
	})
}

// Execute runs all batched operations in a single pipeline
func (rb *RedisBatch) Execute(ctx context.Context) ([]redis.Cmder, error) {
	if len(rb.operations) == 0 {
		return nil, nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add all operations to pipeline
	for _, op := range rb.operations {
		op(pipe)
	}

	// Execute pipeline
	cmds, err := pipe.Exec(ctx)
	duration := time.Since(start)

	rb.logger.Debug("executed Redis batch pipeline",
		zap.Int("operations_count", len(rb.operations)),
		zap.Int("keys_count", len(rb.keys)),
		zap.Duration("duration", duration),
		zap.Bool("success", err == nil))

	if err != nil {
		rb.logger.Error("Redis batch pipeline failed",
			zap.Error(err),
			zap.Int("operations_count", len(rb.operations)))
		return nil, fmt.Errorf("pipeline execution failed: %w", err)
	}

	// Reset batch for reuse
	rb.operations = rb.operations[:0]
	rb.keys = rb.keys[:0]

	return cmds, nil
}

// Size returns the number of operations in the batch
func (rb *RedisBatch) Size() int {
	return len(rb.operations)
}

// Clear resets the batch
func (rb *RedisBatch) Clear() {
	rb.operations = rb.operations[:0]
	rb.keys = rb.keys[:0]
}

// BatchReadMultiple efficiently reads multiple keys using pipeline
func BatchReadMultiple(ctx context.Context, keys []string, logger *zap.Logger) (map[string]string, error) {
	if len(keys) == 0 {
		return make(map[string]string), nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add all GET commands to pipeline
	for _, key := range keys {
		pipe.Get(ctx, key)
	}

	// Execute pipeline
	cmds, err := pipe.Exec(ctx)
	duration := time.Since(start)

	logger.Debug("batch read multiple keys",
		zap.Int("keys_count", len(keys)),
		zap.Duration("duration", duration),
		zap.Bool("success", err == nil))

	if err != nil && err != redis.Nil {
		logger.Error("batch read failed", zap.Error(err))
		return nil, fmt.Errorf("batch read failed: %w", err)
	}

	// Process results
	results := make(map[string]string, len(keys))
	for i, cmd := range cmds {
		if i < len(keys) {
			if stringCmd, ok := cmd.(*redis.StringCmd); ok {
				val, err := stringCmd.Result()
				if err == nil {
					results[keys[i]] = val
				}
			}
		}
	}

	return results, nil
}

// BatchWriteMultiple efficiently writes multiple key-value pairs using pipeline
func BatchWriteMultiple(ctx context.Context, data map[string]interface{}, expiration time.Duration, logger *zap.Logger) error {
	if len(data) == 0 {
		return nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add all SET commands to pipeline
	for key, value := range data {
		pipe.Set(ctx, key, value, expiration)
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	duration := time.Since(start)

	logger.Debug("batch write multiple keys",
		zap.Int("keys_count", len(data)),
		zap.Duration("duration", duration),
		zap.Bool("success", err == nil))

	if err != nil {
		logger.Error("batch write failed", zap.Error(err))
		return fmt.Errorf("batch write failed: %w", err)
	}

	return nil
}
