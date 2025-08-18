package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// BetaGroupHandlers handles beta group and whitelist operations
type BetaGroupHandlers struct {
	logger           *logging.SafeLogger
	betaGroupService *services.BetaGroupService
}

// NewBetaGroupHandlers creates a new beta group handlers instance
func NewBetaGroupHandlers(logger *logging.SafeLogger, betaGroupService *services.BetaGroupService) *BetaGroupHandlers {
	return &BetaGroupHandlers{
		logger:           logger,
		betaGroupService: betaGroupService,
	}
}

// CreateGroup godoc
// @Summary Criar grupo beta
// @Description Cria um novo grupo beta para o chatbot (apenas administradores)
// @Tags Beta Groups
// @Accept json
// @Produce json
// @Param group body models.BetaGroupRequest true "Dados do grupo"
// @Success 201 {object} models.BetaGroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/beta/groups [post]
func (h *BetaGroupHandlers) CreateGroup(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "CreateBetaGroup")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "create_beta_group"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("CreateBetaGroup called")

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "beta_group_request")
	var req models.BetaGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "BetaGroupRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.name", req.Name)
	inputSpan.End()

	// Create group with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "create_group")
	group, err := h.betaGroupService.CreateGroup(ctx, req.Name)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "create_group",
		})
		serviceSpan.End()

		switch err {
		case models.ErrInvalidGroupName, models.ErrGroupNameTooLong:
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNameExists:
			c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to create beta group", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.group_id", group.ID)
	utils.AddSpanAttribute(serviceSpan, "response.group_name", group.Name)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusCreated, group)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("CreateBetaGroup completed",
		zap.String("group_name", req.Name),
		zap.String("group_id", group.ID),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetGroup godoc
// @Summary Obter grupo beta
// @Description Obtém um grupo beta por ID (apenas administradores)
// @Tags Beta Groups
// @Produce json
// @Param group_id path string true "ID do grupo"
// @Success 200 {object} models.BetaGroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/groups/{group_id} [get]
func (h *BetaGroupHandlers) GetGroup(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetBetaGroup")
	defer span.End()

	groupID := c.Param("group_id")

	// Add group ID to span attributes
	span.SetAttributes(
		attribute.String("group_id", groupID),
		attribute.String("operation", "get_beta_group"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("GetBetaGroup called", zap.String("group_id", groupID))

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Validate group ID with tracing
	ctx, idSpan := utils.TraceInputValidation(ctx, "group_id", "group_id")
	if groupID == "" {
		utils.RecordErrorInSpan(idSpan, fmt.Errorf("group ID is required"), map[string]interface{}{
			"group_id": groupID,
		})
		idSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ID do grupo é obrigatório"})
		return
	}
	idSpan.End()

	// Get group with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "get_group")
	group, err := h.betaGroupService.GetGroup(ctx, groupID)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "get_group",
		})
		serviceSpan.End()

		switch err {
		case models.ErrInvalidGroupID:
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to get beta group", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.group_id", group.ID)
	utils.AddSpanAttribute(serviceSpan, "response.group_name", group.Name)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, group)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("GetBetaGroup completed",
		zap.String("group_id", groupID),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// ListGroups godoc
// @Summary Listar grupos beta
// @Description Lista todos os grupos beta com paginação (apenas administradores)
// @Tags Beta Groups
// @Produce json
// @Param page query int false "Página (padrão: 1)"
// @Param per_page query int false "Itens por página (padrão: 10)"
// @Success 200 {object} models.BetaGroupListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Router /admin/beta/groups [get]
func (h *BetaGroupHandlers) ListGroups(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ListBetaGroups")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "list_beta_groups"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("ListBetaGroups called")

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Parse pagination parameters with tracing
	ctx, paginationSpan := utils.TraceInputParsing(ctx, "pagination_parameters")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}
	utils.AddSpanAttribute(paginationSpan, "page", page)
	utils.AddSpanAttribute(paginationSpan, "per_page", perPage)
	paginationSpan.End()

	// List groups with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "list_groups")
	groups, err := h.betaGroupService.ListGroups(ctx, page, perPage)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "list_groups",
		})
		serviceSpan.End()
		h.logger.Error("failed to list beta groups", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.total_groups", groups.TotalGroups)
	utils.AddSpanAttribute(serviceSpan, "response.groups_count", len(groups.Groups))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, groups)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("ListBetaGroups completed",
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int64("total_groups", groups.TotalGroups),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// UpdateGroup godoc
// @Summary Atualizar grupo beta
// @Description Atualiza um grupo beta existente (apenas administradores)
// @Tags Beta Groups
// @Accept json
// @Produce json
// @Param group_id path string true "ID do grupo"
// @Param group body models.BetaGroupRequest true "Dados do grupo"
// @Success 200 {object} models.BetaGroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/beta/groups/{group_id} [put]
func (h *BetaGroupHandlers) UpdateGroup(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateBetaGroup")
	defer span.End()

	groupID := c.Param("group_id")

	// Add group ID to span attributes
	span.SetAttributes(
		attribute.String("group_id", groupID),
		attribute.String("operation", "update_beta_group"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("UpdateBetaGroup called", zap.String("group_id", groupID))

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Validate group ID with tracing
	ctx, idSpan := utils.TraceInputValidation(ctx, "group_id", "group_id")
	if groupID == "" {
		utils.RecordErrorInSpan(idSpan, fmt.Errorf("group ID is required"), map[string]interface{}{
			"group_id": groupID,
		})
		idSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ID do grupo é obrigatório"})
		return
	}
	idSpan.End()

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "beta_group_request")
	var req models.BetaGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "BetaGroupRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.name", req.Name)
	inputSpan.End()

	// Update group with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "update_group")
	group, err := h.betaGroupService.UpdateGroup(ctx, groupID, req.Name)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "update_group",
		})
		serviceSpan.End()

		switch err {
		case models.ErrInvalidGroupID, models.ErrInvalidGroupName, models.ErrGroupNameTooLong:
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNameExists:
			c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to update beta group", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.group_id", group.ID)
	utils.AddSpanAttribute(serviceSpan, "response.group_name", group.Name)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, group)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("UpdateBetaGroup completed",
		zap.String("group_id", groupID),
		zap.String("new_name", req.Name),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// DeleteGroup godoc
// @Summary Excluir grupo beta
// @Description Exclui um grupo beta e remove todas as associações de telefones (apenas administradores)
// @Tags Beta Groups
// @Produce json
// @Param group_id path string true "ID do grupo"
// @Success 204 "Grupo excluído com sucesso"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/groups/{group_id} [delete]
func (h *BetaGroupHandlers) DeleteGroup(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "DeleteBetaGroup")
	defer span.End()

	groupID := c.Param("group_id")

	// Add group ID to span attributes
	span.SetAttributes(
		attribute.String("group_id", groupID),
		attribute.String("operation", "delete_beta_group"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("DeleteBetaGroup called", zap.String("group_id", groupID))

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Validate group ID with tracing
	ctx, idSpan := utils.TraceInputValidation(ctx, "group_id", "group_id")
	if groupID == "" {
		utils.RecordErrorInSpan(idSpan, fmt.Errorf("group ID is required"), map[string]interface{}{
			"group_id": groupID,
		})
		idSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ID do grupo é obrigatório"})
		return
	}
	idSpan.End()

	// Delete group with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "delete_group")
	err = h.betaGroupService.DeleteGroup(ctx, groupID)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "delete_group",
		})
		serviceSpan.End()

		switch err {
		case models.ErrInvalidGroupID:
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to delete beta group", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.Status(http.StatusNoContent)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("DeleteBetaGroup completed",
		zap.String("group_id", groupID),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetBetaStatus godoc
// @Summary Verificar status beta
// @Description Verifica se um número de telefone está na whitelist beta (com cache)
// @Tags Beta Whitelist
// @Produce json
// @Param phone_number path string true "Número de telefone"
// @Success 200 {object} models.BetaStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/beta-status [get]
func (h *BetaGroupHandlers) GetBetaStatus(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetBetaStatus")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "get_beta_status"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("GetBetaStatus called", zap.String("phone_number", phoneNumber))

	// Validate phone number with tracing
	ctx, phoneSpan := utils.TraceInputValidation(ctx, "phone_number", "phone_number")
	if phoneNumber == "" {
		utils.RecordErrorInSpan(phoneSpan, fmt.Errorf("phone number is required"), map[string]interface{}{
			"phone_number": phoneNumber,
		})
		phoneSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}
	phoneSpan.End()

	// Get beta status with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "get_beta_status")
	status, err := h.betaGroupService.GetBetaStatus(ctx, phoneNumber)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "get_beta_status",
		})
		serviceSpan.End()
		h.logger.Error("failed to get beta status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.beta_whitelisted", status.BetaWhitelisted)
	if status.BetaWhitelisted {
		utils.AddSpanAttribute(serviceSpan, "response.group_id", status.GroupID)
		utils.AddSpanAttribute(serviceSpan, "response.group_name", status.GroupName)
	}
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, status)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("GetBetaStatus completed",
		zap.String("phone_number", phoneNumber),
		zap.Duration("total_duration", totalDuration),
		zap.Bool("beta_whitelisted", status.BetaWhitelisted),
		zap.String("status", "success"))
}

