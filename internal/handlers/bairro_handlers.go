package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// BairroHandlers handles bairro-related HTTP requests
type BairroHandlers struct {
	service *services.BairroService
	logger  *logging.SafeLogger
}

// NewBairroHandlers creates a new BairroHandlers instance
func NewBairroHandlers(service *services.BairroService, logger *logging.SafeLogger) *BairroHandlers {
	return &BairroHandlers{
		service: service,
		logger:  logger,
	}
}

// ListBairros godoc
// @Summary Listar bairros
// @Description Recupera a lista paginada de bairros com filtro opcional por nome (busca case-insensitive)
// @Tags bairros
// @Accept json
// @Produce json
// @Param page query int false "Número da página (padrão: 1)" minimum(1)
// @Param limit query int false "Itens por página (padrão: 50, máximo: 100)" minimum(1) maximum(100)
// @Param search query string false "Texto para busca no nome do bairro (case-insensitive)"
// @Success 200 {object} models.BairroListResponse "Lista paginada de bairros obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Parâmetros inválidos"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /bairros [get]
func (h *BairroHandlers) ListBairros(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ListBairros")
	defer span.End()

	logger := observability.Logger()

	span.SetAttributes(
		attribute.String("operation", "list_bairros"),
		attribute.String("service", "bairro"),
	)

	logger.Debug("ListBairros called")

	// Parse and validate pagination parameters
	ctx, paginationSpan := utils.TraceInputParsing(ctx, "pagination_parameters")

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil {
			utils.RecordErrorInSpan(paginationSpan, err, map[string]interface{}{
				"page_param": pageStr,
			})
			paginationSpan.End()
			logger.Error("invalid page parameter", zap.String("page", pageStr), zap.Error(err))
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid page parameter: must be a positive integer"})
			return
		}
		if p < 1 {
			validationErr := fmt.Errorf("page must be >= 1, got %d", p)
			utils.RecordErrorInSpan(paginationSpan, validationErr, map[string]interface{}{
				"page_param": pageStr,
			})
			paginationSpan.End()
			logger.Error("invalid page parameter", zap.String("page", pageStr), zap.Error(validationErr))
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid page parameter: must be a positive integer"})
			return
		}
		page = p
	}

	limit := 50
	if limitStr := c.Query("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil {
			utils.RecordErrorInSpan(paginationSpan, err, map[string]interface{}{
				"limit_param": limitStr,
			})
			paginationSpan.End()
			logger.Error("invalid limit parameter", zap.String("limit", limitStr), zap.Error(err))
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid limit parameter: must be between 1 and 100"})
			return
		}
		if l < 1 || l > 100 {
			validationErr := fmt.Errorf("limit must be between 1 and 100, got %d", l)
			utils.RecordErrorInSpan(paginationSpan, validationErr, map[string]interface{}{
				"limit_param": limitStr,
			})
			paginationSpan.End()
			logger.Error("invalid limit parameter", zap.String("limit", limitStr), zap.Error(validationErr))
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid limit parameter: must be between 1 and 100"})
			return
		}
		limit = l
	}

	utils.AddSpanAttribute(paginationSpan, "page", page)
	utils.AddSpanAttribute(paginationSpan, "limit", limit)
	paginationSpan.End()

	// Parse optional search filter
	ctx, filterSpan := utils.TraceInputParsing(ctx, "filter_parameters")
	search := c.Query("search")
	if search != "" {
		utils.AddSpanAttribute(filterSpan, "search", search)
	}
	filterSpan.End()

	filters := models.BairroFilters{
		Page:   page,
		Limit:  limit,
		Search: search,
	}

	// Query bairros
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "bairros", "list_filtered")
	result, err := h.service.ListBairros(ctx, filters)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "list_bairros",
		})
		querySpan.End()
		logger.Error("failed to list bairros", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve bairros"})
		return
	}
	utils.AddSpanAttribute(querySpan, "results_count", len(result.Bairros))
	utils.AddSpanAttribute(querySpan, "total_count", result.Total)
	querySpan.End()

	observability.DatabaseOperations.WithLabelValues("read", "success").Inc()

	c.JSON(http.StatusOK, result)

	totalDuration := time.Since(startTime)
	logger.Debug("ListBairros completed",
		zap.Int("results_count", len(result.Bairros)),
		zap.Int64("total_count", result.Total),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
