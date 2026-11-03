package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const syntheticCFAddress = "Rua Sintetica do Teste, 4321, Bloco Z, Bairro Ficticio"

// mcpHandlerWithEquipments answers the MCP handshake and replies to equipments_by_address with the given result.
func mcpHandlerWithEquipments(cfResult map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("mcp-session-id", "test-session-logs")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req MCPRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{"protocolVersion": "2024-11-05"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/call":
			params := req.Params.(map[string]interface{})
			if params["name"] == "equipments_by_address" {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": cfResult})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{"content": "Instructions loaded"},
			})
		}
	}
}

func observedLogger(level zapcore.Level) (*logging.SafeLogger, *observer.ObservedLogs) {
	core, logs := observer.New(level)
	return logging.NewSafeLogger(zap.New(core)), logs
}

// assertNoAddressInLogs fails when the address shows up in any message or field of the captured entries.
func assertNoAddressInLogs(t *testing.T, logs *observer.ObservedLogs, address string) {
	t.Helper()
	for _, entry := range logs.All() {
		assert.NotContains(t, entry.Message, address, "address in log message %q", entry.Message)
		for key, value := range entry.ContextMap() {
			assert.NotContains(t, fmt.Sprint(value), address, "address in field %q of log %q", key, entry.Message)
		}
	}
}

func requireLogEntry(t *testing.T, logs *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()
	entries := logs.FilterMessage(message).All()
	require.Len(t, entries, 1, "expected exactly one %q log", message)
	return entries[0]
}

func TestFindNearestCF_LogsAddressHashInsteadOfAddress(t *testing.T) {
	cfResult := map[string]interface{}{
		"structuredContent": map[string]interface{}{
			"equipamentos": []interface{}{
				map[string]interface{}{
					"categoria":      "CF",
					"id_equipamento": "cf-logs-1",
					"nome_oficial":   "CF Oficial Teste",
					"nome_popular":   "Clinica Teste",
					"logradouro":     "Rua da Clinica",
					"numero":         "1",
					"bairro":         "Centro",
					"ativo":          true,
				},
			},
		},
	}

	client, server := setupMCPTest(t, mcpHandlerWithEquipments(cfResult))
	defer server.Close()

	logger, logs := observedLogger(zapcore.InfoLevel)
	client.logger = logger

	result, err := client.FindNearestCF(context.Background(), syntheticCFAddress)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.HealthFacility)

	assertNoAddressInLogs(t, logs, syntheticCFAddress)

	expectedHash := hashAddress(syntheticCFAddress)
	for _, message := range []string{"starting CF lookup via MCP", "MCP CF LOOKUP REQUEST", "CF lookup via MCP completed"} {
		entry := requireLogEntry(t, logs, message)
		assert.Equal(t, expectedHash, entry.ContextMap()["address_hash"], "address_hash on %q", message)
		_, hasAddress := entry.ContextMap()["address"]
		assert.False(t, hasAddress, "%q must not carry the address field", message)
	}

	for _, entry := range logs.All() {
		assert.False(t, strings.EqualFold(entry.Message, "RAW MCP RESPONSE"), "raw MCP response body must not be logged at INFO")
		_, hasBody := entry.ContextMap()["body"]
		assert.False(t, hasBody, "log %q must not carry the raw MCP body at INFO", entry.Message)
	}
}

func TestPerformCFLookup_NoResults_LogsAddressHashInsteadOfAddress(t *testing.T) {
	cfResult := map[string]interface{}{
		"isError":     true,
		"textContent": "no equipment found",
	}

	mcpClient, server := setupMCPTest(t, mcpHandlerWithEquipments(cfResult))
	defer server.Close()

	logger, logs := observedLogger(zapcore.DebugLevel)
	service := NewCFLookupService(nil, mcpClient, logger)

	err := service.PerformCFLookup(context.Background(), "12345678909", syntheticCFAddress)
	require.NoError(t, err)

	assertNoAddressInLogs(t, logs, syntheticCFAddress)

	expectedHash := service.GenerateAddressHash(syntheticCFAddress)
	for _, message := range []string{"performing CF lookup", "no health services found for address"} {
		entry := requireLogEntry(t, logs, message)
		assert.Equal(t, expectedHash, entry.ContextMap()["address_hash"], "address_hash on %q", message)
		_, hasAddress := entry.ContextMap()["address"]
		assert.False(t, hasAddress, "%q must not carry the address field", message)
	}
}

func TestHashAddress_MatchesGenerateAddressHash(t *testing.T) {
	service := NewCFLookupService(nil, nil, logging.NewSafeLogger(nil))
	assert.Equal(t, service.GenerateAddressHash(syntheticCFAddress), hashAddress(syntheticCFAddress))
	assert.Len(t, hashAddress(syntheticCFAddress), 64)
	assert.NotContains(t, hashAddress(syntheticCFAddress), "Rua")
}

// useGlobalLogger swaps the process-wide logger the way logging.InitLogger does in cmd/api and cmd/sync.
func useGlobalLogger(t *testing.T, logger *logging.SafeLogger) {
	t.Helper()
	previous := logging.Logger
	logging.Logger = logger
	t.Cleanup(func() { logging.Logger = previous })
}

