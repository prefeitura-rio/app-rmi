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

// @title API RMI
// @version 1.0
// @description API para gerenciamento de dados de cidadãos do Rio de Janeiro
// @termsOfService http://swagger.io/terms/

// @contact.name Suporte RMI
// @contact.url http://www.rio.rj.gov.br
// @contact.email suporte@rio.rj.gov.br

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath /v1
// @schemes http https

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Tipo: Bearer token. Exemplo: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

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
		// Health check endpoint (no auth required)
		v1.GET("/health", handlers.HealthCheck)
		
		// Citizen endpoints (require auth)
		citizen := v1.Group("/citizen")
		citizen.Use(middleware.AuthMiddleware())
		{
			// Endpoints that require own CPF access
			citizen.GET("/:cpf", middleware.RequireOwnCPF(), handlers.GetCitizenData)
			citizen.PUT("/:cpf/address", middleware.RequireOwnCPF(), handlers.UpdateSelfDeclaredAddress)
			citizen.PUT("/:cpf/phone", middleware.RequireOwnCPF(), handlers.UpdateSelfDeclaredPhone)
			citizen.PUT("/:cpf/email", middleware.RequireOwnCPF(), handlers.UpdateSelfDeclaredEmail)
			citizen.GET("/:cpf/firstlogin", middleware.RequireOwnCPF(), handlers.GetFirstLogin)
			citizen.PUT("/:cpf/firstlogin", middleware.RequireOwnCPF(), handlers.UpdateFirstLogin)
			citizen.GET("/:cpf/optin", middleware.RequireOwnCPF(), handlers.GetOptIn)
			citizen.PUT("/:cpf/optin", middleware.RequireOwnCPF(), handlers.UpdateOptIn)
			citizen.POST("/:cpf/phone/validate", middleware.RequireOwnCPF(), handlers.ValidatePhoneVerification)
		}
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

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Create a deadline for server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		logging.Logger.Fatal("server forced to shutdown", zap.Error(err))
	}

	logging.Logger.Info("server exiting")
} 