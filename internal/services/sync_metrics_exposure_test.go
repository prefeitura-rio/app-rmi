package services

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The worker metrics recorded by Metrics must come out of the /metrics handler that the
// sync process serves (observability.NewMetricsServer).
func TestSyncWorkerMetrics_ExposedOnMetricsHandler(t *testing.T) {
	metrics := NewMetrics()
	metrics.RecordQueueDepth("dlq_self_declared_genero", 3)
	metrics.IncrementSyncOperations("self_declared_genero")
	metrics.IncrementSyncFailures("self_declared_nascimento")
	metrics.SetDegradedMode(true)
	defer metrics.SetDegradedMode(false)

	ts := httptest.NewServer(observability.MetricsHandler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	text := string(body)

	assert.Contains(t, text, `rmi_sync_queue_depth{queue="dlq_self_declared_genero"} 3`)
	assert.Contains(t, text, `rmi_sync_operations_total{queue="self_declared_genero",status="success"}`)
	assert.Contains(t, text, `rmi_sync_failures_total{queue="self_declared_nascimento"}`)
	assert.Contains(t, text, "rmi_degraded_mode_active 1")
}

// Every queue the worker consumes must have its DLQ scanned.
func TestMonitoredDLQQueues_CoverAllWorkerQueues(t *testing.T) {
	worker := NewSyncWorker(nil, nil, 0, nil, nil, nil)

	assert.ElementsMatch(t, worker.queues, monitoredDLQQueues())
	for _, queue := range []string{
		"self_declared_genero",
		"self_declared_renda_familiar",
		"self_declared_escolaridade",
		"self_declared_deficiencia",
		"self_declared_nascimento",
	} {
		assert.Contains(t, monitoredDLQQueues(), queue)
	}
}
