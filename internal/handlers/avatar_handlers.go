package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

// AvatarHandlers contains all avatar-related HTTP handlers
type AvatarHandlers struct {
	logger       *logging.SafeLogger
	cacheService *services.CacheService
}

// NewAvatarHandlers creates a new instance of avatar handlers
func NewAvatarHandlers(logger *logging.SafeLogger, cacheService *services.CacheService) *AvatarHandlers {
	return &AvatarHandlers{
		logger:       logger,
		cacheService: cacheService,
	}
}

// ListAvatars godoc
// @Summary Listar avatares disponíveis
// @Description Obtém lista paginada de avatares de foto de perfil disponíveis
// @Tags avatars
// @Accept json
// @Produce json
// @Param page query int false "Número da página (padrão: 1)"
// @Param per_page query int false "Itens por página (padrão: 20, máximo: 100)"
// @Success 200 {object} models.AvatarsListResponse "Lista de avatares obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Parâmetros de paginação inválidos"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /v1/avatars [get]
func (h *AvatarHandlers) ListAvatars(c *gin.Context) {
	// Create context with tracing
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "list_avatars")
	defer span.End()

	// Parse pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	// Validate pagination
	if page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page must be greater than 0"})
		return
	}
	if perPage < 1 || perPage > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Per page must be between 1 and 100"})
		return
	}

	// Get avatars from service
	response, err := services.AvatarServiceInstance.ListAvatars(ctx, page, perPage)
	if err != nil {
		h.logger.Error("failed to list avatars", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve avatars"})
		return
	}

	utils.AddSpanAttribute(span, "avatars_count", len(response.Data))
	utils.AddSpanAttribute(span, "total_avatars", int(response.Total))

	c.JSON(http.StatusOK, response)
}

// CreateAvatar godoc
// @Summary Criar novo avatar
// @Description Cria um novo avatar de foto de perfil (somente administradores)
// @Tags avatars
// @Accept json
// @Produce json
// @Param data body models.AvatarRequest true "Dados do avatar"
// @Security BearerAuth
// @Success 201 {object} models.AvatarResponse "Avatar criado com sucesso"
// @Failure 400 {object} ErrorResponse "Dados de avatar inválidos ou formato incorreto"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - somente administradores"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /v1/avatars [post]
func (h *AvatarHandlers) CreateAvatar(c *gin.Context) {
	// Create context with tracing
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "create_avatar")
	defer span.End()

	// Parse request body
	var request models.AvatarRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.logger.Debug("invalid avatar request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Validate input
	if request.Name == "" || len(request.Name) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar name must be between 1 and 100 characters"})
		return
	}
	if request.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar URL is required"})
		return
	}

	// Create avatar
	avatar, err := services.AvatarServiceInstance.CreateAvatar(ctx, &request)
	if err != nil {
		h.logger.Error("failed to create avatar", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create avatar"})
		return
	}

	utils.AddSpanAttribute(span, "avatar_id", avatar.ID.Hex())
	utils.AddSpanAttribute(span, "avatar_name", avatar.Name)

	c.JSON(http.StatusCreated, avatar.ToResponse())
}

// DeleteAvatar godoc
// @Summary Excluir avatar
// @Description Exclui um avatar de foto de perfil (exclusão suave, somente administradores)
// @Tags avatars
// @Accept json
// @Produce json
// @Param id path string true "ID do avatar"
// @Security BearerAuth
// @Success 204 "Avatar excluído com sucesso"
// @Failure 400 {object} ErrorResponse "ID do avatar é obrigatório ou inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - somente administradores"
// @Failure 404 {object} ErrorResponse "Avatar não encontrado"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /v1/avatars/{id} [delete]
func (h *AvatarHandlers) DeleteAvatar(c *gin.Context) {
	// Create context with tracing
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "delete_avatar")
	defer span.End()

	// Get avatar ID from path
	avatarID := c.Param("id")
	if avatarID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar ID is required"})
		return
	}

	// Delete avatar
	err := services.AvatarServiceInstance.DeleteAvatar(ctx, avatarID)
	if err != nil {
		if err.Error() == "avatar not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Avatar not found"})
			return
		}
		h.logger.Error("failed to delete avatar", zap.Error(err), zap.String("avatar_id", avatarID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete avatar"})
		return
	}

	utils.AddSpanAttribute(span, "avatar_id", avatarID)

	c.Status(http.StatusNoContent)
}

