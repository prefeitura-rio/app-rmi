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
	
	// Redis connection pool configuration
	RedisPoolSize      int           `json:"redis_pool_size"`
	RedisMinIdleConns  int           `json:"redis_min_idle_conns"`
	RedisDialTimeout   time.Duration `json:"redis_dial_timeout"`
	RedisReadTimeout   time.Duration `json:"redis_read_timeout"`
	RedisWriteTimeout  time.Duration `json:"redis_write_timeout"`
	RedisPoolTimeout   time.Duration `json:"redis_pool_timeout"`

	// Collection names
	CitizenCollection      string `json:"mongo_citizen_collection"`
	SelfDeclaredCollection string `json:"mongo_self_declared_collection"`
	PhoneVerificationCollection string `json:"mongo_phone_verification_collection"`
	UserConfigCollection   string `json:"mongo_user_config_collection"`
	MaintenanceRequestCollection string `json:"mongo_maintenance_request_collection"`
	PhoneMappingCollection string `json:"mongo_phone_mapping_collection"`
	OptInHistoryCollection string `json:"mongo_opt_in_history_collection"`
	BetaGroupCollection    string `json:"mongo_beta_group_collection"`
	AuditLogsCollection    string `json:"mongo_audit_logs_collection"`

	// Phone verification configuration
	PhoneVerificationTTL time.Duration `json:"phone_verification_ttl"`
	PhoneQuarantineTTL   time.Duration `json:"phone_quarantine_ttl"` // 6 months
	BetaStatusCacheTTL   time.Duration `json:"beta_status_cache_ttl"`

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

	// Audit logging configuration
	AuditLogsEnabled bool `json:"audit_logs_enabled"`
	
	// Audit worker configuration
	AuditWorkerCount int `json:"audit_worker_count"`
	AuditBufferSize  int `json:"audit_buffer_size"`
	
	// Verification queue configuration
	VerificationWorkerCount int `json:"verification_worker_count"`
	VerificationQueueSize   int `json:"verification_queue_size"`

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

	phoneQuarantineTTL, err := time.ParseDuration(getEnvOrDefault("PHONE_QUARANTINE_TTL", "4320h")) // 6 months
	if err != nil {
		return fmt.Errorf("invalid PHONE_QUARANTINE_TTL: %w", err)
	}

	betaStatusCacheTTL, err := time.ParseDuration(getEnvOrDefault("BETA_STATUS_CACHE_TTL", "24h")) // 24 hours
	if err != nil {
		return fmt.Errorf("invalid BETA_STATUS_CACHE_TTL: %w", err)
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
	
		// Redis connection pool configuration
		RedisPoolSize:      getEnvAsIntOrDefault("REDIS_POOL_SIZE", 50),
		RedisMinIdleConns:  getEnvAsIntOrDefault("REDIS_MIN_IDLE_CONNS", 20),
		RedisDialTimeout:   getEnvAsDurationOrDefault("REDIS_DIAL_TIMEOUT", 2*time.Second),
		RedisReadTimeout:   getEnvAsDurationOrDefault("REDIS_READ_TIMEOUT", 1*time.Second),
		RedisWriteTimeout:  getEnvAsDurationOrDefault("REDIS_WRITE_TIMEOUT", 1*time.Second),
		RedisPoolTimeout:   getEnvAsDurationOrDefault("REDIS_POOL_TIMEOUT", 2*time.Second),

		// Collection names
		CitizenCollection:      citizenCollection,
		SelfDeclaredCollection: getEnvOrDefault("MONGODB_SELF_DECLARED_COLLECTION", "self_declared"),
		PhoneVerificationCollection: getEnvOrDefault("MONGODB_PHONE_VERIFICATION_COLLECTION", "phone_verifications"),
		UserConfigCollection:   getEnvOrDefault("MONGODB_USER_CONFIG_COLLECTION", "user_config"),
		MaintenanceRequestCollection: maintenanceRequestCollection,
		PhoneMappingCollection: getEnvOrDefault("MONGODB_PHONE_MAPPING_COLLECTION", "phone_cpf_mappings"),
			OptInHistoryCollection: getEnvOrDefault("MONGODB_OPT_IN_HISTORY_COLLECTION", "opt_in_history"),
	BetaGroupCollection:    getEnvOrDefault("MONGODB_BETA_GROUP_COLLECTION", "beta_groups"),
	AuditLogsCollection:    getEnvOrDefault("MONGODB_AUDIT_LOGS_COLLECTION", "audit_logs"),

		// Phone verification configuration
		PhoneVerificationTTL: phoneVerificationTTL,
		PhoneQuarantineTTL:   phoneQuarantineTTL,
		BetaStatusCacheTTL:   betaStatusCacheTTL,

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

			// Audit logging configuration
	AuditLogsEnabled: getEnvOrDefault("AUDIT_LOGS_ENABLED", "true") == "true",
	
	// Audit worker configuration
	AuditWorkerCount: getEnvAsIntOrDefault("AUDIT_WORKER_COUNT", 5),
	AuditBufferSize:  getEnvAsIntOrDefault("AUDIT_BUFFER_SIZE", 1000),
	
	// Verification queue configuration
	VerificationWorkerCount: getEnvAsIntOrDefault("VERIFICATION_WORKER_COUNT", 10),
	VerificationQueueSize:   getEnvAsIntOrDefault("VERIFICATION_QUEUE_SIZE", 5000),

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

// getEnvAsIntOrDefault returns environment variable value as int or default if not set
func getEnvAsIntOrDefault(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// getEnvAsDurationOrDefault returns environment variable value as time.Duration or default if not set
func getEnvAsDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
} 