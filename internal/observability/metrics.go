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

	// PhoneVerificationRequests tracks phone verification requests
	PhoneVerificationRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_rmi_phone_verification_requests_total",
			Help: "Number of phone verification requests",
		},
		[]string{"status"},
	)

	// PhoneVerificationValidations tracks phone verification validations
	PhoneVerificationValidations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_rmi_phone_verification_validations_total",
			Help: "Number of phone verification validations",
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

	// OperationDuration tracks operation duration
	OperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "app_rmi_operation_duration_seconds",
			Help: "Duration of operations in seconds",
		},
		[]string{"operation"},
	)

	// RMI Cache and Sync Metrics (Prometheus + OTLP via existing tracer)
	RMISyncQueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rmi_sync_queue_depth",
			Help: "Current depth of sync queues",
		},
		[]string{"queue"},
	)

	RMISyncOperationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rmi_sync_operations_total",
			Help: "Total number of sync operations",
		},
		[]string{"queue", "status"},
	)

	RMISyncFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rmi_sync_failures_total",
			Help: "Total number of sync failures",
		},
		[]string{"queue"},
	)

	RMICacheHitRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rmi_cache_hit_ratio",
			Help: "Cache hit ratio for different cache types",
		},
		[]string{"cache_type"},
	)

	RMIDegradedModeActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rmi_degraded_mode_active",
			Help: "Whether degraded mode is currently active",
		},
	)
)

// InitMetrics initializes the metrics system
func InitMetrics() {
	// Metrics are automatically initialized by promauto
	// This function can be used for any additional setup
}

// ShutdownMetrics shuts down the metrics system
func ShutdownMetrics() {
	// Prometheus metrics don't need explicit shutdown
	// This function can be used for any cleanup if needed
}
