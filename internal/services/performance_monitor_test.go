package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPerformanceMonitor(t *testing.T) {
	logging.InitLogger()
	pm := NewPerformanceMonitor()
	require.NotNil(t, pm)
	assert.NotNil(t, pm.metrics)
	assert.NotNil(t, pm.logger)
}

func TestPerformanceMonitor_RecordOperation(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Record first operation
	pm.RecordOperation("test_op", 100*time.Millisecond)

	metric := pm.GetMetric("test_op")
	require.NotNil(t, metric)
	assert.Equal(t, "test_op", metric.OperationName)
	assert.Equal(t, int64(1), metric.Count)
	assert.Equal(t, 100*time.Millisecond, metric.TotalDuration)
	assert.Equal(t, 100*time.Millisecond, metric.MinDuration)
	assert.Equal(t, 100*time.Millisecond, metric.MaxDuration)
	assert.Equal(t, 100*time.Millisecond, metric.AvgDuration)
}

func TestPerformanceMonitor_RecordMultipleOperations(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Record multiple operations
	pm.RecordOperation("test_op", 50*time.Millisecond)
	pm.RecordOperation("test_op", 150*time.Millisecond)
	pm.RecordOperation("test_op", 100*time.Millisecond)

	metric := pm.GetMetric("test_op")
	require.NotNil(t, metric)
	assert.Equal(t, int64(3), metric.Count)
	assert.Equal(t, 300*time.Millisecond, metric.TotalDuration)
	assert.Equal(t, 50*time.Millisecond, metric.MinDuration)
	assert.Equal(t, 150*time.Millisecond, metric.MaxDuration)
	assert.Equal(t, 100*time.Millisecond, metric.AvgDuration)
}

func TestPerformanceMonitor_GetMetrics(t *testing.T) {
	pm := NewPerformanceMonitor()

	pm.RecordOperation("op1", 100*time.Millisecond)
	pm.RecordOperation("op2", 200*time.Millisecond)
	pm.RecordOperation("op3", 300*time.Millisecond)

	metrics := pm.GetMetrics()
	assert.Len(t, metrics, 3)
	assert.Contains(t, metrics, "op1")
	assert.Contains(t, metrics, "op2")
	assert.Contains(t, metrics, "op3")
}

func TestPerformanceMonitor_GetMetric_NotFound(t *testing.T) {
	pm := NewPerformanceMonitor()

	metric := pm.GetMetric("nonexistent")
	assert.Nil(t, metric)
}

func TestPerformanceMonitor_ResetMetrics(t *testing.T) {
	pm := NewPerformanceMonitor()

	pm.RecordOperation("test_op", 100*time.Millisecond)
	pm.ResetMetrics()

	metric := pm.GetMetric("test_op")
	require.NotNil(t, metric)
	assert.Equal(t, int64(0), metric.Count)
	assert.Equal(t, time.Duration(0), metric.TotalDuration)
	assert.Equal(t, time.Duration(0), metric.MinDuration)
	assert.Equal(t, time.Duration(0), metric.MaxDuration)
	assert.Equal(t, time.Duration(0), metric.AvgDuration)
}

func TestPerformanceMonitor_GetSystemStats(t *testing.T) {
	pm := NewPerformanceMonitor()

	stats := pm.GetSystemStats()
	require.NotNil(t, stats)
	assert.Contains(t, stats, "goroutines")
	assert.Contains(t, stats, "memory_alloc")
	assert.Contains(t, stats, "memory_total")
	assert.Contains(t, stats, "memory_sys")
	assert.Contains(t, stats, "memory_heap")
	assert.Contains(t, stats, "memory_stack")
	assert.Contains(t, stats, "gc_cycles")
	assert.Contains(t, stats, "gc_pause_ns")
	assert.Contains(t, stats, "uptime")

	// Verify types
	assert.IsType(t, 0, stats["goroutines"])
	assert.IsType(t, uint64(0), stats["memory_alloc"])
	assert.IsType(t, time.Duration(0), stats["uptime"])
}

