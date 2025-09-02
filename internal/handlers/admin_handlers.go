package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// CacheReadRequest represents a request to read from cache
type CacheReadRequest struct {
	Key string `json:"key" binding:"required" example:"citizen:12345678901"`
}

// CacheReadResponse represents a response from cache read
type CacheReadResponse struct {
	Key    string      `json:"key"`
	Value  interface{} `json:"value"`
	Exists bool        `json:"exists"`
	TTL    int64       `json:"ttl_seconds"`
}

// ReadCacheKey godoc
// @Summary Ler chave arbitrária do cache Redis
// @Description Permite aos administradores ler qualquer chave do cache Redis para debug e monitoramento
// @Tags admin
// @Accept json
// @Produce json
// @Param data body CacheReadRequest true "Chave do cache para ler"
// @Security BearerAuth
// @Success 200 {object} CacheReadResponse "Dados da chave do cache"
// @Failure 400 {object} ErrorResponse "Parâmetros inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - somente administradores"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /admin/cache/read [post]
func ReadCacheKey(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ReadCacheKey")
	defer span.End()

	logger := observability.Logger()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "read_cache_key"),
		attribute.String("service", "admin"),
	)

	logger.Debug("ReadCacheKey called")

	// Parse input with tracing
	ctx, parseSpan := utils.TraceInputParsing(ctx, "cache_read_request")
	var request CacheReadRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		utils.RecordErrorInSpan(parseSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "CacheReadRequest",
		})
		parseSpan.End()
		logger.Error("failed to parse cache read request", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(parseSpan, "cache.key", request.Key)
	parseSpan.End()

	// Read from Redis with tracing
	ctx, cacheSpan := utils.TraceCacheGet(ctx, request.Key)
	value, err := config.Redis.Get(ctx, request.Key).Result()

	response := CacheReadResponse{
		Key:    request.Key,
		Exists: err == nil,
		TTL:    -1,
	}

	if err == nil {
		// Key exists, get TTL and value
		response.Value = value
		utils.AddSpanAttribute(cacheSpan, "cache.hit", true)

		// Get TTL
		ttl, ttlErr := config.Redis.TTL(ctx, request.Key).Result()
		if ttlErr == nil {
			response.TTL = int64(ttl.Seconds())
		}

		observability.CacheHits.WithLabelValues("admin_cache_read").Inc()
		logger.Debug("cache key found",
			zap.String("key", request.Key),
			zap.Int("value_length", len(value)),
			zap.Int64("ttl_seconds", response.TTL))
	} else {
		// Key doesn't exist or error occurred
		response.Value = nil
		utils.AddSpanAttribute(cacheSpan, "cache.hit", false)

		if err.Error() == "redis: nil" {
			// Key doesn't exist - this is normal
			logger.Debug("cache key not found", zap.String("key", request.Key))
		} else {
			// Actual Redis error
			utils.RecordErrorInSpan(cacheSpan, err, map[string]interface{}{
				"cache.key": request.Key,
			})
			logger.Error("failed to read from cache", zap.Error(err), zap.String("key", request.Key))
			cacheSpan.End()
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to read from cache: " + err.Error()})
			return
		}
	}
	cacheSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("ReadCacheKey completed",
		zap.String("key", request.Key),
		zap.Bool("exists", response.Exists),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
