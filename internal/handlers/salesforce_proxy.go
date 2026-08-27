package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"go.uber.org/zap"
)

// PatchSalesforceConsentimento proxies PATCH .../cidadao/{cpf}/consentimento.
//
// PATCH /v1/salesforce/cidadao/:cpf/consentimento
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
// GET /v1/salesforce/cidadao/:cpf/exportar
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
// POST /v1/salesforce/cidadao/:cpf/anonimizar
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
// GET /v1/salesforce/anonimizacao/:numeroSolicitacao
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
	c.JSON(http.StatusOK, out)
}

// ListSalesforceChamados proxies GET .../chamados (Community User from JWT).
//
// GET /v1/salesforce/chamados
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
// GET /v1/salesforce/chamados/:protocolo
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
