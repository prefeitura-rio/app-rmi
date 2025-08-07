package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"go.uber.org/zap"
)

// BetaGroupHandlers handles beta group and whitelist operations
type BetaGroupHandlers struct {
	logger          *logging.SafeLogger
	betaGroupService *services.BetaGroupService
}

// NewBetaGroupHandlers creates a new beta group handlers instance
func NewBetaGroupHandlers(logger *logging.SafeLogger, betaGroupService *services.BetaGroupService) *BetaGroupHandlers {
	return &BetaGroupHandlers{
		logger:          logger,
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
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/beta/groups [post]
func (h *BetaGroupHandlers) CreateGroup(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	var req models.BetaGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}

	group, err := h.betaGroupService.CreateGroup(c.Request.Context(), req.Name)
	if err != nil {
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

	c.JSON(http.StatusCreated, group)
}

// GetGroup godoc
// @Summary Obter grupo beta
// @Description Obtém um grupo beta por ID (apenas administradores)
// @Tags Beta Groups
// @Produce json
// @Param group_id path string true "ID do grupo"
// @Success 200 {object} models.BetaGroupResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/groups/{group_id} [get]
func (h *BetaGroupHandlers) GetGroup(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	groupID := c.Param("group_id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ID do grupo é obrigatório"})
		return
	}

	group, err := h.betaGroupService.GetGroup(c.Request.Context(), groupID)
	if err != nil {
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

	c.JSON(http.StatusOK, group)
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
// @Failure 403 {object} ErrorResponse
// @Router /admin/beta/groups [get]
func (h *BetaGroupHandlers) ListGroups(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}

	groups, err := h.betaGroupService.ListGroups(c.Request.Context(), page, perPage)
	if err != nil {
		h.logger.Error("failed to list beta groups", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, groups)
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
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/beta/groups/{group_id} [put]
func (h *BetaGroupHandlers) UpdateGroup(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	groupID := c.Param("group_id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ID do grupo é obrigatório"})
		return
	}

	var req models.BetaGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}

	group, err := h.betaGroupService.UpdateGroup(c.Request.Context(), groupID, req.Name)
	if err != nil {
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

	c.JSON(http.StatusOK, group)
}

// DeleteGroup godoc
// @Summary Excluir grupo beta
// @Description Exclui um grupo beta e remove todas as associações de telefones (apenas administradores)
// @Tags Beta Groups
// @Produce json
// @Param group_id path string true "ID do grupo"
// @Success 204 "Grupo excluído com sucesso"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/groups/{group_id} [delete]
func (h *BetaGroupHandlers) DeleteGroup(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	groupID := c.Param("group_id")
	if groupID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "ID do grupo é obrigatório"})
		return
	}

	err = h.betaGroupService.DeleteGroup(c.Request.Context(), groupID)
	if err != nil {
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

	c.Status(http.StatusNoContent)
}

