package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestCitizenDataWorkflow tests the complete citizen data workflow
func TestCitizenDataWorkflow(t *testing.T) {
	baseURL := getBaseURL(t)
	token := getAuthToken(t)
	cpf := getTestCPF(t)
	client := &http.Client{Timeout: 30 * time.Second}

	t.Run("GetCitizenData", func(t *testing.T) {
		req, err := http.NewRequest("GET", baseURL+"/citizen/"+cpf, nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(body))
		}

		var data map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify basic structure
		if _, ok := data["cpf"]; !ok {
			t.Error("Response missing 'cpf' field")
		}
	})

	t.Run("UpdateSelfDeclaredAddress", func(t *testing.T) {
		payload := map[string]interface{}{
			"logradouro":   "Rua Teste E2E",
			"numero":       "123",
			"complemento":  "Apt 456",
			"bairro":       "Centro",
			"cidade":       "Rio de Janeiro",
			"estado":       "RJ",
			"cep":          "20000-000",
			"tipo_logrado": "Rua",
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		req, err := http.NewRequest("PUT", baseURL+"/citizen/"+cpf+"/address", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 (updated) or 409 (already exists with same data)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200 or 409, got %d. Body: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("UpdateSelfDeclaredEmail", func(t *testing.T) {
		// Use timestamp to avoid conflicts
		timestamp := time.Now().Unix()
		email := fmt.Sprintf("e2e-test-%d@example.com", timestamp)

		payload := map[string]string{
			"email": email,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		req, err := http.NewRequest("PUT", baseURL+"/citizen/"+cpf+"/email", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200 or 409, got %d. Body: %s", resp.StatusCode, string(body))
		}
	})
}

// getAuthToken retrieves or generates an authentication token
func getAuthToken(t *testing.T) string {
	// Check if token is provided directly
	if token := os.Getenv("TEST_BEARER_TOKEN"); token != "" {
		return token
	}

	// Otherwise, authenticate with Keycloak
	keycloakURL := os.Getenv("TEST_KEYCLOAK_URL")
	realm := os.Getenv("TEST_KEYCLOAK_REALM")
	clientID := os.Getenv("TEST_KEYCLOAK_CLIENT_ID")
	username := os.Getenv("TEST_USERNAME")
	password := os.Getenv("TEST_PASSWORD")

	if keycloakURL == "" || realm == "" || clientID == "" || username == "" || password == "" {
		t.Skip("Keycloak credentials not configured")
	}

	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", keycloakURL, realm)

	payload := fmt.Sprintf("grant_type=password&client_id=%s&username=%s&password=%s",
		clientID, username, password)

	req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("Failed to create auth request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to authenticate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Authentication failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		t.Fatalf("Failed to decode token response: %v", err)
	}

	return tokenResponse.AccessToken
}

// getTestCPF retrieves the test CPF from environment
func getTestCPF(t *testing.T) string {
	cpf := os.Getenv("TEST_CPF")
	if cpf == "" {
		t.Skip("TEST_CPF not set")
	}
	return cpf
}
