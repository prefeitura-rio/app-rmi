package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.uber.org/zap"
)

const (
	maxSalesforceWebhookDadosBytes   = 64 * 1024
	maxSalesforceWebhookUpdatedAtLen = 64
)

// SalesforceWebhookRequest is the inbound payload from Salesforce.
// For evento=atualizacao, dados is a required delta (same shape/types as GET /cidadao/{cpf}).
// For evento=anonimizacao, dados is optional and ignored — RMI wipes the SF mirror wholesale.
type SalesforceWebhookRequest struct {
	CPF       string          `json:"cpf" binding:"required"`
	Evento    string          `json:"evento"`
	UpdatedAt string          `json:"updatedAt"`
	Dados     json.RawMessage `json:"dados"`
}

// SalesforceWebhookResponse is returned when the sync job is accepted.
type SalesforceWebhookResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	CPF     string `json:"cpf"`
	Evento  string `json:"evento"`
}

// HandleSalesforceCidadaoWebhook receives Salesforce Person Account events and enqueues
// a salesforce_sync job (origem=salesforce). No GET back to Salesforce.
//
// @Summary Webhook Salesforce cidadão
// @Description JWT Keycloak com azp em SALESFORCE_WEBHOOK_CLIENTS. Body: cpf + evento (atualizacao default ou anonimizacao) + updatedAt opcional. Para atualizacao, dados é delta obrigatório (ausente=não alterar; null=limpar; \"\"=vazio). Para anonimizacao, dados é opcional e ignorado — o RMI limpa o espelho SF por inteiro (self_declared PII + salesforce_consentimentos; não altera citizens nem opt_in/category_opt_ins do RMI). nome/nomeSocial/dataNascimento no delta de atualizacao são ignorados. 202 com CPF mascarado; retry idempotente via updatedAt.
// @Tags salesforce
// @Accept json
// @Produce json
// @Param body body SalesforceWebhookRequestSwagger true "CPF, evento; dados obrigatório só em atualizacao"
// @Success 202 {object} SalesforceWebhookResponse "Job enfileirado"
// @Failure 400 {object} ErrorResponse "CPF inválido, dados ausentes/grandes (atualizacao), updatedAt longo ou evento inválido"
// @Failure 401 {object} ErrorResponse "JWT inválido ou ausente"
// @Failure 403 {object} ErrorResponse "azp do JWT não está em SALESFORCE_WEBHOOK_CLIENTS"
// @Failure 500 {object} ErrorResponse "Falha ao enfileirar salesforce_sync (Redis)"
// @Failure 503 {object} ErrorResponse "Integração Salesforce não configurada"
// @Security BearerAuth
// @Router /webhooks/salesforce/cidadao [post]
func HandleSalesforceCidadaoWebhook(c *gin.Context) {
	logger := observability.Logger()

	if !services.SalesforceBaseURLConfigured() {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "salesforce integration is not configured"})
		return
	}

	var req SalesforceWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cpf is required"})
		return
	}
	cpf := strings.TrimSpace(req.CPF)
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid CPF"})
		return
	}
	if len(req.Dados) > maxSalesforceWebhookDadosBytes {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "dados exceeds maximum size"})
		return
	}
	if len(strings.TrimSpace(req.UpdatedAt)) > maxSalesforceWebhookUpdatedAtLen {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "updatedAt exceeds maximum length"})
		return
	}

	evento := services.NormalizeSalesforceWebhookEvento(req.Evento)
	if evento == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "evento must be atualizacao or anonimizacao"})
		return
	}

	dados := json.RawMessage(strings.TrimSpace(string(req.Dados)))
	hasDados := len(dados) > 0 && string(dados) != "null"

	if evento == services.SalesforceWebhookEventAnonimizacao {
		// Full wipe is owned by RMI; any dados snapshot is ignored (accepted for backward compat).
		if hasDados {
			if _, err := parseSalesforceWebhookDadosObject(dados); err != nil {
				c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
				return
			}
		}
		dados = nil
	} else {
		if len(dados) == 0 {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "dados is required"})
			return
		}
		if _, err := parseSalesforceWebhookDadosObject(dados); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
	}

	if err := services.EnqueueSalesforceSyncJob(c.Request.Context(), config.Redis, cpf, evento, req.UpdatedAt, dados); err != nil {
		logger.Error("failed to enqueue salesforce sync job",
			zap.String("cpf", cpf),
			zap.String("evento", evento),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to enqueue sync job"})
		return
	}

	logger.Info("salesforce webhook accepted",
		zap.String("cpf", cpf),
		zap.String("evento", evento))

	c.JSON(http.StatusAccepted, SalesforceWebhookResponse{
		Status:  "accepted",
		Message: "salesforce sync job enqueued",
		CPF:     utils.MaskCPFForWebhookResponse(cpf),
		Evento:  evento,
	})
}

func parseSalesforceWebhookDadosObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errInvalidDados()
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, errInvalidDados()
	}
	return fields, nil
}

type webhookDadosError struct{}

func (webhookDadosError) Error() string { return "dados must be a JSON object" }

func errInvalidDados() error { return webhookDadosError{} }

func extractBearerToken(c *gin.Context) string {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
