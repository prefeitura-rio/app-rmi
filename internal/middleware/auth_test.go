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
	"github.com/stretchr/testify/assert"
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

// ---------------------------------------------------------------------------
// Helpers for RequireServiceAccount / RequireOwnCPFOrServiceAccount tests
// ---------------------------------------------------------------------------

// adminGroup is the value set in init() — must match config.AppConfig.AdminGroup.
const adminGroup = "go:admin"

// buildRouter creates a gin.Engine with the given middleware chain and a dummy
// GET /test handler that always returns 200. The optional routePath lets callers
// register a parameterised route (e.g. "/citizen/:cpf/data").
func buildRouter(routePath string, middlewares ...gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	path := routePath
	if path == "" {
		path = "/test"
	}
	r.GET(path, append(middlewares, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})...)
	return r
}

// injectClaims returns a gin.HandlerFunc that stores the given claims in the
// context under the "claims" key, mimicking what AuthMiddleware does.
func injectClaims(claims *models.JWTClaims) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("claims", claims)
		c.Next()
	}
}

// makeAdminClaims returns claims for a human admin (has adminGroup in RealmAccess.Roles).
func makeAdminClaims() *models.JWTClaims {
	c := &models.JWTClaims{
		PreferredUsername: "admin-user",
		AZP:               "some-client",
	}
	c.RealmAccess.Roles = []string{adminGroup}
	return c
}

// makeSAClaims returns claims for a Keycloak service account from clientID.
func makeSAClaims(clientID string) *models.JWTClaims {
	return &models.JWTClaims{
		PreferredUsername: "service-account-" + clientID,
		AZP:               clientID,
	}
}

// makeUserClaims returns claims for a regular (non-SA, non-admin) user.
func makeUserClaims(preferredUsername string) *models.JWTClaims {
	return &models.JWTClaims{
		PreferredUsername: preferredUsername,
		AZP:               "some-client",
	}
}

// ---------------------------------------------------------------------------
// RequireServiceAccount tests
// ---------------------------------------------------------------------------

// TestRequireServiceAccount_AdminCaller verifies that a caller with the admin
// role in realm_access.roles is always allowed through, regardless of azp.
func TestRequireServiceAccount_AdminCaller(t *testing.T) {
	// Arrange
	router := buildRouter("", injectClaims(makeAdminClaims()), RequireServiceAccount("superapp"))
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code, "admin caller must bypass RequireServiceAccount")
}

// TestRequireServiceAccount_ValidSACorrectClient verifies that a service account
// from the expected client is allowed through.
func TestRequireServiceAccount_ValidSACorrectClient(t *testing.T) {
	// Arrange
	router := buildRouter("", injectClaims(makeSAClaims("superapp")), RequireServiceAccount("superapp"))
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code, "valid SA from correct client must be allowed")
}

// TestRequireServiceAccount_ValidSAWrongClient verifies that a service account
// from a different client is rejected with 403.
func TestRequireServiceAccount_ValidSAWrongClient(t *testing.T) {
	// Arrange
	router := buildRouter("", injectClaims(makeSAClaims("other-client")), RequireServiceAccount("superapp"))
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusForbidden, w.Code, "SA from wrong client must be rejected")
}

// TestRequireServiceAccount_NotSACorrectAZP verifies that a regular user whose
// azp matches the allowed client ID is still rejected (no service-account- prefix).
func TestRequireServiceAccount_NotSACorrectAZP(t *testing.T) {
	// Arrange — preferred_username does NOT start with "service-account-"
	claims := &models.JWTClaims{
		PreferredUsername: "regular-user",
		AZP:               "superapp",
	}
	router := buildRouter("", injectClaims(claims), RequireServiceAccount("superapp"))
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusForbidden, w.Code, "non-SA caller must be rejected even with correct azp")
}

// TestRequireServiceAccount_NoClaims verifies that a request with no claims in
// context is rejected with 401.
func TestRequireServiceAccount_NoClaims(t *testing.T) {
	// Arrange — no injectClaims middleware, so context has no "claims" key
	router := buildRouter("", RequireServiceAccount("superapp"))
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code, "missing claims must return 401")
}

// TestRequireServiceAccount_MultipleClientIDs_MatchesSecond verifies that when
// multiple client IDs are allowed, a caller matching the second one is accepted.
func TestRequireServiceAccount_MultipleClientIDs_MatchesSecond(t *testing.T) {
	// Arrange — SA from "app-eai-agent", allowed list is ["superapp", "app-eai-agent"]
	router := buildRouter("",
		injectClaims(makeSAClaims("app-eai-agent")),
		RequireServiceAccount("superapp", "app-eai-agent"),
	)
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code, "SA matching second allowed client must be accepted")
}

