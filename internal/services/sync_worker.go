package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
	case "user_configs":
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
	// Clean up the write buffer
	writeKey := fmt.Sprintf("%s:write:%s", job.Type, job.Key)
	w.redis.Del(context.Background(), writeKey)

	// Update the read cache with the synced data
	cacheKey := fmt.Sprintf("%s:cache:%s", job.Type, job.Key)
	dataBytes, _ := json.Marshal(job.Data)
	w.redis.Set(context.Background(), cacheKey, string(dataBytes), 1*time.Hour)

	w.logger.Debug("sync job succeeded",
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
