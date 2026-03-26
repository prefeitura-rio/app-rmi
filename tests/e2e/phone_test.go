package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

// TestPhoneOptInOptOut tests the phone verification workflow
func TestPhoneOptInOptOut(t *testing.T) {
	baseURL := getBaseURL(t)
	token := getAuthToken(t)
	cpf := getTestCPF(t)
	client := &http.Client{Timeout: 30 * time.Second}

	// Generate unique phone number for this test
	timestamp := time.Now().Unix()
	phone := fmt.Sprintf("5521%09d", timestamp%1000000000)

	t.Run("PhoneOptIn", func(t *testing.T) {
		payload := map[string]string{
			"telefone": phone,
			"cpf":      cpf,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		req, err := http.NewRequest("POST", baseURL+"/phone/opt-in", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// Accept 200 (success) or 409 (already opted in)
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200 or 409, got %d. Body: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("GetCPFFromPhone", func(t *testing.T) {
		req, err := http.NewRequest("GET", baseURL+"/phone/"+phone+"/cpf", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(body))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		returnedCPF, ok := result["cpf"].(string)
		if !ok || returnedCPF != cpf {
			t.Errorf("Expected CPF %s, got %v", cpf, result["cpf"])
		}
	})

	t.Run("UpdateSelfDeclaredPhone", func(t *testing.T) {
		payload := map[string]string{
			"telefone": phone,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		req, err := http.NewRequest("PUT", baseURL+"/citizen/"+cpf+"/phone", bytes.NewReader(body))
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

	t.Run("PhoneOptOut", func(t *testing.T) {
		payload := map[string]string{
			"telefone": phone,
		}

		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("Failed to marshal payload: %v", err)
		}

		req, err := http.NewRequest("POST", baseURL+"/phone/opt-out", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected status 200, got %d. Body: %s", resp.StatusCode, string(body))
		}
	})

	t.Run("VerifyOptOutStatus", func(t *testing.T) {
		req, err := http.NewRequest("GET", baseURL+"/phone/"+phone+"/cpf", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		// After opt-out, should return 404
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected status 404 after opt-out, got %d. Body: %s", resp.StatusCode, string(body))
		}
	})
}
