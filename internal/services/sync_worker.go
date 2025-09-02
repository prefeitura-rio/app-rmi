package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// SyncWorker processes sync jobs from Redis queues
type SyncWorker struct {
	id           int
	redis        *redisclient.Client
	mongo        *mongo.Database
	logger       *logging.SafeLogger
	metrics      *Metrics
	degradedMode *DegradedMode
	stopChan     chan struct{}
	queues       []string
}

// NewSyncWorker creates a new sync worker
func NewSyncWorker(redis *redisclient.Client, mongo *mongo.Database, id int, logger *logging.SafeLogger, metrics *Metrics, degradedMode *DegradedMode) *SyncWorker {
	return &SyncWorker{
		id:           id,
		redis:        redis,
		mongo:        mongo,
		logger:       logger,
		metrics:      metrics,
		degradedMode: degradedMode,
		stopChan:     make(chan struct{}),
		queues: []string{
			"citizen",
			"phone_mapping",
			"user_config",
			"opt_in_history",
			"beta_group",
			"phone_verification",
			"maintenance_request",
			"self_declared_address",
			"self_declared_email",
			"self_declared_phone",
			"self_declared_raca",
			"cf_lookup",
		},
	}
}

// Start starts the worker
func (w *SyncWorker) Start() {
	w.logger.Info("sync worker started", zap.Int("worker_id", w.id))

	// Use a ticker for more efficient timing instead of sleep
	ticker := time.NewTicker(50 * time.Millisecond) // Reduced delay for better responsiveness
	defer ticker.Stop()

	for {
		select {
		case <-w.stopChan:
			w.logger.Info("sync worker stopped", zap.Int("worker_id", w.id))
			return
		case <-ticker.C:
			w.processQueuesParallel()
		}
	}
}

// Stop stops the worker
func (w *SyncWorker) Stop() {
	close(w.stopChan)
}

// processQueuesParallel processes all queues for available jobs in parallel
func (w *SyncWorker) processQueuesParallel() {
	// Skip if in degraded mode
	if w.degradedMode.IsActive() {
		w.logger.Debug("skipping all queue processing due to degraded mode",
			zap.String("reason", w.degradedMode.GetReason()))
		return
	}

	// Process a limited number of jobs per cycle to prevent overwhelming
	const maxJobsPerCycle = 3
	jobsProcessed := 0

	// Use round-robin approach to fairly distribute processing across queues
	for _, queue := range w.queues {
		if jobsProcessed >= maxJobsPerCycle {
			break
		}

		// Non-blocking job retrieval
		job, err := w.getJobNonBlocking(queue)
		if err != nil {
			w.logger.Debug("error getting job from queue",
				zap.String("queue", queue),
				zap.Error(err))
			continue
		}

		if job != nil {
			w.logger.Debug("found job to process",
				zap.String("queue", queue),
				zap.String("job_id", job.ID))

			// Process job in current goroutine to maintain order and avoid overwhelming the system
			w.processJob(job)
			jobsProcessed++
		}
	}

	if jobsProcessed > 0 {
		w.logger.Debug("processed jobs in cycle", zap.Int("jobs_processed", jobsProcessed))
	}
}

// getJobNonBlocking gets a job from a specific queue without blocking
func (w *SyncWorker) getJobNonBlocking(queue string) (*SyncJob, error) {
	queueKey := fmt.Sprintf("sync:queue:%s", queue)

	// Use RPOP (non-blocking) instead of BRPop
	result, err := w.redis.RPop(context.Background(), queueKey).Result()
	if err != nil {
		if err.Error() == "redis: nil" {
			return nil, nil // No jobs available
		}
		return nil, err
	}

	// Parse the job
	var job SyncJob
	if err := json.Unmarshal([]byte(result), &job); err != nil {
		w.logger.Error("failed to unmarshal sync job",
			zap.String("queue", queue),
			zap.Error(err))
		return nil, err
	}

	return &job, nil
}

// processJob processes a single sync job
func (w *SyncWorker) processJob(job *SyncJob) {
	start := time.Now()

	w.logger.Info("processing sync job",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("key", job.Key),
		zap.String("collection", job.Collection))

	// Try to sync to MongoDB
	err := w.syncToMongoDB(job)

	duration := time.Since(start)

	if err != nil {
		w.handleSyncFailure(job, err)
		w.metrics.IncrementSyncFailures(job.Type)
	} else {
		w.handleSyncSuccess(job)
		w.metrics.IncrementSyncOperations(job.Type)
	}

	w.logger.Info("sync job completed",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("key", job.Key),
		zap.Duration("duration", duration),
		zap.Error(err))
}

