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
	RedisAddr     string `json:"redis_addr"`
	RedisPassword string `json:"redis_password"`
	RedisDB       int    `json:"redis_db"`

	// Cache configuration
	CacheTTL time.Duration `json:"cache_ttl"`
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

	cacheTTL, err := time.ParseDuration(getEnvOrDefault("CACHE_TTL", "1h"))
	if err != nil {
		return fmt.Errorf("invalid CACHE_TTL: %w", err)
	}

	AppConfig = &Config{
		// Server configuration
		Port:        port,
		Environment: getEnvOrDefault("ENVIRONMENT", "development"),

		// MongoDB configuration
		MongoURI:      getEnvOrDefault("MONGODB_URI", "mongodb://localhost:27017"),
		MongoDatabase: getEnvOrDefault("MONGODB_DATABASE", "rmi"),

		// Redis configuration
		RedisAddr:     getEnvOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnvOrDefault("REDIS_PASSWORD", ""),
		RedisDB:       redisDB,

		// Cache configuration
		CacheTTL: cacheTTL,
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