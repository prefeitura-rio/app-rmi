package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.uber.org/zap"
)

// MCPClient handles communication with the Rio de Janeiro MCP Server for CF lookups
type MCPClient struct {
	baseURL     string
	authToken   string
	client      *http.Client
	logger      *logging.SafeLogger
	retryConfig RetryConfig
}

// MCPRequest represents a JSON-RPC 2.0 request to the MCP server
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int        `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse represents a JSON-RPC 2.0 response from the MCP server
type MCPResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      *int                   `json:"id,omitempty"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *MCPError              `json:"error,omitempty"`
}

// MCPError represents an error in the MCP response
type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCPToolError is a JSON-RPC error returned by an MCP tool call.
type MCPToolError struct {
	Tool    string
	Code    int
	Message string
}

func (e *MCPToolError) Error() string {
	return fmt.Sprintf("MCP tool %s error (code %d): %s", e.Tool, e.Code, e.Message)
}

func newMCPToolError(tool string, errorObj interface{}) *MCPToolError {
	toolErr := &MCPToolError{Tool: tool}
	fields, ok := errorObj.(map[string]interface{})
	if !ok {
		toolErr.Message = fmt.Sprintf("%v", errorObj)
		return toolErr
	}
	if code, ok := fields["code"].(float64); ok {
		toolErr.Code = int(code)
	}
	if message, ok := fields["message"].(string); ok {
		toolErr.Message = message
	}
	return toolErr
}

// MCPHTTPStatusError is an HTTP status from the MCP server that the client handles by code:
// 5xx, 401, 403, 404 (unknown session), 408 and 429.
type MCPHTTPStatusError struct {
	StatusCode int
}

func (e *MCPHTTPStatusError) Error() string {
	if e.StatusCode >= 500 {
		return fmt.Sprintf("server error: %d", e.StatusCode)
	}
	return fmt.Sprintf("MCP server returned status %d", e.StatusCode)
}

func isHandledMCPStatus(code int) bool {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
		http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	default:
		return code >= 500
	}
}

// MCPProtocolError is a response body that is not the expected JSON-RPC message.
type MCPProtocolError struct {
	Err error
}

func (e *MCPProtocolError) Error() string {
	return e.Err.Error()
}

func (e *MCPProtocolError) Unwrap() error {
	return e.Err
}

// RetryConfig defines retry behavior for MCP requests
type RetryConfig struct {
	MaxRetries    int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	BackoffFactor float64
}

// DefaultRetryConfig returns sensible defaults for MCP retries
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		BaseDelay:     500 * time.Millisecond,
		MaxDelay:      10 * time.Second,
		BackoffFactor: 2.0,
	}
}

// NewMCPClient creates a new MCP client instance
func NewMCPClient(cfg *config.Config, logger *logging.SafeLogger) *MCPClient {
	return &MCPClient{
		baseURL:   cfg.MCPServerURL,
		authToken: cfg.MCPAuthToken,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger:      logger,
		retryConfig: DefaultRetryConfig(),
	}
}

// withRetry executes a function with exponential backoff retry logic
func (c *MCPClient) withRetry(ctx context.Context, operation string, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay with exponential backoff
			delay := time.Duration(float64(c.retryConfig.BaseDelay) * math.Pow(c.retryConfig.BackoffFactor, float64(attempt-1)))
			if delay > c.retryConfig.MaxDelay {
				delay = c.retryConfig.MaxDelay
			}

			c.logger.Debug("retrying MCP operation",
				zap.String("operation", operation),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				// Continue with retry
			}
		}

		lastErr = fn()
		if lastErr == nil {
			if attempt > 0 {
				c.logger.Info("MCP operation succeeded after retry",
					zap.String("operation", operation),
					zap.Int("attempts", attempt+1))
			}
			return nil
		}

		// Check if error is retryable
		if !c.isRetryableError(lastErr) {
			c.logger.Debug("non-retryable error, aborting",
				zap.String("operation", operation),
				zap.Error(lastErr))
			return lastErr
		}

		c.logger.Warn("MCP operation failed, will retry",
			zap.String("operation", operation),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", c.retryConfig.MaxRetries),
			zap.Error(lastErr))
	}

	c.logger.Error("MCP operation failed after all retries",
		zap.String("operation", operation),
		zap.Int("total_attempts", c.retryConfig.MaxRetries+1),
		zap.Error(lastErr))

	return fmt.Errorf("operation %s failed after %d attempts: %w", operation, c.retryConfig.MaxRetries+1, lastErr)
}

