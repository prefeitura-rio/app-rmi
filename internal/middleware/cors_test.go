package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newCORSTestRouter(origins []string) *gin.Engine {
	r := gin.New()
	r.Use(CORS(origins))
	r.GET("/v1/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.OPTIONS("/v1/health", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return r
}

func TestCORS_AllowsListedOrigin(t *testing.T) {
	router := newCORSTestRouter([]string{
		"https://app.example.com",
		"https://admin.example.com",
		"https://staging.example.com",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", "https://staging.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://staging.example.com", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_RejectsUnknownOrigin(t *testing.T) {
	router := newCORSTestRouter([]string{"https://app.example.com", "https://staging.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Methods"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORS_EmptyAllowlistRejectsCrossOrigin(t *testing.T) {
	router := newCORSTestRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_NoOriginHeaderUnaffected(t *testing.T) {
	router := newCORSTestRouter([]string{"https://app.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_PreflightAllowsListedOrigin(t *testing.T) {
	router := newCORSTestRouter([]string{"https://admin.example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "https://admin.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.NotEmpty(t, w.Header().Get("Access-Control-Allow-Methods"))
}

func TestCORS_PreflightRejectsUnknownOrigin(t *testing.T) {
	router := newCORSTestRouter([]string{"https://app.example.com"})

	req := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_DefaultAllowlistAllowsKnownFrontends(t *testing.T) {
	router := newCORSTestRouter(config.DefaultCORSAllowedOrigins)

	for _, origin := range []string{
		"https://pref.rio",
		"https://admin.pref.rio",
		"https://staging.app.dados.rio",
		"https://admin.staging.app.dados.rio",
	} {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", http.MethodGet)
			req.Header.Set("Access-Control-Request-Headers", "Content-Type")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusNoContent, w.Code)
			assert.Equal(t, origin, w.Header().Get("Access-Control-Allow-Origin"))
			assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
		})
	}
}

func TestCORS_DefaultAllowlistRejectsOtherOrigins(t *testing.T) {
	router := newCORSTestRouter(config.DefaultCORSAllowedOrigins)

	for _, origin := range []string{
		"https://evil.example",
		"http://pref.rio",
		"https://pref.rio.evil.example",
		"https://evilpref.rio",
		"https://app.dados.rio",
		"null",
	} {
		t.Run(origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/v1/health", nil)
			req.Header.Set("Origin", origin)
			req.Header.Set("Access-Control-Request-Method", http.MethodGet)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Methods"))
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Headers"))
		})
	}
}
