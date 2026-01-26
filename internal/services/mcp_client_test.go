package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupMCPTest creates a test MCP client with a custom HTTP client
func setupMCPTest(t *testing.T, handler http.HandlerFunc) (*MCPClient, *httptest.Server) {
	_ = logging.InitLogger()

	server := httptest.NewServer(handler)

	cfg := &config.Config{
		MCPServerURL: server.URL,
		MCPAuthToken: "test-token",
	}

	client := NewMCPClient(cfg, logging.Logger)

	return client, server
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 500*time.Millisecond, config.BaseDelay)
	assert.Equal(t, 10*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffFactor)
}

func TestNewMCPClient(t *testing.T) {
	_ = logging.InitLogger()

	cfg := &config.Config{
		MCPServerURL: "https://test.example.com",
		MCPAuthToken: "test-token",
	}

	client := NewMCPClient(cfg, logging.Logger)

	require.NotNil(t, client)
	assert.Equal(t, "https://test.example.com", client.baseURL)
	assert.Equal(t, "test-token", client.authToken)
	assert.NotNil(t, client.client)
	assert.NotNil(t, client.logger)
	assert.Equal(t, 30*time.Second, client.client.Timeout)
	assert.Equal(t, 3, client.retryConfig.MaxRetries)
}

func TestIsRetryableError(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger:      logging.Logger,
		retryConfig: DefaultRetryConfig(),
	}

	tests := []struct {
		name        string
		error       error
		shouldRetry bool
	}{
		{
			name:        "timeout error",
			error:       fmt.Errorf("request timeout"),
			shouldRetry: true,
		},
		{
			name:        "connection error",
			error:       fmt.Errorf("connection refused"),
			shouldRetry: true,
		},
		{
			name:        "network error",
			error:       fmt.Errorf("network unreachable"),
			shouldRetry: true,
		},
		{
			name:        "dial error",
			error:       fmt.Errorf("dial tcp error"),
			shouldRetry: true,
		},
		{
			name:        "500 internal server error",
			error:       fmt.Errorf("server error: 500"),
			shouldRetry: true,
		},
		{
			name:        "502 bad gateway",
			error:       fmt.Errorf("server error: 502"),
			shouldRetry: true,
		},
		{
			name:        "503 service unavailable",
			error:       fmt.Errorf("server error: 503"),
			shouldRetry: true,
		},
		{
			name:        "504 gateway timeout",
			error:       fmt.Errorf("server error: 504"),
			shouldRetry: true,
		},
		{
			name:        "session error",
			error:       fmt.Errorf("session expired"),
			shouldRetry: true,
		},
		{
			name:        "non-retryable error",
			error:       fmt.Errorf("invalid request format"),
			shouldRetry: false,
		},
		{
			name:        "400 bad request",
			error:       fmt.Errorf("bad request: 400"),
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.isRetryableError(tt.error)
			assert.Equal(t, tt.shouldRetry, result)
		})
	}
}

func TestWithRetry_Success(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger:      logging.Logger,
		retryConfig: DefaultRetryConfig(),
	}

	ctx := context.Background()
	callCount := 0

	err := client.withRetry(ctx, "test_operation", func() error {
		callCount++
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount, "Should succeed on first attempt")
}

