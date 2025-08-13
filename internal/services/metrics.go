package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Metrics holds all the metrics for the sync service
type Metrics struct {
	queueDepth     map[string]int64
	syncOperations map[string]int64
	syncFailures   map[string]int64
	cacheHits      map[string]int64
	cacheMisses    map[string]int64
	degradedMode   int64
	lastSyncTime   map[string]time.Time
	mu             sync.RWMutex
}

// NewMetrics creates new metrics for the sync service
func NewMetrics() *Metrics {
	return &Metrics{
		queueDepth:     make(map[string]int64),
		syncOperations: make(map[string]int64),
		syncFailures:   make(map[string]int64),
		cacheHits:      make(map[string]int64),
		cacheMisses:    make(map[string]int64),
		lastSyncTime:   make(map[string]time.Time),
	}
}

// RecordQueueDepth records the current queue depth
func (m *Metrics) RecordQueueDepth(queue string, depth int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueDepth[queue] = depth

	// Update Prometheus metrics
	observability.RMISyncQueueDepth.WithLabelValues(queue).Set(float64(depth))

	// Send to OTLP via tracer if available
	if span := trace.SpanFromContext(context.Background()); span != nil {
		span.SetAttributes(
			attribute.String("rmi.queue", queue),
			attribute.Int64("rmi.queue_depth", depth),
		)
	}
}

// GetQueueDepth returns the current queue depth
func (m *Metrics) GetQueueDepth(queue string) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.queueDepth[queue]
}

// IncrementSyncOperations increments the sync operations counter
func (m *Metrics) IncrementSyncOperations(queue string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncOperations[queue]++
	m.lastSyncTime[queue] = time.Now()

	// Update Prometheus metrics
	observability.RMISyncOperationsTotal.WithLabelValues(queue, "success").Inc()

	// Send to OTLP via tracer if available
	if span := trace.SpanFromContext(context.Background()); span != nil {
		span.SetAttributes(
			attribute.String("rmi.queue", queue),
			attribute.String("rmi.operation", "sync_success"),
		)
	}
}

// IncrementSyncFailures increments the sync failures counter
func (m *Metrics) IncrementSyncFailures(queue string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncFailures[queue]++

	// Update Prometheus metrics
	observability.RMISyncFailuresTotal.WithLabelValues(queue).Inc()
	observability.RMISyncOperationsTotal.WithLabelValues(queue, "failure").Inc()

	// Send to OTLP via tracer if available
	if span := trace.SpanFromContext(context.Background()); span != nil {
		span.SetAttributes(
			attribute.String("rmi.queue", queue),
			attribute.String("rmi.operation", "sync_failure"),
		)
	}
}

// IncrementCacheHits increments the cache hits counter
func (m *Metrics) IncrementCacheHits(cacheType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheHits[cacheType]++

	// Update Prometheus metrics
	observability.CacheHits.WithLabelValues(cacheType).Inc()

	// Send to OTLP via tracer if available
	if span := trace.SpanFromContext(context.Background()); span != nil {
		span.SetAttributes(
			attribute.String("rmi.cache_type", cacheType),
			attribute.String("rmi.cache_result", "hit"),
		)
	}
}

// IncrementCacheMisses increments the cache misses counter
func (m *Metrics) IncrementCacheMisses(cacheType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cacheMisses[cacheType]++

	// Send to OTLP via tracer if available
	if span := trace.SpanFromContext(context.Background()); span != nil {
		span.SetAttributes(
			attribute.String("rmi.cache_type", cacheType),
			attribute.String("rmi.cache_result", "miss"),
		)
	}
}

// SetDegradedMode sets whether degraded mode is active
func (m *Metrics) SetDegradedMode(active bool) {
	if active {
		atomic.StoreInt64(&m.degradedMode, 1)
		observability.RMIDegradedModeActive.Set(1)
	} else {
		atomic.StoreInt64(&m.degradedMode, 0)
		observability.RMIDegradedModeActive.Set(0)
	}

	// Send to OTLP via tracer if available
	if span := trace.SpanFromContext(context.Background()); span != nil {
		span.SetAttributes(
			attribute.Bool("rmi.degraded_mode", active),
		)
	}
}

// IsDegradedMode returns whether degraded mode is active
func (m *Metrics) IsDegradedMode() bool {
	return atomic.LoadInt64(&m.degradedMode) == 1
}

// GetCacheHitRatio returns the cache hit ratio for a given cache type
func (m *Metrics) GetCacheHitRatio(cacheType string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hits := m.cacheHits[cacheType]
	misses := m.cacheMisses[cacheType]
	total := hits + misses

	if total == 0 {
		return 0.0
	}

	ratio := float64(hits) / float64(total)

	// Update Prometheus metrics
	observability.RMICacheHitRatio.WithLabelValues(cacheType).Set(ratio)

	return ratio
}

// GetLastSyncTime returns the last sync time for a queue
func (m *Metrics) GetLastSyncTime(queue string) time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSyncTime[queue]
}

// GetAllMetrics returns all metrics as a map for monitoring
func (m *Metrics) GetAllMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make(map[string]interface{})

	// Queue depths
	for queue, depth := range m.queueDepth {
		metrics["rmi_sync_queue_depth_"+queue] = depth
	}

	// Sync operations
	for queue, count := range m.syncOperations {
		metrics["rmi_sync_operations_total_"+queue] = count
	}

	// Sync failures
	for queue, count := range m.syncFailures {
		metrics["rmi_sync_failures_total_"+queue] = count
	}

	// Cache hit ratios
	for cacheType := range m.cacheHits {
		metrics["rmi_cache_hit_ratio_"+cacheType] = m.GetCacheHitRatio(cacheType)
	}

	// Degraded mode
	metrics["rmi_degraded_mode_active"] = m.IsDegradedMode()

	return metrics
}