// isRetryableError determines if an error should trigger a retry
func (c *MCPClient) isRetryableError(err error) bool {
	var statusErr *MCPHTTPStatusError
	if errors.As(err, &statusErr) {
		code := statusErr.StatusCode
		return code >= 500 || code == http.StatusRequestTimeout || code == http.StatusTooManyRequests
	}

	// Network errors are generally retryable
	if strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "network") ||
		strings.Contains(err.Error(), "dial") {
		return true
	}

	// HTTP status codes that indicate temporary issues
	if strings.Contains(err.Error(), "500") || // Internal Server Error
		strings.Contains(err.Error(), "502") || // Bad Gateway
		strings.Contains(err.Error(), "503") || // Service Unavailable
		strings.Contains(err.Error(), "504") { // Gateway Timeout
		return true
	}

	// MCP-specific errors that might be temporary
	if strings.Contains(err.Error(), "session") {
		return true
	}

	return false
}

// mcpPayloadMethod returns the JSON-RPC method of a payload, so debug logs can name the
// request without its params (tools/call carries the citizen's address).
func mcpPayloadMethod(payload interface{}) string {
	switch p := payload.(type) {
	case MCPRequest:
		return p.Method
	case *MCPRequest:
		if p != nil {
			return p.Method
		}
	}
	return "unknown"
}

// mcpSessionHeader is the Streamable HTTP header that carries the MCP session ID.
const mcpSessionHeader = "Mcp-Session-Id"

// setMCPHeaders sets the headers every MCP POST carries. The session header goes only when the
// server assigned a session: a stateless server assigns none, and a new session (initialize) has none yet.
func (c *MCPClient) setMCPHeaders(req *http.Request, sessionID string) {
	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set(mcpSessionHeader, sessionID)
	}
}

// makeRequest sends a JSON-RPC request to the MCP server
func (c *MCPClient) makeRequest(ctx context.Context, sessionID string, payload interface{}) (map[string]interface{}, error) {
	result, _, err := c.doRequest(ctx, sessionID, payload)
	return result, err
}

