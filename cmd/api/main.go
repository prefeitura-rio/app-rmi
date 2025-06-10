package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/handlers"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"

	_ "github.com/prefeitura-rio/app-rmi/docs"
)

// @title           RMI API
// @version         1.0
// @description     API for managing citizen data with self-declared information. This API provides endpoints for retrieving and updating citizen information, with support for caching and data validation. Self-declared data takes precedence over base data when available.

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8080
// @BasePath  /v1

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization

// @tag.name citizen
// @tag.description Operations about citizens

// @tag.name health
// @tag.description Health check operations

func main() {
	// Initialize logger first
	if err := logging.InitLogger(); err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}

	// Load configuration
	if err := config.LoadConfig(); err != nil {
		logging.Logger.Fatal("failed to load config", zap.Error(err))
	}

	// Initialize observability
	observability.InitTracer()
	defer observability.ShutdownTracer()

	// Initialize database connections
	config.InitMongoDB()
	config.InitRedis()

	// Set Gin mode
	if config.AppConfig.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Create router with middleware
	router := gin.New()
	router.Use(
		gin.Recovery(),
		middleware.RequestID(),
		middleware.RequestLogger(),
		middleware.RequestTracker(),
		cors.Default(),
	)

	// Metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1 routes
	v1 := router.Group("/v1")
	{
		// Health check endpoint
		v1.GET("/health", handlers.HealthCheck)
		
		v1.GET("/citizen/:cpf", handlers.GetCitizenData)
		v1.PUT("/citizen/:cpf/address", handlers.UpdateSelfDeclaredAddress)
		v1.PUT("/citizen/:cpf/phone", handlers.UpdateSelfDeclaredPhone)
		v1.PUT("/citizen/:cpf/email", handlers.UpdateSelfDeclaredEmail)
		v1.PUT("/citizen/:cpf/ethnicity", handlers.UpdateSelfDeclaredRaca)
		v1.GET("/citizen/:cpf/firstlogin", handlers.GetFirstLogin)
		v1.PUT("/citizen/:cpf/firstlogin", handlers.UpdateFirstLogin)
		v1.GET("/citizen/:cpf/optin", handlers.GetOptIn)
		v1.PUT("/citizen/:cpf/optin", handlers.UpdateOptIn)

		// Change phone validation endpoint to /citizen/:cpf/phone/validate
		v1.POST("/citizen/:cpf/phone/validate", handlers.ValidatePhoneVerification)
	}

	// Swagger documentation
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Create server with timeouts
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", config.AppConfig.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logging.Logger.Info("starting server",
			zap.Int("port", config.AppConfig.Port),
			zap.String("environment", config.AppConfig.Environment),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Logger.Fatal("failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	logging.Logger.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logging.Logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	logging.Logger.Info("server exited gracefully")
} 