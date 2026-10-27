package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testCPF = "12345678901"
const testCdUA = "TEST-UA-001"

func setupCPFSecretariaRouter(t *testing.T) (*gin.Engine, func()) {
	t.Helper()
	setupTestEnvironment()
	require.NotNil(t, services.CPFSecretariaServiceInstance, "CPFSecretariaServiceInstance must be initialized")

	_ = logging.InitLogger()
	gin.SetMode(gin.TestMode)

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AdminGroup = "go:admin"

	adminMiddleware := func(c *gin.Context) {
		claims := &models.JWTClaims{PreferredUsername: "99999999999"}
		claims.ResourceAccess = map[string]models.ClientAccess{
			"superapp.apps.rio.gov.br": {Roles: []string{"go:admin"}},
		}
		c.Set("claims", claims)
		c.Next()
	}

	r := gin.New()
	r.Use(adminMiddleware)
	r.GET("/admin/cpf-secretaria/:cpf", AdminListCPFSecretaria)
	r.POST("/admin/cpf-secretaria/:cpf", AdminAddCPFSecretaria)
	r.DELETE("/admin/cpf-secretaria/:cpf/:cd_ua", AdminRemoveCPFSecretaria)
	r.GET("/cpf-secretaria/:cpf", GetCPFSecretarias)

	cleanup := func() {
		ctx := context.Background()
		coll := config.MongoDB.Collection(config.AppConfig.CPFSecretariaCollection)
		_, _ = coll.DeleteMany(ctx, map[string]interface{}{"cpf": testCPF})
	}
	cleanup()

	return r, cleanup
}

func TestAdminListCPFSecretaria_InvalidCPF(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/admin/cpf-secretaria/123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminListCPFSecretaria_EmptyList(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/admin/cpf-secretaria/"+testCPF, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp models.CPFSecretariaListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, testCPF, resp.CPF)
	assert.Empty(t, resp.Mappings)
}

func TestAdminAddCPFSecretaria_InvalidCPF(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]string{"cd_ua": testCdUA})
	req, _ := http.NewRequest(http.MethodPost, "/admin/cpf-secretaria/123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminAddCPFSecretaria_InvalidJSON(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, "/admin/cpf-secretaria/"+testCPF, bytes.NewReader([]byte(`{not valid json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminAddCPFSecretaria_MissingCdUA(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]string{"cd_ua": "   "})
	req, _ := http.NewRequest(http.MethodPost, "/admin/cpf-secretaria/"+testCPF, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminAddCPFSecretaria_Success(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]string{"cd_ua": testCdUA})
	req, _ := http.NewRequest(http.MethodPost, "/admin/cpf-secretaria/"+testCPF, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp models.CPFSecretariaResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, testCPF, resp.CPF)
	assert.Equal(t, testCdUA, resp.CdUA)
}

func TestAdminAddCPFSecretaria_Duplicate(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	body, _ := json.Marshal(map[string]string{"cd_ua": testCdUA})
	doPost := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodPost, "/admin/cpf-secretaria/"+testCPF, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusCreated, doPost().Code)

	body, _ = json.Marshal(map[string]string{"cd_ua": testCdUA})
	w := doPost()
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAdminRemoveCPFSecretaria_InvalidCPF(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodDelete, "/admin/cpf-secretaria/123/"+testCdUA, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAdminRemoveCPFSecretaria_NotFound(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodDelete, "/admin/cpf-secretaria/"+testCPF+"/NONEXISTENT", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCPFSecretaria_FullCycle(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	addBody, _ := json.Marshal(map[string]string{"cd_ua": testCdUA})
	addReq, _ := http.NewRequest(http.MethodPost, "/admin/cpf-secretaria/"+testCPF, bytes.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	r.ServeHTTP(addW, addReq)
	require.Equal(t, http.StatusCreated, addW.Code)

	listReq, _ := http.NewRequest(http.MethodGet, "/admin/cpf-secretaria/"+testCPF, nil)
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)
	require.Equal(t, http.StatusOK, listW.Code)
	var listResp models.CPFSecretariaListResponse
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &listResp))
	assert.Len(t, listResp.Mappings, 1)
	assert.Equal(t, testCdUA, listResp.Mappings[0].CdUA)

	delReq, _ := http.NewRequest(http.MethodDelete, "/admin/cpf-secretaria/"+testCPF+"/"+testCdUA, nil)
	delW := httptest.NewRecorder()
	r.ServeHTTP(delW, delReq)
	assert.Equal(t, http.StatusNoContent, delW.Code)

	listReq2, _ := http.NewRequest(http.MethodGet, "/admin/cpf-secretaria/"+testCPF, nil)
	listW2 := httptest.NewRecorder()
	r.ServeHTTP(listW2, listReq2)
	require.Equal(t, http.StatusOK, listW2.Code)
	var listResp2 models.CPFSecretariaListResponse
	require.NoError(t, json.Unmarshal(listW2.Body.Bytes(), &listResp2))
	assert.Empty(t, listResp2.Mappings)
}

