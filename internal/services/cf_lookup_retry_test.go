package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	retryTestCPF     = "00000000191"
	retryTestAddress = "Rua Sintetica, 42, Centro, Rio de Janeiro, RJ"
)

func mcpHandlerWithToolResponse(toolResponse map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("mcp-session-id", "test-session")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req MCPRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": map[string]interface{}{}})
		case "tools/call":
			params, _ := req.Params.(map[string]interface{})
			if params["name"] == "equipments_by_address" {
				_ = json.NewEncoder(w).Encode(toolResponse)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": map[string]interface{}{}})
		}
	}
}

func useCFLookupServiceWithMCP(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	client, server := setupMCPTest(t, handler)
	client.retryConfig.MaxRetries = 0
	previous := CFLookupServiceInstance
	CFLookupServiceInstance = NewCFLookupService(nil, client, &logging.SafeLogger{})
	t.Cleanup(func() {
		CFLookupServiceInstance = previous
		server.Close()
	})
}

func newCFLookupTestJob(retryCount int) *SyncJob {
	return &SyncJob{
		ID:         "cf-retry-test",
		Type:       "cf_lookup",
		Key:        retryTestCPF,
		Collection: "cf_lookup",
		Data: map[string]interface{}{
			"cpf":     retryTestCPF,
			"address": retryTestAddress,
		},
		Timestamp:  time.Now(),
		RetryCount: retryCount,
		MaxRetries: 3,
	}
}

func newBareSyncWorker(t *testing.T) *SyncWorker {
	t.Helper()
	require.NoError(t, logging.InitLogger())
	return &SyncWorker{logger: logging.GetLogger()}
}

func TestSyncJobBackoff_CFLookupIsExponentialAndCapped(t *testing.T) {
	expected := map[int]time.Duration{
		0:  0,
		1:  15 * time.Second,
		2:  30 * time.Second,
		3:  60 * time.Second,
		4:  60 * time.Second,
		40: 60 * time.Second,
	}
	for retryCount, want := range expected {
		job := newCFLookupTestJob(retryCount)
		assert.Equal(t, want, syncJobBackoff(job), "retry_count=%d", retryCount)
	}
}

func TestSyncJobBackoff_OtherJobsKeepLinearBackoff(t *testing.T) {
	job := &SyncJob{Type: "citizen", Collection: "citizens", RetryCount: 1}
	assert.Equal(t, 5*time.Second, syncJobBackoff(job))
	job.RetryCount = 2
	assert.Equal(t, 10*time.Second, syncJobBackoff(job))
	job.RetryCount = 20
	assert.Equal(t, 60*time.Second, syncJobBackoff(job))
}

func TestSyncJobMaxRetries(t *testing.T) {
	assert.Equal(t, cfLookupMaxAttempts, syncJobMaxRetries(newCFLookupTestJob(0)),
		"cf_lookup jobs queued with max_retries=3 get the CF retry budget")

	generous := newCFLookupTestJob(0)
	generous.MaxRetries = 8
	assert.Equal(t, 8, syncJobMaxRetries(generous))

	citizen := &SyncJob{Type: "citizen", Collection: "citizens", MaxRetries: 3}
	assert.Equal(t, 3, syncJobMaxRetries(citizen))
}

func TestSyncJobTimeout(t *testing.T) {
	previous := config.AppConfig
	t.Cleanup(func() { config.AppConfig = previous })

	config.AppConfig = &config.Config{}
	assert.Equal(t, 90*time.Second, syncJobTimeout(newCFLookupTestJob(0)))

	config.AppConfig = &config.Config{CFLookupJobTimeout: 2 * time.Minute}
	assert.Equal(t, 2*time.Minute, syncJobTimeout(newCFLookupTestJob(0)))
	assert.Greater(t, syncJobTimeout(newCFLookupTestJob(0)), defaultSyncJobTimeout)

	assert.Equal(t, 30*time.Second, syncJobTimeout(&SyncJob{Type: "citizen", Collection: "citizens"}))
}

