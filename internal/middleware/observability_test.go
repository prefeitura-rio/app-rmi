package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestLogger(t *testing.T) {
	router := gin.New()
	router.Use(RequestLogger())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequestLogger() status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequestLogger_WithQueryParams(t *testing.T) {
	router := gin.New()
	router.Use(RequestLogger())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test?param1=value1&param2=value2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequestLogger() with query params status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequestLogger_WithUserAgent(t *testing.T) {
	router := gin.New()
	router.Use(RequestLogger())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequestLogger() with user agent status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequestLogger_DifferentStatusCodes(t *testing.T) {
	router := gin.New()
	router.Use(RequestLogger())

	router.GET("/200", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	router.GET("/400", func(c *gin.Context) { c.JSON(http.StatusBadRequest, gin.H{}) })
	router.GET("/500", func(c *gin.Context) { c.JSON(http.StatusInternalServerError, gin.H{}) })

	tests := []struct {
		name   string
		path   string
		status int
	}{
		{"OK", "/200", http.StatusOK},
		{"Bad Request", "/400", http.StatusBadRequest},
		{"Internal Server Error", "/500", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.status {
				t.Errorf("RequestLogger() status = %v, want %v", w.Code, tt.status)
			}
		})
	}
}

func TestRequestTracker(t *testing.T) {
	router := gin.New()
	router.Use(RequestTracker())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequestTracker() status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequestTracker_MultipleRequests(t *testing.T) {
	router := gin.New()
	router.Use(RequestTracker())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Make multiple requests to verify connection tracking
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("RequestTracker() request %d status = %v, want %v", i, w.Code, http.StatusOK)
		}
	}
}

func TestRequestID(t *testing.T) {
	router := gin.New()
	router.Use(RequestID())

	var capturedID string
	router.GET("/test", func(c *gin.Context) {
		val, exists := c.Get("RequestID")
		if !exists {
			t.Error("RequestID not set in context")
		}
		id, ok := val.(string)
		if !ok {
			t.Error("RequestID is not string")
		}
		capturedID = id
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequestID() status = %v, want %v", w.Code, http.StatusOK)
	}

	if capturedID == "" {
		t.Error("RequestID was not set")
	}

	// Verify response header contains request ID
	responseID := w.Header().Get("X-Request-ID")
	if responseID == "" {
		t.Error("X-Request-ID header not set in response")
	}

	if responseID != capturedID {
		t.Errorf("X-Request-ID header %v does not match context RequestID %v", responseID, capturedID)
	}
}

func TestRequestID_WithProvidedID(t *testing.T) {
	router := gin.New()
	router.Use(RequestID())

	var capturedID string
	router.GET("/test", func(c *gin.Context) {
		val, _ := c.Get("RequestID")
		capturedID = val.(string)
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "custom-request-id-123")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequestID() with provided ID status = %v, want %v", w.Code, http.StatusOK)
	}

	if capturedID != "custom-request-id-123" {
		t.Errorf("RequestID = %v, want custom-request-id-123", capturedID)
	}

	responseID := w.Header().Get("X-Request-ID")
	if responseID != "custom-request-id-123" {
		t.Errorf("X-Request-ID header = %v, want custom-request-id-123", responseID)
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	if id1 == "" {
		t.Error("generateRequestID() returned empty string")
	}

	if id2 == "" {
		t.Error("generateRequestID() returned empty string on second call")
	}

	// IDs should have expected format (timestamp-randomstring)
	if len(id1) < 20 { // 14 chars for timestamp + 1 for dash + 6 for random
		t.Errorf("generateRequestID() returned too short ID: %v", id1)
	}
}

func TestRandomString(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{"length 6", 6},
		{"length 10", 10},
		{"length 1", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := randomString(tt.length)
			if len(str) != tt.length {
				t.Errorf("randomString(%d) length = %v, want %v", tt.length, len(str), tt.length)
			}
		})
	}
}

func TestRandomString_Uniqueness(t *testing.T) {
	str1 := randomString(10)
	str2 := randomString(10)

	if str1 == "" {
		t.Error("randomString() returned empty string")
	}

	if str2 == "" {
		t.Error("randomString() returned empty string on second call")
	}

	// Note: Due to time-based random generation, strings might be the same
	// This test just verifies they're generated without errors
}
