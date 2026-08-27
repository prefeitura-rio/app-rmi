package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const otherUserCPF = "11144477735"

func salesforceProxyRouter() *gin.Engine {
	r := gin.New()
	sf := r.Group("/v1/salesforce")
	sf.Use(middleware.AuthMiddleware())
	{
		sf.PATCH("/cidadao/:cpf/consentimento", middleware.RequireOwnCPF(), PatchSalesforceConsentimento)
		sf.GET("/cidadao/:cpf/exportar", middleware.RequireOwnCPF(), ExportSalesforceCidadao)
		sf.POST("/cidadao/:cpf/anonimizar", middleware.RequireOwnCPF(), AnonimizarSalesforceCidadao)
		sf.GET("/anonimizacao/:numeroSolicitacao", GetSalesforceAnonimizacao)
		sf.GET("/chamados", ListSalesforceChamados)
		sf.GET("/chamados/:protocolo", GetSalesforceChamado)
	}
	return r
}

func withSFConfig(t *testing.T, baseURL string) {
	t.Helper()
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL: baseURL,
		SalesforceTimeout: 0,
		AdminGroup:        "go:admin",
	}
	t.Cleanup(func() { config.AppConfig = prev })
}

func TestPatchSalesforceConsentimento_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPatch, "/v1/salesforce/cidadao/"+testWebhookCPF+"/consentimento",
		bytes.NewBufferString(`{"categoria":"PREF_Lembrete_Pagamento","acao":"optin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestPatchSalesforceConsentimento_OptoutMissingMotivo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSFConfig(t, "https://sf.example.com")

	req := httptest.NewRequest(http.MethodPatch, "/v1/salesforce/cidadao/"+testWebhookCPF+"/consentimento",
		bytes.NewBufferString(`{"categoria":"PREF_Lembrete_Pagamento","acao":"optout"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "motivo")
}

func TestPatchSalesforceConsentimento_ForbiddenOtherCPF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSFConfig(t, "https://sf.example.com")

	req := httptest.NewRequest(http.MethodPatch, "/v1/salesforce/cidadao/"+otherUserCPF+"/consentimento",
		bytes.NewBufferString(`{"categoria":"PREF_Lembrete_Pagamento","acao":"optin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPatchSalesforceConsentimento_Upstream400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"DADOS_INVALIDOS","message":"categoria inválida"}]}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodPatch, "/v1/salesforce/cidadao/"+testWebhookCPF+"/consentimento",
		bytes.NewBufferString(`{"categoria":"X","acao":"optin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "DADOS_INVALIDOS")
}

func TestExportSalesforceCidadao_Upstream404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/private/cidadao/"+testWebhookCPF+"/exportar", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"code":"NAO_ENCONTRADO","message":"não encontrado"}]}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodGet, "/v1/salesforce/cidadao/"+testWebhookCPF+"/exportar", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "NAO_ENCONTRADO")
}

func TestExportSalesforceCidadao_ForbiddenOtherCPF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSFConfig(t, "https://sf.example.com")

	req := httptest.NewRequest(http.MethodGet, "/v1/salesforce/cidadao/"+otherUserCPF+"/exportar", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAnonimizarSalesforceCidadao_Conflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{
			"numeroSolicitacao":"PRIVRTBF-00000002",
			"cpf":"` + testWebhookCPF + `",
			"status":"enfileirado",
			"dataSolicitacao":"2026-07-01T19:38:17Z"
		}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodPost, "/v1/salesforce/cidadao/"+testWebhookCPF+"/anonimizar", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "PRIVRTBF-00000002")
	assert.Contains(t, w.Body.String(), "enfileirado")
}

func TestAnonimizarSalesforceCidadao_Upstream500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"code":"ERRO_INTERNO","message":"falha"}]}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodPost, "/v1/salesforce/cidadao/"+testWebhookCPF+"/anonimizar", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "ERRO_INTERNO")
}

func TestGetSalesforceAnonimizacao_Upstream404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/private/anonimizacao/PRIV-MISSING", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"code":"NAO_ENCONTRADO","message":"solicitação inexistente"}]}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodGet, "/v1/salesforce/anonimizacao/PRIV-MISSING", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "NAO_ENCONTRADO")
}

func TestListSalesforceChamados_UnauthorizedWithoutJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withSFConfig(t, "https://sf.example.com")

	req := httptest.NewRequest(http.MethodGet, "/v1/salesforce/chamados", nil)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestListSalesforceChamados_Upstream401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"code":"NAO_AUTORIZADO","message":"token inválido"}]}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodGet, "/v1/salesforce/chamados", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "NAO_AUTORIZADO")
}

func TestListSalesforceChamados_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/private/chamados", r.URL.Path)
		assert.Equal(t, "Bearer "+minimalJWT, r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"protocolos":[{"protocolo":"RIO-1"}]}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodGet, "/v1/salesforce/chamados", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "RIO-1")
}

func TestGetSalesforceChamado_Upstream404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/private/chamados/RIO-MISSING", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"code":"NAO_ENCONTRADO","message":"protocolo não encontrado"}]}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodGet, "/v1/salesforce/chamados/RIO-MISSING", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "protocolo não encontrado")
}

func TestGetSalesforceChamado_Upstream500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"code":"ERRO_INTERNO","message":"indisponível"}]}`))
	}))
	defer sfSrv.Close()
	withSFConfig(t, sfSrv.URL)

	req := httptest.NewRequest(http.MethodGet, "/v1/salesforce/chamados/RIO-1", nil)
	req.Header.Set("Authorization", "Bearer "+minimalJWT)
	w := httptest.NewRecorder()
	salesforceProxyRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "ERRO_INTERNO")
}
