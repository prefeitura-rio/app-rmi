package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupEmailHandlersTest(t *testing.T) (*gin.Engine, func()) {
	logging.InitLogger()
	gin.SetMode(gin.TestMode)

	router := gin.New()

	// Public endpoint - no authentication required
	router.POST("/validate/email", ValidateEmailAddress)

	return router, func() {
		// No cleanup needed for email validation (no DB/Redis connections)
	}
}

// Test handler initialization
func TestEmailHandlers_Initialization(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	assert.NotNil(t, router, "Router should not be nil")
}

// ==================== Success Cases ====================

func TestValidateEmailAddress_Success(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	reqBody := EmailValidationRequest{
		Email: "user@example.com",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status OK")

	var response EmailValidationResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.True(t, response.Valid, "Email should be valid")
	assert.Equal(t, "email válido", response.Message)
	assert.Equal(t, "user", response.LocalPart)
	assert.Equal(t, "example.com", response.Domain)
	assert.Equal(t, "user@example.com", response.Normalized)
	assert.Equal(t, "format_and_structure", response.ValidationType)
}

func TestValidateEmailAddress_ValidEmailFormats(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name               string
		email              string
		expectedLocal      string
		expectedDomain     string
		expectedNormalized string
	}{
		{
			name:               "simple email",
			email:              "user@example.com",
			expectedLocal:      "user",
			expectedDomain:     "example.com",
			expectedNormalized: "user@example.com",
		},
		{
			name:               "email with subdomain",
			email:              "user@mail.example.com",
			expectedLocal:      "user",
			expectedDomain:     "mail.example.com",
			expectedNormalized: "user@mail.example.com",
		},
		{
			name:               "email with dots in local part",
			email:              "first.last@example.com",
			expectedLocal:      "first.last",
			expectedDomain:     "example.com",
			expectedNormalized: "first.last@example.com",
		},
		{
			name:               "email with numbers",
			email:              "user123@example.com",
			expectedLocal:      "user123",
			expectedDomain:     "example.com",
			expectedNormalized: "user123@example.com",
		},
		{
			name:               "email with plus sign",
			email:              "user+tag@example.com",
			expectedLocal:      "user+tag",
			expectedDomain:     "example.com",
			expectedNormalized: "user+tag@example.com",
		},
		{
			name:               "email with underscore",
			email:              "user_name@example.com",
			expectedLocal:      "user_name",
			expectedDomain:     "example.com",
			expectedNormalized: "user_name@example.com",
		},
		{
			name:               "email with hyphen in local part",
			email:              "user-name@example.com",
			expectedLocal:      "user-name",
			expectedDomain:     "example.com",
			expectedNormalized: "user-name@example.com",
		},
		{
			name:               "email with hyphen in domain",
			email:              "user@ex-ample.com",
			expectedLocal:      "user",
			expectedDomain:     "ex-ample.com",
			expectedNormalized: "user@ex-ample.com",
		},
		{
			name:               "email with multiple subdomains",
			email:              "user@mail.corp.example.com",
			expectedLocal:      "user",
			expectedDomain:     "mail.corp.example.com",
			expectedNormalized: "user@mail.corp.example.com",
		},
		{
			name:               "email with numeric domain",
			email:              "user@123.456.com",
			expectedLocal:      "user",
			expectedDomain:     "123.456.com",
			expectedNormalized: "user@123.456.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := EmailValidationRequest{
				Email: tt.email,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Expected status OK for valid email")

			var response EmailValidationResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err, "Failed to unmarshal response")

			assert.True(t, response.Valid, "Email should be valid: %s", tt.email)
			assert.Equal(t, "email válido", response.Message)
			assert.Equal(t, tt.expectedLocal, response.LocalPart)
			assert.Equal(t, tt.expectedDomain, response.Domain)
			assert.Equal(t, tt.expectedNormalized, response.Normalized)
			assert.Equal(t, "format_and_structure", response.ValidationType)
			assert.NotEmpty(t, response.LocalPart, "LocalPart should not be empty")
			assert.NotEmpty(t, response.Domain, "Domain should not be empty")
		})
	}
}

