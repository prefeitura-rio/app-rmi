package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

// authMatrixRouter wires the real chain used by the protected citizen routes:
// AuthMiddleware decodes the bearer token, then RequireOwnCPF or RequireAdmin decides.
func authMatrixRouter() *gin.Engine {
	router := gin.New()
	protected := router.Group("/v1", AuthMiddleware())
	protected.GET("/citizen/:cpf", RequireOwnCPF(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"cpf": c.Param("cpf")})
	})
	protected.GET("/admin/stats", RequireAdmin(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return router
}

func TestAuthorizationMatrix_CitizenRoutes(t *testing.T) {
	withTrustedServiceClients(t, "superapp.apps.rio.gov.br")
	origAdminGroup := config.AppConfig.AdminGroup
	config.AppConfig.AdminGroup = "go:admin"
	t.Cleanup(func() { config.AppConfig.AdminGroup = origAdminGroup })

	adminGroupClaims := models.JWTClaims{PreferredUsername: trustedTestOtherCPF}
	adminGroupClaims.RealmAccess.Roles = []string{"go:admin"}
	rmiAdminClaims := models.JWTClaims{PreferredUsername: trustedTestOtherCPF}
	rmiAdminClaims.RealmAccess.Roles = []string{"rmi-admin"}
	plainRoleClaims := models.JWTClaims{PreferredUsername: trustedTestOtherCPF}
	plainRoleClaims.RealmAccess.Roles = []string{"offline_access", "uma_authorization"}

	cases := []struct {
		name        string
		token       string // empty: no Authorization header
		path        string
		wantCitizen int
		wantAdmin   int
	}{
		{
			name:        "citizen reading own CPF",
			token:       createTestJWT(*citizenLoginClaims("superapp", trustedTestCitizenCPF)),
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusOK,
			wantAdmin:   http.StatusForbidden,
		},
		{
			name:        "citizen reading a third party CPF",
			token:       createTestJWT(*citizenLoginClaims("superapp", trustedTestCitizenCPF)),
			path:        "/v1/citizen/" + trustedTestOtherCPF,
			wantCitizen: http.StatusForbidden,
			wantAdmin:   http.StatusForbidden,
		},
		{
			name:        "citizen login token from the trusted client is not a service account",
			token:       createTestJWT(*citizenLoginClaims("superapp.apps.rio.gov.br", trustedTestCitizenCPF)),
			path:        "/v1/citizen/" + trustedTestOtherCPF,
			wantCitizen: http.StatusForbidden,
			wantAdmin:   http.StatusForbidden,
		},
		{
			name:        "user with unrelated realm roles",
			token:       createTestJWT(plainRoleClaims),
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusForbidden,
			wantAdmin:   http.StatusForbidden,
		},
		{
			name:        "admin group",
			token:       createTestJWT(adminGroupClaims),
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusOK,
			wantAdmin:   http.StatusOK,
		},
		{
			name:        "rmi-admin role",
			token:       createTestJWT(rmiAdminClaims),
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusOK,
			wantAdmin:   http.StatusOK,
		},
		{
			name: "service account of a trusted client (preferred_username)",
			token: createTestJWT(models.JWTClaims{
				AZP:               "superapp.apps.rio.gov.br",
				PreferredUsername: "service-account-superapp.apps.rio.gov.br",
			}),
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusOK,
			wantAdmin:   http.StatusOK,
		},
		{
			name: "service account of a trusted client (client_id claim)",
			token: createTestJWT(models.JWTClaims{
				AZP:      "superapp.apps.rio.gov.br",
				ClientID: "superapp.apps.rio.gov.br",
			}),
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusOK,
			wantAdmin:   http.StatusOK,
		},
		{
			name: "service account of a client that is not trusted",
			token: createTestJWT(models.JWTClaims{
				AZP:               "outro-servico",
				PreferredUsername: "service-account-outro-servico",
				ClientID:          "outro-servico",
			}),
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusForbidden,
			wantAdmin:   http.StatusForbidden,
		},
		{
			name:        "no token",
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusUnauthorized,
			wantAdmin:   http.StatusUnauthorized,
		},
		{
			name:        "malformed token",
			token:       "not-a-jwt",
			path:        "/v1/citizen/" + trustedTestCitizenCPF,
			wantCitizen: http.StatusUnauthorized,
			wantAdmin:   http.StatusUnauthorized,
		},
	}

	router := authMatrixRouter()
	do := func(path, token string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := do(tc.path, tc.token); got != tc.wantCitizen {
				t.Errorf("GET %s status = %d, want %d", tc.path, got, tc.wantCitizen)
			}
			if got := do("/v1/admin/stats", tc.token); got != tc.wantAdmin {
				t.Errorf("GET /v1/admin/stats status = %d, want %d", got, tc.wantAdmin)
			}
		})
	}
}

func TestRequireOwnCPF_WithoutAuthMiddlewareIsUnauthorized(t *testing.T) {
	router := gin.New()
	router.GET("/citizen/:cpf", RequireOwnCPF(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/citizen/"+trustedTestCitizenCPF, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("RequireOwnCPF() without claims status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
