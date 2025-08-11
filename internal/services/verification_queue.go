package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// VerificationJob represents a phone verification job
type VerificationJob struct {
	PhoneNumber string                 `json:"phone_number"`
	Code        string                 `json:"code"`
	CPF         string                 `json:"cpf"`
	UserID      string                 `json:"user_id"`
	IPAddress   string                 `json:"ip_address"`
	UserAgent   string                 `json:"user_agent"`
	RequestID   string                 `json:"request_id"`
	CreatedAt   time.Time              `json:"created_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// VerificationResult represents the result of a verification job
type VerificationResult struct {
	JobID       string    `json:"job_id"`
	PhoneNumber string    `json:"phone_number"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	ProcessedAt time.Time `json:"processed_at"`
}

// VerificationQueue manages asynchronous phone verification processing
type VerificationQueue struct {
	queue           chan VerificationJob
	results         chan VerificationResult
	workers         int
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	processingStats *ProcessingStats
	mu              sync.RWMutex
}

// ProcessingStats tracks queue performance metrics
type ProcessingStats struct {
	JobsEnqueued    int64         `json:"jobs_enqueued"`
	JobsProcessed   int64         `json:"jobs_processed"`
	JobsFailed      int64         `json:"jobs_failed"`
	AverageWaitTime time.Duration `json:"average_wait_time"`
	QueueSize       int           `json:"queue_size"`
	ActiveWorkers   int           `json:"active_workers"`
}

// NewVerificationQueue creates a new verification queue
func NewVerificationQueue(workers int, queueSize int) *VerificationQueue {
	ctx, cancel := context.WithCancel(context.Background())

	queue := &VerificationQueue{
		queue:           make(chan VerificationJob, queueSize),
		results:         make(chan VerificationResult, queueSize),
		workers:         workers,
		ctx:             ctx,
		cancel:          cancel,
		processingStats: &ProcessingStats{},
	}

	// Start workers
	queue.startWorkers()

	// Start result processor
	go queue.processResults()

	return queue
}

// startWorkers starts the worker goroutines
func (vq *VerificationQueue) startWorkers() {
	for i := 0; i < vq.workers; i++ {
		vq.wg.Add(1)
		go vq.worker(i)
	}
}

// worker processes verification jobs from the queue
func (vq *VerificationQueue) worker(id int) {
	defer vq.wg.Done()

	for {
		select {
		case job, ok := <-vq.queue:
			if !ok {
				return
			}
			vq.processJob(job, id)
		case <-vq.ctx.Done():
			return
		}
	}
}

// processJob processes a single verification job
func (vq *VerificationQueue) processJob(job VerificationJob, workerID int) {
	startTime := time.Now()

	// Update stats
	vq.mu.Lock()
	vq.processingStats.JobsProcessed++
	vq.mu.Unlock()

	// Process the verification
	result := VerificationResult{
		JobID:       fmt.Sprintf("%d-%s", workerID, job.PhoneNumber),
		PhoneNumber: job.PhoneNumber,
		ProcessedAt: time.Now(),
	}

	// Validate the verification code
	err := vq.validateVerificationCode(job)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		logging.Logger.Error("verification failed",
			zap.Int("worker_id", workerID),
			zap.String("phone_number", job.PhoneNumber),
			zap.Error(err))
	} else {
		result.Success = true
		logging.Logger.Info("verification successful",
			zap.Int("worker_id", workerID),
			zap.String("phone_number", job.PhoneNumber))
	}

	// Send result
	select {
	case vq.results <- result:
		// Result sent successfully
	default:
		// Results channel is full, log warning
		logging.Logger.Warn("results channel full, dropping result")
	}

	// Update processing time stats
	processingTime := time.Since(startTime)
	vq.mu.Lock()
	if vq.processingStats.AverageWaitTime == 0 {
		vq.processingStats.AverageWaitTime = processingTime
	} else {
		// Simple moving average
		vq.processingStats.AverageWaitTime = (vq.processingStats.AverageWaitTime + processingTime) / 2
	}
	vq.mu.Unlock()
}

