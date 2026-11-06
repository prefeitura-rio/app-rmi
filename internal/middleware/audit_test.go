package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

func TestAuditMiddleware_SkipsGETRequests(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("AuditMiddleware() GET status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestAuditMiddleware_SkipsHealthChecks(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.POST("/v1/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	req, _ := http.NewRequest("POST", "/v1/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("AuditMiddleware() health check status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestAuditMiddleware_SkipsMetrics(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.POST("/v1/metrics", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})
	router.POST("/metrics", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{})
	})

	tests := []string{"/v1/metrics", "/metrics"}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			req, _ := http.NewRequest("POST", path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("AuditMiddleware() metrics status = %v, want %v", w.Code, http.StatusOK)
			}
		})
	}
}

func TestAuditMiddleware_POSTRequest(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.POST("/api/resource", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": "123"})
	})

	body := bytes.NewBufferString(`{"name":"test"}`)
	req, _ := http.NewRequest("POST", "/api/resource", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("AuditMiddleware() POST status = %v, want %v", w.Code, http.StatusCreated)
	}
}

func TestAuditMiddleware_PUTRequest(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.PUT("/api/resource/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})

	body := bytes.NewBufferString(`{"name":"updated"}`)
	req, _ := http.NewRequest("PUT", "/api/resource/123", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("AuditMiddleware() PUT status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestAuditMiddleware_DELETERequest(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.DELETE("/api/resource/:id", func(c *gin.Context) {
		c.JSON(http.StatusNoContent, gin.H{})
	})

	req, _ := http.NewRequest("DELETE", "/api/resource/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("AuditMiddleware() DELETE status = %v, want %v", w.Code, http.StatusNoContent)
	}
}

func TestAuditMiddleware_PATCHRequest(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.PATCH("/api/resource/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})

	body := bytes.NewBufferString(`{"field":"value"}`)
	req, _ := http.NewRequest("PATCH", "/api/resource/123", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("AuditMiddleware() PATCH status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestAuditMiddleware_OnlyLogsSuccessful(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.POST("/api/resource", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"})
	})

	body := bytes.NewBufferString(`{"invalid":"data"}`)
	req, _ := http.NewRequest("POST", "/api/resource", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("AuditMiddleware() error status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestAuditMiddleware_WithQueryParams(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.POST("/api/resource", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": "123"})
	})

	body := bytes.NewBufferString(`{"name":"test"}`)
	req, _ := http.NewRequest("POST", "/api/resource?filter=active&sort=name", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("AuditMiddleware() with query params status = %v, want %v", w.Code, http.StatusCreated)
	}
}

func TestAuditMiddleware_LargeBody(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.POST("/api/resource", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": "123"})
	})

	// Create a body larger than 1000 bytes to test truncation
	largeBody := bytes.NewBufferString(`{"data":"` + string(make([]byte, 2000)) + `"}`)
	req, _ := http.NewRequest("POST", "/api/resource", largeBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("AuditMiddleware() large body status = %v, want %v", w.Code, http.StatusCreated)
	}
}

func TestAuditMiddleware_WithUserAgent(t *testing.T) {
	router := gin.New()
	router.Use(AuditMiddleware())
	router.POST("/api/resource", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": "123"})
	})

	body := bytes.NewBufferString(`{"name":"test"}`)
	req, _ := http.NewRequest("POST", "/api/resource", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestClient/1.0")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("AuditMiddleware() with user agent status = %v, want %v", w.Code, http.StatusCreated)
	}
}

type capturedAuditEvent struct {
	action   string
	resource string
	metadata map[string]string
}

// captureAuditEvents replaces the audit persistence for the duration of the
// test and returns the events the middleware handed to it.
func captureAuditEvents(t *testing.T) *[]capturedAuditEvent {
	t.Helper()
	var events []capturedAuditEvent
	original := logAuditEvent
	logAuditEvent = func(_ context.Context, _ utils.AuditContext, action, resource, _ string, _, _ interface{}, metadata map[string]string) error {
		events = append(events, capturedAuditEvent{action: action, resource: resource, metadata: metadata})
		return nil
	}
	t.Cleanup(func() { logAuditEvent = original })
	return &events
}

func TestAuditMiddleware_ResponseStatusIsHTTPCode(t *testing.T) {
	tests := []struct {
		name   string
		method string
		status int
		want   string
	}{
		{name: "POST 201", method: http.MethodPost, status: http.StatusCreated, want: "201"},
		{name: "PUT 200", method: http.MethodPut, status: http.StatusOK, want: "200"},
		{name: "PATCH 202", method: http.MethodPatch, status: http.StatusAccepted, want: "202"},
		{name: "DELETE 204", method: http.MethodDelete, status: http.StatusNoContent, want: "204"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := captureAuditEvents(t)

			router := gin.New()
			router.Use(AuditMiddleware())
			router.Handle(tt.method, "/api/resource", func(c *gin.Context) {
				c.Status(tt.status)
			})

			req, _ := http.NewRequest(tt.method, "/api/resource", bytes.NewBufferString(`{"name":"test"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d", w.Code, tt.status)
			}
			if len(*events) != 1 {
				t.Fatalf("audit events = %d, want 1", len(*events))
			}
			got := (*events)[0].metadata["response_status"]
			if got != tt.want {
				t.Errorf("metadata[response_status] = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuditMiddleware_NoAuditEventForNon2xx(t *testing.T) {
	events := captureAuditEvents(t)

	router := gin.New()
	router.Use(AuditMiddleware())
	router.POST("/api/resource", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid"})
	})

	req, _ := http.NewRequest(http.MethodPost, "/api/resource", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if len(*events) != 0 {
		t.Errorf("audit events = %d, want 0 for a 4xx response", len(*events))
	}
}