func TestCategorizeError_ToolAndSessionErrors(t *testing.T) {
	service := &CFLookupService{}

	toolErr := &MCPToolError{Tool: "equipments_by_address", Code: -32602, Message: "endereço não geocodificado"}
	assert.Equal(t, "tool", service.categorizeError(toolErr))

	sessionToolErr := &MCPToolError{Tool: "equipments_by_address", Code: -32600, Message: "Bad Request: No valid session ID provided"}
	assert.Equal(t, "session", service.categorizeError(sessionToolErr))

	initErr := errors.New("failed to initialize session: MCP initialization error: map[message:Bad Request: invalid session]")
	assert.Equal(t, "session", service.categorizeError(initErr))

	authErr := errors.New("failed to get session ID: session request failed with status 401")
	assert.Equal(t, "authorization", service.categorizeError(authErr))

	internalToolErr := &MCPToolError{Tool: "equipments_by_address", Code: -32603, Message: "Internal error"}
	assert.Equal(t, "server", service.categorizeError(internalToolErr))

	unknownToolErr := &MCPToolError{Tool: "equipments_by_address", Code: -32601, Message: "Method not found"}
	assert.Equal(t, "tool", service.categorizeError(unknownToolErr))
}

func TestCategorizeError_HTTPStatusAndProtocolErrors(t *testing.T) {
	service := &CFLookupService{}
	expected := map[int]string{
		http.StatusUnauthorized:        "authorization",
		http.StatusForbidden:           "authorization",
		http.StatusNotFound:            "session",
		http.StatusRequestTimeout:      "timeout",
		http.StatusTooManyRequests:     "server",
		http.StatusServiceUnavailable:  "server",
		http.StatusInternalServerError: "server",
	}
	for code, want := range expected {
		err := fmt.Errorf("failed to find CF: %w", &MCPHTTPStatusError{StatusCode: code})
		assert.Equal(t, want, service.categorizeError(err), "status %d", code)
	}

	garbage := fmt.Errorf("failed to find CF: %w", &MCPProtocolError{Err: errors.New("failed to unmarshal response: invalid character '<' looking for beginning of value")})
	assert.Equal(t, "protocol", service.categorizeError(garbage), "an unreadable body is not an address validation error")
}

func TestMakeRequest_RetriesTooManyRequests(t *testing.T) {
	requests := 0
	client, server := setupMCPTest(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("rate limited"))
	})
	defer server.Close()
	client.retryConfig.MaxRetries = 1
	client.retryConfig.BaseDelay = time.Millisecond

	_, err := client.makeRequest(context.Background(), "test-session", MCPRequest{JSONRPC: "2.0", Method: "tools/call"})

	var statusErr *MCPHTTPStatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusTooManyRequests, statusErr.StatusCode)
	assert.Equal(t, 2, requests, "429 is retried inside the MCP client")
}

func TestCFLookupError_Permanent(t *testing.T) {
	for _, category := range []string{"authorization", "validation", "tool"} {
		assert.True(t, IsPermanentCFLookupError(&CFLookupError{Category: category, Err: errors.New("x")}), category)
	}
	for _, category := range []string{"timeout", "network", "session", "server", "protocol", "unknown"} {
		assert.False(t, IsPermanentCFLookupError(&CFLookupError{Category: category, Err: errors.New("x")}), category)
	}
	assert.False(t, IsPermanentCFLookupError(errors.New("plain error")))
}

