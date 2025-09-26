package handlers

import (
	"fmt"
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

// GetPets godoc
// @Summary Obter pets associados ao CPF
// @Description Recupera a lista paginada de pets associados ao CPF do cidadão.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param page query int false "Número da página (padrão: 1)" minimum(1)
// @Param per_page query int false "Itens por página (padrão: 10, máximo: 100)" minimum(1) maximum(100)
// @Security BearerAuth
// @Success 200 {object} models.PaginatedPets "Lista paginada de pets obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou parâmetros de paginação inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/pets [get]
func GetPets(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetPets")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_pets"),
		attribute.String("service", "pet"),
	)

	logger.Debug("GetPets called", zap.String("cpf", cpf))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

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

	// Check if pet service is available
	if services.PetServiceInstance == nil {
		logger.Error("pet service not initialized")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Pet service unavailable"})
		return
	}

	// Query pets with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "pets", "cpf")
	pets, err := services.PetServiceInstance.GetPetsByCPF(ctx, cpf, page, perPage)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "get_pets_by_cpf",
			"cpf":       cpf,
			"page":      page,
			"per_page":  perPage,
		})
		querySpan.End()
		logger.Error("failed to get pets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve pets"})
		return
	}
	utils.AddSpanAttribute(querySpan, "pets_found", len(pets.Data))
	utils.AddSpanAttribute(querySpan, "total_count", pets.Pagination.Total)
	querySpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, pets)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetPets completed",
		zap.String("cpf", cpf),
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int("total_count", pets.Pagination.Total),
		zap.Int("pets_returned", len(pets.Data)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetPet godoc
// @Summary Obter pet específico por ID
// @Description Recupera um pet específico associado ao CPF do cidadão pelo ID do pet.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param pet_id path int true "ID do pet"
// @Security BearerAuth
// @Success 200 {object} models.Pet "Pet obtido com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou ID do pet inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Pet não encontrado"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/pets/{pet_id} [get]
func GetPet(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetPet")
	defer span.End()

	cpf := c.Param("cpf")
	petIDStr := c.Param("pet_id")
	logger := observability.Logger().With(zap.String("cpf", cpf), zap.String("pet_id", petIDStr))

	// Add parameters to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("pet_id", petIDStr),
		attribute.String("operation", "get_pet"),
		attribute.String("service", "pet"),
	)

	logger.Debug("GetPet called", zap.String("cpf", cpf), zap.String("pet_id", petIDStr))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Validate and parse pet ID
	ctx, petIDSpan := utils.TraceInputValidation(ctx, "pet_id_format", "pet_id")
	petID, err := strconv.Atoi(petIDStr)
	if err != nil {
		utils.RecordErrorInSpan(petIDSpan, fmt.Errorf("invalid pet ID format"), map[string]interface{}{
			"pet_id": petIDStr,
		})
		petIDSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid pet ID format"})
		return
	}
	utils.AddSpanAttribute(petIDSpan, "pet_id", petID)
	petIDSpan.End()

	// Check if pet service is available
	if services.PetServiceInstance == nil {
		logger.Error("pet service not initialized")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Pet service unavailable"})
		return
	}

	// Query pet with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "pets", "pet_by_id")
	pet, err := services.PetServiceInstance.GetPetByID(ctx, cpf, petID)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "get_pet_by_id",
			"cpf":       cpf,
			"pet_id":    petID,
		})
		querySpan.End()
		logger.Error("failed to get pet", zap.Error(err))
		if err.Error() == "pet not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Pet not found"})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve pet"})
		}
		return
	}
	utils.AddSpanAttribute(querySpan, "pet_found", true)
	utils.AddSpanAttribute(querySpan, "pet_name", pet.Name)
	querySpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, pet)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetPet completed",
		zap.String("cpf", cpf),
		zap.Int("pet_id", petID),
		zap.String("pet_name", pet.Name),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetPetClinic godoc
// @Summary Obter dados da clínica credenciada para pets do CPF
// @Description Recupera informações da clínica credenciada associada aos pets do cidadão.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Security BearerAuth
// @Success 200 {object} models.PetClinicResponse "Dados da clínica obtidos com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/pets/clinic [get]
func GetPetClinic(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetPetClinic")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_pet_clinic"),
		attribute.String("service", "pet"),
	)

	logger.Debug("GetPetClinic called", zap.String("cpf", cpf))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Check if pet service is available
	if services.PetServiceInstance == nil {
		logger.Error("pet service not initialized")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Pet service unavailable"})
		return
	}

	// Query clinic data with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "pets", "clinic")
	clinic, err := services.PetServiceInstance.GetPetClinic(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "get_pet_clinic",
			"cpf":       cpf,
		})
		querySpan.End()
		logger.Error("failed to get pet clinic", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve pet clinic data"})
		return
	}
	utils.AddSpanAttribute(querySpan, "has_clinic", clinic.Clinic != nil)
	querySpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, clinic)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetPetClinic completed",
		zap.String("cpf", cpf),
		zap.Bool("has_clinic", clinic.Clinic != nil),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetPetStats godoc
// @Summary Obter estatísticas de pets do CPF
// @Description Recupera as estatísticas de quantidade de pets por tipo associadas ao CPF do cidadão.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Security BearerAuth
// @Success 200 {object} models.PetStatsResponse "Estatísticas obtidas com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/pets/stats [get]
func GetPetStats(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetPetStats")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_pet_stats"),
		attribute.String("service", "pet"),
	)

	logger.Debug("GetPetStats called", zap.String("cpf", cpf))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Check if pet service is available
	if services.PetServiceInstance == nil {
		logger.Error("pet service not initialized")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Pet service unavailable"})
		return
	}

	// Query stats data with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "pets", "stats")
	stats, err := services.PetServiceInstance.GetPetStats(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "get_pet_stats",
			"cpf":       cpf,
		})
		querySpan.End()
		logger.Error("failed to get pet stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve pet statistics"})
		return
	}
	utils.AddSpanAttribute(querySpan, "has_stats", stats.Statistics != nil)
	querySpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, stats)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetPetStats completed",
		zap.String("cpf", cpf),
		zap.Bool("has_stats", stats.Statistics != nil),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
