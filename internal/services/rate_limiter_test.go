package services

import (
	"context"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
)

func TestNewRateLimiter(t *testing.T) {
	logger := logging.Logger
	rl := NewRateLimiter(10, 100*time.Millisecond, logger)

	if rl == nil {
		t.Fatal("NewRateLimiter() returned nil")
	}

	tokens, maxTokens := rl.GetStatus()
	if tokens != 10 {
		t.Errorf("NewRateLimiter() initial tokens = %v, want 10", tokens)
	}

	if maxTokens != 10 {
		t.Errorf("NewRateLimiter() maxTokens = %v, want 10", maxTokens)
	}
}

func TestRateLimiter_Allow_InitialTokens(t *testing.T) {
	logger := logging.Logger
	rl := NewRateLimiter(3, 1*time.Second, logger)
	ctx := context.Background()

	// Should allow first 3 requests
	if !rl.Allow(ctx, "test_op") {
		t.Error("Allow() first request = false, want true")
	}

	if !rl.Allow(ctx, "test_op") {
		t.Error("Allow() second request = false, want true")
	}

	if !rl.Allow(ctx, "test_op") {
		t.Error("Allow() third request = false, want true")
	}

	// Fourth request should be denied
	if rl.Allow(ctx, "test_op") {
		t.Error("Allow() fourth request = true, want false (no tokens left)")
	}
}

func TestRateLimiter_Allow_TokenRefill(t *testing.T) {
	logger := logging.Logger
	rl := NewRateLimiter(2, 50*time.Millisecond, logger)
	ctx := context.Background()

	// Consume all tokens
	rl.Allow(ctx, "test_op")
	rl.Allow(ctx, "test_op")

	// Should be denied (no tokens)
	if rl.Allow(ctx, "test_op") {
		t.Error("Allow() should be false when no tokens available")
	}

	// Wait for refill
	time.Sleep(60 * time.Millisecond)

	// Should be allowed now (tokens refilled)
	if !rl.Allow(ctx, "test_op") {
		t.Error("Allow() should be true after token refill")
	}
}

func TestRateLimiter_Allow_MultipleRefills(t *testing.T) {
	logger := logging.Logger
	rl := NewRateLimiter(5, 20*time.Millisecond, logger)
	ctx := context.Background()

	// Consume all tokens
	for i := 0; i < 5; i++ {
		rl.Allow(ctx, "test_op")
	}

	// Wait for multiple refills
	time.Sleep(150 * time.Millisecond) // Should refill at least 7 tokens (150ms / 20ms per token)

	// Try to allow a request - this will trigger refill
	allowed := rl.Allow(ctx, "test_op")
	if !allowed {
		t.Error("Allow() should be true after waiting for refill")
	}

	// Check we got tokens back
	tokens, _ := rl.GetStatus()
	if tokens < 2 {
		t.Errorf("GetStatus() tokens = %v, want at least 2 (should have refilled and used 1)", tokens)
	}
}

func TestRateLimiter_GetStatus(t *testing.T) {
	logger := logging.Logger
	rl := NewRateLimiter(10, 100*time.Millisecond, logger)
	ctx := context.Background()

	tokens, maxTokens := rl.GetStatus()
	if tokens != 10 {
		t.Errorf("GetStatus() tokens = %v, want 10", tokens)
	}

	if maxTokens != 10 {
		t.Errorf("GetStatus() maxTokens = %v, want 10", maxTokens)
	}

	// Consume some tokens
	rl.Allow(ctx, "test_op")
	rl.Allow(ctx, "test_op")
	rl.Allow(ctx, "test_op")

	tokens, maxTokens = rl.GetStatus()
	if tokens != 7 {
		t.Errorf("GetStatus() after 3 requests tokens = %v, want 7", tokens)
	}

	if maxTokens != 10 {
		t.Errorf("GetStatus() maxTokens should not change = %v, want 10", maxTokens)
	}
}

