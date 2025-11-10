package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

type NotificationCategoryHandlers struct {
	service *services.NotificationCategoryService
	logger  *logging.SafeLogger
}

func NewNotificationCategoryHandlers(logger *logging.SafeLogger) *NotificationCategoryHandlers {
	return &NotificationCategoryHandlers{
		service: services.NewNotificationCategoryService(logger),
		logger:  logger,
	}
}

// ListCategories godoc
// @Summary List notification categories
// @Description List all active notification categories
// @Tags notification-categories
// @Accept json
// @Produce json
// @Success 200 {object} models.NotificationCategoriesResponse "List of active categories"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /notification-categories [get]
func (h *NotificationCategoryHandlers) ListCategories(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "ListNotificationCategories")
	defer span.End()

	span.SetAttributes(
		attribute.String("operation", "list_notification_categories"),
		attribute.String("service", "notification_category"),
	)

	h.logger.Debug("ListNotificationCategories called")

	// List active categories with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "notification_category_service", "list_active")
	categories, err := h.service.ListActive(ctx)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "notification_category_service",
			"service.operation": "list_active",
		})
		serviceSpan.End()
		h.logger.Error("failed to list categories", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to list categories"})
		return
	}
	utils.AddSpanAttribute(serviceSpan, "categories.count", len(categories))
	serviceSpan.End()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, models.NotificationCategoriesResponse{Categories: categories})
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("ListNotificationCategories completed",
		zap.Int("count", len(categories)),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// CreateCategory godoc
// @Summary Create notification category
// @Description Create a new notification category (admin only)
// @Tags notification-categories
// @Accept json
// @Produce json
// @Param data body models.CreateNotificationCategoryRequest true "Category data"
// @Security BearerAuth
// @Success 201 {object} models.NotificationCategory "Category created successfully"
// @Failure 400 {object} ErrorResponse "Invalid request body"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 409 {object} ErrorResponse "Category already exists"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /admin/notification-categories [post]
func (h *NotificationCategoryHandlers) CreateCategory(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "CreateNotificationCategory")
	defer span.End()

	span.SetAttributes(
		attribute.String("operation", "create_notification_category"),
		attribute.String("service", "notification_category"),
	)

	h.logger.Debug("CreateNotificationCategory called")

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "create_category_request")
	var req models.CreateNotificationCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "CreateNotificationCategoryRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	utils.AddSpanAttribute(inputSpan, "input.id", req.ID)
	utils.AddSpanAttribute(inputSpan, "input.name", req.Name)
	inputSpan.End()

	// Create category with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "notification_category_service", "create")
	category, err := h.service.Create(ctx, req)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "notification_category_service",
			"service.operation": "create",
		})
		serviceSpan.End()
		h.logger.Error("failed to create category", zap.Error(err), zap.String("id", req.ID))
		if err.Error() == "category with ID "+req.ID+" already exists" {
			c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create category"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "category.id", category.ID)
	serviceSpan.End()

	observability.DatabaseOperations.WithLabelValues("create", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusCreated, category)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("CreateNotificationCategory completed",
		zap.String("id", category.ID),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// UpdateCategory godoc
// @Summary Update notification category
// @Description Update an existing notification category (admin only)
// @Tags notification-categories
// @Accept json
// @Produce json
// @Param category_id path string true "Category ID"
// @Param data body models.UpdateNotificationCategoryRequest true "Updated category data"
// @Security BearerAuth
// @Success 200 {object} models.NotificationCategory "Category updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid request body"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 404 {object} ErrorResponse "Category not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /admin/notification-categories/{category_id} [put]
func (h *NotificationCategoryHandlers) UpdateCategory(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateNotificationCategory")
	defer span.End()

	categoryID := c.Param("category_id")

	span.SetAttributes(
		attribute.String("category_id", categoryID),
		attribute.String("operation", "update_notification_category"),
		attribute.String("service", "notification_category"),
	)

	h.logger.Debug("UpdateNotificationCategory called", zap.String("category_id", categoryID))

	// Parse input with tracing
	ctx, inputSpan := utils.TraceInputParsing(ctx, "update_category_request")
	var req models.UpdateNotificationCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "UpdateNotificationCategoryRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	inputSpan.End()

	// Update category with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "notification_category_service", "update")
	category, err := h.service.Update(ctx, categoryID, req)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "notification_category_service",
			"service.operation": "update",
		})
		serviceSpan.End()
		h.logger.Error("failed to update category", zap.Error(err), zap.String("category_id", categoryID))
		if err.Error() == "category with ID "+categoryID+" not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update category"})
		}
		return
	}
	utils.AddSpanAttribute(serviceSpan, "category.id", category.ID)
	serviceSpan.End()

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()

	// Serialize response with tracing
	_, responseSpan := utils.TraceResponseSerialization(ctx, "success")
	c.JSON(http.StatusOK, category)
	responseSpan.End()

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("UpdateNotificationCategory completed",
		zap.String("category_id", categoryID),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// DeleteCategory godoc
// @Summary Delete notification category
// @Description Soft-delete a notification category by setting active=false (admin only)
// @Tags notification-categories
// @Accept json
// @Produce json
// @Param category_id path string true "Category ID"
// @Security BearerAuth
// @Success 204 "Category deleted successfully"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 404 {object} ErrorResponse "Category not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /admin/notification-categories/{category_id} [delete]
func (h *NotificationCategoryHandlers) DeleteCategory(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "DeleteNotificationCategory")
	defer span.End()

	categoryID := c.Param("category_id")

	span.SetAttributes(
		attribute.String("category_id", categoryID),
		attribute.String("operation", "delete_notification_category"),
		attribute.String("service", "notification_category"),
	)

	h.logger.Debug("DeleteNotificationCategory called", zap.String("category_id", categoryID))

	// Delete category with tracing
	ctx, serviceSpan := utils.TraceExternalService(ctx, "notification_category_service", "delete")
	err := h.service.Delete(ctx, categoryID)
	if err != nil {
		utils.RecordErrorInSpan(serviceSpan, err, map[string]interface{}{
			"service.name":      "notification_category_service",
			"service.operation": "delete",
		})
		serviceSpan.End()
		h.logger.Error("failed to delete category", zap.Error(err), zap.String("category_id", categoryID))
		if err.Error() == "category with ID "+categoryID+" not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete category"})
		}
		return
	}
	serviceSpan.End()

	observability.DatabaseOperations.WithLabelValues("delete", "success").Inc()

	// Return 204 No Content
	c.Status(http.StatusNoContent)

	// Log total operation time
	totalDuration := time.Since(startTime)
	h.logger.Debug("DeleteNotificationCategory completed",
		zap.String("category_id", categoryID),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}
