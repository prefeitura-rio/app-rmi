package config

import (
	"fmt"
	"os"
)

// TestConfig holds configuration for E2E/smoke tests
type TestConfig struct {
	// API endpoint configuration
	BaseURL string // e.g., "https://services.staging.app.dados.rio/rmi/v1"

	// Authentication
	KeycloakURL      string
	KeycloakRealm    string
	KeycloakClientID string
	Username         string
	Password         string

	// Test data
	TestCPF string // Valid CPF for testing

	// Test timeouts
	HealthCheckTimeout int // seconds
	APICallTimeout     int // seconds
}

// LoadTestConfig loads configuration from environment variables
func LoadTestConfig() (*TestConfig, error) {
	baseURL := os.Getenv("TEST_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080/v1" // Default for local testing
	}

	keycloakURL := os.Getenv("TEST_KEYCLOAK_URL")
	if keycloakURL == "" {
		return nil, fmt.Errorf("TEST_KEYCLOAK_URL is required")
	}

	keycloakRealm := os.Getenv("TEST_KEYCLOAK_REALM")
	if keycloakRealm == "" {
		return nil, fmt.Errorf("TEST_KEYCLOAK_REALM is required")
	}

	keycloakClientID := os.Getenv("TEST_KEYCLOAK_CLIENT_ID")
	if keycloakClientID == "" {
		return nil, fmt.Errorf("TEST_KEYCLOAK_CLIENT_ID is required")
	}

	username := os.Getenv("TEST_USERNAME")
	if username == "" {
		return nil, fmt.Errorf("TEST_USERNAME is required")
	}

	password := os.Getenv("TEST_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("TEST_PASSWORD is required")
	}

	testCPF := os.Getenv("TEST_CPF")
	if testCPF == "" {
		return nil, fmt.Errorf("TEST_CPF is required")
	}

	return &TestConfig{
		BaseURL:            baseURL,
		KeycloakURL:        keycloakURL,
		KeycloakRealm:      keycloakRealm,
		KeycloakClientID:   keycloakClientID,
		Username:           username,
		Password:           password,
		TestCPF:            testCPF,
		HealthCheckTimeout: 30,
		APICallTimeout:     10,
	}, nil
}
