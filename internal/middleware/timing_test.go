package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestTiming_Success(t *testing.T) {
	router := gin.New()
	router.Use(RequestTiming())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequestTiming() status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequestTiming_SetsStartTime(t *testing.T) {
	router := gin.New()
	router.Use(RequestTiming())

	var startTime time.Time
	router.GET("/test", func(c *gin.Context) {
		val, exists := c.Get("request_start_time")
		if !exists {
			t.Error("request_start_time not set in context")
		}

		st, ok := val.(time.Time)
		if !ok {
			t.Error("request_start_time is not time.Time")
		}
		startTime = st
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if startTime.IsZero() {
		t.Error("request_start_time was not set")
	}
}

func TestRequestTiming_WithError(t *testing.T) {
	router := gin.New()
	router.Use(RequestTiming())
	router.GET("/error", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "something went wrong"})
	})

	req, _ := http.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("RequestTiming() with error status = %v, want %v", w.Code, http.StatusInternalServerError)
	}
}

func TestRequestTiming_MultipleMethods(t *testing.T) {
	router := gin.New()
	router.Use(RequestTiming())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"method": "GET"})
	})
	router.POST("/test", func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"method": "POST"})
	})
	router.PUT("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"method": "PUT"})
	})
	router.DELETE("/test", func(c *gin.Context) {
		c.JSON(http.StatusNoContent, gin.H{})
	})

	methods := []struct {
		method string
		status int
	}{
		{"GET", http.StatusOK},
		{"POST", http.StatusCreated},
		{"PUT", http.StatusOK},
		{"DELETE", http.StatusNoContent},
	}

	for _, m := range methods {
		t.Run(m.method, func(t *testing.T) {
			req, _ := http.NewRequest(m.method, "/test", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != m.status {
				t.Errorf("RequestTiming() %s status = %v, want %v", m.method, w.Code, m.status)
			}
		})
	}
}

func TestRequestTiming_WithDelay(t *testing.T) {
	router := gin.New()
	router.Use(RequestTiming())
	router.GET("/slow", func(c *gin.Context) {
		time.Sleep(10 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"message": "slow response"})
	})

	start := time.Now()
	req, _ := http.NewRequest("GET", "/slow", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusOK {
		t.Errorf("RequestTiming() with delay status = %v, want %v", w.Code, http.StatusOK)
	}

	if elapsed < 10*time.Millisecond {
		t.Errorf("RequestTiming() elapsed time = %v, should be >= 10ms", elapsed)
	}
}

func TestDatabaseTiming(t *testing.T) {
	router := gin.New()
	router.Use(DatabaseTiming())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("DatabaseTiming() status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestCacheTiming(t *testing.T) {
	router := gin.New()
	router.Use(CacheTiming())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("CacheTiming() status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequestTiming_WithUserAgent(t *testing.T) {
	router := gin.New()
	router.Use(RequestTiming())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("RequestTiming() with user agent status = %v, want %v", w.Code, http.StatusOK)
	}
}

func TestRequestTiming_DifferentStatusCodes(t *testing.T) {
	router := gin.New()
	router.Use(RequestTiming())

	router.GET("/200", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	router.GET("/201", func(c *gin.Context) { c.JSON(http.StatusCreated, gin.H{}) })
	router.GET("/400", func(c *gin.Context) { c.JSON(http.StatusBadRequest, gin.H{}) })
	router.GET("/401", func(c *gin.Context) { c.JSON(http.StatusUnauthorized, gin.H{}) })
	router.GET("/403", func(c *gin.Context) { c.JSON(http.StatusForbidden, gin.H{}) })
	router.GET("/404", func(c *gin.Context) { c.JSON(http.StatusNotFound, gin.H{}) })
	router.GET("/500", func(c *gin.Context) { c.JSON(http.StatusInternalServerError, gin.H{}) })

	statuses := []int{200, 201, 400, 401, 403, 404, 500}

	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/"+strconv.Itoa(status), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != status {
				t.Errorf("RequestTiming() status = %v, want %v", w.Code, status)
			}
		})
	}
}
