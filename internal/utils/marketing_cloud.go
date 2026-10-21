package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/utils/httpclient"
	"go.uber.org/zap"
)

const sfmcTokenCacheKey = "sfmc:token"

type sfmcTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type sfmcRecipient struct {
	ContactKey string            `json:"contactKey"`
	To         string            `json:"to"`
	Attributes map[string]string `json:"attributes"`
}

type sfmcMessageRequest struct {
	DefinitionKey string          `json:"definitionKey"`
	Recipients    []sfmcRecipient `json:"recipients"`
}

func buildSFMCMessageRequest(definitionKey, otpAttribute, contactKey, phone, code string) sfmcMessageRequest {
	return sfmcMessageRequest{
		DefinitionKey: definitionKey,
		Recipients: []sfmcRecipient{
			{
				ContactKey: contactKey,
				To:         phone,
				Attributes: map[string]string{otpAttribute: code},
			},
		},
	}
}

// getMarketingCloudToken gets a Salesforce Marketing Cloud OAuth token, using Redis for caching.
func getMarketingCloudToken(ctx context.Context) (string, error) {
	logger := logging.GetLogger().With(zap.String("operation", "get_sfmc_token"))

	token, err := config.Redis.Get(ctx, sfmcTokenCacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("sfmc_token").Inc()
		return token, nil
	}

	authURL := fmt.Sprintf("https://%s.auth.marketingcloudapis.com/v2/token", config.AppConfig.SFMCSubdomain)
	authBody := map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     config.AppConfig.SFMCClientID,
		"client_secret": config.AppConfig.SFMCClientSecret,
		"account_id":    config.AppConfig.SFMCAccountID,
	}

	jsonBody, err := json.Marshal(authBody)
	if err != nil {
		logger.Error("failed to marshal SFMC auth request", zap.Error(err))
		return "", fmt.Errorf("failed to marshal SFMC auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("failed to create SFMC auth request", zap.Error(err))
		return "", fmt.Errorf("failed to create SFMC auth request: %w", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	client := httpclient.GetGlobalPool().Get()
	defer httpclient.GetGlobalPool().Put(client)

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to send SFMC auth request", zap.Error(err))
		return "", fmt.Errorf("failed to send SFMC auth request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read SFMC auth response body", zap.Error(err))
		return "", fmt.Errorf("failed to read SFMC auth response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SFMC auth request failed with status: %d", resp.StatusCode)
	}

	var authResp sfmcTokenResponse
	if err := json.Unmarshal(body, &authResp); err != nil {
		logger.Error("failed to decode SFMC auth response", zap.Error(err))
		return "", fmt.Errorf("failed to decode SFMC auth response: %w", err)
	}
	if authResp.AccessToken == "" {
		return "", fmt.Errorf("SFMC auth response missing access_token")
	}

	// Cache with a 1-minute safety margin so a token is never used past its expiry.
	ttl := time.Duration(authResp.ExpiresIn)*time.Second - time.Minute
	if ttl > 0 {
		if err := config.Redis.Set(ctx, sfmcTokenCacheKey, authResp.AccessToken, ttl).Err(); err != nil {
			logger.Warn("failed to cache SFMC token", zap.Error(err))
		}
	}

	return authResp.AccessToken, nil
}

// SendMarketingCloudOTP sends a single OTP code via the Salesforce Marketing Cloud OTT API.
func SendMarketingCloudOTP(ctx context.Context, contactKey, phone, code string) error {
	logger := logging.GetLogger().With(
		zap.String("phone", phone),
		zap.String("provider", config.VerificationProviderSalesforce),
	)

	if !config.AppConfig.SFMCEnabled {
		logger.Info("SFMC messaging is disabled, skipping OTP send")
		return nil
	}

	if err := validatePhoneNumber(phone); err != nil {
		logger.Error("invalid phone number", zap.Error(err))
		return err
	}

	token, err := getMarketingCloudToken(ctx)
	if err != nil {
		logger.Error("failed to get SFMC token", zap.Error(err))
		return fmt.Errorf("failed to get SFMC token: %w", err)
	}

	msgReq := buildSFMCMessageRequest(config.AppConfig.SFMCDefinitionKey, config.AppConfig.SFMCOTPAttribute, contactKey, phone, code)

	jsonBody, err := json.Marshal(msgReq)
	if err != nil {
		logger.Error("failed to marshal SFMC message request", zap.Error(err))
		return fmt.Errorf("failed to marshal SFMC message request: %w", err)
	}

	url := fmt.Sprintf("https://%s.rest.marketingcloudapis.com/messaging/v1/ott/messages/", config.AppConfig.SFMCSubdomain)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("failed to create SFMC message request", zap.Error(err))
		return fmt.Errorf("failed to create SFMC message request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := httpclient.GetGlobalPool().Get()
	defer httpclient.GetGlobalPool().Put(client)

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("failed to send SFMC message request", zap.Error(err))
		return fmt.Errorf("failed to send SFMC message request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read SFMC response body", zap.Error(err))
		return fmt.Errorf("failed to read SFMC response body: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		logger.Error("SFMC message request failed",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response_body", string(body)))
		return fmt.Errorf("SFMC message request failed with status: %d", resp.StatusCode)
	}

	return nil
}
