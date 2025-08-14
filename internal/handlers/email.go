package handlers

import (
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

// EmailValidationRequest representa a requisição para validação de email
// swagger:model
// @description Estrutura de entrada contendo o endereço de email a ser validado.
type EmailValidationRequest struct {
	// Endereço de email a ser validado.
	// example: "usuario@exemplo.com"
	Email string `json:"email" binding:"required"`
}

// EmailValidationResponse representa a resposta da validação de email
// swagger:model
// @description Resultado da validação, contendo informações sobre o endereço de email quando válido.
type EmailValidationResponse struct {
	// Indica se o email é válido.
	Valid bool `json:"valid"`
	// Mensagem de retorno.
	Message string `json:"message"`
	// Parte local do email (antes do @)
	LocalPart string `json:"local_part,omitempty"`
	// Domínio do email (após o @)
	Domain string `json:"domain,omitempty"`
	// Email normalizado (lowercase)
	Normalized string `json:"normalized,omitempty"`
	// Tipo de validação aplicada
	ValidationType string `json:"validation_type,omitempty"`
}

// ValidateEmailAddress godoc
// @Summary Valida endereço de email
// @Description Valida formato e estrutura de endereços de email, retornando informações detalhadas sobre o endereço quando válido.
// @Tags validation
// @Accept json
// @Produce json
// @Param data body EmailValidationRequest true "Email a ser validado"
// @Success 200 {object} EmailValidationResponse
// @Failure 400 {object} ErrorResponse
// @Router /validate/email [post]
func ValidateEmailAddress(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ValidateEmailAddress")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "validate_email_address"),
		attribute.String("service", "email_validation"),
	)

	var req EmailValidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(span, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "EmailValidationRequest",
		})
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "campo email é obrigatório"})
		return
	}

	// Add email to span attributes (sanitized for logging)
	emailForLogging := req.Email
	if len(emailForLogging) > 50 {
		emailForLogging = emailForLogging[:47] + "..."
	}
	span.SetAttributes(attribute.String("input.email_prefix", emailForLogging))

	logger := logging.Logger.With(zap.String("email_prefix", emailForLogging))
	logger.Debug("ValidateEmailAddress called")

	// Basic format validation with tracing
	ctx, formatSpan := utils.TraceBusinessLogic(ctx, "validate_email_format")
	
	// Trim whitespace and normalize
	email := strings.TrimSpace(req.Email)
	if email == "" {
		utils.AddSpanAttribute(formatSpan, "validation.error", "empty_email")
		formatSpan.End()
		c.JSON(http.StatusBadRequest, EmailValidationResponse{
			Valid:   false,
			Message: "email não pode estar vazio",
		})
		return
	}

	// Length validation
	if len(email) > 254 {
		utils.AddSpanAttribute(formatSpan, "validation.error", "email_too_long")
		utils.AddSpanAttribute(formatSpan, "email.length", len(email))
		formatSpan.End()
		logger.Warn("email muito longo", zap.Int("length", len(email)))
		c.JSON(http.StatusOK, EmailValidationResponse{
			Valid:   false,
			Message: "email muito longo (máximo 254 caracteres)",
		})
		return
	}

	utils.AddSpanAttribute(formatSpan, "email.length", len(email))
	formatSpan.End()

	// Parse email using Go's standard library with tracing
	ctx, parseSpan := utils.TraceBusinessLogic(ctx, "parse_email_address")
	addr, err := mail.ParseAddress(email)
	if err != nil {
		utils.RecordErrorInSpan(parseSpan, err, map[string]interface{}{
			"parse.email": emailForLogging,
		})
		parseSpan.End()
		logger.Warn("falha ao parsear email", zap.Error(err))
		c.JSON(http.StatusOK, EmailValidationResponse{
			Valid:   false,
			Message: "formato de email inválido",
		})
		return
	}
	utils.AddSpanAttribute(parseSpan, "parse.success", true)
	parseSpan.End()

	// Additional validation with tracing
	ctx, validationSpan := utils.TraceBusinessLogic(ctx, "detailed_email_validation")
	
	// Split email into local and domain parts
	parts := strings.Split(addr.Address, "@")
	if len(parts) != 2 {
		utils.AddSpanAttribute(validationSpan, "validation.error", "invalid_structure")
		validationSpan.End()
		c.JSON(http.StatusOK, EmailValidationResponse{
			Valid:   false,
			Message: "estrutura de email inválida",
		})
		return
	}

	localPart := parts[0]
	domain := parts[1]

	// Validate local part
	if len(localPart) == 0 || len(localPart) > 64 {
		utils.AddSpanAttribute(validationSpan, "validation.error", "invalid_local_part_length")
		utils.AddSpanAttribute(validationSpan, "local_part.length", len(localPart))
		validationSpan.End()
		c.JSON(http.StatusOK, EmailValidationResponse{
			Valid:   false,
			Message: "parte local do email inválida (deve ter 1-64 caracteres)",
		})
		return
	}

	// Validate domain
	if len(domain) == 0 || len(domain) > 253 {
		utils.AddSpanAttribute(validationSpan, "validation.error", "invalid_domain_length")
		utils.AddSpanAttribute(validationSpan, "domain.length", len(domain))
		validationSpan.End()
		c.JSON(http.StatusOK, EmailValidationResponse{
			Valid:   false,
			Message: "domínio do email inválido (deve ter 1-253 caracteres)",
		})
		return
	}

	// Check for valid domain structure
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		utils.AddSpanAttribute(validationSpan, "validation.error", "invalid_domain_structure")
		validationSpan.End()
		c.JSON(http.StatusOK, EmailValidationResponse{
			Valid:   false,
			Message: "domínio não pode começar ou terminar com ponto",
		})
		return
	}

	// Check for consecutive dots
	if strings.Contains(domain, "..") {
		utils.AddSpanAttribute(validationSpan, "validation.error", "consecutive_dots")
		validationSpan.End()
		c.JSON(http.StatusOK, EmailValidationResponse{
			Valid:   false,
			Message: "domínio não pode conter pontos consecutivos",
		})
		return
	}

	// Check if domain has at least one dot
	if !strings.Contains(domain, ".") {
		utils.AddSpanAttribute(validationSpan, "validation.error", "no_domain_extension")
		validationSpan.End()
		c.JSON(http.StatusOK, EmailValidationResponse{
			Valid:   false,
			Message: "domínio deve conter pelo menos um ponto",
		})
		return
	}

	utils.AddSpanAttribute(validationSpan, "validation.valid", true)
	utils.AddSpanAttribute(validationSpan, "local_part.length", len(localPart))
	utils.AddSpanAttribute(validationSpan, "domain.length", len(domain))
	validationSpan.End()

	// Build response with tracing
	ctx, responseSpan := utils.TraceBusinessLogic(ctx, "build_validation_response")
	normalized := strings.ToLower(addr.Address)
	
	response := EmailValidationResponse{
		Valid:          true,
		Message:        "email válido",
		LocalPart:      localPart,
		Domain:         domain,
		Normalized:     normalized,
		ValidationType: "format_and_structure",
	}
	responseSpan.End()

	// Serialize response with tracing
	_, serializeSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	serializeSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("ValidateEmailAddress completed",
		zap.Bool("valid", true),
		zap.String("domain", domain),
		zap.Int("local_part_length", len(localPart)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}