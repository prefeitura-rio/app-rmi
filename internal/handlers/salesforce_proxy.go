package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.uber.org/zap"
)

// PatchSalesforceConsentimento proxies PATCH .../cidadao/{cpf}/consentimento.
//
// @Summary Atualizar consentimento no Salesforce
// @Description Proxy para PATCH /api/private/cidadao/{cpf}/consentimento. Encaminha o JWT do usuário. categoria deve ser valor de picklist SF (ex. PREF_Lembrete_Pagamento). motivo obrigatório quando acao=optout.
// @Tags salesforce
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão"
// @Param body body clients.SalesforceConsentimentoPatchRequest true "Consentimento"
// @Success 200 "Consentimento atualizado (sem corpo)"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse "CPF não pertence ao usuário autenticado"
// @Failure 404 {object} clients.SalesforceErrorBody
// @Failure 409 {object} clients.SalesforceErrorBody
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Security BearerAuth
// @Router /salesforce/cidadao/{cpf}/consentimento [patch]
func PatchSalesforceConsentimento(c *gin.Context) {
	cpf := strings.TrimSpace(c.Param("cpf"))
	sf, ok := salesforceClientFromRequest(c)
	if !ok {
		return
	}

	var req clients.SalesforceConsentimentoPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Origem) == "" {
		req.Origem = services.SalesforceContaOrigem
	}
	if strings.EqualFold(strings.TrimSpace(req.Acao), "optout") && strings.TrimSpace(req.Motivo) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "motivo is required when acao is optout"})
		return
	}

	if err := sf.PatchCidadaoConsentimento(c.Request.Context(), cpf, &req); err != nil {
		writeSalesforceProxyError(c, "patch consentimento", cpf, err)
		return
	}
	c.Status(http.StatusOK)
}

