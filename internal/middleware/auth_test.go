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
	logging.InitLogger()
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
		t.Errorf("AuthMiddleware() status = %v, want %v", w.Code, http.StatusOK)
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
		claims.ResourceAccess.Superapp.Roles = []string{"go:admin"}
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
		claims.ResourceAccess.Superapp.Roles = []string{"user"}
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
	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "12345678901",
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireOwnCPF())
	router.GET("/citizen/:cpf/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "own data"})
	})

	req, _ := http.NewRequest("GET", "/citizen/12345678901/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireOwnCPF() accessing own data status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequireOwnCPF_OtherData(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "12345678901",
		}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireOwnCPF())
	router.GET("/citizen/:cpf/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "other data"})
	})

	req, _ := http.NewRequest("GET", "/citizen/99999999999/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("RequireOwnCPF() accessing other data status = %v, want %v", w.Code, http.StatusForbidden)
	}
}

func TestRequireOwnCPF_AdminAccess(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "12345678901",
		}
		claims.ResourceAccess.Superapp.Roles = []string{"go:admin"}
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(RequireOwnCPF())
	router.GET("/citizen/:cpf/data", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "admin access to other data"})
	})

	req, _ := http.NewRequest("GET", "/citizen/99999999999/data", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequireOwnCPF() admin accessing other data status = %v, want %v", w.Code, http.StatusOK)
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
