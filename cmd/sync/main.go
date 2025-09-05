package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/services"
)

func main() {
	// Load configuration
	if err := config.LoadConfig(); err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize logging
	if err := logging.InitLogger(); err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}

	logging.Logger.Info("Starting RMI Sync Service")

	// Initialize Redis
	config.InitRedis()
	if config.Redis == nil {
		log.Fatal("Failed to initialize Redis client")
	}

	// Initialize MongoDB
	config.InitMongoDB()
	if config.MongoDB == nil {
		log.Fatal("Failed to initialize MongoDB")
	}

	// Initialize CF rate limiter for CF lookup requests
	services.InitCFRateLimiter(config.AppConfig.CFLookupGlobalRateLimit, logging.Logger)

	// Initialize CF lookup service for automatic Clínica da Família lookup
	services.InitCFLookupService()

	// Create sync service
	workerCount := config.AppConfig.DBWorkerCount
	if workerCount == 0 {
		workerCount = 10 // Default value
	}

	syncService := services.NewSyncService(
		config.Redis,
		config.MongoDB,
		workerCount,
		logging.Logger,
	)

	// Start sync service
	syncService.Start()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logging.Logger.Info("Shutdown signal received")

	// Stop sync service
	syncService.Stop()

	logging.Logger.Info("RMI Sync Service stopped")
}
