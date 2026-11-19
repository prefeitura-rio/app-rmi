package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Synthetic personal data (never real): a CPF, a phone with DDI and an e-mail.
const (
	syntheticCPF   = "12345678901"
	syntheticPhone = "5521987654321"
	syntheticEmail = "fulano@example.com"
)

var syntheticPersonalData = []string{syntheticCPF, syntheticPhone, syntheticEmail, "fulano%40example.com"}

func observeGlobalLogger(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	previous := logging.Logger
	logging.Logger = logging.NewSafeLogger(zap.New(core))
	t.Cleanup(func() { logging.Logger = previous })
	return logs
}

func newPIIRouter(middlewares ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middlewares...)
	ok := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) }
	router.GET("/v1/memory/:phone_number", ok)
	router.POST("/v1/citizen/:cpf/phone/validate", ok)
	router.GET("/v1/citizen/:cpf/wallet", ok)
	return router
}

func serve(router *gin.Engine, method, target string) {
	req := httptest.NewRequest(method, target, nil)
	router.ServeHTTP(httptest.NewRecorder(), req)
}

func requestCompletedEntries(logs *observer.ObservedLogs) []observer.LoggedEntry {
	return logs.FilterMessage("request completed").All()
}

func assertNoPersonalDataInLogs(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()
	for _, entry := range logs.All() {
		for key, value := range entry.ContextMap() {
			for _, raw := range syntheticPersonalData {
				assert.NotContains(t, fmt.Sprint(value), raw, "personal data in field %q of log %q", key, entry.Message)
			}
		}
	}
}

func TestRequestLogger_MasksPersonalDataInPathAndQuery(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		target    string
		wantPath  string
		wantQuery string
	}{
		{
			name:     "memory by phone",
			method:   http.MethodGet,
			target:   "/v1/memory/" + syntheticPhone,
			wantPath: "/v1/memory/***",
		},
		{
			name:     "phone validation by CPF",
			method:   http.MethodPost,
			target:   "/v1/citizen/" + syntheticCPF + "/phone/validate",
			wantPath: "/v1/citizen/***/phone/validate",
		},
		{
			name:      "CPF and e-mail in the query",
			method:    http.MethodGet,
			target:    "/v1/citizen/" + syntheticCPF + "/wallet?cpf=" + syntheticCPF + "&email=fulano%40example.com&page=2",
			wantPath:  "/v1/citizen/***/wallet",
			wantQuery: "cpf=***&email=***&page=2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := observeGlobalLogger(t)
			serve(newPIIRouter(RequestLogger()), tt.method, tt.target)

			entries := requestCompletedEntries(logs)
			require.Len(t, entries, 1)
			fields := entries[0].ContextMap()
			assert.Equal(t, tt.wantPath, fields["path"])
			assert.Equal(t, tt.wantQuery, fields["query"])
			assertNoPersonalDataInLogs(t, logs)
		})
	}
}

func TestRequestLogger_MetricLabelHasNoPersonalData(t *testing.T) {
	observeGlobalLogger(t)
	serve(newPIIRouter(RequestLogger()), http.MethodGet, "/v1/memory/"+syntheticPhone)

	status := strconv.Itoa(http.StatusOK)
	assert.False(t, observability.RequestDuration.DeleteLabelValues("/v1/memory/"+syntheticPhone, http.MethodGet, status),
		"request duration histogram labelled with the raw phone")
	assert.True(t, observability.RequestDuration.DeleteLabelValues("/v1/memory/***", http.MethodGet, status),
		"request duration histogram not labelled with the masked path")
}

func TestRequestTiming_MasksPersonalDataInLogAndSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	logs := observeGlobalLogger(t)

	serve(newPIIRouter(RequestTiming()), http.MethodGet, "/v1/citizen/"+syntheticCPF+"/wallet?phone=%2B"+syntheticPhone)

	entries := requestCompletedEntries(logs)
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, "/v1/citizen/***/wallet", fields["path"])
	assert.Equal(t, "/v1/citizen/:cpf/wallet", fields["route"])
	assertNoPersonalDataInLogs(t, logs)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	attrs := map[string]string{}
	for _, attr := range spans[0].Attributes() {
		attrs[string(attr.Key)] = attr.Value.Emit()
	}
	assert.Equal(t, "/v1/citizen/***/wallet?phone=***", attrs["http.url"])
	for key, value := range attrs {
		for _, raw := range syntheticPersonalData {
			assert.False(t, strings.Contains(value, raw), "personal data in span attribute %q", key)
		}
	}
}
