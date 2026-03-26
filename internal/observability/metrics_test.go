package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitMetrics(t *testing.T) {
	// Should not panic
	InitMetrics()
}

func TestShutdownMetrics(t *testing.T) {
	// Should not panic
	ShutdownMetrics()
}

func TestMetricsExist(t *testing.T) {
	// Verify all metrics are initialized
	assert.NotNil(t, RequestDuration)
	assert.NotNil(t, CacheHits)
	assert.NotNil(t, DatabaseOperations)
	assert.NotNil(t, SelfDeclaredUpdates)
	assert.NotNil(t, PhoneVerificationRequests)
	assert.NotNil(t, PhoneVerificationValidations)
	assert.NotNil(t, ActiveConnections)
	assert.NotNil(t, OperationDuration)
	assert.NotNil(t, RMISyncQueueDepth)
	assert.NotNil(t, RMISyncOperationsTotal)
	assert.NotNil(t, RMISyncFailuresTotal)
	assert.NotNil(t, RMICacheHitRatio)
	assert.NotNil(t, RMIDegradedModeActive)
}

func TestRequestDuration(t *testing.T) {
	// Should be able to record metrics
	RequestDuration.WithLabelValues("/test", "GET", "200").Observe(0.5)
	RequestDuration.WithLabelValues("/api/v1/test", "POST", "201").Observe(1.2)
}

func TestCacheHits(t *testing.T) {
	// Should be able to increment counter
	CacheHits.WithLabelValues("get").Inc()
	CacheHits.WithLabelValues("set").Inc()
	CacheHits.WithLabelValues("delete").Inc()
}

func TestDatabaseOperations(t *testing.T) {
	// Should be able to track different operations
	DatabaseOperations.WithLabelValues("insert", "success").Inc()
	DatabaseOperations.WithLabelValues("update", "success").Inc()
	DatabaseOperations.WithLabelValues("delete", "error").Inc()
	DatabaseOperations.WithLabelValues("find", "success").Inc()
}

func TestSelfDeclaredUpdates(t *testing.T) {
	// Should be able to track update status
	SelfDeclaredUpdates.WithLabelValues("success").Inc()
	SelfDeclaredUpdates.WithLabelValues("error").Inc()
	SelfDeclaredUpdates.WithLabelValues("validation_error").Inc()
}

func TestPhoneVerificationMetrics(t *testing.T) {
	// Should be able to track phone verification operations
	PhoneVerificationRequests.WithLabelValues("requested").Inc()
	PhoneVerificationRequests.WithLabelValues("sent").Inc()

	PhoneVerificationValidations.WithLabelValues("success").Inc()
	PhoneVerificationValidations.WithLabelValues("failed").Inc()
	PhoneVerificationValidations.WithLabelValues("expired").Inc()
}

func TestActiveConnections(t *testing.T) {
	// Should be able to set and increment gauge
	ActiveConnections.Set(10)
	ActiveConnections.Inc()
	ActiveConnections.Dec()
	ActiveConnections.Add(5)
	ActiveConnections.Sub(2)
}

func TestOperationDuration(t *testing.T) {
	// Should be able to observe operation durations
	OperationDuration.WithLabelValues("citizen_fetch").Observe(0.1)
	OperationDuration.WithLabelValues("self_declared_update").Observe(0.3)
	OperationDuration.WithLabelValues("cache_lookup").Observe(0.01)
}

func TestRMISyncMetrics(t *testing.T) {
	// Should be able to track sync operations
	RMISyncQueueDepth.WithLabelValues("citizen").Set(100)
	RMISyncQueueDepth.WithLabelValues("self_declared").Set(50)

	RMISyncOperationsTotal.WithLabelValues("citizen", "success").Inc()
	RMISyncOperationsTotal.WithLabelValues("self_declared", "error").Inc()

	RMISyncFailuresTotal.WithLabelValues("citizen").Inc()
	RMISyncFailuresTotal.WithLabelValues("phone_mapping").Inc()
}

func TestRMICacheMetrics(t *testing.T) {
	// Should be able to track cache metrics
	RMICacheHitRatio.WithLabelValues("citizen").Set(0.85)
	RMICacheHitRatio.WithLabelValues("self_declared").Set(0.92)
	RMICacheHitRatio.WithLabelValues("phone_mapping").Set(0.78)
}

func TestRMIDegradedMode(t *testing.T) {
	// Should be able to track degraded mode
	RMIDegradedModeActive.Set(0) // Normal mode
	RMIDegradedModeActive.Set(1) // Degraded mode
	RMIDegradedModeActive.Set(0) // Back to normal
}

func TestMetricsWithMultipleLabels(t *testing.T) {
	// Test metrics with different label combinations
	RequestDuration.WithLabelValues("/api/v1/citizen", "GET", "200").Observe(0.5)
	RequestDuration.WithLabelValues("/api/v1/citizen", "GET", "404").Observe(0.3)
	RequestDuration.WithLabelValues("/api/v1/citizen", "PUT", "200").Observe(0.8)

	DatabaseOperations.WithLabelValues("upsert", "success").Inc()
	DatabaseOperations.WithLabelValues("upsert", "conflict").Inc()

	RMISyncOperationsTotal.WithLabelValues("user_config", "success").Inc()
	RMISyncOperationsTotal.WithLabelValues("user_config", "retry").Inc()
}
