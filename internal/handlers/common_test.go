package handlers

import (
	"os"
	"sync"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"go.uber.org/zap"
)

var (
	testSetupOnce sync.Once
	testInitError error
)

// setupTestEnvironment initializes the test environment once for the entire package
func setupTestEnvironment() {
	testSetupOnce.Do(func() {
		// Ensure test MongoDB URI is set (override any production values)
		mongoURI := os.Getenv("MONGODB_URI")
		if mongoURI == "" {
			mongoURI = "mongodb://localhost:27017"
			os.Setenv("MONGODB_URI", mongoURI)
		}

		// Ensure test Redis address is set
		redisAddr := os.Getenv("REDIS_ADDR")
		if redisAddr == "" {
			redisAddr = "localhost:6379"
			os.Setenv("REDIS_ADDR", redisAddr)
		}

		// Set required MongoDB collection environment variables for tests
		collections := map[string]string{
			"MONGODB_DATABASE":                          "rmi_test",
			"MONGODB_CITIZEN_COLLECTION":                "citizens",
			"MONGODB_SELF_DECLARED_COLLECTION":          "self_declared",
			"MONGODB_PHONE_MAPPING_COLLECTION":          "phone_mapping",
			"MONGODB_PHONE_VERIFICATION_COLLECTION":     "phone_verifications",
			"MONGODB_OPT_IN_HISTORY_COLLECTION":         "opt_in_history",
			"MONGODB_AUDIT_LOG_COLLECTION":              "audit_logs",
			"MONGODB_BETA_GROUPS_COLLECTION":            "beta_groups",
			"MONGODB_WHATSAPP_MEMORY_COLLECTION":        "whatsapp_memory",
			"MONGODB_AVATARS_COLLECTION":                "avatars",
			"MONGODB_MAINTENANCE_REQUEST_COLLECTION":    "maintenance_requests",
			"MONGODB_LEGAL_ENTITY_COLLECTION":           "legal_entities",
			"MONGODB_CHAT_MEMORY_COLLECTION":            "chat_memory",
			"MONGODB_CNAE_COLLECTION":                   "cnae",
			"MONGODB_DEPARTMENT_COLLECTION":             "departments",
			"MONGODB_PET_COLLECTION":                    "pets",
			"MONGODB_PETS_SELF_REGISTERED_COLLECTION":   "pets_self_registered",
			"MONGODB_NOTIFICATION_CATEGORY_COLLECTION":  "notification_categories",
			"MONGODB_USER_CONFIG_COLLECTION":            "user_config",
		}
		for key, defaultValue := range collections {
			if os.Getenv(key) == "" {
				os.Setenv(key, defaultValue)
			}
		}

		// Set other required environment variables
		if os.Getenv("JWT_ISSUER_URL") == "" {
			os.Setenv("JWT_ISSUER_URL", "http://localhost:8080")
		}
		if os.Getenv("PORT") == "" {
			os.Setenv("PORT", "8080")
		}
		if os.Getenv("CF_LOOKUP_ENABLED") == "" {
			os.Setenv("CF_LOOKUP_ENABLED", "false")
		}
		if os.Getenv("WHATSAPP_ENABLED") == "" {
			os.Setenv("WHATSAPP_ENABLED", "false")
		}

		// Initialize configuration and connections only once
		if err := config.LoadConfig(); err != nil {
			testInitError = err
			return
		}
		config.InitMongoDB()
		config.InitRedis()

		zap.L().Info("Test environment initialized for handlers package")
	})
}

// TestMain is the entry point for all tests in the handlers package
func TestMain(m *testing.M) {
	// Setup test environment once
	setupTestEnvironment()

	if testInitError != nil {
		panic(testInitError)
	}

	// Run all tests
	exitCode := m.Run()

	// Cleanup would go here if needed

	os.Exit(exitCode)
}
