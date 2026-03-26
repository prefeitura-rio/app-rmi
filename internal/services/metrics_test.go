package services

import (
	"testing"
	"time"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()

	if m == nil {
		t.Fatal("NewMetrics() returned nil")
	}

	if m.queueDepth == nil {
		t.Error("NewMetrics() queueDepth map is nil")
	}

	if m.syncOperations == nil {
		t.Error("NewMetrics() syncOperations map is nil")
	}

	if m.syncFailures == nil {
		t.Error("NewMetrics() syncFailures map is nil")
	}

	if m.cacheHits == nil {
		t.Error("NewMetrics() cacheHits map is nil")
	}

	if m.cacheMisses == nil {
		t.Error("NewMetrics() cacheMisses map is nil")
	}

	if m.lastSyncTime == nil {
		t.Error("NewMetrics() lastSyncTime map is nil")
	}
}

func TestMetrics_RecordQueueDepth(t *testing.T) {
	m := NewMetrics()

	m.RecordQueueDepth("test_queue", 10)

	depth := m.GetQueueDepth("test_queue")
	if depth != 10 {
		t.Errorf("GetQueueDepth() = %v, want 10", depth)
	}
}

func TestMetrics_RecordQueueDepth_Multiple(t *testing.T) {
	m := NewMetrics()

	m.RecordQueueDepth("queue1", 5)
	m.RecordQueueDepth("queue2", 15)
	m.RecordQueueDepth("queue3", 25)

	if depth := m.GetQueueDepth("queue1"); depth != 5 {
		t.Errorf("GetQueueDepth(queue1) = %v, want 5", depth)
	}

	if depth := m.GetQueueDepth("queue2"); depth != 15 {
		t.Errorf("GetQueueDepth(queue2) = %v, want 15", depth)
	}

	if depth := m.GetQueueDepth("queue3"); depth != 25 {
		t.Errorf("GetQueueDepth(queue3) = %v, want 25", depth)
	}
}

func TestMetrics_RecordQueueDepth_Update(t *testing.T) {
	m := NewMetrics()

	m.RecordQueueDepth("test_queue", 10)
	m.RecordQueueDepth("test_queue", 20)

	depth := m.GetQueueDepth("test_queue")
	if depth != 20 {
		t.Errorf("GetQueueDepth() after update = %v, want 20", depth)
	}
}

func TestMetrics_GetQueueDepth_NonExistent(t *testing.T) {
	m := NewMetrics()

	depth := m.GetQueueDepth("nonexistent")
	if depth != 0 {
		t.Errorf("GetQueueDepth(nonexistent) = %v, want 0", depth)
	}
}

func TestMetrics_IncrementSyncOperations(t *testing.T) {
	m := NewMetrics()

	m.IncrementSyncOperations("test_queue")
	m.IncrementSyncOperations("test_queue")
	m.IncrementSyncOperations("test_queue")

	// Check that last sync time was set
	lastSync := m.GetLastSyncTime("test_queue")
	if lastSync.IsZero() {
		t.Error("GetLastSyncTime() returned zero time")
	}

	// Verify it's recent (within last second)
	if time.Since(lastSync) > 1*time.Second {
		t.Errorf("GetLastSyncTime() = %v, should be recent", lastSync)
	}
}

func TestMetrics_IncrementSyncOperations_Multiple(t *testing.T) {
	m := NewMetrics()

	m.IncrementSyncOperations("queue1")
	m.IncrementSyncOperations("queue2")
	m.IncrementSyncOperations("queue1")

	// Both queues should have last sync times
	if lastSync := m.GetLastSyncTime("queue1"); lastSync.IsZero() {
		t.Error("GetLastSyncTime(queue1) returned zero time")
	}

	if lastSync := m.GetLastSyncTime("queue2"); lastSync.IsZero() {
		t.Error("GetLastSyncTime(queue2) returned zero time")
	}
}

func TestMetrics_IncrementSyncFailures(t *testing.T) {
	m := NewMetrics()

	m.IncrementSyncFailures("test_queue")
	m.IncrementSyncFailures("test_queue")

	// No direct getter for failures, but we can check via GetAllMetrics
	metrics := m.GetAllMetrics()
	if failures, ok := metrics["rmi_sync_failures_total_test_queue"]; ok {
		if failures != int64(2) {
			t.Errorf("sync failures = %v, want 2", failures)
		}
	} else {
		t.Error("sync failures metric not found")
	}
}

func TestMetrics_IncrementCacheHits(t *testing.T) {
	m := NewMetrics()

	m.IncrementCacheHits("test_cache")
	m.IncrementCacheHits("test_cache")
	m.IncrementCacheHits("test_cache")

	ratio := m.GetCacheHitRatio("test_cache")
	if ratio != 1.0 {
		t.Errorf("GetCacheHitRatio() = %v, want 1.0 (all hits)", ratio)
	}
}

func TestMetrics_IncrementCacheMisses(t *testing.T) {
	m := NewMetrics()

	m.IncrementCacheMisses("test_cache")
	m.IncrementCacheMisses("test_cache")

	ratio := m.GetCacheHitRatio("test_cache")
	if ratio != 0.0 {
		t.Errorf("GetCacheHitRatio() = %v, want 0.0 (all misses)", ratio)
	}
}

func TestMetrics_GetCacheHitRatio_Mixed(t *testing.T) {
	m := NewMetrics()

	m.IncrementCacheHits("test_cache")
	m.IncrementCacheHits("test_cache")
	m.IncrementCacheHits("test_cache")
	m.IncrementCacheMisses("test_cache")

	ratio := m.GetCacheHitRatio("test_cache")
	expected := 0.75 // 3 hits / 4 total
	if ratio != expected {
		t.Errorf("GetCacheHitRatio() = %v, want %v", ratio, expected)
	}
}