// GetUserAvatar godoc
// @Summary Obter avatar atual do usuário
// @Description Obtém o avatar de foto de perfil atual de um usuário
// @Tags avatars,citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do usuário"
// @Security BearerAuth
// @Success 200 {object} models.UserAvatarResponse "Avatar do usuário obtido com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Usuário não encontrado"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /v1/citizen/{cpf}/avatar [get]
func (h *AvatarHandlers) GetUserAvatar(c *gin.Context) {
	// Create context with tracing
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "get_user_avatar")
	defer span.End()

	// Get CPF from path
	cpf := c.Param("cpf")
	if cpf == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CPF is required"})
		return
	}

	// Validate CPF format
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CPF format"})
		return
	}

	// Get user config from cache/database
	var userConfig models.UserConfig
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)

	if err != nil {
		// If user config doesn't exist, return default (no avatar)
		if err == services.ErrDocumentNotFound {
			response := &models.UserAvatarResponse{
				AvatarID: nil,
				Avatar:   nil,
			}
			c.JSON(http.StatusOK, response)
			return
		}
		h.logger.Error("failed to get user config", zap.Error(err), zap.String("cpf", cpf))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user avatar"})
		return
	}

	response := &models.UserAvatarResponse{
		AvatarID: userConfig.AvatarID,
		Avatar:   nil,
	}

	// If user has an avatar, get avatar details
	if userConfig.AvatarID != nil && *userConfig.AvatarID != "" {
		avatar, err := services.AvatarServiceInstance.GetAvatarByID(ctx, *userConfig.AvatarID)
		if err != nil {
			h.logger.Warn("failed to get avatar details", zap.Error(err),
				zap.String("cpf", cpf), zap.String("avatar_id", *userConfig.AvatarID))
		} else if avatar != nil {
			avatarResponse := avatar.ToResponse()
			response.Avatar = &avatarResponse
		}
	}

	utils.AddSpanAttribute(span, "cpf", cpf)
	if userConfig.AvatarID != nil {
		utils.AddSpanAttribute(span, "avatar_id", *userConfig.AvatarID)
	}

	c.JSON(http.StatusOK, response)
}

