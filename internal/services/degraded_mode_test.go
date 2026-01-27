package services

import (
	"testing"
	"time"
)

func TestNewDegradedMode(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	if dm == nil {
		t.Fatal("NewDegradedMode() returned nil")
	}

	if dm.metrics != metrics {
		t.Error("NewDegradedMode() metrics not set correctly")
	}

	if dm.stopChan == nil {
		t.Error("NewDegradedMode() stopChan is nil")
	}

	if dm.logger == nil {
		t.Error("NewDegradedMode() logger is nil")
	}
}

func TestDegradedMode_Activate(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Initially should not be active
	if dm.IsActive() {
		t.Error("IsActive() initially = true, want false")
	}

	// Activate degraded mode
	dm.Activate("test_reason")

	if !dm.IsActive() {
		t.Error("IsActive() after Activate = false, want true")
	}

	reason := dm.GetReason()
	if reason != "test_reason" {
		t.Errorf("GetReason() = %v, want test_reason", reason)
	}

	// Check metrics were updated
	if !metrics.IsDegradedMode() {
		t.Error("Metrics degraded mode = false, want true after Activate()")
	}
}

func TestDegradedMode_Activate_Idempotent(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Activate multiple times
	dm.Activate("reason1")
	firstActivation := dm.activatedAt

	time.Sleep(10 * time.Millisecond)

	dm.Activate("reason2")
	secondActivation := dm.activatedAt

	// Activation time should not change (idempotent)
	if !firstActivation.Equal(secondActivation) {
		t.Error("Multiple Activate() calls should not change activation time")
	}

	// Reason should remain the same
	if dm.GetReason() != "reason1" {
		t.Errorf("GetReason() = %v, want reason1 (first activation)", dm.GetReason())
	}
}

func TestDegradedMode_Deactivate(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Activate then deactivate
	dm.Activate("test_reason")
	dm.Deactivate()

	if dm.IsActive() {
		t.Error("IsActive() after Deactivate = true, want false")
	}

	reason := dm.GetReason()
	if reason != "" {
		t.Errorf("GetReason() after Deactivate = %v, want empty string", reason)
	}

	// Check metrics were updated
	if metrics.IsDegradedMode() {
		t.Error("Metrics degraded mode = true, want false after Deactivate()")
	}
}

func TestDegradedMode_Deactivate_WhenNotActive(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Deactivate when not active should be safe
	dm.Deactivate()

	if dm.IsActive() {
		t.Error("IsActive() = true, want false")
	}
}

func TestDegradedMode_GetDuration(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Initially duration should be 0
	duration := dm.GetDuration()
	if duration != 0 {
		t.Errorf("GetDuration() when not active = %v, want 0", duration)
	}

	// Activate and check duration
	dm.Activate("test_reason")
	time.Sleep(50 * time.Millisecond)

	duration = dm.GetDuration()
	if duration < 40*time.Millisecond {
		t.Errorf("GetDuration() = %v, should be at least 40ms", duration)
	}

	// After deactivate, duration should be 0 again
	dm.Deactivate()
	duration = dm.GetDuration()
	if duration != 0 {
		t.Errorf("GetDuration() after deactivate = %v, want 0", duration)
	}
}

func TestDegradedMode_GetReason_NotActive(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	reason := dm.GetReason()
	if reason != "" {
		t.Errorf("GetReason() when not active = %v, want empty string", reason)
	}
}

func TestDegradedMode_Stop(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Stop should not panic
	dm.Stop()

	// Note: Calling Stop multiple times will panic due to channel close
	// This is expected behavior - stop should only be called once
}

func TestDegradedMode_ActivateDeactivateCycle(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Multiple activate/deactivate cycles
	for i := 0; i < 5; i++ {
		dm.Activate("reason")
		if !dm.IsActive() {
			t.Errorf("Cycle %d: IsActive() = false, want true", i)
		}

		dm.Deactivate()
		if dm.IsActive() {
			t.Errorf("Cycle %d: IsActive() = true, want false", i)
		}
	}
}

func TestDegradedMode_Concurrent_Activate(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Run concurrent activations
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			dm.Activate("concurrent_test")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should be active
	if !dm.IsActive() {
		t.Error("IsActive() after concurrent Activate = false, want true")
	}
}

func TestDegradedMode_Concurrent_Deactivate(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	dm.Activate("test")

	// Run concurrent deactivations
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			dm.Deactivate()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should be inactive
	if dm.IsActive() {
		t.Error("IsActive() after concurrent Deactivate = true, want false")
	}
}

func TestDegradedMode_Concurrent_IsActive(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	dm.Activate("test")

	// Run concurrent reads
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			_ = dm.IsActive()
			_ = dm.GetReason()
			_ = dm.GetDuration()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should still be active
	if !dm.IsActive() {
		t.Error("IsActive() after concurrent reads = false, want true")
	}
}

func TestDegradedMode_Concurrent_MixedOperations(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	// Run concurrent mixed operations
	done := make(chan bool)

	// Activators
	for i := 0; i < 50; i++ {
		go func() {
			dm.Activate("concurrent_test")
			done <- true
		}()
	}

	// Deactivators
	for i := 0; i < 50; i++ {
		go func() {
			dm.Deactivate()
			done <- true
		}()
	}

	// Readers
	for i := 0; i < 100; i++ {
		go func() {
			_ = dm.IsActive()
			_ = dm.GetReason()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 200; i++ {
		<-done
	}

	// Just verify no panics occurred
	// Final state is non-deterministic but should be valid
	_ = dm.IsActive()
}

func TestDegradedMode_ActivationTimestamp(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	before := time.Now()
	dm.Activate("test_reason")
	after := time.Now()

	// Activation time should be between before and after
	activatedAt := dm.activatedAt
	if activatedAt.Before(before) || activatedAt.After(after) {
		t.Errorf("activatedAt = %v, should be between %v and %v", activatedAt, before, after)
	}
}

func TestDegradedMode_ReasonPersistence(t *testing.T) {
	metrics := NewMetrics()
	dm := NewDegradedMode(nil, nil, metrics)

	reasons := []string{"mongodb_down", "redis_memory_high", "test_reason"}

	for _, reason := range reasons {
		dm.Deactivate() // Reset
		dm.Activate(reason)

		if dm.GetReason() != reason {
			t.Errorf("GetReason() = %v, want %v", dm.GetReason(), reason)
		}
	}
}