// syncToMongoDB syncs a job to MongoDB
func (w *SyncWorker) syncToMongoDB(job *SyncJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Handle special job types
	if err := w.handleSpecialJobTypes(ctx, job); err != nil {
		if err.Error() == "not_special_job" {
			// Continue with normal processing
		} else {
			return err
		}
	} else {
		// Special job was handled successfully
		return nil
	}

	// Convert the job data to BSON
	dataBytes, err := json.Marshal(job.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal job data: %w", err)
	}

	var bsonData bson.M
	if err := json.Unmarshal(dataBytes, &bsonData); err != nil {
		return fmt.Errorf("failed to unmarshal to BSON: %w", err)
	}

	// Use appropriate filter based on collection type to match unique indexes
	var filter bson.M
	switch job.Collection {
	case "citizens":
		filter = bson.M{"cpf": job.Key}
	case "self_declared":
		filter = bson.M{"cpf": job.Key}
	case config.AppConfig.UserConfigCollection:
		filter = bson.M{"cpf": job.Key}
	case "phone_cpf_mappings":
		filter = bson.M{"phone_number": job.Key}
	case "opt_in_histories":
		filter = bson.M{"cpf": job.Key}
	case "beta_groups":
		filter = bson.M{"cpf": job.Key}
	case "phone_verifications":
		filter = bson.M{"phone_number": job.Key}
	case "maintenance_requests":
		filter = bson.M{"cpf": job.Key}
	default:
		// Default to _id for other collections
		filter = bson.M{"_id": job.Key}
	}

	update := bson.M{"$set": bsonData}
	opts := options.Update().SetUpsert(true)

	_, err = w.mongo.Collection(job.Collection).UpdateOne(ctx, filter, update, opts)
	if err != nil {
		// Check if it's a duplicate key error - this is expected and not an error
		if mongo.IsDuplicateKeyError(err) {
			w.logger.Debug("duplicate key during sync - data already exists",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.String("key", job.Key),
				zap.String("collection", job.Collection))
			// Return nil because this is not an error - the data already exists
			return nil
		}
		return fmt.Errorf("failed to sync to MongoDB: %w", err)
	}

	return nil
}

// handleSyncSuccess handles a successful sync
func (w *SyncWorker) handleSyncSuccess(job *SyncJob) {
	ctx := context.Background()

	// First, update the read cache with synced data (increased TTL to match DataManager)
	cacheKey := fmt.Sprintf("%s:cache:%s", job.Type, job.Key)
	dataBytes, err := json.Marshal(job.Data)
	if err != nil {
		w.logger.Error("failed to marshal data for cache update",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("key", job.Key),
			zap.Error(err))
	} else {
		// Use 3 hours TTL to match DataManager read cache
		err = w.redis.Set(ctx, cacheKey, string(dataBytes), 3*time.Hour).Err()
		if err != nil {
			w.logger.Error("failed to update read cache after sync",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.String("key", job.Key),
				zap.Error(err))
		} else {
			w.logger.Debug("updated read cache after successful sync",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.String("key", job.Key),
				zap.String("cache_key", cacheKey))
		}
	}

	// Now clean up the write buffer (only after cache is updated)
	writeKey := fmt.Sprintf("%s:write:%s", job.Type, job.Key)
	err = w.redis.Del(ctx, writeKey).Err()
	if err != nil {
		w.logger.Warn("failed to cleanup write buffer after sync",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("key", job.Key),
			zap.String("write_key", writeKey),
			zap.Error(err))
	} else {
		w.logger.Debug("cleaned up write buffer after successful sync",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("key", job.Key),
			zap.String("write_key", writeKey))
	}

	w.logger.Info("sync job succeeded - cache updated and write buffer cleaned",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("key", job.Key))
}

// handleSyncFailure handles a failed sync
func (w *SyncWorker) handleSyncFailure(job *SyncJob, err error) {
	job.RetryCount++

	if job.RetryCount >= job.MaxRetries {
		// Move to dead letter queue
		w.moveToDLQ(job, err)
	} else {
		// Re-queue with backoff
		w.requeueJob(job)
	}
}

// moveToDLQ moves a failed job to the dead letter queue
func (w *SyncWorker) moveToDLQ(job *SyncJob, err error) {
	dlqJob := DLQJob{
		OriginalJob: *job,
		Error:       err.Error(),
		FailedAt:    time.Now(),
	}

	dlqBytes, _ := json.Marshal(dlqJob)
	dlqKey := fmt.Sprintf("sync:dlq:%s", job.Type)

	w.redis.LPush(context.Background(), dlqKey, string(dlqBytes))

	w.logger.Error("job moved to DLQ",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("key", job.Key),
		zap.Error(err))
}

