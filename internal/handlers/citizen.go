package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// GetCitizenData godoc
// @Summary Obter dados do cidadão
// @Description Recupera os dados do cidadão por CPF, combinando dados base com atualizações autodeclaradas. Dados autodeclarados têm precedência sobre dados base. Os resultados são armazenados em cache usando Redis com TTL configurável.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Security BearerAuth
// @Success 200 {object} models.Citizen "Dados completos do cidadão"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf} [get]
func GetCitizenData(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetCitizenData")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("GetCitizenData called", zap.String("cpf", cpf))

	// Try to get from cache first
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("get_citizen").Inc()
		var citizen models.Citizen
		if err := json.Unmarshal([]byte(cachedData), &citizen); err == nil {
			c.JSON(http.StatusOK, citizen)
			return
		}
		logger.Warn("failed to unmarshal cached data", zap.Error(err))
	}

	// Get base data
	var citizen models.Citizen
	err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			observability.DatabaseOperations.WithLabelValues("find", "not_found").Inc()
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "citizen not found"})
			return
		}
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Error("failed to get citizen data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Get self-declared data
	var selfDeclared models.SelfDeclaredData
	err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&selfDeclared)
	if err == nil {
		observability.DatabaseOperations.WithLabelValues("find", "success").Inc()
		logger.Info("found self-declared data",
			zap.Any("email", selfDeclared.Email),
			zap.Any("telefone", selfDeclared.Telefone),
			zap.Any("endereco", selfDeclared.Endereco),
			zap.Any("raca", selfDeclared.Raca),
			zap.Any("raw_data", selfDeclared))

		// Merge self-declared data
		if selfDeclared.Endereco != nil && selfDeclared.Endereco.Principal != nil {
			logger.Info("merging endereco", zap.Any("endereco", selfDeclared.Endereco))
			if citizen.Endereco == nil {
				citizen.Endereco = &models.Endereco{}
			}
			citizen.Endereco.Principal = selfDeclared.Endereco.Principal
			if citizen.Endereco.Indicador == nil {
				citizen.Endereco.Indicador = utils.BoolPtr(true)
			}
		}
		if selfDeclared.Email != nil && selfDeclared.Email.Principal != nil {
			logger.Info("merging email", zap.Any("email", selfDeclared.Email))
			if citizen.Email == nil {
				citizen.Email = &models.Email{}
			}
			citizen.Email.Principal = selfDeclared.Email.Principal
			if citizen.Email.Indicador == nil {
				citizen.Email.Indicador = utils.BoolPtr(true)
			}
		}
		if selfDeclared.Telefone != nil && selfDeclared.Telefone.Principal != nil && selfDeclared.Telefone.Indicador != nil && *selfDeclared.Telefone.Indicador {
			logger.Info("merging telefone", zap.Any("telefone", selfDeclared.Telefone))
			if citizen.Telefone == nil {
				citizen.Telefone = &models.Telefone{}
			}
			citizen.Telefone.Principal = selfDeclared.Telefone.Principal
			citizen.Telefone.Indicador = utils.BoolPtr(true)
		}
		if selfDeclared.Raca != nil {
			logger.Info("merging raca", zap.Any("raca", selfDeclared.Raca))
			citizen.Raca = selfDeclared.Raca
		}
	} else if err != mongo.ErrNoDocuments {
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Warn("failed to get self-declared data", zap.Error(err))
	}

	// Cache the result
	if jsonData, err := json.Marshal(citizen); err == nil {
		config.Redis.Set(ctx, cacheKey, jsonData, config.AppConfig.RedisTTL)
	}

	c.JSON(http.StatusOK, citizen)
}

