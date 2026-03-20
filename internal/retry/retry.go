package retry

import (
	"context"
	"errors"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

var (
	// ErrMaxRetriesExceeded is returned when the maximum number of retries is exceeded
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")
)

// Config holds retry configuration
type Config struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Logger         *zap.Logger
}

// DefaultConfig returns a default retry configuration
func DefaultConfig(logger *zap.Logger) Config {
	return Config{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Multiplier:     2.0,
		Logger:         logger,
	}
}

// IsRetryable determines if an error should trigger a retry
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// MongoDB network errors and timeouts are retryable
	if mongo.IsNetworkError(err) || mongo.IsTimeout(err) {
		return true
	}

	// Context cancellation is not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for specific MongoDB errors that are retryable
	switch err {
	case mongo.ErrClientDisconnected:
		return true
	}

	return false
}

// WithExponentialBackoff executes a function with exponential backoff retry logic
func WithExponentialBackoff(ctx context.Context, config Config, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration with exponential increase
			backoff := time.Duration(float64(config.InitialBackoff) * math.Pow(config.Multiplier, float64(attempt-1)))
			if backoff > config.MaxBackoff {
				backoff = config.MaxBackoff
			}

			config.Logger.Debug("retrying operation after backoff",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
				zap.Error(lastErr))

			// Wait with context cancellation support
			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Execute the operation
		err := operation()
		if err == nil {
			if attempt > 0 {
				config.Logger.Info("operation succeeded after retry",
					zap.Int("attempts", attempt+1))
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryable(err) {
			config.Logger.Debug("error is not retryable, stopping",
				zap.Error(err))
			return err
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Log retry attempt
		config.Logger.Warn("operation failed, will retry",
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", config.MaxRetries),
			zap.Error(err))
	}

	// All retries exhausted
	config.Logger.Error("all retry attempts exhausted",
		zap.Int("attempts", config.MaxRetries+1),
		zap.Error(lastErr))

	return errors.Join(ErrMaxRetriesExceeded, lastErr)
}

// WithExponentialBackoffValue executes a function with exponential backoff retry logic and returns a value
func WithExponentialBackoffValue[T any](ctx context.Context, config Config, operation func() (T, error)) (T, error) {
	var (
		result  T
		lastErr error
	)

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration with exponential increase
			backoff := time.Duration(float64(config.InitialBackoff) * math.Pow(config.Multiplier, float64(attempt-1)))
			if backoff > config.MaxBackoff {
				backoff = config.MaxBackoff
			}

			config.Logger.Debug("retrying operation after backoff",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
				zap.Error(lastErr))

			// Wait with context cancellation support
			select {
			case <-time.After(backoff):
				// Continue with retry
			case <-ctx.Done():
				return result, ctx.Err()
			}
		}

		// Execute the operation
		val, err := operation()
		if err == nil {
			if attempt > 0 {
				config.Logger.Info("operation succeeded after retry",
					zap.Int("attempts", attempt+1))
			}
			return val, nil
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryable(err) {
			config.Logger.Debug("error is not retryable, stopping",
				zap.Error(err))
			return result, err
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		// Log retry attempt
		config.Logger.Warn("operation failed, will retry",
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", config.MaxRetries),
			zap.Error(err))
	}

	// All retries exhausted
	config.Logger.Error("all retry attempts exhausted",
		zap.Int("attempts", config.MaxRetries+1),
		zap.Error(lastErr))

	return result, errors.Join(ErrMaxRetriesExceeded, lastErr)
}
