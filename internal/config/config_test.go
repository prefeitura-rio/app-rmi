package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		setEnv       bool
		want         string
	}{
		{
			name:         "environment variable set",
			key:          "TEST_KEY_1",
			defaultValue: "default",
			envValue:     "custom",
			setEnv:       true,
			want:         "custom",
		},
		{
			name:         "environment variable not set",
			key:          "TEST_KEY_2",
			defaultValue: "default",
			envValue:     "",
			setEnv:       false,
			want:         "default",
		},
		{
			name:         "empty environment variable",
			key:          "TEST_KEY_3",
			defaultValue: "default",
			envValue:     "",
			setEnv:       true,
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvAsIntOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		setEnv       bool
		want         int
	}{
		{
			name:         "valid integer",
			key:          "TEST_INT_1",
			defaultValue: 10,
			envValue:     "42",
			setEnv:       true,
			want:         42,
		},
		{
			name:         "not set",
			key:          "TEST_INT_2",
			defaultValue: 10,
			envValue:     "",
			setEnv:       false,
			want:         10,
		},
		{
			name:         "invalid integer",
			key:          "TEST_INT_3",
			defaultValue: 10,
			envValue:     "invalid",
			setEnv:       true,
			want:         10,
		},
		{
			name:         "negative integer",
			key:          "TEST_INT_4",
			defaultValue: 10,
			envValue:     "-5",
			setEnv:       true,
			want:         -5,
		},
		{
			name:         "zero",
			key:          "TEST_INT_5",
			defaultValue: 10,
			envValue:     "0",
			setEnv:       true,
			want:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvAsIntOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvAsIntOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvAsDurationOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue time.Duration
		envValue     string
		setEnv       bool
		want         time.Duration
	}{
		{
			name:         "valid duration",
			key:          "TEST_DUR_1",
			defaultValue: 10 * time.Second,
			envValue:     "5m",
			setEnv:       true,
			want:         5 * time.Minute,
		},
		{
			name:         "not set",
			key:          "TEST_DUR_2",
			defaultValue: 10 * time.Second,
			envValue:     "",
			setEnv:       false,
			want:         10 * time.Second,
		},
		{
			name:         "invalid duration",
			key:          "TEST_DUR_3",
			defaultValue: 10 * time.Second,
			envValue:     "invalid",
			setEnv:       true,
			want:         10 * time.Second,
		},
		{
			name:         "hours",
			key:          "TEST_DUR_4",
			defaultValue: 10 * time.Second,
			envValue:     "2h",
			setEnv:       true,
			want:         2 * time.Hour,
		},
		{
			name:         "milliseconds",
			key:          "TEST_DUR_5",
			defaultValue: 10 * time.Second,
			envValue:     "500ms",
			setEnv:       true,
			want:         500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			got := getEnvAsDurationOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvAsDurationOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCommaSeparatedList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{
			name:  "simple list",
			value: "a,b,c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "list with spaces",
			value: "a, b, c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "single item",
			value: "single",
			want:  []string{"single"},
		},
		{
			name:  "empty string",
			value: "",
			want:  []string{},
		},
		{
			name:  "only commas",
			value: ",,,",
			want:  []string{},
		},
		{
			name:  "trailing comma",
			value: "a,b,",
			want:  []string{"a", "b"},
		},
		{
			name:  "leading comma",
			value: ",a,b",
			want:  []string{"a", "b"},
		},
		{
			name:  "multiple spaces",
			value: "a  ,  b  ,  c",
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "redis cluster addresses",
			value: "redis1:6379,redis2:6379,redis3:6379",
			want:  []string{"redis1:6379", "redis2:6379", "redis3:6379"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaSeparatedList(tt.value)
			if len(got) != len(tt.want) {
				t.Errorf("parseCommaSeparatedList() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseCommaSeparatedList()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestConfig_Structure(t *testing.T) {
	cfg := &Config{
		Port:        8080,
		Environment: "test",
		MongoURI:    "mongodb://localhost:27017",
	}

	if cfg.Port != 8080 {
		t.Errorf("Config.Port = %v, want 8080", cfg.Port)
	}

	if cfg.Environment != "test" {
		t.Errorf("Config.Environment = %v, want test", cfg.Environment)
	}

	if cfg.MongoURI != "mongodb://localhost:27017" {
		t.Errorf("Config.MongoURI = %v, want mongodb://localhost:27017", cfg.MongoURI)
	}
}

func TestGetEnvOrDefault_Concurrent(t *testing.T) {
	key := "TEST_CONCURRENT"
	defaultValue := "default"

	os.Setenv(key, "value")
	defer os.Unsetenv(key)

	// Run concurrent reads
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			result := getEnvOrDefault(key, defaultValue)
			if result != "value" {
				t.Errorf("getEnvOrDefault() = %v, want value", result)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestParseCommaSeparatedList_EmptyItems(t *testing.T) {
	value := "a,,b,,c"
	got := parseCommaSeparatedList(value)

	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Errorf("parseCommaSeparatedList() length = %v, want %v", len(got), len(want))
	}
}

func TestGetEnvAsIntOrDefault_LargeNumber(t *testing.T) {
	key := "TEST_LARGE_INT"
	os.Setenv(key, "2147483647") // max int32
	defer os.Unsetenv(key)

	got := getEnvAsIntOrDefault(key, 0)
	if got != 2147483647 {
		t.Errorf("getEnvAsIntOrDefault() = %v, want 2147483647", got)
	}
}

func TestGetEnvAsDurationOrDefault_Complex(t *testing.T) {
	key := "TEST_COMPLEX_DUR"
	os.Setenv(key, "1h30m45s")
	defer os.Unsetenv(key)

	expected := 1*time.Hour + 30*time.Minute + 45*time.Second
	got := getEnvAsDurationOrDefault(key, 0)
	if got != expected {
		t.Errorf("getEnvAsDurationOrDefault() = %v, want %v", got, expected)
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	// Test that Config struct can be initialized with zero values
	cfg := &Config{}

	if cfg.Port != 0 {
		t.Errorf("Config.Port default = %v, want 0", cfg.Port)
	}

	if cfg.RedisDB != 0 {
		t.Errorf("Config.RedisDB default = %v, want 0", cfg.RedisDB)
	}

	if cfg.RedisTTL != 0 {
		t.Errorf("Config.RedisTTL default = %v, want 0", cfg.RedisTTL)
	}
}

func TestLoadConfig_Success(t *testing.T) {
	// Save original AppConfig and restore after test
	originalConfig := AppConfig
	defer func() { AppConfig = originalConfig }()

	// Set all required environment variables
	os.Setenv("PORT", "8080")
	os.Setenv("ENVIRONMENT", "test")
	os.Setenv("MONGODB_URI", "mongodb://localhost:27017")
	os.Setenv("MONGODB_DATABASE", "test_db")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	os.Setenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION", "maintenance_requests")
	os.Setenv("MONGODB_LEGAL_ENTITY_COLLECTION", "legal_entities")
	os.Setenv("MONGODB_PET_COLLECTION", "pets")
	os.Setenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION", "pets_self_registered")
	os.Setenv("MONGODB_CHAT_MEMORY_COLLECTION", "chat_memory")
	os.Setenv("MONGODB_DEPARTMENT_COLLECTION", "departments")
	os.Setenv("MONGODB_CNAE_COLLECTION", "cnae")
	os.Setenv("REDIS_URI", "localhost:6379")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("WHATSAPP_API_BASE_URL", "https://api.whatsapp.com")
	os.Setenv("WHATSAPP_API_USERNAME", "test_user")
	os.Setenv("WHATSAPP_API_PASSWORD", "test_pass")
	os.Setenv("WHATSAPP_HSM_ID", "hsm123")
	os.Setenv("WHATSAPP_COST_CENTER_ID", "cc123")
	os.Setenv("WHATSAPP_CAMPAIGN_NAME", "test_campaign")
	os.Setenv("CF_LOOKUP_ENABLED", "false")

	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("MONGODB_URI")
		os.Unsetenv("MONGODB_DATABASE")
		os.Unsetenv("MONGODB_CITIZEN_COLLECTION")
		os.Unsetenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")
		os.Unsetenv("MONGODB_LEGAL_ENTITY_COLLECTION")
		os.Unsetenv("MONGODB_PET_COLLECTION")
		os.Unsetenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION")
		os.Unsetenv("MONGODB_CHAT_MEMORY_COLLECTION")
		os.Unsetenv("MONGODB_DEPARTMENT_COLLECTION")
		os.Unsetenv("MONGODB_CNAE_COLLECTION")
		os.Unsetenv("REDIS_URI")
		os.Unsetenv("REDIS_DB")
		os.Unsetenv("REDIS_TTL")
		os.Unsetenv("WHATSAPP_API_BASE_URL")
		os.Unsetenv("WHATSAPP_API_USERNAME")
		os.Unsetenv("WHATSAPP_API_PASSWORD")
		os.Unsetenv("WHATSAPP_HSM_ID")
		os.Unsetenv("WHATSAPP_COST_CENTER_ID")
		os.Unsetenv("WHATSAPP_CAMPAIGN_NAME")
		os.Unsetenv("CF_LOOKUP_ENABLED")
	}()

	err := LoadConfig()
	if err != nil {
		t.Errorf("LoadConfig() error = %v, want nil", err)
	}

	if AppConfig == nil {
		t.Fatal("AppConfig should not be nil after LoadConfig()")
	}

	// Verify some key config values
	if AppConfig.Port != 8080 {
		t.Errorf("AppConfig.Port = %v, want 8080", AppConfig.Port)
	}

	if AppConfig.Environment != "test" {
		t.Errorf("AppConfig.Environment = %v, want test", AppConfig.Environment)
	}

	if AppConfig.CitizenCollection != "citizens" {
		t.Errorf("AppConfig.CitizenCollection = %v, want citizens", AppConfig.CitizenCollection)
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	os.Setenv("PORT", "invalid")
	defer os.Unsetenv("PORT")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid PORT")
	}

	if !strings.Contains(err.Error(), "invalid PORT") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid PORT'", err)
	}
}

func TestLoadConfig_InvalidRedisDB(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "invalid")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid REDIS_DB")
	}

	if !strings.Contains(err.Error(), "invalid REDIS_DB") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid REDIS_DB'", err)
	}
}

func TestLoadConfig_InvalidRedisTTL(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "invalid")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid REDIS_TTL")
	}

	if !strings.Contains(err.Error(), "invalid REDIS_TTL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid REDIS_TTL'", err)
	}
}

func TestLoadConfig_MissingCitizenCollection(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing MONGODB_CITIZEN_COLLECTION")
	}

	if !strings.Contains(err.Error(), "MONGODB_CITIZEN_COLLECTION") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MONGODB_CITIZEN_COLLECTION'", err)
	}
}