func TestRateLimiter_MaxTokensCap(t *testing.T) {
	logger := logging.Logger
	rl := NewRateLimiter(5, 10*time.Millisecond, logger)
	ctx := context.Background()

	// Wait long enough to potentially refill way more than max
	time.Sleep(200 * time.Millisecond)

	// Try to use tokens
	tokens, maxTokens := rl.GetStatus()
	if tokens > maxTokens {
		t.Errorf("GetStatus() tokens = %v exceeds maxTokens = %v", tokens, maxTokens)
	}

	if tokens != 5 {
		t.Errorf("GetStatus() tokens = %v, want 5 (capped at max)", tokens)
	}

	// Should allow exactly maxTokens requests
	for i := 0; i < 5; i++ {
		if !rl.Allow(ctx, "test_op") {
			t.Errorf("Allow() request %d = false, want true", i)
		}
	}

	// Next should fail
	if rl.Allow(ctx, "test_op") {
		t.Error("Allow() beyond max tokens = true, want false")
	}
}

func TestNewCFRateLimiterManager(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(60, logger)

	if manager == nil {
		t.Fatal("NewCFRateLimiterManager() returned nil")
	}

	if manager.globalLimiter == nil {
		t.Error("NewCFRateLimiterManager() globalLimiter is nil")
	}

	if manager.logger == nil {
		t.Error("NewCFRateLimiterManager() logger is nil")
	}
}

func TestCFRateLimiterManager_ShouldAllowCFLookup(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(10, logger)
	ctx := context.Background()

	// Should allow first requests
	allowed, reason := manager.ShouldAllowCFLookup(ctx, "12345678901", 1*time.Minute)
	if !allowed {
		t.Errorf("ShouldAllowCFLookup() allowed = false, want true (reason: %s)", reason)
	}

	if reason != "" {
		t.Errorf("ShouldAllowCFLookup() reason = %v, want empty string", reason)
	}
}

func TestCFRateLimiterManager_ShouldAllowCFLookup_GlobalLimit(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(3, logger)
	ctx := context.Background()

	// Consume all global tokens
	manager.ShouldAllowCFLookup(ctx, "11111111111", 1*time.Minute)
	manager.ShouldAllowCFLookup(ctx, "22222222222", 1*time.Minute)
	manager.ShouldAllowCFLookup(ctx, "33333333333", 1*time.Minute)

	// Should be denied due to global rate limit
	allowed, reason := manager.ShouldAllowCFLookup(ctx, "44444444444", 1*time.Minute)
	if allowed {
		t.Error("ShouldAllowCFLookup() allowed = true, want false (global limit exceeded)")
	}

	if reason != "global rate limit exceeded" {
		t.Errorf("ShouldAllowCFLookup() reason = %v, want 'global rate limit exceeded'", reason)
	}
}

func TestCFRateLimiterManager_GetGlobalLimiterStatus(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(20, logger)

	tokens, maxTokens := manager.GetGlobalLimiterStatus()
	if tokens != 20 {
		t.Errorf("GetGlobalLimiterStatus() tokens = %v, want 20", tokens)
	}

	if maxTokens != 20 {
		t.Errorf("GetGlobalLimiterStatus() maxTokens = %v, want 20", maxTokens)
	}
}

func TestCFRateLimiterManager_GetCacheSize_Empty(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(10, logger)

	size := manager.GetCacheSize()
	if size != 0 {
		t.Errorf("GetCacheSize() = %v, want 0 (empty cache)", size)
	}
}

func TestCFRateLimiterManager_CleanupOldEntries_Empty(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(10, logger)

	// Should not panic on empty cache
	manager.CleanupOldEntries(1 * time.Hour)

	size := manager.GetCacheSize()
	if size != 0 {
		t.Errorf("GetCacheSize() after cleanup = %v, want 0", size)
	}
}