// Helper: Get merged citizen data (as delivered by /citizen/{cpf})
func getMergedCitizenData(ctx context.Context, cpf string) (*models.Citizen, error) {
	var citizen models.Citizen
	err := config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}
	var selfDeclared models.SelfDeclaredData
	err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&selfDeclared)
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, err
	}
	// Merge logic (same as in GetCitizenData)
	if selfDeclared.Endereco != nil && selfDeclared.Endereco.Principal != nil {
		if citizen.Endereco == nil {
			citizen.Endereco = &models.Endereco{}
		}
		citizen.Endereco.Principal = selfDeclared.Endereco.Principal
		if citizen.Endereco.Indicador == nil {
			citizen.Endereco.Indicador = utils.BoolPtr(true)
		}
	}
	if selfDeclared.Email != nil && selfDeclared.Email.Principal != nil {
		if citizen.Email == nil {
			citizen.Email = &models.Email{}
		}
		citizen.Email.Principal = selfDeclared.Email.Principal
		if citizen.Email.Indicador == nil {
			citizen.Email.Indicador = utils.BoolPtr(true)
		}
	}
	if selfDeclared.Telefone != nil && selfDeclared.Telefone.Principal != nil && selfDeclared.Telefone.Indicador != nil && *selfDeclared.Telefone.Indicador {
		if citizen.Telefone == nil {
			citizen.Telefone = &models.Telefone{}
		}
		citizen.Telefone.Principal = selfDeclared.Telefone.Principal
		citizen.Telefone.Indicador = utils.BoolPtr(true)
	}
	if selfDeclared.Raca != nil {
		citizen.Raca = selfDeclared.Raca
	}
	return &citizen, nil
}