func TestLoadConfig_MissingMaintenanceRequestCollection(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")
	defer os.Unsetenv("MONGODB_CITIZEN_COLLECTION")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing MONGODB_MAINTENANCE_REQUEST_COLLECTION")
	}

	if !strings.Contains(err.Error(), "MONGODB_MAINTENANCE_REQUEST_COLLECTION") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MONGODB_MAINTENANCE_REQUEST_COLLECTION'", err)
	}
}

func TestLoadConfig_MissingLegalEntityCollection(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	os.Setenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION", "maintenance_requests")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")
	defer os.Unsetenv("MONGODB_CITIZEN_COLLECTION")
	defer os.Unsetenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing MONGODB_LEGAL_ENTITY_COLLECTION")
	}

	if !strings.Contains(err.Error(), "MONGODB_LEGAL_ENTITY_COLLECTION") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MONGODB_LEGAL_ENTITY_COLLECTION'", err)
	}
}

func TestLoadConfig_MissingPetCollection(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	os.Setenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION", "maintenance_requests")
	os.Setenv("MONGODB_LEGAL_ENTITY_COLLECTION", "legal_entities")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")
	defer os.Unsetenv("MONGODB_CITIZEN_COLLECTION")
	defer os.Unsetenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")
	defer os.Unsetenv("MONGODB_LEGAL_ENTITY_COLLECTION")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing MONGODB_PET_COLLECTION")
	}

	if !strings.Contains(err.Error(), "MONGODB_PET_COLLECTION") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MONGODB_PET_COLLECTION'", err)
	}
}