func TestWithRetry_SuccessAfterRetries(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
		retryConfig: RetryConfig{
			MaxRetries:    3,
			BaseDelay:     10 * time.Millisecond, // Short delay for testing
			MaxDelay:      100 * time.Millisecond,
			BackoffFactor: 2.0,
		},
	}

	ctx := context.Background()
	callCount := 0

	err := client.withRetry(ctx, "test_operation", func() error {
		callCount++
		if callCount < 3 {
			return fmt.Errorf("temporary timeout error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount, "Should succeed on third attempt")
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
		retryConfig: RetryConfig{
			MaxRetries:    3,
			BaseDelay:     10 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			BackoffFactor: 2.0,
		},
	}

	ctx := context.Background()
	callCount := 0

	err := client.withRetry(ctx, "test_operation", func() error {
		callCount++
		return fmt.Errorf("bad request: 400")
	})

	assert.Error(t, err)
	assert.Equal(t, 1, callCount, "Should not retry non-retryable errors")
	assert.Contains(t, err.Error(), "bad request")
}

func TestWithRetry_ExhaustedRetries(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
		retryConfig: RetryConfig{
			MaxRetries:    2,
			BaseDelay:     10 * time.Millisecond,
			MaxDelay:      100 * time.Millisecond,
			BackoffFactor: 2.0,
		},
	}

	ctx := context.Background()
	callCount := 0

	err := client.withRetry(ctx, "test_operation", func() error {
		callCount++
		return fmt.Errorf("timeout error")
	})

	assert.Error(t, err)
	assert.Equal(t, 3, callCount, "Should try initial + 2 retries")
	assert.Contains(t, err.Error(), "failed after 3 attempts")
}

func TestWithRetry_ContextCancellation(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
		retryConfig: RetryConfig{
			MaxRetries:    5,
			BaseDelay:     100 * time.Millisecond,
			MaxDelay:      1 * time.Second,
			BackoffFactor: 2.0,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	// Cancel context after first failed attempt
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := client.withRetry(ctx, "test_operation", func() error {
		callCount++
		return fmt.Errorf("timeout error")
	})

	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestGetSessionID_Success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "HEAD", r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		w.Header().Set("mcp-session-id", "test-session-123")
		w.WriteHeader(http.StatusMethodNotAllowed) // Expected for HEAD request
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	sessionID, err := client.getSessionID(ctx)

	assert.NoError(t, err)
	assert.Equal(t, "test-session-123", sessionID)
}

func TestGetSessionID_WithWhitespace(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("mcp-session-id", "  test-session-456  ")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	sessionID, err := client.getSessionID(ctx)

	assert.NoError(t, err)
	assert.Equal(t, "test-session-456", sessionID)
}

func TestGetSessionID_NoSessionID(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	sessionID, err := client.getSessionID(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no session ID received")
	assert.Empty(t, sessionID)
}

func TestGetSessionID_UnexpectedStatusCode(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	// Override retry config to fail faster
	client.retryConfig.MaxRetries = 1
	client.retryConfig.BaseDelay = 10 * time.Millisecond

	ctx := context.Background()
	sessionID, err := client.getSessionID(ctx)

	assert.Error(t, err)
	assert.Empty(t, sessionID)
}

func TestMakeRequest_Success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-session", r.Header.Get("mcp-session-id"))

		response := map[string]interface{}{
			"result": map[string]interface{}{
				"status": "success",
				"data":   "test data",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	payload := MCPRequest{
		JSONRPC: "2.0",
		ID:      intPtr(1),
		Method:  "test_method",
		Params:  map[string]interface{}{"key": "value"},
	}

	result, err := client.makeRequest(ctx, "test-session", payload)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "success", result["result"].(map[string]interface{})["status"])
}

func TestMakeRequest_SSEFormat(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		response := `event: message
data: {"result":{"status":"success","data":"sse data"}}

`
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	payload := MCPRequest{
		JSONRPC: "2.0",
		ID:      intPtr(1),
		Method:  "test_method",
	}

	result, err := client.makeRequest(ctx, "test-session", payload)

	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "success", result["result"].(map[string]interface{})["status"])
}

func TestMakeRequest_ServerError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	// Override retry config to fail faster
	client.retryConfig.MaxRetries = 1
	client.retryConfig.BaseDelay = 10 * time.Millisecond

	ctx := context.Background()
	payload := MCPRequest{
		JSONRPC: "2.0",
		Method:  "test_method",
	}

	result, err := client.makeRequest(ctx, "test-session", payload)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestSendNotification_Success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "test-session", r.Header.Get("mcp-session-id"))

		w.WriteHeader(http.StatusAccepted)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	payload := MCPRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	err := client.sendNotification(ctx, "test-session", payload)

	assert.NoError(t, err)
}

func TestSendNotification_WithOKStatus(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	payload := MCPRequest{
		JSONRPC: "2.0",
		Method:  "notifications/test",
	}

	err := client.sendNotification(ctx, "test-session", payload)

	assert.NoError(t, err)
}

