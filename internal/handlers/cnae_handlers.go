package handlers

import (
	"net/http"
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

// CNAEHandlers handles CNAE-related HTTP requests
type CNAEHandlers struct {
	service *services.CNAEService
	logger  *logging.SafeLogger
}

// NewCNAEHandlers creates a new CNAE handlers instance
func NewCNAEHandlers(service *services.CNAEService, logger *logging.SafeLogger) *CNAEHandlers {
	return &CNAEHandlers{
		service: service,
		logger:  logger,
	}
}

// ListCNAEs godoc
// @Summary Listar CNAEs
// @Description Recupera a lista paginada de CNAEs (Classificação Nacional de Atividades Econômicas) com filtros opcionais
// @Tags cnaes
// @Accept json
// @Produce json
// @Param page query int false "Número da página (padrão: 1)" minimum(1)
// @Param per_page query int false "Itens por página (padrão: 10, máximo: 100)" minimum(1) maximum(100)
// @Param search query string false "Texto para busca na denominação"
// @Param secao query string false "Filtro por seção (ex: A)"
// @Param divisao query string false "Filtro por divisão (ex: 1)"
// @Param grupo query string false "Filtro por grupo (ex: 1.1)"
// @Param classe query string false "Filtro por classe (ex: 01.12-1)"
// @Param subclasse query string false "Filtro por subclasse"
// @Success 200 {object} models.CNAEListResponse "Lista paginada de CNAEs obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Parâmetros inválidos"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /cnaes [get]
func (h *CNAEHandlers) ListCNAEs(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ListCNAEs")
	defer span.End()

	logger := observability.Logger()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "list_cnaes"),
		attribute.String("service", "cnae"),
	)

	logger.Debug("ListCNAEs called")

	// Parse pagination parameters with tracing
	ctx, paginationSpan := utils.TraceInputParsing(ctx, "pagination_parameters")
	page, perPage, err := services.ValidatePaginationParams(c.Query("page"), c.Query("per_page"))
	if err != nil {
		utils.RecordErrorInSpan(paginationSpan, err, map[string]interface{}{
			"page_param":     c.Query("page"),
			"per_page_param": c.Query("per_page"),
		})
		paginationSpan.End()
		logger.Error("invalid pagination parameters", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	utils.AddSpanAttribute(paginationSpan, "page", page)
	utils.AddSpanAttribute(paginationSpan, "per_page", perPage)
	paginationSpan.End()

	// Parse filter parameters with tracing
	ctx, filterSpan := utils.TraceInputParsing(ctx, "filter_parameters")
	searchQuery := c.Query("search")
	secao := c.Query("secao")
	divisao := c.Query("divisao")
	grupo := c.Query("grupo")
	classe := c.Query("classe")
	subclasse := c.Query("subclasse")

	if searchQuery != "" {
		utils.AddSpanAttribute(filterSpan, "search", searchQuery)
	}
	if secao != "" {
		utils.AddSpanAttribute(filterSpan, "secao", secao)
	}
	if divisao != "" {
		utils.AddSpanAttribute(filterSpan, "divisao", divisao)
	}
	if grupo != "" {
		utils.AddSpanAttribute(filterSpan, "grupo", grupo)
	}
	if classe != "" {
		utils.AddSpanAttribute(filterSpan, "classe", classe)
	}
	if subclasse != "" {
		utils.AddSpanAttribute(filterSpan, "subclasse", subclasse)
	}
	filterSpan.End()

	// Build filters
	filters := models.CNAEFilters{
		Page:      page,
		PerPage:   perPage,
		Search:    searchQuery,
		Secao:     secao,
		Divisao:   divisao,
		Grupo:     grupo,
		Classe:    classe,
		Subclasse: subclasse,
	}

	// Query CNAEs with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "cnaes", "list_filtered")
	cnaes, err := h.service.ListCNAEs(ctx, filters)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "list_cnaes",
			"filters":   filters,
		})
		querySpan.End()
		logger.Error("failed to list CNAEs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to list CNAEs"})
		return
	}
	utils.AddSpanAttribute(querySpan, "results_count", len(cnaes.CNAEs))
	utils.AddSpanAttribute(querySpan, "total_count", cnaes.Pagination.Total)
	querySpan.End()

	// Record success metrics
	observability.DatabaseOperations.WithLabelValues("read", "success").Inc()

	// Return response
	c.JSON(http.StatusOK, cnaes)

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("ListCNAEs completed",
		zap.Int("results_count", len(cnaes.CNAEs)),
		zap.Int("total_count", cnaes.Pagination.Total),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