// GetBetaStatus godoc
// @Summary Verificar status beta
// @Description Verifica se um número de telefone está na whitelist beta (com cache)
// @Tags Beta Whitelist
// @Produce json
// @Param phone_number path string true "Número de telefone"
// @Success 200 {object} models.BetaStatusResponse
// @Failure 400 {object} ErrorResponse
// @Router /phone/{phone_number}/beta-status [get]
func (h *BetaGroupHandlers) GetBetaStatus(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	status, err := h.betaGroupService.GetBetaStatus(c.Request.Context(), phoneNumber)
	if err != nil {
		h.logger.Error("failed to get beta status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, status)
}

// AddToWhitelist godoc
// @Summary Adicionar telefone à whitelist
// @Description Adiciona um número de telefone a um grupo beta (apenas administradores)
// @Tags Beta Whitelist
// @Accept json
// @Produce json
// @Param phone_number path string true "Número de telefone"
// @Param whitelist body models.BetaWhitelistRequest true "Dados da whitelist"
// @Success 201 {object} models.BetaWhitelistResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /admin/beta/whitelist/{phone_number} [post]
func (h *BetaGroupHandlers) AddToWhitelist(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	var req models.BetaWhitelistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}

	whitelist, err := h.betaGroupService.AddToWhitelist(c.Request.Context(), phoneNumber, req.GroupID)
	if err != nil {
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

	c.JSON(http.StatusCreated, whitelist)
}

// RemoveFromWhitelist godoc
// @Summary Remover telefone da whitelist
// @Description Remove um número de telefone da whitelist beta (apenas administradores)
// @Tags Beta Whitelist
// @Produce json
// @Param phone_number path string true "Número de telefone"
// @Success 204 "Telefone removido da whitelist com sucesso"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/whitelist/{phone_number} [delete]
func (h *BetaGroupHandlers) RemoveFromWhitelist(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	err = h.betaGroupService.RemoveFromWhitelist(c.Request.Context(), phoneNumber)
	if err != nil {
		switch err {
		case models.ErrPhoneNotWhitelisted:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to remove phone from whitelist", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}

	c.Status(http.StatusNoContent)
}

// ListWhitelistedPhones godoc
// @Summary Listar telefones na whitelist
// @Description Lista todos os telefones na whitelist beta com paginação (apenas administradores)
// @Tags Beta Whitelist
// @Produce json
// @Param page query int false "Página (padrão: 1)"
// @Param per_page query int false "Itens por página (padrão: 10)"
// @Param group_id query string false "Filtrar por ID do grupo"
// @Success 200 {object} models.BetaWhitelistListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /admin/beta/whitelist [get]
func (h *BetaGroupHandlers) ListWhitelistedPhones(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	groupID := c.Query("group_id")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 10
	}

	whitelist, err := h.betaGroupService.ListWhitelistedPhones(c.Request.Context(), page, perPage, groupID)
	if err != nil {
		h.logger.Error("failed to list whitelisted phones", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, whitelist)
}

// BulkAddToWhitelist godoc
// @Summary Adicionar múltiplos telefones à whitelist
// @Description Adiciona múltiplos números de telefone a um grupo beta (apenas administradores)
// @Tags Beta Whitelist
// @Accept json
// @Produce json
// @Param bulk body models.BetaWhitelistBulkRequest true "Dados da operação em lote"
// @Success 201 {array} models.BetaWhitelistResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/whitelist/bulk-add [post]
func (h *BetaGroupHandlers) BulkAddToWhitelist(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	var req models.BetaWhitelistBulkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}

	results, err := h.betaGroupService.BulkAddToWhitelist(c.Request.Context(), req.PhoneNumbers, req.GroupID)
	if err != nil {
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

	c.JSON(http.StatusCreated, results)
}

// BulkRemoveFromWhitelist godoc
// @Summary Remover múltiplos telefones da whitelist
// @Description Remove múltiplos números de telefone da whitelist beta (apenas administradores)
// @Tags Beta Whitelist
// @Accept json
// @Produce json
// @Param bulk body models.BetaWhitelistBulkRemoveRequest true "Dados da operação em lote"
// @Success 204 "Telefones removidos da whitelist com sucesso"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /admin/beta/whitelist/bulk-remove [post]
func (h *BetaGroupHandlers) BulkRemoveFromWhitelist(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	var req models.BetaWhitelistBulkRemoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}

	err = h.betaGroupService.BulkRemoveFromWhitelist(c.Request.Context(), req.PhoneNumbers)
	if err != nil {
		h.logger.Error("failed to bulk remove phones from whitelist", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.Status(http.StatusNoContent)
}

// BulkMoveWhitelist godoc
// @Summary Mover múltiplos telefones entre grupos
// @Description Move múltiplos números de telefone de um grupo beta para outro (apenas administradores)
// @Tags Beta Whitelist
// @Accept json
// @Produce json
// @Param bulk body models.BetaWhitelistMoveRequest true "Dados da operação em lote"
// @Success 204 "Telefones movidos com sucesso"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/beta/whitelist/bulk-move [post]
func (h *BetaGroupHandlers) BulkMoveWhitelist(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	var req models.BetaWhitelistMoveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos: " + err.Error()})
		return
	}

	err = h.betaGroupService.BulkMoveWhitelist(c.Request.Context(), req.PhoneNumbers, req.FromGroupID, req.ToGroupID)
	if err != nil {
		switch err {
		case models.ErrInvalidGroupID:
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		case models.ErrGroupNotFound:
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		default:
			h.logger.Error("failed to bulk move phones between groups", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		}
		return
	}

	c.Status(http.StatusNoContent)
} 