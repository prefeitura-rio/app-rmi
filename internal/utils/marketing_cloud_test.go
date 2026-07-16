package utils

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSFMCMessageRequest_ContractMatchesDoc(t *testing.T) {
	req := buildSFMCMessageRequest("PREFRIO-OTT-DEFINITION-TEST", "codigo_otp", "12345678900", "5521999999999", "123456")

	body, err := json.Marshal(req)
	require.NoError(t, err)

	expected := `{"definitionKey":"PREFRIO-OTT-DEFINITION-TEST","recipients":[{"contactKey":"12345678900","to":"5521999999999","attributes":{"codigo_otp":"123456"}}]}`
	assert.JSONEq(t, expected, string(body))
}

func TestBuildSFMCMessageRequest_CustomOTPAttribute(t *testing.T) {
	req := buildSFMCMessageRequest("DEF", "otp", "cpf1", "5521988887777", "654321")

	require.Len(t, req.Recipients, 1)
	assert.Equal(t, "DEF", req.DefinitionKey)
	assert.Equal(t, "cpf1", req.Recipients[0].ContactKey)
	assert.Equal(t, "5521988887777", req.Recipients[0].To)
	assert.Equal(t, "654321", req.Recipients[0].Attributes["otp"])
}

func TestSendMarketingCloudOTP_DisabledSkips(t *testing.T) {
	original := config.AppConfig
	defer func() { config.AppConfig = original }()
	config.AppConfig = &config.Config{SFMCEnabled: false}

	err := SendMarketingCloudOTP(context.Background(), "12345678900", "5521999999999", "123456")
	assert.NoError(t, err)
}

func TestSendMarketingCloudOTP_InvalidPhone(t *testing.T) {
	original := config.AppConfig
	defer func() { config.AppConfig = original }()
	config.AppConfig = &config.Config{SFMCEnabled: true}

	err := SendMarketingCloudOTP(context.Background(), "12345678900", "invalid-phone", "123456")
	assert.Error(t, err)
}

func TestGetMarketingCloudToken_CacheHit(t *testing.T) {
	ctx := context.Background()
	cachedToken := "cached-test-token-12345"

	require.NoError(t, config.Redis.Set(ctx, sfmcTokenCacheKey, cachedToken, time.Minute).Err())
	defer config.Redis.Del(ctx, sfmcTokenCacheKey)

	token, err := getMarketingCloudToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, cachedToken, token, "a cached token must be returned without calling the SFMC auth API")
}

// TestGetMarketingCloudToken_CacheMissInvalidSubdomain exercises the cache-miss branch
// (auth body construction and HTTP request creation) using a subdomain with a control
// character, which makes the auth URL invalid and fails fast without any network I/O.
func TestGetMarketingCloudToken_CacheMissInvalidSubdomain(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, config.Redis.Del(ctx, sfmcTokenCacheKey).Err())

	original := config.AppConfig
	defer func() { config.AppConfig = original }()
	config.AppConfig = &config.Config{SFMCSubdomain: "bad\nsubdomain"}

	_, err := getMarketingCloudToken(ctx)
	assert.Error(t, err)
}

// TestSendMarketingCloudOTP_MessageRequestCreationFails exercises the message-send branch
// of SendMarketingCloudOTP up through HTTP request creation, using a cached token (to skip
// the auth call) and an invalid subdomain (to fail request creation without network I/O).
func TestSendMarketingCloudOTP_MessageRequestCreationFails(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, config.Redis.Set(ctx, sfmcTokenCacheKey, "cached-token", time.Minute).Err())
	defer config.Redis.Del(ctx, sfmcTokenCacheKey)

	original := config.AppConfig
	defer func() { config.AppConfig = original }()
	config.AppConfig = &config.Config{
		SFMCEnabled:       true,
		SFMCSubdomain:     "bad\nsubdomain",
		SFMCDefinitionKey: "PREFRIO-OTT-DEFINITION-TEST",
		SFMCOTPAttribute:  "codigo_otp",
	}

	err := SendMarketingCloudOTP(ctx, "12345678900", "5521999999999", "123456")
	assert.Error(t, err)
}