// UpdateSelfDeclaredAddress godoc
// @Summary Atualizar endereço autodeclarado
// @Description Atualiza ou cria o endereço autodeclarado de um cidadão por CPF. Apenas o campo de endereço é atualizado.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Param data body models.SelfDeclaredAddressInput true "Endereço autodeclarado"
// @Security BearerAuth
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/address [put]
func UpdateSelfDeclaredAddress(c *gin.Context) {
	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("UpdateSelfDeclaredAddress called", zap.String("cpf", cpf))

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var input models.SelfDeclaredAddressInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Sanity check: compare with current merged data
	current, err := getMergedCitizenData(c.Request.Context(), cpf)
	if err != nil {
		logger.Error("failed to fetch current data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	if current.Endereco != nil && current.Endereco.Principal != nil &&
		*current.Endereco.Principal.Bairro == input.Bairro &&
		*current.Endereco.Principal.CEP == input.CEP &&
		(current.Endereco.Principal.Complemento == nil && input.Complemento == nil ||
			(current.Endereco.Principal.Complemento != nil && input.Complemento != nil && *current.Endereco.Principal.Complemento == *input.Complemento)) &&
		*current.Endereco.Principal.Estado == input.Estado &&
		*current.Endereco.Principal.Logradouro == input.Logradouro &&
		*current.Endereco.Principal.Municipio == input.Municipio &&
		*current.Endereco.Principal.Numero == input.Numero &&
		(current.Endereco.Principal.TipoLogradouro == nil && input.TipoLogradouro == nil ||
			(current.Endereco.Principal.TipoLogradouro != nil && input.TipoLogradouro != nil && *current.Endereco.Principal.TipoLogradouro == *input.TipoLogradouro)) {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: address matches current data"})
		return
	}

	origem := "self-declared"
	sistema := "rmi"
	endereco := models.Endereco{
		Indicador: utils.BoolPtr(true),
		Principal: &models.EnderecoPrincipal{
			Bairro:         &input.Bairro,
			CEP:            &input.CEP,
			Complemento:    input.Complemento,
			Estado:         &input.Estado,
			Logradouro:     &input.Logradouro,
			Municipio:      &input.Municipio,
			Numero:         &input.Numero,
			TipoLogradouro: input.TipoLogradouro,
			Origem:         &origem,
			Sistema:        &sistema,
		},
	}

	update := bson.M{
		"$set": bson.M{
			"endereco":   endereco,
			"updated_at": time.Now(),
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		c,
		bson.M{"cpf": cpf},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		logger.Error("failed to update self-declared address", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update address: " + err.Error()})
		return
	}

	// Invalidate cache
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	if err := config.Redis.Del(c.Request.Context(), cacheKey).Err(); err != nil {
		logger.Warn("failed to invalidate cache", zap.Error(err))
	}
	c.JSON(http.StatusOK, SuccessResponse{Message: "Self-declared address updated successfully"})
}

// UpdateSelfDeclaredPhone godoc
// @Summary Atualizar telefone autodeclarado
// @Description Atualiza ou cria o telefone autodeclarado de um cidadão por CPF. Apenas o campo de telefone é atualizado (armazenado como pendente até verificado).
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Param data body models.SelfDeclaredPhoneInput true "Telefone autodeclarado"
// @Security BearerAuth
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/phone [put]
func UpdateSelfDeclaredPhone(c *gin.Context) {
	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("UpdateSelfDeclaredPhone called", zap.String("cpf", cpf))

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var input models.SelfDeclaredPhoneInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Sanity check: compare with current merged data
	current, err := getMergedCitizenData(c.Request.Context(), cpf)
	if err != nil {
		logger.Error("failed to fetch current data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	if current.Telefone != nil && current.Telefone.Principal != nil &&
		current.Telefone.Principal.DDI != nil && *current.Telefone.Principal.DDI == input.DDI &&
		current.Telefone.Principal.DDD != nil && *current.Telefone.Principal.DDD == input.DDD &&
		current.Telefone.Principal.Valor != nil && *current.Telefone.Principal.Valor == input.Valor {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: phone matches current data"})
		return
	}

	// Build full phone number for unique index
	fullPhone := input.DDI + input.DDD + input.Valor
	// Delete any previous verification for this CPF (regardless of phone number)
	verColl := config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection)
	_, _ = verColl.DeleteMany(c, bson.M{"cpf": cpf})
	// Generate verification code and store in phone_verifications collection
	code := utils.GenerateVerificationCode()
	verification := models.PhoneVerification{
		CPF: cpf,
		Telefone: &models.Telefone{
			Indicador: utils.BoolPtr(false),
			Principal: &models.TelefonePrincipal{
				DDD:   &input.DDD,
				DDI:   &input.DDI,
				Valor: &input.Valor,
			},
		},
		PhoneNumber: fullPhone,
		Code:        code,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(config.AppConfig.PhoneVerificationTTL),
	}
	_, err = verColl.InsertOne(c, verification)
	if err != nil {
		logger.Error("failed to store phone verification request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to start phone verification: " + err.Error()})
		return
	}
	// Envia código via WhatsApp (DDD pode ser vazio para números internacionais)
	if input.DDI != "" && input.Valor != "" {
		phone := fmt.Sprintf("%s%s%s", input.DDI, input.DDD, input.Valor)
		err = utils.SendVerificationCode(c.Request.Context(), phone, code)
		if err != nil {
			logger.Error("failed to send WhatsApp message", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to send verification code: " + err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, SuccessResponse{Message: "Self-declared phone submitted for validation. Verification code sent."})
}

// UpdateSelfDeclaredEmail godoc
// @Summary Atualizar email autodeclarado
// @Description Atualiza ou cria o email autodeclarado de um cidadão por CPF. Apenas o campo de email é atualizado.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Param data body models.SelfDeclaredEmailInput true "Email autodeclarado"
// @Security BearerAuth
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/email [put]
func UpdateSelfDeclaredEmail(c *gin.Context) {
	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("UpdateSelfDeclaredEmail called", zap.String("cpf", cpf))

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var input models.SelfDeclaredEmailInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Sanity check: compare with current merged data
	current, err := getMergedCitizenData(c.Request.Context(), cpf)
	if err != nil {
		logger.Error("failed to fetch current data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	if current.Email != nil && current.Email.Principal != nil &&
		current.Email.Principal.Valor != nil && *current.Email.Principal.Valor == input.Valor {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: email matches current data"})
		return
	}

	origem := "self-declared"
	sistema := "rmi"
	email := models.Email{
		Indicador: utils.BoolPtr(true),
		Principal: &models.EmailPrincipal{
			Valor:   &input.Valor,
			Origem:  &origem,
			Sistema: &sistema,
		},
	}

	update := bson.M{
		"$set": bson.M{
			"email":      email,
			"updated_at": time.Now(),
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		c,
		bson.M{"cpf": cpf},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		logger.Error("failed to update self-declared email", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update email: " + err.Error()})
		return
	}

	// Invalidate cache
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	if err := config.Redis.Del(c.Request.Context(), cacheKey).Err(); err != nil {
		logger.Warn("failed to invalidate cache", zap.Error(err))
	}
	c.JSON(http.StatusOK, SuccessResponse{Message: "Self-declared email updated successfully"})
}

// UpdateSelfDeclaredRaca godoc
// @Summary Atualizar etnia autodeclarada
// @Description Atualiza ou cria a etnia autodeclarada de um cidadão por CPF. Apenas o campo de etnia é atualizado. O valor deve ser uma das opções válidas retornadas pelo endpoint /citizen/ethnicity/options.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Param data body models.SelfDeclaredRacaInput true "Etnia autodeclarada"
// @Security BearerAuth
// @Success 200 {object} SuccessResponse "Etnia atualizada com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou valor de etnia inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/ethnicity [put]
func UpdateSelfDeclaredRaca(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateSelfDeclaredRaca")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Parse input
	var input models.SelfDeclaredRacaInput
	if err := c.ShouldBindJSON(&input); err != nil {
		logger.Error("failed to parse input", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid input format"})
		return
	}

	// Validate ethnicity
	if !models.IsValidEthnicity(input.Valor) {
		logger.Error("invalid ethnicity value", zap.String("value", input.Valor))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid ethnicity value"})
		return
	}

	// Get existing self-declared data
	var selfDeclared models.SelfDeclaredData
	err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&selfDeclared)
	if err != nil && err != mongo.ErrNoDocuments {
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Error("failed to get self-declared data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	// Update ethnicity
	selfDeclared.CPF = cpf
	selfDeclared.Raca = &input.Valor

	// Upsert the document
	opts := options.Update().SetUpsert(true)
	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		bson.M{"$set": selfDeclared},
		opts,
	)
	if err != nil {
		observability.DatabaseOperations.WithLabelValues("update", "error").Inc()
		logger.Error("failed to update self-declared data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()
	observability.SelfDeclaredUpdates.WithLabelValues("success").Inc()

	// Invalidate cache
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	config.Redis.Del(c.Request.Context(), cacheKey)

	c.JSON(http.StatusOK, SuccessResponse{Message: "ethnicity updated successfully"})
}

// HealthCheck godoc
// @Summary Verificação de saúde
// @Description Verifica a saúde da API e suas dependências (MongoDB e Redis). Retorna status detalhado para cada serviço.
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse "Todos os serviços estão saudáveis"
// @Failure 503 {object} HealthResponse "Um ou mais serviços estão indisponíveis"
// @Router /health [get]
func HealthCheck(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "HealthCheck")
	defer span.End()

	// Try to get from cache first
	cacheKey := "health:status"
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("health_check").Inc()
		var health HealthResponse
		if err := json.Unmarshal([]byte(cachedData), &health); err == nil {
			if health.Status == "healthy" {
				c.JSON(http.StatusOK, health)
			} else {
				c.JSON(http.StatusServiceUnavailable, health)
			}
			return
		}
		observability.Logger().Warn("failed to unmarshal cached health data", zap.Error(err))
	}

	health := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Services:  make(map[string]string),
	}

	// Check MongoDB
	if err := config.MongoDB.Client().Ping(ctx, nil); err != nil {
		health.Status = "unhealthy"
		health.Services["mongodb"] = "unhealthy"
	} else {
		health.Services["mongodb"] = "healthy"
	}

	// Check Redis
	if err := config.Redis.Ping(ctx).Err(); err != nil {
		health.Status = "unhealthy"
		health.Services["redis"] = "unhealthy"
	} else {
		health.Services["redis"] = "healthy"
	}

	// Cache the result with different TTLs based on health status
	healthJSON, err := json.Marshal(health)
	if err == nil {
		ttl := 5 * time.Second // Default TTL for healthy responses
		if health.Status == "unhealthy" {
			ttl = 1 * time.Second // Shorter TTL for unhealthy responses
		}
		if err := config.Redis.Set(ctx, cacheKey, healthJSON, ttl).Err(); err != nil {
			observability.Logger().Warn("failed to cache health status", zap.Error(err))
		}
	}

	if health.Status == "healthy" {
		c.JSON(http.StatusOK, health)
	} else {
		c.JSON(http.StatusServiceUnavailable, health)
	}
}

// GetFirstLogin godoc
// @Summary Obter status do primeiro login
// @Description Verifica se este é o primeiro login do usuário
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Security BearerAuth
// @Success 200 {object} models.UserConfigResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/firstlogin [get]
func GetFirstLogin(c *gin.Context) {
	ctx := c.Request.Context()
	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("GetFirstLogin called", zap.String("cpf", cpf))

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	// Get user config
	var userConfig models.UserConfig
	err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).FindOne(
		ctx,
		bson.M{"cpf": cpf},
	).Decode(&userConfig)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// If no config exists, it's first login
			c.JSON(http.StatusOK, models.UserConfigResponse{FirstLogin: true})
			return
		}
		logger.Error("failed to get user config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get user config"})
		return
	}

	c.JSON(http.StatusOK, models.UserConfigResponse{FirstLogin: userConfig.FirstLogin})
}

// UpdateFirstLogin godoc
// @Summary Atualizar status do primeiro login
// @Description Define o status do primeiro login como falso para um usuário
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Security BearerAuth
// @Success 200 {object} models.UserConfigResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/firstlogin [put]
func UpdateFirstLogin(c *gin.Context) {
	ctx := c.Request.Context()
	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("UpdateFirstLogin called", zap.String("cpf", cpf))

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	update := bson.M{
		"$set": bson.M{
			"first_login": false,
			"updated_at":  time.Now(),
		},
	}

	_, err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		logger.Error("failed to update first login status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update first login status"})
		return
	}

	c.JSON(http.StatusOK, models.UserConfigResponse{FirstLogin: false})
}

// GetOptIn godoc
// @Summary Obter status de opt-in
// @Description Verifica se o usuário optou por receber notificações
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Security BearerAuth
// @Success 200 {object} models.UserConfigOptInResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/optin [get]
func GetOptIn(c *gin.Context) {
	ctx := c.Request.Context()
	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("GetOptIn called", zap.String("cpf", cpf))

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var userConfig models.UserConfig
	err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&userConfig)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// If no config exists, default to opted in
			c.JSON(http.StatusOK, models.UserConfigOptInResponse{OptIn: true})
			return
		}
		logger.Error("failed to get user config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get user config"})
		return
	}

	c.JSON(http.StatusOK, models.UserConfigOptInResponse{OptIn: userConfig.OptIn})
}

// UpdateOptIn godoc
// @Summary Atualizar status de opt-in
// @Description Atualiza o status de opt-in do usuário para notificações
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Param data body models.UserConfigOptInResponse true "Status de opt-in"
// @Security BearerAuth
// @Success 200 {object} models.UserConfigOptInResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/optin [put]
func UpdateOptIn(c *gin.Context) {
	ctx := c.Request.Context()
	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("UpdateOptIn called", zap.String("cpf", cpf))

	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var input models.UserConfigOptInResponse
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	update := bson.M{
		"$set": bson.M{
			"opt_in":     input.OptIn,
			"updated_at": time.Now(),
		},
	}

	_, err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		logger.Error("failed to update opt-in status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update opt-in status"})
		return
	}

	c.JSON(http.StatusOK, input)
}

// GetEthnicityOptions godoc
// @Summary Listar opções de etnia
// @Description Retorna a lista de opções válidas de etnia para autodeclaração. Esta lista é usada para validar as atualizações de etnia autodeclarada.
// @Tags citizen
// @Accept json
// @Produce json
// @Success 200 {array} string "Lista de opções de etnia válidas"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/ethnicity/options [get]
func GetEthnicityOptions(c *gin.Context) {
	_, span := otel.Tracer("").Start(c.Request.Context(), "GetEthnicityOptions")
	defer span.End()

	logger := observability.Logger()
	logger.Info("GetEthnicityOptions called")

	options := models.ValidEthnicityOptions()
	c.JSON(http.StatusOK, options)
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}

type HealthResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Services  map[string]string `json:"services"`
}

type UserConfigResponse struct {
	FirstLogin bool `json:"first_login"`
}

type UserConfigOptInResponse struct {
	OptIn bool `json:"opt_in"`
}
