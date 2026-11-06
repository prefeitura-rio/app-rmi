package observability

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metricsServerReadHeaderTimeout bounds slow clients on the metrics listener.
const metricsServerReadHeaderTimeout = 5 * time.Second

// MetricsHandler serves GET /metrics with every metric of the default Prometheus registry
// (all promauto metrics of the process, including rmi_sync_* and rmi_degraded_mode_active).
func MetricsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

// NewMetricsServer builds the HTTP server that exposes MetricsHandler on its own port, for
// processes without an HTTP API (the sync worker). Port 0 disables it and returns (nil, nil).
func NewMetricsServer(port int) (*http.Server, error) {
	if port == 0 {
		return nil, nil
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid metrics port %d: must be between 1 and 65535, or 0 to disable", port)
	}

	return &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           MetricsHandler(),
		ReadHeaderTimeout: metricsServerReadHeaderTimeout,
	}, nil
}