func TestLoadConfig_MissingPetsSelfRegisteredCollection(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	os.Setenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION", "maintenance_requests")
	os.Setenv("MONGODB_LEGAL_ENTITY_COLLECTION", "legal_entities")
	os.Setenv("MONGODB_PET_COLLECTION", "pets")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")
	defer os.Unsetenv("MONGODB_CITIZEN_COLLECTION")
	defer os.Unsetenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")
	defer os.Unsetenv("MONGODB_LEGAL_ENTITY_COLLECTION")
	defer os.Unsetenv("MONGODB_PET_COLLECTION")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing MONGODB_PETS_SELF_REGISTERED_COLLECTION")
	}

	if !strings.Contains(err.Error(), "MONGODB_PETS_SELF_REGISTERED_COLLECTION") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MONGODB_PETS_SELF_REGISTERED_COLLECTION'", err)
	}
}

func TestLoadConfig_MissingChatMemoryCollection(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	os.Setenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION", "maintenance_requests")
	os.Setenv("MONGODB_LEGAL_ENTITY_COLLECTION", "legal_entities")
	os.Setenv("MONGODB_PET_COLLECTION", "pets")
	os.Setenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION", "pets_self_registered")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")
	defer os.Unsetenv("MONGODB_CITIZEN_COLLECTION")
	defer os.Unsetenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")
	defer os.Unsetenv("MONGODB_LEGAL_ENTITY_COLLECTION")
	defer os.Unsetenv("MONGODB_PET_COLLECTION")
	defer os.Unsetenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing MONGODB_CHAT_MEMORY_COLLECTION")
	}

	if !strings.Contains(err.Error(), "MONGODB_CHAT_MEMORY_COLLECTION") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MONGODB_CHAT_MEMORY_COLLECTION'", err)
	}
}

