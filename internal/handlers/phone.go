package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nyaruka/phonenumbers"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
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
	Message string `json:"message,omitempty"`
	// DDI (código do país)
	DDI string `json:"ddi,omitempty"`
	// DDD (código de área)
	DDD string `json:"ddd,omitempty"`
	// Número do assinante
	Numero string `json:"numero,omitempty"`
	// Representação E.164 do número
	E164 string `json:"e164,omitempty"`
	// Região ISO 3166-1 alpha-2
	Region string `json:"region,omitempty"`
}

// ValidatePhoneNumber godoc
// @Summary Valida número de telefone
// @Description Valida DDI, DDD e número para qualquer telefone internacional.
// @Tags validation
// @Accept json
// @Produce json
// @Param data body PhoneValidationRequest true "Telefone a ser validado"
// @Success 200 {object} PhoneValidationResponse
// @Failure 400 {object} ErrorResponse
// @Router /validate/phone [post]
func ValidatePhoneNumber(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ValidatePhoneNumber")
	defer span.End()

	var req PhoneValidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "campo phone é obrigatório"})
		return
	}

	logger := logging.Logger.With(zap.String("phone", req.Phone))

	// evitando erro de variável não utilizada
	_ = ctx

	num, err := phonenumbers.Parse(req.Phone, "")
	if err != nil {
		logger.Warn("falha ao parsear número", zap.Error(err))
		c.JSON(http.StatusBadRequest, PhoneValidationResponse{
			Valid:   false,
			Message: "formato de telefone inválido",
		})
		return
	}

	if !phonenumbers.IsValidNumber(num) {
		c.JSON(http.StatusOK, PhoneValidationResponse{
			Valid:   false,
			Message: "telefone inválido",
		})
		return
	}

	countryCode := num.GetCountryCode()
	nationalNumber := phonenumbers.GetNationalSignificantNumber(num)
	region := phonenumbers.GetRegionCodeForNumber(num)

	// Deriva DDD apenas para Brasil (código 55). Para outros países, pode variar.
	ddd := ""
	subscriber := nationalNumber
	if countryCode == 55 && len(nationalNumber) >= 2 {
		ddd = nationalNumber[:2]
		subscriber = nationalNumber[2:]
	}

	c.JSON(http.StatusOK, PhoneValidationResponse{
		Valid:   true,
		Message: "telefone válido",
		DDI:     fmt.Sprintf("%d", countryCode),
		DDD:     ddd,
		Numero:  subscriber,
		E164:    phonenumbers.Format(num, phonenumbers.E164),
		Region:  region,
	})
}
