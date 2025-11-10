package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
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

// queueCFLookupJob queues a background job to perform CF lookup for a citizen
func queueCFLookupJob(ctx context.Context, cpf, address string) {
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Create CF lookup job
	job := services.SyncJob{
		ID:         fmt.Sprintf("cf_lookup_%s_%d", cpf, time.Now().UnixNano()),
		Type:       "cf_lookup",
		Key:        cpf,
		Collection: "cf_lookup",
		Data: map[string]interface{}{
			"cpf":     cpf,
			"address": address,
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	jobBytes, err := json.Marshal(job)
	if err != nil {
		logger.Error("failed to marshal CF lookup job", zap.Error(err))
		return
	}

	// Queue job using Redis
	queueKey := "sync:queue:cf_lookup"
	err = config.Redis.LPush(ctx, queueKey, string(jobBytes)).Err()
	if err != nil {
		logger.Error("failed to queue CF lookup job", zap.Error(err))
		return
	}

	logger.Debug("CF lookup job queued successfully", zap.String("job_id", job.ID))
}

// GetCitizenData godoc
// @Summary Obter dados do cidadão
// @Description Recupera os dados do cidadão por CPF, incluindo informações básicas e dados autodeclarados.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Security BearerAuth
// @Success 200 {object} models.Citizen "Dados do cidadão obtidos com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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

	// Check for CF lookup and queue background job if needed (only if enabled)
	ctx, cfSpan := utils.TraceBusinessLogic(ctx, "cf_lookup_check")
	if services.CFLookupServiceInstance != nil {
		shouldLookup, address, err := services.CFLookupServiceInstance.ShouldLookupCF(ctx, cpf, citizen)
		if err != nil {
			logger.Warn("failed to check CF lookup status", zap.Error(err))
		} else if shouldLookup && address != "" {
			// Queue background CF lookup job
			logger.Debug("queuing CF lookup job", zap.String("cpf", cpf), zap.String("address", address))
			queueCFLookupJob(ctx, cpf, address)
		}
	} else {
		logger.Debug("CF lookup service disabled - skipping CF lookup check", zap.String("cpf", cpf))
	}
	cfSpan.End()

	// Convert to response model (excluding wallet fields) with tracing
	ctx, convertSpan := utils.TraceBusinessLogic(ctx, "convert_to_citizen_response")
	citizenResponse := citizen.ToCitizenResponse()
	convertSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, citizenResponse)
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

	// Use batched Redis operations for self-declared data with MongoDB fallback
	selfDeclared := getBatchedSelfDeclaredData(ctx, cpf)
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
	// Always set exhibition name field (even if nil) to ensure it appears in JSON response
	citizen.NomeExibicao = selfDeclared.NomeExibicao

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
		fmt.Sprintf("self_declared_nome_exibicao:write:%s", cpf),
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
		CPF       string           `json:"cpf"`
		Endereco  *models.Endereco `json:"endereco"`
		UpdatedAt string           `json:"updated_at"`
	}
	if parseResult(keys[0], "address", &addressData) && addressData.Endereco != nil {
		selfDeclared.Endereco = addressData.Endereco
	}

	var emailData struct {
		CPF       string        `json:"cpf"`
		Email     *models.Email `json:"email"`
		UpdatedAt string        `json:"updated_at"`
	}
	if parseResult(keys[1], "email", &emailData) && emailData.Email != nil {
		selfDeclared.Email = emailData.Email
	}

	var phoneData struct {
		CPF       string           `json:"cpf"`
		Telefone  *models.Telefone `json:"telefone"`
		UpdatedAt string           `json:"updated_at"`
	}
	if parseResult(keys[2], "phone", &phoneData) && phoneData.Telefone != nil {
		selfDeclared.Telefone = phoneData.Telefone
	}

	var racaData struct {
		CPF       string  `json:"cpf"`
		Raca      *string `json:"raca"`
		UpdatedAt string  `json:"updated_at"`
	}
	if parseResult(keys[3], "raca", &racaData) && racaData.Raca != nil {
		selfDeclared.Raca = racaData.Raca
	}

	var nomeExibicaoData struct {
		CPF          string  `json:"cpf"`
		NomeExibicao *string `json:"nome_exibicao"`
		UpdatedAt    string  `json:"updated_at"`
	}
	if parseResult(keys[4], "nome_exibicao", &nomeExibicaoData) && nomeExibicaoData.NomeExibicao != nil {
		selfDeclared.NomeExibicao = nomeExibicaoData.NomeExibicao
	}

	// If write buffer didn't have everything, try read cache in batch
	if selfDeclared.Endereco == nil || selfDeclared.Email == nil ||
		selfDeclared.Telefone == nil || selfDeclared.Raca == nil || selfDeclared.NomeExibicao == nil {

		cacheKeys := []string{
			fmt.Sprintf("self_declared_address:cache:%s", cpf),
			fmt.Sprintf("self_declared_email:cache:%s", cpf),
			fmt.Sprintf("self_declared_phone:cache:%s", cpf),
			fmt.Sprintf("self_declared_raca:cache:%s", cpf),
			fmt.Sprintf("self_declared_nome_exibicao:cache:%s", cpf),
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
		if selfDeclared.NomeExibicao == nil && parseCacheResult(cacheKeys[4], "nome_exibicao", &nomeExibicaoData) && nomeExibicaoData.NomeExibicao != nil {
			selfDeclared.NomeExibicao = nomeExibicaoData.NomeExibicao
		}
	}

	// Final fallback to MongoDB for any missing individual fields
	if selfDeclared.Endereco == nil || selfDeclared.Email == nil ||
		selfDeclared.Telefone == nil || selfDeclared.Raca == nil || selfDeclared.NomeExibicao == nil {

		observability.Logger().Debug("fallback to MongoDB for missing self-declared fields",
			zap.String("cpf", cpf),
			zap.Bool("missing_endereco", selfDeclared.Endereco == nil),
			zap.Bool("missing_email", selfDeclared.Email == nil),
			zap.Bool("missing_telefone", selfDeclared.Telefone == nil),
			zap.Bool("missing_raca", selfDeclared.Raca == nil),
			zap.Bool("missing_nome_exibicao", selfDeclared.NomeExibicao == nil))

		var mongoSelfDeclared models.SelfDeclaredData
		err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&mongoSelfDeclared)
		if err != nil && err != mongo.ErrNoDocuments {
			observability.Logger().Warn("failed to fetch self-declared data from MongoDB",
				zap.String("cpf", cpf), zap.Error(err))
		} else if err == nil {
			// Fill in missing fields from MongoDB
			if selfDeclared.Endereco == nil && mongoSelfDeclared.Endereco != nil {
				selfDeclared.Endereco = mongoSelfDeclared.Endereco
			}
			if selfDeclared.Email == nil && mongoSelfDeclared.Email != nil {
				selfDeclared.Email = mongoSelfDeclared.Email
			}
			if selfDeclared.Telefone == nil && mongoSelfDeclared.Telefone != nil {
				selfDeclared.Telefone = mongoSelfDeclared.Telefone
			}
			if selfDeclared.Raca == nil && mongoSelfDeclared.Raca != nil {
				selfDeclared.Raca = mongoSelfDeclared.Raca
			}
			if selfDeclared.NomeExibicao == nil && mongoSelfDeclared.NomeExibicao != nil {
				selfDeclared.NomeExibicao = mongoSelfDeclared.NomeExibicao
			}

			observability.Logger().Debug("filled missing self-declared fields from MongoDB",
				zap.String("cpf", cpf))
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
// @Success 200 {object} SuccessResponse "Endereço autodeclarado atualizado com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou dados de endereço incorretos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 409 {object} ErrorResponse "Conflito - endereço não alterado (dados idênticos aos atuais)"
// @Failure 422 {object} ErrorResponse "Dados não processáveis - informações de endereço inválidas"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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

	// Handle CF data invalidation when address changes (only if enabled)
	ctx, cfInvalidateSpan := utils.TraceBusinessLogic(ctx, "cf_invalidate_on_address_change")
	if services.CFLookupServiceInstance != nil {
		newAddress := fmt.Sprintf("%s, %s, %s, %s, %s, %s",
			input.Logradouro, input.Numero,
			func() string {
				if input.Complemento != nil {
					return *input.Complemento
				} else {
					return ""
				}
			}(),
			input.Bairro, input.Municipio, input.Estado)
		newAddressHash := services.CFLookupServiceInstance.GenerateAddressHash(newAddress)

		err = services.CFLookupServiceInstance.InvalidateCFDataForAddress(ctx, cpf, newAddressHash)
		if err != nil {
			logger.Warn("failed to invalidate CF data for address change", zap.Error(err))
		} else {
			// Queue new CF lookup for the updated address
			logger.Debug("queuing CF lookup for updated address", zap.String("cpf", cpf))
			queueCFLookupJob(ctx, cpf, newAddress)
		}
	} else {
		logger.Debug("CF lookup service disabled - skipping CF data invalidation", zap.String("cpf", cpf))
	}
	cfInvalidateSpan.End()

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
// @Success 200 {object} SuccessResponse "Telefone autodeclarado submetido para validação com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou dados de telefone incorretos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 409 {object} ErrorResponse "Conflito - telefone não alterado (telefone corresponde aos dados atuais verificados)"
// @Failure 422 {object} ErrorResponse "Dados não processáveis - formato de telefone inválido"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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
	// Only return 409 if phone numbers match AND the current phone is verified (Indicador == true)
	// AND the data is not outdated (updated within threshold OR no updated_at timestamp)
	// This allows users to re-enter the same phone number if:
	// 1. They never verified it (Indicador != true)
	// 2. The data is outdated (updated_at > threshold ago)
	// 3. No updated_at timestamp exists (legacy data)
	if current != nil && current.Telefone != nil && current.Telefone.Principal != nil &&
		current.Telefone.Indicador != nil && *current.Telefone.Indicador &&
		current.Telefone.Principal.DDI != nil && *current.Telefone.Principal.DDI == input.DDI &&
		current.Telefone.Principal.DDD != nil && *current.Telefone.Principal.DDD == input.DDD &&
		current.Telefone.Principal.Valor != nil && *current.Telefone.Principal.Valor == input.Valor {

		// Check if data is outdated (allow re-declaration if outdated or no timestamp)
		isOutdated := current.UpdatedAt == nil || time.Since(*current.UpdatedAt) > config.AppConfig.SelfDeclaredOutdatedThreshold

		if !isOutdated {
			// Data is recent and matches - return conflict
			compareSpan.End()
			c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: phone matches current data"})
			return
		}

		// Data is outdated or has no timestamp - allow re-declaration
		logger.Debug("allowing phone re-declaration for outdated data",
			zap.String("cpf", cpf),
			zap.Bool("has_updated_at", current.UpdatedAt != nil),
			zap.Duration("threshold", config.AppConfig.SelfDeclaredOutdatedThreshold))
	}
	compareSpan.End()

	// Build full phone number with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_phone_number")
	fullPhone := input.DDI + input.DDD + input.Valor
	buildSpan.End()

	// Validate basic phone number format (length and digits only)
	ctx, validationSpan := utils.TraceInputValidation(ctx, "phone_format", "phone")
	if len(fullPhone) < 10 || len(fullPhone) > 15 {
		utils.RecordErrorInSpan(validationSpan, fmt.Errorf("invalid phone number length"), map[string]interface{}{
			"phone":  fullPhone,
			"length": len(fullPhone),
		})
		validationSpan.End()
		logger.Warn("invalid phone number length", zap.String("phone", fullPhone), zap.Int("length", len(fullPhone)))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
		return
	}
	utils.AddSpanAttribute(validationSpan, "validation.valid", true)
	utils.AddSpanAttribute(validationSpan, "phone", fullPhone)
	validationSpan.End()

	// DON'T update self-declared phone data yet - only store verification data
	// This preserves any existing verified phone until the new one is verified
	ctx, skipUpdateSpan := utils.TraceBusinessLogic(ctx, "skip_phone_update_until_verified")
	logger.Debug("skipping phone update until verification - preserving existing verified phone",
		zap.String("cpf", cpf),
		zap.String("new_phone", fullPhone))
	skipUpdateSpan.End()

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
	if current != nil && current.Telefone != nil && current.Telefone.Principal != nil {
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
// @Success 200 {object} SuccessResponse "Email autodeclarado atualizado com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou dados de email incorretos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 409 {object} ErrorResponse "Conflito - email não alterado (email corresponde aos dados atuais)"
// @Failure 422 {object} ErrorResponse "Dados não processáveis - formato de email inválido"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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
	// Only return 409 if email matches AND the data is not outdated
	// This allows users to re-enter the same email if:
	// 1. The data is outdated (updated_at > threshold ago)
	// 2. No updated_at timestamp exists (legacy data)
	if current != nil && current.Email != nil && current.Email.Principal != nil &&
		current.Email.Principal.Valor != nil && *current.Email.Principal.Valor == input.Valor {

		// Check if data is outdated (allow re-declaration if outdated or no timestamp)
		isOutdated := current.UpdatedAt == nil || time.Since(*current.UpdatedAt) > config.AppConfig.SelfDeclaredOutdatedThreshold

		if !isOutdated {
			// Data is recent and matches - return conflict
			compareSpan.End()
			c.JSON(http.StatusConflict, ErrorResponse{Error: "No change: email matches current data"})
			return
		}

		// Data is outdated or has no timestamp - allow re-declaration
		logger.Debug("allowing email re-declaration for outdated data",
			zap.String("cpf", cpf),
			zap.Bool("has_updated_at", current.UpdatedAt != nil),
			zap.Duration("threshold", config.AppConfig.SelfDeclaredOutdatedThreshold))
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
	if current != nil && current.Email != nil && current.Email.Principal != nil && current.Email.Principal.Valor != nil {
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
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 422 {object} ErrorResponse "Dados não processáveis - valor de etnia não é válido"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
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

// UpdateSelfDeclaredNomeExibicao godoc
// @Summary Atualizar nome de exibição autodeclarado
// @Description Atualiza ou cria o nome de exibição autodeclarado de um cidadão por CPF. Apenas o campo de nome de exibição é atualizado. O nome de exibição é o nome que aparece na interface do usuário, permitindo ao cidadão controlar como seu nome é exibido no aplicativo.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Param data body models.SelfDeclaredNomeExibicaoInput true "Nome de exibição autodeclarado"
// @Security BearerAuth
// @Success 200 {object} SuccessResponse "Nome de exibição atualizado com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou valor de nome de exibição inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 422 {object} ErrorResponse "Dados não processáveis - nome de exibição muito longo ou vazio"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /citizen/{cpf}/exhibition-name [put]
func UpdateSelfDeclaredNomeExibicao(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateSelfDeclaredNomeExibicao")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger().With(zap.String("cpf", cpf))

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "update_exhibition_name"),
		attribute.String("service", "citizen"),
	)

	logger.Debug("UpdateSelfDeclaredNomeExibicao called", zap.String("cpf", cpf))

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
	ctx, inputSpan := utils.TraceInputParsing(ctx, "exhibition_name")
	var input models.SelfDeclaredNomeExibicaoInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "SelfDeclaredNomeExibicaoInput",
		})
		inputSpan.End()
		logger.Error("failed to parse input", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid input format"})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.valor", input.Valor)
	inputSpan.End()

	// Basic validation - check if name is not empty and not too long
	ctx, validationSpan := utils.TraceInputValidation(ctx, "exhibition_name_value", "exhibition_name")
	if len(strings.TrimSpace(input.Valor)) == 0 {
		utils.RecordErrorInSpan(validationSpan, fmt.Errorf("exhibition name cannot be empty"), map[string]interface{}{
			"invalid_value": input.Valor,
		})
		validationSpan.End()
		logger.Error("exhibition name cannot be empty", zap.String("value", input.Valor))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "exhibition name cannot be empty"})
		return
	}
	if len(input.Valor) > 255 {
		utils.RecordErrorInSpan(validationSpan, fmt.Errorf("exhibition name too long: %d characters", len(input.Valor)), map[string]interface{}{
			"invalid_value": input.Valor,
			"length":        len(input.Valor),
		})
		validationSpan.End()
		logger.Error("exhibition name too long", zap.String("value", input.Valor), zap.Int("length", len(input.Valor)))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "exhibition name too long (maximum 255 characters)"})
		return
	}
	utils.AddSpanAttribute(validationSpan, "validated_value", input.Valor)
	validationSpan.End()

	// Use cache service for update with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "update_exhibition_name_via_cache")
	cacheService := services.NewCacheService()
	err := cacheService.UpdateSelfDeclaredNomeExibicao(ctx, cpf, input.Valor)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"cache.operation": "update_self_declared_nome_exibicao",
			"cache.service":   "unified_cache_service",
		})
		updateSpan.End()
		observability.DatabaseOperations.WithLabelValues("update", "error").Inc()
		logger.Error("failed to update self-declared exhibition name via cache service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	utils.AddSpanAttribute(updateSpan, "new_exhibition_name", input.Valor)
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
	ctx, auditSpan := utils.TraceAuditLogging(ctx, "update", "exhibition_name")
	auditCtx := utils.AuditContext{
		CPF:       cpf,
		UserID:    c.GetString("user_id"),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		RequestID: c.GetString("RequestID"),
	}

	// For audit, we log the new value as both old and new for now since we don't have historical data
	err = utils.LogExhibitionNameUpdate(ctx, auditCtx, "", input.Valor)
	if err != nil {
		utils.RecordErrorInSpan(auditSpan, err, map[string]interface{}{
			"audit.action":   "update",
			"audit.resource": "exhibition_name",
		})
		logger.Warn("failed to log audit event", zap.Error(err))
	}
	auditSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Self-declared exhibition name updated successfully"})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("UpdateSelfDeclaredNomeExibicao completed",
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
// @Success 200 {string} string "Métricas Prometheus obtidas com sucesso"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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
// @Success 200 {object} models.UserConfigResponse "Status do primeiro login obtido com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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
		// Use the new ErrDocumentNotFound error type for better error handling
		if err == services.ErrDocumentNotFound {
			// If no config exists, it's first login
			utils.AddSpanAttribute(dbSpan, "user_config.found", false)
			utils.AddSpanAttribute(dbSpan, "user_config.first_login", true)
			utils.AddSpanAttribute(dbSpan, "user_config.default_response", true)
			dbSpan.End()

			logger.Debug("no user config found - returning first login true",
				zap.String("cpf", cpf),
				zap.String("reason", "document_not_found"))

			// Serialize response with tracing
			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, models.UserConfigResponse{FirstLogin: true})
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Debug("GetFirstLogin completed (first login - no config)",
				zap.String("cpf", cpf),
				zap.Bool("first_login", true),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
		// Handle other database errors
		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"operation":  "dataManager.Read",
			"cpf":        cpf,
			"type":       "user_config",
			"error_type": "database_error",
		})
		dbSpan.End()
		logger.Error("failed to get user config via DataManager",
			zap.String("cpf", cpf),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get user config"})
		return
	}
	utils.AddSpanAttribute(dbSpan, "user_config.found", true)
	utils.AddSpanAttribute(dbSpan, "user_config.first_login", userConfig.FirstLogin)
	utils.AddSpanAttribute(dbSpan, "user_config.opt_in", userConfig.OptIn)
	utils.AddSpanAttribute(dbSpan, "user_config.updated_at", userConfig.UpdatedAt.String())
	dbSpan.End()

	logger.Debug("user config found and returned",
		zap.String("cpf", cpf),
		zap.Bool("first_login", userConfig.FirstLogin),
		zap.Bool("opt_in", userConfig.OptIn),
		zap.Time("updated_at", userConfig.UpdatedAt))

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
// @Success 200 {object} models.UserConfigResponse "Status do primeiro login atualizado com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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
		OptIn:      true, // Default to opted in
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
// @Success 200 {object} models.UserConfigOptInResponse "Status do opt-in obtido com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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
		// Use the new ErrDocumentNotFound error type for better error handling
		if err == services.ErrDocumentNotFound {
			// If no config exists, default to opted in
			utils.AddSpanAttribute(dbSpan, "user_config.found", false)
			utils.AddSpanAttribute(dbSpan, "user_config.opt_in", true)
			utils.AddSpanAttribute(dbSpan, "user_config.default_value", true)
			dbSpan.End()

			logger.Debug("no user config found - returning default opt_in true",
				zap.String("cpf", cpf),
				zap.String("reason", "document_not_found"))

			// Serialize response with tracing (with nil category_opt_ins for backward compatibility)
			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, models.UserConfigOptInResponse{
				OptIn:          true,
				CategoryOptIns: nil, // Empty for users without config
			})
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Debug("GetOptIn completed (default value - no config)",
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
	c.JSON(http.StatusOK, models.UserConfigOptInResponse{
		OptIn:          userConfig.OptIn,
		CategoryOptIns: userConfig.CategoryOptIns,
	})
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
// @Success 200 {object} models.UserConfigOptInResponse "Status do opt-in atualizado com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou dados de opt-in incorretos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 422 {object} ErrorResponse "Dados não processáveis - valor de opt-in inválido"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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
// @Success 200 {array} string "Lista de opções de etnia válidas obtida com sucesso"
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
// @Success 200 {object} models.CitizenWallet "Dados da carteira do cidadão obtidos com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Cidadão não encontrado"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
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
		if err == services.ErrDocumentNotFound {
			logger.Debug("citizen wallet not found",
				zap.String("cpf", cpf),
				zap.String("collection", config.AppConfig.CitizenCollection))
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

	// Check if we need to populate CF data in saude.clinica_familia
	ctx, cfDataSpan := utils.TraceBusinessLogic(ctx, "cf_data_integration_wallet")
	needsCFData := false
	if wallet.Saude == nil || wallet.Saude.ClinicaFamilia == nil ||
		wallet.Saude.ClinicaFamilia.Indicador == nil || !*wallet.Saude.ClinicaFamilia.Indicador {
		needsCFData = true
	}

	logger.Info("WALLET CF CHECK", zap.Bool("needs_cf_data", needsCFData))

	// Debug the specific condition checks
	saudeIsNil := wallet.Saude == nil
	clinicaFamiliaIsNil := wallet.Saude == nil || wallet.Saude.ClinicaFamilia == nil
	indicadorIsNil := wallet.Saude == nil || wallet.Saude.ClinicaFamilia == nil || wallet.Saude.ClinicaFamilia.Indicador == nil
	indicadorIsFalse := false
	if wallet.Saude != nil && wallet.Saude.ClinicaFamilia != nil && wallet.Saude.ClinicaFamilia.Indicador != nil {
		indicadorIsFalse = !*wallet.Saude.ClinicaFamilia.Indicador
	}

	logger.Info("CF data integration detailed check",
		zap.String("cpf", cpf),
		zap.Bool("needs_cf_data", needsCFData),
		zap.Bool("saude_is_nil", saudeIsNil),
		zap.Bool("clinica_familia_is_nil", clinicaFamiliaIsNil),
		zap.Bool("indicador_is_nil", indicadorIsNil),
		zap.Bool("indicador_is_false", indicadorIsFalse),
		zap.String("operation", "cf_integration_debug"))

	if needsCFData {
		logger.Debug("attempting to get CF data for citizen", zap.String("cpf", cpf))

		// Check if CF lookup service is available
		if services.CFLookupServiceInstance == nil {
			logger.Info("CF lookup service disabled - skipping CF data integration", zap.String("cpf", cpf))
		} else {
			// First try to get existing cached CF data
			logger.Info("CHECKING FOR CACHED CF DATA", zap.String("cpf", cpf))
			cfData, err := services.CFLookupServiceInstance.GetCFDataForCitizen(ctx, cpf)
			if err != nil || cfData == nil || !cfData.IsActive {
				// No cached data, try synchronous CF lookup
				logger.Info("NO CACHED CF DATA - ATTEMPTING SYNCHRONOUS LOOKUP", zap.String("cpf", cpf))

				// Get citizen address for CF lookup - prioritize self-declared address directly
				address := getSelfDeclaredAddressForCFLookup(ctx, cpf)
				if address == "" {
					// Fallback to extraction from citizen data
					address = services.CFLookupServiceInstance.ExtractAddress(&citizen)
				}
				logger.Info("EXTRACTED ADDRESS FOR CF LOOKUP", zap.String("address", address))

				if address != "" {
					logger.Info("CALLING TrySynchronousCFLookup", zap.String("cpf", cpf), zap.String("address", address))
					cfData, err = services.CFLookupServiceInstance.TrySynchronousCFLookup(ctx, cpf, address)
					if err != nil {
						logger.Info("SYNCHRONOUS CF LOOKUP FAILED", zap.Error(err), zap.String("cpf", cpf))
					} else {
						logger.Info("SYNCHRONOUS CF LOOKUP RESULT", zap.Bool("cf_data_found", cfData != nil))
					}
				} else {
					logger.Info("NO ADDRESS EXTRACTED - SKIPPING CF LOOKUP", zap.String("cpf", cpf))
				}
			} else {
				logger.Info("FOUND CACHED CF DATA", zap.String("cpf", cpf), zap.Bool("is_active", cfData.IsActive))
			}

			if cfData != nil && cfData.IsActive {
				logger.Info("integrating CF data into wallet saude.clinica_familia",
					zap.String("cpf", cpf),
					zap.String("cf_name", cfData.CFData.NomePopular),
					zap.String("cf_address", cfData.AddressUsed),
					zap.String("cf_source", cfData.LookupSource),
					zap.String("operation", "cf_integration_success"))

				// Initialize saude if needed
				if wallet.Saude == nil {
					wallet.Saude = &models.Saude{}
				}

				// Replace/populate clinica_familia with CF data
				wallet.Saude.ClinicaFamilia = cfData.ToClinicaFamilia()

				// Replace/populate equipe_saude_familia with Family Health Team data if available
				if cfData.EquipeSaudeData != nil {
					logger.Info("integrating Family Health Team data into wallet saude.equipe_saude_familia",
						zap.String("cpf", cpf),
						zap.String("team_name", cfData.EquipeSaudeData.NomeOficial),
						zap.Int("doctors_count", len(cfData.EquipeSaudeData.Medicos)),
						zap.Int("nurses_count", len(cfData.EquipeSaudeData.Enfermeiros)),
						zap.String("operation", "family_health_team_integration_success"))

					wallet.Saude.EquipeSaudeFamilia = cfData.ToEquipeSaudeFamilia()
				}
			} else {
				logger.Debug("no CF data available for citizen",
					zap.String("cpf", cpf),
					zap.Bool("cf_data_is_nil", cfData == nil),
					zap.Bool("cf_data_is_active", cfData != nil && cfData.IsActive))
			}
		}
	} else if wallet.Saude != nil && wallet.Saude.ClinicaFamilia != nil {
		// Mark existing CF data as coming from bigquery
		fonte := "bigquery"
		wallet.Saude.ClinicaFamilia.Fonte = &fonte
	}
	cfDataSpan.End()
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
// @Success 200 {object} models.PaginatedMaintenanceRequests "Lista paginada de chamados do 1746 obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou parâmetros de paginação inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
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
		SetSort(bson.D{{Key: "data_inicio", Value: -1}}) // Sort by data_inicio descending (newest first)

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

	// Build addresses for maintenance requests with tracing
	_, addressSpan := utils.TraceBusinessLogic(ctx, "build_maintenance_request_addresses")
	for i := range requests {
		request := &requests[i]

		// Extract address components
		var bairroID, logradouroID string
		var numeroLogradouro interface{}

		if request.IDBairro != nil {
			bairroID = *request.IDBairro
		}
		if request.IDLogradouro != nil {
			logradouroID = *request.IDLogradouro
		}
		if request.NumeroLogradouro != nil {
			numeroLogradouro = *request.NumeroLogradouro
		}

		// Build address if we have address components
		if bairroID != "" || logradouroID != "" {
			endereco, err := services.AddressServiceInstance.BuildAddress(ctx, bairroID, logradouroID, numeroLogradouro)
			if err != nil {
				logger.Warn("failed to build address for maintenance request",
					zap.Error(err),
					zap.String("request_id", request.ID),
					zap.String("bairro_id", bairroID),
					zap.String("logradouro_id", logradouroID))
			} else if endereco != nil {
				request.Endereco = endereco
			}
		}
	}
	utils.AddSpanAttribute(addressSpan, "addresses_built", len(requests))
	addressSpan.End()

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
// @Success 200 {object} models.ValidatePhoneVerificationResponse "Código de verificação validado com sucesso e telefone ativado"
// @Failure 400 {object} ErrorResponse "Formato de CPF inválido ou código de verificação incorreto"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Código de verificação não encontrado ou expirado"
// @Failure 422 {object} ErrorResponse "Dados não processáveis - código de verificação inválido"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
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
		CPF       string           `json:"cpf"`
		Endereco  *models.Endereco `json:"endereco"`
		UpdatedAt string           `json:"updated_at"`
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

// PhoneDataWithTimestamp holds phone data with its update timestamp
type PhoneDataWithTimestamp struct {
	Telefone  *models.Telefone
	UpdatedAt *time.Time
}

// getCurrentPhoneData gets only the phone field and updated_at from self_declared data for comparison
func getCurrentPhoneData(ctx context.Context, cpf string) (*PhoneDataWithTimestamp, error) {
	// Try to get from cache first using DataManager
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())

	var phoneData struct {
		CPF       string           `json:"cpf"`
		Telefone  *models.Telefone `json:"telefone"`
		UpdatedAt string           `json:"updated_at"`
	}

	err := dataManager.Read(ctx, cpf, config.AppConfig.SelfDeclaredCollection, "self_declared_phone", &phoneData)
	if err == nil && phoneData.Telefone != nil {
		var updatedAt *time.Time
		if phoneData.UpdatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, phoneData.UpdatedAt); err == nil {
				updatedAt = &parsed
			}
		}
		return &PhoneDataWithTimestamp{
			Telefone:  phoneData.Telefone,
			UpdatedAt: updatedAt,
		}, nil
	}

	// Fallback to MongoDB with field projection (get telefone and updated_at fields)
	var selfDeclared struct {
		Telefone  *models.Telefone `bson:"telefone"`
		UpdatedAt *time.Time       `bson:"updated_at"`
	}
	err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(
		ctx,
		bson.M{"cpf": cpf},
		options.FindOne().SetProjection(bson.M{"telefone": 1, "updated_at": 1}),
	).Decode(&selfDeclared)

	if err == mongo.ErrNoDocuments {
		return nil, nil // No data found, this is okay
	}
	if err != nil {
		return nil, err
	}

	return &PhoneDataWithTimestamp{
		Telefone:  selfDeclared.Telefone,
		UpdatedAt: selfDeclared.UpdatedAt,
	}, nil
}

