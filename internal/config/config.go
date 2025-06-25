package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration values
type Config struct {
	// Server configuration
	Port        int    `json:"port"`
	Environment string `json:"environment"`

	// MongoDB configuration
	MongoURI      string `json:"mongo_uri"`
	MongoDatabase string `json:"mongo_database"`

	// Redis configuration
	RedisURI      string        `json:"redis_uri"`
	RedisPassword string        `json:"redis_password"`
	RedisDB       int          `json:"redis_db"`
	RedisTTL      time.Duration `json:"redis_ttl"`

	// Collection names
	CitizenCollection      string `json:"mongo_citizen_collection"`
	SelfDeclaredCollection string `json:"mongo_self_declared_collection"`
	PhoneVerificationCollection string `json:"mongo_phone_verification_collection"`
	UserConfigCollection   string `json:"mongo_user_config_collection"`
	MaintenanceRequestCollection string `json:"mongo_maintenance_request_collection"`

	// Phone verification configuration
	PhoneVerificationTTL time.Duration `json:"phone_verification_ttl"`

	// WhatsApp configuration
	WhatsAppEnabled      bool   `json:"whatsapp_enabled"`
	WhatsAppBaseURL      string `json:"whatsapp_base_url"`
	WhatsAppUsername     string `json:"whatsapp_username"`
	WhatsAppPassword     string `json:"whatsapp_password"`
	WhatsAppHSMID        string `json:"whatsapp_hsm_id"`
	WhatsAppCostCenterID string `json:"whatsapp_cost_center_id"`
	WhatsAppCampaignName string `json:"whatsapp_campaign_name"`

	// Tracing configuration
	TracingEnabled  bool   `json:"tracing_enabled"`
	TracingEndpoint string `json:"tracing_endpoint"`

	// Authorization configuration
	AdminGroup string `json:"admin_group"`

	// Index maintenance configuration
	IndexMaintenanceInterval time.Duration `json:"index_maintenance_interval"`
}

var (
	AppConfig *Config
)

// LoadConfig loads configuration from environment variables
func LoadConfig() error {
	port, err := strconv.Atoi(getEnvOrDefault("PORT", "8080"))
	if err != nil {
		return fmt.Errorf("invalid PORT: %w", err)
	}

	redisDB, err := strconv.Atoi(getEnvOrDefault("REDIS_DB", "0"))
	if err != nil {
		return fmt.Errorf("invalid REDIS_DB: %w", err)
	}

	redisTTL, err := time.ParseDuration(getEnvOrDefault("REDIS_TTL", "60m"))
	if err != nil {
		return fmt.Errorf("invalid REDIS_TTL: %w", err)
	}

	// Check if MONGODB_CITIZEN_COLLECTION is set
	citizenCollection := os.Getenv("MONGODB_CITIZEN_COLLECTION")
	if citizenCollection == "" {
		return fmt.Errorf("MONGODB_CITIZEN_COLLECTION environment variable is required")
	}

	// Check if MONGODB_MAINTENANCE_REQUEST_COLLECTION is set
	maintenanceRequestCollection := os.Getenv("MONGODB_MAINTENANCE_REQUEST_COLLECTION")
	if maintenanceRequestCollection == "" {
		return fmt.Errorf("MONGODB_MAINTENANCE_REQUEST_COLLECTION environment variable is required")
	}

	phoneVerificationTTL, err := time.ParseDuration(getEnvOrDefault("PHONE_VERIFICATION_TTL", "5m"))
	if err != nil {
		return fmt.Errorf("invalid PHONE_VERIFICATION_TTL: %w", err)
	}

	// WhatsApp configuration
	whatsappEnabled := os.Getenv("WHATSAPP_ENABLED")
	if whatsappEnabled == "" {
		whatsappEnabled = "true" // Default to enabled
	}
	whatsappEnabledBool, err := strconv.ParseBool(whatsappEnabled)
	if err != nil {
		return fmt.Errorf("invalid WHATSAPP_ENABLED value: %w", err)
	}

	whatsappBaseURL := os.Getenv("WHATSAPP_API_BASE_URL")
	if whatsappBaseURL == "" {
		return fmt.Errorf("WHATSAPP_API_BASE_URL is required")
	}

	whatsappUsername := os.Getenv("WHATSAPP_API_USERNAME")
	if whatsappUsername == "" {
		return fmt.Errorf("WHATSAPP_API_USERNAME is required")
	}

	whatsappPassword := os.Getenv("WHATSAPP_API_PASSWORD")
	if whatsappPassword == "" {
		return fmt.Errorf("WHATSAPP_API_PASSWORD is required")
	}

	whatsappHSMID := os.Getenv("WHATSAPP_HSM_ID")
	if whatsappHSMID == "" {
		return fmt.Errorf("WHATSAPP_HSM_ID is required")
	}

	whatsappCostCenterID := os.Getenv("WHATSAPP_COST_CENTER_ID")
	if whatsappCostCenterID == "" {
		return fmt.Errorf("WHATSAPP_COST_CENTER_ID is required")
	}

	whatsappCampaignName := os.Getenv("WHATSAPP_CAMPAIGN_NAME")
	if whatsappCampaignName == "" {
		return fmt.Errorf("WHATSAPP_CAMPAIGN_NAME is required")
	}

	indexMaintenanceInterval, err := time.ParseDuration(getEnvOrDefault("INDEX_MAINTENANCE_INTERVAL", "1h"))
	if err != nil {
		return fmt.Errorf("invalid INDEX_MAINTENANCE_INTERVAL: %w", err)
	}

	AppConfig = &Config{
		// Server configuration
		Port:        port,
		Environment: getEnvOrDefault("ENVIRONMENT", "development"),

		// MongoDB configuration
		MongoURI:      getEnvOrDefault("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDatabase: getEnvOrDefault("MONGODB_DATABASE", "rmi"),

		// Redis configuration
		RedisURI:      getEnvOrDefault("REDIS_URI", "localhost:6379"),
		RedisPassword: getEnvOrDefault("REDIS_PASSWORD", ""),
		RedisDB:       redisDB,
		RedisTTL:      redisTTL,

		// Collection names
		CitizenCollection:      citizenCollection,
		SelfDeclaredCollection: getEnvOrDefault("MONGODB_SELF_DECLARED_COLLECTION", "self_declared"),
		PhoneVerificationCollection: getEnvOrDefault("MONGODB_PHONE_VERIFICATION_COLLECTION", "phone_verifications"),
		UserConfigCollection:   getEnvOrDefault("MONGODB_USER_CONFIG_COLLECTION", "user_config"),
		MaintenanceRequestCollection: maintenanceRequestCollection,

		// Phone verification configuration
		PhoneVerificationTTL: phoneVerificationTTL,

		// WhatsApp configuration
		WhatsAppEnabled:      whatsappEnabledBool,
		WhatsAppBaseURL:      whatsappBaseURL,
		WhatsAppUsername:     whatsappUsername,
		WhatsAppPassword:     whatsappPassword,
		WhatsAppHSMID:        whatsappHSMID,
		WhatsAppCostCenterID: whatsappCostCenterID,
		WhatsAppCampaignName: whatsappCampaignName,

		// Tracing configuration
		TracingEnabled:  getEnvOrDefault("TRACING_ENABLED", "false") == "true",
		TracingEndpoint: getEnvOrDefault("TRACING_ENDPOINT", "localhost:4317"),

		// Authorization configuration
		AdminGroup: getEnvOrDefault("ADMIN_GROUP", "rmi-admin"),

		// Index maintenance configuration
		IndexMaintenanceInterval: indexMaintenanceInterval,
	}

	return nil
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
} 