func TestInitCFLookupService_WiresGlobalLoggerIntoServiceAndMCPClient(t *testing.T) {
	server := httptest.NewServer(mcpHandlerWithEquipments(map[string]interface{}{
		"isError":     true,
		"textContent": "no equipment found",
	}))
	defer server.Close()

	previousConfig := config.AppConfig
	previousInstance := CFLookupServiceInstance
	t.Cleanup(func() {
		config.AppConfig = previousConfig
		CFLookupServiceInstance = previousInstance
	})
	config.AppConfig = &config.Config{
		CFLookupEnabled: true,
		MCPServerURL:    server.URL,
		MCPAuthToken:    "test-token",
	}

	logger, logs := observedLogger(zapcore.DebugLevel)
	useGlobalLogger(t, logger)

	InitCFLookupService()
	require.NotNil(t, CFLookupServiceInstance)
	requireLogEntry(t, logs, "CF lookup service initialized successfully")

	err := CFLookupServiceInstance.PerformCFLookup(context.Background(), "12345678909", syntheticCFAddress)
	require.NoError(t, err)

	assert.Equal(t, "cf_lookup_service", requireLogEntry(t, logs, "performing CF lookup").LoggerName)
	assert.Equal(t, "cf_lookup_service", requireLogEntry(t, logs, "no health services found for address").LoggerName)
	assert.Equal(t, "mcp_client", requireLogEntry(t, logs, "starting CF lookup via MCP").LoggerName)
	assert.Equal(t, "mcp_client", requireLogEntry(t, logs, "MCP CF LOOKUP REQUEST").LoggerName)
	assert.Equal(t, "mcp_client", requireLogEntry(t, logs, "CF lookup via MCP completed").LoggerName)

	assertNoAddressInLogs(t, logs, syntheticCFAddress)
}

func TestInitCFLookupService_LogsMCPSessionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	previousConfig := config.AppConfig
	previousInstance := CFLookupServiceInstance
	t.Cleanup(func() {
		config.AppConfig = previousConfig
		CFLookupServiceInstance = previousInstance
	})
	config.AppConfig = &config.Config{
		CFLookupEnabled: true,
		MCPServerURL:    server.URL,
		MCPAuthToken:    "test-token",
	}

	logger, logs := observedLogger(zapcore.InfoLevel)
	useGlobalLogger(t, logger)

	InitCFLookupService()
	require.NotNil(t, CFLookupServiceInstance)
	CFLookupServiceInstance.mcpClient.retryConfig.MaxRetries = 0

	_, err := CFLookupServiceInstance.mcpClient.FindNearestCF(context.Background(), syntheticCFAddress)
	require.Error(t, err)

	entry := requireLogEntry(t, logs, "session ID acquisition failed")
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	assert.Equal(t, "mcp_client", entry.LoggerName)
	assertNoAddressInLogs(t, logs, syntheticCFAddress)
}

func TestFindNearestCF_DebugLogsOmitPayloadAndResponseBody(t *testing.T) {
	cfResult := map[string]interface{}{
		"structuredContent": map[string]interface{}{
			"endereco_consultado": syntheticCFAddress,
			"equipamentos": []interface{}{
				map[string]interface{}{
					"categoria":      "CF",
					"id_equipamento": "cf-debug-1",
					"nome_oficial":   "CF Oficial Teste",
					"nome_popular":   "Clinica Teste",
					"logradouro":     "Rua da Clinica",
					"numero":         "1",
					"bairro":         "Centro",
					"ativo":          true,
				},
			},
		},
	}

	client, server := setupMCPTest(t, mcpHandlerWithEquipments(cfResult))
	defer server.Close()

	logger, logs := observedLogger(zapcore.DebugLevel)
	client.logger = logger

	result, err := client.FindNearestCF(context.Background(), syntheticCFAddress)
	require.NoError(t, err)
	require.NotNil(t, result)

	assertNoAddressInLogs(t, logs, syntheticCFAddress)

	requests := logs.FilterMessage("sending MCP request").All()
	require.NotEmpty(t, requests)
	methods := make([]string, 0, len(requests))
	for _, entry := range requests {
		fields := entry.ContextMap()
		_, hasPayload := fields["payload"]
		assert.False(t, hasPayload, "sending MCP request must not carry the JSON-RPC payload")
		assert.NotZero(t, fields["payload_bytes"])
		methods = append(methods, fmt.Sprint(fields["method"]))
	}
	assert.Contains(t, methods, "tools/call")
	assert.Contains(t, methods, "initialize")

	for _, entry := range logs.All() {
		fields := entry.ContextMap()
		_, hasBody := fields["body"]
		_, hasResponse := fields["response"]
		assert.False(t, hasBody, "log %q must not carry the raw MCP body", entry.Message)
		assert.False(t, hasResponse, "log %q must not carry the parsed MCP response", entry.Message)
	}

	notification := requireLogEntry(t, logs, "sending MCP notification")
	assert.Equal(t, "notifications/initialized", notification.ContextMap()["method"])
	_, hasPayload := notification.ContextMap()["payload"]
	assert.False(t, hasPayload)
}

func TestFindNearestCF_DebugLogsOmitUpstreamReasonText(t *testing.T) {
	cases := map[string]map[string]interface{}{
		"isError textContent": {
			"isError":     true,
			"textContent": "nenhum equipamento para " + syntheticCFAddress,
		},
		"equipment error": {
			"structuredContent": map[string]interface{}{
				"equipamentos": []interface{}{
					map[string]interface{}{"error": "endereco nao localizado: " + syntheticCFAddress},
				},
			},
		},
	}

	for name, cfResult := range cases {
		t.Run(name, func(t *testing.T) {
			client, server := setupMCPTest(t, mcpHandlerWithEquipments(cfResult))
			defer server.Close()

			logger, logs := observedLogger(zapcore.DebugLevel)
			client.logger = logger

			result, err := client.FindNearestCF(context.Background(), syntheticCFAddress)
			require.NoError(t, err)
			if result != nil {
				assert.Nil(t, result.HealthFacility)
			}

			assertNoAddressInLogs(t, logs, syntheticCFAddress)
			for _, entry := range logs.All() {
				_, hasReason := entry.ContextMap()["reason"]
				assert.False(t, hasReason, "log %q must not carry the upstream reason text", entry.Message)
			}
		})
	}
}
