package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func authValidateRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/auth/validate", middleware.AuthMiddleware(), ValidateAuth)
	return r
}

func withSalesforceBaseURL(t *testing.T, baseURL string) {
	t.Helper()
	prev := config.AppConfig
	cfg := *config.AppConfig
	cfg.SalesforceBaseURL = baseURL
	cfg.SalesforceTimeout = 2 * time.Second
	config.AppConfig = &cfg
	t.Cleanup(func() { config.AppConfig = prev })
}

func getAuthValidate(t *testing.T, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/validate", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	w := httptest.NewRecorder()
	authValidateRouter().ServeHTTP(w, req)
	return w
}

func TestValidateAuth_MatchForwardsTheCitizenJWT(t *testing.T) {
	var gotAuth []string
	sf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		if r.Method != http.MethodGet {
			t.Errorf("Salesforce called with %s, want GET for an existing account", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"001TEST","cpf":"` + testWebhookCPF + `","nome":"Ana"}`))
	}))
	defer sf.Close()
	withSalesforceBaseURL(t, sf.URL)

	w := getAuthValidate(t, "Bearer "+minimalJWT)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var result services.AuthValidateResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, "matched", result.Action)
	require.NotNil(t, result.Cidadao)
	assert.Equal(t, "001TEST", result.Cidadao.AccountID)

	require.NotEmpty(t, gotAuth)
	for _, h := range gotAuth {
		assert.Equal(t, "Bearer "+minimalJWT, h, "Salesforce receives the citizen's own JWT, nothing else")
	}
}

func TestValidateAuth_SalesforceFailureIsBadGateway(t *testing.T) {
	sf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"message":"down"}]}`))
	}))
	defer sf.Close()
	withSalesforceBaseURL(t, sf.URL)

	w := getAuthValidate(t, "Bearer "+minimalJWT)
	assert.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "failed to sync citizen with salesforce", body.Error)
	assert.NotContains(t, w.Body.String(), "down", "upstream error details are not echoed")
}

func TestValidateAuth_NotConfigured(t *testing.T) {
	withSalesforceBaseURL(t, "")
	w := getAuthValidate(t, "Bearer "+minimalJWT)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestValidateAuth_RequiresAuthentication(t *testing.T) {
	sf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Salesforce must not be called without authentication")
	}))
	defer sf.Close()
	withSalesforceBaseURL(t, sf.URL)

	assert.Equal(t, http.StatusUnauthorized, getAuthValidate(t, "").Code)
	assert.Equal(t, http.StatusUnauthorized, getAuthValidate(t, "Bearer not-a-jwt").Code)

	// Without AuthMiddleware there are no claims.
	r := gin.New()
	r.GET("/v1/auth/validate", ValidateAuth)
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/validate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