// doRequest sends a JSON-RPC request to the MCP server and returns the decoded body and the
// response headers (initialize reads the session ID from them).
func (c *MCPClient) doRequest(ctx context.Context, sessionID string, payload interface{}) (map[string]interface{}, http.Header, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var result map[string]interface{}
	var header http.Header

	err = c.withRetry(ctx, "make_mcp_request", func() error {
		req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		c.setMCPHeaders(req, sessionID)

		c.logger.Debug("sending MCP request",
			zap.String("session_id", sessionID),
			zap.String("method", mcpPayloadMethod(payload)),
			zap.Int("payload_bytes", len(jsonData)))

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if isHandledMCPStatus(resp.StatusCode) {
			return &MCPHTTPStatusError{StatusCode: resp.StatusCode}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		// Handle Server-Sent Events format
		bodyStr := string(body)
		c.logger.Debug("raw MCP response",
			zap.Int("status", resp.StatusCode),
			zap.Int("body_bytes", len(body)))

		if strings.Contains(bodyStr, "event: message") {
			// Parse SSE format more robustly
			lines := strings.Split(bodyStr, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "data: ") {
					bodyStr = strings.TrimPrefix(line, "data: ")
					break
				}
			}
		}

		result = nil
		err = json.Unmarshal([]byte(bodyStr), &result)
		if err != nil {
			return &MCPProtocolError{Err: fmt.Errorf("failed to unmarshal response: %w", err)}
		}
		header = resp.Header

		_, hasError := result["error"]
		c.logger.Debug("received MCP response", zap.Bool("has_error", hasError))

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	return result, header, nil
}

// sendNotification sends a notification request (expects no response body)
func (c *MCPClient) sendNotification(ctx context.Context, sessionID string, payload interface{}) error {
	err := c.withRetry(ctx, "send_notification", func() error {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		c.setMCPHeaders(req, sessionID)

		c.logger.Debug("sending MCP notification",
			zap.String("session_id", sessionID),
			zap.String("method", mcpPayloadMethod(payload)),
			zap.Int("payload_bytes", len(jsonData)))

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send notification: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		// Notifications should return 202 (Accepted) with empty body
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("notification failed with status: %d", resp.StatusCode)
		}

		c.logger.Debug("notification sent successfully", zap.Int("status", resp.StatusCode))

		return nil
	})

	return err
}

// initializeSession opens an MCP session as the Streamable HTTP transport defines it: a POST with
// initialize and no session header, then notifications/initialized. A server that keeps sessions
// returns the session ID in the Mcp-Session-Id header of the initialize response, and the later
// requests carry it; a stateless server returns none, and the later requests go without it.
func (c *MCPClient) initializeSession(ctx context.Context) (string, error) {
	ctx, span := utils.TraceBusinessLogic(ctx, "mcp_initialize_session")
	defer span.End()

	// Step 1: Initialize session
	initPayload := MCPRequest{
		JSONRPC: "2.0",
		ID:      intPtr(1),
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"clientInfo":      map[string]interface{}{"name": "cf-lookup-api", "version": "1.0"},
		},
	}

	result, header, err := c.doRequest(ctx, "", initPayload)
	if err != nil {
		return "", fmt.Errorf("failed to initialize session: %w", err)
	}

	// Check for errors in response
	if errorObj, exists := result["error"]; exists {
		return "", fmt.Errorf("MCP initialization error: %v", errorObj)
	}

	sessionID := strings.TrimSpace(header.Get(mcpSessionHeader))

	// Step 2: Send initialized notification
	notifyPayload := MCPRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	err = c.sendNotification(ctx, sessionID, notifyPayload)
	if err != nil {
		return "", fmt.Errorf("failed to send initialized notification: %w", err)
	}

	c.logger.Debug("MCP session initialized successfully",
		zap.String("session_id", sessionID),
		zap.Bool("stateless", sessionID == ""))

	return sessionID, nil
}

// loadEquipmentInstructions loads the equipment instructions (required before CF lookup)
func (c *MCPClient) loadEquipmentInstructions(ctx context.Context, sessionID string) error {
	instructionsPayload := MCPRequest{
		JSONRPC: "2.0",
		ID:      intPtr(2),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "equipments_instructions",
			"arguments": map[string]interface{}{},
		},
	}

	result, err := c.makeRequest(ctx, sessionID, instructionsPayload)
	if err != nil {
		return fmt.Errorf("failed to load equipment instructions: %w", err)
	}

	// Check for errors in response
	if errorObj, exists := result["error"]; exists {
		return fmt.Errorf("MCP equipment instructions error: %v", errorObj)
	}

	c.logger.Debug("equipment instructions loaded successfully")

	return nil
}

