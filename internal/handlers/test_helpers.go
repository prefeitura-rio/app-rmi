package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

// createTestJWT creates a fake JWT token for testing
func createTestJWT(claims models.JWTClaims) string {
	claimsJSON, _ := json.Marshal(claims)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Create a fake JWT (header.payload.signature)
	return "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + claimsB64 + ".fake-signature"
}

// createTestRequest creates an HTTP request with optional JWT authentication
func createTestRequest(method, url string, body string, claims *models.JWTClaims) *http.Request {
	req, _ := http.NewRequest(method, url, nil)
	if body != "" {
		req, _ = http.NewRequest(method, url, nil)
	}

	if claims != nil {
		token := createTestJWT(*claims)
		req.Header.Set("Authorization", "Bearer "+token)
	}

	req.Header.Set("Content-Type", "application/json")
	return req
}

// createAdminClaims creates JWT claims with admin role
func createAdminClaims(cpf string) models.JWTClaims {
	claims := models.JWTClaims{
		SUB:               "admin-user",
		ISS:               "test-issuer",
		PreferredUsername: cpf,
	}
	claims.ResourceAccess.Superapp.Roles = []string{"go:admin"}
	return claims
}

// createUserClaims creates JWT claims for a regular user
func createUserClaims(cpf string) models.JWTClaims {
	return models.JWTClaims{
		SUB:               "user-" + cpf,
		ISS:               "test-issuer",
		PreferredUsername: cpf,
	}
}

// setupTestRouter creates a test router with auth middleware
func setupTestRouter(withAuth bool, isAdmin bool) *gin.Engine {
	router := gin.New()

	if withAuth {
		if isAdmin {
			// Mock admin user
			router.Use(func(c *gin.Context) {
				claims := createAdminClaims("12345678901")
				c.Set("claims", &claims)
				c.Next()
			})
		} else {
			// Mock regular user
			router.Use(func(c *gin.Context) {
				claims := createUserClaims("12345678901")
				c.Set("claims", &claims)
				c.Next()
			})
		}
	}

	return router
}

// executeRequest executes a request and returns the response recorder
func executeRequest(router *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}
