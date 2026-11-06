package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsServer_PortZeroDisables(t *testing.T) {
	server, err := NewMetricsServer(0)
	require.NoError(t, err)
	assert.Nil(t, server)
}

func TestNewMetricsServer_InvalidPort(t *testing.T) {
	for _, port := range []int{-1, 65536} {
		server, err := NewMetricsServer(port)
		require.Error(t, err, "port %d", port)
		assert.Nil(t, server)
	}
}

func TestNewMetricsServer_ServesMetrics(t *testing.T) {
	server, err := NewMetricsServer(9090)
	require.NoError(t, err)
	require.NotNil(t, server)
	assert.Equal(t, ":9090", server.Addr)
	assert.NotZero(t, server.ReadHeaderTimeout)

	RMIDegradedModeActive.Set(0)

	ts := httptest.NewServer(server.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "rmi_degraded_mode_active 0")

	other, err := http.Get(ts.URL + "/v1/citizen")
	require.NoError(t, err)
	defer other.Body.Close()
	assert.Equal(t, http.StatusNotFound, other.StatusCode)
}