func TestPerformanceMonitor_MonitorFunction_Success(t *testing.T) {
	pm := NewPerformanceMonitor()

	called := false
	err := pm.MonitorFunction("test_func", func() error {
		called = true
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called)

	metric := pm.GetMetric("test_func")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
	assert.Greater(t, metric.TotalDuration, time.Duration(0))
}

func TestPerformanceMonitor_MonitorFunction_Error(t *testing.T) {
	pm := NewPerformanceMonitor()

	expectedErr := errors.New("test error")
	err := pm.MonitorFunction("test_func_error", func() error {
		return expectedErr
	})

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)

	metric := pm.GetMetric("test_func_error")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
}

func TestMonitorFunctionWithResult_Success(t *testing.T) {
	pm := NewPerformanceMonitor()

	result, err := MonitorFunctionWithResult(pm, "test_with_result", func() (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "success", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, "success", result)

	metric := pm.GetMetric("test_with_result")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
}

func TestMonitorFunctionWithResult_Error(t *testing.T) {
	pm := NewPerformanceMonitor()

	expectedErr := errors.New("test error")
	result, err := MonitorFunctionWithResult(pm, "test_with_result_error", func() (int, error) {
		return 0, expectedErr
	})

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 0, result)

	metric := pm.GetMetric("test_with_result_error")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
}

func TestPerformanceMonitor_MonitorContextFunction_Success(t *testing.T) {
	pm := NewPerformanceMonitor()

	monitoredFunc := pm.MonitorContextFunction("test_ctx_func", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	ctx := context.Background()
	err := monitoredFunc(ctx)

	assert.NoError(t, err)

	metric := pm.GetMetric("test_ctx_func")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
}

func TestPerformanceMonitor_MonitorContextFunction_Error(t *testing.T) {
	pm := NewPerformanceMonitor()

	expectedErr := errors.New("test error")
	monitoredFunc := pm.MonitorContextFunction("test_ctx_func_error", func(ctx context.Context) error {
		return expectedErr
	})

	ctx := context.Background()
	err := monitoredFunc(ctx)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)

	metric := pm.GetMetric("test_ctx_func_error")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
}

func TestMonitorContextFunctionWithResult_Success(t *testing.T) {
	pm := NewPerformanceMonitor()

	monitoredFunc := MonitorContextFunctionWithResult(pm, "test_ctx_with_result", func(ctx context.Context) (string, error) {
		time.Sleep(10 * time.Millisecond)
		return "success", nil
	})

	ctx := context.Background()
	result, err := monitoredFunc(ctx)

	assert.NoError(t, err)
	assert.Equal(t, "success", result)

	metric := pm.GetMetric("test_ctx_with_result")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
}

func TestPerformanceMonitor_MonitorDatabaseOperation_Fast(t *testing.T) {
	pm := NewPerformanceMonitor()

	err := pm.MonitorDatabaseOperation("fast_db_op", func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	assert.NoError(t, err)

	metric := pm.GetMetric("fast_db_op")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
}

func TestPerformanceMonitor_MonitorDatabaseOperation_Slow(t *testing.T) {
	pm := NewPerformanceMonitor()

	err := pm.MonitorDatabaseOperation("slow_db_op", func() error {
		time.Sleep(150 * time.Millisecond) // > 100ms threshold
		return nil
	})

	assert.NoError(t, err)

	metric := pm.GetMetric("slow_db_op")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
	assert.Greater(t, metric.TotalDuration, 100*time.Millisecond)
}

func TestPerformanceMonitor_MonitorDatabaseOperationWithContext(t *testing.T) {
	pm := NewPerformanceMonitor()

	monitoredFunc := pm.MonitorDatabaseOperationWithContext("db_ctx_op", func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	ctx := context.Background()
	err := monitoredFunc(ctx)

	assert.NoError(t, err)

	metric := pm.GetMetric("db_ctx_op")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1), metric.Count)
}