func TestLoadConfig_MissingDepartmentCollection(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	os.Setenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION", "maintenance_requests")
	os.Setenv("MONGODB_LEGAL_ENTITY_COLLECTION", "legal_entities")
	os.Setenv("MONGODB_PET_COLLECTION", "pets")
	os.Setenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION", "pets_self_registered")
	os.Setenv("MONGODB_CHAT_MEMORY_COLLECTION", "chat_memory")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")
	defer os.Unsetenv("MONGODB_CITIZEN_COLLECTION")
	defer os.Unsetenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")
	defer os.Unsetenv("MONGODB_LEGAL_ENTITY_COLLECTION")
	defer os.Unsetenv("MONGODB_PET_COLLECTION")
	defer os.Unsetenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION")
	defer os.Unsetenv("MONGODB_CHAT_MEMORY_COLLECTION")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing MONGODB_DEPARTMENT_COLLECTION")
	}

	if !strings.Contains(err.Error(), "MONGODB_DEPARTMENT_COLLECTION") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MONGODB_DEPARTMENT_COLLECTION'", err)
	}
}

func TestLoadConfig_MissingCNAECollection(t *testing.T) {
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	os.Setenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION", "maintenance_requests")
	os.Setenv("MONGODB_LEGAL_ENTITY_COLLECTION", "legal_entities")
	os.Setenv("MONGODB_PET_COLLECTION", "pets")
	os.Setenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION", "pets_self_registered")
	os.Setenv("MONGODB_CHAT_MEMORY_COLLECTION", "chat_memory")
	os.Setenv("MONGODB_DEPARTMENT_COLLECTION", "departments")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")
	defer os.Unsetenv("MONGODB_CITIZEN_COLLECTION")
	defer os.Unsetenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")
	defer os.Unsetenv("MONGODB_LEGAL_ENTITY_COLLECTION")
	defer os.Unsetenv("MONGODB_PET_COLLECTION")
	defer os.Unsetenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION")
	defer os.Unsetenv("MONGODB_CHAT_MEMORY_COLLECTION")
	defer os.Unsetenv("MONGODB_DEPARTMENT_COLLECTION")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing MONGODB_CNAE_COLLECTION")
	}

	if !strings.Contains(err.Error(), "MONGODB_CNAE_COLLECTION") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MONGODB_CNAE_COLLECTION'", err)
	}
}

func TestLoadConfig_InvalidPhoneVerificationTTL(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("PHONE_VERIFICATION_TTL", "invalid")
	defer os.Unsetenv("PHONE_VERIFICATION_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid PHONE_VERIFICATION_TTL")
	}

	if !strings.Contains(err.Error(), "invalid PHONE_VERIFICATION_TTL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid PHONE_VERIFICATION_TTL'", err)
	}
}

func TestLoadConfig_InvalidPhoneQuarantineTTL(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("PHONE_QUARANTINE_TTL", "invalid")
	defer os.Unsetenv("PHONE_QUARANTINE_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid PHONE_QUARANTINE_TTL")
	}

	if !strings.Contains(err.Error(), "invalid PHONE_QUARANTINE_TTL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid PHONE_QUARANTINE_TTL'", err)
	}
}