func TestSendNotification_FailedStatus(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	// Override retry config to fail faster
	client.retryConfig.MaxRetries = 0

	ctx := context.Background()
	payload := MCPRequest{
		JSONRPC: "2.0",
		Method:  "notifications/test",
	}

	err := client.sendNotification(ctx, "test-session", payload)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "notification failed")
}

func TestInitializeSession_Success(t *testing.T) {
	callCount := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if callCount == 1 {
			// Initialize request
			response := map[string]interface{}{
				"result": map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]interface{}{},
				},
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(response)
		} else if callCount == 2 {
			// Notification request
			w.WriteHeader(http.StatusAccepted)
		}
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	err := client.initializeSession(ctx, "test-session")

	assert.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestInitializeSession_InitializeError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"code":    -32600,
				"message": "Invalid request",
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	err := client.initializeSession(ctx, "test-session")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initialization error")
}

func TestLoadEquipmentInstructions_Success(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"result": map[string]interface{}{
				"instructions": "Equipment instructions loaded",
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	err := client.loadEquipmentInstructions(ctx, "test-session")

	assert.NoError(t, err)
}

func TestLoadEquipmentInstructions_Error(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"code":    -32601,
				"message": "Method not found",
			},
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	err := client.loadEquipmentInstructions(ctx, "test-session")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "equipment instructions error")
}

func TestParseProfessionalsFromText(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	tests := []struct {
		name                string
		input               string
		expectedMedicos     []string
		expectedEnfermeiros []string
	}{
		{
			name: "standard format",
			input: `MEDICOS:
Dr. João Silva
Dr. Maria Santos

ENFERMEIROS:
Enf. Ana Costa
Enf. Pedro Oliveira`,
			expectedMedicos:     []string{"Dr. João Silva", "Dr. Maria Santos"},
			expectedEnfermeiros: []string{"Enf. Ana Costa", "Enf. Pedro Oliveira"},
		},
		{
			name: "with extra whitespace",
			input: `MEDICOS:

  Dr. João Silva

ENFERMEIROS:

  Enf. Ana Costa
`,
			expectedMedicos:     []string{"Dr. João Silva"},
			expectedEnfermeiros: []string{"Enf. Ana Costa"},
		},
		{
			name:                "only medicos",
			input:               "MEDICOS:\nDr. João Silva",
			expectedMedicos:     []string{"Dr. João Silva"},
			expectedEnfermeiros: nil,
		},
		{
			name:                "only enfermeiros",
			input:               "ENFERMEIROS:\nEnf. Ana Costa",
			expectedMedicos:     nil,
			expectedEnfermeiros: []string{"Enf. Ana Costa"},
		},
		{
			name:                "empty text",
			input:               "",
			expectedMedicos:     nil,
			expectedEnfermeiros: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			medicos, enfermeiros := client.parseProfessionalsFromText(tt.input)
			assert.Equal(t, tt.expectedMedicos, medicos)
			assert.Equal(t, tt.expectedEnfermeiros, enfermeiros)
		})
	}
}

