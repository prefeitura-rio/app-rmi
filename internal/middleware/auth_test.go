package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

func init() {
	_ = logging.InitLogger()
	gin.SetMode(gin.TestMode)

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{
			AdminGroup: "go:admin",
		}
	}
}

func createTestJWT(claims models.JWTClaims) string {
	claimsJSON, _ := json.Marshal(claims)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Create a fake JWT (header.payload.signature)
	return "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + claimsB64 + ".fake-signature"
}

func TestAuthMiddleware_Success(t *testing.T) {
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	claims := models.JWTClaims{
		SUB:               "user123",
		ISS:               "test-issuer",
		PreferredUsername: "12345678901",
	}
	token := createTestJWT(claims)

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("AuthMiddleware() with valid token status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestAuthMiddleware_UrlEncodedJWTWithUnderscores(t *testing.T) {
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		claims, exists := c.Get("claims")
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "no claims"})
			return
		}
		jwtClaims := claims.(*models.JWTClaims)
		c.JSON(http.StatusOK, gin.H{
			"preferred_username": jwtClaims.PreferredUsername,
			"name":               jwtClaims.Name,
		})
	})

	token := "eyJhbGciOiJSUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICJMWUFNbUUwVUZBd1hyUXJGbEgwSlNrMmtCR3FSblFiMDFWRG55a3R1UE5NIn0.eyJqdGkiOiJmNTlhODQyZC04MmVmLTRlYzAtYmIzZS1jODU0YTVkMzg1YWUiLCJleHAiOjE3ODk4MzA0MDYsIm5iZiI6MCwiaWF0IjoxNzg5NzQ0NTQ3LCJpc3MiOiJodHRwczovL2F1dGgtaWRyaW9ob20uYXBwcy5yaW8uZ292LmJyL2F1dGgvcmVhbG1zL2lkcmlvX2NpZGFkYW8iLCJhdWQiOlsiYnJva2VyIiwiYWNjb3VudCJdLCJzdWIiOiI0M2E1MTg3Ni1hZWQwLTQwMjMtODFiMy05YmQ5Y2JhYzk1YmQiLCJ0eXAiOiJCZWFyZXIiLCJhenAiOiJzdXBlcmFwcCIsIm5vbmNlIjoiZDMyOGE0YTMtM2I1Ny00MDI5LWIwOWMtODBmMzIwMDdhY2VlIiwiYXV0aF90aW1lIjoxNzg5NzQ0MDA2LCJzZXNzaW9uX3N0YXRlIjoiOWRkNTIxNTAtNDc5NS00MmExLTk0MmUtZTIwNDEzM2U3OGZlIiwiYWNyIjoiMSIsImFsbG93ZWQtb3JpZ2lucyI6WyJodHRwczovL3VzZmV6azM5YzVoci5zaGFyZS56cm9rLmlvIiwiaHR0cHM6Ly9zdGFnaW5nLnBlcXVlbm9zY2FyaW9jYXMuZGFkb3MucmlvIiwiaHR0cHM6Ly9wZXF1ZW5vc2NhcmlvY2FzLmRhZG9zLnJpbyIsImh0dHBzOi8vYWRtaW4uc3RhZ2luZy5hcHAuZGFkb3MucmlvIiwiaHR0cDovL2xvY2FsaG9zdDozMDAxIiwiaHR0cDovL2xvY2FsaG9zdDo4MDAwIiwiaHR0cHM6Ly9wcmVmLnJpbyIsImh0dHA6Ly9sb2NhbGhvc3Q6MzAwMCIsImh0dHBzOi8vYzg2YzZjYTMwOWE4Lm5ncm9rLWZyZWUuYXBwIiwiaHR0cHM6Ly9zdGFnaW5nLmFwcC5kYWRvcy5yaW8iXSwicmVhbG1fYWNjZXNzIjp7InJvbGVzIjpbIm9mZmxpbmVfYWNjZXNzIiwiY2FyaW9jYS1yaW8iLCJ1bWFfYXV0aG9yaXphdGlvbiIsInVzZXIiXX0sInJlc291cmNlX2FjY2VzcyI6eyJicm9rZXIiOnsicm9sZXMiOlsicmVhZC10b2tlbiJdfSwiYWNjb3VudCI6eyJyb2xlcyI6WyJtYW5hZ2UtYWNjb3VudCIsIm1hbmFnZS1hY2NvdW50LWxpbmtzIiwidmlldy1wcm9maWxlIl19fSwic2NvcGUiOiJwaG9uZSBhZGRyZXNzIHByb2ZpbGUgZW1haWwiLCJhZGRyZXNzIjp7fSwiZW1haWxfdmVyaWZpZWQiOnRydWUsIm5hbWUiOiJKb8OjbyBTaWx2YSIsInBob25lX251bWJlciI6IjIxOTk3MDE1MTI4IiwicHJlZmVycmVkX3VzZXJuYW1lIjoiMDI5MjkzNjcwMjQiLCJnaXZlbl9uYW1lIjoiSm_Do28iLCJmYW1pbHlfbmFtZSI6IlNpbHZhIiwiZW1haWwiOiJsdWNhc3RhdmFyZXN0dEBnbWFpbC5jb20ifQ.fake-signature"

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("AuthMiddleware() failed for valid URL-encoded JWT: status = %v, want %v (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAuthMiddleware_NoAuthHeader(t *testing.T) {
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("AuthMiddleware() with no auth header status = %v, want %v", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddleware_InvalidHeaderFormat(t *testing.T) {
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	tests := []struct {
		name   string
		header string
	}{
		{"no Bearer prefix", "token123"},
		{"wrong prefix", "Basic token123"},
		{"extra parts", "Bearer token1 token2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", tt.header)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("AuthMiddleware() with %s status = %v, want %v", tt.name, w.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	router := gin.New()
	router.Use(AuthMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	tests := []struct {
		name  string
		token string
	}{
		{"not JWT format", "not.a.jwt"},
		{"invalid base64", "header.!!!invalid!!!.signature"},
		{"empty token", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("AuthMiddleware() with %s status = %v, want %v", tt.name, w.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestRequireAdmin_Success(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "12345678901",
		}
		claims.ResourceAccess = map[string]models.ClientAccess{
			"superapp.apps.rio.gov.br": {Roles: []string{"go:admin"}},
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireAdmin())
	router.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access"})
	})

	req, _ := http.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireAdmin() with admin role status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequireAdmin_NoClaims(t *testing.T) {
	router := gin.New()
	router.Use(RequireAdmin())
	router.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access"})
	})

	req, _ := http.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("RequireAdmin() with no claims status = %v, want %v", w.Code, http.StatusUnauthorized)
	}
}

func TestRequireAdmin_NotAdmin(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "12345678901",
		}
		claims.ResourceAccess = map[string]models.ClientAccess{
			"superapp.apps.rio.gov.br": {Roles: []string{"user"}},
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireAdmin())
	router.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access"})
	})

	req, _ := http.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("RequireAdmin() without admin role status = %v, want %v", w.Code, http.StatusForbidden)
	}
}

