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
	"github.com/prefeitura-rio/app-rmi/internal/services"
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
// @Description Recupera os dados do cidadão por CPF, incluindo informações básicas e dados autodeclarados.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Security BearerAuth
// @Success 200 {object} models.Citizen "Dados do cidadão"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
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

	logger.Debug("GetCitizenData called", zap.String("cpf", cpf))

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

	// Use getMergedCitizenData which implements cache-aware reading
	ctx, getDataSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.CitizenCollection, "cpf")
	citizen, err := getMergedCitizenData(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(getDataSpan, err, map[string]interface{}{
			"operation": "getMergedCitizenData",
			"cpf":       cpf,
		})
		getDataSpan.End()
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Citizen not found"})
			return
		}
		logger.Error("failed to get citizen data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}
	getDataSpan.End()

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Cache the merged result with tracing
	ctx, cacheSetSpan := utils.TraceCacheSet(ctx, fmt.Sprintf("citizen:%s", cpf), config.AppConfig.RedisTTL)
	if jsonData, err := json.Marshal(citizen); err == nil {
		config.Redis.Set(ctx, fmt.Sprintf("citizen:%s", cpf), jsonData, config.AppConfig.RedisTTL)
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
	logger.Debug("GetCitizenData completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// Helper: Get merged citizen data (as delivered by /citizen/{cpf})
func getMergedCitizenData(ctx context.Context, cpf string) (*models.Citizen, error) {
	// Create data manager for cache-aware reads
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	
	var citizen models.Citizen
	err := dataManager.Read(ctx, cpf, config.AppConfig.CitizenCollection, "citizen", &citizen)
	if err != nil && err != mongo.ErrNoDocuments {
		// Fallback to direct MongoDB query if DataManager fails
		err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
	}
	
	// Use batched Redis operations for self-declared data
	selfDeclared := getBatchedSelfDeclaredData(ctx, cpf)
	
	// Only fallback to MongoDB if we have no cached data
	if selfDeclared.Endereco == nil && selfDeclared.Email == nil && 
	   selfDeclared.Telefone == nil && selfDeclared.Raca == nil {
		// Fallback to MongoDB for complete self-declared data
		var mongoSelfDeclared models.SelfDeclaredData
		err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&mongoSelfDeclared)
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, err
		}
		selfDeclared = mongoSelfDeclared
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

// getBatchedSelfDeclaredData efficiently retrieves self-declared data using Redis batch operations
func getBatchedSelfDeclaredData(ctx context.Context, cpf string) models.SelfDeclaredData {
	var selfDeclared models.SelfDeclaredData
	
	// Use batched Redis operations to fetch all self-declared data
	keys := []string{
		fmt.Sprintf("self_declared_address:write:%s", cpf),
		fmt.Sprintf("self_declared_email:write:%s", cpf),
		fmt.Sprintf("self_declared_phone:write:%s", cpf),
		fmt.Sprintf("self_declared_raca:write:%s", cpf),
	}
	
	// Try write buffer first (most recent data)
	results, err := services.BatchReadMultiple(ctx, keys, observability.Logger().Unwrap())
	if err != nil {
		observability.Logger().Warn("batch read from write buffer failed", 
			zap.String("cpf", cpf), zap.Error(err))
	}
	
	// Parse results from write buffer
	parseResult := func(key, dataType string, target interface{}) bool {
		if data, exists := results[key]; exists && data != "" {
			if err := json.Unmarshal([]byte(data), target); err == nil {
				return true
			}
		}
		return false
	}
	
	var addressData struct {
		CPF       string              `json:"cpf"`
		Endereco  *models.Endereco    `json:"endereco"`
		UpdatedAt string              `json:"updated_at"`
	}
	if parseResult(keys[0], "address", &addressData) && addressData.Endereco != nil {
		selfDeclared.Endereco = addressData.Endereco
	}
	
	var emailData struct {
		CPF       string          `json:"cpf"`
		Email     *models.Email   `json:"email"`
		UpdatedAt string          `json:"updated_at"`
	}
	if parseResult(keys[1], "email", &emailData) && emailData.Email != nil {
		selfDeclared.Email = emailData.Email
	}
	
	var phoneData struct {
		CPF       string             `json:"cpf"`
		Telefone  *models.Telefone   `json:"telefone"`
		UpdatedAt string             `json:"updated_at"`
	}
	if parseResult(keys[2], "phone", &phoneData) && phoneData.Telefone != nil {
		selfDeclared.Telefone = phoneData.Telefone
	}
	
	var racaData struct {
		CPF       string      `json:"cpf"`
		Raca      *string     `json:"raca"`
		UpdatedAt string      `json:"updated_at"`
	}
	if parseResult(keys[3], "raca", &racaData) && racaData.Raca != nil {
		selfDeclared.Raca = racaData.Raca
	}
	
	// If write buffer didn't have everything, try read cache in batch
	if selfDeclared.Endereco == nil || selfDeclared.Email == nil || 
	   selfDeclared.Telefone == nil || selfDeclared.Raca == nil {
		   
		cacheKeys := []string{
			fmt.Sprintf("self_declared_address:cache:%s", cpf),
			fmt.Sprintf("self_declared_email:cache:%s", cpf),
			fmt.Sprintf("self_declared_phone:cache:%s", cpf),
			fmt.Sprintf("self_declared_raca:cache:%s", cpf),
		}
		
		cacheResults, err := services.BatchReadMultiple(ctx, cacheKeys, observability.Logger().Unwrap())
		if err != nil {
			observability.Logger().Warn("batch read from cache failed", 
				zap.String("cpf", cpf), zap.Error(err))
		}
		
		// Parse missing data from cache
		parseCacheResult := func(key, dataType string, target interface{}) bool {
			if data, exists := cacheResults[key]; exists && data != "" {
				if err := json.Unmarshal([]byte(data), target); err == nil {
					return true
				}
			}
			return false
		}
		
		if selfDeclared.Endereco == nil && parseCacheResult(cacheKeys[0], "address", &addressData) && addressData.Endereco != nil {
			selfDeclared.Endereco = addressData.Endereco
		}
		if selfDeclared.Email == nil && parseCacheResult(cacheKeys[1], "email", &emailData) && emailData.Email != nil {
			selfDeclared.Email = emailData.Email
		}
		if selfDeclared.Telefone == nil && parseCacheResult(cacheKeys[2], "phone", &phoneData) && phoneData.Telefone != nil {
			selfDeclared.Telefone = phoneData.Telefone
		}
		if selfDeclared.Raca == nil && parseCacheResult(cacheKeys[3], "raca", &racaData) && racaData.Raca != nil {
			selfDeclared.Raca = racaData.Raca
		}
	}
	
	return selfDeclared
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
// @Failure 409 {object} ErrorResponse "Endereço não alterado"
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
		attribute.String("operation", "update_self_declared_address"),
		attribute.String("service", "citizen"),
	)

	logger.Debug("UpdateSelfDeclaredAddress called", zap.String("cpf", cpf))

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
	ctx, parseSpan := utils.TraceInputParsing(ctx, "address_input")
	var input models.SelfDeclaredAddressInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RecordErrorInSpan(parseSpan, err, map[string]interface{}{
			"input": input,
		})
		parseSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid input: " + err.Error()})
		return
	}
	parseSpan.End()

	// Validate input with tracing
	ctx, validateSpan := utils.TraceInputValidation(ctx, "address_validation", "address")
	// Note: SelfDeclaredAddressInput doesn't have a Validate method
	// We'll rely on the binding validation and business logic validation
	validateSpan.End()

	// Get current address data for comparison with tracing (optimized to fetch only address field)
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.SelfDeclaredCollection, "cpf")
	current, err := getCurrentAddressData(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.SelfDeclaredCollection,
			"db.filter":     "cpf",
		})
		findSpan.End()
		logger.Error("failed to fetch current address data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	findSpan.End()

	// Compare data with tracing
	ctx, compareSpan := utils.TraceDataComparison(ctx, "address_comparison")
	if current != nil && current.Principal != nil &&
		*current.Principal.Bairro == input.Bairro &&
		*current.Principal.CEP == input.CEP &&
		(current.Principal.Complemento == nil && input.Complemento == nil ||
			(current.Principal.Complemento != nil && input.Complemento != nil && *current.Principal.Complemento == *input.Complemento)) &&
		*current.Principal.Estado == input.Estado &&
		*current.Principal.Logradouro == input.Logradouro &&
		*current.Principal.Municipio == input.Municipio &&
		*current.Principal.Numero == input.Numero &&
		(current.Principal.TipoLogradouro == nil && input.TipoLogradouro == nil ||
			(current.Principal.TipoLogradouro != nil && input.TipoLogradouro != nil && *current.Principal.TipoLogradouro == *input.TipoLogradouro)) {
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

	// Use cache service for update with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "update_citizen_via_cache")

	// Create unified cache service
	cacheService := services.NewCacheService()

	// Update via cache service (this will queue for MongoDB sync)
	err = cacheService.UpdateSelfDeclaredAddress(ctx, cpf, &endereco)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"cache.operation": "update_self_declared_address",
			"cache.service":   "unified_cache_service",
		})
		updateSpan.End()
		logger.Error("failed to update self-declared address via cache service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update address: " + err.Error()})
		return
	}
	updateSpan.End()

	// Record metrics
	observability.SelfDeclaredUpdates.WithLabelValues("success").Inc()

	// Invalidate old cache with tracing
	ctx, cacheSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("citizen:%s", cpf))
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	cacheStart := time.Now()
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		utils.AddSpanAttribute(cacheSpan, "cache.invalidation_error", err.Error())
		logger.Warn("failed to invalidate old cache", zap.Error(err))
	} else {
		utils.AddSpanAttribute(cacheSpan, "cache.invalidation_success", true)
	}
	cacheDuration := time.Since(cacheStart)
	utils.AddSpanAttribute(cacheSpan, "cache.duration_ms", cacheDuration.Milliseconds())
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

	err = utils.LogAddressUpdate(ctx, auditCtx, current, &endereco)
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
	logger.Debug("UpdateSelfDeclaredAddress completed",
		zap.String("cpf", cpf),
		zap.Duration("total_duration", totalDuration),
		zap.Duration("cache_duration", cacheDuration),
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

	logger.Debug("UpdateSelfDeclaredPhone called", zap.String("cpf", cpf))

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

	// Get current phone data for comparison with tracing (optimized to fetch only phone field)
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.SelfDeclaredCollection, "cpf")
	current, err := getCurrentPhoneData(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.SelfDeclaredCollection,
			"db.filter":     "cpf",
		})
		findSpan.End()
		logger.Error("failed to fetch current phone data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	findSpan.End()

	// Compare data with tracing
	ctx, compareSpan := utils.TraceDataComparison(ctx, "phone_comparison")
	if current != nil && current.Principal != nil &&
		current.Principal.DDI != nil && *current.Principal.DDI == input.DDI &&
		current.Principal.DDD != nil && *current.Principal.DDD == input.DDD &&
		current.Principal.Valor != nil && *current.Principal.Valor == input.Valor {
		compareSpan.End()
		c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: phone matches current data"})
		return
	}
	compareSpan.End()

	// Build full phone number with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_phone_number")
	fullPhone := input.DDI + input.DDD + input.Valor
	buildSpan.End()

	// Build phone object with tracing
	ctx, buildPhoneSpan := utils.TraceBusinessLogic(ctx, "build_phone_object")
	origem := "self-declared"
	sistema := "rmi"
	now := time.Now()
	telefone := models.Telefone{
		Indicador: utils.BoolPtr(false), // Set to false initially (pending verification)
		Principal: &models.TelefonePrincipal{
			DDI:       &input.DDI,
			DDD:       &input.DDD,
			Valor:     &input.Valor,
			Origem:    &origem,
			Sistema:   &sistema,
			UpdatedAt: &now,
		},
	}
	buildPhoneSpan.End()

	// Use cache service for update with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "update_phone_via_cache")
	cacheService := services.NewCacheService()
	err = cacheService.UpdateSelfDeclaredPhone(ctx, cpf, &telefone)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"cache.operation": "update_self_declared_phone",
			"cache.service":   "unified_cache_service",
		})
		updateSpan.End()
		logger.Error("failed to update self-declared phone via cache service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update phone: " + err.Error()})
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

	// Generate verification code for phone verification process with tracing
	ctx, codeSpan := utils.TraceBusinessLogic(ctx, "generate_verification_code")
	code := utils.GenerateVerificationCode()
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
	if current != nil && current.Principal != nil {
		oldValue = fmt.Sprintf("%s%s%s",
			*current.Principal.DDI,
			*current.Principal.DDD,
			*current.Principal.Valor)
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
	logger.Debug("UpdateSelfDeclaredPhone completed",
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

	logger.Debug("UpdateSelfDeclaredEmail called", zap.String("cpf", cpf))

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

	// Get current email data for comparison with tracing (optimized to fetch only email field)
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.SelfDeclaredCollection, "cpf")
	current, err := getCurrentEmailData(ctx, cpf)
	if err != nil {
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.SelfDeclaredCollection,
			"db.filter":     "cpf",
		})
		findSpan.End()
		logger.Error("failed to fetch current email data for comparison", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to check current data: " + err.Error()})
		return
	}
	findSpan.End()

	// Compare data with tracing
	ctx, compareSpan := utils.TraceDataComparison(ctx, "email_comparison")
	if current != nil && current.Principal != nil &&
		current.Principal.Valor != nil && *current.Principal.Valor == input.Valor {
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

	// Use cache service for update with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "update_email_via_cache")
	cacheService := services.NewCacheService()
	err = cacheService.UpdateSelfDeclaredEmail(ctx, cpf, &email)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"cache.operation": "update_self_declared_email",
			"cache.service":   "unified_cache_service",
		})
		updateSpan.End()
		logger.Error("failed to update self-declared email via cache service", zap.Error(err))
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
	if current != nil && current.Principal != nil {
		oldValue = *current.Principal.Valor
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
	logger.Debug("UpdateSelfDeclaredEmail completed",
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

	logger.Debug("UpdateSelfDeclaredRaca called", zap.String("cpf", cpf))

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

	// Use cache service for update with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "update_ethnicity_via_cache")
	cacheService := services.NewCacheService()
	err = cacheService.UpdateSelfDeclaredRaca(ctx, cpf, input.Valor)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"cache.operation": "update_self_declared_raca",
			"cache.service":   "unified_cache_service",
		})
		updateSpan.End()
		observability.DatabaseOperations.WithLabelValues("update", "error").Inc()
		logger.Error("failed to update self-declared ethnicity via cache service", zap.Error(err))
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
		utils.AddSpanAttribute(cacheSpan, "cache.invalidation_error", err.Error())
		logger.Warn("failed to invalidate old cache", zap.Error(err))
	} else {
		utils.AddSpanAttribute(cacheSpan, "cache.invalidation_success", true)
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
	logger.Debug("UpdateSelfDeclaredRaca completed",
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
// @Failure 500 {object} ErrorResponse "Um ou mais serviços não estão saudáveis"
// @Router /health [get]
func HealthCheck(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "HealthCheck")
	defer span.End()

	logger := observability.Logger()
	logger.Info("Health check requested")

	// Check MongoDB connection
	mongoHealthy := false
	if config.MongoDB != nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err := config.MongoDB.Client().Ping(ctx, nil)
		mongoHealthy = err == nil

		if err != nil {
			logger.Warn("MongoDB health check failed", zap.Error(err))
		}
	}

	// Check Redis connection
	redisHealthy := false
	if config.Redis != nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		_, err := config.Redis.Ping(ctx).Result()
		redisHealthy = err == nil

		if err != nil {
			logger.Warn("Redis health check failed", zap.Error(err))
		}
	}

	// Determine overall health
	overallHealthy := mongoHealthy && redisHealthy
	statusCode := http.StatusOK
	if !overallHealthy {
		statusCode = http.StatusInternalServerError
	}

	// Add health status to span
	span.SetAttributes(
		attribute.Bool("health.mongodb", mongoHealthy),
		attribute.Bool("health.redis", redisHealthy),
		attribute.Bool("health.overall", overallHealthy),
	)

	response := HealthResponse{
		Status:    overallHealthy,
		Timestamp: time.Now(),
		Services: map[string]ServiceHealth{
			"mongodb": {
				Status: mongoHealthy,
				Message: func() string {
					if mongoHealthy {
						return "Connected"
					} else {
						return "Connection failed"
					}
				}(),
				Timestamp: time.Now(),
			},
			"redis": {
				Status: redisHealthy,
				Message: func() string {
					if redisHealthy {
						return "Connected"
					} else {
						return "Connection failed"
					}
				}(),
				Timestamp: time.Now(),
			},
		},
	}

	c.JSON(statusCode, response)

	// Log health check result
	if overallHealthy {
		logger.Debug("Health check completed successfully")
	} else {
		logger.Error("Health check failed",
			zap.Bool("mongodb_healthy", mongoHealthy),
			zap.Bool("redis_healthy", redisHealthy))
	}
}

