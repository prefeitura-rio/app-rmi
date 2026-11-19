package main

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

func getMetrics(url string) (int, string, error) {
	resp, err := http.Get(url) //nolint:gosec // test-only request to a local server
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), err
}

func TestStartMetricsServer_ServesWorkerMetrics(t *testing.T) {
	port := freePort(t)
	observability.RMISyncQueueDepth.WithLabelValues("dlq_self_declared_nascimento").Set(4)

	server := startMetricsServer(port)
	require.NotNil(t, server)

	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/metrics"
	var (
		status int
		body   string
		err    error
	)
	require.Eventually(t, func() bool {
		status, body, err = getMetrics(url)
		return err == nil
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, `rmi_sync_queue_depth{queue="dlq_self_declared_nascimento"} 4`)
	assert.Contains(t, body, "rmi_degraded_mode_active")

	stopMetricsServer(server)

	_, _, err = getMetrics(url)
	assert.Error(t, err, "metrics server must be closed after stopMetricsServer")
}

func TestStartMetricsServer_Disabled(t *testing.T) {
	assert.Nil(t, startMetricsServer(0))
	assert.Nil(t, startMetricsServer(70000))
	stopMetricsServer(nil)
}