func TestLoadConfig_InvalidBetaStatusCacheTTL(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("BETA_STATUS_CACHE_TTL", "invalid")
	defer os.Unsetenv("BETA_STATUS_CACHE_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid BETA_STATUS_CACHE_TTL")
	}

	if !strings.Contains(err.Error(), "invalid BETA_STATUS_CACHE_TTL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid BETA_STATUS_CACHE_TTL'", err)
	}
}

func TestLoadConfig_InvalidSelfDeclaredOutdatedThreshold(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("SELF_DECLARED_OUTDATED_THRESHOLD", "invalid")
	defer os.Unsetenv("SELF_DECLARED_OUTDATED_THRESHOLD")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid SELF_DECLARED_OUTDATED_THRESHOLD")
	}

	if !strings.Contains(err.Error(), "invalid SELF_DECLARED_OUTDATED_THRESHOLD") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid SELF_DECLARED_OUTDATED_THRESHOLD'", err)
	}
}

func TestLoadConfig_InvalidAddressCacheTTL(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("ADDRESS_CACHE_TTL", "invalid")
	defer os.Unsetenv("ADDRESS_CACHE_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid ADDRESS_CACHE_TTL")
	}

	if !strings.Contains(err.Error(), "invalid ADDRESS_CACHE_TTL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid ADDRESS_CACHE_TTL'", err)
	}
}

func TestLoadConfig_InvalidAvatarCacheTTL(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("AVATAR_CACHE_TTL", "invalid")
	defer os.Unsetenv("AVATAR_CACHE_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid AVATAR_CACHE_TTL")
	}

	if !strings.Contains(err.Error(), "invalid AVATAR_CACHE_TTL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid AVATAR_CACHE_TTL'", err)
	}
}

func TestLoadConfig_InvalidNotificationCategoryCacheTTL(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("NOTIFICATION_CATEGORY_CACHE_TTL", "invalid")
	defer os.Unsetenv("NOTIFICATION_CATEGORY_CACHE_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid NOTIFICATION_CATEGORY_CACHE_TTL")
	}

	if !strings.Contains(err.Error(), "invalid NOTIFICATION_CATEGORY_CACHE_TTL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid NOTIFICATION_CATEGORY_CACHE_TTL'", err)
	}
}

func TestLoadConfig_CFLookupEnabledWithoutMCPServer(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("CF_LOOKUP_ENABLED", "true")
	defer os.Unsetenv("CF_LOOKUP_ENABLED")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error when CF_LOOKUP_ENABLED=true but MCP_SERVER_URL is missing")
	}

	if !strings.Contains(err.Error(), "MCP_SERVER_URL is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MCP_SERVER_URL is required'", err)
	}
}

func TestLoadConfig_CFLookupEnabledWithoutMCPToken(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("CF_LOOKUP_ENABLED", "true")
	os.Setenv("MCP_SERVER_URL", "https://mcp.example.com")
	defer os.Unsetenv("CF_LOOKUP_ENABLED")
	defer os.Unsetenv("MCP_SERVER_URL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error when CF_LOOKUP_ENABLED=true but MCP_AUTH_TOKEN is missing")
	}

	if !strings.Contains(err.Error(), "MCP_AUTH_TOKEN is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'MCP_AUTH_TOKEN is required'", err)
	}
}

func TestLoadConfig_InvalidCFLookupCacheTTL(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("CF_LOOKUP_CACHE_TTL", "invalid")
	defer os.Unsetenv("CF_LOOKUP_CACHE_TTL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid CF_LOOKUP_CACHE_TTL")
	}

	if !strings.Contains(err.Error(), "invalid CF_LOOKUP_CACHE_TTL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid CF_LOOKUP_CACHE_TTL'", err)
	}
}

func TestLoadConfig_InvalidCFLookupRateLimit(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("CF_LOOKUP_RATE_LIMIT", "invalid")
	defer os.Unsetenv("CF_LOOKUP_RATE_LIMIT")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid CF_LOOKUP_RATE_LIMIT")
	}

	if !strings.Contains(err.Error(), "invalid CF_LOOKUP_RATE_LIMIT") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid CF_LOOKUP_RATE_LIMIT'", err)
	}
}