// EmailDataWithTimestamp holds email data with its update timestamp
type EmailDataWithTimestamp struct {
	Email     *models.Email
	UpdatedAt *time.Time
}

// getCurrentEmailData gets only the email field and updated_at from self_declared data for comparison
func getCurrentEmailData(ctx context.Context, cpf string) (*EmailDataWithTimestamp, error) {
	// Try to get from cache first using DataManager
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())

	var emailData struct {
		CPF       string        `json:"cpf"`
		Email     *models.Email `json:"email"`
		UpdatedAt string        `json:"updated_at"`
	}

	err := dataManager.Read(ctx, cpf, config.AppConfig.SelfDeclaredCollection, "self_declared_email", &emailData)
	if err == nil && emailData.Email != nil {
		var updatedAt *time.Time
		if emailData.UpdatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, emailData.UpdatedAt); err == nil {
				updatedAt = &parsed
			}
		}
		return &EmailDataWithTimestamp{
			Email:     emailData.Email,
			UpdatedAt: updatedAt,
		}, nil
	}

	// Fallback to MongoDB with field projection (get email and updated_at fields)
	var selfDeclared struct {
		Email     *models.Email `bson:"email"`
		UpdatedAt *time.Time    `bson:"updated_at"`
	}
	err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(
		ctx,
		bson.M{"cpf": cpf},
		options.FindOne().SetProjection(bson.M{"email": 1, "updated_at": 1}),
	).Decode(&selfDeclared)

	if err == mongo.ErrNoDocuments {
		return nil, nil // No data found, this is okay
	}
	if err != nil {
		return nil, err
	}

	return &EmailDataWithTimestamp{
		Email:     selfDeclared.Email,
		UpdatedAt: selfDeclared.UpdatedAt,
	}, nil
}

