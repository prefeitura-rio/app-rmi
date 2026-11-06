package services

import (
	"context"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// SyncService manages background sync operations from Redis to MongoDB
type SyncService struct {
	redis        *redisclient.Client
	mongo        *mongo.Database
	workers      []*SyncWorker
	workerCount  int
	logger       *logging.SafeLogger
	metrics      *Metrics
	degradedMode *DegradedMode
}

// NewSyncService creates a new sync service
func NewSyncService(redis *redisclient.Client, mongo *mongo.Database, workerCount int, logger *logging.SafeLogger) *SyncService {
	metrics := NewMetrics()
	degradedMode := NewDegradedMode(redis, mongo, metrics)

	return &SyncService{
		redis:        redis,
		mongo:        mongo,
		workerCount:  workerCount,
		logger:       logger,
		metrics:      metrics,
		degradedMode: degradedMode,
	}
}

// Start starts the sync service
func (s *SyncService) Start() {
	s.logger.Info("starting sync service", zap.Int("worker_count", s.workerCount))

	// Start degraded mode monitoring
	go s.degradedMode.StartMonitoring()

	// Start workers
	for i := 0; i < s.workerCount; i++ {
		worker := NewSyncWorker(s.redis, s.mongo, i, s.logger, s.metrics, s.degradedMode)
		s.workers = append(s.workers, worker)
		go worker.Start()
	}

	// Start DLQ monitoring
	go s.monitorDLQ()

	s.logger.Info("sync service started successfully")
}

// Stop stops the sync service
func (s *SyncService) Stop() {
	s.logger.Info("stopping sync service")

	// Stop degraded mode monitoring
	s.degradedMode.Stop()

	// Stop all workers
	for _, worker := range s.workers {
		worker.Stop()
	}

	s.logger.Info("sync service stopped")
}

// monitorDLQ monitors the dead letter queue
func (s *SyncService) monitorDLQ() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.scanDLQs()
	}
}

// monitoredDLQQueues is the set of sync queues whose DLQs are scanned for alerts/metrics:
// every queue the worker consumes.
func monitoredDLQQueues() []string {
	return syncQueueNames()
}

// scanDLQs records the depth of every dead-letter queue and warns for the non-empty ones.
func (s *SyncService) scanDLQs() {
	for _, queue := range monitoredDLQQueues() {
		dlqKey := syncDLQKey(queue)
		dlqSize, err := s.redis.LLen(context.Background(), dlqKey).Result()
		if err != nil {
			continue
		}

		// Record the depth even when it is zero, so the gauge drops back after a DLQ is drained.
		s.metrics.RecordQueueDepth("dlq_"+queue, dlqSize)

		if dlqSize > 0 {
			s.logger.Warn("DLQ has failed jobs",
				zap.String("queue", queue),
				zap.String("dlq_key", observability.MaskCacheKey(dlqKey)),
				zap.Int64("dlq_size", dlqSize))
		}
	}
}

// GetMetrics returns the metrics for monitoring
func (s *SyncService) GetMetrics() *Metrics {
	return s.metrics
}

// IsDegradedMode returns whether degraded mode is active
func (s *SyncService) IsDegradedMode() bool {
	return s.degradedMode.IsActive()
}
