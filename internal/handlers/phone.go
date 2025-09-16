package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nyaruka/phonenumbers"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

// PhoneValidationRequest representa a requisição para validação de telefone
// swagger:model
// @description Estrutura de entrada contendo o número de telefone a ser validado.
type PhoneValidationRequest struct {
	// Número de telefone em formato internacional ou nacional.
	// example: "+55 11 99988-7766"
	Phone string `json:"phone" binding:"required"`
}

// PhoneValidationResponse representa a resposta da validação de telefone
// swagger:model
// @description Resultado da validação, contendo a decomposição (DDI, DDD, número) quando válida.
type PhoneValidationResponse struct {
	// Indica se o número é válido.
	Valid bool `json:"valid"`
	// Mensagem de retorno.
	Message string `json:"message"`
	// DDI (código do país)
	DDI string `json:"ddi"`
	// DDD (código de área)
	DDD string `json:"ddd"`
	// Número do assinante
	Numero string `json:"numero"`
	// Representação E.164 do número
	E164 string `json:"e164"`
	// Região ISO 3166-1 alpha-2
	Region string `json:"region"`
}

// ValidatePhoneNumber godoc
// @Summary Valida número de telefone
// @Description Valida DDI, DDD e número para qualquer telefone internacional.
// @Tags validation
// @Accept json
// @Produce json
// @Param data body PhoneValidationRequest true "Telefone a ser validado"
// @Success 200 {object} PhoneValidationResponse "Telefone validado com sucesso"
// @Failure 400 {object} ErrorResponse "Campo telefone é obrigatório ou formato inválido"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /validate/phone [post]
func ValidatePhoneNumber(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ValidatePhoneNumber")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "validate_phone_number"),
		attribute.String("service", "phone_validation"),
	)

	var req PhoneValidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(span, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "PhoneValidationRequest",
		})
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "campo phone é obrigatório"})
		return
	}

	// Add phone number to span attributes
	span.SetAttributes(attribute.String("input.phone", req.Phone))

	logger := logging.Logger.With(zap.String("phone", req.Phone))
	logger.Debug("ValidatePhoneNumber called", zap.String("phone", req.Phone))

	// Parse phone number with tracing
	ctx, parseSpan := utils.TraceBusinessLogic(ctx, "parse_phone_number")
	num, err := phonenumbers.Parse(req.Phone, "")
	if err != nil {
		utils.RecordErrorInSpan(parseSpan, err, map[string]interface{}{
			"parse.phone": req.Phone,
		})
		parseSpan.End()
		logger.Warn("falha ao parsear número", zap.Error(err))
		c.JSON(http.StatusBadRequest, PhoneValidationResponse{
			Valid:   false,
			Message: "formato de telefone inválido",
		})
		return
	}
	utils.AddSpanAttribute(parseSpan, "parse.success", true)
	parseSpan.End()

	// Validate phone number with basic checks (accept both 8-digit and 9-digit Brazilian numbers)
	ctx, validationSpan := utils.TraceBusinessLogic(ctx, "validate_phone_number")
	nationalNumber := phonenumbers.GetNationalSignificantNumber(num)

	// Basic validation: check reasonable length for phone numbers
	if len(nationalNumber) < 8 || len(nationalNumber) > 15 {
		utils.AddSpanAttribute(validationSpan, "validation.valid", false)
		utils.AddSpanAttribute(validationSpan, "validation.reason", "invalid_length")
		utils.AddSpanAttribute(validationSpan, "national_number_length", len(nationalNumber))
		validationSpan.End()
		c.JSON(http.StatusOK, PhoneValidationResponse{
			Valid:   false,
			Message: "telefone inválido",
		})
		return
	}
	utils.AddSpanAttribute(validationSpan, "validation.valid", true)
	utils.AddSpanAttribute(validationSpan, "national_number_length", len(nationalNumber))
	validationSpan.End()

	// Extract phone components with tracing
	ctx, extractSpan := utils.TraceBusinessLogic(ctx, "extract_phone_components")
	countryCode := num.GetCountryCode()
	// nationalNumber already declared above
	region := phonenumbers.GetRegionCodeForNumber(num)

	// Deriva DDD apenas para Brasil (código 55). Para outros países, pode variar.
	ddd := ""
	subscriber := nationalNumber
	if countryCode == 55 && len(nationalNumber) >= 2 {
		ddd = nationalNumber[:2]
		subscriber = nationalNumber[2:]
	}

	utils.AddSpanAttribute(extractSpan, "country_code", countryCode)
	utils.AddSpanAttribute(extractSpan, "national_number", nationalNumber)
	utils.AddSpanAttribute(extractSpan, "region", region)
	utils.AddSpanAttribute(extractSpan, "ddd", ddd)
	utils.AddSpanAttribute(extractSpan, "subscriber", subscriber)
	extractSpan.End()

	// Build response with tracing
	ctx, responseSpan := utils.TraceBusinessLogic(ctx, "build_validation_response")
	response := PhoneValidationResponse{
		Valid:   true,
		Message: "telefone válido",
		DDI:     fmt.Sprintf("%d", countryCode),
		DDD:     ddd,
		Numero:  subscriber,
		E164:    phonenumbers.Format(num, phonenumbers.E164),
		Region:  region,
	}
	responseSpan.End()

	// Serialize response with tracing
	_, serializeSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	serializeSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("ValidatePhoneNumber completed",
		zap.String("phone", req.Phone),
		zap.Bool("valid", true),
		zap.String("country_code", fmt.Sprintf("%d", countryCode)),
		zap.String("region", region),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
