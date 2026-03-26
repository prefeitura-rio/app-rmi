package services

import (
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{
			UserConfigCollection: "user_config",
		}
	}
}

func TestCitizenDataOperation_GetKey(t *testing.T) {
	op := &CitizenDataOperation{
		CPF: "12345678901",
	}
	assert.Equal(t, "12345678901", op.GetKey())
}

func TestCitizenDataOperation_GetCollection(t *testing.T) {
	op := &CitizenDataOperation{}
	assert.Equal(t, "citizens", op.GetCollection())
}

func TestCitizenDataOperation_GetData(t *testing.T) {
	citizen := &models.Citizen{
		CPF: "12345678901",
	}
	op := &CitizenDataOperation{
		Data: citizen,
	}
	data := op.GetData()
	require.NotNil(t, data)
	assert.Equal(t, citizen, data.(*models.Citizen))
}

func TestCitizenDataOperation_GetTTL(t *testing.T) {
	op := &CitizenDataOperation{}
	assert.Equal(t, 24*time.Hour, op.GetTTL())
}

func TestCitizenDataOperation_GetType(t *testing.T) {
	op := &CitizenDataOperation{}
	assert.Equal(t, "citizen", op.GetType())
}

func TestPhoneMappingDataOperation_GetKey(t *testing.T) {
	op := &PhoneMappingDataOperation{
		PhoneNumber: "+5521999887766",
	}
	assert.Equal(t, "+5521999887766", op.GetKey())
}

func TestPhoneMappingDataOperation_GetCollection(t *testing.T) {
	op := &PhoneMappingDataOperation{}
	assert.Equal(t, "phone_mappings", op.GetCollection())
}

func TestPhoneMappingDataOperation_GetData(t *testing.T) {
	mapping := &models.PhoneCPFMapping{
		PhoneNumber: "+5521999887766",
		CPF:         "12345678901",
	}
	op := &PhoneMappingDataOperation{
		Data: mapping,
	}
	data := op.GetData()
	require.NotNil(t, data)
	assert.Equal(t, mapping, data.(*models.PhoneCPFMapping))
}

func TestPhoneMappingDataOperation_GetTTL(t *testing.T) {
	op := &PhoneMappingDataOperation{}
	assert.Equal(t, 24*time.Hour, op.GetTTL())
}

func TestPhoneMappingDataOperation_GetType(t *testing.T) {
	op := &PhoneMappingDataOperation{}
	assert.Equal(t, "phone_mapping", op.GetType())
}

func TestUserConfigDataOperation_GetKey(t *testing.T) {
	op := &UserConfigDataOperation{
		UserID: "user123",
	}
	assert.Equal(t, "user123", op.GetKey())
}

func TestUserConfigDataOperation_GetCollection(t *testing.T) {
	op := &UserConfigDataOperation{}
	assert.Equal(t, config.AppConfig.UserConfigCollection, op.GetCollection())
}

func TestUserConfigDataOperation_GetData(t *testing.T) {
	userConfig := &models.UserConfig{
		FirstLogin: true,
	}
	op := &UserConfigDataOperation{
		Data: userConfig,
	}
	data := op.GetData()
	require.NotNil(t, data)
	assert.Equal(t, userConfig, data.(*models.UserConfig))
}

func TestUserConfigDataOperation_GetTTL(t *testing.T) {
	op := &UserConfigDataOperation{}
	assert.Equal(t, 24*time.Hour, op.GetTTL())
}

func TestUserConfigDataOperation_GetType(t *testing.T) {
	op := &UserConfigDataOperation{}
	assert.Equal(t, "user_config", op.GetType())
}

func TestCitizenDataOperation_NilData(t *testing.T) {
	op := &CitizenDataOperation{
		CPF:  "12345678901",
		Data: nil,
	}
	data := op.GetData()
	assert.Nil(t, data)
}

func TestPhoneMappingDataOperation_EmptyPhoneNumber(t *testing.T) {
	op := &PhoneMappingDataOperation{
		PhoneNumber: "",
	}
	assert.Equal(t, "", op.GetKey())
}

func TestUserConfigDataOperation_EmptyUserID(t *testing.T) {
	op := &UserConfigDataOperation{
		UserID: "",
	}
	assert.Equal(t, "", op.GetKey())
}

func TestAllDataOperations_Consistency(t *testing.T) {
	// Test that all operations have 24h TTL
	ops := []interface {
		GetTTL() time.Duration
	}{
		&CitizenDataOperation{},
		&PhoneMappingDataOperation{},
		&UserConfigDataOperation{},
	}

	for _, op := range ops {
		assert.Equal(t, 24*time.Hour, op.GetTTL(), "All operations should have 24h TTL")
	}
}

func TestAllDataOperations_Types(t *testing.T) {
	tests := []struct {
		name         string
		op           interface{ GetType() string }
		expectedType string
	}{
		{"citizen", &CitizenDataOperation{}, "citizen"},
		{"phone_mapping", &PhoneMappingDataOperation{}, "phone_mapping"},
		{"user_config", &UserConfigDataOperation{}, "user_config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedType, tt.op.GetType())
		})
	}
}
