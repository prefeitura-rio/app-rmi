package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// MemoryModel representa a estrutura de memória
type MemoryModel struct {
	MemoryID    string    `json:"memory_id,omitempty" bson:"memory_id,omitempty"`
	MemoryName  string    `json:"memory_name" bson:"memory_name" binding:"required"`
	Description string    `json:"description" bson:"description" binding:"required"`
	Relevance   string    `json:"relevance" bson:"relevance" binding:"required,oneof=low medium high"`
	MemoryType  string    `json:"memory_type" bson:"memory_type" binding:"required,oneof=base appended"`
	Value       string    `json:"value" bson:"value" binding:"required"`
	CreatedAt   time.Time `json:"created_at,omitempty" bson:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty" bson:"updated_at,omitempty"`
}

// Helper: Get memory list data (cache-aware with MongoDB fallback)
func getMemoryListData(ctx context.Context, phoneNumber string) ([]MemoryModel, error) {
	cacheKey := fmt.Sprintf("memory_list:%s", phoneNumber)

	// Try to get from cache first
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache hit - decode the data
		var memories []MemoryModel
		if err := json.Unmarshal([]byte(cachedData), &memories); err == nil {
			return memories, nil
		}
	}

	// Cache miss - query MongoDB for all memory entries for this phone number with timeout
	collection := config.MongoDB.Collection(config.AppConfig.ChatMemoryCollection)
	cursor, err := utils.FindWithTimeout(ctx, collection, bson.M{"phone_number": phoneNumber}, utils.DefaultQueryTimeout)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var memories []MemoryModel
	if err := cursor.All(ctx, &memories); err != nil {
		return nil, err
	}

	// Cache the result
	if jsonData, err := json.Marshal(memories); err == nil {
		config.Redis.Set(ctx, cacheKey, jsonData, config.AppConfig.RedisTTL)
	}

	return memories, nil
}

