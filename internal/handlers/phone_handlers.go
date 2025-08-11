package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
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
// @Summary Obter status do telefone
// @Description Obtém o status de um número de telefone (quarentena, CPF vinculado, etc.)
// @Tags phone
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Success 200 {object} models.PhoneStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/status [get]
func (h *PhoneHandlers) GetPhoneStatus(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetPhoneStatus")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "get_phone_status"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("GetPhoneStatus called", zap.String("phone_number", phoneNumber))

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

	// Get phone status with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "get_phone_status")
	response, err := h.phoneMappingService.GetPhoneStatus(ctx, phoneNumber)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "get_phone_status",
		})
		serviceSpan.End()
		h.logger.Error("failed to get phone status", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.found", response.Found)
	utils.AddSpanAttribute(serviceSpan, "response.quarantined", response.Quarantined)
	utils.AddSpanAttribute(serviceSpan, "response.beta_whitelisted", response.BetaWhitelisted)
	if response.Found {
		utils.AddSpanAttribute(serviceSpan, "response.cpf", response.CPF)
		utils.AddSpanAttribute(serviceSpan, "response.name", response.Name)
	}
	if response.Quarantined && response.QuarantineUntil != nil {
		utils.AddSpanAttribute(serviceSpan, "response.quarantine_until", response.QuarantineUntil.String())
	}
	if response.BetaWhitelisted {
		utils.AddSpanAttribute(serviceSpan, "response.beta_group_id", response.BetaGroupID)
		utils.AddSpanAttribute(serviceSpan, "response.beta_group_name", response.BetaGroupName)
	}
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("GetPhoneStatus completed",
		zap.String("phone_number", phoneNumber),
		zap.Bool("found", response.Found),
		zap.Bool("quarantined", response.Quarantined),
		zap.Bool("beta_whitelisted", response.BetaWhitelisted),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetCitizenByPhone godoc
// @Summary Obter cidadão por telefone
// @Description Obtém informações do cidadão associado a um número de telefone
// @Tags phone
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Security BearerAuth
// @Success 200 {object} models.PhoneCitizenResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/citizen [get]
func (h *PhoneHandlers) GetCitizenByPhone(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetCitizenByPhone")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "get_citizen_by_phone"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("GetCitizenByPhone called", zap.String("phone_number", phoneNumber))

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

	// Validate access with tracing
	ctx, accessSpan := utils.TraceBusinessLogic(ctx, "validate_cpf_access")
	if err := h.validateCPFAccess(c); err != nil {
		utils.RecordErrorInSpan(accessSpan, err, map[string]interface{}{
			"access.type": "cpf_validation",
		})
		accessSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}
	accessSpan.End()

	// Get citizen by phone with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "get_citizen_by_phone")
	response, err := h.phoneMappingService.FindCPFByPhone(ctx, phoneNumber)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "get_citizen_by_phone",
		})
		serviceSpan.End()
		h.logger.Error("failed to get citizen by phone", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.found", response.Found)
	if response.Found {
		utils.AddSpanAttribute(serviceSpan, "response.cpf", response.CPF)
		utils.AddSpanAttribute(serviceSpan, "response.name", response.Name)
		utils.AddSpanAttribute(serviceSpan, "response.first_name", response.FirstName)
	}
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("GetCitizenByPhone completed",
		zap.String("phone_number", phoneNumber),
		zap.Bool("found", response.Found),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// ValidateRegistration godoc
// @Summary Validar registro
// @Description Valida um registro de usuário contra dados base
// @Tags phone
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param data body models.ValidateRegistrationRequest true "Dados do registro"
// @Security BearerAuth
// @Success 200 {object} models.ValidateRegistrationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/validate-registration [post]
func (h *PhoneHandlers) ValidateRegistration(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ValidateRegistration")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "validate_registration"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("ValidateRegistration called", zap.String("phone_number", phoneNumber))

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
	ctx, inputSpan := utils.TraceInputParsing(ctx, "validate_registration_request")
	var req models.ValidateRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "ValidateRegistrationRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.name", req.Name)
	utils.AddSpanAttribute(inputSpan, "input.cpf", req.CPF)
	utils.AddSpanAttribute(inputSpan, "input.birth_date", req.BirthDate)
	utils.AddSpanAttribute(inputSpan, "input.channel", req.Channel)
	inputSpan.End()

	// Validate access with tracing
	ctx, accessSpan := utils.TraceBusinessLogic(ctx, "validate_cpf_access")
	if err := h.validateCPFAccess(c); err != nil {
		utils.RecordErrorInSpan(accessSpan, err, map[string]interface{}{
			"access.type": "cpf_validation",
		})
		accessSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}
	accessSpan.End()

	// Process validation with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "validate_registration")
	response, err := h.phoneMappingService.ValidateRegistration(ctx, phoneNumber, req.Name, req.CPF, req.BirthDate)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "validate_registration",
		})
		serviceSpan.End()
		h.logger.Error("failed to validate registration", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.valid", response.Valid)
	if response.Valid {
		utils.AddSpanAttribute(serviceSpan, "response.matched_cpf", response.MatchedCPF)
		utils.AddSpanAttribute(serviceSpan, "response.matched_name", response.MatchedName)
	}
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("ValidateRegistration completed",
		zap.String("phone_number", phoneNumber),
		zap.Duration("total_duration", totalDuration),
		zap.Bool("validation_result", response.Valid),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "PhoneOptIn")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "phone_opt_in"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("PhoneOptIn called", zap.String("phone_number", phoneNumber))

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
	ctx, inputSpan := utils.TraceInputParsing(ctx, "opt_in_request")
	var req models.OptInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "OptInRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.cpf", req.CPF)
	utils.AddSpanAttribute(inputSpan, "input.channel", req.Channel)
	inputSpan.End()

	// Validate access with tracing
	ctx, accessSpan := utils.TraceBusinessLogic(ctx, "validate_cpf_access")
	if err := h.validateCPFAccess(c); err != nil {
		utils.RecordErrorInSpan(accessSpan, err, map[string]interface{}{
			"access.type": "cpf_validation",
		})
		accessSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}
	accessSpan.End()

	// Process opt-in with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "opt_in")
	response, err := h.phoneMappingService.OptIn(ctx, phoneNumber, req.CPF, req.Channel)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "opt_in",
		})
		serviceSpan.End()
		h.logger.Error("failed to process opt-in", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.status", response.Status)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("PhoneOptIn completed",
		zap.String("phone_number", phoneNumber),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "PhoneOptOut")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "phone_opt_out"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("PhoneOptOut called", zap.String("phone_number", phoneNumber))

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
	ctx, inputSpan := utils.TraceInputParsing(ctx, "opt_out_request")
	var req models.OptOutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "OptOutRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.channel", req.Channel)
	utils.AddSpanAttribute(inputSpan, "input.reason", req.Reason)
	inputSpan.End()

	// Validate access with tracing
	ctx, accessSpan := utils.TraceBusinessLogic(ctx, "validate_cpf_access")
	if err := h.validateCPFAccess(c); err != nil {
		utils.RecordErrorInSpan(accessSpan, err, map[string]interface{}{
			"access.type": "cpf_validation",
		})
		accessSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}
	accessSpan.End()

	// Process opt-out with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "opt_out")
	response, err := h.phoneMappingService.OptOut(ctx, phoneNumber, req.Reason, req.Channel)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "opt_out",
		})
		serviceSpan.End()
		h.logger.Error("failed to process opt-out", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.status", response.Status)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("PhoneOptOut completed",
		zap.String("phone_number", phoneNumber),
		zap.String("reason", req.Reason),
		zap.String("channel", req.Channel),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "RejectRegistration")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "reject_registration"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("RejectRegistration called", zap.String("phone_number", phoneNumber))

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
	ctx, inputSpan := utils.TraceInputParsing(ctx, "reject_registration_request")
	var req models.RejectRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "RejectRegistrationRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.cpf", req.CPF)
	utils.AddSpanAttribute(inputSpan, "input.channel", req.Channel)
	utils.AddSpanAttribute(inputSpan, "input.reason", req.Reason)
	inputSpan.End()

	// Validate access with tracing
	ctx, accessSpan := utils.TraceBusinessLogic(ctx, "validate_cpf_access")
	if err := h.validateCPFAccess(c); err != nil {
		utils.RecordErrorInSpan(accessSpan, err, map[string]interface{}{
			"access.type": "cpf_validation",
		})
		accessSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}
	accessSpan.End()

	// Process rejection with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "reject_registration")
	response, err := h.phoneMappingService.RejectRegistration(ctx, phoneNumber, req.CPF)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "reject_registration",
		})
		serviceSpan.End()
		h.logger.Error("failed to reject registration", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.status", response.Status)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("RejectRegistration completed",
		zap.String("phone_number", phoneNumber),
		zap.String("cpf", req.CPF),
		zap.String("channel", req.Channel),
		zap.String("reason", req.Reason),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "BindPhoneToCPF")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "bind_phone_to_cpf"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("BindPhoneToCPF called", zap.String("phone_number", phoneNumber))

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
	ctx, inputSpan := utils.TraceInputParsing(ctx, "bind_request")
	var req models.BindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "BindRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dados inválidos"})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.cpf", req.CPF)
	utils.AddSpanAttribute(inputSpan, "input.channel", req.Channel)
	inputSpan.End()

	// Validate access with tracing
	ctx, accessSpan := utils.TraceBusinessLogic(ctx, "validate_cpf_access")
	if err := h.validateCPFAccess(c); err != nil {
		utils.RecordErrorInSpan(accessSpan, err, map[string]interface{}{
			"access.type": "cpf_validation",
		})
		accessSpan.End()
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Acesso negado"})
		return
	}
	accessSpan.End()

	// Process binding with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "bind_phone_to_cpf")
	response, err := h.phoneMappingService.BindPhoneToCPF(ctx, phoneNumber, req.CPF, req.Channel)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "bind_phone_to_cpf",
		})
		serviceSpan.End()
		h.logger.Error("failed to bind phone to CPF", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.status", response.Status)
	utils.AddSpanAttribute(serviceSpan, "response.cpf", response.CPF)
	utils.AddSpanAttribute(serviceSpan, "response.opt_in", response.OptIn)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("BindPhoneToCPF completed",
		zap.String("phone_number", phoneNumber),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "QuarantinePhone")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "quarantine_phone"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("QuarantinePhone called", zap.String("phone_number", phoneNumber))

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

	// Quarantine phone with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "quarantine_phone")
	response, err := h.phoneMappingService.QuarantinePhone(ctx, phoneNumber)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "quarantine_phone",
		})
		serviceSpan.End()
		h.logger.Error("failed to quarantine phone", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.status", response.Status)
	utils.AddSpanAttribute(serviceSpan, "response.quarantine_until", response.QuarantineUntil.String())
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("QuarantinePhone completed",
		zap.String("phone_number", phoneNumber),
		zap.String("status", response.Status),
		zap.Time("quarantine_until", response.QuarantineUntil),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// ReleaseQuarantine godoc
// @Summary Liberar telefone da quarentena
// @Description Libera um número de telefone da quarentena (apenas administradores)
// @Tags phone
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Security BearerAuth
// @Success 200 {object} models.QuarantineResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /phone/{phone_number}/quarantine [delete]
func (h *PhoneHandlers) ReleaseQuarantine(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ReleaseQuarantine")
	defer span.End()

	phoneNumber := c.Param("phone_number")

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "release_quarantine"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("ReleaseQuarantine called", zap.String("phone_number", phoneNumber))

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

	// Release quarantine with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "release_quarantine")
	response, err := h.phoneMappingService.ReleaseQuarantine(ctx, phoneNumber)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "release_quarantine",
		})
		serviceSpan.End()
		h.logger.Error("failed to release quarantine", zap.Error(err), zap.String("phone_number", phoneNumber))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.status", response.Status)
	utils.AddSpanAttribute(serviceSpan, "response.quarantine_until", response.QuarantineUntil.String())
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("ReleaseQuarantine completed",
		zap.String("phone_number", phoneNumber),
		zap.String("status", response.Status),
		zap.Time("quarantine_until", response.QuarantineUntil),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetQuarantinedPhones godoc
// @Summary Listar telefones em quarentena
// @Description Lista todos os telefones em quarentena com paginação (apenas administradores)
// @Tags phone
// @Produce json
// @Param page query int false "Página (padrão: 1)"
// @Param per_page query int false "Itens por página (padrão: 10)"
// @Success 200 {object} models.QuarantinedListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Router /admin/phone/quarantined [get]
func (h *PhoneHandlers) GetQuarantinedPhones(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetQuarantinedPhones")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "get_quarantined_phones"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("GetQuarantinedPhones called")

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

	// Get quarantined phones with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "get_quarantined_phones")
	response, err := h.phoneMappingService.GetQuarantinedPhones(ctx, page, perPage, false) // false = not expired
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "get_quarantined_phones",
		})
		serviceSpan.End()
		h.logger.Error("failed to get quarantined phones", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.total_count", response.Pagination.Total)
	utils.AddSpanAttribute(serviceSpan, "response.phones_count", len(response.Data))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("GetQuarantinedPhones completed",
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int("total_count", response.Pagination.Total),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetQuarantineStats godoc
// @Summary Obter estatísticas de quarentena
// @Description Obtém estatísticas sobre telefones em quarentena (apenas administradores)
// @Tags phone
// @Produce json
// @Success 200 {object} models.QuarantineStats
// @Failure 403 {object} ErrorResponse
// @Router /admin/phone/quarantine/stats [get]
func (h *PhoneHandlers) GetQuarantineStats(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetQuarantineStats")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "get_quarantine_stats"),
		attribute.String("service", "phone"),
	)

	h.logger.Info("GetQuarantineStats called")

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

	// Get quarantine stats with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "get_quarantine_stats")
	response, err := h.phoneMappingService.GetQuarantineStats(ctx)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "phone_mapping_service",
			"service.operation": "get_quarantine_stats",
		})
		serviceSpan.End()
		h.logger.Error("failed to get quarantine stats", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Erro interno do servidor"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "response.total_quarantined", response.TotalQuarantined)
	utils.AddSpanAttribute(serviceSpan, "response.expired_quarantines", response.ExpiredQuarantines)
	utils.AddSpanAttribute(serviceSpan, "response.active_quarantines", response.ActiveQuarantines)
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("GetQuarantineStats completed",
		zap.Int("total_quarantined", response.TotalQuarantined),
		zap.Int("expired_quarantines", response.ExpiredQuarantines),
		zap.Int("active_quarantines", response.ActiveQuarantines),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetAvailableChannels godoc
// @Summary Obter canais disponíveis
// @Description Obtém a lista de canais disponíveis para comunicação
// @Tags config
// @Produce json
// @Success 200 {array} string
// @Router /config/channels [get]
func (h *PhoneHandlers) GetAvailableChannels(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetAvailableChannels")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "get_available_channels"),
		attribute.String("service", "config"),
	)

	h.logger.Info("GetAvailableChannels called")

	// Get available channels with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "get_available_channels")
	channels := h.configService.GetAvailableChannels()
	utils.AddSpanAttribute(serviceSpan, "response.channels_count", len(channels.Channels))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, channels)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("GetAvailableChannels completed",
		zap.Int("channels_count", len(channels.Channels)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetOptOutReasons godoc
// @Summary Obter motivos de opt-out
// @Description Obtém a lista de motivos válidos para opt-out
// @Tags config
// @Produce json
// @Success 200 {array} string
// @Router /config/opt-out-reasons [get]
func (h *PhoneHandlers) GetOptOutReasons(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetOptOutReasons")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "get_opt_out_reasons"),
		attribute.String("service", "config"),
	)

	h.logger.Info("GetOptOutReasons called")

	// Get opt-out reasons with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "phone_mapping_service", "get_opt_out_reasons")
	reasons := h.configService.GetOptOutReasons()
	utils.AddSpanAttribute(serviceSpan, "response.reasons_count", len(reasons.Reasons))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, reasons)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Info("GetOptOutReasons completed",
		zap.Int("reasons_count", len(reasons.Reasons)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