// UpdateUserAvatar godoc
// @Summary Atualizar avatar do usuário
// @Description Define ou altera o avatar de foto de perfil de um usuário
// @Tags avatars,citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do usuário"
// @Param data body models.UserAvatarRequest true "Seleção de avatar"
// @Security BearerAuth
// @Success 200 {object} models.UserAvatarResponse "Avatar do usuário atualizado com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou ID de avatar inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Usuário ou avatar não encontrado"
// @Failure 422 {object} ErrorResponse "Dados não processáveis - avatar inexistente ou inativo"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /v1/citizen/{cpf}/avatar [put]
func (h *AvatarHandlers) UpdateUserAvatar(c *gin.Context) {
	// Create context with tracing
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "update_user_avatar")
	defer span.End()

	// Get CPF from path
	cpf := c.Param("cpf")
	if cpf == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CPF is required"})
		return
	}

	// Validate CPF format
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CPF format"})
		return
	}

	// Parse request body
	var request models.UserAvatarRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.logger.Debug("invalid avatar update request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Validate avatar exists if provided
	if request.AvatarID != nil && *request.AvatarID != "" {
		exists, err := services.AvatarServiceInstance.ValidateAvatarExists(ctx, *request.AvatarID)
		if err != nil {
			// Check if it's a validation error (invalid ObjectID format)
			if strings.Contains(err.Error(), "invalid avatar ID") {
				h.logger.Debug("invalid avatar ID format", zap.Error(err), zap.String("avatar_id", *request.AvatarID))
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid avatar ID format"})
				return
			}
			h.logger.Error("failed to validate avatar", zap.Error(err), zap.String("avatar_id", *request.AvatarID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate avatar"})
			return
		}
		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar not found or inactive"})
			return
		}
	}

	// Get existing user config or create new one
	var userConfig models.UserConfig
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)

	if err != nil && err != services.ErrDocumentNotFound {
		h.logger.Error("failed to get user config", zap.Error(err), zap.String("cpf", cpf))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user configuration"})
		return
	}

	// Create new config if doesn't exist
	if err == services.ErrDocumentNotFound {
		userConfig = models.UserConfig{
			CPF:        cpf,
			FirstLogin: true,
			OptIn:      true,
			UpdatedAt:  time.Now(),
		}
	}

	// Update avatar
	userConfig.AvatarID = request.AvatarID
	userConfig.UpdatedAt = time.Now()

	// Save via cache service
	err = h.cacheService.UpdateUserConfig(ctx, cpf, &userConfig)
	if err != nil {
		h.logger.Error("failed to update user avatar", zap.Error(err), zap.String("cpf", cpf))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update avatar"})
		return
	}

	// Prepare response
	response := &models.UserAvatarResponse{
		AvatarID: userConfig.AvatarID,
		Avatar:   nil,
	}

	// Get avatar details if set
	if userConfig.AvatarID != nil && *userConfig.AvatarID != "" {
		avatar, err := services.AvatarServiceInstance.GetAvatarByID(ctx, *userConfig.AvatarID)
		if err != nil {
			h.logger.Warn("failed to get updated avatar details", zap.Error(err),
				zap.String("cpf", cpf), zap.String("avatar_id", *userConfig.AvatarID))
		} else if avatar != nil {
			avatarResponse := avatar.ToResponse()
			response.Avatar = &avatarResponse
		}
	}

	// Log audit trail
	_, auditSpan := utils.TraceAuditLogging(ctx, "update", "avatar_id")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}
	err = utils.LogUserConfigUpdate(ctx, auditCtx, "avatar_id",
		userConfig.AvatarID, request.AvatarID)
	auditSpan.End()
	if err != nil {
		h.logger.Warn("failed to log avatar update audit", zap.Error(err), zap.String("cpf", cpf))
	}

	utils.AddSpanAttribute(span, "cpf", cpf)
	if request.AvatarID != nil {
		utils.AddSpanAttribute(span, "new_avatar_id", *request.AvatarID)
	}

	c.JSON(http.StatusOK, response)
}

// Legacy handlers for global access (similar to existing citizen.go pattern)

// ListAvatars is a global handler function for listing avatars
func ListAvatars(c *gin.Context) {
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "list_avatars_legacy")
	defer span.End()

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	if page < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page must be greater than 0"})
		return
	}
	if perPage < 1 || perPage > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Per page must be between 1 and 100"})
		return
	}

	response, err := services.AvatarServiceInstance.ListAvatars(ctx, page, perPage)
	if err != nil {
		observability.Logger().Error("failed to list avatars", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve avatars"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// CreateAvatar is a global handler function for creating avatars
func CreateAvatar(c *gin.Context) {
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "create_avatar_legacy")
	defer span.End()

	var request models.AvatarRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	if request.Name == "" || len(request.Name) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar name must be between 1 and 100 characters"})
		return
	}
	if request.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar URL is required"})
		return
	}

	avatar, err := services.AvatarServiceInstance.CreateAvatar(ctx, &request)
	if err != nil {
		observability.Logger().Error("failed to create avatar", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create avatar"})
		return
	}

	c.JSON(http.StatusCreated, avatar.ToResponse())
}