// GetMemoryList godoc
// @Summary Obter memórias associadas ao número de telefone.
// @Description Recupera a lista de memórias associadas ao telefone do cidadão.
// @Tags memory
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Security BearerAuth
// @Success 200 {array} MemoryModel "Lista de memórias obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Número de telefone inválido"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /memory/{phone_number} [get]
func GetMemoryList(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetMemoryList")
	defer span.End()

	phoneNumber := c.Param("phone_number")
	logger := observability.Logger().With(zap.String("phone_number", phoneNumber))

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "get_memory_list"),
		attribute.String("service", "memory"),
	)

	logger.Debug("GetMemoryList called", zap.String("phone_number", phoneNumber))

	// Validate phone number with tracing using utils.ValidatePhoneFormat directly
	// ctx, phoneSpan := utils.TraceInputValidation(ctx, "phone_number_format", "phone_number")
	// if err := utils.ValidatePhoneFormat(phoneNumber); err != nil {
	// 	utils.RecordErrorInSpan(phoneSpan, fmt.Errorf("Invalid phone number format"), map[string]interface{}{
	// 		"phone_number": phoneNumber,
	// 	})
	// 	phoneSpan.End()
	// 	c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
	// 	return
	// }
	// phoneSpan.End()

	// Use getMemoryListData which implements cache-aware reading for multiple entries
	ctx, getDataSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.ChatMemoryCollection, "phone_number")
	memories, err := getMemoryListData(ctx, phoneNumber)
	if err != nil {
		utils.RecordErrorInSpan(getDataSpan, err, map[string]interface{}{
			"operation":    "getMemoryListData",
			"phone_number": phoneNumber,
		})
		getDataSpan.End()
		logger.Error("failed to get memory list data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}
	getDataSpan.End()

	// Check if memories list is empty
	if len(memories) == 0 {
		c.JSON(http.StatusOK, []MemoryModel{}) // Return empty array instead of 404
		return
	}

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, memories)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetMemoryList completed",
		zap.String("phone_number", phoneNumber),
		zap.Int("memory_count", len(memories)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// GetMemoryByName godoc
// @Summary Obter memória associada ao número de telefone pelo nome.
// @Description Recupera uma única memória associada ao telefone do cidadão.
// @Tags memory
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param memory_name path string true "Nome da memória"
// @Security BearerAuth
// @Success 200 {object} MemoryModel "Memória obtida com sucesso"
// @Failure 400 {object} ErrorResponse "Número de telefone ou nome da memória inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Memória não encontrada"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /memory/{phone_number}/{memory_name} [get]
func GetMemoryByName(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetMemoryByName")
	defer span.End()

	phoneNumber := c.Param("phone_number")
	memoryName := c.Param("memory_name")
	logger := observability.Logger().With(
		zap.String("phone_number", phoneNumber),
		zap.String("memory_name", memoryName),
	)

	// Add parameters to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("memory_name", memoryName),
		attribute.String("operation", "get_memory_by_name"),
		attribute.String("service", "memory"),
	)

	logger.Debug("GetMemoryByName called",
		zap.String("phone_number", phoneNumber),
		zap.String("memory_name", memoryName),
	)

	// Validate phone number with tracing using utils.ValidatePhoneFormat directly
	// ctx, phoneSpan := utils.TraceInputValidation(ctx, "phone_number_format", "phone_number")
	// if err := utils.ValidatePhoneFormat(phoneNumber); err != nil {
	// 	utils.RecordErrorInSpan(phoneSpan, fmt.Errorf("Invalid phone number format"), map[string]interface{}{
	// 		"phone_number": phoneNumber,
	// 	})
	// 	phoneSpan.End()
	// 	c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
	// 	return
	// }
	// phoneSpan.End()

	// Validate memory name (basic validation - not empty)
	ctx, memorySpan := utils.TraceInputValidation(ctx, "memory_name_format", "memory_name")
	if memoryName == "" {
		utils.RecordErrorInSpan(memorySpan, fmt.Errorf("Memory name cannot be empty"), map[string]interface{}{
			"memory_name": memoryName,
		})
		memorySpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Memory name cannot be empty"})
		return
	}
	memorySpan.End()

	// First, try to get from cache with composite key
	cacheKey := fmt.Sprintf("memory:%s:%s", phoneNumber, memoryName)
	var memory MemoryModel

	ctx, cacheGetSpan := utils.TraceCacheGet(ctx, cacheKey)
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		// Cache hit - decode the data
		if err := json.Unmarshal([]byte(cachedData), &memory); err == nil {
			utils.AddSpanAttribute(cacheGetSpan, "cache.hit", true)
			cacheGetSpan.End()

			// Return cached data
			_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
			c.JSON(http.StatusOK, memory)
			responseSpan.End()

			// Log total operation time
			totalDuration := time.Since(startTime)
			logger.Debug("GetMemoryByName completed (cache hit)",
				zap.String("phone_number", phoneNumber),
				zap.String("memory_name", memoryName),
				zap.Duration("total_duration", totalDuration),
				zap.String("status", "success"))
			return
		}
	}
	utils.AddSpanAttribute(cacheGetSpan, "cache.hit", false)
	cacheGetSpan.End()

	// Cache miss - query MongoDB with FindOne and timeout
	ctx, dbFindSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.ChatMemoryCollection, "phone_number_and_memory_name")

	// Create a context with timeout for the FindOne operation
	findCtx, cancel := context.WithTimeout(ctx, utils.DefaultQueryTimeout)
	defer cancel()

	err = config.MongoDB.Collection(config.AppConfig.ChatMemoryCollection).FindOne(
		findCtx,
		bson.M{
			"phone_number": phoneNumber,
			"memory_name":  memoryName,
		},
	).Decode(&memory)
	dbFindSpan.End()

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Memory not found for phone number and memory name"})
			return
		}
		utils.RecordErrorInSpan(dbFindSpan, err, map[string]interface{}{
			"operation":    "mongodb_find_one",
			"phone_number": phoneNumber,
			"memory_name":  memoryName,
		})
		logger.Error("failed to find memory data from MongoDB", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}

	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	// Cache the result with tracing
	ctx, cacheSetSpan := utils.TraceCacheSet(ctx, cacheKey, config.AppConfig.RedisTTL)
	if jsonData, err := json.Marshal(memory); err == nil {
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
	c.JSON(http.StatusOK, memory)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetMemoryByName completed",
		zap.String("phone_number", phoneNumber),
		zap.String("memory_name", memoryName),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// CreateMemory godoc
// @Summary Cria memória associada ao número de telefone.
// @Description Cria uma única memória associada ao telefone do cidadão.
// @Tags memory
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param memory body MemoryModel true "Dados da memória a ser criada"
// @Security BearerAuth
// @Success 201 {object} MemoryModel "Memória criada com sucesso"
// @Failure 400 {object} ErrorResponse "Número de telefone ou dados da memória inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 409 {object} ErrorResponse "Memória com esse nome já existe para este número de telefone"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /memory/{phone_number} [post]
func CreateMemory(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "CreateMemory")
	defer span.End()

	// Initialize performance monitoring
	monitor := utils.NewPerformanceMonitor(ctx, "CreateMemory")
	defer monitor.End()

	phoneNumber := c.Param("phone_number")
	logger := observability.Logger().With(zap.String("phone_number", phoneNumber))

	// Add phone number to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "create_memory"),
		attribute.String("service", "memory"),
	)

	logger.Debug("CreateMemory called", zap.String("phone_number", phoneNumber))

	// Validate phone number with tracing using utils.ValidatePhoneFormat directly
	// ctx, phoneSpan := utils.TraceInputValidation(ctx, "phone_number_format", "phone_number")
	// if err := utils.ValidatePhoneFormat(phoneNumber); err != nil {
	// 	utils.RecordErrorInSpan(phoneSpan, fmt.Errorf("Invalid phone number format"), map[string]interface{}{
	// 		"phone_number": phoneNumber,
	// 	})
	// 	phoneSpan.End()
	// 	c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
	// 	return
	// }
	// phoneSpan.End()

	// Parse and validate request body using Gin's built-in validation
	var memory MemoryModel
	ctx, bodySpan := utils.TraceInputValidation(ctx, "request_body", "memory_data")
	if err := c.ShouldBindJSON(&memory); err != nil {
		utils.RecordErrorInSpan(bodySpan, err, map[string]interface{}{
			"operation": "json_binding",
		})
		bodySpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid memory data format"})
		return
	}
	bodySpan.End()

	// Normalize memory name: trim spaces and convert to lowercase for case-insensitive uniqueness
	memory.MemoryName = strings.TrimSpace(strings.ToLower(memory.MemoryName))

	// Always generate memory_id and timestamps on the server, ignore client values
	memory.MemoryID = utils.GenerateUUID()
	currentTime := time.Now().UTC()
	memory.CreatedAt = currentTime
	memory.UpdatedAt = currentTime

	// Check if memory with same name already exists for this phone number
	ctx, checkSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.ChatMemoryCollection, "check_duplicate_memory_name")
	var existingMemory MemoryModel
	err := config.MongoDB.Collection(config.AppConfig.ChatMemoryCollection).FindOne(
		ctx,
		bson.M{
			"phone_number": phoneNumber,
			"memory_name":  memory.MemoryName,
		},
	).Decode(&existingMemory)
	checkSpan.End()

	if err == nil {
		// Memory with same name already exists for this phone number
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Memory name already exists for this phone number"})
		return
	} else if err != mongo.ErrNoDocuments {
		// Some other error occurred
		utils.RecordErrorInSpan(span, err, map[string]interface{}{
			"operation":    "mongodb_find_duplicate",
			"phone_number": phoneNumber,
			"memory_name":  memory.MemoryName,
		})
		logger.Error("failed to check for duplicate memory name", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}

	// Prepare document for MongoDB insertion (include phone_number)
	document := bson.M{
		"phone_number": phoneNumber,
		"memory_id":    memory.MemoryID,
		"memory_name":  memory.MemoryName,
		"description":  memory.Description,
		"relevance":    memory.Relevance,
		"memory_type":  memory.MemoryType,
		"value":        memory.Value,
		"created_at":   memory.CreatedAt,
		"updated_at":   memory.UpdatedAt,
	}

	// Insert into MongoDB with tracing
	ctx, insertSpan := utils.TraceDatabaseUpdate(ctx, config.AppConfig.ChatMemoryCollection, "insert", false)
	_, err = config.MongoDB.Collection(config.AppConfig.ChatMemoryCollection).InsertOne(ctx, document)
	if err != nil {
		utils.RecordErrorInSpan(insertSpan, err, map[string]interface{}{
			"operation":    "mongodb_insert",
			"phone_number": phoneNumber,
			"memory_id":    memory.MemoryID,
		})
		insertSpan.End()

		// Check for duplicate key error (E11000)
		if mongo.IsDuplicateKeyError(err) {
			logger.Warn("duplicate memory name for phone number",
				zap.String("phone_number", phoneNumber),
				zap.String("memory_name", memory.MemoryName),
				zap.Error(err))
			c.JSON(http.StatusConflict, ErrorResponse{Error: "Memory name already exists for this phone number"})
			return
		}

		logger.Error("failed to insert memory into MongoDB", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}
	insertSpan.End()

	observability.DatabaseOperations.WithLabelValues("insert", "success").Inc()

	// Invalidate cache for this phone number using Redis pipeline for batch operations
	ctx, cacheDelSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("memory:%s:%s", phoneNumber, memory.MemoryName))

	keys := []string{
		fmt.Sprintf("memory:%s:%s", phoneNumber, memory.MemoryName),
		fmt.Sprintf("memory_list:%s", phoneNumber),
	}

	pipeline := utils.NewRedisPipeline(ctx)
	if err := pipeline.BatchDelete(keys); err != nil {
		logger.Warn("failed to batch delete cache keys", zap.Error(err))
		// Fall back to individual deletions
		for _, key := range keys {
			config.Redis.Del(ctx, key)
		}
	}
	cacheDelSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusCreated, memory)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("CreateMemory completed",
		zap.String("phone_number", phoneNumber),
		zap.String("memory_id", memory.MemoryID),
		zap.String("memory_name", memory.MemoryName),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// UpdateMemory godoc
// @Summary Atualiza memória associada ao número de telefone.
// @Description Atualiza uma única memória associada ao telefone do cidadão.
// @Tags memory
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param memory body MemoryModel true "Dados da memória a ser atualizada (memory_name identifica qual memória atualizar)"
// @Security BearerAuth
// @Success 200 {object} SuccessResponse "Memória atualizada com sucesso"
// @Failure 400 {object} ErrorResponse "Número de telefone ou nome da memória inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Memória não encontrada"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /memory/{phone_number} [put]
func UpdateMemory(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateMemory")
	defer span.End()

	// Initialize performance monitoring
	monitor := utils.NewPerformanceMonitor(ctx, "UpdateMemory")
	defer monitor.End()

	phoneNumber := c.Param("phone_number")
	logger := observability.Logger().With(
		zap.String("phone_number", phoneNumber),
	)

	// Add parameters to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "update_memory"),
		attribute.String("service", "memory"),
	)

	logger.Debug("UpdateMemory called",
		zap.String("phone_number", phoneNumber),
	)

	// Validate phone number with tracing using utils.ValidatePhoneFormat directly
	// ctx, phoneSpan := utils.TraceInputValidation(ctx, "phone_number_format", "phone_number")
	// if err := utils.ValidatePhoneFormat(phoneNumber); err != nil {
	// 	utils.RecordErrorInSpan(phoneSpan, fmt.Errorf("Invalid phone number format"), map[string]interface{}{
	// 		"phone_number": phoneNumber,
	// 	})
	// 	phoneSpan.End()
	// 	c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
	// 	return
	// }
	// phoneSpan.End()

	// Parse and validate request body using Gin's built-in validation
	var memory MemoryModel
	ctx, bodySpan := utils.TraceInputValidation(ctx, "request_body", "memory_data")
	if err := c.ShouldBindJSON(&memory); err != nil {
		utils.RecordErrorInSpan(bodySpan, err, map[string]interface{}{
			"operation": "json_binding",
		})
		bodySpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid memory data format"})
		return
	}
	bodySpan.End()

	// Normalize memory name: trim spaces and convert to lowercase for case-insensitive uniqueness
	memory.MemoryName = strings.TrimSpace(strings.ToLower(memory.MemoryName))

	// Validate memory name (basic validation - not empty)
	if memory.MemoryName == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Memory name cannot be empty"})
		return
	}

	// Update logger and span with memory_name from body
	logger = logger.With(zap.String("memory_name", memory.MemoryName))
	span.SetAttributes(attribute.String("memory_name", memory.MemoryName))

	// First, check if the memory exists
	ctx, checkSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.ChatMemoryCollection, "check_memory_exists")
	var existingMemory MemoryModel
	err := config.MongoDB.Collection(config.AppConfig.ChatMemoryCollection).FindOne(
		ctx,
		bson.M{
			"phone_number": phoneNumber,
			"memory_name":  memory.MemoryName,
		},
	).Decode(&existingMemory)
	checkSpan.End()

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Memory not found
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Memory not found for phone number and memory name"})
			return
		}
		// Some other error occurred
		utils.RecordErrorInSpan(span, err, map[string]interface{}{
			"operation":    "mongodb_find",
			"phone_number": phoneNumber,
			"memory_name":  memory.MemoryName,
		})
		logger.Error("failed to check if memory exists", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}

	// Update timestamps with UTC for consistency
	currentTime := time.Now().UTC()
	memory.UpdatedAt = currentTime
	// Preserve the original created_at and memory_id
	memory.CreatedAt = existingMemory.CreatedAt
	memory.MemoryID = existingMemory.MemoryID

	// Prepare update document for MongoDB
	update := bson.M{
		"$set": bson.M{
			"memory_name": memory.MemoryName,
			"description": memory.Description,
			"relevance":   memory.Relevance,
			"memory_type": memory.MemoryType,
			"value":       memory.Value,
			"updated_at":  memory.UpdatedAt,
		},
	}

	// Update in MongoDB with tracing
	ctx, updateSpan := utils.TraceDatabaseUpdate(ctx, config.AppConfig.ChatMemoryCollection, "update", false)
	result, err := config.MongoDB.Collection(config.AppConfig.ChatMemoryCollection).UpdateOne(
		ctx,
		bson.M{
			"phone_number": phoneNumber,
			"memory_name":  memory.MemoryName,
		},
		update,
	)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"operation":    "mongodb_update",
			"phone_number": phoneNumber,
			"memory_name":  memory.MemoryName,
		})
		updateSpan.End()
		logger.Error("failed to update memory in MongoDB", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}
	updateSpan.End()

	// Check if any document was actually updated
	if result.MatchedCount == 0 {
		// This should not happen since we checked existence, but handle it anyway
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Memory not found for phone number and memory name"})
		return
	}

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()

	// Handle cache invalidation and update using Redis pipeline
	cacheKey := fmt.Sprintf("memory:%s:%s", phoneNumber, memory.MemoryName)

	// Prepare keys for batch operations
	keysToDelete := []string{
		fmt.Sprintf("memory_list:%s", phoneNumber),
	}

	// Prepare new cache data
	var keysToSet map[string]interface{}
	if jsonData, err := json.Marshal(memory); err == nil {
		keysToSet = map[string]interface{}{
			cacheKey: jsonData,
		}
	}

	// Use Redis pipeline for batch operations
	pipeline := utils.NewRedisPipeline(ctx)

	// Batch delete operations
	if len(keysToDelete) > 0 {
		if err := pipeline.BatchDelete(keysToDelete); err != nil {
			logger.Warn("failed to batch delete cache keys", zap.Error(err))
			// Fall back to individual deletions
			for _, key := range keysToDelete {
				config.Redis.Del(ctx, key)
			}
		}
	}

	// Batch set operations
	if len(keysToSet) > 0 {
		if err := pipeline.BatchSet(keysToSet, config.AppConfig.RedisTTL); err != nil {
			logger.Warn("failed to batch set cache keys", zap.Error(err))
			// Fall back to individual sets
			for key, value := range keysToSet {
				config.Redis.Set(ctx, key, value, config.AppConfig.RedisTTL)
			}
		}
	}

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{Message: "Memory updated successfully"})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("UpdateMemory completed",
		zap.String("phone_number", phoneNumber),
		zap.String("memory_name", memory.MemoryName),
		zap.Int64("matched_count", result.MatchedCount),
		zap.Int64("modified_count", result.ModifiedCount),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// DeleteMemory godoc
// @Summary Deleta memória associada ao número de telefone.
// @Description Deleta uma única memória associada ao telefone do cidadão.
// @Tags memory
// @Accept json
// @Produce json
// @Param phone_number path string true "Número do telefone"
// @Param memory_name path string true "Nome da memória a ser deletada"
// @Security BearerAuth
// @Success 204 "Memória deletada com sucesso"
// @Failure 400 {object} ErrorResponse "Número de telefone ou nome da memória inválidos"
// @Failure 401 {object} ErrorResponse "Token de autenticação não fornecido ou inválido"
// @Failure 403 {object} ErrorResponse "Acesso negado - permissões insuficientes"
// @Failure 404 {object} ErrorResponse "Memória não encontrada"
// @Failure 429 {object} ErrorResponse "Muitas requisições - limite de taxa excedido"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /memory/{phone_number}/{memory_name} [delete]
func DeleteMemory(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "DeleteMemory")
	defer span.End()

	// Initialize performance monitoring
	monitor := utils.NewPerformanceMonitor(ctx, "DeleteMemory")
	defer monitor.End()

	phoneNumber := c.Param("phone_number")
	memoryName := c.Param("memory_name")
	logger := observability.Logger().With(
		zap.String("phone_number", phoneNumber),
		zap.String("memory_name", memoryName),
	)

	// Add parameters to span attributes
	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("memory_name", memoryName),
		attribute.String("operation", "delete_memory"),
		attribute.String("service", "memory"),
	)

	logger.Debug("DeleteMemory called",
		zap.String("phone_number", phoneNumber),
		zap.String("memory_name", memoryName),
	)

	// Validate phone number with tracing using utils.ValidatePhoneFormat directly
	// ctx, phoneSpan := utils.TraceInputValidation(ctx, "phone_number_format", "phone_number")
	// if err := utils.ValidatePhoneFormat(phoneNumber); err != nil {
	// 	utils.RecordErrorInSpan(phoneSpan, fmt.Errorf("Invalid phone number format"), map[string]interface{}{
	// 		"phone_number": phoneNumber,
	// 	})
	// 	phoneSpan.End()
	// 	c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
	// 	return
	// }
	// phoneSpan.End()

	// Validate memory name (basic validation - not empty)
	ctx, memorySpan := utils.TraceInputValidation(ctx, "memory_name_format", "memory_name")
	if memoryName == "" {
		utils.RecordErrorInSpan(memorySpan, fmt.Errorf("Memory name cannot be empty"), map[string]interface{}{
			"memory_name": memoryName,
		})
		memorySpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Memory name cannot be empty"})
		return
	}
	memorySpan.End()

	// First, check if the memory exists
	ctx, checkSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.ChatMemoryCollection, "check_memory_exists")
	var existingMemory MemoryModel
	err := config.MongoDB.Collection(config.AppConfig.ChatMemoryCollection).FindOne(
		ctx,
		bson.M{
			"phone_number": phoneNumber,
			"memory_name":  memoryName,
		},
	).Decode(&existingMemory)
	checkSpan.End()

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Memory not found
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Memory not found for phone number and memory name"})
			return
		}
		// Some other error occurred
		utils.RecordErrorInSpan(span, err, map[string]interface{}{
			"operation":    "mongodb_find",
			"phone_number": phoneNumber,
			"memory_name":  memoryName,
		})
		logger.Error("failed to check if memory exists", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}

	// Delete from MongoDB with tracing
	ctx, deleteSpan := utils.TraceDatabaseUpdate(ctx, config.AppConfig.ChatMemoryCollection, "delete", false)
	result, err := config.MongoDB.Collection(config.AppConfig.ChatMemoryCollection).DeleteOne(
		ctx,
		bson.M{
			"phone_number": phoneNumber,
			"memory_name":  memoryName,
		},
	)
	if err != nil {
		utils.RecordErrorInSpan(deleteSpan, err, map[string]interface{}{
			"operation":    "mongodb_delete",
			"phone_number": phoneNumber,
			"memory_name":  memoryName,
		})
		deleteSpan.End()
		logger.Error("failed to delete memory from MongoDB", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
		return
	}
	deleteSpan.End()

	// Check if any document was actually deleted
	if result.DeletedCount == 0 {
		// This should not happen since we checked existence, but handle it anyway
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Memory not found for phone number and memory name"})
		return
	}

	observability.DatabaseOperations.WithLabelValues("delete", "success").Inc()

	// Invalidate cache for this memory using Redis pipeline for batch operations
	ctx, cacheDelSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("memory:%s:%s", phoneNumber, memoryName))

	keys := []string{
		fmt.Sprintf("memory:%s:%s", phoneNumber, memoryName),
		fmt.Sprintf("memory_list:%s", phoneNumber),
	}

	pipeline := utils.NewRedisPipeline(ctx)
	if err := pipeline.BatchDelete(keys); err != nil {
		logger.Warn("failed to batch delete cache keys", zap.Error(err))
		// Fall back to individual deletions
		for _, key := range keys {
			config.Redis.Del(ctx, key)
		}
	}
	cacheDelSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.Status(http.StatusNoContent)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("DeleteMemory completed",
		zap.String("phone_number", phoneNumber),
		zap.String("memory_name", memoryName),
		zap.Int64("deleted_count", result.DeletedCount),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
