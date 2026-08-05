package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DataRelayClient talks to the Data Relay API (mailman) for transactional email.
type DataRelayClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewDataRelayClient creates a Data Relay client. timeout defaults to 30s when <= 0.
func NewDataRelayClient(baseURL, apiKey string, timeout time.Duration) *DataRelayClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &DataRelayClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Configured reports whether base URL and API key are both set.
func (c *DataRelayClient) Configured() bool {
	return c != nil && c.baseURL != "" && c.apiKey != ""
}

// EmailRequest is the Data Relay mailman payload.
type EmailRequest struct {
	ToAddresses  []string `json:"to_addresses"`
	CCAddresses  []string `json:"cc_addresses,omitempty"`
	BCCAddresses []string `json:"bcc_addresses,omitempty"`
	ReplyTo      []string `json:"reply_to,omitempty"`
	Subject      string   `json:"subject"`
	Body         string   `json:"body"`
	IsHTMLBody   bool     `json:"is_html_body"`
}

// SendEmail posts to {baseURL}/data/mailman with X-Api-Key.
func (c *DataRelayClient) SendEmail(ctx context.Context, req *EmailRequest) error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("data relay base URL not configured")
	}
	if c.apiKey == "" {
		return fmt.Errorf("data relay API key not configured")
	}
	if req == nil {
		return fmt.Errorf("email request is nil")
	}
	if len(req.ToAddresses) == 0 {
		return fmt.Errorf("to_addresses is required")
	}

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal email request: %w", err)
	}

	url := c.baseURL + "/data/mailman"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("data relay API returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