func TestParseEquipeSaudeData(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	tests := []struct {
		name     string
		input    map[string]interface{}
		validate func(t *testing.T, result *models.EquipeSaudeInfo, err error)
	}{
		{
			name: "complete data",
			input: map[string]interface{}{
				"nome_oficial":        "Equipe 001",
				"nome_popular":        "MEDICOS:\nDr. João\n\nENFERMEIROS:\nEnf. Maria",
				"regiao_planejamento": "1.0",
				"ativo":               true,
				"aberto_ao_publico":   true,
				"contato": map[string]interface{}{
					"telefones": []interface{}{"21-1234-5678", "21-9876-5432"},
					"email":     "equipe@saude.rio",
				},
				"updated_at": "2024-01-01T10:00:00Z",
			},
			validate: func(t *testing.T, result *models.EquipeSaudeInfo, err error) {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "Equipe 001", result.NomeOficial)
				assert.Equal(t, "MEDICOS:\nDr. João\n\nENFERMEIROS:\nEnf. Maria", result.NomePopular)
				assert.Equal(t, []string{"Dr. João"}, result.Medicos)
				assert.Equal(t, []string{"Enf. Maria"}, result.Enfermeiros)
				assert.Equal(t, "1.0", result.RegiaoPlaneamento)
				assert.True(t, result.Ativo)
				assert.True(t, result.AbertoAoPublico)
				assert.Equal(t, []string{"21-1234-5678", "21-9876-5432"}, result.Contato.Telefones)
				assert.Equal(t, "equipe@saude.rio", result.Contato.Email)
				assert.Equal(t, 2024, result.UpdatedAt.Year())
			},
		},
		{
			name: "minimal data",
			input: map[string]interface{}{
				"nome_oficial": "Equipe 002",
			},
			validate: func(t *testing.T, result *models.EquipeSaudeInfo, err error) {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, "Equipe 002", result.NomeOficial)
				assert.Empty(t, result.Medicos)
				assert.Empty(t, result.Enfermeiros)
			},
		},
		{
			name: "invalid updated_at",
			input: map[string]interface{}{
				"nome_oficial": "Equipe 003",
				"updated_at":   "invalid-date",
			},
			validate: func(t *testing.T, result *models.EquipeSaudeInfo, err error) {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.True(t, result.UpdatedAt.IsZero())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.parseEquipeSaudeData(tt.input)
			tt.validate(t, result, err)
		})
	}
}

func TestParseHealthServicesResponse_Success(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	idEquip := "cf-123"
	complemento := "1º andar"
	response := map[string]interface{}{
		"result": map[string]interface{}{
			"structuredContent": map[string]interface{}{
				"equipamentos": []interface{}{
					map[string]interface{}{
						"categoria":             "CF",
						"id_equipamento":        &idEquip,
						"nome_oficial":          "CF Centro",
						"nome_popular":          "Clinica Centro",
						"logradouro":            "Rua Principal",
						"numero":                "100",
						"complemento":           &complemento,
						"bairro":                "Centro",
						"regiao_administrativa": "I RA",
						"ativo":                 true,
						"aberto_ao_publico":     true,
					},
					map[string]interface{}{
						"categoria":           "EQUIPE DA FAMILIA",
						"nome_oficial":        "Equipe 001",
						"nome_popular":        "MEDICOS:\nDr. João\n\nENFERMEIROS:\nEnf. Maria",
						"regiao_planejamento": "1.0",
						"ativo":               true,
						"aberto_ao_publico":   true,
					},
				},
			},
		},
	}

	result, err := client.parseHealthServicesResponse(response)

	assert.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.HealthFacility)
	assert.Equal(t, "CF Centro", result.HealthFacility.NomeOficial)
	assert.Equal(t, "Clinica Centro", result.HealthFacility.NomePopular)
	assert.Equal(t, "Centro", result.HealthFacility.Bairro)

	require.NotNil(t, result.FamilyHealthTeam)
	assert.Equal(t, "Equipe 001", result.FamilyHealthTeam.NomeOficial)
	assert.Equal(t, []string{"Dr. João"}, result.FamilyHealthTeam.Medicos)
	assert.Equal(t, []string{"Enf. Maria"}, result.FamilyHealthTeam.Enfermeiros)
}