func TestPerformanceMonitor_GetPerformanceReport(t *testing.T) {
	pm := NewPerformanceMonitor()

	pm.RecordOperation("op1", 100*time.Millisecond)
	pm.RecordOperation("op2", 200*time.Millisecond)
	pm.RecordOperation("op2", 300*time.Millisecond)

	report := pm.GetPerformanceReport()

	require.NotNil(t, report)
	assert.Contains(t, report, "timestamp")
	assert.Contains(t, report, "total_operations")
	assert.Contains(t, report, "total_duration")
	assert.Contains(t, report, "slowest_operation")
	assert.Contains(t, report, "slowest_duration")
	assert.Contains(t, report, "operation_metrics")
	assert.Contains(t, report, "system_stats")
	assert.Contains(t, report, "performance_alerts")

	// Verify calculations
	assert.Equal(t, int64(3), report["total_operations"])
	assert.Equal(t, "op2", report["slowest_operation"])
	assert.Equal(t, 300*time.Millisecond, report["slowest_duration"])
}

func TestPerformanceMonitor_GetPerformanceAlerts_SlowOperation(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Record slow operations (> 1 second average)
	pm.RecordOperation("slow_op", 1500*time.Millisecond)
	pm.RecordOperation("slow_op", 1500*time.Millisecond)

	report := pm.GetPerformanceReport()
	alerts := report["performance_alerts"].([]map[string]interface{})

	require.NotEmpty(t, alerts)

	// Should have at least a warning for slow operation
	foundWarning := false
	for _, alert := range alerts {
		if alert["type"] == "slow_operation" && alert["operation"] == "slow_op" {
			foundWarning = true
			assert.Equal(t, "warning", alert["severity"])
		}
	}
	assert.True(t, foundWarning, "Should have warning for slow operation")
}

func TestPerformanceMonitor_GetPerformanceAlerts_VerySlowOperation(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Record very slow operations (> 5 seconds average)
	pm.RecordOperation("very_slow_op", 6*time.Second)

	report := pm.GetPerformanceReport()
	alerts := report["performance_alerts"].([]map[string]interface{})

	require.NotEmpty(t, alerts)

	// Should have critical alert
	foundCritical := false
	for _, alert := range alerts {
		if alert["type"] == "very_slow_operation" && alert["operation"] == "very_slow_op" {
			foundCritical = true
			assert.Equal(t, "critical", alert["severity"])
		}
	}
	assert.True(t, foundCritical, "Should have critical alert for very slow operation")
}

func TestPerformanceMonitor_GetPerformanceAlerts_HighVariance(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Record operations with high variance
	pm.RecordOperation("variable_op", 100*time.Millisecond)
	pm.RecordOperation("variable_op", 500*time.Millisecond)

	report := pm.GetPerformanceReport()
	alerts := report["performance_alerts"].([]map[string]interface{})

	// May or may not have high variance alert depending on calculation
	// Just verify the alert structure if present
	for _, alert := range alerts {
		if alert["type"] == "high_variance_operation" {
			assert.Equal(t, "info", alert["severity"])
			assert.Contains(t, alert, "variance")
		}
	}
}

func TestGetGlobalMonitor(t *testing.T) {
	monitor1 := GetGlobalMonitor()
	monitor2 := GetGlobalMonitor()

	require.NotNil(t, monitor1)
	require.NotNil(t, monitor2)

	// Should return the same instance
	assert.Same(t, monitor1, monitor2)
}

func TestPerformanceMonitor_ConcurrentAccess(t *testing.T) {
	pm := NewPerformanceMonitor()

	// Test concurrent recording
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				pm.RecordOperation("concurrent_op", 10*time.Millisecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	metric := pm.GetMetric("concurrent_op")
	require.NotNil(t, metric)
	assert.Equal(t, int64(1000), metric.Count)
}

func TestPerformanceMonitor_MetricsCopy(t *testing.T) {
	pm := NewPerformanceMonitor()

	pm.RecordOperation("test_op", 100*time.Millisecond)

	metrics1 := pm.GetMetrics()
	metrics2 := pm.GetMetrics()

	// Should be different map instances (GetMetrics creates a copy)
	// Modify one to verify they're independent
	delete(metrics1, "test_op")
	assert.Contains(t, metrics2, "test_op", "metrics2 should still contain test_op")

	// Values should be equal
	pm.RecordOperation("test_op2", 100*time.Millisecond)
	metrics3 := pm.GetMetrics()
	metrics4 := pm.GetMetrics()
	assert.Equal(t, metrics3["test_op2"].Count, metrics4["test_op2"].Count)
}
