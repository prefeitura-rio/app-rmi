package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"go.uber.org/zap"
)

// PhoneHandlers contains all phone-based endpoint handlers
type PhoneHandlers struct {
	phoneMappingService *services.PhoneMappingService
	configService       *services.ConfigService
	logger              *logging.SafeLogger
}

// NewPhoneHandlers creates a new PhoneHandlers instance
func NewPhoneHandlers(phoneMappingService *services.PhoneMappingService, configService *services.ConfigService) *PhoneHandlers {
	return &PhoneHandlers{
		phoneMappingService: phoneMappingService,
		configService:       configService,
		logger:              observability.Logger(),
	}
}

// GetCitizenByPhone godoc
// @Summary Obter cidadão por número de telefone
// @Description Busca um cidadão por número de telefone e retorna dados mascarados
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Phone number"
// @Success 200 {object} models.PhoneCitizenResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/citizen [get]
func (h *PhoneHandlers) GetCitizenByPhone(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	logger := h.logger.With(zap.String("phone_number", phoneNumber))
	logger.Info("GetCitizenByPhone called")

	response, err := h.phoneMappingService.FindCPFByPhone(c.Request.Context(), phoneNumber)
	if err != nil {
		logger.Error("failed to find citizen by phone", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to find citizen: " + err.Error()})
		return
	}

	if !response.Found {
		c.JSON(http.StatusNotFound, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

// ValidateRegistration godoc
// @Summary Validar dados de registro
// @Description Valida nome, CPF e data de nascimento contra dados base
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Phone number"
// @Param data body models.ValidateRegistrationRequest true "Registration data"
// @Success 200 {object} models.ValidateRegistrationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/validate-registration [post]
func (h *PhoneHandlers) ValidateRegistration(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	logger := h.logger.With(zap.String("phone_number", phoneNumber))
	logger.Info("ValidateRegistration called")

	var req models.ValidateRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	response, err := h.phoneMappingService.ValidateRegistration(c.Request.Context(), phoneNumber, &req)
	if err != nil {
		logger.Error("failed to validate registration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to validate registration: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// OptIn godoc
// @Summary Opt-in para um número de telefone
// @Description Processa opt-in para um número de telefone com validação de CPF
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Phone number"
// @Param data body models.OptInRequest true "Opt-in data"
// @Security BearerAuth
// @Success 200 {object} models.OptInResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/opt-in [post]
func (h *PhoneHandlers) OptIn(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	logger := h.logger.With(zap.String("phone_number", phoneNumber))
	logger.Info("OptIn called")

	var req models.OptInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Validate JWT token and CPF access
	if err := h.validateCPFAccess(c, req.CPF); err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied"})
		return
	}

	response, err := h.phoneMappingService.OptIn(c.Request.Context(), phoneNumber, &req)
	if err != nil {
		logger.Error("failed to process opt-in", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to process opt-in: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// OptOut godoc
// @Summary Opt-out para um número de telefone
// @Description Processa opt-out para um número de telefone
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Phone number"
// @Param data body models.OptOutRequest true "Opt-out data"
// @Security BearerAuth
// @Success 200 {object} models.OptOutResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/opt-out [post]
func (h *PhoneHandlers) OptOut(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	logger := h.logger.With(zap.String("phone_number", phoneNumber))
	logger.Info("OptOut called")

	var req models.OptOutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// For opt-out, we need to get the CPF from the phone mapping first
	// This will be handled in the service, but we need to validate access
	// We'll validate access after getting the CPF from the mapping

	response, err := h.phoneMappingService.OptOut(c.Request.Context(), phoneNumber, &req)
	if err != nil {
		logger.Error("failed to process opt-out", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to process opt-out: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// RejectRegistration godoc
// @Summary Rejeitar registro
// @Description Rejeita um registro e bloqueia o mapeamento phone-CPF
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Phone number"
// @Param data body models.RejectRegistrationRequest true "Rejection data"
// @Security BearerAuth
// @Success 200 {object} models.RejectRegistrationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/reject-registration [post]
func (h *PhoneHandlers) RejectRegistration(c *gin.Context) {
	phoneNumber := c.Param("phone_number")
	logger := h.logger.With(zap.String("phone_number", phoneNumber))
	logger.Info("RejectRegistration called")

	var req models.RejectRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Validate JWT token and CPF access
	if err := h.validateCPFAccess(c, req.CPF); err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Access denied"})
		return
	}

	response, err := h.phoneMappingService.RejectRegistration(c.Request.Context(), phoneNumber, &req)
	if err != nil {
		logger.Error("failed to reject registration", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to reject registration: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// GetAvailableChannels godoc
// @Summary Obter canais disponíveis
// @Description Retorna a lista de canais de comunicação disponíveis
// @Tags config
// @Accept json
// @Produce json
// @Success 200 {object} models.ChannelsResponse
// @Router /config/channels [get]
func (h *PhoneHandlers) GetAvailableChannels(c *gin.Context) {
	response := h.configService.GetAvailableChannels()
	c.JSON(http.StatusOK, response)
}

// GetOptOutReasons godoc
// @Summary Obter motivos de opt-out
// @Description Retorna a lista de motivos disponíveis para opt-out
// @Tags config
// @Accept json
// @Produce json
// @Success 200 {object} models.OptOutReasonsResponse
// @Router /config/opt-out-reasons [get]
func (h *PhoneHandlers) GetOptOutReasons(c *gin.Context) {
	response := h.configService.GetOptOutReasons()
	c.JSON(http.StatusOK, response)
}

// validateCPFAccess validates that the JWT token has access to the specified CPF
func (h *PhoneHandlers) validateCPFAccess(c *gin.Context, cpf string) error {
	// Extract CPF from JWT token
	tokenCPF, err := middleware.ExtractCPFFromToken(c)
	if err != nil {
		return err
	}

	// Check if user is admin or if CPF matches
	isAdmin, err := middleware.IsAdmin(c)
	if err != nil {
		return err
	}

	if cpf != tokenCPF && !isAdmin {
		return middleware.ErrAccessDenied
	}

	return nil
} 