// ==================== Email Normalization Tests ====================

func TestValidateEmailAddress_Normalization(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name       string
		email      string
		normalized string
	}{
		{
			name:       "uppercase email",
			email:      "USER@EXAMPLE.COM",
			normalized: "user@example.com",
		},
		{
			name:       "mixed case email",
			email:      "User@Example.Com",
			normalized: "user@example.com",
		},
		{
			name:       "email with leading spaces",
			email:      " user@example.com",
			normalized: "user@example.com",
		},
		{
			name:       "email with trailing spaces",
			email:      "user@example.com ",
			normalized: "user@example.com",
		},
		{
			name:       "email with both leading and trailing spaces",
			email:      " user@example.com ",
			normalized: "user@example.com",
		},
		{
			name:       "mixed case with spaces",
			email:      " User@Example.COM ",
			normalized: "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := EmailValidationRequest{
				Email: tt.email,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Expected status OK")

			var response EmailValidationResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.True(t, response.Valid, "Email should be valid")
			assert.Equal(t, tt.normalized, response.Normalized)
			assert.Equal(t, strings.ToLower(response.Normalized), response.Normalized, "Normalized should be lowercase")
		})
	}
}

// ==================== Invalid JSON / Request Body Tests ====================

func TestValidateEmailAddress_InvalidJSON(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Expected Bad Request for invalid JSON")

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "campo email é obrigatório", response.Error)
}

func TestValidateEmailAddress_EmptyRequestBody(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Expected Bad Request for empty request")

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "campo email é obrigatório", response.Error)
}

