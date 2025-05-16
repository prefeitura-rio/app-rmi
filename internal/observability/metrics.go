package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestDuration tracks request duration
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "app_rmi_request_duration_seconds",
			Help: "Duration of HTTP requests in seconds",
		},
		[]string{"path", "method", "status"},
	)

	// CacheHits tracks cache hits/misses
	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_rmi_cache_hits_total",
			Help: "Number of cache hits",
		},
		[]string{"operation"},
	)

	// DatabaseOperations tracks database operations
	DatabaseOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_rmi_database_operations_total",
			Help: "Number of database operations",
		},
		[]string{"operation", "status"},
	)

	// SelfDeclaredUpdates tracks self-declared data updates
	SelfDeclaredUpdates = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_rmi_self_declared_updates_total",
			Help: "Number of self-declared data updates",
		},
		[]string{"status"},
	)

	// ActiveConnections tracks active connections
	ActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "app_rmi_active_connections",
			Help: "Number of active connections",
		},
	)
) 