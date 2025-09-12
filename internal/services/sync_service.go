package services

import (
	"context"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
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
		// Check all DLQ sizes
		queues := []string{"citizen", "phone_mapping", "user_config", "opt_in_history", "beta_group", "phone_verification", "maintenance_request", "self_declared_address", "self_declared_email", "self_declared_phone", "self_declared_raca", "self_declared_nome_exibicao", "cf_lookup"}

		for _, queue := range queues {
			dlqKey := fmt.Sprintf("sync:dlq:%s", queue)
			dlqSize, err := s.redis.LLen(context.Background(), dlqKey).Result()
			if err != nil {
				continue
			}

			if dlqSize > 0 {
				s.logger.Warn("DLQ has failed jobs",
					zap.String("queue", queue),
					zap.Int64("dlq_size", dlqSize))

				// Update metrics - record DLQ size
				s.metrics.RecordQueueDepth("dlq_"+queue, dlqSize)
			}
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