func TestValidateEmailAddress_MissingEmailField(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer([]byte(`{"other_field": "value"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Expected Bad Request for missing email field")
}

// ==================== Empty Email Tests ====================

func TestValidateEmailAddress_EmptyEmail(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	reqBody := EmailValidationRequest{
		Email: "",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Expected Bad Request for empty email")

	var response ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "campo email é obrigatório", response.Error)
}

func TestValidateEmailAddress_WhitespaceOnly(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	reqBody := EmailValidationRequest{
		Email: "   ",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "Expected Bad Request for whitespace only")

	var response EmailValidationResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.False(t, response.Valid)
	assert.Equal(t, "email não pode estar vazio", response.Message)
}

// ==================== Length Validation Tests ====================

func TestValidateEmailAddress_EmailTooLong(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	// Create an email longer than 254 characters
	longEmail := "user@" + strings.Repeat("a", 250) + ".com"

	reqBody := EmailValidationRequest{
		Email: longEmail,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response EmailValidationResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.False(t, response.Valid)
	assert.Equal(t, "email muito longo (máximo 254 caracteres)", response.Message)
}

func TestValidateEmailAddress_LocalPartTooLong(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	// Create local part longer than 64 characters
	longLocalPart := strings.Repeat("a", 65) + "@example.com"

	reqBody := EmailValidationRequest{
		Email: longLocalPart,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response EmailValidationResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.False(t, response.Valid)
	assert.Equal(t, "parte local do email inválida (deve ter 1-64 caracteres)", response.Message)
}

func TestValidateEmailAddress_DomainTooLong(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	// Create domain longer than 253 characters
	longDomain := "user@" + strings.Repeat("a", 250) + ".com"

	reqBody := EmailValidationRequest{
		Email: longDomain,
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response EmailValidationResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.False(t, response.Valid)
	// Could be either "email muito longo" or "domínio do email inválido"
	assert.Contains(t, []string{
		"email muito longo (máximo 254 caracteres)",
		"domínio do email inválido (deve ter 1-253 caracteres)",
	}, response.Message)
}

// ==================== Invalid Format Tests ====================

func TestValidateEmailAddress_InvalidFormat(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name            string
		email           string
		expectedMessage string
	}{
		{
			name:            "missing @ symbol",
			email:           "userexample.com",
			expectedMessage: "formato de email inválido",
		},
		{
			name:            "multiple @ symbols",
			email:           "user@@example.com",
			expectedMessage: "formato de email inválido",
		},
		{
			name:            "no domain after @",
			email:           "user@",
			expectedMessage: "formato de email inválido",
		},
		{
			name:            "no local part before @",
			email:           "@example.com",
			expectedMessage: "formato de email inválido",
		},
		{
			name:            "space in local part",
			email:           "user name@example.com",
			expectedMessage: "formato de email inválido",
		},
		// Note: "user!@example.com" and "user😀@example.com" are actually valid per RFC 5322
		// Go's mail.ParseAddress() correctly accepts them, so we don't test them as invalid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := EmailValidationRequest{
				Email: tt.email,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Expected OK status for invalid format")

			var response EmailValidationResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.False(t, response.Valid, "Email should be invalid: %s", tt.email)
			assert.Equal(t, tt.expectedMessage, response.Message)
			assert.Empty(t, response.LocalPart, "LocalPart should be empty for invalid email")
			assert.Empty(t, response.Domain, "Domain should be empty for invalid email")
			assert.Empty(t, response.Normalized, "Normalized should be empty for invalid email")
		})
	}
}

// ==================== Invalid Structure Tests ====================

func TestValidateEmailAddress_InvalidDomainStructure(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name            string
		email           string
		expectedMessage string
	}{
		{
			name:            "domain starts with dot",
			email:           "user@.example.com",
			expectedMessage: "domínio não pode começar ou terminar com ponto",
		},
		{
			name:            "domain ends with dot",
			email:           "user@example.com.",
			expectedMessage: "domínio não pode começar ou terminar com ponto",
		},
		{
			name:            "consecutive dots in domain",
			email:           "user@example..com",
			expectedMessage: "domínio não pode conter pontos consecutivos",
		},
		{
			name:            "no domain extension",
			email:           "user@example",
			expectedMessage: "domínio deve conter pelo menos um ponto",
		},
		{
			name:            "domain is localhost",
			email:           "user@localhost",
			expectedMessage: "domínio deve conter pelo menos um ponto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := EmailValidationRequest{
				Email: tt.email,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Expected OK status for invalid structure")

			var response EmailValidationResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.False(t, response.Valid, "Email should be invalid: %s", tt.email)
			assert.Equal(t, tt.expectedMessage, response.Message)
		})
	}
}

// ==================== Edge Cases Tests ====================

func TestValidateEmailAddress_EdgeCases(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name          string
		email         string
		expectedValid bool
		description   string
	}{
		{
			name:          "email with maximum local part length (64 chars)",
			email:         strings.Repeat("a", 64) + "@example.com",
			expectedValid: true,
			description:   "Maximum valid local part length",
		},
		{
			name:          "email with maximum domain length (253 chars)",
			email:         "user@" + strings.Repeat("a", 240) + ".example.com",
			expectedValid: true,
			description:   "Maximum valid domain length",
		},
		{
			name:          "email with maximum total length (254 chars)",
			email:         strings.Repeat("a", 64) + "@" + strings.Repeat("b", 180) + ".example.com",
			expectedValid: true,
			description:   "Maximum valid total length",
		},
		{
			name:          "single character local part",
			email:         "a@example.com",
			expectedValid: true,
			description:   "Minimum local part length",
		},
		{
			name:          "single character domain segments",
			email:         "user@a.b.com",
			expectedValid: true,
			description:   "Single character domain segments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := EmailValidationRequest{
				Email: tt.email,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Expected status OK")

			var response EmailValidationResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedValid, response.Valid, "Test case: %s", tt.description)
			if tt.expectedValid {
				assert.NotEmpty(t, response.LocalPart)
				assert.NotEmpty(t, response.Domain)
				assert.NotEmpty(t, response.Normalized)
				assert.Equal(t, "format_and_structure", response.ValidationType)
			}
		})
	}
}

// ==================== Response Structure Tests ====================

func TestValidateEmailAddress_ResponseFields(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	reqBody := EmailValidationRequest{
		Email: "testuser@example.com",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status OK")

	var response EmailValidationResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify all response fields are present and correct
	assert.True(t, response.Valid)
	assert.Equal(t, "email válido", response.Message)
	assert.Equal(t, "testuser", response.LocalPart)
	assert.Equal(t, "example.com", response.Domain)
	assert.Equal(t, "testuser@example.com", response.Normalized)
	assert.Equal(t, "format_and_structure", response.ValidationType)
}

func TestValidateEmailAddress_InvalidEmailResponseFields(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	reqBody := EmailValidationRequest{
		Email: "invalid@",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status OK")

	var response EmailValidationResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify response fields for invalid email
	assert.False(t, response.Valid)
	assert.NotEmpty(t, response.Message)
	assert.Empty(t, response.LocalPart, "LocalPart should be empty for invalid email")
	assert.Empty(t, response.Domain, "Domain should be empty for invalid email")
	assert.Empty(t, response.Normalized, "Normalized should be empty for invalid email")
	assert.Empty(t, response.ValidationType, "ValidationType should be empty for invalid email")
}

// ==================== Content Type Tests ====================

func TestValidateEmailAddress_WithoutContentType(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	reqBody := EmailValidationRequest{
		Email: "user@example.com",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	// Test without Content-Type header
	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Gin should still handle it
	assert.Equal(t, http.StatusOK, w.Code, "Should work without explicit Content-Type")
}

func TestValidateEmailAddress_WithWrongContentType(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	reqBody := EmailValidationRequest{
		Email: "user@example.com",
	}

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should still work as Gin is flexible with content types
	assert.Equal(t, http.StatusOK, w.Code)
}

// ==================== Special Characters in Email Tests ====================

func TestValidateEmailAddress_SpecialCharacters(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name          string
		email         string
		expectedValid bool
	}{
		{
			name:          "email with plus sign",
			email:         "user+filter@example.com",
			expectedValid: true,
		},
		{
			name:          "email with dot in local part",
			email:         "first.last@example.com",
			expectedValid: true,
		},
		{
			name:          "email with underscore",
			email:         "user_name@example.com",
			expectedValid: true,
		},
		{
			name:          "email with hyphen in local part",
			email:         "user-name@example.com",
			expectedValid: true,
		},
		{
			name:          "email with numbers",
			email:         "user123@example456.com",
			expectedValid: true,
		},
		{
			name:          "email with multiple dots",
			email:         "first.middle.last@example.com",
			expectedValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := EmailValidationRequest{
				Email: tt.email,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Expected status OK")

			var response EmailValidationResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedValid, response.Valid, "Email: %s", tt.email)
			if tt.expectedValid {
				assert.NotEmpty(t, response.LocalPart)
				assert.NotEmpty(t, response.Domain)
				assert.NotEmpty(t, response.Normalized)
			}
		})
	}
}

// ==================== Real-World Email Examples ====================

func TestValidateEmailAddress_RealWorldExamples(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name          string
		email         string
		expectedValid bool
	}{
		{
			name:          "Gmail address",
			email:         "user@gmail.com",
			expectedValid: true,
		},
		{
			name:          "Corporate email",
			email:         "john.doe@company.com",
			expectedValid: true,
		},
		{
			name:          "Government email",
			email:         "contact@gov.br",
			expectedValid: true,
		},
		{
			name:          "Educational email",
			email:         "student@university.edu",
			expectedValid: true,
		},
		{
			name:          "Email with country code",
			email:         "user@example.co.uk",
			expectedValid: true,
		},
		{
			name:          "Rio de Janeiro government",
			email:         "contato@rio.rj.gov.br",
			expectedValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := EmailValidationRequest{
				Email: tt.email,
			}

			body, err := json.Marshal(reqBody)
			require.NoError(t, err)

			req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Expected status OK")

			var response EmailValidationResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedValid, response.Valid, "Email: %s", tt.email)
		})
	}
}

// ==================== Concurrent Request Tests ====================

func TestValidateEmailAddress_ConcurrentRequests(t *testing.T) {
	router, cleanup := setupEmailHandlersTest(t)
	defer cleanup()

	// Test that the handler can handle concurrent requests
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	numRequests := 10
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(index int) {
			reqBody := EmailValidationRequest{
				Email: "user@example.com",
			}

			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/validate/email", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		<-done
	}
}
