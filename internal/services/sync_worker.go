package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"github.com/redis/go-redis/v9"
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
	emailSender  EmailSender
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
		emailSender:  ResolveDefaultEmailSender(logger),
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
			"self_declared_nome_exibicao",
			"self_declared_genero",
			"self_declared_renda_familiar",
			"self_declared_escolaridade",
			"self_declared_deficiencia",
			"cf_lookup",
			MobilidadeInviteEmailQueue,
		},
	}
}

// SetEmailSender overrides the email sender (used by tests).
func (w *SyncWorker) SetEmailSender(sender EmailSender) {
	w.emailSender = sender
}

// Start starts the worker
func (w *SyncWorker) Start() {
	w.logger.Info("sync worker started", zap.Int("worker_id", w.id))
	w.recoverStaleInflightJobs()

	ticker := time.NewTicker(50 * time.Millisecond) // Reduced delay for better responsiveness
	recoverTicker := time.NewTicker(inflightRecoverInterval)
	defer ticker.Stop()
	defer recoverTicker.Stop()

	for {
		select {
		case <-w.stopChan:
			w.logger.Info("sync worker stopped", zap.Int("worker_id", w.id))
			return
		case <-ticker.C:
			w.processQueuesParallel()
		case <-recoverTicker.C:
			w.recoverStaleInflightJobs()
		}
	}
}

// inflightVisibilityTimeout is how long a claimed invite-email job may stay in processing
// before another worker may requeue it. Must exceed typical send latency so rolling deploys
// do not duplicate active work.
var inflightVisibilityTimeout = 5 * time.Minute

// inflightRecoverInterval is how often workers attempt stale inflight recovery.
var inflightRecoverInterval = 1 * time.Minute

// recoverStaleInflightJobs requeues only processing jobs older than the visibility timeout.
// A short Redis lock prevents concurrent recoverers from double-moving the same payload.
func (w *SyncWorker) recoverStaleInflightJobs() {
	ctx := context.Background()
	const maxRecover = 1000

	for _, queue := range w.queues {
		if !usesReliableQueue(queue) {
			continue
		}

		lockKey := syncInflightRecoverLockKey(queue)
		acquired, err := w.redis.SetNX(ctx, lockKey, fmt.Sprintf("worker-%d", w.id), 30*time.Second).Result()
		if err != nil {
			w.logger.Warn("inflight recover lock failed",
				zap.String("queue", queue),
				zap.Error(err))
			continue
		}
		if !acquired {
			continue
		}

		func() {
			defer func() {
				if delErr := w.redis.Del(ctx, lockKey).Err(); delErr != nil {
					w.logger.Debug("inflight recover lock release failed",
						zap.String("queue", queue),
						zap.Error(delErr))
				}
			}()

			processingKey := syncProcessingKey(queue)

			items, err := w.redis.LRange(ctx, processingKey, 0, -1).Result()
			if err != nil {
				w.logger.Error("failed to list inflight sync jobs",
					zap.String("queue", queue),
					zap.Error(err))
				return
			}

			cutoff := time.Now().Add(-inflightVisibilityTimeout).Unix()
			recovered := 0
			for _, item := range items {
				if recovered >= maxRecover {
					w.logger.Error("inflight recovery hit cap; remaining stale jobs wait for next cycle",
						zap.String("queue", queue),
						zap.Int("cap", maxRecover))
					break
				}
				moved, recErr := w.recoverStaleInflightItem(ctx, queue, item, cutoff)
				if recErr != nil {
					w.logger.Error("failed to recover stale inflight job",
						zap.String("queue", queue),
						zap.Error(recErr))
					continue
				}
				if !moved {
					continue
				}
				recovered++
				w.logger.Warn("recovered stale inflight sync job",
					zap.String("queue", queue),
					zap.Int("payload_len", len(item)))
			}
			if recovered > 0 {
				w.logger.Info("recovered stale inflight jobs into main queue",
					zap.String("queue", queue),
					zap.Int("count", recovered))
			}
		}()
	}
}

