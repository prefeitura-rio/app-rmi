package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

func TestDefaultConfig(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultConfig(logger)

	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", config.MaxRetries)
	}
	if config.InitialBackoff != 100*time.Millisecond {
		t.Errorf("Expected InitialBackoff 100ms, got %v", config.InitialBackoff)
	}
	if config.MaxBackoff != 5*time.Second {
		t.Errorf("Expected MaxBackoff 5s, got %v", config.MaxBackoff)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier 2.0, got %v", config.Multiplier)
	}
}

func TestDefaultConfig_WithNilLogger(t *testing.T) {
	// Test that DefaultConfig handles nil logger gracefully
	config := DefaultConfig(nil)

	if config.Logger == nil {
		t.Error("Expected logger to be non-nil (should use no-op logger)")
	}
	if config.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", config.MaxRetries)
	}
	if config.InitialBackoff != 100*time.Millisecond {
		t.Errorf("Expected InitialBackoff 100ms, got %v", config.InitialBackoff)
	}
	if config.MaxBackoff != 5*time.Second {
		t.Errorf("Expected MaxBackoff 5s, got %v", config.MaxBackoff)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier 2.0, got %v", config.Multiplier)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil error", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
		{"client disconnected", mongo.ErrClientDisconnected, true},
		{"generic error", errors.New("generic"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.retryable {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

func TestWithExponentialBackoff_Success(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultConfig(logger)
	ctx := context.Background()

	attempts := 0
	err := WithExponentialBackoff(ctx, config, func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
}

func TestWithExponentialBackoff_ImmediateSuccess(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultConfig(logger)
	ctx := context.Background()

	err := WithExponentialBackoff(ctx, config, func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestWithExponentialBackoff_SuccessAfterRetries(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		MaxRetries:     3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
		Logger:         logger,
	}
	ctx := context.Background()

	attempts := 0
	err := WithExponentialBackoff(ctx, config, func() error {
		attempts++
		if attempts < 3 {
			return mongo.ErrClientDisconnected // retryable error
		}
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestWithExponentialBackoff_NonRetryableError(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultConfig(logger)
	ctx := context.Background()

	nonRetryableErr := errors.New("non-retryable")
	attempts := 0

	err := WithExponentialBackoff(ctx, config, func() error {
		attempts++
		return nonRetryableErr
	})

	if err != nonRetryableErr {
		t.Errorf("Expected non-retryable error, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempts)
	}
}

func TestWithExponentialBackoff_MaxRetriesExceeded(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
		Logger:         logger,
	}
	ctx := context.Background()

	retryableErr := mongo.ErrClientDisconnected
	attempts := 0

	err := WithExponentialBackoff(ctx, config, func() error {
		attempts++
		return retryableErr
	})

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Expected ErrMaxRetriesExceeded, got %v", err)
	}
	if attempts != 3 { // initial + 2 retries
		t.Errorf("Expected 3 attempts (1 initial + 2 retries), got %d", attempts)
	}
}

func TestWithExponentialBackoff_ContextCanceled(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		Multiplier:     2.0,
		Logger:         logger,
	}

	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	errChan := make(chan error, 1)

	go func() {
		err := WithExponentialBackoff(ctx, config, func() error {
			attempts++
			if attempts == 2 {
				cancel() // Cancel after first retry
			}
			return mongo.ErrClientDisconnected
		})
		errChan <- err
	}()

	err := <-errChan
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestWithExponentialBackoffValue_Success(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultConfig(logger)
	ctx := context.Background()

	result, err := WithExponentialBackoffValue(ctx, config, func() (string, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
}

func TestWithExponentialBackoffValue_Retry(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		MaxRetries:     2,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
		Logger:         logger,
	}
	ctx := context.Background()

	attempts := 0
	result, err := WithExponentialBackoffValue(ctx, config, func() (int, error) {
		attempts++
		if attempts < 2 {
			return 0, mongo.ErrClientDisconnected
		}
		return 42, nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result != 42 {
		t.Errorf("Expected 42, got %v", result)
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestWithExponentialBackoffValue_Failure(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		MaxRetries:     1,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		Multiplier:     2.0,
		Logger:         logger,
	}
	ctx := context.Background()

	result, err := WithExponentialBackoffValue(ctx, config, func() (string, error) {
		return "", mongo.ErrClientDisconnected
	})

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Errorf("Expected ErrMaxRetriesExceeded, got %v", err)
	}
	if result != "" {
		t.Errorf("Expected empty string on error, got %v", result)
	}
}

func TestBackoffCalculation(t *testing.T) {
	logger := zap.NewNop()
	config := Config{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     500 * time.Millisecond,
		Multiplier:     2.0,
		Logger:         logger,
	}
	ctx := context.Background()

	attempts := 0
	startTime := time.Now()

	_ = WithExponentialBackoff(ctx, config, func() error {
		attempts++
		// Always fail to test backoff
		return mongo.ErrClientDisconnected
	})

	duration := time.Since(startTime)

	// Expected backoffs: 100ms, 200ms, 400ms, 500ms (capped), 500ms (capped)
	// Total: ~1700ms minimum
	minExpected := 1700 * time.Millisecond
	if duration < minExpected {
		t.Errorf("Expected at least %v total backoff time, got %v", minExpected, duration)
	}
}
