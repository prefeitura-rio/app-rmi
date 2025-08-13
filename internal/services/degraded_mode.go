package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.uber.org/zap"
)

// DegradedMode manages degraded mode when MongoDB is down or Redis memory is high
type DegradedMode struct {
	redis       *redisclient.Client
	mongo       *mongo.Database
	metrics     *Metrics
	isActive    bool
	reason      string
	activatedAt time.Time
	mu          sync.RWMutex
	stopChan    chan struct{}
	logger      *logging.SafeLogger
}

// NewDegradedMode creates a new degraded mode manager
func NewDegradedMode(redis *redisclient.Client, mongo *mongo.Database, metrics *Metrics) *DegradedMode {
	return &DegradedMode{
		redis:    redis,
		mongo:    mongo,
		metrics:  metrics,
		stopChan: make(chan struct{}),
		logger:   logging.Logger,
	}
}

// StartMonitoring starts the degraded mode monitoring
func (dm *DegradedMode) StartMonitoring() {
	dm.logger.Info("starting degraded mode monitoring")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			dm.CheckConditions()
		case <-dm.stopChan:
			dm.logger.Info("degraded mode monitoring stopped")
			return
		}
	}
}

// Stop stops the degraded mode monitoring
func (dm *DegradedMode) Stop() {
	close(dm.stopChan)
}

// CheckConditions checks if degraded mode should be activated
func (dm *DegradedMode) CheckConditions() {
	// Check MongoDB health
	if dm.isMongoDBDown() {
		dm.Activate("mongodb_down")
		return
	}

	// Check Redis memory usage
	if dm.isRedisMemoryHigh() {
		dm.Activate("redis_memory_high")
		return
	}

	// If no conditions are met, deactivate degraded mode
	dm.Deactivate()
}

// Activate activates degraded mode
func (dm *DegradedMode) Activate(reason string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if !dm.isActive {
		dm.isActive = true
		dm.reason = reason
		dm.activatedAt = time.Now()

		dm.logger.Warn("degraded mode activated",
			zap.String("reason", reason),
			zap.Time("activated_at", dm.activatedAt))

		// Update metrics
		dm.metrics.SetDegradedMode(true)
	}
}

// Deactivate deactivates degraded mode
func (dm *DegradedMode) Deactivate() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if dm.isActive {
		dm.isActive = false
		duration := time.Since(dm.activatedAt)

		dm.logger.Info("degraded mode deactivated",
			zap.String("previous_reason", dm.reason),
			zap.Duration("duration", duration))

		// Update metrics
		dm.metrics.SetDegradedMode(false)

		dm.reason = ""
		dm.activatedAt = time.Time{}
	}
}

// IsActive returns whether degraded mode is active
func (dm *DegradedMode) IsActive() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.isActive
}

// GetReason returns the reason for degraded mode
func (dm *DegradedMode) GetReason() string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.reason
}

// GetDuration returns how long degraded mode has been active
func (dm *DegradedMode) GetDuration() time.Duration {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	if !dm.isActive {
		return 0
	}

	return time.Since(dm.activatedAt)
}

// isMongoDBDown checks if MongoDB is down
func (dm *DegradedMode) isMongoDBDown() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := dm.mongo.Client().Ping(ctx, readpref.Primary())
	return err != nil
}

// isRedisMemoryHigh checks if Redis memory usage is above 85%
func (dm *DegradedMode) isRedisMemoryHigh() bool {
	info, err := dm.redis.Info(context.Background(), "memory").Result()
	if err != nil {
		return false // Can't determine, assume OK
	}

	// Parse Redis memory info
	lines := strings.Split(info, "\n")
	var usedMemory, maxMemory int64

	for _, line := range lines {
		if strings.HasPrefix(line, "used_memory:") {
			if _, err := fmt.Sscanf(line, "used_memory:%d", &usedMemory); err != nil {
				continue // Skip malformed lines
			}
		}
		if strings.HasPrefix(line, "maxmemory:") {
			if _, err := fmt.Sscanf(line, "maxmemory:%d", &maxMemory); err != nil {
				continue // Skip malformed lines
			}
		}
	}

	if maxMemory == 0 {
		return false // No max memory set
	}

	usagePercentage := float64(usedMemory) / float64(maxMemory) * 100
	return usagePercentage >= 85
}