// usesReliableQueue reports whether the queue uses RPOPLPUSH + processing list (needs hash tags on cluster).
func usesReliableQueue(queue string) bool {
	return queue == MobilidadeInviteEmailQueue
}

func syncQueueKey(queue string) string {
	if usesReliableQueue(queue) {
		// Hash tag keeps queue + processing on the same Redis Cluster slot (RPOPLPUSH).
		return fmt.Sprintf("sync:queue:{%s}", queue)
	}
	return fmt.Sprintf("sync:queue:%s", queue)
}

func syncProcessingKey(queue string) string {
	if usesReliableQueue(queue) {
		return fmt.Sprintf("sync:processing:{%s}", queue)
	}
	return fmt.Sprintf("sync:processing:%s", queue)
}

func syncProcessingClaimKey(queue string) string {
	if usesReliableQueue(queue) {
		return fmt.Sprintf("sync:processing:claimed:{%s}", queue)
	}
	return fmt.Sprintf("sync:processing:claimed:%s", queue)
}

func syncInflightRecoverLockKey(queue string) string {
	if usesReliableQueue(queue) {
		return fmt.Sprintf("sync:lock:recover_inflight:{%s}", queue)
	}
	return fmt.Sprintf("sync:lock:recover_inflight:%s", queue)
}

