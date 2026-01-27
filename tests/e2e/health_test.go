package e2e_test

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestHealth verifies the health endpoint is responding
func TestHealth(t *testing.T) {
	baseURL := getBaseURL(t)
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var health map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}

	status, ok := health["status"].(string)
	if !ok || status != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", health["status"])
	}
}

// getBaseURL retrieves the base URL from environment variable
func getBaseURL(t *testing.T) string {
	baseURL := os.Getenv("TEST_BASE_URL")
	if baseURL == "" {
		t.Skip("TEST_BASE_URL not set, skipping E2E test")
	}
	return baseURL
}
