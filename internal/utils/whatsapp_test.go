package utils

import (
	"context"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
)

func TestValidatePhoneNumber(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		wantErr bool
	}{
		{
			name:    "valid 10 digits",
			phone:   "1234567890",
			wantErr: false,
		},
		{
			name:    "valid 11 digits",
			phone:   "12345678901",
			wantErr: false,
		},
		{
			name:    "valid 13 digits (international)",
			phone:   "5521999999999",
			wantErr: false,
		},
		{
			name:    "valid 15 digits (max)",
			phone:   "123456789012345",
			wantErr: false,
		},
		{
			name:    "too short - 9 digits",
			phone:   "123456789",
			wantErr: true,
		},
		{
			name:    "too long - 16 digits",
			phone:   "1234567890123456",
			wantErr: true,
		},
		{
			name:    "contains letters",
			phone:   "123456789a",
			wantErr: true,
		},
		{
			name:    "contains special characters",
			phone:   "+1234567890",
			wantErr: true,
		},
		{
			name:    "contains spaces",
			phone:   "12345 67890",
			wantErr: true,
		},
		{
			name:    "contains dashes",
			phone:   "12345-67890",
			wantErr: true,
		},
		{
			name:    "empty string",
			phone:   "",
			wantErr: true,
		},
		{
			name:    "only letters",
			phone:   "abcdefghij",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePhoneNumber(tt.phone)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePhoneNumber() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPhoneRegex_Pattern(t *testing.T) {
	// Test the regex directly
	validPhones := []string{
		"1234567890",
		"12345678901",
		"123456789012",
		"1234567890123",
		"12345678901234",
		"123456789012345",
	}

	for _, phone := range validPhones {
		if !phoneRegex.MatchString(phone) {
			t.Errorf("phoneRegex should match valid phone: %s", phone)
		}
	}

	invalidPhones := []string{
		"123456789",        // too short
		"1234567890123456", // too long
		"123456789a",       // contains letter
		"+1234567890",      // contains +
		"(12) 3456-7890",   // formatted
	}

	for _, phone := range invalidPhones {
		if phoneRegex.MatchString(phone) {
			t.Errorf("phoneRegex should not match invalid phone: %s", phone)
		}
	}
}

func TestSendWhatsAppMessage_Disabled(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// Save original value
	original := config.AppConfig.WhatsAppEnabled
	defer func() {
		config.AppConfig.WhatsAppEnabled = original
	}()

	config.AppConfig.WhatsAppEnabled = false

	ctx := context.Background()
	phones := []string{"5521999999999"}
	hsmID := "test_hsm"
	varsList := []map[string]interface{}{
		{"code": "123456"},
	}

	err := SendWhatsAppMessage(ctx, phones, hsmID, varsList)
	if err != nil {
		t.Errorf("SendWhatsAppMessage() with disabled WhatsApp error = %v, want nil", err)
	}
}

func TestSendWhatsAppMessage_PhoneVarsMismatch(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// Save original value
	original := config.AppConfig.WhatsAppEnabled
	defer func() {
		config.AppConfig.WhatsAppEnabled = original
	}()

	config.AppConfig.WhatsAppEnabled = true

	ctx := context.Background()
	phones := []string{"5521999999999", "5521888888888"}
	hsmID := "test_hsm"
	varsList := []map[string]interface{}{
		{"code": "123456"},
		// Missing second vars
	}

	err := SendWhatsAppMessage(ctx, phones, hsmID, varsList)
	if err == nil {
		t.Error("SendWhatsAppMessage() with mismatched phones/vars error = nil, want error")
	}
}

func TestSendWhatsAppMessage_InvalidPhone(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// Save original value
	original := config.AppConfig.WhatsAppEnabled
	defer func() {
		config.AppConfig.WhatsAppEnabled = original
	}()

	config.AppConfig.WhatsAppEnabled = true

	ctx := context.Background()
	phones := []string{"invalid_phone"}
	hsmID := "test_hsm"
	varsList := []map[string]interface{}{
		{"code": "123456"},
	}

	err := SendWhatsAppMessage(ctx, phones, hsmID, varsList)
	if err == nil {
		t.Error("SendWhatsAppMessage() with invalid phone error = nil, want error")
	}
}

func TestAuthResponse_Structure(t *testing.T) {
	resp := authResponse{
		StatusCode: 200,
		Message:    "success",
	}

	resp.Data.Item.Token = "test_token"
	resp.Data.Item.Expiration = 1234567890

	if resp.StatusCode != 200 {
		t.Errorf("authResponse StatusCode = %v, want 200", resp.StatusCode)
	}

	if resp.Message != "success" {
		t.Errorf("authResponse Message = %v, want success", resp.Message)
	}

	if resp.Data.Item.Token != "test_token" {
		t.Errorf("authResponse Token = %v, want test_token", resp.Data.Item.Token)
	}

	if resp.Data.Item.Expiration != 1234567890 {
		t.Errorf("authResponse Expiration = %v, want 1234567890", resp.Data.Item.Expiration)
	}
}

func TestDestination_Structure(t *testing.T) {
	dest := destination{
		To: "5521999999999",
		Vars: map[string]interface{}{
			"code": "123456",
			"name": "Test User",
		},
	}

	if dest.To != "5521999999999" {
		t.Errorf("destination To = %v, want 5521999999999", dest.To)
	}

	if dest.Vars["code"] != "123456" {
		t.Errorf("destination Vars[code] = %v, want 123456", dest.Vars["code"])
	}

	if dest.Vars["name"] != "Test User" {
		t.Errorf("destination Vars[name] = %v, want Test User", dest.Vars["name"])
	}
}

func TestMessageRequest_Structure(t *testing.T) {
	msgReq := messageRequest{
		CostCenterID: 123,
		CampaignName: "test_campaign",
		Destinations: []destination{
			{
				To: "5521999999999",
				Vars: map[string]interface{}{
					"code": "123456",
				},
			},
		},
	}

	if msgReq.CostCenterID != 123 {
		t.Errorf("messageRequest CostCenterID = %v, want 123", msgReq.CostCenterID)
	}

	if msgReq.CampaignName != "test_campaign" {
		t.Errorf("messageRequest CampaignName = %v, want test_campaign", msgReq.CampaignName)
	}

	if len(msgReq.Destinations) != 1 {
		t.Errorf("messageRequest Destinations length = %v, want 1", len(msgReq.Destinations))
	}

	if msgReq.Destinations[0].To != "5521999999999" {
		t.Errorf("messageRequest Destinations[0].To = %v, want 5521999999999", msgReq.Destinations[0].To)
	}
}

func TestErrorResponse_Structure(t *testing.T) {
	errResp := errorResponse{
		StatusCode: 400,
		Message:    "Bad Request",
	}

	if errResp.StatusCode != 400 {
		t.Errorf("errorResponse StatusCode = %v, want 400", errResp.StatusCode)
	}

	if errResp.Message != "Bad Request" {
		t.Errorf("errorResponse Message = %v, want Bad Request", errResp.Message)
	}
}

func TestValidatePhoneNumber_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		wantErr bool
	}{
		{
			name:    "exactly 10 digits",
			phone:   "1234567890",
			wantErr: false,
		},
		{
			name:    "exactly 15 digits",
			phone:   "123456789012345",
			wantErr: false,
		},
		{
			name:    "mixed with parentheses",
			phone:   "(11)99999-9999",
			wantErr: true,
		},
		{
			name:    "international format with +",
			phone:   "+5521999999999",
			wantErr: true,
		},
		{
			name:    "dots as separator",
			phone:   "12.3456.7890",
			wantErr: true,
		},
		{
			name:    "leading zero",
			phone:   "01234567890",
			wantErr: false,
		},
		{
			name:    "all zeros",
			phone:   "0000000000",
			wantErr: false,
		},
		{
			name:    "all nines",
			phone:   "9999999999",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePhoneNumber(tt.phone)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePhoneNumber() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSendWhatsAppMessage_EmptyPhones(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// Save original value
	original := config.AppConfig.WhatsAppEnabled
	defer func() {
		config.AppConfig.WhatsAppEnabled = original
	}()

	config.AppConfig.WhatsAppEnabled = false // Test with disabled to avoid HTTP calls

	ctx := context.Background()
	phones := []string{}
	hsmID := "test_hsm"
	varsList := []map[string]interface{}{}

	// With disabled WhatsApp, this should return nil
	err := SendWhatsAppMessage(ctx, phones, hsmID, varsList)
	if err != nil {
		t.Errorf("SendWhatsAppMessage() with empty phones and disabled WhatsApp error = %v, want nil", err)
	}
}

func TestMessageRequest_MultipleDestinations(t *testing.T) {
	destinations := []destination{
		{
			To: "5521999999999",
			Vars: map[string]interface{}{
				"code": "123456",
				"name": "User 1",
			},
		},
		{
			To: "5521888888888",
			Vars: map[string]interface{}{
				"code": "654321",
				"name": "User 2",
			},
		},
	}

	msgReq := messageRequest{
		CostCenterID: 123,
		CampaignName: "test_campaign",
		Destinations: destinations,
	}

	if len(msgReq.Destinations) != 2 {
		t.Errorf("messageRequest Destinations length = %v, want 2", len(msgReq.Destinations))
	}

	if msgReq.Destinations[0].Vars["name"] != "User 1" {
		t.Errorf("messageRequest Destinations[0].Vars[name] = %v, want User 1", msgReq.Destinations[0].Vars["name"])
	}

	if msgReq.Destinations[1].Vars["name"] != "User 2" {
		t.Errorf("messageRequest Destinations[1].Vars[name] = %v, want User 2", msgReq.Destinations[1].Vars["name"])
	}
}