// AddToWhitelist godoc
// @Summary Adicionar telefone à whitelist
// @Description Adiciona um número de telefone a um grupo beta (apenas administradores)
// @Tags Beta Whitelist
// @Accept json
// @Produce json
// @Param phone_number path string true "Número de telefone"
// @Param data body models.BetaWhitelistRequest true "Dados da whitelist"
// @Success 200 {object} models.BetaWhitelistResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/beta/whitelist/{phone_number} [post]
func (h *BetaGroupHandlers) AddToWhitelist(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "AddToWhitelist")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "add_to_whitelist"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("AddToWhitelist called", zap.String("phone_number", phoneNumber))

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Validate phone number with tracing
	ctx, phoneSpan := utils.TraceInputValidation(ctx, "phone_number", "phone_number")
	if phoneNumber == "" {
		utils.RecordErrorInSpan(phoneSpan, fmt.Errorf("phone number is required"), map[string]interface{}{
			"phone_number": phoneNumber,
		})
		phoneSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}
	phoneSpan.End()

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "beta_whitelist_request")
	var req models.BetaWhitelistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "BetaWhitelistRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.group_id", req.GroupID)
	inputSpan.End()

	// Add to whitelist with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "add_to_whitelist")
	response, err := h.betaGroupService.AddToWhitelist(ctx, phoneNumber, req.GroupID)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "add_to_whitelist",
		})
		serviceSpan.End()

		switch err {
		case models.ErrInvalidGroupID:
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		case models.ErrPhoneAlreadyWhitelisted:
			c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to add phone to whitelist", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.phone_number", response.PhoneNumber)
	utils.AddSpanAttribute(serviceSpan, "response.group_id", response.GroupID)
	utils.AddSpanAttribute(serviceSpan, "response.group_name", response.GroupName)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("AddToWhitelist completed",
		zap.String("phone_number", phoneNumber),
		zap.String("group_id", req.GroupID),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// RemoveFromWhitelist godoc
// @Summary Remover telefone da whitelist
// @Description Remove um número de telefone da whitelist beta (apenas administradores)
// @Tags Beta Whitelist
// @Produce json
// @Param phone_number path string true "Número de telefone"
// @Success 200 {object} models.BetaWhitelistResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/whitelist/{phone_number} [delete]
func (h *BetaGroupHandlers) RemoveFromWhitelist(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "RemoveFromWhitelist")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "remove_from_whitelist"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("RemoveFromWhitelist called", zap.String("phone_number", phoneNumber))

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Validate phone number with tracing
	ctx, phoneSpan := utils.TraceInputValidation(ctx, "phone_number", "phone_number")
	if phoneNumber == "" {
		utils.RecordErrorInSpan(phoneSpan, fmt.Errorf("phone number is required"), map[string]interface{}{
			"phone_number": phoneNumber,
		})
		phoneSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}
	phoneSpan.End()

	// Remove from whitelist with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "remove_from_whitelist")
	err = h.betaGroupService.RemoveFromWhitelist(ctx, phoneNumber)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "remove_from_whitelist",
		})
		serviceSpan.End()

		switch err {
		case models.ErrPhoneNotWhitelisted:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to remove phone from whitelist", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "operation.success", true)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Phone removed from whitelist successfully"})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("RemoveFromWhitelist completed",
		zap.String("phone_number", phoneNumber),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// ListWhitelistedPhones godoc
// @Summary Listar telefones na whitelist
// @Description Lista telefones na whitelist beta com paginação (apenas administradores)
// @Tags Beta Whitelist
// @Produce json
// @Param page query int false "Página (padrão: 1)"
// @Param per_page query int false "Itens por página (padrão: 10)"
// @Param group_id query string false "Filtrar por ID do grupo"
// @Success 200 {object} models.BetaWhitelistListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Router /admin/beta/whitelist [get]
func (h *BetaGroupHandlers) ListWhitelistedPhones(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ListWhitelistedPhones")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "list_whitelisted_phones"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("ListWhitelistedPhones called")

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Parse pagination parameters with tracing
	ctx, paginationSpan := utils.TraceInputParsing(ctx, "pagination_parameters")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	groupID := c.Query("group_id")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}
	utils.AddSpanAttribute(paginationSpan, "page", page)
	utils.AddSpanAttribute(paginationSpan, "per_page", perPage)
	if groupID != "" {
		utils.AddSpanAttribute(paginationSpan, "filter.group_id", groupID)
	}
	paginationSpan.End()

	// List whitelisted phones with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "list_whitelisted_phones")
	phones, err := h.betaGroupService.ListWhitelistedPhones(ctx, page, perPage, groupID)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "list_whitelisted_phones",
		})
		serviceSpan.End()
		h.logger.Error("failed to list whitelisted phones", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.total_count", phones.TotalCount)
	utils.AddSpanAttribute(serviceSpan, "response.phones_count", len(phones.Whitelisted))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, phones)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("ListWhitelistedPhones completed",
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.String("group_id_filter", groupID),
		zap.Int64("total_count", phones.TotalCount),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// BulkAddToWhitelist godoc
// @Summary Adicionar múltiplos telefones à whitelist
// @Description Adiciona múltiplos números de telefone a um grupo beta (apenas administradores)
// @Tags Beta Whitelist
// @Accept json
// @Produce json
// @Param data body models.BetaWhitelistBulkRequest true "Dados da operação em lote"
// @Success 200 {array} models.BetaWhitelistResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/whitelist/bulk-add [post]
func (h *BetaGroupHandlers) BulkAddToWhitelist(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "BulkAddToWhitelist")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "bulk_add_to_whitelist"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("BulkAddToWhitelist called")

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "beta_whitelist_bulk_request")
	var req models.BetaWhitelistBulkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "BetaWhitelistBulkRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.group_id", req.GroupID)
	utils.AddSpanAttribute(inputSpan, "input.phone_count", len(req.PhoneNumbers))
	inputSpan.End()

	// Bulk add to whitelist with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "bulk_add_to_whitelist")
	response, err := h.betaGroupService.BulkAddToWhitelist(ctx, req.PhoneNumbers, req.GroupID)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "bulk_add_to_whitelist",
		})
		serviceSpan.End()

		switch err {
		case models.ErrInvalidGroupID:
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to bulk add phones to whitelist", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.added_count", len(response))
	utils.AddSpanAttribute(serviceSpan, "response.total_count", len(req.PhoneNumbers))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("BulkAddToWhitelist completed",
		zap.String("group_id", req.GroupID),
		zap.Int("phone_count", len(req.PhoneNumbers)),
		zap.Int("added_count", len(response)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// BulkRemoveFromWhitelist godoc
// @Summary Remover múltiplos telefones da whitelist
// @Description Remove múltiplos números de telefone da whitelist beta (apenas administradores)
// @Tags Beta Whitelist
// @Accept json
// @Produce json
// @Param data body models.BetaWhitelistBulkRemoveRequest true "Dados da operação em lote"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Router /admin/beta/whitelist/bulk-remove [post]
func (h *BetaGroupHandlers) BulkRemoveFromWhitelist(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "BulkRemoveFromWhitelist")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "bulk_remove_from_whitelist"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("BulkRemoveFromWhitelist called")

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "beta_whitelist_bulk_remove_request")
	var req models.BetaWhitelistBulkRemoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "BetaWhitelistBulkRemoveRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.phone_count", len(req.PhoneNumbers))
	inputSpan.End()

	// Bulk remove from whitelist with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "bulk_remove_from_whitelist")
	err = h.betaGroupService.BulkRemoveFromWhitelist(ctx, req.PhoneNumbers)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "bulk_remove_from_whitelist",
		})
		serviceSpan.End()
		h.logger.Error("failed to bulk remove phones from whitelist", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "operation.success", true)
	utils.AddSpanAttribute(serviceSpan, "phones_count", len(req.PhoneNumbers))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Phones removed from whitelist successfully"})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("BulkRemoveFromWhitelist completed",
		zap.Int("phone_count", len(req.PhoneNumbers)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// BulkMoveWhitelist godoc
// @Summary Mover múltiplos telefones entre grupos
// @Description Move múltiplos números de telefone de um grupo beta para outro (apenas administradores)
// @Tags Beta Whitelist
// @Accept json
// @Produce json
// @Param data body models.BetaWhitelistMoveRequest true "Dados da operação de movimentação"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/whitelist/bulk-move [post]
func (h *BetaGroupHandlers) BulkMoveWhitelist(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "BulkMoveWhitelist")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "bulk_move_whitelist"),
		attribute.String("service", "beta_group"),
	)

	h.logger.Debug("BulkMoveWhitelist called")

	// Check admin access with tracing
	ctx, adminSpan := utils.TraceBusinessLogic(ctx, "admin_access_check")
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		utils.RecordErrorInSpan(adminSpan, fmt.Errorf("access denied"), map[string]interface{}{
			"is_admin": isAdmin,
			"error":    err,
		})
		adminSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}
	adminSpan.End()

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "beta_whitelist_move_request")
	var req models.BetaWhitelistMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "BetaWhitelistMoveRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.from_group_id", req.FromGroupID)
	utils.AddSpanAttribute(inputSpan, "input.to_group_id", req.ToGroupID)
	utils.AddSpanAttribute(inputSpan, "input.phone_count", len(req.PhoneNumbers))
	inputSpan.End()

	// Bulk move whitelist with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "beta_group_service", "bulk_move_whitelist")
	err = h.betaGroupService.BulkMoveWhitelist(ctx, req.PhoneNumbers, req.FromGroupID, req.ToGroupID)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "beta_group_service",
			"service.operation": "bulk_move_whitelist",
		})
		serviceSpan.End()

		switch err {
		case models.ErrInvalidGroupID:
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to bulk move whitelist", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "operation.success", true)
	utils.AddSpanAttribute(serviceSpan, "phones_count", len(req.PhoneNumbers))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Phones moved between groups successfully"})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("BulkMoveWhitelist completed",
		zap.String("from_group_id", req.FromGroupID),
		zap.String("to_group_id", req.ToGroupID),
		zap.Int("phone_count", len(req.PhoneNumbers)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

type SuccessResponse struct {
	Message string `json:"message"`
}