// validateVerificationCode validates the verification code against the database
func (vq *VerificationQueue) validateVerificationCode(job VerificationJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find the verification code in the database
	collection := config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection)

	var verification struct {
		Code      string    `bson:"code"`
		ExpiresAt time.Time `bson:"expires_at"`
		Used      bool      `bson:"used"`
	}

	err := collection.FindOne(ctx, bson.M{
		"phone_number": job.PhoneNumber,
		"code":         job.Code,
		"used":         false,
		"expires_at":   bson.M{"$gt": time.Now()},
	}).Decode(&verification)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("invalid or expired verification code")
		}
		return fmt.Errorf("database error: %w", err)
	}

	// Mark code as used
	_, err = collection.UpdateOne(ctx,
		bson.M{"phone_number": job.PhoneNumber, "code": job.Code},
		bson.M{"$set": bson.M{"used": true, "used_at": time.Now()}},
	)
	if err != nil {
		return fmt.Errorf("failed to mark code as used: %w", err)
	}

	// Update phone mapping to verified
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	_, err = phoneCollection.UpdateOne(ctx,
		bson.M{"phone_number": job.PhoneNumber},
		bson.M{"$set": bson.M{
			"verified":    true,
			"verified_at": time.Now(),
			"updated_at":  time.Now(),
		}},
	)
	if err != nil {
		return fmt.Errorf("failed to update phone verification status: %w", err)
	}

	// Log audit event
	auditCtx := utils.GetAuditContextFromRequest(job.CPF, job.UserID, job.RequestID, job.IPAddress, job.UserAgent)
	if err := utils.LogPhoneVerificationSuccess(ctx, auditCtx, job.PhoneNumber); err != nil {
		logging.Logger.Warn("failed to log phone verification success", zap.Error(err))
	}

	// Invalidate related caches
	cacheKey := fmt.Sprintf("phone:%s:status", job.PhoneNumber)
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		logging.Logger.Warn("failed to invalidate phone status cache", zap.Error(err))
	}

	return nil
}

// processResults processes verification results
func (vq *VerificationQueue) processResults() {
	for {
		select {
		case result, ok := <-vq.results:
			if !ok {
				return
			}
			vq.handleResult(result)
		case <-vq.ctx.Done():
			return
		}
	}
}

// handleResult handles a verification result
func (vq *VerificationQueue) handleResult(result VerificationResult) {
	vq.mu.Lock()
	if !result.Success {
		vq.processingStats.JobsFailed++
	}
	vq.mu.Unlock()

	// Log result
	logger := logging.Logger.With(
		zap.String("phone_number", result.PhoneNumber),
		zap.Bool("success", result.Success),
	)

	if result.Success {
		logger.Info("verification job completed successfully")
	} else {
		logger.Error("verification job failed", zap.String("error", result.Error))
	}
}

// Enqueue adds a verification job to the queue
func (vq *VerificationQueue) Enqueue(job VerificationJob) error {
	// Update stats
	vq.mu.Lock()
	vq.processingStats.JobsEnqueued++
	vq.processingStats.QueueSize = len(vq.queue)
	vq.mu.Unlock()

	select {
	case vq.queue <- job:
		return nil
	default:
		return fmt.Errorf("verification queue is full")
	}
}

// GetStats returns the current processing statistics
func (vq *VerificationQueue) GetStats() ProcessingStats {
	vq.mu.RLock()
	defer vq.mu.RUnlock()

	stats := *vq.processingStats
	stats.QueueSize = len(vq.queue)
	stats.ActiveWorkers = vq.workers

	return stats
}

// Stop gracefully stops the verification queue
func (vq *VerificationQueue) Stop() {
	vq.cancel()
	close(vq.queue)
	close(vq.results)
	vq.wg.Wait()
}

// IsHealthy checks if the queue is healthy
func (vq *VerificationQueue) IsHealthy() bool {
	stats := vq.GetStats()

	// Check if queue is not overflowing
	if stats.QueueSize > 1000 {
		return false
	}

	// Check if workers are processing jobs
	if stats.JobsProcessed == 0 && stats.JobsEnqueued > 0 {
		return false
	}

	return true
}