// FindNearestCF finds the nearest Clínica da Família and Family Health Team for a given address
func (c *MCPClient) FindNearestCF(ctx context.Context, address string) (*models.HealthServicesResult, error) {
	startTime := time.Now()
	ctx, span := utils.TraceBusinessLogic(ctx, "mcp_find_nearest_cf")
	defer span.End()

	c.logger.Info("starting CF lookup via MCP",
		zap.String("operation", "mcp_cf_lookup_start"))

	defer func() {
		c.logger.Info("CF lookup via MCP completed",
			zap.Duration("total_duration", time.Since(startTime)),
			zap.String("operation", "mcp_cf_lookup_complete"))
	}()

	// Step 1: Open the session (initialize + notifications/initialized)
	initStart := time.Now()
	sessionID, err := c.initializeSession(ctx)
	if err != nil {
		c.logger.Error("session initialization failed",
			zap.Error(err),
			zap.String("operation", "initialize_session"),
			zap.Duration("duration", time.Since(initStart)))
		return nil, fmt.Errorf("failed to initialize session: %w", err)
	}
	c.logger.Debug("session initialized",
		zap.String("session_id", sessionID),
		zap.Duration("duration", time.Since(initStart)))

	// Step 2: Load equipment instructions
	err = c.loadEquipmentInstructions(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load equipment instructions: %w", err)
	}

	// Step 3: Find nearest CF
	c.logger.Info("MCP CF LOOKUP REQUEST", zap.String("tool", "equipments_by_address"))
	cfPayload := MCPRequest{
		JSONRPC: "2.0",
		ID:      intPtr(3),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "equipments_by_address",
			"arguments": map[string]interface{}{
				"address":    address,
				"categories": []string{"CF", "CMS"},
			},
		},
	}

	result, err := c.makeRequest(ctx, sessionID, cfPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to find CF: %w", err)
	}

	// A JSON-RPC error has no "result"; report it as a tool error instead of a malformed response.
	if errorObj, exists := result["error"]; exists && errorObj != nil {
		return nil, newMCPToolError("equipments_by_address", errorObj)
	}

	// Parse response
	return c.parseHealthServicesResponse(result)
}

// parseHealthServicesResponse parses the MCP response and extracts both CF and Family Health Team information
func (c *MCPClient) parseHealthServicesResponse(result map[string]interface{}) (*models.HealthServicesResult, error) {
	resultData, ok := result["result"].(map[string]interface{})
	if !ok {
		return nil, &MCPProtocolError{Err: fmt.Errorf("invalid response format: missing result")}
	}

	// Check for error flag
	if isError, exists := resultData["isError"]; exists && isError == true {
		// Try to extract error message from response
		errorMsg := "no health services found for this address"
		if textContent, exists := resultData["textContent"]; exists {
			if textStr, ok := textContent.(string); ok && textStr != "" {
				errorMsg = textStr
			}
		}

		c.logger.Debug("MCP server reported no health services available",
			zap.Int("reason_bytes", len(errorMsg)),
			zap.String("operation", "health_services_lookup_no_results"))

		// Return nil (no services found) instead of error - this is expected behavior
		return nil, nil
	}

	structuredContent, ok := resultData["structuredContent"].(map[string]interface{})
	if !ok {
		return nil, &MCPProtocolError{Err: fmt.Errorf("no structured content in response")}
	}

	equipamentos, ok := structuredContent["equipamentos"].([]interface{})
	if !ok || len(equipamentos) == 0 {
		c.logger.Debug("no equipment found in response")
		return nil, nil
	}

	var healthResult models.HealthServicesResult

	// Process each equipment object
	for _, eq := range equipamentos {
		equipamento, ok := eq.(map[string]interface{})
		if !ok {
			continue
		}

		// Check for error in equipment data
		if errorMsg, exists := equipamento["error"]; exists {
			c.logger.Debug("MCP server reported no equipment found",
				zap.Int("reason_bytes", len(fmt.Sprintf("%v", errorMsg))),
				zap.String("operation", "equipment_lookup_no_equipment"))
			continue
		}

		// Determine the type of equipment based on categoria
		categoria, ok := equipamento["categoria"].(string)
		if !ok {
			continue
		}

		switch categoria {
		case "CF", "CMS":
			// Parse as health facility
			cfBytes, err := json.Marshal(equipamento)
			if err != nil {
				c.logger.Error("failed to marshal CF data", zap.Error(err))
				continue
			}

			var cfInfo models.CFInfo
			err = json.Unmarshal(cfBytes, &cfInfo)
			if err != nil {
				c.logger.Error("failed to unmarshal CF data", zap.Error(err))
				continue
			}

			healthResult.HealthFacility = &cfInfo

		case "EQUIPE DA FAMILIA":
			// Parse as family health team
			equipeSaudeData, err := c.parseEquipeSaudeData(equipamento)
			if err != nil {
				c.logger.Error("failed to parse equipe saude data", zap.Error(err))
				continue
			}

			healthResult.FamilyHealthTeam = equipeSaudeData
		}
	}

	// Log what we found
	if healthResult.HealthFacility != nil {
		c.logger.Info("Health facility found",
			zap.String("cf_name", healthResult.HealthFacility.NomePopular),
			zap.String("cf_bairro", healthResult.HealthFacility.Bairro))
	}

	if healthResult.FamilyHealthTeam != nil {
		c.logger.Info("Family health team found",
			zap.String("team_name", healthResult.FamilyHealthTeam.NomeOficial),
			zap.Int("doctors_count", len(healthResult.FamilyHealthTeam.Medicos)),
			zap.Int("nurses_count", len(healthResult.FamilyHealthTeam.Enfermeiros)))
	}

	return &healthResult, nil
}