func TestCFRateLimiterManager_CleanupOldEntries_WithData(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(10, logger)

	// Add some old entries
	oldTime := time.Now().Add(-2 * time.Hour)
	manager.perCPFCache.Store("old_cpf_1", &oldTime)
	manager.perCPFCache.Store("old_cpf_2", &oldTime)

	// Add a recent entry
	recentTime := time.Now()
	manager.perCPFCache.Store("recent_cpf", &recentTime)

	initialSize := manager.GetCacheSize()
	if initialSize != 3 {
		t.Errorf("GetCacheSize() before cleanup = %v, want 3", initialSize)
	}

	// Cleanup entries older than 1 hour
	manager.CleanupOldEntries(1 * time.Hour)

	finalSize := manager.GetCacheSize()
	if finalSize != 1 {
		t.Errorf("GetCacheSize() after cleanup = %v, want 1 (only recent entry)", finalSize)
	}

	// Verify recent entry is still there
	_, exists := manager.perCPFCache.Load("recent_cpf")
	if !exists {
		t.Error("Recent entry should still exist after cleanup")
	}
}

func TestCFRateLimiterManager_CleanupOldEntries_AllOld(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(10, logger)

	// Add only old entries
	oldTime := time.Now().Add(-3 * time.Hour)
	manager.perCPFCache.Store("old_cpf_1", &oldTime)
	manager.perCPFCache.Store("old_cpf_2", &oldTime)
	manager.perCPFCache.Store("old_cpf_3", &oldTime)

	// Cleanup entries older than 1 hour
	manager.CleanupOldEntries(1 * time.Hour)

	finalSize := manager.GetCacheSize()
	if finalSize != 0 {
		t.Errorf("GetCacheSize() after cleanup = %v, want 0 (all entries old)", finalSize)
	}
}

func TestRateLimiter_Concurrent_Allow(t *testing.T) {
	logger := logging.Logger
	rl := NewRateLimiter(100, 10*time.Millisecond, logger)
	ctx := context.Background()

	// Run concurrent requests
	done := make(chan bool)
	allowedCount := 0
	deniedCount := 0
	results := make(chan bool, 150)

	for i := 0; i < 150; i++ {
		go func() {
			allowed := rl.Allow(ctx, "concurrent_test")
			results <- allowed
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 150; i++ {
		<-done
	}

	close(results)

	// Count results
	for allowed := range results {
		if allowed {
			allowedCount++
		} else {
			deniedCount++
		}
	}

	// We started with 100 tokens, so at most 100 should be allowed
	if allowedCount > 100 {
		t.Errorf("Concurrent Allow() allowed = %v, should not exceed 100", allowedCount)
	}

	// At least 50 should be denied (150 requests - 100 tokens)
	if deniedCount < 50 {
		t.Errorf("Concurrent Allow() denied = %v, should be at least 50", deniedCount)
	}

	// Total should be 150
	if allowedCount+deniedCount != 150 {
		t.Errorf("Concurrent Allow() total = %v, want 150", allowedCount+deniedCount)
	}
}

func TestRateLimiter_Concurrent_GetStatus(t *testing.T) {
	logger := logging.Logger
	rl := NewRateLimiter(50, 10*time.Millisecond, logger)

	// Run concurrent GetStatus calls
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			tokens, maxTokens := rl.GetStatus()
			if tokens < 0 || tokens > maxTokens {
				t.Errorf("GetStatus() tokens = %v, maxTokens = %v (invalid state)", tokens, maxTokens)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestCFRateLimiterManager_Concurrent_ShouldAllow(t *testing.T) {
	logger := logging.Logger
	manager := NewCFRateLimiterManager(50, logger)
	ctx := context.Background()

	// Run concurrent lookups
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(id int) {
			cpf := "1234567890" + string(rune('0'+id%10))
			manager.ShouldAllowCFLookup(ctx, cpf, 1*time.Minute)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Check global limiter was used
	tokens, maxTokens := manager.GetGlobalLimiterStatus()
	if tokens >= maxTokens {
		t.Error("Global limiter should have consumed some tokens")
	}
}
