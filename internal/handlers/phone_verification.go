package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// ValidatePhoneVerification godoc
// @Summary Validate phone verification
// @Description Validates the verification code sent to the phone number
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "CPF number"
// @Param data body models.PhoneVerificationValidateRequest true "Verification code and phone"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/phone/validate [post]
func ValidatePhoneVerification(c *gin.Context) {
	cpf := c.Param("cpf")
	var req models.PhoneVerificationValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request body: " + err.Error(),
		})
		return
	}

	// Build full phone number for lookup
	fullPhone := req.DDI + req.DDD + req.Valor

	// Find verification request
	var verification models.PhoneVerification
	err := config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).FindOne(
		context.Background(),
		bson.M{
			"cpf": cpf,
			"code": req.Code,
			"phone_number": fullPhone,
			"expires_at": bson.M{"$gt": time.Now()},
		},
	).Decode(&verification)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Invalid or expired verification code",
			})
			return
		}
		observability.Logger.Error("failed to find verification request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to validate verification code",
		})
		return
	}

	// Update self-declared data to replace the phone number with the verified one
	origem := "self-declared"
	sistema := "rmi"
	if verification.Telefone != nil {
		verification.Telefone.Indicador = new(bool)
		*verification.Telefone.Indicador = true
		if verification.Telefone.Principal != nil {
			verification.Telefone.Principal.Origem = &origem
			verification.Telefone.Principal.Sistema = &sistema
		}
	}
	update := bson.M{
		"$set": bson.M{
			"telefone": verification.Telefone,
			"updated_at": time.Now(),
		},
	}

	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
		context.Background(),
		bson.M{"cpf": cpf},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		observability.Logger.Error("failed to update self-declared data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to update phone verification status",
		})
		return
	}

	// Delete used verification code
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).DeleteOne(
		context.Background(),
		bson.M{"cpf": verification.CPF, "phone_number": verification.PhoneNumber},
	)

	// Invalidate cache
	cacheKey := fmt.Sprintf("citizen:%s", cpf)
	if err := config.Redis.Del(context.Background(), cacheKey).Err(); err != nil {
		observability.Logger.Warn("failed to invalidate cache", zap.Error(err))
	}

	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Phone number verified successfully",
	})
} 