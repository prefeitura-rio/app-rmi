package utils

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.uber.org/zap"
)

// PerformanceMonitor monitors performance metrics for operations
type PerformanceMonitor struct {
	startTime    time.Time
	operation    string
	logger       *logging.SafeLogger
	checkpoints  []Checkpoint
	memoryBefore runtime.MemStats
}

// Checkpoint represents a performance checkpoint
type Checkpoint struct {
	Name     string
	Duration time.Duration
	Memory   uint64
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(ctx context.Context, operation string) *PerformanceMonitor {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &PerformanceMonitor{
		startTime:    time.Now(),
		operation:    operation,
		logger:       logging.Logger,
		checkpoints:  make([]Checkpoint, 0),
		memoryBefore: memStats,
	}
}

// Checkpoint adds a performance checkpoint
func (pm *PerformanceMonitor) Checkpoint(name string) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	checkpoint := Checkpoint{
		Name:     name,
		Duration: time.Since(pm.startTime),
		Memory:   memStats.Alloc - pm.memoryBefore.Alloc,
	}

	pm.checkpoints = append(pm.checkpoints, checkpoint)

	pm.logger.Info("performance checkpoint",
		zap.String("checkpoint", name),
		zap.Duration("duration", checkpoint.Duration),
		zap.Int64("memory_delta_bytes", int64(checkpoint.Memory)),
	)
}

// End completes the performance monitoring and logs results
func (pm *PerformanceMonitor) End() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	totalDuration := time.Since(pm.startTime)
	totalMemoryDelta := memStats.Alloc - pm.memoryBefore.Alloc

	// Log performance summary
	pm.logger.Info("performance monitoring completed",
		zap.String("operation", pm.operation),
		zap.Duration("total_duration", totalDuration),
		zap.Int64("total_memory_delta_bytes", int64(totalMemoryDelta)),
		zap.Int("checkpoint_count", len(pm.checkpoints)),
	)

	// Log individual checkpoints if there are any
	if len(pm.checkpoints) > 0 {
		for _, cp := range pm.checkpoints {
			pm.logger.Info("checkpoint details",
				zap.String("operation", pm.operation),
				zap.String("checkpoint", cp.Name),
				zap.Duration("duration", cp.Duration),
				zap.Int64("memory_delta_bytes", int64(cp.Memory)),
			)
		}
	}

	// Update metrics
	observability.OperationDuration.WithLabelValues(pm.operation).Observe(totalDuration.Seconds())
	observability.OperationMemoryUsage.WithLabelValues(pm.operation).Observe(float64(totalMemoryDelta))
}

// PerformanceWarning logs a performance warning if duration exceeds threshold
func (pm *PerformanceMonitor) PerformanceWarning(threshold time.Duration, message string) {
	elapsed := time.Since(pm.startTime)
	if elapsed > threshold {
		pm.logger.Warn("performance warning",
			zap.String("operation", pm.operation),
			zap.Duration("elapsed", elapsed),
			zap.Duration("threshold", threshold),
			zap.String("message", message),
		)
	}
}

// MemoryWarning logs a memory warning if usage exceeds threshold
func (pm *PerformanceMonitor) MemoryWarning(thresholdBytes int64, message string) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memoryDelta := int64(memStats.Alloc - pm.memoryBefore.Alloc)
	if memoryDelta > thresholdBytes {
		pm.logger.Warn("memory usage warning",
			zap.String("operation", pm.operation),
			zap.Int64("memory_delta_bytes", memoryDelta),
			zap.Int64("threshold_bytes", thresholdBytes),
			zap.String("message", message),
		)
	}
}

// GetPerformanceReport returns a formatted performance report
func (pm *PerformanceMonitor) GetPerformanceReport() string {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	totalDuration := time.Since(pm.startTime)
	totalMemoryDelta := memStats.Alloc - pm.memoryBefore.Alloc

	report := fmt.Sprintf("Performance Report for %s:\n", pm.operation)
	report += fmt.Sprintf("  Total Duration: %s\n", totalDuration)
	report += fmt.Sprintf("  Total Memory Delta: %d bytes\n", totalMemoryDelta)
	report += fmt.Sprintf("  Checkpoints: %d\n", len(pm.checkpoints))

	for _, cp := range pm.checkpoints {
		report += fmt.Sprintf("    %s: %s (%d bytes)\n", cp.Name, cp.Duration, cp.Memory)
	}

	return report
}

// MonitorFunction monitors the performance of a function
func MonitorFunction(ctx context.Context, operation string, fn func() error) error {
	monitor := NewPerformanceMonitor(ctx, operation)
	defer monitor.End()

	monitor.Checkpoint("start")

	err := fn()

	monitor.Checkpoint("end")

	if err != nil {
		monitor.logger.Error("operation failed",
			zap.String("operation", operation),
			zap.Error(err),
		)
	}

	return err
}

// MonitorFunctionWithResult monitors the performance of a function that returns a result
func MonitorFunctionWithResult[T any](ctx context.Context, operation string, fn func() (T, error)) (T, error) {
	monitor := NewPerformanceMonitor(ctx, operation)
	defer monitor.End()

	monitor.Checkpoint("start")

	result, err := fn()

	monitor.Checkpoint("end")

	if err != nil {
		monitor.logger.Error("operation failed",
			zap.String("operation", operation),
			zap.Error(err),
		)
	}

	return result, err
}
