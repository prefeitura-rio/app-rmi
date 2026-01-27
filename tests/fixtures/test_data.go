package fixtures

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/prefeitura-rio/app-rmi/tests/config"
)

// TokenResponse represents Keycloak token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// GetAuthToken obtains JWT token from Keycloak
func GetAuthToken(cfg *config.TestConfig) (string, error) {
	tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		cfg.KeycloakURL, cfg.KeycloakRealm)

	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", cfg.KeycloakClientID)
	data.Set("username", cfg.Username)
	data.Set("password", cfg.Password)

	client := &http.Client{Timeout: time.Duration(cfg.APICallTimeout) * time.Second}
	resp, err := client.PostForm(tokenURL, data)
	if err != nil {
		return "", fmt.Errorf("failed to request token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// APIClient wraps HTTP client with common test functionality
type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

// NewAPIClient creates a new API client for testing
func NewAPIClient(cfg *config.TestConfig, token string) *APIClient {
	return &APIClient{
		BaseURL: cfg.BaseURL,
		HTTPClient: &http.Client{
			Timeout: time.Duration(cfg.APICallTimeout) * time.Second,
		},
		Token: token,
	}
}

// Get performs authenticated GET request
func (c *APIClient) Get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	return c.HTTPClient.Do(req)
}

// Post performs authenticated POST request
func (c *APIClient) Post(path string, body interface{}) (*http.Response, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+path, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	return c.HTTPClient.Do(req)
}

// Put performs authenticated PUT request
func (c *APIClient) Put(path string, body interface{}) (*http.Response, error) {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PUT", c.BaseURL+path, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	return c.HTTPClient.Do(req)
}

// Delete performs authenticated DELETE request
func (c *APIClient) Delete(path string) (*http.Response, error) {
	req, err := http.NewRequest("DELETE", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	return c.HTTPClient.Do(req)
}

// TestSelfDeclaredData represents test data for self-declared updates
type TestSelfDeclaredData struct {
	Address   string `json:"address"`
	Phone     string `json:"phone"`
	Email     string `json:"email"`
	Ethnicity string `json:"ethnicity"`
}

// GetTestSelfDeclaredData returns sample self-declared data for testing
func GetTestSelfDeclaredData() *TestSelfDeclaredData {
	return &TestSelfDeclaredData{
		Address:   "Rua Teste, 123 - Teste, Rio de Janeiro - RJ, 20000-000",
		Phone:     "21987654321",
		Email:     "test@example.com",
		Ethnicity: "Parda",
	}
}

// TestAvatarData represents test data for avatar operations
type TestAvatarData struct {
	AvatarID string `json:"avatar_id"`
}

// GetTestAvatarID returns a valid avatar ID for testing
func GetTestAvatarID() string {
	return "avatar_1" // Should match valid avatars in the system
}