func TestFindNearestCF_ReportsToolError(t *testing.T) {
	client, server := setupMCPTest(t, mcpHandlerWithToolResponse(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"error":   map[string]interface{}{"code": -32602, "message": "endereço não encontrado"},
	}))
	defer server.Close()
	result, err := client.FindNearestCF(context.Background(), retryTestAddress)

	assert.Nil(t, result)
	var toolErr *MCPToolError
	require.ErrorAs(t, err, &toolErr)
	assert.Equal(t, "equipments_by_address", toolErr.Tool)
	assert.Equal(t, -32602, toolErr.Code)
	assert.Equal(t, "endereço não encontrado", toolErr.Message)
}

func TestHandleCFLookupJob_TransientMCPFailureIsRetryable(t *testing.T) {
	useCFLookupServiceWithMCP(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	worker := newBareSyncWorker(t)

	err := worker.handleCFLookupJob(context.Background(), newCFLookupTestJob(0))

	require.Error(t, err)
	assert.False(t, isNonRetryableSyncError(err), "MCP 5xx must be retried with backoff, not sent straight to the DLQ")
	assert.Contains(t, err.Error(), "CF lookup failed")
	assert.NotContains(t, err.Error(), retryTestAddress, "the address must not leak into errors, logs or the DLQ entry")
}

func TestHandleCFLookupJob_RejectedCredentialsAreNotRetried(t *testing.T) {
	useCFLookupServiceWithMCP(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	worker := newBareSyncWorker(t)

	err := worker.handleCFLookupJob(context.Background(), newCFLookupTestJob(0))

	require.Error(t, err)
	assert.True(t, isNonRetryableSyncError(err))
	assert.Contains(t, err.Error(), "(authorization)")
	assert.NotContains(t, err.Error(), retryTestAddress)
}

func TestHandleCFLookupJob_ToolErrorIsNotRetried(t *testing.T) {
	useCFLookupServiceWithMCP(t, mcpHandlerWithToolResponse(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"error":   map[string]interface{}{"code": -32602, "message": "endereço não encontrado"},
	}))
	worker := newBareSyncWorker(t)

	err := worker.handleCFLookupJob(context.Background(), newCFLookupTestJob(0))

	require.Error(t, err)
	assert.True(t, isNonRetryableSyncError(err))
	assert.Contains(t, err.Error(), "(tool)")
	assert.NotContains(t, err.Error(), retryTestAddress)
}

func TestHandleCFLookupJob_RateLimitedToolCallIsRetryable(t *testing.T) {
	useCFLookupServiceWithMCP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("mcp-session-id", "test-session")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req MCPRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": map[string]interface{}{}})
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limited"))
		}
	})
	worker := newBareSyncWorker(t)

	err := worker.handleCFLookupJob(context.Background(), newCFLookupTestJob(0))

	require.Error(t, err)
	assert.False(t, isNonRetryableSyncError(err), "a rate-limited MCP must be retried with backoff, not sent to the DLQ")
	assert.Contains(t, err.Error(), "(server)")
}

func TestHandleCFLookupJob_UnreadableToolResponseIsRetryable(t *testing.T) {
	useCFLookupServiceWithMCP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("mcp-session-id", "test-session")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req MCPRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": map[string]interface{}{}})
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("<html>gateway</html>"))
		}
	})
	worker := newBareSyncWorker(t)

	err := worker.handleCFLookupJob(context.Background(), newCFLookupTestJob(0))

	require.Error(t, err)
	assert.False(t, isNonRetryableSyncError(err))
	assert.Contains(t, err.Error(), "(protocol)")
}

func TestHandleCFLookupJob_InternalToolErrorIsRetryable(t *testing.T) {
	useCFLookupServiceWithMCP(t, mcpHandlerWithToolResponse(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"error":   map[string]interface{}{"code": -32603, "message": "Internal error"},
	}))
	worker := newBareSyncWorker(t)

	err := worker.handleCFLookupJob(context.Background(), newCFLookupTestJob(0))

	require.Error(t, err)
	assert.False(t, isNonRetryableSyncError(err), "JSON-RPC internal error is a server failure, not a property of the address")
	assert.Contains(t, err.Error(), "(server)")
}