// ExportSalesforceCidadao proxies GET .../cidadao/{cpf}/exportar.
//
// @Summary Exportar dados pessoais (LGPD) do Salesforce
// @Description Proxy síncrono para GET /api/private/cidadao/{cpf}/exportar.
// @Tags salesforce
// @Produce json
// @Param cpf path string true "CPF do cidadão"
// @Success 200 {object} clients.SalesforceExportacao
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} clients.SalesforceErrorBody
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Security BearerAuth
// @Router /salesforce/cidadao/{cpf}/exportar [get]
func ExportSalesforceCidadao(c *gin.Context) {
	cpf := strings.TrimSpace(c.Param("cpf"))
	sf, ok := salesforceClientFromRequest(c)
	if !ok {
		return
	}

	out, err := sf.ExportarCidadao(c.Request.Context(), cpf)
	if err != nil {
		writeSalesforceProxyError(c, "exportar cidadao", cpf, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// AnonimizarSalesforceCidadao proxies POST .../cidadao/{cpf}/anonimizar.
//
// @Summary Solicitar anonimização no Salesforce
// @Description Proxy para POST /api/private/cidadao/{cpf}/anonimizar (assíncrono). 202 enfileirado; 409 se já houver solicitação aberta.
// @Tags salesforce
// @Produce json
// @Param cpf path string true "CPF do cidadão"
// @Success 202 {object} clients.SalesforceAnonimizacaoStatus
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} clients.SalesforceAnonimizacaoStatus "Solicitação já em aberto"
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Security BearerAuth
// @Router /salesforce/cidadao/{cpf}/anonimizar [post]
func AnonimizarSalesforceCidadao(c *gin.Context) {
	cpf := strings.TrimSpace(c.Param("cpf"))
	sf, ok := salesforceClientFromRequest(c)
	if !ok {
		return
	}

	out, err := sf.AnonimizarCidadao(c.Request.Context(), cpf)
	if err != nil {
		var apiErr *clients.SalesforceAPIError
		if errors.As(err, &apiErr) && apiErr.IsConflict() && out != nil {
			c.JSON(http.StatusConflict, out)
			return
		}
		writeSalesforceProxyError(c, "anonimizar cidadao", cpf, err)
		return
	}
	c.JSON(http.StatusAccepted, out)
}

// GetSalesforceAnonimizacao proxies GET .../anonimizacao/{numeroSolicitacao}.
//
// @Summary Consultar status de anonimização
// @Description Proxy para GET /api/private/anonimizacao/{numeroSolicitacao}.
// @Tags salesforce
// @Produce json
// @Param numeroSolicitacao path string true "Número da solicitação"
// @Success 200 {object} clients.SalesforceAnonimizacaoStatus
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} clients.SalesforceErrorBody
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Security BearerAuth
// @Router /salesforce/anonimizacao/{numeroSolicitacao} [get]
func GetSalesforceAnonimizacao(c *gin.Context) {
	numero := strings.TrimSpace(c.Param("numeroSolicitacao"))
	sf, ok := salesforceClientFromRequest(c)
	if !ok {
		return
	}

	out, err := sf.GetAnonimizacao(c.Request.Context(), numero)
	if err != nil {
		writeSalesforceProxyError(c, "get anonimizacao", numero, err)
		return
	}
	if !salesforceAnonimizacaoOwnedByCaller(c, out) {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied"})
		return
	}
	c.JSON(http.StatusOK, out)
}

// ListSalesforceChamados proxies GET .../chamados (Community User from JWT).
//
// @Summary Listar chamados do cidadão
// @Description Proxy para GET /api/private/chamados usando o JWT do usuário (Community User).
// @Tags salesforce
// @Produce json
// @Success 200 {object} clients.SalesforceChamadosList
// @Failure 401 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Security BearerAuth
// @Router /salesforce/chamados [get]
func ListSalesforceChamados(c *gin.Context) {
	sf, ok := salesforceClientFromRequest(c)
	if !ok {
		return
	}

	out, err := sf.ListChamados(c.Request.Context())
	if err != nil {
		writeSalesforceProxyError(c, "list chamados", "", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// GetSalesforceChamado proxies GET .../chamados/{protocolo}.
//
// @Summary Detalhar chamado por protocolo
// @Description Proxy para GET /api/private/chamados/{protocolo}.
// @Tags salesforce
// @Produce json
// @Param protocolo path string true "Protocolo do chamado"
// @Success 200 {object} clients.SalesforceProtocolo
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} clients.SalesforceErrorBody
// @Failure 502 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Security BearerAuth
// @Router /salesforce/chamados/{protocolo} [get]
func GetSalesforceChamado(c *gin.Context) {
	protocolo := strings.TrimSpace(c.Param("protocolo"))
	sf, ok := salesforceClientFromRequest(c)
	if !ok {
		return
	}

	out, err := sf.GetChamado(c.Request.Context(), protocolo)
	if err != nil {
		writeSalesforceProxyError(c, "get chamado", protocolo, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func salesforceClientFromRequest(c *gin.Context) (*clients.SalesforceClient, bool) {
	if !services.SalesforceBaseURLConfigured() {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "salesforce integration is not configured"})
		return nil, false
	}
	bearer := extractBearerToken(c)
	if bearer == "" {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Authorization Bearer token is required"})
		return nil, false
	}
	timeout := config.AppConfig.SalesforceTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return clients.NewSalesforceClient(
		config.AppConfig.SalesforceBaseURL,
		timeout,
		clients.StaticBearerToken(bearer),
	), true
}

func writeSalesforceProxyError(c *gin.Context, op, key string, err error) {
	logger := observability.Logger()
	var apiErr *clients.SalesforceAPIError
	if errors.As(err, &apiErr) {
		logger.Warn("salesforce proxy upstream error",
			zap.String("op", op),
			zap.String("key", key),
			zap.Int("status", apiErr.StatusCode),
			zap.Error(err))
		if len(apiErr.Errors) > 0 {
			c.JSON(apiErr.StatusCode, clients.SalesforceErrorBody{Errors: apiErr.Errors})
			return
		}
		if strings.TrimSpace(apiErr.Body) != "" && jsonLooksLikeObject(apiErr.Body) {
			c.Data(apiErr.StatusCode, "application/json", []byte(apiErr.Body))
			return
		}
		c.JSON(apiErr.StatusCode, ErrorResponse{Error: apiErr.Error()})
		return
	}
	logger.Error("salesforce proxy failed",
		zap.String("op", op),
		zap.String("key", key),
		zap.Error(err))
	c.JSON(http.StatusBadGateway, ErrorResponse{Error: "failed to call salesforce"})
}

func jsonLooksLikeObject(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[")
}

// salesforceAnonimizacaoOwnedByCaller ensures the polling user can only read their own LGPD request.
// When Salesforce returns a CPF on the status payload, it must match the authenticated user.
func salesforceAnonimizacaoOwnedByCaller(c *gin.Context, status *clients.SalesforceAnonimizacaoStatus) bool {
	if status == nil {
		return true
	}
	cpfInResponse := strings.TrimSpace(status.CPF)
	if cpfInResponse == "" {
		return true
	}
	claimsVal, ok := c.Get("claims")
	if !ok {
		return false
	}
	claims, ok := claimsVal.(*models.JWTClaims)
	if !ok || claims == nil {
		return false
	}
	if config.AppConfig != nil && claims.HasRole(config.AppConfig.AdminGroup) {
		return true
	}
	return utils.NormalizeCPF(claims.PreferredUsername) == utils.NormalizeCPF(cpfInResponse)
}
