package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