func syncDLQKey(queue string) string {
	if usesReliableQueue(queue) {
		return fmt.Sprintf("sync:dlq:{%s}", queue)
	}
	return fmt.Sprintf("sync:dlq:%s", queue)
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

// getJobNonBlocking gets a job from a specific queue without blocking.
// Mobilidade invite email uses atomic RPOPLPUSH+ZADD into an inflight/processing list.
func (w *SyncWorker) getJobNonBlocking(queue string) (*SyncJob, error) {
	ctx := context.Background()
	queueKey := syncQueueKey(queue)

	var (
		result       string
		err          error
		fromInflight bool
	)

	if usesReliableQueue(queue) {
		result, err = w.claimReliableJob(ctx, queue)
		fromInflight = true
		if err == nil && result == "" {
			return nil, nil
		}
	} else {
		// Use RPOP (non-blocking) instead of BRPop for other queues
		result, err = w.redis.RPop(ctx, queueKey).Result()
	}
	if err != nil {
		if errors.Is(err, redis.Nil) || err.Error() == "redis: nil" {
			return nil, nil // No jobs available
		}
		return nil, err
	}

	var job SyncJob
	if err := json.Unmarshal([]byte(result), &job); err != nil {
		w.logger.Error("failed to unmarshal sync job",
			zap.String("queue", queue),
			zap.Error(err))
		if fromInflight {
			if remErr := w.ackReliableJob(ctx, queue, result); remErr != nil {
				w.logger.Error("failed to discard poison inflight job",
					zap.String("queue", queue),
					zap.Error(remErr))
			}
		}
		return nil, err
	}

	job.rawRedisPayload = result
	job.fromInflight = fromInflight
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

	if !job.AvailableAt.IsZero() && time.Now().Before(job.AvailableAt) {
		if err := w.returnJobToQueue(job); err != nil {
			w.logger.Error("failed to defer sync job until available_at",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.Time("available_at", job.AvailableAt),
				zap.Error(err))
			// Keep inflight for recovery if we could not put the job back.
			return
		}
		w.logger.Debug("deferred sync job until available_at",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.Time("available_at", job.AvailableAt))
		return
	}

	writeBufferJob, err := w.syncToMongoDB(job)

	duration := time.Since(start)

	if err != nil {
		w.handleSyncFailure(job, err)
		w.metrics.IncrementSyncFailures(job.Type)
	} else {
		// Special jobs (invite email, cf_lookup, avatar cleanup) must not touch write-buffer keys.
		if writeBufferJob {
			w.handleSyncSuccess(job)
		}
		w.ackInflightJob(job)
		w.metrics.IncrementSyncOperations(job.Type)
	}

	w.logger.Info("sync job completed",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("key", job.Key),
		zap.Duration("duration", duration),
		zap.Error(err))
}

// ackInflightJob removes a job from the processing list after success or successful requeue/DLQ.
func (w *SyncWorker) ackInflightJob(job *SyncJob) {
	if job == nil || !job.fromInflight || job.rawRedisPayload == "" {
		return
	}
	ctx := context.Background()
	if err := w.ackReliableJob(ctx, job.Type, job.rawRedisPayload); err != nil {
		w.logger.Error("failed to ack inflight sync job",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("key", job.Key),
			zap.Error(err))
	}
}

// syncToMongoDB syncs a job to MongoDB.
// writeBufferJob is true for normal Redis write-buffer → Mongo syncs that need handleSyncSuccess.
func (w *SyncWorker) syncToMongoDB(job *SyncJob) (writeBufferJob bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Handle special job types
	if err := w.handleSpecialJobTypes(ctx, job); err != nil {
		if err.Error() != "not_special_job" {
			return false, err
		}
	} else {
		// Special job was handled successfully (no write-buffer/cache cleanup).
		return false, nil
	}

	// Convert the job data to BSON
	dataBytes, err := json.Marshal(job.Data)
	if err != nil {
		return true, fmt.Errorf("failed to marshal job data: %w", err)
	}

	var bsonData bson.M
	if err := json.Unmarshal(dataBytes, &bsonData); err != nil {
		return true, fmt.Errorf("failed to unmarshal to BSON: %w", err)
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

	// For self_declared collection, use field-specific updates to avoid overwriting other fields
	var update bson.M
	if job.Collection == "self_declared" {
		fieldName := getFieldNameFromJobType(job.Type)
		if fieldName != "" && bsonData[fieldName] != nil {
			// Only update the specific field and timestamp, preserve other fields
			update = bson.M{
				"$set": bson.M{
					fieldName:    bsonData[fieldName],
					"updated_at": bsonData["updated_at"],
				},
			}
			w.logger.Debug("using field-specific update for self_declared collection",
				zap.String("job_id", job.ID),
				zap.String("job_type", job.Type),
				zap.String("field_name", fieldName),
				zap.String("cpf", job.Key))
		} else {
			// Fallback to full document update if field mapping fails
			update = bson.M{"$set": bsonData}
			w.logger.Warn("falling back to full document update for self_declared",
				zap.String("job_id", job.ID),
				zap.String("job_type", job.Type),
				zap.String("field_name", fieldName),
				zap.String("cpf", job.Key))
		}
	} else {
		// For other collections, update the entire document
		update = bson.M{"$set": bsonData}
	}

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
			return true, nil
		}
		return true, fmt.Errorf("failed to sync to MongoDB: %w", err)
	}

	return true, nil
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

	var persisted bool
	if job.RetryCount >= job.MaxRetries {
		// Move to dead letter queue
		persisted = w.moveToDLQ(job, err)
	} else {
		// Re-queue with backoff
		persisted = w.requeueJob(job)
	}
	// Reliable-queue requeue/DLQ already removes processing+claim atomically.
	// For other paths, ack only after the job is safely back on a Redis list.
	if persisted && (!job.fromInflight || !usesReliableQueue(job.Type)) {
		w.ackInflightJob(job)
	}
}

