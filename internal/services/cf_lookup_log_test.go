package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

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

// TestFindNearestCF_DebugLogsCarryNoAddress runs the lookup with DEBUG enabled against an MCP server that echoes the
// address back, so neither the request payload nor the response body may reach the logs.
func TestFindNearestCF_DebugLogsCarryNoAddress(t *testing.T) {
	cfResult := map[string]interface{}{
		"structuredContent": map[string]interface{}{
			"endereco_consultado": syntheticCFAddress,
			"equipamentos": []interface{}{
				map[string]interface{}{
					"categoria":      "CF",
					"id_equipamento": "cf-debug-1",
					"nome_oficial":   "CF Oficial Debug",
					"nome_popular":   "Clinica Debug",
					"logradouro":     "Rua da Clinica",
					"numero":         "2",
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
	require.NotNil(t, result.HealthFacility)

	assertNoAddressInLogs(t, logs, syntheticCFAddress)

	requests := logs.FilterMessage("sending MCP request").All()
	require.Len(t, requests, 3, "initialize, equipments_instructions and equipments_by_address")
	var lookupRequest *observer.LoggedEntry
	for i := range requests {
		fields := requests[i].ContextMap()
		_, hasPayload := fields["payload"]
		assert.False(t, hasPayload, "sending MCP request must not carry the payload")
		assert.Greater(t, fields["payload_bytes"], int64(0))
		if fields["tool"] == "equipments_by_address" {
			lookupRequest = &requests[i]
		}
	}
	require.NotNil(t, lookupRequest, "the equipments_by_address request is logged with its tool name")
	assert.Equal(t, "tools/call", lookupRequest.ContextMap()["method"])

	for _, message := range []string{"raw MCP response", "received MCP response"} {
		for _, entry := range logs.FilterMessage(message).All() {
			fields := entry.ContextMap()
			for _, forbidden := range []string{"body", "response"} {
				_, has := fields[forbidden]
				assert.False(t, has, "%q must not carry the field %q", message, forbidden)
			}
		}
	}
	responses := logs.FilterMessage("raw MCP response").All()
	require.Len(t, responses, 3)
	for _, entry := range responses {
		assert.Greater(t, entry.ContextMap()["body_bytes"], int64(0))
	}
}

func TestDescribeMCPPayload(t *testing.T) {
	toolCall := MCPRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "equipments_by_address",
			"arguments": map[string]interface{}{"address": syntheticCFAddress},
		},
	}

	method, tool := describeMCPPayload(toolCall)
	assert.Equal(t, "tools/call", method)
	assert.Equal(t, "equipments_by_address", tool)

	method, tool = describeMCPPayload(&toolCall)
	assert.Equal(t, "tools/call", method)
	assert.Equal(t, "equipments_by_address", tool)

	method, tool = describeMCPPayload(MCPRequest{JSONRPC: "2.0", Method: "notifications/initialized"})
	assert.Equal(t, "notifications/initialized", method)
	assert.Empty(t, tool)

	var nilRequest *MCPRequest
	method, tool = describeMCPPayload(nilRequest)
	assert.Empty(t, method)
	assert.Empty(t, tool)

	method, tool = describeMCPPayload(map[string]interface{}{"method": "tools/call"})
	assert.Empty(t, method)
	assert.Empty(t, tool)
}

// TestFindNearestCF_DebugNoResultReasonsCarryNoAddress covers the "no results" answers whose server text echoes the
// queried address: the reason reaches the DEBUG log only as its size.
func TestFindNearestCF_DebugNoResultReasonsCarryNoAddress(t *testing.T) {
	cases := map[string]struct {
		cfResult map[string]interface{}
		message  string
	}{
		"isError with textContent": {
			cfResult: map[string]interface{}{
				"isError":     true,
				"textContent": "Nenhum equipamento encontrado para " + syntheticCFAddress,
			},
			message: "MCP server reported no health services available",
		},
		"equipment error": {
			cfResult: map[string]interface{}{
				"structuredContent": map[string]interface{}{
					"equipamentos": []interface{}{
						map[string]interface{}{"error": "endereco nao localizado: " + syntheticCFAddress},
					},
				},
			},
			message: "MCP server reported no equipment found",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			client, server := setupMCPTest(t, mcpHandlerWithEquipments(tc.cfResult))
			defer server.Close()

			logger, logs := observedLogger(zapcore.DebugLevel)
			client.logger = logger

			result, err := client.FindNearestCF(context.Background(), syntheticCFAddress)
			require.NoError(t, err)
			if result != nil {
				assert.Nil(t, result.HealthFacility)
			}

			assertNoAddressInLogs(t, logs, syntheticCFAddress)

			entry := requireLogEntry(t, logs, tc.message)
			_, hasReason := entry.ContextMap()["reason"]
			assert.False(t, hasReason, "%q must not carry the server text", tc.message)
			assert.Greater(t, entry.ContextMap()["reason_bytes"], int64(0))
		})
	}
}
