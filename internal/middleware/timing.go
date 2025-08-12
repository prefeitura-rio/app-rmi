package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// RequestTiming adds comprehensive timing information to requests
func RequestTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Add start time to context for handlers to use
		c.Set("request_start_time", start)

		// Create a span for the entire request
		ctx, span := otel.Tracer("http").Start(c.Request.Context(), "http.request")

		// Set attributes after span creation
		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.url", c.Request.URL.String()),
			attribute.String("http.route", c.FullPath()),
			attribute.String("http.user_agent", c.Request.UserAgent()),
			attribute.String("http.client_ip", c.ClientIP()),
		)
		defer span.End()

		// Update the request context
		c.Request = c.Request.WithContext(ctx)

		// Process request
		c.Next()

		// Calculate timing metrics
		latency := time.Since(start)
		status := c.Writer.Status()

		// Add timing attributes to span
		span.SetAttributes(
			attribute.Int("http.status_code", status),
			attribute.Int64("http.duration_ms", latency.Milliseconds()),
			attribute.String("http.duration", latency.String()),
		)

		// Log request completion with timing
		observability.Logger().Info("request completed",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("route", c.FullPath()),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		)

		// Update metrics
		observability.RequestDuration.WithLabelValues(
			c.FullPath(),
			c.Request.Method,
			string(rune(status)),
		).Observe(latency.Seconds())

		// Record errors in span if status indicates error
		if status >= 400 {
			span.SetAttributes(attribute.String("http.error", "true"))
		}
	}
}

// DatabaseTiming adds timing information to database operations
func DatabaseTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		// This middleware can be used to add database operation timing
		// It's a placeholder for future database-specific timing middleware
		c.Next()
	}
}

// CacheTiming adds timing information to cache operations
func CacheTiming() gin.HandlerFunc {
	return func(c *gin.Context) {
		// This middleware can be used to add cache operation timing
		// It's a placeholder for future cache-specific timing middleware
		c.Next()
	}
}
