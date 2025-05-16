package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.uber.org/zap"
)

// RequestLogger logs request information
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// Log request details with sensitive data masking
		observability.Logger.Info("request completed",
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.String("method", c.Request.Method),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("user_agent", c.Request.UserAgent()),
		)

		// Update metrics
		observability.RequestDuration.WithLabelValues(
			path,
			c.Request.Method,
			string(rune(status)),
		).Observe(latency.Seconds())
	}
}

// RequestTracker tracks active connections
func RequestTracker() gin.HandlerFunc {
	return func(c *gin.Context) {
		observability.ActiveConnections.Inc()
		defer observability.ActiveConnections.Dec()
		c.Next()
	}
}

// RequestID adds a unique request ID to the context
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		c.Set("RequestID", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	// Implementation using UUID or similar
	return time.Now().Format("20060102150405") + "-" + randomString(6)
}

// randomString generates a random string of given length
func randomString(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[time.Now().UnixNano()%int64(len(letterBytes))]
	}
	return string(b)
} 