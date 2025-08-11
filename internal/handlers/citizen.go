package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
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
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// GetCitizenData godoc
// @Summary Obter dados do cidadão
// @Description Obtém os dados completos de um cidadão, incluindo dados autodeclarados
// @Tags citizen
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Security BearerAuth
// @Success 200 {object} models.Citizen
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse
// @Router /citizen/{cpf} [get]
func GetCitizenData(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetCitizenData")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_citizen_data"),
		attribute.String("service", "citizen"),
	)

	logger.Info("GetCitizenData called", zap.String("cpf", cpf))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Check cache with tracing
	ctx, cacheSpan := utils.TraceCacheGet(ctx, fmt.Sprintf("citizen:%s", cpf))
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		utils.AddSpanAttribute(cacheSpan, "cache.hit", true)
		observability.CacheHits.WithLabelValues("citizen_data").Inc()
		var citizen models.Citizen
		if err := json.Unmarshal([]byte(cachedData), &citizen); err == nil {
			cacheSpan.End()

			// Serialize response with tracing
			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, citizen)
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Info("GetCitizenData completed (cache hit)",
				zap.String("cpf", cpf),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
		utils.AddSpanAttribute(cacheSpan, "cache.unmarshal_error", err.Error())
		observability.Logger().Warn("failed to unmarshal cached citizen data", zap.Error(err))
	}
	utils.AddSpanAttribute(cacheSpan, "cache.hit", false)
	cacheSpan.End()

	// Get citizen data from database with tracing
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.CitizenCollection, "cpf")
	var citizen models.Citizen
	err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
	if err != nil {
		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.CitizenCollection,
			"db.filter":     "cpf",
		})
		dbSpan.End()
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Citizen not found"})
			return
		}
		logger.Error("failed to get citizen data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}
	utils.AddSpanAttribute(dbSpan, "response.citizen_found", true)
	dbSpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Get self-declared data with tracing
	ctx, selfDeclaredSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.SelfDeclaredCollection, "cpf")
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

		// Merge self-declared data with tracing
		_, mergeSpan := utils.TraceBusinessLogic(ctx, "merge_self_declared_data")
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
		mergeSpan.End()
	} else if err != mongo.ErrNoDocuments {
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Warn("failed to get self-declared data", zap.Error(err))
	}
	selfDeclaredSpan.End()

	// Cache the result with tracing
	ctx, cacheSetSpan := utils.TraceCacheSet(ctx, cacheKey, config.AppConfig.RedisTTL)
	if jsonData, err := json.Marshal(citizen); err == nil {
		config.Redis.Set(ctx, cacheKey, jsonData, config.AppConfig.RedisTTL)
		utils.AddSpanAttribute(cacheSetSpan, "cache.set_success", true)
	} else {
		utils.RecordErrorInSpan(cacheSetSpan, err, map[string]interface{}{
			"cache.operation": "set",
		})
	}
	cacheSetSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, citizen)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("GetCitizenData completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateSelfDeclaredAddress")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "update_address"),
		attribute.String("service", "citizen"),
	)

	logger.Info("UpdateSelfDeclaredAddress called", zap.String("cpf", cpf))

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "address")
	var input models.SelfDeclaredAddressInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "SelfDeclaredAddressInput",
		})
		inputSpan.End()
		logger.Error("failed to parse input", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.bairro", input.Bairro)
	utils.AddSpanAttribute(inputSpan, "input.cep", input.CEP)
	utils.AddSpanAttribute(inputSpan, "input.estado", input.Estado)
	utils.AddSpanAttribute(inputSpan, "input.logradouro", input.Logradouro)
	utils.AddSpanAttribute(inputSpan, "input.municipio", input.Municipio)
	utils.AddSpanAttribute(inputSpan, "input.numero", input.Numero)
	inputSpan.End()

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Get current data for comparison with tracing
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.CitizenCollection, "cpf")
	current, err := getMergedCitizenData(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.CitizenCollection,
			"db.filter":     "cpf",
		})
		findSpan.End()
		logger.Error("failed to fetch current data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	findSpan.End()

	// Compare data with tracing
	ctx, compareSpan := utils.TraceDataComparison(ctx, "address_comparison")
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
		compareSpan.End()
		c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: address matches current data"})
		return
	}
	compareSpan.End()

	// Build address object with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_address_object")
	origem := "self-declared"
	sistema := "rmi"
	now := time.Now()
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
			UpdatedAt:      &now,
		},
	}
	buildSpan.End()

	// Update database with tracing
	ctx, updateSpan := utils.TraceDatabaseUpsert(ctx, config.AppConfig.SelfDeclaredCollection, "cpf")
	update := bson.M{
		"$set": bson.M{
			"endereco":   endereco,
			"updated_at": time.Now(),
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
		utils.GetUpdateOptionsWithWriteConcern("user_data", true),
	)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.SelfDeclaredCollection,
			"db.operation":  "upsert",
		})
		updateSpan.End()
		logger.Error("failed to update self-declared address", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update address: " + err.Error()})
		return
	}
	updateSpan.End()

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()

	// Invalidate cache with tracing
	ctx, cacheSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("citizen:%s", cpf))
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		utils.RecordErrorInSpan(cacheSpan, err, map[string]interface{}{
			"cache.key": cacheKey,
		})
		logger.Warn("failed to invalidate cache", zap.Error(err))
	}
	cacheSpan.End()

	// Log audit event with tracing
	ctx, auditSpan := utils.TraceAuditLogging(ctx, "update", "address")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}

	oldValue := "none"
	if current.Endereco != nil && current.Endereco.Principal != nil {
		oldValue = fmt.Sprintf("%s, %s, %s",
			*current.Endereco.Principal.Logradouro,
			*current.Endereco.Principal.Numero,
			*current.Endereco.Principal.Bairro)
	}

	newValue := fmt.Sprintf("%s, %s, %s", input.Logradouro, input.Numero, input.Bairro)

	err = utils.LogAddressUpdate(ctx, auditCtx, oldValue, newValue)
	if err != nil {
		utils.RecordErrorInSpan(auditSpan, err, map[string]interface{}{
			"audit.action":   "update",
			"audit.resource": "address",
		})
		logger.Warn("failed to log audit event", zap.Error(err))
	}
	auditSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Self-declared address updated successfully"})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("UpdateSelfDeclaredAddress completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateSelfDeclaredPhone")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "update_phone"),
		attribute.String("service", "citizen"),
	)

	logger.Info("UpdateSelfDeclaredPhone called", zap.String("cpf", cpf))

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "phone")
	var input models.SelfDeclaredPhoneInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "SelfDeclaredPhoneInput",
		})
		inputSpan.End()
		logger.Error("failed to parse input", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.ddi", input.DDI)
	utils.AddSpanAttribute(inputSpan, "input.ddd", input.DDD)
	utils.AddSpanAttribute(inputSpan, "input.valor", input.Valor)
	inputSpan.End()

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Get current data for comparison with tracing
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.CitizenCollection, "cpf")
	current, err := getMergedCitizenData(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.CitizenCollection,
			"db.filter":     "cpf",
		})
		findSpan.End()
		logger.Error("failed to fetch current data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	findSpan.End()

	// Compare data with tracing
	ctx, compareSpan := utils.TraceDataComparison(ctx, "phone_comparison")
	if current.Telefone != nil && current.Telefone.Principal != nil &&
		current.Telefone.Principal.DDI != nil && *current.Telefone.Principal.DDI == input.DDI &&
		current.Telefone.Principal.DDD != nil && *current.Telefone.Principal.DDD == input.DDD &&
		current.Telefone.Principal.Valor != nil && *current.Telefone.Principal.Valor == input.Valor {
		compareSpan.End()
		c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: phone matches current data"})
		return
	}
	compareSpan.End()

	// Build full phone number with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_phone_number")
	fullPhone := input.DDI + input.DDD + input.Valor
	buildSpan.End()

	// Delete previous verifications with tracing
	ctx, deleteSpan := utils.TraceDatabaseUpdate(ctx, config.AppConfig.PhoneVerificationCollection, "cpf", false)
	verColl := config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection)
	_, err = verColl.DeleteMany(ctx, bson.M{"cpf": cpf})
	if err != nil {
		utils.RecordErrorInSpan(deleteSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.PhoneVerificationCollection,
			"db.operation":  "delete_many",
		})
		logger.Warn("failed to delete previous verifications", zap.Error(err))
	}
	deleteSpan.End()

	// Generate verification code with tracing
	ctx, codeSpan := utils.TraceBusinessLogic(ctx, "generate_verification_code")
	code := utils.GenerateVerificationCode()
	now := time.Now()
	expiresAt := now.Add(config.AppConfig.PhoneVerificationTTL)
	codeSpan.End()

	// Prepare verification data with tracing
	ctx, dataSpan := utils.TraceBusinessLogic(ctx, "prepare_verification_data")
	verificationData := utils.PhoneVerificationData{
		CPF:         cpf,
		DDI:         input.DDI,
		DDD:         input.DDD,
		Valor:       input.Valor,
		PhoneNumber: fullPhone,
		Code:        code,
		ExpiresAt:   expiresAt,
	}
	dataSpan.End()

	// Create verification record with tracing
	ctx, createSpan := utils.TraceDatabaseUpdate(ctx, config.AppConfig.PhoneVerificationCollection, "cpf", false)
	if err := utils.CreatePhoneVerification(ctx, verificationData); err != nil {
		utils.RecordErrorInSpan(createSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.PhoneVerificationCollection,
			"db.operation":  "create",
		})
		createSpan.End()
		logger.Error("failed to create phone verification", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to start phone verification: " + err.Error()})
		return
	}
	createSpan.End()

	// Update pending phone with tracing
	ctx, updateSpan := utils.TraceDatabaseUpdate(ctx, config.AppConfig.SelfDeclaredCollection, "cpf", false)
	if err := utils.UpdateSelfDeclaredPendingPhone(ctx, cpf, verificationData); err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.SelfDeclaredCollection,
			"db.operation":  "update_pending_phone",
		})
		updateSpan.End()
		logger.Error("failed to update pending phone", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update pending phone: " + err.Error()})
		return
	}
	updateSpan.End()

	// Log audit event with tracing
	ctx, auditSpan := utils.TraceAuditLogging(ctx, "update", "phone")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}

	oldValue := "none"
	if current.Telefone != nil && current.Telefone.Principal != nil {
		oldValue = fmt.Sprintf("%s%s%s",
			*current.Telefone.Principal.DDI,
			*current.Telefone.Principal.DDD,
			*current.Telefone.Principal.Valor)
	}

	newValue := fullPhone

	err = utils.LogPhoneUpdate(ctx, auditCtx, oldValue, newValue)
	if err != nil {
		utils.RecordErrorInSpan(auditSpan, err, map[string]interface{}{
			"audit.action":   "update",
			"audit.resource": "phone",
		})
		logger.Warn("failed to log audit event", zap.Error(err))
	}
	auditSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Self-declared phone submitted for validation. Verification code sent."})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("UpdateSelfDeclaredPhone completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateSelfDeclaredEmail")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "update_email"),
		attribute.String("service", "citizen"),
	)

	logger.Info("UpdateSelfDeclaredEmail called", zap.String("cpf", cpf))

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "email")
	var input models.SelfDeclaredEmailInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "SelfDeclaredEmailInput",
		})
		inputSpan.End()
		logger.Error("failed to parse input", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.valor", input.Valor)
	inputSpan.End()

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Get current data for comparison with tracing
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.CitizenCollection, "cpf")
	current, err := getMergedCitizenData(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.CitizenCollection,
			"db.filter":     "cpf",
		})
		findSpan.End()
		logger.Error("failed to fetch current data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	findSpan.End()

	// Compare data with tracing
	ctx, compareSpan := utils.TraceDataComparison(ctx, "email_comparison")
	if current.Email != nil && current.Email.Principal != nil &&
		current.Email.Principal.Valor != nil && *current.Email.Principal.Valor == input.Valor {
		compareSpan.End()
		c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: email matches current data"})
		return
	}
	compareSpan.End()

	// Build email object with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_email_object")
	origem := "self-declared"
	sistema := "rmi"
	now := time.Now()
	email := models.Email{
		Indicador: utils.BoolPtr(true),
		Principal: &models.EmailPrincipal{
			Valor:     &input.Valor,
			Origem:    &origem,
			Sistema:   &sistema,
			UpdatedAt: &now,
		},
	}
	buildSpan.End()

	// Update database with tracing
	ctx, updateSpan := utils.TraceDatabaseUpsert(ctx, config.AppConfig.SelfDeclaredCollection, "cpf")
	update := bson.M{
		"$set": bson.M{
			"email":      email,
			"updated_at": time.Now(),
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
		utils.GetUpdateOptionsWithWriteConcern("user_data", true),
	)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.SelfDeclaredCollection,
			"db.operation":  "upsert",
		})
		updateSpan.End()
		logger.Error("failed to update self-declared email", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update email: " + err.Error()})
		return
	}
	updateSpan.End()

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()

	// Invalidate cache with tracing
	ctx, cacheSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("citizen:%s", cpf))
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		utils.RecordErrorInSpan(cacheSpan, err, map[string]interface{}{
			"cache.key": cacheKey,
		})
		logger.Warn("failed to invalidate cache", zap.Error(err))
	}
	cacheSpan.End()

	// Log audit event with tracing
	ctx, auditSpan := utils.TraceAuditLogging(ctx, "update", "email")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}

	oldValue := "none"
	if current.Email != nil && current.Email.Principal != nil {
		oldValue = *current.Email.Principal.Valor
	}

	newValue := input.Valor

	err = utils.LogEmailUpdate(ctx, auditCtx, oldValue, newValue)
	if err != nil {
		utils.RecordErrorInSpan(auditSpan, err, map[string]interface{}{
			"audit.action":   "update",
			"audit.resource": "email",
		})
		logger.Warn("failed to log audit event", zap.Error(err))
	}
	auditSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Self-declared email updated successfully"})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("UpdateSelfDeclaredEmail completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateSelfDeclaredRaca")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "update_ethnicity"),
		attribute.String("service", "citizen"),
	)

	logger.Info("UpdateSelfDeclaredRaca called", zap.String("cpf", cpf))

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "ethnicity")
	var input models.SelfDeclaredRacaInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "SelfDeclaredRacaInput",
		})
		inputSpan.End()
		logger.Error("failed to parse input", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid input format"})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.valor", input.Valor)
	inputSpan.End()

	// Validate ethnicity with tracing
	ctx, validationSpan := utils.TraceInputValidation(ctx, "ethnicity_value", "ethnicity")
	if !models.IsValidEthnicity(input.Valor) {
		utils.RecordErrorInSpan(validationSpan, fmt.Errorf("invalid ethnicity value: %s", input.Valor), map[string]interface{}{
			"invalid_value": input.Valor,
		})
		validationSpan.End()
		logger.Error("invalid ethnicity value", zap.String("value", input.Valor))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid ethnicity value"})
		return
	}
	utils.AddSpanAttribute(validationSpan, "validated_value", input.Valor)
	validationSpan.End()

	// Get existing self-declared data with tracing
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.SelfDeclaredCollection, "cpf")
	var selfDeclared models.SelfDeclaredData
	err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&selfDeclared)
	if err != nil && err != mongo.ErrNoDocuments {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.SelfDeclaredCollection,
			"db.filter":     "cpf",
		})
		findSpan.End()
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Error("failed to get self-declared data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	// Log old ethnicity for audit
	oldEthnicity := ""
	if selfDeclared.Raca != nil {
		oldEthnicity = *selfDeclared.Raca
	}
	utils.AddSpanAttribute(findSpan, "old_ethnicity", oldEthnicity)
	utils.AddSpanAttribute(findSpan, "document_exists", err != mongo.ErrNoDocuments)
	findSpan.End()

	// Update ethnicity with tracing
	ctx, updateSpan := utils.TraceDatabaseUpsert(ctx, config.AppConfig.SelfDeclaredCollection, "cpf")
	selfDeclared.CPF = cpf
	selfDeclared.Raca = &input.Valor

	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		bson.M{"$set": bson.M{
			"cpf":        cpf,
			"raca":       input.Valor,
			"updated_at": time.Now(),
		}},
		utils.GetUpdateOptionsWithWriteConcern("user_data", true),
	)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.SelfDeclaredCollection,
			"db.operation":  "upsert",
		})
		updateSpan.End()
		observability.DatabaseOperations.WithLabelValues("update", "error").Inc()
		logger.Error("failed to update self-declared data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	utils.AddSpanAttribute(updateSpan, "new_ethnicity", input.Valor)
	updateSpan.End()

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()
	observability.SelfDeclaredUpdates.WithLabelValues("success").Inc()

	// Invalidate cache with tracing
	ctx, cacheSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("citizen:%s", cpf))
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	cacheStart := time.Now()
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		utils.RecordErrorInSpan(cacheSpan, err, map[string]interface{}{
			"cache.key": cacheKey,
		})
		logger.Warn("failed to invalidate cache", zap.Error(err))
	}
	cacheDuration := time.Since(cacheStart)
	utils.AddSpanAttribute(cacheSpan, "cache.duration_ms", cacheDuration.Milliseconds())
	cacheSpan.End()

	// Log audit event with tracing
	ctx, auditSpan := utils.TraceAuditLogging(ctx, "update", "ethnicity")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}

	err = utils.LogEthnicityUpdate(ctx, auditCtx, oldEthnicity, input.Valor)
	if err != nil {
		utils.RecordErrorInSpan(auditSpan, err, map[string]interface{}{
			"audit.action":   "update",
			"audit.resource": "ethnicity",
		})
		logger.Warn("failed to log audit event", zap.Error(err))
	}
	auditSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Self-declared ethnicity updated successfully"})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("UpdateSelfDeclaredRaca completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.Duration("cache_duration", cacheDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "HealthCheck")
	defer span.End()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "health_check"),
		attribute.String("service", "health"),
	)

	logger := observability.Logger()
	logger.Info("HealthCheck called")

	// Try to get from cache first with tracing
	ctx, cacheSpan := utils.TraceCacheGet(ctx, "health:status")
	cacheKey := "health:status"
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		utils.AddSpanAttribute(cacheSpan, "cache.hit", true)
		observability.CacheHits.WithLabelValues("health_check").Inc()
		var health HealthResponse
		if err := json.Unmarshal([]byte(cachedData), &health); err == nil {
			utils.AddSpanAttribute(cacheSpan, "cache.unmarshal_success", true)
			cacheSpan.End()

			// Serialize response with tracing
			_, responseSpan := utils.TraceResponseSerialization(ctx, "cached")
			if health.Status == "healthy" {
				c.JSON(http.StatusOK, health)
			} else {
				c.JSON(http.StatusServiceUnavailable, health)
			}
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Info("HealthCheck completed (cache hit)",
				zap.String("status", health.Status),
				zap.Duration("total_duration", totalDuration),
				zap.String("result", "success"))
			return
		}
		utils.AddSpanAttribute(cacheSpan, "cache.unmarshal_error", err.Error())
		observability.Logger().Warn("failed to unmarshal cached health data", zap.Error(err))
	}
	utils.AddSpanAttribute(cacheSpan, "cache.hit", false)
	cacheSpan.End()

	// Build health response with tracing
	_, buildSpan := utils.TraceBusinessLogic(ctx, "build_health_response")
	health := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now(),
		Services:  make(map[string]string),
	}
	buildSpan.End()

	// Check MongoDB with tracing
	_, mongoSpan := utils.TraceExternalService(ctx, "mongodb", "ping")
	if err := config.MongoDB.Client().Ping(ctx, nil); err != nil {
		utils.RecordErrorInSpan(mongoSpan, err, map[string]interface{}{
			"service.name":      "mongodb",
			"service.operation": "ping",
		})
		health.Status = "unhealthy"
		health.Services["mongodb"] = "unhealthy"
	} else {
		utils.AddSpanAttribute(mongoSpan, "service.status", "healthy")
		health.Services["mongodb"] = "healthy"
	}
	mongoSpan.End()

	// Check Redis with tracing
	_, redisSpan := utils.TraceExternalService(ctx, "redis", "ping")
	if err := config.Redis.Ping(ctx).Err(); err != nil {
		utils.RecordErrorInSpan(redisSpan, err, map[string]interface{}{
			"service.name":      "redis",
			"service.operation": "ping",
		})
		health.Status = "unhealthy"
		health.Services["redis"] = "unhealthy"
	} else {
		utils.AddSpanAttribute(redisSpan, "service.status", "healthy")
		health.Services["redis"] = "healthy"
	}
	redisSpan.End()

	// Cache the result with different TTLs based on health status with tracing
	_, cacheSetSpan := utils.TraceCacheSet(ctx, cacheKey, 5*time.Second)
	healthJSON, err := json.Marshal(health)
	if err == nil {
		ttl := 5 * time.Second // Default TTL for healthy responses
		if health.Status == "unhealthy" {
			ttl = 1 * time.Second // Shorter TTL for unhealthy responses
		}
		if err := config.Redis.Set(ctx, cacheKey, healthJSON, ttl).Err(); err != nil {
			utils.RecordErrorInSpan(cacheSetSpan, err, map[string]interface{}{
				"cache.operation": "set",
			})
			observability.Logger().Warn("failed to cache health status", zap.Error(err))
		} else {
			utils.AddSpanAttribute(cacheSetSpan, "cache.set_success", true)
			utils.AddSpanAttribute(cacheSetSpan, "cache.ttl", ttl.String())
		}
	} else {
		utils.RecordErrorInSpan(cacheSetSpan, err, map[string]interface{}{
			"cache.operation": "marshal",
		})
	}
	cacheSetSpan.End()

	// Add health status to span attributes
	span.SetAttributes(
		attribute.String("health.status", health.Status),
		attribute.String("health.mongodb", health.Services["mongodb"]),
		attribute.String("health.redis", health.Services["redis"]),
	)

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "live")
	if health.Status == "healthy" {
		c.JSON(http.StatusOK, health)
	} else {
		c.JSON(http.StatusServiceUnavailable, health)
	}
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("HealthCheck completed",
		zap.String("status", health.Status),
		zap.String("mongodb", health.Services["mongodb"]),
		zap.String("redis", health.Services["redis"]),
		zap.Duration("total_duration", totalDuration),
		zap.String("result", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetFirstLogin")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_first_login"),
		attribute.String("service", "citizen"),
	)

	logger.Info("GetFirstLogin called", zap.String("cpf", cpf))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Get user config with tracing
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.UserConfigCollection, "cpf")
	var userConfig models.UserConfig
	err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).FindOne(
		ctx,
		bson.M{"cpf": cpf},
	).Decode(&userConfig)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// If no config exists, it's first login
			utils.AddSpanAttribute(dbSpan, "user_config.found", false)
			utils.AddSpanAttribute(dbSpan, "user_config.first_login", true)
			dbSpan.End()

			// Serialize response with tracing
			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, models.UserConfigResponse{FirstLogin: true})
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Info("GetFirstLogin completed (first login)",
				zap.String("cpf", cpf),
				zap.Bool("first_login", true),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.UserConfigCollection,
			"db.filter":     "cpf",
		})
		dbSpan.End()
		logger.Error("failed to get user config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get user config"})
		return
	}
	utils.AddSpanAttribute(dbSpan, "user_config.found", true)
	utils.AddSpanAttribute(dbSpan, "user_config.first_login", userConfig.FirstLogin)
	dbSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, models.UserConfigResponse{FirstLogin: userConfig.FirstLogin})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("GetFirstLogin completed",
		zap.String("cpf", cpf),
		zap.Bool("first_login", userConfig.FirstLogin),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateFirstLogin")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "update_first_login"),
		attribute.String("service", "citizen"),
	)

	logger.Info("UpdateFirstLogin called", zap.String("cpf", cpf))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Build update object with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_update_object")
	update := bson.M{
		"$set": bson.M{
			"first_login": false,
			"updated_at":  time.Now(),
		},
	}
	buildSpan.End()

	// Update database with tracing
	ctx, updateSpan := utils.TraceDatabaseUpsert(ctx, config.AppConfig.UserConfigCollection, "cpf")
	_, err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
		utils.GetUpdateOptionsWithWriteConcern("user_data", true),
	)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.UserConfigCollection,
			"db.operation":  "upsert",
		})
		updateSpan.End()
		logger.Error("failed to update first login status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update first login status"})
		return
	}
	updateSpan.End()

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()

	// Log audit event with tracing
	ctx, auditSpan := utils.TraceAuditLogging(ctx, "update", "first_login")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}

	err = utils.LogUserConfigUpdate(ctx, auditCtx, "first_login", true, false)
	if err != nil {
		utils.RecordErrorInSpan(auditSpan, err, map[string]interface{}{
			"audit.action":   "update",
			"audit.resource": "first_login",
		})
		logger.Warn("failed to log audit event", zap.Error(err))
	}
	auditSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, models.UserConfigResponse{FirstLogin: false})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("UpdateFirstLogin completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetOptIn")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_opt_in"),
		attribute.String("service", "citizen"),
	)

	logger.Info("GetOptIn called", zap.String("cpf", cpf))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Get user config with tracing
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.UserConfigCollection, "cpf")
	var userConfig models.UserConfig
	err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&userConfig)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// If no config exists, default to opted in
			utils.AddSpanAttribute(dbSpan, "user_config.found", false)
			utils.AddSpanAttribute(dbSpan, "user_config.opt_in", true)
			utils.AddSpanAttribute(dbSpan, "user_config.default_value", true)
			dbSpan.End()

			// Serialize response with tracing
			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, models.UserConfigOptInResponse{OptIn: true})
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Info("GetOptIn completed (default value)",
				zap.String("cpf", cpf),
				zap.Bool("opt_in", true),
				zap.Bool("default_value", true),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.UserConfigCollection,
			"db.filter":     "cpf",
		})
		dbSpan.End()
		logger.Error("failed to get user config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get user config"})
		return
	}
	utils.AddSpanAttribute(dbSpan, "user_config.found", true)
	utils.AddSpanAttribute(dbSpan, "user_config.opt_in", userConfig.OptIn)
	dbSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, models.UserConfigOptInResponse{OptIn: userConfig.OptIn})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("GetOptIn completed",
		zap.String("cpf", cpf),
		zap.Bool("opt_in", userConfig.OptIn),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateOptIn")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "update_opt_in"),
		attribute.String("service", "citizen"),
	)

	logger.Info("UpdateOptIn called", zap.String("cpf", cpf))

	// Validate CPF with tracing
	ctx, cpfSpan := utils.TraceInputValidation(ctx, "cpf_format", "cpf")
	if !utils.ValidateCPF(cpf) {
		utils.RecordErrorInSpan(cpfSpan, fmt.Errorf("invalid CPF format"), map[string]interface{}{
			"cpf": cpf,
		})
		cpfSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}
	cpfSpan.End()

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "opt_in")
	var input models.UserConfigOptInResponse
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "UserConfigOptInResponse",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.opt_in", input.OptIn)
	inputSpan.End()

	// Build update object with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_update_object")
	update := bson.M{
		"$set": bson.M{
			"opt_in":     input.OptIn,
			"updated_at": time.Now(),
		},
	}
	buildSpan.End()

	// Update database with tracing
	ctx, updateSpan := utils.TraceDatabaseUpsert(ctx, config.AppConfig.UserConfigCollection, "cpf")
	_, err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).UpdateOne(
		ctx,
		bson.M{"cpf": cpf},
		update,
		utils.GetUpdateOptionsWithWriteConcern("user_data", true),
	)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.UserConfigCollection,
			"db.operation":  "upsert",
		})
		updateSpan.End()
		logger.Error("failed to update opt-in status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update opt-in status"})
		return
	}
	updateSpan.End()

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()

	// Invalidate cache with tracing
	ctx, cacheSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("citizen:%s", cpf))
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		utils.RecordErrorInSpan(cacheSpan, err, map[string]interface{}{
			"cache.key": cacheKey,
		})
		logger.Warn("failed to invalidate cache", zap.Error(err))
	}
	cacheSpan.End()

	// Log audit event with tracing
	ctx, auditSpan := utils.TraceAuditLogging(ctx, "update", "opt_in")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}

	err = utils.LogUserConfigUpdate(ctx, auditCtx, "opt_in", !input.OptIn, input.OptIn)
	if err != nil {
		utils.RecordErrorInSpan(auditSpan, err, map[string]interface{}{
			"audit.action":   "update",
			"audit.resource": "opt_in",
		})
		logger.Warn("failed to log audit event", zap.Error(err))
	}
	auditSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, input)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("UpdateOptIn completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.Bool("new_opt_in_status", input.OptIn),
		zap.String("status", "success"))
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
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetEthnicityOptions")
	defer span.End()

	logger := observability.Logger()

	// Add operation to span attributes
	span.SetAttributes(
		attribute.String("operation", "get_ethnicity_options"),
		attribute.String("service", "citizen"),
	)

	logger.Info("GetEthnicityOptions called")

	// Get ethnicity options with tracing
	ctx, optionsSpan := utils.TraceBusinessLogic(ctx, "get_valid_ethnicity_options")
	options := models.ValidEthnicityOptions()
	utils.AddSpanAttribute(optionsSpan, "options.count", len(options))
	optionsSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, options)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("GetEthnicityOptions completed",
		zap.Int("options_count", len(options)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetCitizenWallet godoc
// @Summary Obter dados da carteira do cidadão
// @Description Recupera os dados da carteira do cidadão por CPF, incluindo informações de saúde e outros dados da carteira.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Security BearerAuth
// @Success 200 {object} models.CitizenWallet "Dados da carteira do cidadão"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/wallet [get]
func GetCitizenWallet(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetCitizenWallet")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_citizen_wallet"),
		attribute.String("service", "citizen"),
	)

	logger.Info("GetCitizenWallet called", zap.String("cpf", cpf))

	// Try to get from cache first with tracing
	ctx, cacheSpan := utils.TraceCacheGet(ctx, fmt.Sprintf("citizen_wallet:%s", cpf))
	cacheKey := fmt.Sprintf("citizen_wallet:%s", cpf)
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		utils.AddSpanAttribute(cacheSpan, "cache.hit", true)
		observability.CacheHits.WithLabelValues("get_citizen_wallet").Inc()
		var wallet models.CitizenWallet
		if err := json.Unmarshal([]byte(cachedData), &wallet); err == nil {
			cacheSpan.End()

			// Serialize response with tracing
			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, wallet)
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Info("GetCitizenWallet completed (cache hit)",
				zap.String("cpf", cpf),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
		utils.AddSpanAttribute(cacheSpan, "cache.unmarshal_error", err.Error())
		logger.Warn("failed to unmarshal cached wallet data", zap.Error(err))
	}
	utils.AddSpanAttribute(cacheSpan, "cache.hit", false)
	cacheSpan.End()

	// Get base data with tracing
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.CitizenCollection, "cpf")
	var citizen models.Citizen
	err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.AddSpanAttribute(dbSpan, "citizen.found", false)
			utils.AddSpanAttribute(dbSpan, "citizen.reason", "not_found")
			dbSpan.End()
			observability.DatabaseOperations.WithLabelValues("find", "not_found").Inc()
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "citizen not found"})
			return
		}
		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.CitizenCollection,
			"db.filter":     "cpf",
		})
		dbSpan.End()
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Error("failed to get citizen data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	utils.AddSpanAttribute(dbSpan, "citizen.found", true)
	utils.AddSpanAttribute(dbSpan, "citizen.has_documentos", citizen.Documentos != nil)
	utils.AddSpanAttribute(dbSpan, "citizen.has_saude", citizen.Saude != nil)
	utils.AddSpanAttribute(dbSpan, "citizen.has_assistencia_social", citizen.AssistenciaSocial != nil)
	utils.AddSpanAttribute(dbSpan, "citizen.has_educacao", citizen.Educacao != nil)
	dbSpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Create wallet response with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_citizen_wallet")
	wallet := models.CitizenWallet{
		CPF:               cpf,
		Documentos:        citizen.Documentos,
		Saude:             citizen.Saude,
		AssistenciaSocial: citizen.AssistenciaSocial,
		Educacao:          citizen.Educacao,
	}
	buildSpan.End()

	// Cache the result with tracing
	ctx, cacheSetSpan := utils.TraceCacheSet(ctx, cacheKey, config.AppConfig.RedisTTL)
	if jsonData, err := json.Marshal(wallet); err == nil {
		config.Redis.Set(ctx, cacheKey, jsonData, config.AppConfig.RedisTTL)
		utils.AddSpanAttribute(cacheSetSpan, "cache.set_success", true)
	} else {
		utils.RecordErrorInSpan(cacheSetSpan, err, map[string]interface{}{
			"cache.operation": "set",
		})
	}
	cacheSetSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, wallet)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("GetCitizenWallet completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetMaintenanceRequests godoc
// @Summary Obter chamados do 1746 do cidadão
// @Description Recupera os chamados do 1746 de um cidadão por CPF com paginação. Cada documento representa um chamado individual.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param page query int false "Número da página (padrão: 1)" minimum(1)
// @Param per_page query int false "Itens por página (padrão: 10, máximo: 100)" minimum(1) maximum(100)
// @Security BearerAuth
// @Success 200 {object} models.PaginatedMaintenanceRequests "Lista paginada de chamados do 1746"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou parâmetros de paginação inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/maintenance-request [get]
func GetMaintenanceRequests(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetMaintenanceRequests")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_maintenance_requests"),
		attribute.String("service", "citizen"),
	)

	logger.Info("GetMaintenanceRequests called", zap.String("cpf", cpf))

	// Parse pagination parameters with tracing
	ctx, paginationSpan := utils.TraceInputParsing(ctx, "pagination_parameters")
	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		} else {
			utils.RecordErrorInSpan(paginationSpan, fmt.Errorf("invalid page parameter"), map[string]interface{}{
				"page_str": pageStr,
			})
			paginationSpan.End()
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid page parameter"})
			return
		}
	}

	perPage := 10
	if perPageStr := c.Query("per_page"); perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 && pp <= 100 {
			perPage = pp
		} else {
			utils.RecordErrorInSpan(paginationSpan, fmt.Errorf("invalid per_page parameter"), map[string]interface{}{
				"per_page_str": perPageStr,
			})
			paginationSpan.End()
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid per_page parameter (must be between 1 and 100)"})
			return
		}
	}

	// Calculate skip value
	skip := (page - 1) * perPage
	utils.AddSpanAttribute(paginationSpan, "page", page)
	utils.AddSpanAttribute(paginationSpan, "per_page", perPage)
	utils.AddSpanAttribute(paginationSpan, "skip", skip)
	paginationSpan.End()

	// Try to get from cache first (include pagination in cache key) with tracing
	ctx, cacheSpan := utils.TraceCacheGet(ctx, fmt.Sprintf("maintenance_requests:%s:page_%d_per_%d", cpf, page, perPage))
	cacheKey := fmt.Sprintf("maintenance_requests:%s:page_%d_per_%d", cpf, page, perPage)
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		utils.AddSpanAttribute(cacheSpan, "cache.hit", true)
		observability.CacheHits.WithLabelValues("get_maintenance_requests").Inc()
		var response models.PaginatedMaintenanceRequests
		if err := json.Unmarshal([]byte(cachedData), &response); err == nil {
			cacheSpan.End()

			// Serialize response with tracing
			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, response)
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Info("GetMaintenanceRequests completed (cache hit)",
				zap.String("cpf", cpf),
				zap.Int("page", page),
				zap.Int("per_page", perPage),
				zap.Int("total_count", response.Pagination.Total),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
		utils.AddSpanAttribute(cacheSpan, "cache.unmarshal_error", err.Error())
		logger.Warn("failed to unmarshal cached maintenance requests data", zap.Error(err))
	}
	utils.AddSpanAttribute(cacheSpan, "cache.hit", false)
	cacheSpan.End()

	// Get total count with tracing
	ctx, countSpan := utils.TraceDatabaseCount(ctx, config.AppConfig.MaintenanceRequestCollection, "cpf")
	total, err := config.MongoDB.Collection(config.AppConfig.MaintenanceRequestCollection).CountDocuments(ctx, bson.M{"cpf": cpf})
	if err != nil {
		utils.RecordErrorInSpan(countSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.MaintenanceRequestCollection,
			"db.filter":     "cpf",
		})
		countSpan.End()
		observability.DatabaseOperations.WithLabelValues("count", "error").Inc()
		logger.Error("failed to count maintenance requests", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	utils.AddSpanAttribute(countSpan, "total_count", total)
	countSpan.End()

	// Get maintenance request documents with pagination with tracing
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.MaintenanceRequestCollection, "cpf_paginated")
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPage)).
		SetSort(bson.D{{Key: "chamados_1746.data.data_inicio", Value: -1}}) // Sort by data_inicio descending (newest first)

	cursor, err := config.MongoDB.Collection(config.AppConfig.MaintenanceRequestCollection).Find(ctx, bson.M{"cpf": cpf}, opts)
	if err != nil {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.MaintenanceRequestCollection,
			"db.filter":     "cpf",
			"db.skip":       skip,
			"db.limit":      perPage,
		})
		findSpan.End()
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Error("failed to get maintenance requests", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	defer cursor.Close(ctx)

	var docs []models.MaintenanceRequestDocument
	if err = cursor.All(ctx, &docs); err != nil {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.operation": "cursor_all",
		})
		findSpan.End()
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Error("failed to decode maintenance request documents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	utils.AddSpanAttribute(findSpan, "documents_found", len(docs))
	utils.AddSpanAttribute(findSpan, "skip", skip)
	utils.AddSpanAttribute(findSpan, "limit", perPage)
	findSpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Convert documents to MaintenanceRequest format for backward compatibility with tracing
	_, convertSpan := utils.TraceBusinessLogic(ctx, "convert_documents_to_maintenance_requests")
	var requests []models.MaintenanceRequest
	for _, doc := range docs {
		request := doc.ConvertToMaintenanceRequest()
		requests = append(requests, *request)
	}
	utils.AddSpanAttribute(convertSpan, "converted_requests_count", len(requests))
	convertSpan.End()

	// Calculate total pages with tracing
	_, calcSpan := utils.TraceBusinessLogic(ctx, "calculate_pagination")
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	utils.AddSpanAttribute(calcSpan, "total_pages", totalPages)
	calcSpan.End()

	// Create response with tracing
	_, buildSpan := utils.TraceBusinessLogic(ctx, "build_paginated_response")
	response := models.PaginatedMaintenanceRequests{
		Data: requests,
	}
	response.Pagination.Page = page
	response.Pagination.PerPage = perPage
	response.Pagination.Total = int(total)
	response.Pagination.TotalPages = totalPages
	buildSpan.End()

	// Cache the result with tracing
	_, cacheSetSpan := utils.TraceCacheSet(ctx, cacheKey, config.AppConfig.RedisTTL)
	if jsonData, err := json.Marshal(response); err == nil {
		config.Redis.Set(ctx, cacheKey, jsonData, config.AppConfig.RedisTTL)
		utils.AddSpanAttribute(cacheSetSpan, "cache.set_success", true)
	} else {
		utils.RecordErrorInSpan(cacheSetSpan, err, map[string]interface{}{
			"cache.operation": "set",
		})
	}
	cacheSetSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, response)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("GetMaintenanceRequests completed",
		zap.String("cpf", cpf),
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int("total_count", int(total)),
		zap.Int("total_pages", totalPages),
		zap.Int("requests_returned", len(requests)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// ValidatePhoneVerification godoc
// @Summary Validar código de verificação de telefone
// @Description Valida o código de verificação enviado para o telefone e ativa o mapeamento
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Param data body models.ValidatePhoneVerificationRequest true "Código de verificação"
// @Security BearerAuth
// @Success 200 {object} models.ValidatePhoneVerificationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/phone/validate [post]

type ErrorResponse struct {
	Error string `json:"error"`
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