func TestMetrics_GetCacheHitRatio_Empty(t *testing.T) {
	m := NewMetrics()

	ratio := m.GetCacheHitRatio("nonexistent_cache")
	if ratio != 0.0 {
		t.Errorf("GetCacheHitRatio(nonexistent) = %v, want 0.0", ratio)
	}
}

func TestMetrics_SetDegradedMode(t *testing.T) {
	m := NewMetrics()

	// Initially should be false
	if m.IsDegradedMode() {
		t.Error("IsDegradedMode() initially = true, want false")
	}

	// Set to true
	m.SetDegradedMode(true)
	if !m.IsDegradedMode() {
		t.Error("IsDegradedMode() after SetDegradedMode(true) = false, want true")
	}

	// Set back to false
	m.SetDegradedMode(false)
	if m.IsDegradedMode() {
		t.Error("IsDegradedMode() after SetDegradedMode(false) = true, want false")
	}
}

func TestMetrics_SetDegradedMode_Multiple(t *testing.T) {
	m := NewMetrics()

	m.SetDegradedMode(true)
	m.SetDegradedMode(true)
	m.SetDegradedMode(true)

	if !m.IsDegradedMode() {
		t.Error("IsDegradedMode() after multiple SetDegradedMode(true) = false, want true")
	}

	m.SetDegradedMode(false)
	m.SetDegradedMode(false)

	if m.IsDegradedMode() {
		t.Error("IsDegradedMode() after multiple SetDegradedMode(false) = true, want false")
	}
}

func TestMetrics_GetLastSyncTime_NonExistent(t *testing.T) {
	m := NewMetrics()

	lastSync := m.GetLastSyncTime("nonexistent")
	if !lastSync.IsZero() {
		t.Errorf("GetLastSyncTime(nonexistent) = %v, want zero time", lastSync)
	}
}

func TestMetrics_GetAllMetrics_Empty(t *testing.T) {
	m := NewMetrics()

	metrics := m.GetAllMetrics()

	if metrics == nil {
		t.Fatal("GetAllMetrics() returned nil")
	}

	// Should contain degraded mode metric even if empty
	if _, ok := metrics["rmi_degraded_mode_active"]; !ok {
		t.Error("GetAllMetrics() missing rmi_degraded_mode_active")
	}
}

func TestMetrics_GetAllMetrics_Populated(t *testing.T) {
	m := NewMetrics()

	// Add various metrics
	m.RecordQueueDepth("queue1", 10)
	m.RecordQueueDepth("queue2", 20)
	m.IncrementSyncOperations("queue1")
	m.IncrementSyncOperations("queue1")
	m.IncrementSyncFailures("queue2")
	m.IncrementCacheHits("cache1")
	m.IncrementCacheMisses("cache1")
	m.SetDegradedMode(true)

	metrics := m.GetAllMetrics()

	// Check queue depths
	if depth, ok := metrics["rmi_sync_queue_depth_queue1"]; !ok || depth != int64(10) {
		t.Errorf("queue1 depth = %v, want 10", depth)
	}

	if depth, ok := metrics["rmi_sync_queue_depth_queue2"]; !ok || depth != int64(20) {
		t.Errorf("queue2 depth = %v, want 20", depth)
	}

	// Check sync operations
	if ops, ok := metrics["rmi_sync_operations_total_queue1"]; !ok || ops != int64(2) {
		t.Errorf("queue1 operations = %v, want 2", ops)
	}

	// Check sync failures
	if failures, ok := metrics["rmi_sync_failures_total_queue2"]; !ok || failures != int64(1) {
		t.Errorf("queue2 failures = %v, want 1", failures)
	}

	// Check cache hit ratio
	if ratio, ok := metrics["rmi_cache_hit_ratio_cache1"]; !ok || ratio != 0.5 {
		t.Errorf("cache1 hit ratio = %v, want 0.5", ratio)
	}

	// Check degraded mode
	if degraded, ok := metrics["rmi_degraded_mode_active"]; !ok || degraded != true {
		t.Errorf("degraded mode = %v, want true", degraded)
	}
}

func TestMetrics_Concurrent_RecordQueueDepth(t *testing.T) {
	m := NewMetrics()

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(val int) {
			m.RecordQueueDepth("test_queue", int64(val))
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Just verify it doesn't panic and has some value
	depth := m.GetQueueDepth("test_queue")
	if depth < 0 || depth >= 10 {
		t.Errorf("GetQueueDepth() = %v, should be between 0 and 9", depth)
	}
}

func TestMetrics_Concurrent_IncrementOperations(t *testing.T) {
	m := NewMetrics()

	// Run concurrent increments
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			m.IncrementSyncOperations("test_queue")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Last sync time should be set
	lastSync := m.GetLastSyncTime("test_queue")
	if lastSync.IsZero() {
		t.Error("GetLastSyncTime() should be set after concurrent operations")
	}
}

func TestMetrics_Concurrent_CacheOperations(t *testing.T) {
	m := NewMetrics()

	// Run concurrent cache operations
	done := make(chan bool)
	for i := 0; i < 50; i++ {
		go func() {
			m.IncrementCacheHits("test_cache")
			done <- true
		}()
	}

	for i := 0; i < 50; i++ {
		go func() {
			m.IncrementCacheMisses("test_cache")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Ratio should be 0.5 (50 hits, 50 misses)
	ratio := m.GetCacheHitRatio("test_cache")
	if ratio != 0.5 {
		t.Errorf("GetCacheHitRatio() = %v, want 0.5", ratio)
	}
}
