package middleware

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

// Synthetic CPFs (valid check digits, not real people).
const (
	trustedTestCitizenCPF = "52998224725"
	trustedTestOtherCPF   = "11144477735"
)

func withTrustedServiceClients(t *testing.T, clients ...string) {
	t.Helper()
	orig := config.AppConfig.TrustedServiceClients
	config.AppConfig.TrustedServiceClients = clients
	t.Cleanup(func() { config.AppConfig.TrustedServiceClients = orig })
}

// citizenLoginClaims mimics the token any citizen gets when logging into the
// public Superapp client: azp is the login client, preferred_username is the CPF.
func citizenLoginClaims(azp, cpf string) *models.JWTClaims {
	return &models.JWTClaims{
		AZP:               azp,
		PreferredUsername: cpf,
		Scope:             "phone address profile email",
	}
}

func serveWithClaims(claims *models.JWTClaims, mw gin.HandlerFunc, method, route, path string) int {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("claims", claims)
		c.Next()
	})
	router.Use(mw)
	router.Handle(method, route, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

func TestIsTrustedServiceClient(t *testing.T) {
	withTrustedServiceClients(t, "superapp", " app-go-api ")

	tests := []struct {
		name   string
		claims *models.JWTClaims
		want   bool
	}{
		{"nil claims", nil, false},
		{"citizen login token with trusted azp", citizenLoginClaims("superapp", trustedTestCitizenCPF), false},
		{"citizen token with empty username", &models.JWTClaims{AZP: "superapp"}, false},
		{"username of another service account", &models.JWTClaims{AZP: "superapp", PreferredUsername: "service-account-app-go-api"}, false},
		{"username only prefixed", &models.JWTClaims{AZP: "superapp", PreferredUsername: "service-account-superapp-x"}, false},
		{"client_id of another client", &models.JWTClaims{AZP: "superapp", PreferredUsername: trustedTestCitizenCPF, ClientID: "app-go-api"}, false},
		{"service account of untrusted client", &models.JWTClaims{AZP: "other", PreferredUsername: "service-account-other", ClientID: "other"}, false},
		{"empty azp", &models.JWTClaims{PreferredUsername: "service-account-"}, false},
		{"service account username", &models.JWTClaims{AZP: "app-go-api", PreferredUsername: "service-account-app-go-api"}, true},
		{"azp case differs from configured client", &models.JWTClaims{AZP: "App-Go-API", PreferredUsername: "service-account-app-go-api"}, false},
		{"service account client_id claim", &models.JWTClaims{AZP: "app-go-api", ClientID: "app-go-api"}, true},
		{"service account legacy clientId claim", &models.JWTClaims{AZP: "app-go-api", LegacyClientID: "app-go-api"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTrustedServiceClient(tt.claims); got != tt.want {
				t.Errorf("IsTrustedServiceClient() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsTrustedServiceClient_CaseInsensitiveUsername(t *testing.T) {
	withTrustedServiceClients(t, "App-Go-API")

	claims := &models.JWTClaims{AZP: "App-Go-API", PreferredUsername: "service-account-app-go-api"}
	if !IsTrustedServiceClient(claims) {
		t.Error("Keycloak lowercases service account usernames; mixed-case client id must still match")
	}
}

func TestIsTrustedServiceClient_EmptyList(t *testing.T) {
	withTrustedServiceClients(t)

	claims := &models.JWTClaims{AZP: "app-go-api", PreferredUsername: "service-account-app-go-api", ClientID: "app-go-api"}
	if IsTrustedServiceClient(claims) {
		t.Error("no client is trusted when TRUSTED_SERVICE_CLIENTS is empty")
	}
}

func TestRequireAdmin_CitizenTokenWithTrustedAZP_Forbidden(t *testing.T) {
	withTrustedServiceClients(t, "superapp")

	code := serveWithClaims(citizenLoginClaims("superapp", trustedTestCitizenCPF), RequireAdmin(), http.MethodPost, "/admin/cache/read", "/admin/cache/read")
	if code != http.StatusForbidden {
		t.Errorf("RequireAdmin() citizen token with trusted azp status = %v, want %v", code, http.StatusForbidden)
	}
}

func TestRequireOwnCPF_CitizenTokenWithTrustedAZP_OtherCPFForbidden(t *testing.T) {
	withTrustedServiceClients(t, "superapp")

	code := serveWithClaims(citizenLoginClaims("superapp", trustedTestCitizenCPF), RequireOwnCPF(), http.MethodGet, "/citizen/:cpf/optin", "/citizen/"+trustedTestOtherCPF+"/optin")
	if code != http.StatusForbidden {
		t.Errorf("RequireOwnCPF() citizen token with trusted azp on other CPF status = %v, want %v", code, http.StatusForbidden)
	}
}

func TestRequireOwnCPF_CitizenTokenWithTrustedAZP_OwnCPFAllowed(t *testing.T) {
	withTrustedServiceClients(t, "superapp")

	code := serveWithClaims(citizenLoginClaims("superapp", trustedTestCitizenCPF), RequireOwnCPF(), http.MethodGet, "/citizen/:cpf/optin", "/citizen/"+trustedTestCitizenCPF+"/optin")
	if code != http.StatusOK {
		t.Errorf("RequireOwnCPF() citizen token on own CPF status = %v, want %v", code, http.StatusOK)
	}
}

func TestIsAdmin_CitizenTokenWithTrustedAZP_False(t *testing.T) {
	withTrustedServiceClients(t, "superapp")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("claims", citizenLoginClaims("superapp", trustedTestCitizenCPF))

	isAdmin, err := IsAdmin(c)
	if err != nil {
		t.Fatalf("IsAdmin() error = %v", err)
	}
	if isAdmin {
		t.Error("IsAdmin() must be false for a citizen login token whose azp is in TRUSTED_SERVICE_CLIENTS")
	}
}

func TestRequireAdmin_ServiceAccountClientIDClaim_Allowed(t *testing.T) {
	withTrustedServiceClients(t, "app-go-api")

	claims := &models.JWTClaims{AZP: "app-go-api", PreferredUsername: "service-account-app-go-api", ClientID: "app-go-api"}
	code := serveWithClaims(claims, RequireAdmin(), http.MethodGet, "/admin", "/admin")
	if code != http.StatusOK {
		t.Errorf("RequireAdmin() trusted service account status = %v, want %v", code, http.StatusOK)
	}
}

func TestExtractClaims_ServiceAccountClientIDClaims(t *testing.T) {
	// Keycloak >= 20 emits client_id; older versions emit clientId.
	for _, tc := range []struct {
		name    string
		payload string
	}{
		{"client_id", `{"azp":"app-go-api","client_id":"app-go-api"}`},
		{"clientId", `{"azp":"app-go-api","clientId":"app-go-api"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + rawURL(tc.payload) + ".sig"
			claims, err := extractClaims(token)
			if err != nil {
				t.Fatalf("extractClaims() error = %v", err)
			}
			if claims.ServiceAccountClientID() != "app-go-api" {
				t.Errorf("ServiceAccountClientID() = %q, want app-go-api", claims.ServiceAccountClientID())
			}
		})
	}
}

func rawURL(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
