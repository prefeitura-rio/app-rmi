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

	// OperationMemoryUsage tracks operation memory usage
	OperationMemoryUsage = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "app_rmi_operation_memory_bytes",
			Help: "Memory usage of operations in bytes",
		},
		[]string{"operation"},
	)

	// RedisOperations tracks Redis operations
	RedisOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_rmi_redis_operations_total",
			Help: "Number of Redis operations",
		},
		[]string{"operation", "status"},
	)

	// RedisOperationDuration tracks Redis operation duration
	RedisOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "app_rmi_redis_operation_duration_seconds",
			Help: "Duration of Redis operations in seconds",
		},
		[]string{"operation"},
	)

	// RedisConnectionPool tracks Redis connection pool status
	RedisConnectionPool = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_rmi_redis_connection_pool",
			Help: "Redis connection pool status",
		},
		[]string{"status", "uri"},
	)

	// RedisConnectionPoolSize tracks Redis connection pool configuration
	RedisConnectionPoolSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_rmi_redis_connection_pool_size",
			Help: "Redis connection pool configuration",
		},
		[]string{"type", "uri"},
	)

	// RedisLatency tracks Redis operation latency
	RedisLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "app_rmi_redis_latency_seconds",
			Help: "Redis operation latency in seconds",
		},
		[]string{"operation", "uri"},
	)

	// MongoDBConnectionPool tracks MongoDB connection pool status
	MongoDBConnectionPool = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "app_rmi_mongodb_connection_pool",
			Help: "MongoDB connection pool status",
		},
		[]string{"status", "database"},
	)

	// MongoDBOperationDuration tracks MongoDB operation duration
	MongoDBOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "app_rmi_mongodb_operation_duration_seconds",
			Help: "Duration of MongoDB operations in seconds",
		},
		[]string{"operation", "collection", "database"},
	)
) 