func TestLoadConfig_InvalidCFLookupGlobalRateLimit(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("CF_LOOKUP_GLOBAL_RATE_LIMIT", "invalid")
	defer os.Unsetenv("CF_LOOKUP_GLOBAL_RATE_LIMIT")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid CF_LOOKUP_GLOBAL_RATE_LIMIT")
	}

	if !strings.Contains(err.Error(), "invalid CF_LOOKUP_GLOBAL_RATE_LIMIT") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid CF_LOOKUP_GLOBAL_RATE_LIMIT'", err)
	}
}

func TestLoadConfig_InvalidCFLookupSyncTimeout(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("CF_LOOKUP_SYNC_TIMEOUT", "invalid")
	defer os.Unsetenv("CF_LOOKUP_SYNC_TIMEOUT")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid CF_LOOKUP_SYNC_TIMEOUT")
	}

	if !strings.Contains(err.Error(), "invalid CF_LOOKUP_SYNC_TIMEOUT") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid CF_LOOKUP_SYNC_TIMEOUT'", err)
	}
}

func TestLoadConfig_InvalidWhatsAppEnabled(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("WHATSAPP_ENABLED", "invalid")
	defer os.Unsetenv("WHATSAPP_ENABLED")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid WHATSAPP_ENABLED")
	}

	if !strings.Contains(err.Error(), "invalid WHATSAPP_ENABLED") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid WHATSAPP_ENABLED'", err)
	}
}

func TestLoadConfig_MissingWhatsAppBaseURL(t *testing.T) {
	setupMinimalEnv(t)
	os.Unsetenv("WHATSAPP_API_BASE_URL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing WHATSAPP_API_BASE_URL")
	}

	if !strings.Contains(err.Error(), "WHATSAPP_API_BASE_URL is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'WHATSAPP_API_BASE_URL is required'", err)
	}
}

func TestLoadConfig_MissingWhatsAppUsername(t *testing.T) {
	setupMinimalEnv(t)
	os.Unsetenv("WHATSAPP_API_USERNAME")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing WHATSAPP_API_USERNAME")
	}

	if !strings.Contains(err.Error(), "WHATSAPP_API_USERNAME is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'WHATSAPP_API_USERNAME is required'", err)
	}
}

func TestLoadConfig_MissingWhatsAppPassword(t *testing.T) {
	setupMinimalEnv(t)
	os.Unsetenv("WHATSAPP_API_PASSWORD")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing WHATSAPP_API_PASSWORD")
	}

	if !strings.Contains(err.Error(), "WHATSAPP_API_PASSWORD is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'WHATSAPP_API_PASSWORD is required'", err)
	}
}

func TestLoadConfig_MissingWhatsAppHSMID(t *testing.T) {
	setupMinimalEnv(t)
	os.Unsetenv("WHATSAPP_HSM_ID")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing WHATSAPP_HSM_ID")
	}

	if !strings.Contains(err.Error(), "WHATSAPP_HSM_ID is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'WHATSAPP_HSM_ID is required'", err)
	}
}

func TestLoadConfig_MissingWhatsAppCostCenterID(t *testing.T) {
	setupMinimalEnv(t)
	os.Unsetenv("WHATSAPP_COST_CENTER_ID")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing WHATSAPP_COST_CENTER_ID")
	}

	if !strings.Contains(err.Error(), "WHATSAPP_COST_CENTER_ID is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'WHATSAPP_COST_CENTER_ID is required'", err)
	}
}

func TestLoadConfig_MissingWhatsAppCampaignName(t *testing.T) {
	setupMinimalEnv(t)
	os.Unsetenv("WHATSAPP_CAMPAIGN_NAME")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for missing WHATSAPP_CAMPAIGN_NAME")
	}

	if !strings.Contains(err.Error(), "WHATSAPP_CAMPAIGN_NAME is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'WHATSAPP_CAMPAIGN_NAME is required'", err)
	}
}

func TestLoadConfig_InvalidIndexMaintenanceInterval(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("INDEX_MAINTENANCE_INTERVAL", "invalid")
	defer os.Unsetenv("INDEX_MAINTENANCE_INTERVAL")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error for invalid INDEX_MAINTENANCE_INTERVAL")
	}

	if !strings.Contains(err.Error(), "invalid INDEX_MAINTENANCE_INTERVAL") {
		t.Errorf("LoadConfig() error = %v, want error containing 'invalid INDEX_MAINTENANCE_INTERVAL'", err)
	}
}