// MetricsHandler exposes Prometheus metrics for monitoring
// @Summary Métricas Prometheus
// @Description Expõe métricas Prometheus para monitoramento do sistema
// @Tags metrics
// @Produce text/plain
// @Success 200 {string} string "Métricas Prometheus"
// @Router /metrics [get]
func MetricsHandler(c *gin.Context) {
	// Prometheus metrics are automatically exposed by promauto
	// This handler can be used for additional custom metrics if needed
	c.String(http.StatusOK, "# RMI API Metrics\n# Prometheus metrics are automatically exposed\n")
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

	logger.Debug("GetFirstLogin called", zap.String("cpf", cpf))

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

	// Use DataManager for cache-aware reading with tracing
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.UserConfigCollection, "cpf")
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	
	var userConfig models.UserConfig
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)

	if err != nil {
		if err.Error() == fmt.Sprintf("document not found: %s", cpf) {
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
			logger.Debug("GetFirstLogin completed (first login)",
				zap.String("cpf", cpf),
				zap.Bool("first_login", true),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"operation": "dataManager.Read",
			"cpf":       cpf,
			"type":      "user_config",
		})
		dbSpan.End()
		logger.Error("failed to get user config via DataManager", zap.Error(err))
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
	logger.Debug("GetFirstLogin completed",
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

	logger.Debug("UpdateFirstLogin called", zap.String("cpf", cpf))

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

	// Build user config object with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_user_config_object")
	userConfig := &models.UserConfig{
		CPF:        cpf,
		FirstLogin: false,
		UpdatedAt:  time.Now(),
	}
	buildSpan.End()

	// Update via cache service with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "update_user_config_via_cache")
	cacheService := services.NewCacheService()
	err := cacheService.UpdateUserConfig(ctx, cpf, userConfig)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"cache.operation": "update_user_config",
			"cache.service":   "unified_cache_service",
		})
		updateSpan.End()
		logger.Error("failed to update first login status via cache service", zap.Error(err))
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
	logger.Debug("UpdateFirstLogin completed",
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

	logger.Debug("GetOptIn called", zap.String("cpf", cpf))

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

	// Use DataManager for cache-aware reading with tracing
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.UserConfigCollection, "cpf")
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	
	var userConfig models.UserConfig
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)
	if err != nil {
		if err.Error() == fmt.Sprintf("document not found: %s", cpf) {
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
			logger.Debug("GetOptIn completed (default value)",
				zap.String("cpf", cpf),
				zap.Bool("opt_in", true),
				zap.Bool("default_value", true),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"operation": "dataManager.Read",
			"cpf":       cpf,
			"type":      "user_config",
		})
		dbSpan.End()
		logger.Error("failed to get user config via DataManager", zap.Error(err))
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
	logger.Debug("GetOptIn completed",
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

	logger.Debug("UpdateOptIn called", zap.String("cpf", cpf))

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

	// Build user config object with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_user_config_object")
	userConfig := &models.UserConfig{
		CPF:       cpf,
		OptIn:     input.OptIn,
		UpdatedAt: time.Now(),
	}
	buildSpan.End()

	// Update via cache service with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "update_user_config_via_cache")
	cacheService := services.NewCacheService()
	err := cacheService.UpdateUserConfig(ctx, cpf, userConfig)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"cache.operation": "update_user_config",
			"cache.service":   "unified_cache_service",
		})
		updateSpan.End()
		logger.Error("failed to update opt-in status via cache service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update opt-in status"})
		return
	}
	updateSpan.End()

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()

	// Invalidate cache with tracing
	ctx, cacheSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("user_config:%s", cpf))
	cacheKey := fmt.Sprintf("user_config:%s", cpf)
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
	logger.Debug("UpdateOptIn completed",
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

	logger.Debug("GetEthnicityOptions called")

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
	logger.Debug("GetEthnicityOptions completed",
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

	logger.Debug("GetCitizenWallet called", zap.String("cpf", cpf))

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

	// Use DataManager for cache-aware reading with tracing
	ctx, dataSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.CitizenCollection, "cpf")
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	
	var citizen models.Citizen
	err := dataManager.Read(ctx, cpf, config.AppConfig.CitizenCollection, "citizen", &citizen)
	if err != nil {
		utils.RecordErrorInSpan(dataSpan, err, map[string]interface{}{
			"operation": "dataManager.Read",
			"cpf":       cpf,
			"type":      "citizen",
		})
		dataSpan.End()
		if err.Error() == fmt.Sprintf("document not found: %s", cpf) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "citizen not found"})
			return
		}
		logger.Error("failed to get citizen data via DataManager", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	utils.AddSpanAttribute(dataSpan, "citizen.found", true)
	utils.AddSpanAttribute(dataSpan, "citizen.has_documentos", citizen.Documentos != nil)
	utils.AddSpanAttribute(dataSpan, "citizen.has_saude", citizen.Saude != nil)
	utils.AddSpanAttribute(dataSpan, "citizen.has_assistencia_social", citizen.AssistenciaSocial != nil)
	utils.AddSpanAttribute(dataSpan, "citizen.has_educacao", citizen.Educacao != nil)
	dataSpan.End()

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

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, wallet)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetCitizenWallet completed",
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

	logger.Debug("GetMaintenanceRequests called", zap.String("cpf", cpf))

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
			logger.Debug("GetMaintenanceRequests completed (cache hit)",
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
	logger.Debug("GetMaintenanceRequests completed",
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

type ServiceHealth struct {
	Status    bool      `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type HealthResponse struct {
	Status    bool                     `json:"status"`
	Timestamp time.Time                `json:"timestamp"`
	Services  map[string]ServiceHealth `json:"services"`
}

type UserConfigResponse struct {
	FirstLogin bool `json:"first_login"`
}

type UserConfigOptInResponse struct {
	OptIn bool `json:"opt_in"`
}

// getCurrentAddressData gets only the address field from self_declared data for comparison
func getCurrentAddressData(ctx context.Context, cpf string) (*models.Endereco, error) {
	// Try to get from cache first using DataManager
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	
	var addressData struct {
		CPF       string              `json:"cpf"`
		Endereco  *models.Endereco    `json:"endereco"`
		UpdatedAt string              `json:"updated_at"`
	}
	
	err := dataManager.Read(ctx, cpf, config.AppConfig.SelfDeclaredCollection, "self_declared_address", &addressData)
	if err == nil && addressData.Endereco != nil {
		return addressData.Endereco, nil
	}
	
	// Fallback to MongoDB with field projection (only get endereco field)
	var selfDeclared struct {
		Endereco *models.Endereco `bson:"endereco"`
	}
	err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(
		ctx, 
		bson.M{"cpf": cpf},
		options.FindOne().SetProjection(bson.M{"endereco": 1}),
	).Decode(&selfDeclared)
	
	if err == mongo.ErrNoDocuments {
		return nil, nil // No data found, this is okay
	}
	if err != nil {
		return nil, err
	}
	
	return selfDeclared.Endereco, nil
}

// getCurrentPhoneData gets only the phone field from self_declared data for comparison
func getCurrentPhoneData(ctx context.Context, cpf string) (*models.Telefone, error) {
	// Try to get from cache first using DataManager
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	
	var phoneData struct {
		CPF       string             `json:"cpf"`
		Telefone  *models.Telefone   `json:"telefone"`
		UpdatedAt string             `json:"updated_at"`
	}
	
	err := dataManager.Read(ctx, cpf, config.AppConfig.SelfDeclaredCollection, "self_declared_phone", &phoneData)
	if err == nil && phoneData.Telefone != nil {
		return phoneData.Telefone, nil
	}
	
	// Fallback to MongoDB with field projection (only get telefone field)
	var selfDeclared struct {
		Telefone *models.Telefone `bson:"telefone"`
	}
	err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(
		ctx, 
		bson.M{"cpf": cpf},
		options.FindOne().SetProjection(bson.M{"telefone": 1}),
	).Decode(&selfDeclared)
	
	if err == mongo.ErrNoDocuments {
		return nil, nil // No data found, this is okay
	}
	if err != nil {
		return nil, err
	}
	
	return selfDeclared.Telefone, nil
}

// getCurrentEmailData gets only the email field from self_declared data for comparison
func getCurrentEmailData(ctx context.Context, cpf string) (*models.Email, error) {
	// Try to get from cache first using DataManager
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())
	
	var emailData struct {
		CPF       string          `json:"cpf"`
		Email     *models.Email   `json:"email"`
		UpdatedAt string          `json:"updated_at"`
	}
	
	err := dataManager.Read(ctx, cpf, config.AppConfig.SelfDeclaredCollection, "self_declared_email", &emailData)
	if err == nil && emailData.Email != nil {
		return emailData.Email, nil
	}
	
	// Fallback to MongoDB with field projection (only get email field)
	var selfDeclared struct {
		Email *models.Email `bson:"email"`
	}
	err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(
		ctx, 
		bson.M{"cpf": cpf},
		options.FindOne().SetProjection(bson.M{"email": 1}),
	).Decode(&selfDeclared)
	
	if err == mongo.ErrNoDocuments {
		return nil, nil // No data found, this is okay
	}
	if err != nil {
		return nil, err
	}
	
	return selfDeclared.Email, nil
}
