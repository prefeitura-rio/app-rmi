package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.uber.org/zap"
)

// SalesforceWebhookRequest is the inbound payload from Salesforce:
// CPF plus the changed Person Account fields (no outbound GET required).
type SalesforceWebhookRequest struct {
	CPF   string                     `json:"cpf" binding:"required"`
	Dados *clients.SalesforceCidadao `json:"dados" binding:"required"`
}

// SalesforceWebhookResponse is returned when the sync job is accepted.
type SalesforceWebhookResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	CPF     string `json:"cpf"`
}

// HandleSalesforceCidadaoWebhook receives CPF + changed citizen data from Salesforce
// and enqueues a salesforce_sync job (origem=salesforce). No GET back to Salesforce.
//
// POST /v1/webhooks/salesforce/cidadao
// Auth: Bearer JWT issued by Keycloak for the Salesforce client (azp allowlist).
func HandleSalesforceCidadaoWebhook(c *gin.Context) {
	logger := observability.Logger()

	if !services.SalesforceBaseURLConfigured() {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "salesforce integration is not configured"})
		return
	}

	var req SalesforceWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cpf and dados are required"})
		return
	}
	cpf := strings.TrimSpace(req.CPF)
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid CPF"})
		return
	}
	if req.Dados == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "dados is required"})
		return
	}

	cidadao := req.Dados
	if strings.TrimSpace(cidadao.CPF) == "" {
		cidadao.CPF = cpf
	}

	if err := services.EnqueueSalesforceSyncJob(c.Request.Context(), config.Redis, cpf, cidadao); err != nil {
		logger.Error("failed to enqueue salesforce sync job",
			zap.String("cpf", cpf),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue sync job"})
		return
	}

	logger.Info("salesforce webhook accepted",
		zap.String("cpf", cpf),
		zap.String("account_id", cidadao.AccountID))

	c.JSON(http.StatusAccepted, SalesforceWebhookResponse{
		Status:  "accepted",
		Message: "salesforce sync job enqueued",
		CPF:     cpf,
	})
}

func extractBearerToken(c *gin.Context) string {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