func TestLoadConfig_RedisClusterWithoutAddresses(t *testing.T) {
	setupMinimalEnv(t)
	os.Setenv("REDIS_CLUSTER_ENABLED", "true")
	defer os.Unsetenv("REDIS_CLUSTER_ENABLED")

	err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should return error when REDIS_CLUSTER_ENABLED=true but REDIS_CLUSTER_ADDRS is missing")
	}

	if !strings.Contains(err.Error(), "REDIS_CLUSTER_ADDRS is required") {
		t.Errorf("LoadConfig() error = %v, want error containing 'REDIS_CLUSTER_ADDRS is required'", err)
	}
}

func TestLoadConfig_RedisClusterWithAddresses(t *testing.T) {
	originalConfig := AppConfig
	defer func() { AppConfig = originalConfig }()

	setupMinimalEnv(t)
	os.Setenv("REDIS_CLUSTER_ENABLED", "true")
	os.Setenv("REDIS_CLUSTER_ADDRS", "node1:6379,node2:6379,node3:6379")
	defer os.Unsetenv("REDIS_CLUSTER_ENABLED")
	defer os.Unsetenv("REDIS_CLUSTER_ADDRS")

	err := LoadConfig()
	if err != nil {
		t.Errorf("LoadConfig() error = %v, want nil", err)
	}

	if !AppConfig.RedisClusterEnabled {
		t.Error("AppConfig.RedisClusterEnabled should be true")
	}

	if len(AppConfig.RedisClusterAddrs) != 3 {
		t.Errorf("AppConfig.RedisClusterAddrs length = %d, want 3", len(AppConfig.RedisClusterAddrs))
	}
}

func TestLoadConfig_DefaultValues(t *testing.T) {
	originalConfig := AppConfig
	defer func() { AppConfig = originalConfig }()

	setupMinimalEnv(t)

	err := LoadConfig()
	if err != nil {
		t.Errorf("LoadConfig() error = %v, want nil", err)
	}

	// Test defaults
	if AppConfig.Port != 8080 {
		t.Errorf("Default Port = %d, want 8080", AppConfig.Port)
	}

	if AppConfig.Environment != "development" {
		t.Errorf("Default Environment = %s, want development", AppConfig.Environment)
	}

	if AppConfig.MongoURI != "mongodb://localhost:27017" {
		t.Errorf("Default MongoURI = %s, want mongodb://localhost:27017", AppConfig.MongoURI)
	}

	if AppConfig.MongoDatabase != "rmi" {
		t.Errorf("Default MongoDatabase = %s, want rmi", AppConfig.MongoDatabase)
	}

	if AppConfig.RedisURI != "localhost:6379" {
		t.Errorf("Default RedisURI = %s, want localhost:6379", AppConfig.RedisURI)
	}

	if AppConfig.RedisDB != 0 {
		t.Errorf("Default RedisDB = %d, want 0", AppConfig.RedisDB)
	}

	if AppConfig.RedisTTL != 60*time.Minute {
		t.Errorf("Default RedisTTL = %v, want 60m", AppConfig.RedisTTL)
	}

	if AppConfig.PhoneVerificationTTL != 5*time.Minute {
		t.Errorf("Default PhoneVerificationTTL = %v, want 5m", AppConfig.PhoneVerificationTTL)
	}

	if AppConfig.RedisPoolSize != 200 {
		t.Errorf("Default RedisPoolSize = %d, want 200", AppConfig.RedisPoolSize)
	}

	if AppConfig.RedisMinIdleConns != 50 {
		t.Errorf("Default RedisMinIdleConns = %d, want 50", AppConfig.RedisMinIdleConns)
	}
}

