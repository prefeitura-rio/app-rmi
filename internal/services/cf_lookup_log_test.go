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
