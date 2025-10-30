package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// ListDepartments godoc
// @Summary Listar departamentos/unidades administrativas
// @Description Recupera a lista paginada de departamentos com filtros opcionais (hierarquia, nível, busca).
// @Tags departments
// @Accept json
// @Produce json
// @Param parent_id query string false "Código da UA pai para filtrar departamentos filhos"
// @Param min_level query int false "Nível mínimo do departamento" minimum(1)
// @Param max_level query int false "Nível máximo do departamento" minimum(1)
// @Param exact_level query int false "Nível exato do departamento (sobrescreve min/max)" minimum(1)
// @Param sigla_ua query string false "Sigla da UA para filtro exato"
// @Param search query string false "Texto para busca textual em nome_ua"
// @Param page query int false "Número da página (padrão: 1)" minimum(1)
// @Param per_page query int false "Itens por página (padrão: 10, máximo: 100)" minimum(1) maximum(100)
// @Success 200 {object} models.DepartmentListResponse "Lista paginada de departamentos obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Parâmetros inválidos"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /departments [get]
func ListDepartments(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ListDepartments")
	defer span.End()

	logger := observability.Logger()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "list_departments"),
		attribute.String("service", "department"),
	)

	logger.Debug("ListDepartments called")

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
	filters := services.DepartmentFilters{
		ParentID: c.Query("parent_id"),
		SiglaUA:  c.Query("sigla_ua"),
		Search:   c.Query("search"),
		Page:     page,
		PerPage:  perPage,
	}

	// Parse level filters
	if c.Query("exact_level") != "" {
		exactLevel, err := strconv.Atoi(c.Query("exact_level"))
		if err != nil {
			utils.RecordErrorInSpan(filterSpan, err, map[string]interface{}{
				"exact_level_param": c.Query("exact_level"),
			})
			filterSpan.End()
			logger.Error("invalid exact_level parameter", zap.Error(err))
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid exact_level parameter"})
			return
		}
		filters.ExactLevel = &exactLevel
		utils.AddSpanAttribute(filterSpan, "exact_level", exactLevel)
	}

	if c.Query("min_level") != "" {
		minLevel, err := strconv.Atoi(c.Query("min_level"))
		if err != nil {
			utils.RecordErrorInSpan(filterSpan, err, map[string]interface{}{
				"min_level_param": c.Query("min_level"),
			})
			filterSpan.End()
			logger.Error("invalid min_level parameter", zap.Error(err))
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid min_level parameter"})
			return
		}
		filters.MinLevel = &minLevel
		utils.AddSpanAttribute(filterSpan, "min_level", minLevel)
	}

	if c.Query("max_level") != "" {
		maxLevel, err := strconv.Atoi(c.Query("max_level"))
		if err != nil {
			utils.RecordErrorInSpan(filterSpan, err, map[string]interface{}{
				"max_level_param": c.Query("max_level"),
			})
			filterSpan.End()
			logger.Error("invalid max_level parameter", zap.Error(err))
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid max_level parameter"})
			return
		}
		filters.MaxLevel = &maxLevel
		utils.AddSpanAttribute(filterSpan, "max_level", maxLevel)
	}

	if filters.ParentID != "" {
		utils.AddSpanAttribute(filterSpan, "parent_id", filters.ParentID)
	}
	if filters.SiglaUA != "" {
		utils.AddSpanAttribute(filterSpan, "sigla_ua", filters.SiglaUA)
	}
	if filters.Search != "" {
		utils.AddSpanAttribute(filterSpan, "search", filters.Search)
	}
	filterSpan.End()

	// Check if department service is available
	if services.DepartmentServiceInstance == nil {
		logger.Error("department service not initialized")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Department service unavailable"})
		return
	}

	// Query departments with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "departments", "list_filtered")
	departments, err := services.DepartmentServiceInstance.ListDepartments(ctx, filters)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "list_departments",
			"filters":   filters,
		})
		querySpan.End()
		logger.Error("failed to list departments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve departments"})
		return
	}
	utils.AddSpanAttribute(querySpan, "departments_found", len(departments.Departments))
	utils.AddSpanAttribute(querySpan, "total_count", departments.TotalCount)
	querySpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, departments)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("ListDepartments completed",
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int64("total_count", departments.TotalCount),
		zap.Int("departments_returned", len(departments.Departments)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetDepartment godoc
// @Summary Obter departamento/unidade administrativa por ID
// @Description Recupera um departamento específico pelo código da UA (cd_ua).
// @Tags departments
// @Accept json
// @Produce json
// @Param cd_ua path string true "Código da Unidade Administrativa"
// @Success 200 {object} models.DepartmentResponse "Departamento obtido com sucesso"
// @Failure 400 {object} ErrorResponse "Código da UA inválido"
// @Failure 404 {object} ErrorResponse "Departamento não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /departments/{cd_ua} [get]
func GetDepartment(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetDepartment")
	defer span.End()

	cdUA := c.Param("cd_ua")
	logger := observability.Logger().With(zap.String("cd_ua", cdUA))

	// Add parameters to span attributes
	span.SetAttributes(
		attribute.String("cd_ua", cdUA),
		attribute.String("operation", "get_department"),
		attribute.String("service", "department"),
	)

	logger.Debug("GetDepartment called", zap.String("cd_ua", cdUA))

	// Validate cd_ua parameter
	ctx, validationSpan := utils.TraceInputValidation(ctx, "cd_ua_format", "cd_ua")
	if cdUA == "" {
		utils.RecordErrorInSpan(validationSpan, nil, map[string]interface{}{
			"error": "cd_ua parameter is required",
		})
		validationSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cd_ua parameter is required"})
		return
	}
	validationSpan.End()

	// Check if department service is available
	if services.DepartmentServiceInstance == nil {
		logger.Error("department service not initialized")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Department service unavailable"})
		return
	}

	// Query department with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "departments", "by_cd_ua")
	department, err := services.DepartmentServiceInstance.GetDepartmentByID(ctx, cdUA)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "get_department_by_id",
			"cd_ua":     cdUA,
		})
		querySpan.End()
		logger.Error("failed to get department", zap.Error(err))
		if err.Error() == "department not found with cd_ua: "+cdUA {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Department not found"})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve department"})
		}
		return
	}
	utils.AddSpanAttribute(querySpan, "department_found", true)
	utils.AddSpanAttribute(querySpan, "department_name", department.NomeUA)
	querySpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Convert to response format
	response := department.ToResponse()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetDepartment completed",
		zap.String("cd_ua", cdUA),
		zap.String("department_name", department.NomeUA),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
