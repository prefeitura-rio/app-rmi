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
	"github.com/prefeitura-rio/app-rmi/internal/utils"
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

	// Initialize metrics system
	observability.InitMetrics()
	defer observability.ShutdownMetrics()

	// Initialize database connections
	config.InitMongoDB()
	config.InitRedis()

	// Initialize audit worker for asynchronous audit logging
	// This prevents connection pool exhaustion from audit operations
	utils.InitAuditWorker(config.AppConfig.AuditWorkerCount, config.AppConfig.AuditBufferSize)

	// Initialize verification queue for asynchronous phone verification
	verificationQueue := services.NewVerificationQueue(config.AppConfig.VerificationWorkerCount, config.AppConfig.VerificationQueueSize)

	// Initialize performance monitor
	_ = services.GetGlobalMonitor()

	// Initialize services
	phoneMappingService := services.NewPhoneMappingService(observability.Logger())
	configService := services.NewConfigService()
	betaGroupService := services.NewBetaGroupService(observability.Logger())

	// Initialize address service for maintenance request addresses
	services.InitAddressService()

	// Initialize avatar service for profile pictures
	services.InitAvatarService()

	// Initialize CF rate limiter for CF lookup requests
	services.InitCFRateLimiter(config.AppConfig.CFLookupGlobalRateLimit, observability.Logger())

	// Initialize CF lookup service for automatic Clínica da Família lookup
	services.InitCFLookupService()

	// Initialize handlers
	phoneHandlers := handlers.NewPhoneHandlers(observability.Logger(), phoneMappingService, configService)
	betaGroupHandlers := handlers.NewBetaGroupHandlers(observability.Logger(), betaGroupService)

	// Set Gin mode to reduce verbose route logging
	gin.SetMode(gin.ReleaseMode)

	// Create router with middleware
	router := gin.New()
	router.Use(
		gin.Recovery(),
		middleware.RequestID(),
		middleware.RequestTiming(), // Add comprehensive timing middleware
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

		// Metrics endpoint (no auth required) - for Prometheus scraping
		v1.GET("/metrics", handlers.MetricsHandler)

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

			// Avatar endpoints
			citizen.GET("/:cpf/avatar", middleware.RequireOwnCPF(), handlers.GetUserAvatar)
			citizen.PUT("/:cpf/avatar", middleware.RequireOwnCPF(), handlers.UpdateUserAvatar)
		}

		// Public citizen endpoints (no auth required)
		public := v1.Group("/citizen")
		{
			public.GET("/ethnicity/options", handlers.GetEthnicityOptions)
		}

		// Public avatar endpoints (no auth required)
		avatars := v1.Group("/avatars")
		{
			avatars.GET("", handlers.ListAvatars) // Public avatar listing with pagination
		}

		// Admin-only avatar management endpoints
		avatarAdmin := v1.Group("/avatars")
		avatarAdmin.Use(middleware.AuthMiddleware(), middleware.RequireAdmin())
		{
			avatarAdmin.POST("", handlers.CreateAvatar)       // Create new avatar
			avatarAdmin.DELETE("/:id", handlers.DeleteAvatar) // Delete avatar
		}

		// Public validation endpoints (no auth required)
		validationGroup := v1.Group("/validate")
		{
			validationGroup.POST("/phone", handlers.ValidatePhoneNumber)
			validationGroup.POST("/email", handlers.ValidateEmailAddress)
		}

		// Phone routes (public)
		phoneGroup := v1.Group("/phone")
		{
			phoneGroup.GET("/:phone_number/status", phoneHandlers.GetPhoneStatus)
			phoneGroup.GET("/:phone_number/beta-status", betaGroupHandlers.GetBetaStatus)
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
		adminGroup.Use(middleware.AuthMiddleware(), middleware.RequireAdmin())
		{
			adminGroup.GET("/phone/quarantined", phoneHandlers.GetQuarantinedPhones)
			adminGroup.GET("/phone/quarantine/stats", phoneHandlers.GetQuarantineStats)

			// Beta group management
			adminGroup.GET("/beta/groups", betaGroupHandlers.ListGroups)
			adminGroup.POST("/beta/groups", betaGroupHandlers.CreateGroup)
			adminGroup.GET("/beta/groups/:group_id", betaGroupHandlers.GetGroup)
			adminGroup.PUT("/beta/groups/:group_id", betaGroupHandlers.UpdateGroup)
			adminGroup.DELETE("/beta/groups/:group_id", betaGroupHandlers.DeleteGroup)

			// Beta whitelist management
			adminGroup.GET("/beta/whitelist", betaGroupHandlers.ListWhitelistedPhones)
			adminGroup.POST("/beta/whitelist/:phone_number", betaGroupHandlers.AddToWhitelist)
			adminGroup.DELETE("/beta/whitelist/:phone_number", betaGroupHandlers.RemoveFromWhitelist)
			adminGroup.POST("/beta/whitelist/bulk-add", betaGroupHandlers.BulkAddToWhitelist)
			adminGroup.POST("/beta/whitelist/bulk-remove", betaGroupHandlers.BulkRemoveFromWhitelist)
			adminGroup.POST("/beta/whitelist/bulk-move", betaGroupHandlers.BulkMoveWhitelist)

			// Cache management
			adminGroup.POST("/cache/read", handlers.ReadCacheKey)
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

	// Stop audit worker gracefully
	if utils.GetAuditWorker() != nil {
		utils.GetAuditWorker().Stop()
	}

	// Stop verification queue gracefully
	if verificationQueue != nil {
		verificationQueue.Stop()
	}

	logging.Logger.Info("server exiting")
}
