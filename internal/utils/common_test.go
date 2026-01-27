package utils

import (
	"os"
	"sync"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"go.uber.org/zap"
)

var (
	testInitOnce  sync.Once
	testInitError error
)

// setupTestEnvironment initializes the test environment once for all tests
func setupTestEnvironment() {
	testInitOnce.Do(func() {
		// Set default environment variables for testing (only if not already set)
		// Don't override MONGODB_URI as it may contain auth credentials
		if os.Getenv("REDIS_ADDR") == "" {
			os.Setenv("REDIS_ADDR", "localhost:6379")
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

		// Initialize MongoDB/Redis if not already initialized
		if config.MongoDB == nil {
			config.InitMongoDB()
		}
		if config.Redis == nil {
			config.InitRedis()
		}

		zap.L().Info("Test environment initialized for utils package")
	})
}

// TestMain is the entry point for all tests in the utils package
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
