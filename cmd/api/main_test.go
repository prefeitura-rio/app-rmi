package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

// phoneLookupTestRouter registers the real routes with stub handlers that answer 200
// with the route name, so a 200 proves the request reached the handler.
func phoneLookupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	stub := func(name string) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"handler": name})
		}
	}
	registerPhoneLookupRoutes(router.Group("/v1"), phoneLookupHandlers{
		PhoneStatus:   stub("phone_status"),
		BetaStatus:    stub("beta_status"),
		ValidatePhone: stub("validate_phone"),
		ValidateEmail: stub("validate_email"),
	})
	return router
}

func testBearerToken(t *testing.T) string {
	t.Helper()
	claimsJSON, err := json.Marshal(models.JWTClaims{SUB: "user-test", PreferredUsername: "12345678901"})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + base64.RawURLEncoding.EncodeToString(claimsJSON) + ".fake-signature"
}

var phoneLookupRoutes = []struct {
	method  string
	path    string
	body    string
	handler string
}{
	{http.MethodGet, "/v1/phone/+5521000000000/status", "", "phone_status"},
	{http.MethodGet, "/v1/phone/+5521000000000/beta-status", "", "beta_status"},
	{http.MethodPost, "/v1/validate/phone", `{"phone":"+5521999999999"}`, "validate_phone"},
	{http.MethodPost, "/v1/validate/email", `{"email":"teste@example.com"}`, "validate_email"},
}

func TestPhoneLookupRoutes_RequireAuthentication(t *testing.T) {
	router := phoneLookupTestRouter()

	authHeaders := []struct {
		name  string
		value string
	}{
		{"no authorization header", ""},
		{"not a bearer token", "Basic dXNlcjpwYXNz"},
		{"malformed bearer token", "Bearer not-a-jwt"},
	}

	for _, route := range phoneLookupRoutes {
		for _, auth := range authHeaders {
			t.Run(route.method+" "+route.path+" "+auth.name, func(t *testing.T) {
				req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				req.Header.Set("Content-Type", "application/json")
				if auth.value != "" {
					req.Header.Set("Authorization", auth.value)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				if w.Code != http.StatusUnauthorized {
					t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusUnauthorized, w.Body.String())
				}
				if strings.Contains(w.Body.String(), route.handler) {
					t.Fatalf("handler %s ran without a valid token", route.handler)
				}
			})
		}
	}
}

func TestPhoneLookupRoutes_ValidTokenReachesHandler(t *testing.T) {
	router := phoneLookupTestRouter()
	token := testBearerToken(t)

	for _, route := range phoneLookupRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, http.StatusOK, w.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["handler"] != route.handler {
				t.Fatalf("handler = %q, want %q", body["handler"], route.handler)
			}
		})
	}
}
