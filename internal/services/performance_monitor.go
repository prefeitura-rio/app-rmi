package services

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.uber.org/zap"
)

// PerformanceMetric tracks performance data for a specific operation
type PerformanceMetric struct {
	OperationName string        `json:"operation_name"`
	Count         int64         `json:"count"`
	TotalDuration time.Duration `json:"total_duration"`
	MinDuration   time.Duration `json:"min_duration"`
	MaxDuration   time.Duration `json:"max_duration"`
	AvgDuration   time.Duration `json:"avg_duration"`
	LastUpdated   time.Time     `json:"last_updated"`
	mu            sync.RWMutex
}

// PerformanceMonitor provides comprehensive performance monitoring
type PerformanceMonitor struct {
	metrics map[string]*PerformanceMetric
	mu      sync.RWMutex
	logger  *logging.SafeLogger
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor() *PerformanceMonitor {
	return &PerformanceMonitor{
		metrics: make(map[string]*PerformanceMetric),
		logger:  logging.Logger,
	}
}

// RecordOperation records performance data for an operation
func (pm *PerformanceMonitor) RecordOperation(name string, duration time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	metric, exists := pm.metrics[name]
	if !exists {
		metric = &PerformanceMetric{
			OperationName: name,
			MinDuration:   duration,
			MaxDuration:   duration,
		}
		pm.metrics[name] = metric
	}

	metric.mu.Lock()
	defer metric.mu.Unlock()

	metric.Count++
	metric.TotalDuration += duration
	metric.LastUpdated = time.Now()

	// Update min/max
	if duration < metric.MinDuration {
		metric.MinDuration = duration
	}
	if duration > metric.MaxDuration {
		metric.MaxDuration = duration
	}

	// Calculate average
	metric.AvgDuration = metric.TotalDuration / time.Duration(metric.Count)

	// Record to Prometheus metrics
	observability.OperationDuration.WithLabelValues(name).Observe(duration.Seconds())
}

// GetMetrics returns all performance metrics
func (pm *PerformanceMonitor) GetMetrics() map[string]*PerformanceMetric {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Create a copy to avoid race conditions
	metrics := make(map[string]*PerformanceMetric)
	for name, metric := range pm.metrics {
		metric.mu.RLock()
		metrics[name] = &PerformanceMetric{
			OperationName: metric.OperationName,
			Count:         metric.Count,
			TotalDuration: metric.TotalDuration,
			MinDuration:   metric.MinDuration,
			MaxDuration:   metric.MaxDuration,
			AvgDuration:   metric.AvgDuration,
			LastUpdated:   metric.LastUpdated,
		}
		metric.mu.RUnlock()
	}

	return metrics
}

// GetMetric returns a specific performance metric
func (pm *PerformanceMonitor) GetMetric(name string) *PerformanceMetric {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	metric, exists := pm.metrics[name]
	if !exists {
		return nil
	}

	metric.mu.RLock()
	defer metric.mu.RUnlock()

	return &PerformanceMetric{
		OperationName: metric.OperationName,
		Count:         metric.Count,
		TotalDuration: metric.TotalDuration,
		MinDuration:   metric.MinDuration,
		MaxDuration:   metric.MaxDuration,
		AvgDuration:   metric.AvgDuration,
		LastUpdated:   metric.LastUpdated,
	}
}

// ResetMetrics resets all performance metrics
func (pm *PerformanceMonitor) ResetMetrics() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, metric := range pm.metrics {
		metric.mu.Lock()
		metric.Count = 0
		metric.TotalDuration = 0
		metric.MinDuration = 0
		metric.MaxDuration = 0
		metric.AvgDuration = 0
		metric.LastUpdated = time.Time{}
		metric.mu.Unlock()
	}
}

// GetSystemStats returns current system statistics
func (pm *PerformanceMonitor) GetSystemStats() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"goroutines":   runtime.NumGoroutine(),
		"memory_alloc": m.Alloc,
		"memory_total": m.TotalAlloc,
		"memory_sys":   m.Sys,
		"memory_heap":  m.HeapAlloc,
		"memory_stack": m.StackInuse,
		"gc_cycles":    m.NumGC,
		"gc_pause_ns":  m.PauseTotalNs,
		"uptime":       time.Since(startTime),
	}
}

// MonitorFunction wraps a function with performance monitoring
func (pm *PerformanceMonitor) MonitorFunction(name string, fn func() error) error {
	start := time.Now()
	err := fn()
	duration := time.Since(start)

	pm.RecordOperation(name, duration)

	if err != nil {
		pm.logger.Error("operation failed",
			zap.String("operation", name),
			zap.Duration("duration", duration),
			zap.Error(err))
	} else {
		pm.logger.Info("operation completed",
			zap.String("operation", name),
			zap.Duration("duration", duration))
	}

	return err
}

