package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"go.uber.org/zap"
)

// metricsShutdownTimeout bounds how long the metrics server may take to close on shutdown.
const metricsShutdownTimeout = 5 * time.Second

func main() {
	// Load configuration
	if err := config.LoadConfig(); err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Initialize logging
	if err := logging.InitLogger(); err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}

	logging.GetLogger().Info("Starting RMI Sync Service")

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
	services.InitCFRateLimiter(config.AppConfig.CFLookupGlobalRateLimit, logging.GetLogger())

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
		logging.GetLogger(),
	)

	// Start sync service
	syncService.Start()

	// Expose the worker metrics (rmi_sync_*, rmi_degraded_mode_active) on their own port.
	metricsServer := startMetricsServer(config.AppConfig.MetricsPort)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logging.GetLogger().Info("Shutdown signal received")

	// Stop sync service
	syncService.Stop()

	stopMetricsServer(metricsServer)

	logging.GetLogger().Info("RMI Sync Service stopped")
}

// startMetricsServer serves GET /metrics on the given port. A listener failure is logged and
// does not stop the worker: syncing keeps going without metrics.
func startMetricsServer(port int) *http.Server {
	server, err := observability.NewMetricsServer(port)
	if err != nil {
		logging.GetLogger().Error("metrics server disabled", zap.Error(err))
		return nil
	}
	if server == nil {
		logging.GetLogger().Info("metrics server disabled (METRICS_PORT=0)")
		return nil
	}

	go func() {
		logging.GetLogger().Info("metrics server listening", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.GetLogger().Error("metrics server failed", zap.Error(err))
		}
	}()

	return server
}

// stopMetricsServer closes the metrics server, if it was started.
func stopMetricsServer(server *http.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), metricsShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logging.GetLogger().Warn("metrics server shutdown failed", zap.Error(err))
	}
}
