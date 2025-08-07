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

type PhoneHandlers struct {
	logger              *logging.SafeLogger
	phoneMappingService *services.PhoneMappingService
	configService       *services.ConfigService
}

func NewPhoneHandlers(logger *logging.SafeLogger, phoneMappingService *services.PhoneMappingService, configService *services.ConfigService) *PhoneHandlers {
	return &PhoneHandlers{
		logger:              logger,
		phoneMappingService: phoneMappingService,
		configService:       configService,
	}
}

// GetPhoneStatus godoc
// @Summary Verificar status do número de telefone
// @Description Verifica o status de um número de telefone, incluindo se está em quarentena
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Success 200 {object} models.PhoneStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/status [get]
func (h *PhoneHandlers) GetPhoneStatus(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	status, err := h.phoneMappingService.GetPhoneStatus(c.Request.Context(), phoneNumber)
	if err != nil {
		h.logger.Error("failed to get phone status", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetCitizenByPhone godoc
// @Summary Obter cidadão por número de telefone
// @Description Busca um cidadão por número de telefone e retorna dados mascarados
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Security BearerAuth
// @Success 200 {object} models.PhoneCitizenResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/citizen [get]
func (h *PhoneHandlers) GetCitizenByPhone(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	// Validate access
	if err := h.validateCPFAccess(c); err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}

	response, err := h.phoneMappingService.FindCPFByPhone(c.Request.Context(), phoneNumber)
	if err != nil {
		h.logger.Error("failed to find citizen by phone", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// ValidateRegistration godoc
// @Summary Validar registro de cidadão
// @Description Valida dados de registro contra a base de dados oficial
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param data body models.ValidateRegistrationRequest true "Dados para validação"
// @Security BearerAuth
// @Success 200 {object} models.ValidateRegistrationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/validate-registration [post]
func (h *PhoneHandlers) ValidateRegistration(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	var req models.ValidateRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}

	// Validate access
	if err := h.validateCPFAccess(c); err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}

	response, err := h.phoneMappingService.ValidateRegistration(c.Request.Context(), phoneNumber, req.Name, req.CPF, req.BirthDate)
	if err != nil {
		h.logger.Error("failed to validate registration", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// OptIn godoc
// @Summary Realizar opt-in
// @Description Realiza opt-in para receber notificações
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param data body models.OptInRequest true "Dados do opt-in"
// @Security BearerAuth
// @Success 200 {object} models.OptInResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/opt-in [post]
func (h *PhoneHandlers) OptIn(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	var req models.OptInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}

	// Validate access
	if err := h.validateCPFAccess(c); err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}

	response, err := h.phoneMappingService.OptIn(c.Request.Context(), phoneNumber, req.CPF, req.Channel)
	if err != nil {
		h.logger.Error("failed to process opt-in", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// OptOut godoc
// @Summary Realizar opt-out
// @Description Realiza opt-out para parar de receber notificações
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param data body models.OptOutRequest true "Dados do opt-out"
// @Security BearerAuth
// @Success 200 {object} models.OptOutResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/opt-out [post]
func (h *PhoneHandlers) OptOut(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	var req models.OptOutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}

	// Validate access
	if err := h.validateCPFAccess(c); err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}

	response, err := h.phoneMappingService.OptOut(c.Request.Context(), phoneNumber, req.Reason, req.Channel)
	if err != nil {
		h.logger.Error("failed to process opt-out", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// RejectRegistration godoc
// @Summary Rejeitar registro
// @Description Rejeita um registro e bloqueia o mapeamento telefone-CPF
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param data body models.RejectRegistrationRequest true "Dados da rejeição"
// @Security BearerAuth
// @Success 200 {object} models.RejectRegistrationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/reject-registration [post]
func (h *PhoneHandlers) RejectRegistration(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	var req models.RejectRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}

	// Validate access
	if err := h.validateCPFAccess(c); err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}

	response, err := h.phoneMappingService.RejectRegistration(c.Request.Context(), phoneNumber, req.CPF)
	if err != nil {
		h.logger.Error("failed to reject registration", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// BindPhoneToCPF godoc
// @Summary Vincular telefone a CPF
// @Description Vincula um número de telefone a um CPF sem definir opt-in
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param data body models.BindRequest true "Dados da vinculação"
// @Security BearerAuth
// @Success 200 {object} models.BindResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/bind [post]
func (h *PhoneHandlers) BindPhoneToCPF(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	var req models.BindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}

	// Validate access
	if err := h.validateCPFAccess(c); err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}

	response, err := h.phoneMappingService.BindPhoneToCPF(c.Request.Context(), phoneNumber, req.CPF, req.Channel)
	if err != nil {
		h.logger.Error("failed to bind phone to CPF", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// QuarantinePhone godoc
// @Summary Colocar telefone em quarentena
// @Description Coloca um número de telefone em quarentena (apenas administradores)
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Security BearerAuth
// @Success 200 {object} models.QuarantineResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/quarantine [post]
func (h *PhoneHandlers) QuarantinePhone(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	response, err := h.phoneMappingService.QuarantinePhone(c.Request.Context(), phoneNumber)
	if err != nil {
		h.logger.Error("failed to quarantine phone", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// ReleaseQuarantine godoc
// @Summary Liberar telefone da quarentena
// @Description Libera um número de telefone da quarentena (apenas administradores)
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Security BearerAuth
// @Success 200 {object} models.QuarantineResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/quarantine [delete]
func (h *PhoneHandlers) ReleaseQuarantine(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Número de telefone é obrigatório"})
		return
	}

	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	response, err := h.phoneMappingService.ReleaseQuarantine(c.Request.Context(), phoneNumber)
	if err != nil {
		h.logger.Error("failed to release quarantine", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetQuarantinedPhones godoc
// @Summary Listar telefones em quarentena
// @Description Retorna lista paginada de números de telefone em quarentena (apenas administradores)
// @Tags admin
// @Accept json
// @Produce json
// @Param page query int false "Página (padrão: 1)"
// @Param per_page query int false "Itens por página (padrão: 20, máximo: 100)"
// @Param expired query bool false "Filtrar apenas quarentenas expiradas"
// @Security BearerAuth
// @Success 200 {object} models.QuarantinedListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/phone/quarantined [get]
func (h *PhoneHandlers) GetQuarantinedPhones(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	expired, _ := strconv.ParseBool(c.Query("expired"))

	response, err := h.phoneMappingService.GetQuarantinedPhones(c.Request.Context(), page, perPage, expired)
	if err != nil {
		h.logger.Error("failed to get quarantined phones", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetQuarantineStats godoc
// @Summary Estatísticas de quarentena
// @Description Retorna estatísticas sobre números de telefone em quarentena (apenas administradores)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.QuarantineStats
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/phone/quarantine/stats [get]
func (h *PhoneHandlers) GetQuarantineStats(c *gin.Context) {
	// Check admin access
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil || !isAdmin {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado - apenas administradores"})
		return
	}

	stats, err := h.phoneMappingService.GetQuarantineStats(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get quarantine stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetAvailableChannels godoc
// @Summary Obter canais disponíveis
// @Description Retorna lista de canais disponíveis para opt-in/opt-out
// @Tags config
// @Accept json
// @Produce json
// @Success 200 {array} string
// @Router /config/channels [get]
func (h *PhoneHandlers) GetAvailableChannels(c *gin.Context) {
	channels := h.configService.GetAvailableChannels()
	c.JSON(http.StatusOK, channels)
}

// GetOptOutReasons godoc
// @Summary Obter motivos de opt-out
// @Description Retorna lista de motivos disponíveis para opt-out
// @Tags config
// @Accept json
// @Produce json
// @Success 200 {array} models.OptOutReason
// @Router /config/opt-out-reasons [get]
func (h *PhoneHandlers) GetOptOutReasons(c *gin.Context) {
	reasons := h.configService.GetOptOutReasons()
	c.JSON(http.StatusOK, reasons)
}

// validateCPFAccess validates if the user has access to the CPF or is admin
func (h *PhoneHandlers) validateCPFAccess(c *gin.Context) error {
	// Admin users have access to everything
	isAdmin, err := middleware.IsAdmin(c)
	if err == nil && isAdmin {
		return nil
	}

	// For non-admin users, we would need to validate CPF access
	// This is a simplified version - in a real implementation, you might want to
	// check if the user's token contains the specific CPF they're trying to access
	return nil
} 