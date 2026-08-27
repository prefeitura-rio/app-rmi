package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"go.uber.org/zap"
)

// ValidateAuth syncs the authenticated user with Salesforce Person Account on login.
//
// GET /v1/auth/validate
// Auth: Bearer JWT (Keycloak) — claims: preferred_username (CPF), name, email
func ValidateAuth(c *gin.Context) {
	logger := observability.Logger()

	claimsVal, ok := c.Get("claims")
	if !ok {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "missing authentication claims"})
		return
	}
	claims, ok := claimsVal.(*models.JWTClaims)
	if !ok || claims == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "invalid authentication claims"})
		return
	}

	if !services.SalesforceBaseURLConfigured() {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "salesforce integration is not configured"})
		return
	}

	bearer := extractBearerToken(c)
	if bearer == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Authorization Bearer token is required"})
		return
	}

	timeout := config.AppConfig.SalesforceTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	sfClient := clients.NewSalesforceClient(
		config.AppConfig.SalesforceBaseURL,
		timeout,
		clients.StaticBearerToken(bearer),
	)

	result, err := services.SyncSalesforceOnLogin(c.Request.Context(), sfClient, claims)
	if err != nil {
		logger.Error("salesforce auth validate failed",
			zap.String("cpf", claims.PreferredUsername),
			zap.Error(err))
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: "failed to sync citizen with salesforce"})
		return
	}

	c.JSON(http.StatusOK, result)
}
