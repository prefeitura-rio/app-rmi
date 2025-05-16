package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetCitizenData godoc
// @Summary Get citizen data
// @Description Retrieves citizen data by CPF, combining base data with any self-declared updates. Self-declared data takes precedence over base data. Results are cached using Redis with configurable TTL.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Citizen CPF (11 digits)" minLength(11) maxLength(11)
// @Success 200 {object} models.Citizen "Complete citizen data"
// @Failure 400 {object} ErrorResponse "Invalid CPF format"
// @Failure 404 {object} ErrorResponse "Citizen not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /citizen/{cpf} [get]
func GetCitizenData(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetCitizenData")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger.With(zap.String("cpf", cpf))

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
	err = config.MongoDB.Collection("data_rmi").FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
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
	err = config.MongoDB.Collection("self_declared").FindOne(ctx, bson.M{"cpf": cpf}).Decode(&selfDeclared)
	if err == nil {
		observability.DatabaseOperations.WithLabelValues("find", "success").Inc()
		// Merge self-declared data
		if selfDeclared.Endereco != nil {
			citizen.Endereco = selfDeclared.Endereco
		}
		if selfDeclared.Contato != nil {
			citizen.Contato = selfDeclared.Contato
		}
	} else if err != mongo.ErrNoDocuments {
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Warn("failed to get self-declared data", zap.Error(err))
	}

	// Cache the result
	if jsonData, err := json.Marshal(citizen); err == nil {
		config.Redis.Set(ctx, cacheKey, jsonData, config.AppConfig.CacheTTL)
	}

	c.JSON(http.StatusOK, citizen)
}

// UpdateSelfDeclaredData godoc
// @Summary Update self-declared data
// @Description Updates or creates self-declared information for a citizen. Only specific fields (address and contact) can be updated. All fields are optional, and only provided fields will be updated.
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Citizen CPF (11 digits)" minLength(11) maxLength(11)
// @Param data body models.CadastroUpdate true "Update Data"
// @Success 200 {object} SuccessResponse "Data updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid request body or validation errors"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /citizen/{cpf}/self-declared [put]
func UpdateSelfDeclaredData(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateSelfDeclaredData")
	defer span.End()

	cpf := c.Param("cpf")
	logger := observability.Logger.With(zap.String("cpf", cpf))

	var updateData models.CadastroUpdate
	if err := c.ShouldBindJSON(&updateData); err != nil {
		logger.Warn("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	// Convert to SelfDeclaredData
	selfDeclared := updateData.ToSelfDeclaredData()
	selfDeclared.CPF = cpf

	// Update or insert self-declared data
	filter := bson.M{"cpf": cpf}
	update := bson.M{"$set": selfDeclared}
	opts := options.Update().SetUpsert(true)

	_, err := config.MongoDB.Collection("self_declared").UpdateOne(ctx, filter, update, opts)
	if err != nil {
		observability.DatabaseOperations.WithLabelValues("update", "error").Inc()
		observability.SelfDeclaredUpdates.WithLabelValues("error").Inc()
		logger.Error("failed to update self-declared data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()
	observability.SelfDeclaredUpdates.WithLabelValues("success").Inc()

	// Invalidate cache
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	config.Redis.Del(ctx, cacheKey)

	c.JSON(http.StatusOK, SuccessResponse{Message: "data updated successfully"})
}

// HealthCheck godoc
// @Summary Health check endpoint
// @Description Checks the health of the API and its dependencies (MongoDB and Redis). Returns detailed status for each service.
// @Tags health
// @Produce json
// @Success 200 {object} HealthResponse "All services are healthy"
// @Failure 503 {object} HealthResponse "One or more services are unhealthy"
// @Router /health [get]
func HealthCheck(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "HealthCheck")
	defer span.End()

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

	if health.Status == "healthy" {
		c.JSON(http.StatusOK, health)
	} else {
		c.JSON(http.StatusServiceUnavailable, health)
	}
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