// getSelfDeclaredAddressForCFLookup gets the current self-declared address for CF lookup
func getSelfDeclaredAddressForCFLookup(ctx context.Context, cpf string) string {
	// Try to get from cache first using DataManager
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, observability.Logger())

	var addressData struct {
		CPF       string           `json:"cpf"`
		Endereco  *models.Endereco `json:"endereco"`
		UpdatedAt string           `json:"updated_at"`
	}

	err := dataManager.Read(ctx, cpf, config.AppConfig.SelfDeclaredCollection, "self_declared_address", &addressData)
	if err == nil && addressData.Endereco != nil && addressData.Endereco.Principal != nil {
		return buildAddressString(addressData.Endereco.Principal)
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

	if err == nil && selfDeclared.Endereco != nil && selfDeclared.Endereco.Principal != nil {
		return buildAddressString(selfDeclared.Endereco.Principal)
	}

	return ""
}

// buildAddressString builds a complete address string from address components
func buildAddressString(endereco *models.EnderecoPrincipal) string {
	if endereco.Logradouro == nil || *endereco.Logradouro == "" {
		return ""
	}

	parts := []string{*endereco.Logradouro}

	if endereco.Numero != nil && *endereco.Numero != "" {
		parts = append(parts, *endereco.Numero)
	}

	if endereco.Complemento != nil && *endereco.Complemento != "" {
		parts = append(parts, *endereco.Complemento)
	}

	if endereco.Bairro != nil && *endereco.Bairro != "" {
		parts = append(parts, *endereco.Bairro)
	}

	if endereco.Municipio != nil && *endereco.Municipio != "" {
		parts = append(parts, *endereco.Municipio)
	} else {
		parts = append(parts, "Rio de Janeiro") // Default city
	}

	if endereco.Estado != nil && *endereco.Estado != "" {
		parts = append(parts, *endereco.Estado)
	} else {
		parts = append(parts, "RJ") // Default state
	}

	return strings.Join(parts, ", ")
}