// parseEquipeSaudeData parses family health team data from MCP response
func (c *MCPClient) parseEquipeSaudeData(equipamento map[string]interface{}) (*models.EquipeSaudeInfo, error) {
	var equipeSaude models.EquipeSaudeInfo

	// Extract basic information
	if nomeOficial, ok := equipamento["nome_oficial"].(string); ok {
		equipeSaude.NomeOficial = nomeOficial
	}

	if nomePopular, ok := equipamento["nome_popular"].(string); ok {
		equipeSaude.NomePopular = nomePopular

		// Parse doctors and nurses from nome_popular field
		// Format: "MEDICOS:\nName1\nName2\n\nENFERMEIROS:\nName3\nName4"
		medicos, enfermeiros := c.parseProfessionalsFromText(nomePopular)
		equipeSaude.Medicos = medicos
		equipeSaude.Enfermeiros = enfermeiros
	}

	if regiaoPlaneamento, ok := equipamento["regiao_planejamento"].(string); ok {
		equipeSaude.RegiaoPlaneamento = regiaoPlaneamento
	}

	if ativo, ok := equipamento["ativo"].(bool); ok {
		equipeSaude.Ativo = ativo
	}

	if abertoAoPublico, ok := equipamento["aberto_ao_publico"].(bool); ok {
		equipeSaude.AbertoAoPublico = abertoAoPublico
	}

	// Extract contact information
	if contato, ok := equipamento["contato"].(map[string]interface{}); ok {
		var contactInfo models.CFContactInfo

		if telefones, ok := contato["telefones"].([]interface{}); ok {
			for _, tel := range telefones {
				if telStr, ok := tel.(string); ok {
					contactInfo.Telefones = append(contactInfo.Telefones, telStr)
				}
			}
		}

		if email, ok := contato["email"].(string); ok {
			contactInfo.Email = email
		}

		equipeSaude.Contato = contactInfo
	}

	// Extract updated_at
	if updatedAtStr, ok := equipamento["updated_at"].(string); ok {
		if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
			equipeSaude.UpdatedAt = updatedAt
		}
	}

	return &equipeSaude, nil
}

// parseProfessionalsFromText extracts doctors and nurses from the nome_popular field
func (c *MCPClient) parseProfessionalsFromText(text string) ([]string, []string) {
	var medicos []string
	var enfermeiros []string

	lines := strings.Split(text, "\n")
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.Contains(line, "MEDICOS:") {
			currentSection = "medicos"
			continue
		}

		if strings.Contains(line, "ENFERMEIROS:") {
			currentSection = "enfermeiros"
			continue
		}

		// Add names to appropriate section
		if currentSection == "medicos" && line != "" {
			medicos = append(medicos, line)
		} else if currentSection == "enfermeiros" && line != "" {
			enfermeiros = append(enfermeiros, line)
		}
	}

	return medicos, enfermeiros
}

// intPtr returns a pointer to an integer
func intPtr(i int) *int {
	return &i
}
