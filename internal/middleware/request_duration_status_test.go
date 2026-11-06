package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

// requestDurationStatuses returns the status labels of app_rmi_request_duration_seconds
// series recorded for the given path label.
func requestDurationStatuses(t *testing.T, path string) []string {
	t.Helper()

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)

	var statuses []string
	for _, family := range families {
		if family.GetName() != "app_rmi_request_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["path"] == path {
				statuses = append(statuses, labels["status"])
			}
		}
	}
	return statuses
}

func TestRequestLogger_StatusLabelIsHTTPCode(t *testing.T) {
	const path = "/dev370-request-logger-status"
	router := gin.New()
	router.Use(RequestLogger())
	router.GET(path, func(c *gin.Context) {
		c.Status(http.StatusTeapot)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusTeapot, w.Code)

	require.Equal(t, []string{"418"}, requestDurationStatuses(t, path))
}

func TestRequestTiming_StatusLabelIsHTTPCode(t *testing.T) {
	const path = "/dev370-request-timing-status"
	router := gin.New()
	router.Use(RequestTiming())
	router.GET(path, func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	require.Equal(t, http.StatusOK, w.Code)

	require.Equal(t, []string{"200"}, requestDurationStatuses(t, path))
}
