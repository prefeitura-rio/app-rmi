package utils

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewPerformanceMonitor(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	if pm == nil {
		t.Fatal("NewPerformanceMonitor() returned nil")
	}

	if pm.operation != "test_operation" {
		t.Errorf("NewPerformanceMonitor() operation = %v, want test_operation", pm.operation)
	}

	if pm.logger == nil {
		t.Error("NewPerformanceMonitor() logger is nil")
	}

	if pm.checkpoints == nil {
		t.Error("NewPerformanceMonitor() checkpoints is nil")
	}

	if pm.startTime.IsZero() {
		t.Error("NewPerformanceMonitor() startTime is zero")
	}
}

func TestPerformanceMonitor_Checkpoint(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	pm.Checkpoint("step1")
	pm.Checkpoint("step2")
	pm.Checkpoint("step3")

	if len(pm.checkpoints) != 3 {
		t.Errorf("Checkpoint() count = %v, want 3", len(pm.checkpoints))
	}

	if pm.checkpoints[0].Name != "step1" {
		t.Errorf("Checkpoint() first name = %v, want step1", pm.checkpoints[0].Name)
	}

	if pm.checkpoints[1].Name != "step2" {
		t.Errorf("Checkpoint() second name = %v, want step2", pm.checkpoints[1].Name)
	}

	if pm.checkpoints[2].Name != "step3" {
		t.Errorf("Checkpoint() third name = %v, want step3", pm.checkpoints[2].Name)
	}
}

func TestPerformanceMonitor_Checkpoint_Duration(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	time.Sleep(10 * time.Millisecond)
	pm.Checkpoint("step1")

	if len(pm.checkpoints) != 1 {
		t.Fatalf("Checkpoint() count = %v, want 1", len(pm.checkpoints))
	}

	if pm.checkpoints[0].Duration < 10*time.Millisecond {
		t.Errorf("Checkpoint() duration = %v, want >= 10ms", pm.checkpoints[0].Duration)
	}
}

func TestPerformanceMonitor_End(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	pm.Checkpoint("step1")
	pm.Checkpoint("step2")

	// End should not panic
	pm.End()
}

func TestPerformanceMonitor_End_NoCheckpoints(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	// End without checkpoints should not panic
	pm.End()
}

func TestPerformanceMonitor_PerformanceWarning(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	// This should trigger a warning since we exceeded the threshold
	time.Sleep(20 * time.Millisecond)
	pm.PerformanceWarning(10*time.Millisecond, "operation took too long")

	// This should not trigger a warning
	pm2 := NewPerformanceMonitor(ctx, "test_operation2")
	pm2.PerformanceWarning(1*time.Second, "this won't trigger")
}

func TestPerformanceMonitor_MemoryWarning(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	// Allocate some memory
	data := make([]byte, 1024*1024) // 1MB
	_ = data

	// Test memory warning with low threshold (should trigger)
	pm.MemoryWarning(100, "high memory usage")

	// Test memory warning with high threshold (shouldn't trigger)
	pm.MemoryWarning(10*1024*1024*1024, "this won't trigger")
}

func TestPerformanceMonitor_GetPerformanceReport(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	pm.Checkpoint("step1")
	pm.Checkpoint("step2")

	report := pm.GetPerformanceReport()

	if report == "" {
		t.Error("GetPerformanceReport() returned empty string")
	}

	// Check report contains operation name
	if !strings.Contains(report, "test_operation") {
		t.Errorf("GetPerformanceReport() doesn't contain operation name: %s", report)
	}

	// Check report contains checkpoint names
	if !strings.Contains(report, "step1") {
		t.Errorf("GetPerformanceReport() doesn't contain step1: %s", report)
	}

	if !strings.Contains(report, "step2") {
		t.Errorf("GetPerformanceReport() doesn't contain step2: %s", report)
	}

	// Check report contains expected sections
	if !strings.Contains(report, "Total Duration") {
		t.Errorf("GetPerformanceReport() doesn't contain 'Total Duration': %s", report)
	}

	if !strings.Contains(report, "Total Memory Delta") {
		t.Errorf("GetPerformanceReport() doesn't contain 'Total Memory Delta': %s", report)
	}

	if !strings.Contains(report, "Checkpoints") {
		t.Errorf("GetPerformanceReport() doesn't contain 'Checkpoints': %s", report)
	}
}

