package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// GetLegalEntities godoc
// @Summary Obter entidades jurídicas associadas ao CPF
// @Description Recupera a lista paginada de entidades jurídicas (pessoas jurídicas) associadas ao CPF do cidadão. A busca é feita através do campo 'cpf_socio' no array 'socios' de cada entidade.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param page query int false "Número da página (padrão: 1)" minimum(1)
// @Param per_page query int false "Itens por página (padrão: 10, máximo: 100)" minimum(1) maximum(100)
// @Param natureza_juridica_id query string false "Filtro opcional por ID da natureza jurídica"
// @Security BearerAuth
// @Success 200 {object} models.PaginatedLegalEntities "Lista paginada de entidades jurídicas obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou parâmetros de paginação inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/legal-entities [get]
func GetLegalEntities(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetLegalEntities")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_legal_entities"),
		attribute.String("service", "legal_entity"),
	)

	logger.Debug("GetLegalEntities called", zap.String("cpf", cpf))

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

	// Get optional legal nature filter
	var legalNatureID *string
	if legalNatureParam := c.Query("natureza_juridica_id"); legalNatureParam != "" {
		legalNatureID = &legalNatureParam
		span.SetAttributes(attribute.String("legal_nature_filter", legalNatureParam))
	}

	// Check if legal entity service is available
	if services.LegalEntityServiceInstance == nil {
		logger.Error("legal entity service not initialized")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Legal entity service unavailable"})
		return
	}

	// Query legal entities with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "legal_entities", "cpf_partners")
	entities, err := services.LegalEntityServiceInstance.GetLegalEntitiesByCPF(ctx, cpf, page, perPage, legalNatureID)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "get_legal_entities_by_cpf",
			"cpf":       cpf,
			"page":      page,
			"per_page":  perPage,
		})
		querySpan.End()
		logger.Error("failed to get legal entities", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve legal entities"})
		return
	}
	utils.AddSpanAttribute(querySpan, "entities_found", len(entities.Data))
	utils.AddSpanAttribute(querySpan, "total_count", entities.Pagination.Total)
	querySpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, entities)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetLegalEntities completed",
		zap.String("cpf", cpf),
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int("total_count", entities.Pagination.Total),
		zap.Int("entities_returned", len(entities.Data)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetLegalEntityByCNPJ godoc
// @Summary Obter informações de uma entidade jurídica por CNPJ
// @Description Recupera informações detalhadas de uma entidade jurídica (pessoa jurídica) pelo CNPJ. O acesso é permitido para administradores ou se o CPF do JWT estiver como responsável ou sócio da entidade.
// @Tags legal-entity
// @Accept json
// @Produce json
// @Param cnpj path string true "CNPJ da entidade (14 dígitos)" minLength(14) maxLength(14)
// @Security BearerAuth
// @Success 200 {object} models.LegalEntity "Informações da entidade jurídica obtidas com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CNPJ inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - sem permissão para visualizar esta entidade"
// @Failure 404 {object} ErrorResponse "Entidade jurídica não encontrada"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /legal-entity/{cnpj} [get]
func GetLegalEntityByCNPJ(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetLegalEntityByCNPJ")
	defer span.End()

	cnpj := c.Param("cnpj")
	logger := observability.Logger().With(zap.String("cnpj", cnpj))

	// Add CNPJ to span attributes
	span.SetAttributes(
		attribute.String("cnpj", cnpj),
		attribute.String("operation", "get_legal_entity_by_cnpj"),
		attribute.String("service", "legal_entity"),
	)

	logger.Debug("GetLegalEntityByCNPJ called", zap.String("cnpj", cnpj))

	// Validate CNPJ with tracing
	ctx, cnpjSpan := utils.TraceInputValidation(ctx, "cnpj_format", "cnpj")
	if !utils.ValidateCNPJ(cnpj) {
		utils.RecordErrorInSpan(cnpjSpan, fmt.Errorf("invalid CNPJ format"), map[string]interface{}{
			"cnpj": cnpj,
		})
		cnpjSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CNPJ format"})
		return
	}
	cnpjSpan.End()

	// Check if legal entity service is available
	if services.LegalEntityServiceInstance == nil {
		logger.Error("legal entity service not initialized")
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Legal entity service unavailable"})
		return
	}

	// Query legal entity with tracing
	ctx, querySpan := utils.TraceDatabaseFind(ctx, "legal_entities", "cnpj")
	entity, err := services.LegalEntityServiceInstance.GetLegalEntityByCNPJ(ctx, cnpj)
	if err != nil {
		utils.RecordErrorInSpan(querySpan, err, map[string]interface{}{
			"operation": "get_legal_entity_by_cnpj",
			"cnpj":      cnpj,
		})
		querySpan.End()

		if err.Error() == "legal entity not found" {
			logger.Warn("legal entity not found", zap.String("cnpj", cnpj))
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Legal entity not found"})
			return
		}

		logger.Error("failed to get legal entity", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to retrieve legal entity"})
		return
	}
	querySpan.End()

	// Check access permissions with tracing
	ctx, accessSpan := utils.TraceBusinessLogic(ctx, "validate_legal_entity_access")

	// Get authenticated CPF from claims
	claims, exists := c.Get("claims")
	if !exists {
		utils.RecordErrorInSpan(accessSpan, fmt.Errorf("claims not found in context"), nil)
		accessSpan.End()
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
		return
	}

	jwtClaims, ok := claims.(*models.JWTClaims)
	if !ok {
		utils.RecordErrorInSpan(accessSpan, fmt.Errorf("invalid claims type"), nil)
		accessSpan.End()
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
		return
	}

	// Check if user is admin
	isAdmin := false
	for _, role := range jwtClaims.ResourceAccess.Superapp.Roles {
		if role == "go:admin" {
			isAdmin = true
			utils.AddSpanAttribute(accessSpan, "is_admin", true)
			break
		}
	}

	// If admin, allow access
	if isAdmin {
		utils.AddSpanAttribute(accessSpan, "access_granted", "admin")
		accessSpan.End()
		observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

		_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
		c.JSON(http.StatusOK, entity)
		responseSpan.End()

		logger.Debug("GetLegalEntityByCNPJ completed (admin access)",
			zap.String("cnpj", cnpj),
			zap.Duration("total_duration", time.Since(startTime)),
			zap.String("status", "success"))
		return
	}

	// Get CPF from token (stored in PreferredUsername)
	authenticatedCPF := jwtClaims.PreferredUsername
	if authenticatedCPF == "" {
		utils.RecordErrorInSpan(accessSpan, fmt.Errorf("CPF not found in token"), nil)
		accessSpan.End()
		logger.Warn("access denied - CPF not in token", zap.String("cnpj", cnpj))
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied"})
		return
	}
	utils.AddSpanAttribute(accessSpan, "authenticated_cpf", authenticatedCPF)

	// Check if CPF is the responsible person
	if entity.ResponsiblePerson.CPF == authenticatedCPF {
		utils.AddSpanAttribute(accessSpan, "access_granted", "responsible_person")
		accessSpan.End()
		observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

		_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
		c.JSON(http.StatusOK, entity)
		responseSpan.End()

		logger.Debug("GetLegalEntityByCNPJ completed (responsible person access)",
			zap.String("cnpj", cnpj),
			zap.String("cpf", authenticatedCPF),
			zap.Duration("total_duration", time.Since(startTime)),
			zap.String("status", "success"))
		return
	}

	// Check if CPF is in partners list
	for _, partner := range entity.Partners {
		if partner.PartnerCPF != nil && *partner.PartnerCPF == authenticatedCPF {
			utils.AddSpanAttribute(accessSpan, "access_granted", "partner")
			accessSpan.End()
			observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, entity)
			responseSpan.End()

			logger.Debug("GetLegalEntityByCNPJ completed (partner access)",
				zap.String("cnpj", cnpj),
				zap.String("cpf", authenticatedCPF),
				zap.Duration("total_duration", time.Since(startTime)),
				zap.String("status", "success"))
			return
		}
	}

	// Access denied
	utils.RecordErrorInSpan(accessSpan, fmt.Errorf("CPF not authorized to access this legal entity"), map[string]interface{}{
		"authenticated_cpf": authenticatedCPF,
		"cnpj":              cnpj,
	})
	accessSpan.End()

	logger.Warn("access denied - CPF not in responsible person or partners",
		zap.String("cnpj", cnpj),
		zap.String("cpf", authenticatedCPF))
	c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied - you are not authorized to view this legal entity"})
}
