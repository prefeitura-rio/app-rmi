package services

import (
	"context"
	"sync"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.uber.org/zap"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	tokens     int
	maxTokens  int
	refillRate time.Duration
	lastRefill time.Time
	mutex      sync.Mutex
	logger     *logging.SafeLogger
}

// NewRateLimiter creates a new token bucket rate limiter
func NewRateLimiter(maxTokens int, refillRate time.Duration, logger *logging.SafeLogger) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
		logger:     logger,
	}
}

// Allow checks if a request should be allowed based on rate limiting
func (rl *RateLimiter) Allow(ctx context.Context, operation string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)

	// Calculate how many tokens to add
	tokensToAdd := int(elapsed / rl.refillRate)
	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > rl.maxTokens {
			rl.tokens = rl.maxTokens
		}
		rl.lastRefill = now

		rl.logger.Debug("rate limiter tokens refilled",
			zap.String("operation", operation),
			zap.Int("tokens_added", tokensToAdd),
			zap.Int("current_tokens", rl.tokens),
			zap.Int("max_tokens", rl.maxTokens))
	}

	// Check if we have tokens available
	if rl.tokens > 0 {
		rl.tokens--
		rl.logger.Debug("rate limiter allowed request",
			zap.String("operation", operation),
			zap.Int("remaining_tokens", rl.tokens))
		return true
	}

	rl.logger.Warn("rate limiter rejected request",
		zap.String("operation", operation),
		zap.Int("tokens", rl.tokens),
		zap.Int("max_tokens", rl.maxTokens))
	return false
}

// GetStatus returns the current status of the rate limiter
func (rl *RateLimiter) GetStatus() (int, int) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	return rl.tokens, rl.maxTokens
}

// CFRateLimiterManager manages rate limiting for CF lookup operations
type CFRateLimiterManager struct {
	globalLimiter *RateLimiter
	perCPFCache   sync.Map // map[string]*time.Time for per-CPF rate limiting
	logger        *logging.SafeLogger
}

// NewCFRateLimiterManager creates a new CF rate limiter manager
func NewCFRateLimiterManager(maxRequestsPerMinute int, logger *logging.SafeLogger) *CFRateLimiterManager {
	// Calculate refill rate: if we want maxRequestsPerMinute requests per minute,
	// we need to add one token every (60 seconds / maxRequestsPerMinute)
	refillRate := time.Minute / time.Duration(maxRequestsPerMinute)

	return &CFRateLimiterManager{
		globalLimiter: NewRateLimiter(maxRequestsPerMinute, refillRate, logger),
		logger:        logger,
	}
}

// ShouldAllowCFLookup checks if a CF lookup should be allowed for a given CPF
func (m *CFRateLimiterManager) ShouldAllowCFLookup(ctx context.Context, cpf string, perCPFCooldown time.Duration) (bool, string) {
	// Check global rate limit only
	if !m.globalLimiter.Allow(ctx, "cf_lookup") {
		return false, "global rate limit exceeded"
	}

	m.logger.Debug("CF lookup allowed",
		zap.String("cpf", cpf))

	return true, ""
}

// CleanupOldEntries removes old entries from the per-CPF cache
func (m *CFRateLimiterManager) CleanupOldEntries(olderThan time.Duration) {
	cutoff := time.Now().Add(-olderThan)

	m.perCPFCache.Range(func(key, value interface{}) bool {
		if lastLookup, ok := value.(*time.Time); ok {
			if lastLookup.Before(cutoff) {
				m.perCPFCache.Delete(key)
				m.logger.Debug("cleaned up old rate limit entry",
					zap.String("cpf", key.(string)),
					zap.Time("last_lookup", *lastLookup))
			}
		}
		return true
	})
}

// GetGlobalLimiterStatus returns the current status of the global rate limiter
func (m *CFRateLimiterManager) GetGlobalLimiterStatus() (int, int) {
	return m.globalLimiter.GetStatus()
}

// GetCacheSize returns the number of entries in the per-CPF cache
func (m *CFRateLimiterManager) GetCacheSize() int {
	count := 0
	m.perCPFCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Global CF rate limiter instance
var CFRateLimiterInstance *CFRateLimiterManager

// InitCFRateLimiter initializes the global CF rate limiter
func InitCFRateLimiter(maxRequestsPerMinute int, logger *logging.SafeLogger) {
	CFRateLimiterInstance = NewCFRateLimiterManager(maxRequestsPerMinute, logger)
	logger.Info("CF rate limiter initialized",
		zap.Int("max_requests_per_minute", maxRequestsPerMinute))

	// Start cleanup goroutine to remove old entries every hour
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			CFRateLimiterInstance.CleanupOldEntries(24 * time.Hour)
		}
	}()
}