// requeueJob re-queues a job for retry
func (w *SyncWorker) requeueJob(job *SyncJob) {
	// Add exponential backoff delay
	backoffDelay := time.Duration(job.RetryCount) * 5 * time.Second
	if backoffDelay > 60*time.Second {
		backoffDelay = 60 * time.Second
	}

	// Re-queue with delay
	time.Sleep(backoffDelay)

	jobBytes, _ := json.Marshal(job)
	queueKey := fmt.Sprintf("sync:queue:%s", job.Type)

	w.redis.LPush(context.Background(), queueKey, string(jobBytes))

	w.logger.Info("job re-queued for retry",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("key", job.Key),
		zap.Int("retry_count", job.RetryCount),
		zap.Duration("backoff_delay", backoffDelay))
}

// handleSpecialJobTypes handles special job types that don't follow normal MongoDB sync pattern
func (w *SyncWorker) handleSpecialJobTypes(ctx context.Context, job *SyncJob) error {
	// Check if this is an avatar cleanup job
	if data, ok := job.Data.(map[string]interface{}); ok {
		if jobType, exists := data["type"]; exists && jobType == "avatar_cleanup" {
			return w.handleAvatarCleanup(ctx, data)
		}
	}
	
	// Check if this is a CF lookup job (identified by job type or collection)
	if job.Type == "cf_lookup" || job.Collection == "cf_lookup" {
		return w.handleCFLookupJob(ctx, job)
	}
	
	// Not a special job type
	return fmt.Errorf("not_special_job")
}

// handleAvatarCleanup handles orphaned avatar cleanup jobs
func (w *SyncWorker) handleAvatarCleanup(ctx context.Context, data map[string]interface{}) error {
	avatarID, ok := data["avatar_id"].(string)
	if !ok {
		return fmt.Errorf("invalid avatar_id in cleanup job")
	}

	w.logger.Info("processing avatar cleanup job", zap.String("avatar_id", avatarID))

	// Reset all user configs that reference this deleted avatar
	userConfigCollection := w.mongo.Collection(config.AppConfig.UserConfigCollection)
	
	filter := bson.M{"avatar_id": avatarID}
	update := bson.M{
		"$unset": bson.M{"avatar_id": ""},
		"$set":   bson.M{"updated_at": time.Now()},
	}
	
	result, err := userConfigCollection.UpdateMany(ctx, filter, update)
	if err != nil {
		w.logger.Error("failed to cleanup avatar references", 
			zap.Error(err), 
			zap.String("avatar_id", avatarID))
		return fmt.Errorf("failed to cleanup avatar references: %w", err)
	}

	w.logger.Info("avatar cleanup completed",
		zap.String("avatar_id", avatarID),
		zap.Int64("affected_users", result.ModifiedCount))

	// Clear any cached user configs that might reference this avatar
	// Since we don't know which users were affected, we'll let cache entries expire naturally
	// or clear them individually when accessed
	
	return nil
}

// handleCFLookupJob handles CF lookup jobs
func (w *SyncWorker) handleCFLookupJob(ctx context.Context, job *SyncJob) error {
	w.logger.Info("processing CF lookup job", zap.String("job_id", job.ID))

	// Extract CPF and address from job data
	data, ok := job.Data.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid job data format for CF lookup")
	}

	cpf, ok := data["cpf"].(string)
	if !ok || cpf == "" {
		return fmt.Errorf("missing or invalid CPF in CF lookup job")
	}

	address, ok := data["address"].(string)
	if !ok || address == "" {
		return fmt.Errorf("missing or invalid address in CF lookup job")
	}

	w.logger.Debug("extracted CF lookup job data", 
		zap.String("cpf", cpf),
		zap.String("address", address))

	// Perform CF lookup using the CF lookup service
	if CFLookupServiceInstance == nil {
		return fmt.Errorf("CF lookup service not initialized")
	}

	err := CFLookupServiceInstance.PerformCFLookup(ctx, cpf, address)
	if err != nil {
		w.logger.Error("CF lookup failed", 
			zap.Error(err),
			zap.String("cpf", cpf),
			zap.String("address", address))
		return fmt.Errorf("CF lookup failed: %w", err)
	}

	w.logger.Info("CF lookup completed successfully", 
		zap.String("cpf", cpf),
		zap.String("address", address))

	// Invalidate wallet cache so fresh wallet requests get the new CF data
	// Note: We don't invalidate citizen cache since CF data only appears in wallet endpoint
	walletCacheKey := fmt.Sprintf("citizen_wallet:%s", cpf)
	
	err = config.Redis.Del(ctx, walletCacheKey).Err()
	if err != nil {
		w.logger.Warn("failed to invalidate wallet cache after CF lookup", 
			zap.Error(err),
			zap.String("cpf", cpf))
		// Don't fail the job for cache invalidation errors
	} else {
		w.logger.Debug("invalidated wallet cache after CF lookup", zap.String("cpf", cpf))
	}

	return nil
}