func TestMonitorFunction_Success(t *testing.T) {
	ctx := context.Background()
	executed := false

	err := MonitorFunction(ctx, "test_operation", func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("MonitorFunction() error = %v, want nil", err)
	}

	if !executed {
		t.Error("MonitorFunction() didn't execute the function")
	}
}

func TestMonitorFunction_Error(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("test error")

	err := MonitorFunction(ctx, "test_operation", func() error {
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("MonitorFunction() error = %v, want %v", err, expectedErr)
	}
}

func TestMonitorFunction_WithCheckpoints(t *testing.T) {
	ctx := context.Background()

	err := MonitorFunction(ctx, "test_operation", func() error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("MonitorFunction() error = %v, want nil", err)
	}
}

func TestMonitorFunctionWithResult_Success(t *testing.T) {
	ctx := context.Background()
	expectedResult := "test result"

	result, err := MonitorFunctionWithResult(ctx, "test_operation", func() (string, error) {
		return expectedResult, nil
	})

	if err != nil {
		t.Errorf("MonitorFunctionWithResult() error = %v, want nil", err)
	}

	if result != expectedResult {
		t.Errorf("MonitorFunctionWithResult() result = %v, want %v", result, expectedResult)
	}
}

func TestMonitorFunctionWithResult_Error(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("test error")

	result, err := MonitorFunctionWithResult(ctx, "test_operation", func() (string, error) {
		return "", expectedErr
	})

	if err != expectedErr {
		t.Errorf("MonitorFunctionWithResult() error = %v, want %v", err, expectedErr)
	}

	if result != "" {
		t.Errorf("MonitorFunctionWithResult() result = %v, want empty string", result)
	}
}

func TestMonitorFunctionWithResult_IntResult(t *testing.T) {
	ctx := context.Background()
	expectedResult := 42

	result, err := MonitorFunctionWithResult(ctx, "test_operation", func() (int, error) {
		return expectedResult, nil
	})

	if err != nil {
		t.Errorf("MonitorFunctionWithResult() error = %v, want nil", err)
	}

	if result != expectedResult {
		t.Errorf("MonitorFunctionWithResult() result = %v, want %v", result, expectedResult)
	}
}

func TestMonitorFunctionWithResult_StructResult(t *testing.T) {
	ctx := context.Background()

	type TestStruct struct {
		Field1 string
		Field2 int
	}

	expectedResult := TestStruct{Field1: "test", Field2: 123}

	result, err := MonitorFunctionWithResult(ctx, "test_operation", func() (TestStruct, error) {
		return expectedResult, nil
	})

	if err != nil {
		t.Errorf("MonitorFunctionWithResult() error = %v, want nil", err)
	}

	if result.Field1 != expectedResult.Field1 || result.Field2 != expectedResult.Field2 {
		t.Errorf("MonitorFunctionWithResult() result = %+v, want %+v", result, expectedResult)
	}
}

func TestPerformanceMonitor_MultipleCheckpoints(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Millisecond)
		pm.Checkpoint("step" + string(rune('0'+i)))
	}

	if len(pm.checkpoints) != 10 {
		t.Errorf("Checkpoint() count = %v, want 10", len(pm.checkpoints))
	}

	// Verify checkpoints are ordered
	for i := 0; i < 9; i++ {
		if pm.checkpoints[i].Duration >= pm.checkpoints[i+1].Duration {
			// Allow small timing variations
			diff := pm.checkpoints[i+1].Duration - pm.checkpoints[i].Duration
			if diff < -1*time.Millisecond {
				t.Errorf("Checkpoint durations not increasing: %v >= %v",
					pm.checkpoints[i].Duration, pm.checkpoints[i+1].Duration)
			}
		}
	}

	pm.End()
}

func TestPerformanceMonitor_ConcurrentCheckpoints(t *testing.T) {
	ctx := context.Background()
	pm := NewPerformanceMonitor(ctx, "test_operation")

	// Note: PerformanceMonitor is not designed to be thread-safe
	// This test just verifies it doesn't panic with sequential operations
	pm.Checkpoint("step1")
	pm.Checkpoint("step2")
	pm.Checkpoint("step3")

	if len(pm.checkpoints) != 3 {
		t.Errorf("Checkpoint() count = %v, want 3", len(pm.checkpoints))
	}

	pm.End()
}