// MonitorFunctionWithResult wraps a function with performance monitoring and result
func MonitorFunctionWithResult[T any](pm *PerformanceMonitor, name string, fn func() (T, error)) (T, error) {
	start := time.Now()
	result, err := fn()
	duration := time.Since(start)

	pm.RecordOperation(name, duration)

	if err != nil {
		pm.logger.Error("operation failed",
			zap.String("operation", name),
			zap.Duration("duration", duration),
			zap.Error(err))
	} else {
		pm.logger.Info("operation completed",
			zap.String("operation", name),
			zap.Duration("duration", duration))
	}

	return result, err
}

// MonitorContextFunction wraps a context function with performance monitoring
func (pm *PerformanceMonitor) MonitorContextFunction(name string, fn func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		start := time.Now()
		err := fn(ctx)
		duration := time.Since(start)

		pm.RecordOperation(name, duration)

		if err != nil {
			pm.logger.Error("operation failed",
				zap.String("operation", name),
				zap.Duration("duration", duration),
				zap.Error(err))
		} else {
			pm.logger.Info("operation completed",
				zap.String("operation", name),
				zap.Duration("duration", duration))
		}

		return err
	}
}

// MonitorContextFunctionWithResult wraps a context function with performance monitoring and result
func MonitorContextFunctionWithResult[T any](pm *PerformanceMonitor, name string, fn func(context.Context) (T, error)) func(context.Context) (T, error) {
	return func(ctx context.Context) (T, error) {
		start := time.Now()
		result, err := fn(ctx)
		duration := time.Since(start)

		pm.RecordOperation(name, duration)

		if err != nil {
			pm.logger.Error("operation failed",
				zap.String("operation", name),
				zap.Duration("duration", duration),
				zap.Error(err))
		} else {
			pm.logger.Info("operation completed",
				zap.String("operation", name),
				zap.Duration("duration", duration))
		}

		return result, err
	}
}

// GetPerformanceReport returns a comprehensive performance report
func (pm *PerformanceMonitor) GetPerformanceReport() map[string]interface{} {
	metrics := pm.GetMetrics()
	systemStats := pm.GetSystemStats()

	// Calculate summary statistics
	var totalOperations int64
	var totalDuration time.Duration
	var slowestOperation string
	var slowestDuration time.Duration

	for name, metric := range metrics {
		totalOperations += metric.Count
		totalDuration += metric.TotalDuration

		if metric.MaxDuration > slowestDuration {
			slowestDuration = metric.MaxDuration
			slowestOperation = name
		}
	}

	report := map[string]interface{}{
		"timestamp":          time.Now(),
		"total_operations":   totalOperations,
		"total_duration":     totalDuration,
		"slowest_operation":  slowestOperation,
		"slowest_duration":   slowestDuration,
		"operation_metrics":  metrics,
		"system_stats":       systemStats,
		"performance_alerts": pm.getPerformanceAlerts(metrics),
	}

	return report
}

// getPerformanceAlerts identifies performance issues
func (pm *PerformanceMonitor) getPerformanceAlerts(metrics map[string]*PerformanceMetric) []map[string]interface{} {
	var alerts []map[string]interface{}

	for name, metric := range metrics {
		// Alert for slow operations (> 1 second average)
		if metric.AvgDuration > time.Second {
			alerts = append(alerts, map[string]interface{}{
				"type":         "slow_operation",
				"operation":    name,
				"avg_duration": metric.AvgDuration,
				"severity":     "warning",
			})
		}

		// Alert for very slow operations (> 5 seconds average)
		if metric.AvgDuration > 5*time.Second {
			alerts = append(alerts, map[string]interface{}{
				"type":         "very_slow_operation",
				"operation":    name,
				"avg_duration": metric.AvgDuration,
				"severity":     "critical",
			})
		}

		// Alert for operations with high variance
		if metric.MaxDuration > 0 && metric.MinDuration > 0 {
			variance := metric.MaxDuration - metric.MinDuration
			if variance > metric.AvgDuration*2 {
				alerts = append(alerts, map[string]interface{}{
					"type":         "high_variance_operation",
					"operation":    name,
					"variance":     variance,
					"avg_duration": metric.AvgDuration,
					"severity":     "info",
				})
			}
		}
	}

	return alerts
}

// Global performance monitor instance
var (
	globalMonitor *PerformanceMonitor
	startTime     = time.Now()
	once          sync.Once
)

// GetGlobalMonitor returns the global performance monitor
func GetGlobalMonitor() *PerformanceMonitor {
	once.Do(func() {
		globalMonitor = NewPerformanceMonitor()
	})
	return globalMonitor
}
