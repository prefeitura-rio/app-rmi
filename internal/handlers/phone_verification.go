package handlers

import (
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
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// ValidatePhoneVerification godoc
// @Summary Validar verificação de telefone
// @Description Valida o código de verificação enviado para o número de telefone
// @Tags citizen
// @Accept json
// @Produce json
// @Param cpf path string true "Número do CPF"
// @Param data body models.PhoneVerificationValidateRequest true "Código de verificação e telefone"
// @Security BearerAuth
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /citizen/{cpf}/phone/validate [post]
func ValidatePhoneVerification(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ValidatePhoneVerification")
	defer span.End()

	cpf := c.Param("cpf")

	// Add CPF to span attributes
	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "validate_phone_verification"),
		attribute.String("service", "phone_verification"),
	)

	logger := observability.Logger().With(zap.String("cpf", cpf))
	logger.Info("ValidatePhoneVerification called", zap.String("cpf", cpf))

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "phone_verification_validate_request")
	var req models.PhoneVerificationValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "PhoneVerificationValidateRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request body: " + err.Error(),
		})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.ddi", req.DDI)
	utils.AddSpanAttribute(inputSpan, "input.ddd", req.DDD)
	utils.AddSpanAttribute(inputSpan, "input.valor", req.Valor)
	utils.AddSpanAttribute(inputSpan, "input.code", req.Code)
	inputSpan.End()

	// Build full phone number for lookup with tracing
	ctx, buildSpan := utils.TraceBusinessLogic(ctx, "build_full_phone_number")
	fullPhone := req.DDI + req.DDD + req.Valor
	utils.AddSpanAttribute(buildSpan, "full_phone_number", fullPhone)
	buildSpan.End()

	// Find verification request with tracing
	ctx, findSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.PhoneVerificationCollection, "verification_lookup")
	var verification models.PhoneVerification
	err := config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).FindOne(
		ctx,
		bson.M{
			"cpf":          cpf,
			"code":         req.Code,
			"phone_number": fullPhone,
			"expires_at":   bson.M{"$gt": time.Now()},
		},
	).Decode(&verification)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			utils.AddSpanAttribute(findSpan, "verification.found", false)
			utils.AddSpanAttribute(findSpan, "verification.reason", "invalid_or_expired_code")
			findSpan.End()
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Invalid or expired verification code",
			})
			return
		}
		utils.RecordErrorInSpan(findSpan, err, map[string]interface{}{
			"db.collection": config.AppConfig.PhoneVerificationCollection,
			"db.operation":  "find_one",
		})
		findSpan.End()
		observability.Logger().Error("failed to find verification request", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to validate verification code",
		})
		return
	}
	utils.AddSpanAttribute(findSpan, "verification.found", true)
	utils.AddSpanAttribute(findSpan, "verification.cpf", verification.CPF)
	utils.AddSpanAttribute(findSpan, "verification.phone_number", verification.PhoneNumber)
	utils.AddSpanAttribute(findSpan, "verification.expires_at", verification.ExpiresAt.String())
	findSpan.End()

	// Update self-declared data to replace the phone number with the verified one with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "prepare_phone_verification_update")
	origem := "self-declared"
	sistema := "rmi"
	now := time.Now()
	if verification.Telefone != nil {
		verification.Telefone.Indicador = new(bool)
		*verification.Telefone.Indicador = true
		if verification.Telefone.Principal != nil {
			verification.Telefone.Principal.Origem = &origem
			verification.Telefone.Principal.Sistema = &sistema
			verification.Telefone.Principal.UpdatedAt = &now
		}
	}

	update := bson.M{
		"$set": bson.M{
			"telefone":   verification.Telefone,
			"updated_at": time.Now(),
		},
		"$unset": bson.M{
			"telefone_pending": "",
		},
	}
	updateSpan.End()

	// Use transaction for atomic update and cleanup with tracing
	ctx, transactionSpan := utils.TraceDatabaseTransaction(ctx, "phone_verification_transaction")
	operations := []utils.DatabaseOperation{
		{
			Operation: func() error {
				_, err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).UpdateOne(
					ctx,
					bson.M{"cpf": cpf},
					update,
					options.Update().SetUpsert(true),
				)
				return err
			},
			Rollback: func() error {
				// No rollback needed for this operation
				return nil
			},
		},
		{
			Operation: func() error {
				_, err := config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).DeleteOne(
					ctx,
					bson.M{"cpf": verification.CPF, "phone_number": verification.PhoneNumber},
				)
				return err
			},
			Rollback: func() error {
				// No rollback needed for delete operation
				return nil
			},
		},
	}

	if err := utils.ExecuteWithTransaction(ctx, operations); err != nil {
		utils.RecordErrorInSpan(transactionSpan, err, map[string]interface{}{
			"transaction.operations_count": len(operations),
			"transaction.type":             "phone_verification",
		})
		transactionSpan.End()
		observability.Logger().Error("failed to complete phone verification", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to complete phone verification",
		})
		return
	}
	utils.AddSpanAttribute(transactionSpan, "transaction.success", true)
	utils.AddSpanAttribute(transactionSpan, "transaction.operations_count", len(operations))
	transactionSpan.End()

	// Invalidate all related caches with tracing
	ctx, cacheSpan := utils.TraceCacheInvalidation(ctx, fmt.Sprintf("citizen:%s", cpf))
	if err := utils.InvalidateCitizenCache(ctx, cpf); err != nil {
		utils.RecordErrorInSpan(cacheSpan, err, map[string]interface{}{
			"cache.key": fmt.Sprintf("citizen:%s", cpf),
		})
		observability.Logger().Warn("failed to invalidate cache", zap.Error(err))
	}
	cacheSpan.End()

	// Log audit event with tracing
	ctx, auditSpan := utils.TraceAuditLogging(ctx, "phone_verification_success", "phone_verification")
	auditCtx := utils.GetAuditContextFromGin(c, cpf)
	if err := utils.LogPhoneVerificationSuccess(ctx, auditCtx, verification.PhoneNumber); err != nil {
		utils.RecordErrorInSpan(auditSpan, err, map[string]interface{}{
			"audit.action":   "phone_verification_success",
			"audit.resource": "phone_verification",
		})
		observability.Logger().Warn("failed to log audit event", zap.Error(err))
	}
	auditSpan.End()

	// Serialize response with tracing
	ctx, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, SuccessResponse{
		Message: "Phone number verified successfully",
	})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Info("ValidatePhoneVerification completed",
		zap.String("cpf", cpf),
		zap.String("phone_number", fullPhone),
		zap.String("code", req.Code),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