// moveToDLQ moves a failed job to the dead letter queue.
// Returns true when the job was successfully written to the DLQ.
func (w *SyncWorker) moveToDLQ(job *SyncJob, err error) bool {
	dlqJob := DLQJob{
		OriginalJob: *job,
		Error:       err.Error(),
		FailedAt:    time.Now(),
	}

	dlqBytes, marshalErr := json.Marshal(dlqJob)
	if marshalErr != nil {
		w.logger.Error("failed to marshal DLQ job",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("key", job.Key),
			zap.Error(marshalErr))
		w.markMobilidadeInviteQueuePersistFailure(job, fmt.Sprintf("dlq marshal failed: %v", marshalErr))
		return false
	}
	dlqKey := syncDLQKey(job.Type)
	ctx := context.Background()

	if job.fromInflight && usesReliableQueue(job.Type) && job.rawRedisPayload != "" {
		moved, moveErr := w.completeReliableJobToList(ctx, job.Type, dlqKey, job.rawRedisPayload, string(dlqBytes))
		if moveErr != nil {
			w.logger.Error("failed to move job to DLQ",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.String("key", job.Key),
				zap.Error(moveErr))
			if w.metrics != nil {
				w.metrics.IncrementSyncFailures(job.Type)
			}
			w.markMobilidadeInviteQueuePersistFailure(job, fmt.Sprintf("dlq failed: %v", moveErr))
			return false
		}
		if !moved {
			w.logger.Error("failed to move job to DLQ: inflight payload missing",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.String("key", job.Key))
			w.markMobilidadeInviteQueuePersistFailure(job, "dlq failed: inflight payload missing")
			return false
		}
	} else if pushErr := w.redis.LPush(ctx, dlqKey, string(dlqBytes)).Err(); pushErr != nil {
		w.logger.Error("failed to move job to DLQ",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("key", job.Key),
			zap.Error(pushErr))
		if w.metrics != nil {
			w.metrics.IncrementSyncFailures(job.Type)
		}
		w.markMobilidadeInviteQueuePersistFailure(job, fmt.Sprintf("dlq failed: %v", pushErr))
		return false
	}

	w.logger.Error("job moved to DLQ",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("key", job.Key),
		zap.Error(err))
	return true
}

// requeueJob re-queues a job for retry without blocking the worker goroutine.
// Sets AvailableAt for deferred visibility; processJob returns the job to the queue until then.
// Returns true when the job was successfully written back to the queue.
func (w *SyncWorker) requeueJob(job *SyncJob) bool {
	backoffDelay := time.Duration(job.RetryCount) * 5 * time.Second
	if backoffDelay > 60*time.Second {
		backoffDelay = 60 * time.Second
	}
	job.AvailableAt = time.Now().UTC().Add(backoffDelay)

	jobBytes, marshalErr := json.Marshal(job)
	if marshalErr != nil {
		w.logger.Error("failed to marshal job for requeue",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("key", job.Key),
			zap.Error(marshalErr))
		w.markMobilidadeInviteQueuePersistFailure(job, fmt.Sprintf("requeue marshal failed: %v", marshalErr))
		return false
	}
	queueKey := syncQueueKey(job.Type)
	ctx := context.Background()

	if job.fromInflight && usesReliableQueue(job.Type) && job.rawRedisPayload != "" {
		moved, moveErr := w.completeReliableJobToList(ctx, job.Type, queueKey, job.rawRedisPayload, string(jobBytes))
		if moveErr != nil {
			w.logger.Error("failed to requeue job",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.String("key", job.Key),
				zap.Int("retry_count", job.RetryCount),
				zap.Error(moveErr))
			if w.metrics != nil {
				w.metrics.IncrementSyncFailures(job.Type)
			}
			w.markMobilidadeInviteQueuePersistFailure(job, fmt.Sprintf("requeue failed: %v", moveErr))
			return false
		}
		if !moved {
			w.logger.Error("failed to requeue job: inflight payload missing",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.String("key", job.Key))
			w.markMobilidadeInviteQueuePersistFailure(job, "requeue failed: inflight payload missing")
			return false
		}
	} else if pushErr := w.redis.LPush(ctx, queueKey, string(jobBytes)).Err(); pushErr != nil {
		w.logger.Error("failed to requeue job",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("key", job.Key),
			zap.Int("retry_count", job.RetryCount),
			zap.Error(pushErr))
		if w.metrics != nil {
			w.metrics.IncrementSyncFailures(job.Type)
		}
		w.markMobilidadeInviteQueuePersistFailure(job, fmt.Sprintf("requeue failed: %v", pushErr))
		return false
	}

	w.logger.Info("job re-queued for retry",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("key", job.Key),
		zap.Int("retry_count", job.RetryCount),
		zap.Time("available_at", job.AvailableAt),
		zap.Duration("backoff_delay", backoffDelay))
	return true
}

