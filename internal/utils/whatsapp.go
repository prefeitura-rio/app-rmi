package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/utils/httpclient"
	"go.uber.org/zap"
)

type authResponse struct {
	Data struct {
		Item struct {
			Token      string `json:"token"`
			Expiration int64  `json:"expiration"`
		} `json:"item"`
	} `json:"data"`
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

type destination struct {
	To   string                 `json:"to"`
	Vars map[string]interface{} `json:"vars"`
}

type messageRequest struct {
	CostCenterID  int           `json:"costCenterId"`
	CampaignName  string        `json:"campaignName"`
	Destinations  []destination `json:"destinations"`
}

type errorResponse struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

var phoneRegex = regexp.MustCompile(`^[0-9]{10,15}$`)

// validatePhoneNumber validates if the phone number is in the correct format
func validatePhoneNumber(phone string) error {
	if !phoneRegex.MatchString(phone) {
		return fmt.Errorf("invalid phone number format: %s", phone)
	}
	return nil
}

// getAuthToken gets a WhatsApp API token, using Redis for caching
func getAuthToken(ctx context.Context) (string, error) {
	logger := logging.Logger.With(zap.String("operation", "get_auth_token"))

	// Try to get from Redis first
	cacheKey := "whatsapp:token"
	token, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("whatsapp_token").Inc()
		return token, nil
	}

	// If not in cache, get new token
	authURL := fmt.Sprintf("%s/users/login", config.AppConfig.WhatsAppBaseURL)
	authBody := map[string]string{
		"username": config.AppConfig.WhatsAppUsername,
		"password": config.AppConfig.WhatsAppPassword,
	}

	jsonBody, err := json.Marshal(authBody)
	if err != nil {
		logger.Error("failed to marshal auth request", zap.Error(err))
		return "", fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := http.NewRequest("POST", authURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("failed to create auth request", zap.Error(err))
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	// Use HTTP client pool for optimal performance
	client := httpclient.GetGlobalPool().Get()
	defer httpclient.GetGlobalPool().Put(client)
	
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to send auth request", zap.Error(err))
		return "", fmt.Errorf("failed to send auth request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read auth response body", zap.Error(err))
		return "", fmt.Errorf("failed to read auth response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth request failed with status: %d", resp.StatusCode)
	}

	var authResp authResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		logger.Error("failed to decode auth response", zap.Error(err))
		return "", fmt.Errorf("failed to decode auth response: %w", err)
	}

	// Cache the token in Redis with TTL slightly less than expiration
	expiresAt := time.Unix(0, authResp.Data.Item.Expiration*int64(time.Millisecond))
	ttl := time.Until(expiresAt) - time.Minute // Cache for 1 minute less than expiration
	if ttl > 0 {
		if err := config.Redis.Set(ctx, cacheKey, authResp.Data.Item.Token, ttl).Err(); err != nil {
			logger.Warn("failed to cache WhatsApp token", zap.Error(err))
		}
	}

	return authResp.Data.Item.Token, nil
}

// SendWhatsAppMessage sends a message to multiple phone numbers using the WhatsApp API
func SendWhatsAppMessage(ctx context.Context, phones []string, hsmID string, varsList []map[string]interface{}) error {
	logger := logging.Logger.With(
		zap.Strings("phones", phones),
		zap.String("hsm_id", hsmID),
	)

	// Check if WhatsApp is enabled
	if !config.AppConfig.WhatsAppEnabled {
		logger.Info("WhatsApp messaging is disabled, skipping message send")
		return nil
	}

	if len(phones) != len(varsList) {
		return fmt.Errorf("number of phones (%d) must match number of vars (%d)", len(phones), len(varsList))
	}

	// Validate all phone numbers
	for _, phone := range phones {
		if err := validatePhoneNumber(phone); err != nil {
			logger.Error("invalid phone number", zap.Error(err))
			return err
		}
	}

	token, err := getAuthToken(ctx)
	if err != nil {
		logger.Error("failed to get auth token", zap.Error(err))
		return fmt.Errorf("failed to get auth token: %w", err)
	}

	destinations := make([]destination, len(phones))
	for i, phone := range phones {
		destinations[i] = destination{
			To:   phone,
			Vars: varsList[i],
		}
	}

	costCenterID, err := strconv.Atoi(config.AppConfig.WhatsAppCostCenterID)
	if err != nil {
		logger.Error("invalid cost center ID", 
			zap.String("cost_center_id", config.AppConfig.WhatsAppCostCenterID),
			zap.Error(err))
		return fmt.Errorf("invalid cost center ID: %w", err)
	}

	msgReq := messageRequest{
		CostCenterID: costCenterID,
		CampaignName: config.AppConfig.WhatsAppCampaignName,
		Destinations: destinations,
	}

	jsonBody, err := json.Marshal(msgReq)
	if err != nil {
		logger.Error("failed to marshal message request", zap.Error(err))
		return fmt.Errorf("failed to marshal message request: %w", err)
	}

	url := fmt.Sprintf("%s/callcenter/hsm/send/%s", config.AppConfig.WhatsAppBaseURL, hsmID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("failed to create message request", zap.Error(err))
		return fmt.Errorf("failed to create message request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	// Use HTTP client pool for optimal performance
	client := httpclient.GetGlobalPool().Get()
	defer httpclient.GetGlobalPool().Put(client)
	
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to send message request", zap.Error(err))
		return fmt.Errorf("failed to send message request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read response body", zap.Error(err))
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		var errResp errorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
			logger.Error("message request failed",
				zap.Int("status_code", resp.StatusCode),
				zap.String("error_message", errResp.Message))
			return fmt.Errorf("message request failed: %s", errResp.Message)
		}
		logger.Error("message request failed",
			zap.Int("status_code", resp.StatusCode))
		return fmt.Errorf("message request failed with status: %d", resp.StatusCode)
	}

	return nil
} 