func TestGetCPFSecretarias_InvalidCPF(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/cpf-secretaria/123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetCPFSecretarias_EmptyResult(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/cpf-secretaria/"+testCPF, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp models.CPFSecretariaQueryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, testCPF, resp.CPF)
	assert.Empty(t, resp.CdUAs)
}

func TestGetCPFSecretarias_WithMappings(t *testing.T) {
	r, cleanup := setupCPFSecretariaRouter(t)
	defer cleanup()

	addBody, _ := json.Marshal(map[string]string{"cd_ua": testCdUA})
	addReq, _ := http.NewRequest(http.MethodPost, "/admin/cpf-secretaria/"+testCPF, bytes.NewReader(addBody))
	addReq.Header.Set("Content-Type", "application/json")
	addW := httptest.NewRecorder()
	r.ServeHTTP(addW, addReq)
	require.Equal(t, http.StatusCreated, addW.Code)

	req, _ := http.NewRequest(http.MethodGet, "/cpf-secretaria/"+testCPF, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp models.CPFSecretariaQueryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, testCPF, resp.CPF)
	assert.Contains(t, resp.CdUAs, testCdUA)
}

// setupCPFSecretariaQueryRouter mounts GET /cpf-secretaria/:cpf with RequireOwnCPF,
// mirroring cmd/api/main.go. claimsInjector sets JWT claims before the guard.
func setupCPFSecretariaQueryRouter(t *testing.T, claimsInjector gin.HandlerFunc) (*gin.Engine, func()) {
	t.Helper()
	setupTestEnvironment()
	require.NotNil(t, services.CPFSecretariaServiceInstance)

	_ = logging.InitLogger()
	gin.SetMode(gin.TestMode)

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	r := gin.New()
	r.Use(claimsInjector)
	r.GET("/cpf-secretaria/:cpf", middleware.RequireOwnCPF(), GetCPFSecretarias)

	cleanup := func() {
		ctx := context.Background()
		coll := config.MongoDB.Collection(config.AppConfig.CPFSecretariaCollection)
		_, _ = coll.DeleteMany(ctx, map[string]interface{}{"cpf": testCPF})
	}
	cleanup()

	return r, cleanup
}

func TestGetCPFSecretarias_OwnCPF_Allowed(t *testing.T) {
	r, cleanup := setupCPFSecretariaQueryRouter(t, func(c *gin.Context) {
		c.Set("claims", &models.JWTClaims{PreferredUsername: testCPF})
		c.Next()
	})
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/cpf-secretaria/"+testCPF, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetCPFSecretarias_ThirdPartyCPF_Forbidden(t *testing.T) {
	const ownCPF = "03561350712"
	r, cleanup := setupCPFSecretariaQueryRouter(t, func(c *gin.Context) {
		c.Set("claims", &models.JWTClaims{PreferredUsername: ownCPF})
		c.Next()
	})
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/cpf-secretaria/"+testCPF, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetCPFSecretarias_Admin_Allowed(t *testing.T) {
	origAdmin := config.AppConfig.AdminGroup
	config.AppConfig.AdminGroup = "go:admin"
	defer func() { config.AppConfig.AdminGroup = origAdmin }()

	r, cleanup := setupCPFSecretariaQueryRouter(t, func(c *gin.Context) {
		claims := &models.JWTClaims{PreferredUsername: "99999999999"}
		claims.ResourceAccess = map[string]models.ClientAccess{
			"superapp.apps.rio.gov.br": {Roles: []string{"go:admin"}},
		}
		c.Set("claims", claims)
		c.Next()
	})
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/cpf-secretaria/"+testCPF, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetCPFSecretarias_TrustedServiceAccount_Allowed(t *testing.T) {
	origTrusted := config.AppConfig.TrustedServiceClients
	config.AppConfig.TrustedServiceClients = []string{"app-go-api-service"}
	defer func() { config.AppConfig.TrustedServiceClients = origTrusted }()

	r, cleanup := setupCPFSecretariaQueryRouter(t, func(c *gin.Context) {
		c.Set("claims", &models.JWTClaims{
			PreferredUsername: "service-account-app-go-api-service",
			AZP:               "app-go-api-service",
		})
		c.Next()
	})
	defer cleanup()

	req, _ := http.NewRequest(http.MethodGet, "/cpf-secretaria/"+testCPF, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetCPFSecretarias_InternalError_NoLeak(t *testing.T) {
	r, cleanup := setupCPFSecretariaQueryRouter(t, func(c *gin.Context) {
		c.Set("claims", &models.JWTClaims{PreferredUsername: testCPF})
		c.Next()
	})
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/cpf-secretaria/"+testCPF, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "failed to retrieve secretaria mappings")
	assert.False(t, strings.Contains(strings.ToLower(body), "mongo"), "must not leak mongo details: %s", body)
	assert.False(t, strings.Contains(strings.ToLower(body), "context canceled"), "must not leak context error: %s", body)
	assert.False(t, strings.Contains(body, "cpf_secretaria:"), "must not leak internal error prefix: %s", body)
}