// returnJobToQueue puts a not-yet-available job back on the main queue and acks inflight.
func (w *SyncWorker) returnJobToQueue(job *SyncJob) error {
	jobBytes, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal deferred job: %w", err)
	}
	queueKey := syncQueueKey(job.Type)
	ctx := context.Background()

	if job.fromInflight && usesReliableQueue(job.Type) && job.rawRedisPayload != "" {
		moved, moveErr := w.completeReliableJobToList(ctx, job.Type, queueKey, job.rawRedisPayload, string(jobBytes))
		if moveErr != nil {
			return fmt.Errorf("return deferred job: %w", moveErr)
		}
		if !moved {
			return fmt.Errorf("return deferred job: inflight payload missing")
		}
		return nil
	}

	if err := w.redis.LPush(ctx, queueKey, string(jobBytes)).Err(); err != nil {
		return fmt.Errorf("lpush deferred job: %w", err)
	}
	w.ackInflightJob(job)
	return nil
}

// markMobilidadeInviteQueuePersistFailure records email_status=failed when requeue/DLQ cannot persist the job.
func (w *SyncWorker) markMobilidadeInviteQueuePersistFailure(job *SyncJob, msg string) {
	if job == nil {
		return
	}
	if job.Type != MobilidadeInviteEmailQueue && job.Collection != MobilidadeInviteEmailQueue {
		return
	}
	conductorID := job.Key
	if conductorID == "" {
		if payload, err := parseMobilidadeInviteEmailPayload(job.Data); err == nil {
			conductorID = payload.ConductorID
		}
	}
	if conductorID == "" {
		return
	}
	_ = w.persistInviteEmailStatus(context.Background(), conductorID, models.InviteEmailStatusFailed, msg)
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

	// Mobilidade conductor invite email
	if job.Type == MobilidadeInviteEmailQueue || job.Collection == MobilidadeInviteEmailQueue {
		return w.handleMobilidadeInviteEmailJob(ctx, job)
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
		return fmt.Errorf("CF lookup service disabled")
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

func (w *SyncWorker) handleMobilidadeInviteEmailJob(ctx context.Context, job *SyncJob) error {
	w.logger.Info("processing mobilidade invite email job",
		zap.String("job_id", job.ID),
		zap.String("key", job.Key))

	payload, err := parseMobilidadeInviteEmailPayload(job.Data)
	if err != nil {
		conductorID := job.Key
		if conductorID == "" {
			conductorID = payload.ConductorID
		}
		if conductorID != "" {
			_ = w.persistInviteEmailStatus(ctx, conductorID, models.InviteEmailStatusFailed, err.Error())
		}
		return err
	}

	outcome := payload.DeliveryOutcome
	if outcome == "" {
		sender := w.emailSender
		if sender == nil {
			sender = NewLoggingEmailSender(w.logger)
		}

		if err := ProcessMobilidadeInviteEmail(ctx, sender, payload, DefaultMobilidadeInviteDeepLinkBase()); err != nil {
			if errors.Is(err, ErrEmailDeliverySkipped) {
				outcome = string(models.InviteEmailStatusSkipped)
				w.logger.Info("mobilidade invite email skipped (provider not configured)",
					zap.String("job_id", job.ID),
					zap.String("conductor_id", payload.ConductorID),
					zap.String("notify_email", utils.MaskEmail(payload.NotifyEmail)))
			} else {
				w.logger.Error("mobilidade invite email failed",
					zap.Error(err),
					zap.String("job_id", job.ID),
					zap.String("conductor_id", payload.ConductorID),
					zap.String("notify_email", utils.MaskEmail(payload.NotifyEmail)))
				_ = w.persistInviteEmailStatus(ctx, payload.ConductorID, models.InviteEmailStatusFailed, err.Error())
				return err
			}
		} else {
			outcome = string(models.InviteEmailStatusSent)
			w.logger.Info("mobilidade invite email sent",
				zap.String("job_id", job.ID),
				zap.String("conductor_id", payload.ConductorID),
				zap.String("notify_email", utils.MaskEmail(payload.NotifyEmail)))
		}

		payload.DeliveryOutcome = outcome
		job.Data = payload
		if err := w.checkpointInviteDeliveryOutcome(ctx, job, payload); err != nil {
			// Still try to persist status; if that also fails the requeue carries DeliveryOutcome.
			w.logger.Warn("failed to checkpoint invite delivery outcome on inflight payload",
				zap.String("job_id", job.ID),
				zap.String("conductor_id", payload.ConductorID),
				zap.Error(err))
		}
	}

	status := models.InviteEmailStatus(outcome)
	lastError := ""
	if status == models.InviteEmailStatusSkipped {
		lastError = ErrEmailDeliverySkipped.Error()
	}
	if err := w.persistInviteEmailStatus(ctx, payload.ConductorID, status, lastError); err != nil {
		// Keep job retryable without re-sending: DeliveryOutcome is already on job.Data
		// (and preferably checkpointed on the inflight Redis payload).
		return fmt.Errorf("persist invite email status (%s): %w", status, err)
	}
	return nil
}

// checkpointInviteDeliveryOutcome rewrites the inflight Redis payload so a crash/recovery
// after provider success does not re-send the email.
func (w *SyncWorker) checkpointInviteDeliveryOutcome(ctx context.Context, job *SyncJob, payload MobilidadeInviteEmailPayload) error {
	if job == nil || !job.fromInflight || job.rawRedisPayload == "" || !usesReliableQueue(job.Type) {
		return nil
	}
	job.Data = payload
	jobBytes, err := json.Marshal(job)
	if err != nil {
		return err
	}
	newPayload := string(jobBytes)
	if err := w.replaceReliableInflightPayload(ctx, job.Type, job.rawRedisPayload, newPayload); err != nil {
		return err
	}
	job.rawRedisPayload = newPayload
	return nil
}

func (w *SyncWorker) persistInviteEmailStatus(ctx context.Context, conductorID string, status models.InviteEmailStatus, lastError string) error {
	if err := SetConductorInviteEmailStatus(ctx, w.mongo, conductorID, status, lastError); err != nil {
		w.logger.Warn("mobilidade invite email status update failed",
			zap.String("conductor_id", conductorID),
			zap.String("email_status", string(status)),
			zap.Error(err))
		return err
	}
	return nil
}

func parseMobilidadeInviteEmailPayload(data interface{}) (MobilidadeInviteEmailPayload, error) {
	var payload MobilidadeInviteEmailPayload
	raw, err := json.Marshal(data)
	if err != nil {
		return payload, fmt.Errorf("marshal invite email payload: %w", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, fmt.Errorf("unmarshal invite email payload: %w", err)
	}
	return payload, nil
}

// getFieldNameFromJobType maps job types to their corresponding database field names
// This ensures that self_declared updates only modify specific fields instead of overwriting the entire document
func getFieldNameFromJobType(jobType string) string {
	switch jobType {
	case "self_declared_address":
		return "endereco"
	case "self_declared_email":
		return "email"
	case "self_declared_phone":
		return "telefone"
	case "self_declared_raca":
		return "raca"
	case "self_declared_nome_exibicao":
		return "nome_exibicao"
	case "self_declared_genero":
		return "genero"
	case "self_declared_renda_familiar":
		return "renda_familiar"
	case "self_declared_escolaridade":
		return "escolaridade"
	case "self_declared_deficiencia":
		return "deficiencia"
	default:
		return ""
	}
}
