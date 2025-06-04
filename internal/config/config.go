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

	// Phone verification configuration
	PhoneVerificationTTL time.Duration `json:"phone_verification_ttl"`
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

	phoneVerificationTTL, err := time.ParseDuration(getEnvOrDefault("PHONE_VERIFICATION_TTL", "5m"))
	if err != nil {
		return fmt.Errorf("invalid PHONE_VERIFICATION_TTL: %w", err)
	}

	AppConfig = &Config{
		// Server configuration
		Port:        port,
		Environment: getEnvOrDefault("ENVIRONMENT", "development"),

		// MongoDB configuration
		MongoURI:      getEnvOrDefault("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDatabase: getEnvOrDefault("MONGODB_DATABASE", "rmi"),

		// Redis configuration
		RedisURI:      getEnvOrDefault("REDIS_URI", "redis://localhost:6379"),
		RedisPassword: getEnvOrDefault("REDIS_PASSWORD", ""),
		RedisDB:       redisDB,
		RedisTTL:      redisTTL,

		// Collection names
		CitizenCollection:      citizenCollection,
		SelfDeclaredCollection: getEnvOrDefault("MONGODB_SELF_DECLARED_COLLECTION", "self_declared"),
		PhoneVerificationCollection: getEnvOrDefault("MONGODB_PHONE_VERIFICATION_COLLECTION", "phone_verifications"),

		// Phone verification configuration
		PhoneVerificationTTL: phoneVerificationTTL,
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