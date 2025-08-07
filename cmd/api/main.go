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
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"

	_ "github.com/prefeitura-rio/app-rmi/docs"
)

// @title API RMI
// @version 1.0
// @description API para gerenciamento de dados de cidadãos do Rio de Janeiro, incluindo autodeclaração de informações e verificação de contato.
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
// @tag.description Operações relacionadas a cidadãos, incluindo consulta e atualização de dados autodeclarados

// @tag.name health
// @tag.description Operações de verificação de saúde da API

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

	// Initialize services
	phoneMappingService := services.NewPhoneMappingService(observability.Logger())
	configService := services.NewConfigService()

	// Initialize handlers
	phoneHandlers := handlers.NewPhoneHandlers(observability.Logger(), phoneMappingService, configService)

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
			citizen.GET("/:cpf/wallet", middleware.RequireOwnCPF(), handlers.GetCitizenWallet)
			citizen.GET("/:cpf/maintenance-request", middleware.RequireOwnCPF(), handlers.GetMaintenanceRequests)
			citizen.PUT("/:cpf/address", middleware.RequireOwnCPF(), handlers.UpdateSelfDeclaredAddress)
			citizen.PUT("/:cpf/phone", middleware.RequireOwnCPF(), handlers.UpdateSelfDeclaredPhone)
			citizen.PUT("/:cpf/email", middleware.RequireOwnCPF(), handlers.UpdateSelfDeclaredEmail)
			citizen.PUT("/:cpf/ethnicity", middleware.RequireOwnCPF(), handlers.UpdateSelfDeclaredRaca)
			citizen.GET("/:cpf/firstlogin", middleware.RequireOwnCPF(), handlers.GetFirstLogin)
			citizen.PUT("/:cpf/firstlogin", middleware.RequireOwnCPF(), handlers.UpdateFirstLogin)
			citizen.GET("/:cpf/optin", middleware.RequireOwnCPF(), handlers.GetOptIn)
			citizen.PUT("/:cpf/optin", middleware.RequireOwnCPF(), handlers.UpdateOptIn)
			citizen.POST("/:cpf/phone/validate", middleware.RequireOwnCPF(), handlers.ValidatePhoneVerification)
		}

		// Public citizen endpoints (no auth required)
		public := v1.Group("/citizen")
		{
			public.GET("/ethnicity/options", handlers.GetEthnicityOptions)
		}

		// Endpoint de validação de telefone
		v1.POST("/validate/phone", handlers.ValidatePhoneNumber)

		// Phone routes (public)
		phoneGroup := v1.Group("/phone")
		{
			phoneGroup.GET("/:phone_number/status", phoneHandlers.GetPhoneStatus)
		}

		// Phone routes (protected)
		protectedPhoneGroup := v1.Group("/phone")
		protectedPhoneGroup.Use(middleware.AuthMiddleware())
		{
			protectedPhoneGroup.GET("/:phone_number/citizen", phoneHandlers.GetCitizenByPhone)
			protectedPhoneGroup.POST("/:phone_number/validate-registration", phoneHandlers.ValidateRegistration)
			protectedPhoneGroup.POST("/:phone_number/opt-in", phoneHandlers.OptIn)
			protectedPhoneGroup.POST("/:phone_number/opt-out", phoneHandlers.OptOut)
			protectedPhoneGroup.POST("/:phone_number/reject-registration", phoneHandlers.RejectRegistration)
			protectedPhoneGroup.POST("/:phone_number/bind", phoneHandlers.BindPhoneToCPF)
			protectedPhoneGroup.POST("/:phone_number/quarantine", phoneHandlers.QuarantinePhone)
			protectedPhoneGroup.DELETE("/:phone_number/quarantine", phoneHandlers.ReleaseQuarantine)
		}

		// Admin routes
		adminGroup := v1.Group("/admin")
		adminGroup.Use(middleware.AuthMiddleware())
		{
			adminGroup.GET("/phone/quarantined", phoneHandlers.GetQuarantinedPhones)
			adminGroup.GET("/phone/quarantine/stats", phoneHandlers.GetQuarantineStats)
		}

		// Config routes (public)
		configGroup := v1.Group("/config")
		{
			configGroup.GET("/channels", phoneHandlers.GetAvailableChannels)
			configGroup.GET("/opt-out-reasons", phoneHandlers.GetOptOutReasons)
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