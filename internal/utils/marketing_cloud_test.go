package utils

import (
	"context"
	"encoding/json"
	"testing"

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