func TestRequireOwnCPF_OwnData(t *testing.T) {
	config.AppConfig.AdminGroup = "go:admin"
	const ownCPF = "03561350712"

	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: ownCPF,
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireOwnCPF())
	router.GET("/citizen/:cpf/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "own data"})
	})

	req, _ := http.NewRequest("GET", "/citizen/"+ownCPF+"/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireOwnCPF() accessing own data status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequireOwnCPF_OtherData(t *testing.T) {
	config.AppConfig.AdminGroup = "go:admin"
	const ownCPF = "03561350712"
	const otherCPF = "45049725810"

	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: ownCPF,
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireOwnCPF())
	router.GET("/citizen/:cpf/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "other data"})
	})

	req, _ := http.NewRequest("GET", "/citizen/"+otherCPF+"/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("RequireOwnCPF() accessing other data status = %v, want %v", w.Code, http.StatusForbidden)
	}
}

func TestRequireOwnCPF_AdminAccess(t *testing.T) {
	config.AppConfig.AdminGroup = "go:admin"
	const ownCPF = "03561350712"
	const otherCPF = "45049725810"

	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: ownCPF,
		}
		claims.ResourceAccess = map[string]models.ClientAccess{
			"superapp.apps.rio.gov.br": {Roles: []string{"go:admin"}},
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireOwnCPF())
	router.GET("/citizen/:cpf/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access to other data"})
	})

	req, _ := http.NewRequest("GET", "/citizen/"+otherCPF+"/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireOwnCPF() admin accessing other data status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequireOwnCPF_RmiAdminRole(t *testing.T) {
	origAdmin := config.AppConfig.AdminGroup
	config.AppConfig.AdminGroup = "heimdall-admin"
	defer func() { config.AppConfig.AdminGroup = origAdmin }()

	const ownCPF = "03561350712"
	const otherCPF = "45049725810"

	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: ownCPF,
		}
		claims.RealmAccess.Roles = []string{"rmi-admin"}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireOwnCPF())
	router.GET("/citizen/:cpf/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "rmi-admin access"})
	})

	req, _ := http.NewRequest("GET", "/citizen/"+otherCPF+"/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireOwnCPF() with rmi-admin role status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequireOwnCPF_TrustedServiceClient(t *testing.T) {
	origTrusted := config.AppConfig.TrustedServiceClients
	config.AppConfig.TrustedServiceClients = []string{"superapp.apps.rio.gov.br"}
	defer func() { config.AppConfig.TrustedServiceClients = origTrusted }()

	const otherCPF = "45049725810"

	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "service-account-superapp",
			AZP:               "superapp.apps.rio.gov.br",
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireOwnCPF())
	router.GET("/citizen/:cpf/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "trusted service access"})
	})

	req, _ := http.NewRequest("GET", "/citizen/"+otherCPF+"/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireOwnCPF() with trusted service azp status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequireAdmin_RmiAdminRole(t *testing.T) {
	origAdmin := config.AppConfig.AdminGroup
	config.AppConfig.AdminGroup = "heimdall-admin"
	defer func() { config.AppConfig.AdminGroup = origAdmin }()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "admin_user",
		}
		claims.RealmAccess.Roles = []string{"rmi-admin"}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireAdmin())
	router.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access"})
	})

	req, _ := http.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireAdmin() with rmi-admin role status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequireAdmin_TrustedServiceClient(t *testing.T) {
	origTrusted := config.AppConfig.TrustedServiceClients
	config.AppConfig.TrustedServiceClients = []string{"superapp.apps.rio.gov.br"}
	defer func() { config.AppConfig.TrustedServiceClients = origTrusted }()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "service-account-superapp",
			AZP:               "superapp.apps.rio.gov.br",
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireAdmin())
	router.GET("/admin", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access"})
	})

	req, _ := http.NewRequest("GET", "/admin", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireAdmin() with trusted service client status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestExtractCPFFromToken_Success(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	claims := &models.JWTClaims{
		PreferredUsername: "12345678901",
	}
	c.Set("claims", claims)

	cpf, err := ExtractCPFFromToken(c)
	if err != nil {
		t.Errorf("ExtractCPFFromToken() error = %v, want nil", err)
	}

	if cpf != "12345678901" {
		t.Errorf("ExtractCPFFromToken() cpf = %v, want 12345678901", cpf)
	}
}

func TestExtractCPFFromToken_NoClaims(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, err := ExtractCPFFromToken(c)
	if err == nil {
		t.Error("ExtractCPFFromToken() with no claims should return error")
	}
}

func TestIsAdmin_True(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	claims := &models.JWTClaims{
		PreferredUsername: "12345678901",
	}
	claims.RealmAccess.Roles = []string{"go:admin"}
	c.Set("claims", claims)

	isAdmin, err := IsAdmin(c)
	if err != nil {
		t.Errorf("IsAdmin() error = %v, want nil", err)
	}

	if !isAdmin {
		t.Error("IsAdmin() = false, want true")
	}
}

func TestIsAdmin_False(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	claims := &models.JWTClaims{
		PreferredUsername: "12345678901",
	}
	claims.RealmAccess.Roles = []string{"user"}
	c.Set("claims", claims)

	isAdmin, err := IsAdmin(c)
	if err != nil {
		t.Errorf("IsAdmin() error = %v, want nil", err)
	}

	if isAdmin {
		t.Error("IsAdmin() = true, want false")
	}
}

func TestIsAdmin_NoClaims(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	_, err := IsAdmin(c)
	if err == nil {
		t.Error("IsAdmin() with no claims should return error")
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"a smaller", 1, 2, 1},
		{"b smaller", 5, 3, 3},
		{"equal", 4, 4, 4},
		{"negative", -1, 0, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := min(tt.a, tt.b); got != tt.want {
				t.Errorf("min(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestRequireSalesforceWebhookClient_AllowedAZP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{SalesforceWebhookClients: []string{"salesforce-rmi"}}
	defer func() { config.AppConfig = prev }()

	r := gin.New()
	r.POST("/hook", func(c *gin.Context) {
		c.Set("claims", &models.JWTClaims{AZP: "salesforce-rmi"})
		c.Next()
	}, RequireSalesforceWebhookClient(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hook", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestRequireSalesforceWebhookClient_ForbiddenAZP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{SalesforceWebhookClients: []string{"salesforce-rmi"}}
	defer func() { config.AppConfig = prev }()

	r := gin.New()
	r.POST("/hook", func(c *gin.Context) {
		c.Set("claims", &models.JWTClaims{AZP: "superapp"})
		c.Next()
	}, RequireSalesforceWebhookClient(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hook", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestRequireSalesforceWebhookClient_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{}
	defer func() { config.AppConfig = prev }()

	r := gin.New()
	r.POST("/hook", func(c *gin.Context) {
		c.Set("claims", &models.JWTClaims{AZP: "salesforce-rmi"})
		c.Next()
	}, RequireSalesforceWebhookClient(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hook", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestRequireSalesforceWebhookClient_NoClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{SalesforceWebhookClients: []string{"salesforce-rmi"}}
	defer func() { config.AppConfig = prev }()

	r := gin.New()
	r.POST("/hook", RequireSalesforceWebhookClient(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hook", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
