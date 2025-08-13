package utils

import (
	"context"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.uber.org/zap"
)

// RedisPipeline provides utilities for batch Redis operations
type RedisPipeline struct {
	ctx context.Context
}

// NewRedisPipeline creates a new Redis pipeline utility
func NewRedisPipeline(ctx context.Context) *RedisPipeline {
	return &RedisPipeline{ctx: ctx}
}

// BatchDelete deletes multiple keys using Redis pipeline
func (rp *RedisPipeline) BatchDelete(keys []string) error {
	if len(keys) == 0 {
		return nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add delete operations to pipeline
	for _, key := range keys {
		pipe.Del(rp.ctx, key)
	}

	// Execute pipeline
	cmds, err := pipe.Exec(rp.ctx)
	if err != nil {
		logging.Logger.Error("failed to execute Redis pipeline",
			zap.Error(err),
			zap.Int("key_count", len(keys)))
		return err
	}

	// Log performance metrics
	duration := time.Since(start)
	logging.Logger.Debug("Redis pipeline batch delete completed",
		zap.Int("key_count", len(keys)),
		zap.Int("command_count", len(cmds)),
		zap.Duration("duration", duration))

	return nil
}

// BatchSet sets multiple key-value pairs using Redis pipeline
func (rp *RedisPipeline) BatchSet(keyValues map[string]interface{}, ttl time.Duration) error {
	if len(keyValues) == 0 {
		return nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add set operations to pipeline
	for key, value := range keyValues {
		pipe.Set(rp.ctx, key, value, ttl)
	}

	// Execute pipeline
	cmds, err := pipe.Exec(rp.ctx)
	if err != nil {
		logging.Logger.Error("failed to execute Redis pipeline",
			zap.Error(err),
			zap.Int("key_count", len(keyValues)))
		return err
	}

	// Log performance metrics
	duration := time.Since(start)
	logging.Logger.Debug("Redis pipeline batch set completed",
		zap.Int("key_count", len(keyValues)),
		zap.Int("command_count", len(cmds)),
		zap.Duration("duration", duration))

	return nil
}

// BatchGet retrieves multiple values using Redis pipeline
func (rp *RedisPipeline) BatchGet(keys []string) (map[string]interface{}, error) {
	if len(keys) == 0 {
		return make(map[string]interface{}), nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add get operations to pipeline
	cmds := make([]interface{}, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Get(rp.ctx, key)
	}

	// Execute pipeline
	_, err := pipe.Exec(rp.ctx)
	if err != nil {
		logging.Logger.Error("failed to execute Redis pipeline",
			zap.Error(err),
			zap.Int("key_count", len(keys)))
		return nil, err
	}

	// Extract results
	results := make(map[string]interface{})
	for i, cmd := range cmds {
		if getCmd, ok := cmd.(interface{ Val() interface{} }); ok {
			results[keys[i]] = getCmd.Val()
		}
	}

	// Log performance metrics
	duration := time.Since(start)
	logging.Logger.Debug("Redis pipeline batch get completed",
		zap.Int("key_count", len(keys)),
		zap.Int("result_count", len(results)),
		zap.Duration("duration", duration))

	return results, nil
}

// BatchExpire sets expiration for multiple keys using Redis pipeline
func (rp *RedisPipeline) BatchExpire(keys []string, ttl time.Duration) error {
	if len(keys) == 0 {
		return nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add expire operations to pipeline
	for _, key := range keys {
		pipe.Expire(rp.ctx, key, ttl)
	}

	// Execute pipeline
	cmds, err := pipe.Exec(rp.ctx)
	if err != nil {
		logging.Logger.Error("failed to execute Redis pipeline",
			zap.Error(err),
			zap.Int("key_count", len(keys)))
		return err
	}

	// Log performance metrics
	duration := time.Since(start)
	logging.Logger.Debug("Redis pipeline batch expire completed",
		zap.Int("key_count", len(keys)),
		zap.Int("command_count", len(cmds)),
		zap.Duration("duration", duration))

	return nil
}

// BatchExists checks existence of multiple keys using Redis pipeline
func (rp *RedisPipeline) BatchExists(keys []string) (map[string]bool, error) {
	if len(keys) == 0 {
		return make(map[string]bool), nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add exists operations to pipeline
	cmds := make([]interface{}, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Exists(rp.ctx, key)
	}

	// Execute pipeline
	_, err := pipe.Exec(rp.ctx)
	if err != nil {
		logging.Logger.Error("failed to execute Redis pipeline",
			zap.Error(err),
			zap.Int("key_count", len(keys)))
		return nil, err
	}

	// Extract results
	results := make(map[string]bool)
	for i, cmd := range cmds {
		if existsCmd, ok := cmd.(interface{ Val() int64 }); ok {
			results[keys[i]] = existsCmd.Val() > 0
		}
	}

	// Log performance metrics
	duration := time.Since(start)
	logging.Logger.Debug("Redis pipeline batch exists completed",
		zap.Int("key_count", len(keys)),
		zap.Int("result_count", len(results)),
		zap.Duration("duration", duration))

	return results, nil
}

// BatchIncr increments multiple counters using Redis pipeline
func (rp *RedisPipeline) BatchIncr(keys []string) (map[string]int64, error) {
	if len(keys) == 0 {
		return make(map[string]int64), nil
	}

	start := time.Now()
	pipe := config.Redis.Pipeline()

	// Add incr operations to pipeline
	cmds := make([]interface{}, len(keys))
	for i, key := range keys {
		cmds[i] = pipe.Incr(rp.ctx, key)
	}

	// Execute pipeline
	_, err := pipe.Exec(rp.ctx)
	if err != nil {
		logging.Logger.Error("failed to execute Redis pipeline",
			zap.Error(err),
			zap.Int("key_count", len(keys)))
		return nil, err
	}

	// Extract results
	results := make(map[string]int64)
	for i, cmd := range cmds {
		if incrCmd, ok := cmd.(interface{ Val() int64 }); ok {
			results[keys[i]] = incrCmd.Val()
		}
	}

	// Log performance metrics
	duration := time.Since(start)
	logging.Logger.Debug("Redis pipeline batch incr completed",
		zap.Int("key_count", len(keys)),
		zap.Int("result_count", len(results)),
		zap.Duration("duration", duration))

	return results, nil
}