func TestLoadConfig_CustomValues(t *testing.T) {
	originalConfig := AppConfig
	defer func() { AppConfig = originalConfig }()

	setupMinimalEnv(t)
	os.Setenv("PORT", "9000")
	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("MONGODB_URI", "mongodb://prod-mongo:27017")
	os.Setenv("MONGODB_DATABASE", "prod_db")
	os.Setenv("REDIS_URI", "prod-redis:6379")
	os.Setenv("REDIS_DB", "5")
	os.Setenv("REDIS_TTL", "120m")
	os.Setenv("REDIS_POOL_SIZE", "300")
	os.Setenv("REDIS_MIN_IDLE_CONNS", "100")
	defer os.Unsetenv("PORT")
	defer os.Unsetenv("ENVIRONMENT")
	defer os.Unsetenv("MONGODB_URI")
	defer os.Unsetenv("MONGODB_DATABASE")
	defer os.Unsetenv("REDIS_URI")
	defer os.Unsetenv("REDIS_DB")
	defer os.Unsetenv("REDIS_TTL")
	defer os.Unsetenv("REDIS_POOL_SIZE")
	defer os.Unsetenv("REDIS_MIN_IDLE_CONNS")

	err := LoadConfig()
	if err != nil {
		t.Errorf("LoadConfig() error = %v, want nil", err)
	}

	if AppConfig.Port != 9000 {
		t.Errorf("Custom Port = %d, want 9000", AppConfig.Port)
	}

	if AppConfig.Environment != "production" {
		t.Errorf("Custom Environment = %s, want production", AppConfig.Environment)
	}

	if AppConfig.MongoURI != "mongodb://prod-mongo:27017" {
		t.Errorf("Custom MongoURI = %s, want mongodb://prod-mongo:27017", AppConfig.MongoURI)
	}

	if AppConfig.RedisPoolSize != 300 {
		t.Errorf("Custom RedisPoolSize = %d, want 300", AppConfig.RedisPoolSize)
	}

	if AppConfig.RedisMinIdleConns != 100 {
		t.Errorf("Custom RedisMinIdleConns = %d, want 100", AppConfig.RedisMinIdleConns)
	}
}

// setupMinimalEnv sets up the minimal required environment variables for LoadConfig
func setupMinimalEnv(t *testing.T) {
	t.Helper()
	os.Setenv("PORT", "8080")
	os.Setenv("REDIS_DB", "0")
	os.Setenv("REDIS_TTL", "60m")
	os.Setenv("MONGODB_CITIZEN_COLLECTION", "citizens")
	os.Setenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION", "maintenance_requests")
	os.Setenv("MONGODB_LEGAL_ENTITY_COLLECTION", "legal_entities")
	os.Setenv("MONGODB_PET_COLLECTION", "pets")
	os.Setenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION", "pets_self_registered")
	os.Setenv("MONGODB_CHAT_MEMORY_COLLECTION", "chat_memory")
	os.Setenv("MONGODB_DEPARTMENT_COLLECTION", "departments")
	os.Setenv("MONGODB_CNAE_COLLECTION", "cnae")
	os.Setenv("WHATSAPP_API_BASE_URL", "https://api.whatsapp.com")
	os.Setenv("WHATSAPP_API_USERNAME", "test_user")
	os.Setenv("WHATSAPP_API_PASSWORD", "test_pass")
	os.Setenv("WHATSAPP_HSM_ID", "hsm123")
	os.Setenv("WHATSAPP_COST_CENTER_ID", "cc123")
	os.Setenv("WHATSAPP_CAMPAIGN_NAME", "test_campaign")
	os.Setenv("CF_LOOKUP_ENABLED", "false")

	t.Cleanup(func() {
		os.Unsetenv("PORT")
		os.Unsetenv("REDIS_DB")
		os.Unsetenv("REDIS_TTL")
		os.Unsetenv("MONGODB_CITIZEN_COLLECTION")
		os.Unsetenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")
		os.Unsetenv("MONGODB_LEGAL_ENTITY_COLLECTION")
		os.Unsetenv("MONGODB_PET_COLLECTION")
		os.Unsetenv("MONGODB_PETS_SELF_REGISTERED_COLLECTION")
		os.Unsetenv("MONGODB_CHAT_MEMORY_COLLECTION")
		os.Unsetenv("MONGODB_DEPARTMENT_COLLECTION")
		os.Unsetenv("MONGODB_CNAE_COLLECTION")
		os.Unsetenv("WHATSAPP_API_BASE_URL")
		os.Unsetenv("WHATSAPP_API_USERNAME")
		os.Unsetenv("WHATSAPP_API_PASSWORD")
		os.Unsetenv("WHATSAPP_HSM_ID")
		os.Unsetenv("WHATSAPP_COST_CENTER_ID")
		os.Unsetenv("WHATSAPP_CAMPAIGN_NAME")
		os.Unsetenv("CF_LOOKUP_ENABLED")
	})
}