// TestRequireServiceAccount_MultipleClientIDs_MatchesNone verifies that a SA
// whose azp does not match any of the allowed client IDs is rejected with 403.
func TestRequireServiceAccount_MultipleClientIDs_MatchesNone(t *testing.T) {
	// Arrange — SA from "unknown-client", allowed list is ["superapp", "app-eai-agent"]
	router := buildRouter("",
		injectClaims(makeSAClaims("unknown-client")),
		RequireServiceAccount("superapp", "app-eai-agent"),
	)
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusForbidden, w.Code, "SA matching no allowed client must be rejected")
}

// ---------------------------------------------------------------------------
// RequireOwnCPFOrServiceAccount tests
// ---------------------------------------------------------------------------

// TestRequireOwnCPFOrServiceAccount_AdminCaller verifies that an admin is always
// allowed through, regardless of the :cpf param or azp.
func TestRequireOwnCPFOrServiceAccount_AdminCaller(t *testing.T) {
	// Arrange
	router := buildRouter("/citizen/:cpf/data",
		injectClaims(makeAdminClaims()),
		RequireOwnCPFOrServiceAccount("superapp"),
	)
	req, _ := http.NewRequest(http.MethodGet, "/citizen/99999999999/data", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code, "admin caller must bypass RequireOwnCPFOrServiceAccount")
}

// TestRequireOwnCPFOrServiceAccount_ValidSAAllowedClient verifies that a service
// account from an allowed client can access any CPF.
func TestRequireOwnCPFOrServiceAccount_ValidSAAllowedClient(t *testing.T) {
	// Arrange
	router := buildRouter("/citizen/:cpf/data",
		injectClaims(makeSAClaims("superapp")),
		RequireOwnCPFOrServiceAccount("superapp", "app-eai-agent"),
	)
	req, _ := http.NewRequest(http.MethodGet, "/citizen/12345678901/data", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code, "SA from allowed client must be accepted")
}

// TestRequireOwnCPFOrServiceAccount_ValidSADisallowedClient verifies that a
// service account from a client NOT in the allowed list is rejected with 403.
func TestRequireOwnCPFOrServiceAccount_ValidSADisallowedClient(t *testing.T) {
	// Arrange — SA from "app-sms-gateway", allowed list is ["superapp"]
	router := buildRouter("/citizen/:cpf/data",
		injectClaims(makeSAClaims("app-sms-gateway")),
		RequireOwnCPFOrServiceAccount("superapp"),
	)
	req, _ := http.NewRequest(http.MethodGet, "/citizen/12345678901/data", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusForbidden, w.Code, "SA from disallowed client must be rejected")
}

// TestRequireOwnCPFOrServiceAccount_CitizenAccessesOwnCPF verifies that a
// regular citizen whose preferred_username matches the :cpf param is allowed.
func TestRequireOwnCPFOrServiceAccount_CitizenAccessesOwnCPF(t *testing.T) {
	// Arrange — preferred_username == CPF in URL
	router := buildRouter("/citizen/:cpf/data",
		injectClaims(makeUserClaims("12345678901")),
		RequireOwnCPFOrServiceAccount("superapp"),
	)
	req, _ := http.NewRequest(http.MethodGet, "/citizen/12345678901/data", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code, "citizen accessing own CPF must be allowed")
}

// TestRequireOwnCPFOrServiceAccount_CitizenAccessesOtherCPF verifies that a
// regular citizen trying to access another citizen's CPF is rejected with 403.
func TestRequireOwnCPFOrServiceAccount_CitizenAccessesOtherCPF(t *testing.T) {
	// Arrange — preferred_username != CPF in URL, not SA, not admin
	router := buildRouter("/citizen/:cpf/data",
		injectClaims(makeUserClaims("12345678901")),
		RequireOwnCPFOrServiceAccount("superapp"),
	)
	req, _ := http.NewRequest(http.MethodGet, "/citizen/99999999999/data", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusForbidden, w.Code, "citizen accessing another's CPF must be rejected")
}

// TestRequireOwnCPFOrServiceAccount_NoClaims verifies that a request with no
// claims in context is rejected with 401.
func TestRequireOwnCPFOrServiceAccount_NoClaims(t *testing.T) {
	// Arrange — no injectClaims middleware
	router := buildRouter("/citizen/:cpf/data", RequireOwnCPFOrServiceAccount("superapp"))
	req, _ := http.NewRequest(http.MethodGet, "/citizen/12345678901/data", nil)
	w := httptest.NewRecorder()

	// Act
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusUnauthorized, w.Code, "missing claims must return 401")
}