// DeleteAvatar is a global handler function for deleting avatars
func DeleteAvatar(c *gin.Context) {
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "delete_avatar_legacy")
	defer span.End()

	avatarID := c.Param("id")
	if avatarID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar ID is required"})
		return
	}

	err := services.AvatarServiceInstance.DeleteAvatar(ctx, avatarID)
	if err != nil {
		if err.Error() == "avatar not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Avatar not found"})
			return
		}
		observability.Logger().Error("failed to delete avatar", zap.Error(err), zap.String("avatar_id", avatarID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete avatar"})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetUserAvatar is a global handler function for getting user avatar
func GetUserAvatar(c *gin.Context) {
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "get_user_avatar_legacy")
	defer span.End()

	cpf := c.Param("cpf")
	if cpf == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CPF is required"})
		return
	}

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CPF format"})
		return
	}

	var userConfig models.UserConfig
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)

	if err != nil {
		if err == services.ErrDocumentNotFound {
			response := &models.UserAvatarResponse{
				AvatarID: nil,
				Avatar:   nil,
			}
			c.JSON(http.StatusOK, response)
			return
		}
		observability.Logger().Error("failed to get user config", zap.Error(err), zap.String("cpf", cpf))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user avatar"})
		return
	}

	response := &models.UserAvatarResponse{
		AvatarID: userConfig.AvatarID,
		Avatar:   nil,
	}

	if userConfig.AvatarID != nil && *userConfig.AvatarID != "" {
		avatar, err := services.AvatarServiceInstance.GetAvatarByID(ctx, *userConfig.AvatarID)
		if err != nil {
			observability.Logger().Warn("failed to get avatar details", zap.Error(err),
				zap.String("cpf", cpf), zap.String("avatar_id", *userConfig.AvatarID))
		} else if avatar != nil {
			avatarResponse := avatar.ToResponse()
			response.Avatar = &avatarResponse
		}
	}

	c.JSON(http.StatusOK, response)
}

// UpdateUserAvatar is a global handler function for updating user avatar
func UpdateUserAvatar(c *gin.Context) {
	ctx := c.Request.Context()
	_, span := utils.TraceBusinessLogic(ctx, "update_user_avatar_legacy")
	defer span.End()

	cpf := c.Param("cpf")
	if cpf == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CPF is required"})
		return
	}

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid CPF format"})
		return
	}

	var request models.UserAvatarRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	if request.AvatarID != nil && *request.AvatarID != "" {
		exists, err := services.AvatarServiceInstance.ValidateAvatarExists(ctx, *request.AvatarID)
		if err != nil {
			// Check if it's a validation error (invalid ObjectID format)
			if strings.Contains(err.Error(), "invalid avatar ID") {
				observability.Logger().Debug("invalid avatar ID format", zap.Error(err), zap.String("avatar_id", *request.AvatarID))
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid avatar ID format"})
				return
			}
			observability.Logger().Error("failed to validate avatar", zap.Error(err), zap.String("avatar_id", *request.AvatarID))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate avatar"})
			return
		}
		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Avatar not found or inactive"})
			return
		}
	}

	var userConfig models.UserConfig
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)

	if err != nil && err != services.ErrDocumentNotFound {
		observability.Logger().Error("failed to get user config", zap.Error(err), zap.String("cpf", cpf))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user configuration"})
		return
	}

	if err == services.ErrDocumentNotFound {
		userConfig = models.UserConfig{
			CPF:        cpf,
			FirstLogin: true,
			OptIn:      true,
			UpdatedAt:  time.Now(),
		}
	}

	userConfig.AvatarID = request.AvatarID
	userConfig.UpdatedAt = time.Now()

	cacheService := services.NewCacheService()
	err = cacheService.UpdateUserConfig(ctx, cpf, &userConfig)
	if err != nil {
		observability.Logger().Error("failed to update user avatar", zap.Error(err), zap.String("cpf", cpf))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update avatar"})
		return
	}

	response := &models.UserAvatarResponse{
		AvatarID: userConfig.AvatarID,
		Avatar:   nil,
	}

	if userConfig.AvatarID != nil && *userConfig.AvatarID != "" {
		avatar, err := services.AvatarServiceInstance.GetAvatarByID(ctx, *userConfig.AvatarID)
		if err != nil {
			observability.Logger().Warn("failed to get updated avatar details", zap.Error(err),
				zap.String("cpf", cpf), zap.String("avatar_id", *userConfig.AvatarID))
		} else if avatar != nil {
			avatarResponse := avatar.ToResponse()
			response.Avatar = &avatarResponse
		}
	}

	_, auditSpan := utils.TraceAuditLogging(ctx, "update", "avatar_id")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}
	err = utils.LogUserConfigUpdate(ctx, auditCtx, "avatar_id",
		userConfig.AvatarID, request.AvatarID)
	auditSpan.End()
	if err != nil {
		observability.Logger().Warn("failed to log avatar update audit", zap.Error(err), zap.String("cpf", cpf))
	}

	c.JSON(http.StatusOK, response)
}