func TestParseHealthServicesResponse_IsError(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	response := map[string]interface{}{
		"result": map[string]interface{}{
			"isError":     true,
			"textContent": "Address not found",
		},
	}

	result, err := client.parseHealthServicesResponse(response)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseHealthServicesResponse_NoEquipment(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	response := map[string]interface{}{
		"result": map[string]interface{}{
			"structuredContent": map[string]interface{}{
				"equipamentos": []interface{}{},
			},
		},
	}

	result, err := client.parseHealthServicesResponse(response)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no equipment found")
}

func TestParseHealthServicesResponse_InvalidFormat(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	tests := []struct {
		name     string
		response map[string]interface{}
		errorMsg string
	}{
		{
			name:     "missing result",
			response: map[string]interface{}{},
			errorMsg: "missing result",
		},
		{
			name: "missing structured content",
			response: map[string]interface{}{
				"result": map[string]interface{}{},
			},
			errorMsg: "no structured content",
		},
		{
			name: "invalid equipamentos type",
			response: map[string]interface{}{
				"result": map[string]interface{}{
					"structuredContent": map[string]interface{}{
						"equipamentos": "not an array",
					},
				},
			},
			errorMsg: "no equipment found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.parseHealthServicesResponse(tt.response)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

func TestParseHealthServicesResponse_EquipmentWithError(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	response := map[string]interface{}{
		"result": map[string]interface{}{
			"structuredContent": map[string]interface{}{
				"equipamentos": []interface{}{
					map[string]interface{}{
						"error": "Equipment not found",
					},
				},
			},
		},
	}

	result, err := client.parseHealthServicesResponse(response)

	// Equipment with error is skipped, returns empty result
	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.HealthFacility)
	assert.Nil(t, result.FamilyHealthTeam)
}

func TestFindNearestCF_Integration(t *testing.T) {
	requestCount := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		switch {
		case r.Method == "HEAD":
			// Session ID request
			w.Header().Set("mcp-session-id", "test-session-123")
			w.WriteHeader(http.StatusMethodNotAllowed)

		case r.Method == "POST":
			// Read the request body to determine which request this is
			var req MCPRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			if req.Method == "initialize" {
				// Initialize session
				response := map[string]interface{}{
					"result": map[string]interface{}{
						"protocolVersion": "2024-11-05",
					},
				}
				_ = json.NewEncoder(w).Encode(response)

			} else if req.Method == "notifications/initialized" {
				// Notification
				w.WriteHeader(http.StatusAccepted)

			} else if req.Method == "tools/call" {
				params := req.Params.(map[string]interface{})
				toolName := params["name"].(string)

				if toolName == "equipments_instructions" {
					// Equipment instructions
					response := map[string]interface{}{
						"result": map[string]interface{}{
							"content": "Instructions loaded",
						},
					}
					_ = json.NewEncoder(w).Encode(response)

				} else if toolName == "equipments_by_address" {
					// CF lookup
					idEquip := "cf-123"
					response := map[string]interface{}{
						"result": map[string]interface{}{
							"structuredContent": map[string]interface{}{
								"equipamentos": []interface{}{
									map[string]interface{}{
										"categoria":             "CF",
										"id_equipamento":        &idEquip,
										"nome_oficial":          "CF Test",
										"nome_popular":          "Clinica Test",
										"logradouro":            "Rua Test",
										"numero":                "100",
										"bairro":                "Centro",
										"regiao_administrativa": "I RA",
										"ativo":                 true,
										"aberto_ao_publico":     true,
									},
								},
							},
						},
					}
					_ = json.NewEncoder(w).Encode(response)
				}
			}
		}
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	ctx := context.Background()
	result, err := client.FindNearestCF(ctx, "Rua Test, 100, Centro")

	assert.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.HealthFacility)
	assert.Equal(t, "CF Test", result.HealthFacility.NomeOficial)
	assert.True(t, requestCount >= 4, "Should make at least 4 requests: HEAD, initialize, notification, instructions, CF lookup")
}

func TestFindNearestCF_SessionError(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	// Override retry config to fail faster
	client.retryConfig.MaxRetries = 1
	client.retryConfig.BaseDelay = 10 * time.Millisecond

	ctx := context.Background()
	result, err := client.FindNearestCF(ctx, "Rua Test, 100")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get session ID")
}

func TestIntPtr(t *testing.T) {
	value := 42
	ptr := intPtr(value)

	require.NotNil(t, ptr)
	assert.Equal(t, value, *ptr)
}

func TestMCPRequest_JSONMarshaling(t *testing.T) {
	id := 1
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "test_method",
		Params: map[string]interface{}{
			"key": "value",
		},
	}

	data, err := json.Marshal(req)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"jsonrpc":"2.0"`)
	assert.Contains(t, string(data), `"id":1`)
	assert.Contains(t, string(data), `"method":"test_method"`)
}

func TestMCPResponse_JSONUnmarshaling(t *testing.T) {
	jsonData := `{
		"jsonrpc": "2.0",
		"id": 1,
		"result": {
			"status": "success"
		}
	}`

	var resp MCPResponse
	err := json.Unmarshal([]byte(jsonData), &resp)

	assert.NoError(t, err)
	assert.Equal(t, "2.0", resp.JSONRPC)
	require.NotNil(t, resp.ID)
	assert.Equal(t, 1, *resp.ID)
	require.NotNil(t, resp.Result)
	assert.Equal(t, "success", resp.Result["status"])
	assert.Nil(t, resp.Error)
}

func TestMCPError_JSONUnmarshaling(t *testing.T) {
	jsonData := `{
		"jsonrpc": "2.0",
		"id": 1,
		"error": {
			"code": -32600,
			"message": "Invalid Request",
			"data": "additional info"
		}
	}`

	var resp MCPResponse
	err := json.Unmarshal([]byte(jsonData), &resp)

	assert.NoError(t, err)
	assert.Equal(t, "2.0", resp.JSONRPC)
	require.NotNil(t, resp.Error)
	assert.Equal(t, -32600, resp.Error.Code)
	assert.Equal(t, "Invalid Request", resp.Error.Message)
	assert.Equal(t, "additional info", resp.Error.Data)
	assert.Nil(t, resp.Result)
}

func TestParseHealthServicesResponse_CMSCategory(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	idEquip := "cms-456"
	response := map[string]interface{}{
		"result": map[string]interface{}{
			"structuredContent": map[string]interface{}{
				"equipamentos": []interface{}{
					map[string]interface{}{
						"categoria":             "CMS",
						"id_equipamento":        &idEquip,
						"nome_oficial":          "CMS Test",
						"nome_popular":          "Centro Municipal Test",
						"logradouro":            "Av Test",
						"numero":                "200",
						"bairro":                "Zona Sul",
						"regiao_administrativa": "II RA",
						"ativo":                 true,
						"aberto_ao_publico":     true,
					},
				},
			},
		},
	}

	result, err := client.parseHealthServicesResponse(response)

	assert.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.HealthFacility)
	assert.Equal(t, "CMS Test", result.HealthFacility.NomeOficial)
}

func TestWithRetry_BackoffCalculation(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
		retryConfig: RetryConfig{
			MaxRetries:    3,
			BaseDelay:     100 * time.Millisecond,
			MaxDelay:      500 * time.Millisecond,
			BackoffFactor: 2.0,
		},
	}

	ctx := context.Background()
	callCount := 0
	startTime := time.Now()

	err := client.withRetry(ctx, "test_operation", func() error {
		callCount++
		if callCount <= 3 {
			return fmt.Errorf("timeout error")
		}
		return nil
	})

	elapsed := time.Since(startTime)

	assert.NoError(t, err)
	assert.Equal(t, 4, callCount)
	// First attempt: 0ms
	// Second attempt: 100ms delay
	// Third attempt: 200ms delay
	// Fourth attempt: 400ms delay (capped at 500ms but not reached)
	// Total minimum: 700ms
	assert.True(t, elapsed >= 700*time.Millisecond, "Backoff delays should be applied")
}

func TestMakeRequest_InvalidJSON(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("invalid json {"))
	}

	client, server := setupMCPTest(t, handler)
	defer server.Close()

	// Override retry config to fail faster
	client.retryConfig.MaxRetries = 0

	ctx := context.Background()
	payload := MCPRequest{
		JSONRPC: "2.0",
		Method:  "test",
	}

	result, err := client.makeRequest(ctx, "test-session", payload)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to unmarshal")
}

func TestParseHealthServicesResponse_UnknownCategory(t *testing.T) {
	_ = logging.InitLogger()
	client := &MCPClient{
		logger: logging.Logger,
	}

	response := map[string]interface{}{
		"result": map[string]interface{}{
			"structuredContent": map[string]interface{}{
				"equipamentos": []interface{}{
					map[string]interface{}{
						"categoria":    "UNKNOWN_TYPE",
						"nome_oficial": "Unknown Equipment",
					},
				},
			},
		},
	}

	result, err := client.parseHealthServicesResponse(response)

	// Should not fail, but returns empty result (no recognized equipment)
	// The implementation skips unknown categories but still returns an empty HealthServicesResult
	assert.NoError(t, err)
	require.NotNil(t, result)
	assert.Nil(t, result.HealthFacility)
	assert.Nil(t, result.FamilyHealthTeam)
}
