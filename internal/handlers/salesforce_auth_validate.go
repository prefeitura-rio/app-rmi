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
// @Summary Sincronizar Person Account no login
// @Description Orquestra o Person Account no Salesforce: GET → match / PATCH silencioso de email / POST create em 404 (contaOrigem Portal Pref.Rio). Usa CPF, nome e email do JWT. Não persiste dados do SF no Mongo do RMI.
// @Tags salesforce
// @Produce json
// @Success 200 {object} AuthValidateResponse
// @Failure 401 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse "Falha ao chamar Salesforce"
// @Failure 503 {object} ErrorResponse "SALESFORCE_BASE_URL não configurado"
// @Security BearerAuth
// @Router /auth/validate [get]
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
