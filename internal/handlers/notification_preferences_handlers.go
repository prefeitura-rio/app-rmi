package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

type NotificationPreferencesHandlers struct {
	categoryService *services.NotificationCategoryService
	logger          *logging.SafeLogger
}

func NewNotificationPreferencesHandlers(logger *logging.SafeLogger) *NotificationPreferencesHandlers {
	return &NotificationPreferencesHandlers{
		categoryService: services.NewNotificationCategoryService(logger),
		logger:          logger,
	}
}

// GetCitizenPreferences godoc
// @Summary Get citizen notification preferences
// @Description Get notification preferences for a citizen including global and category opt-ins
// @Tags notification-preferences
// @Accept json
// @Produce json
// @Param cpf path string true "CPF number"
// @Security BearerAuth
// @Success 200 {object} models.NotificationPreferencesResponse "Notification preferences"
// @Failure 400 {object} ErrorResponse "Invalid CPF format"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Access denied"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /citizen/{cpf}/notification-preferences [get]
func (h *NotificationPreferencesHandlers) GetCitizenPreferences(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetCitizenNotificationPreferences")
	defer span.End()

	cpf := c.Param("cpf")
	logger := h.logger.With(zap.String("cpf", cpf))

	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "get_citizen_notification_preferences"),
		attribute.String("service", "notification_preferences"),
	)

	logger.Debug("GetCitizenNotificationPreferences called")

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
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, h.logger)

	var userConfig models.UserConfig
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)
	if err != nil {
		if err == services.ErrDocumentNotFound {
			// No config exists - return defaults
			utils.AddSpanAttribute(dbSpan, "user_config.found", false)
			dbSpan.End()

			// Initialize with defaults from categories
			ctx, initSpan := utils.TraceBusinessLogic(ctx, "initialize_default_preferences")
			categoryOptIns, err := h.categoryService.InitializeCategoryOptIns(ctx, true) // Default global opt-in = true
			if err != nil {
				utils.RecordErrorInSpan(initSpan, err, map[string]interface{}{
					"operation": "initialize_category_opt_ins",
				})
				initSpan.End()
				logger.Error("failed to initialize category opt-ins", zap.Error(err))
				c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get preferences"})
				return
			}
			initSpan.End()

			response := models.NotificationPreferencesResponse{
				CPF:            cpf,
				OptIn:          true,
				CategoryOptIns: categoryOptIns,
				UpdatedAt:      time.Now(),
			}

			logger.Debug("GetCitizenNotificationPreferences completed (defaults)",
				zap.String("cpf", cpf),
				zap.Bool("opt_in", true),
				zap.Duration("total_duration", time.Since(startTime)),
				zap.String("status", "success"))

			c.JSON(http.StatusOK, response)
			return
		}

		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"operation": "dataManager.Read",
			"cpf":       cpf,
			"type":      "user_config",
		})
		dbSpan.End()
		logger.Error("failed to get user config via DataManager", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get preferences"})
		return
	}
	utils.AddSpanAttribute(dbSpan, "user_config.found", true)
	dbSpan.End()

	// If no category opt-ins exist, initialize with current global opt-in value
	if len(userConfig.CategoryOptIns) == 0 {
		ctx, initSpan := utils.TraceBusinessLogic(ctx, "initialize_category_opt_ins")
		categoryOptIns, err := h.categoryService.InitializeCategoryOptIns(ctx, userConfig.OptIn)
		if err != nil {
			utils.RecordErrorInSpan(initSpan, err, map[string]interface{}{
				"operation": "initialize_category_opt_ins",
			})
			initSpan.End()
			logger.Error("failed to initialize category opt-ins", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get preferences"})
			return
		}
		userConfig.CategoryOptIns = categoryOptIns
		userConfig.UpdatedAt = time.Now()

		// Persist the initialization to avoid re-initialization on every read
		cacheService := services.NewCacheService()
		if err := cacheService.UpdateUserConfig(ctx, cpf, &userConfig); err != nil {
			logger.Warn("failed to persist initialized category opt-ins", zap.String("cpf", cpf), zap.Error(err))
		} else {
			// Invalidate cache to ensure fresh read
			cacheKey := fmt.Sprintf("user_config:%s", cpf)
			if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
				logger.Warn("failed to invalidate cache after initialization", zap.String("cache_key", cacheKey), zap.Error(err))
			}
		}
		initSpan.End()
	}

	response := models.NotificationPreferencesResponse{
		CPF:            cpf,
		OptIn:          userConfig.OptIn,
		CategoryOptIns: userConfig.CategoryOptIns,
		UpdatedAt:      userConfig.UpdatedAt,
	}

	c.JSON(http.StatusOK, response)

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("GetCitizenNotificationPreferences completed",
		zap.String("cpf", cpf),
		zap.Bool("opt_in", userConfig.OptIn),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// UpdateCitizenPreferences godoc
// @Summary Update citizen notification preferences
// @Description Update notification preferences for a citizen (global and/or category opt-ins)
// @Tags notification-preferences
// @Accept json
// @Produce json
// @Param cpf path string true "CPF number"
// @Param data body models.UpdateNotificationPreferencesRequest true "Updated preferences"
// @Security BearerAuth
// @Success 200 {object} models.NotificationPreferencesResponse "Preferences updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid CPF format or request body"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Access denied"
// @Failure 422 {object} ErrorResponse "Invalid category ID"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /citizen/{cpf}/notification-preferences [put]
func (h *NotificationPreferencesHandlers) UpdateCitizenPreferences(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateCitizenNotificationPreferences")
	defer span.End()

	cpf := c.Param("cpf")
	logger := h.logger.With(zap.String("cpf", cpf))

	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("operation", "update_citizen_notification_preferences"),
		attribute.String("service", "notification_preferences"),
	)

	logger.Debug("UpdateCitizenNotificationPreferences called")

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
	ctx, inputSpan := utils.TraceInputParsing(ctx, "update_preferences_request")
	var input models.UpdateNotificationPreferencesRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.RecordErrorInSpan(inputSpan, err, map[string]interface{}{
			"error.type": "input_parsing",
			"input.type": "UpdateNotificationPreferencesRequest",
		})
		inputSpan.End()
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}
	if input.OptIn != nil {
		utils.AddSpanAttribute(inputSpan, "input.opt_in", *input.OptIn)
	}
	utils.AddSpanAttribute(inputSpan, "input.channel", input.Channel)
	if input.Reason != nil {
		utils.AddSpanAttribute(inputSpan, "input.reason", *input.Reason)
	}
	inputSpan.End()

	// Validate category IDs exist and are active
	if len(input.CategoryOptIns) > 0 {
		ctx, validateSpan := utils.TraceBusinessLogic(ctx, "validate_categories")
		for categoryID := range input.CategoryOptIns {
			if err := h.categoryService.ValidateCategoryExists(ctx, categoryID); err != nil {
				utils.RecordErrorInSpan(validateSpan, err, map[string]interface{}{
					"category_id": categoryID,
				})
				validateSpan.End()
				logger.Warn("invalid category ID", zap.String("category_id", categoryID), zap.Error(err))
				c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: fmt.Sprintf("Invalid category: %s", categoryID)})
				return
			}
		}
		validateSpan.End()
	}

	// Get current user config
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.UserConfigCollection, "cpf")
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, h.logger)

	var userConfig models.UserConfig
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)
	configExists := err == nil
	if err != nil && err != services.ErrDocumentNotFound {
		utils.RecordErrorInSpan(dbSpan, err, map[string]interface{}{
			"operation": "dataManager.Read",
			"cpf":       cpf,
		})
		dbSpan.End()
		logger.Error("failed to get user config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update preferences"})
		return
	}
	dbSpan.End()

	// Initialize if doesn't exist
	if !configExists {
		ctx, initSpan := utils.TraceBusinessLogic(ctx, "initialize_user_config")
		defaultCategoryOptIns, err := h.categoryService.InitializeCategoryOptIns(ctx, true)
		if err != nil {
			utils.RecordErrorInSpan(initSpan, err, map[string]interface{}{})
			initSpan.End()
			logger.Error("failed to initialize category opt-ins", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update preferences"})
			return
		}
		userConfig = models.UserConfig{
			CPF:            cpf,
			OptIn:          true,
			CategoryOptIns: defaultCategoryOptIns,
			FirstLogin:     false,
			UpdatedAt:      time.Now(),
		}
		initSpan.End()
	}

	// Track old values for audit
	oldGlobalOptIn := userConfig.OptIn
	oldCategoryOptIns := make(map[string]bool)
	if userConfig.CategoryOptIns != nil {
		for k, v := range userConfig.CategoryOptIns {
			oldCategoryOptIns[k] = v
		}
	}

	// Update global opt-in if provided
	if input.OptIn != nil {
		userConfig.OptIn = *input.OptIn
	}

	// Update category opt-ins if provided
	if len(input.CategoryOptIns) > 0 {
		if userConfig.CategoryOptIns == nil {
			userConfig.CategoryOptIns = make(map[string]bool)
		}
		for categoryID, opted := range input.CategoryOptIns {
			userConfig.CategoryOptIns[categoryID] = opted
		}
	}

	userConfig.UpdatedAt = time.Now()

	// Update via cache service with tracing
	ctx, updateSpan := utils.TraceBusinessLogic(ctx, "update_user_config_via_cache")
	cacheService := services.NewCacheService()
	err = cacheService.UpdateUserConfig(ctx, cpf, &userConfig)
	if err != nil {
		utils.RecordErrorInSpan(updateSpan, err, map[string]interface{}{
			"cache.operation": "update_user_config",
		})
		updateSpan.End()
		logger.Error("failed to update preferences via cache service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update preferences"})
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

	// Log opt-in history for global preference change
	if input.OptIn != nil && *input.OptIn != oldGlobalOptIn {
		h.recordOptInHistory(ctx, cpf, oldGlobalOptIn, *input.OptIn, models.OptInScopeGlobal, nil, input.Channel, input.Reason)
	}

	// Log opt-in history for category preference changes
	for categoryID, newValue := range input.CategoryOptIns {
		oldValue, existed := oldCategoryOptIns[categoryID]
		if !existed || oldValue != newValue {
			h.recordOptInHistory(ctx, cpf, oldValue, newValue, models.OptInScopeCategory, &categoryID, input.Channel, input.Reason)
		}
	}

	response := models.NotificationPreferencesResponse{
		CPF:            cpf,
		OptIn:          userConfig.OptIn,
		CategoryOptIns: userConfig.CategoryOptIns,
		UpdatedAt:      userConfig.UpdatedAt,
	}

	c.JSON(http.StatusOK, response)

	// Log total operation time
	totalDuration := time.Since(startTime)
	logger.Debug("UpdateCitizenNotificationPreferences completed",
		zap.String("cpf", cpf),
		zap.Bool("old_global_opt_in", oldGlobalOptIn),
		zap.Bool("new_global_opt_in", userConfig.OptIn),
		zap.Duration("total_duration", totalDuration),
		zap.String("status", "success"))
}

// UpdateCitizenCategoryPreference godoc
// @Summary Update single category preference
// @Description Update opt-in preference for a single notification category
// @Tags notification-preferences
// @Accept json
// @Produce json
// @Param cpf path string true "CPF number"
// @Param category_id path string true "Category ID"
// @Param data body models.UpdateCategoryPreferenceRequest true "Category preference"
// @Security BearerAuth
// @Success 200 {object} models.NotificationPreferencesResponse "Preference updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid CPF format or request body"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Access denied"
// @Failure 422 {object} ErrorResponse "Invalid category ID"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /citizen/{cpf}/notification-preferences/categories/{category_id} [patch]
func (h *NotificationPreferencesHandlers) UpdateCitizenCategoryPreference(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdateCitizenCategoryPreference")
	defer span.End()

	cpf := c.Param("cpf")
	categoryID := c.Param("category_id")
	logger := h.logger.With(zap.String("cpf", cpf), zap.String("category_id", categoryID))

	span.SetAttributes(
		attribute.String("cpf", cpf),
		attribute.String("category_id", categoryID),
		attribute.String("operation", "update_citizen_category_preference"),
		attribute.String("service", "notification_preferences"),
	)

	logger.Debug("UpdateCitizenCategoryPreference called")

	// Validate CPF
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	// Validate category exists
	if err := h.categoryService.ValidateCategoryExists(ctx, categoryID); err != nil {
		logger.Warn("invalid category ID", zap.Error(err))
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: fmt.Sprintf("Invalid category: %s", categoryID)})
		return
	}

	// Parse input
	var input models.UpdateCategoryPreferenceRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Convert to full update request with single category
	fullUpdate := models.UpdateNotificationPreferencesRequest{
		CategoryOptIns: map[string]bool{categoryID: input.OptIn},
	}

	// Reuse the full update logic by internally calling UpdateCitizenPreferences
	c.Params = append(c.Params, gin.Param{Key: "cpf", Value: cpf})
	c.Set("update_request", fullUpdate)

	// For simplicity, just update the single category
	dataManager := services.NewDataManager(config.Redis, config.MongoDB, h.logger)
	var userConfig models.UserConfig
	err := dataManager.Read(ctx, cpf, config.AppConfig.UserConfigCollection, "user_config", &userConfig)
	configExists := err == nil

	if !configExists {
		defaultCategoryOptIns, err := h.categoryService.InitializeCategoryOptIns(ctx, true)
		if err != nil {
			logger.Error("failed to initialize category opt-ins", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update preference"})
			return
		}
		userConfig = models.UserConfig{
			CPF:            cpf,
			OptIn:          true,
			CategoryOptIns: defaultCategoryOptIns,
			FirstLogin:     false,
			UpdatedAt:      time.Now(),
		}
	}

	if userConfig.CategoryOptIns == nil {
		userConfig.CategoryOptIns = make(map[string]bool)
	}

	oldValue := userConfig.CategoryOptIns[categoryID]
	userConfig.CategoryOptIns[categoryID] = input.OptIn
	userConfig.UpdatedAt = time.Now()

	cacheService := services.NewCacheService()
	err = cacheService.UpdateUserConfig(ctx, cpf, &userConfig)
	if err != nil {
		logger.Error("failed to update preference", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update preference"})
		return
	}

	// Invalidate cache
	cacheKey := fmt.Sprintf("user_config:%s", cpf)
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		logger.Warn("failed to invalidate cache", zap.String("cache_key", cacheKey), zap.Error(err))
	}

	// Log opt-in history for category preference change
	if oldValue != input.OptIn {
		h.recordOptInHistory(ctx, cpf, oldValue, input.OptIn, models.OptInScopeCategory, &categoryID, input.Channel, input.Reason)
	}

	response := models.NotificationPreferencesResponse{
		CPF:            cpf,
		OptIn:          userConfig.OptIn,
		CategoryOptIns: userConfig.CategoryOptIns,
		UpdatedAt:      userConfig.UpdatedAt,
	}

	c.JSON(http.StatusOK, response)

	logger.Debug("UpdateCitizenCategoryPreference completed",
		zap.Bool("old_value", oldValue),
		zap.Bool("new_value", input.OptIn),
		zap.Duration("total_duration", time.Since(startTime)))
}

// GetPhonePreferences godoc
// @Summary Get phone notification preferences (admin only)
// @Description Get notification preferences for a phone number including global and category opt-ins (synced with CPF). Admin only.
// @Tags notification-preferences
// @Accept json
// @Produce json
// @Param phone_number path string true "Phone number"
// @Security BearerAuth
// @Success 200 {object} models.PhoneNotificationPreferencesResponse "Notification preferences"
// @Failure 400 {object} ErrorResponse "Invalid phone format"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Admin access required"
// @Failure 404 {object} ErrorResponse "Phone not found"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /phone/{phone_number}/notification-preferences [get]
func (h *NotificationPreferencesHandlers) GetPhonePreferences(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetPhoneNotificationPreferences")
	defer span.End()

	phoneNumber := c.Param("phone_number")
	logger := h.logger.With(zap.String("phone_number", phoneNumber))

	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "get_phone_notification_preferences"),
		attribute.String("service", "notification_preferences"),
	)

	logger.Debug("GetPhoneNotificationPreferences called")

	// Validate phone number
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Phone number is required"})
		return
	}

	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		logger.Error("failed to parse phone number", zap.String("phone_number", phoneNumber), zap.Error(err))
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
		return
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)
	logger.Debug("parsed phone components",
		zap.String("input_phone", phoneNumber),
		zap.String("ddi", components.DDI),
		zap.String("ddd", components.DDD),
		zap.String("valor", components.Valor),
		zap.String("storage_phone", storagePhone))

	// Get phone mapping from database
	collection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	var mapping models.PhoneCPFMapping
	err = collection.FindOne(ctx, map[string]interface{}{"phone_number": storagePhone}).Decode(&mapping)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			logger.Warn("phone mapping not found", zap.String("phone_number", phoneNumber))
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Phone number not found"})
		} else {
			logger.Error("failed to get phone mapping", zap.String("phone_number", phoneNumber), zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get preferences"})
		}
		return
	}

	// If no category opt-ins exist, initialize with current opt-in value
	if len(mapping.CategoryOptIns) == 0 {
		categoryOptIns, err := h.categoryService.InitializeCategoryOptIns(ctx, mapping.OptIn)
		if err != nil {
			logger.Error("failed to initialize category opt-ins", zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get preferences"})
			return
		}
		mapping.CategoryOptIns = categoryOptIns
		now := time.Now()
		mapping.UpdatedAt = &now

		// Persist the initialization to avoid re-initialization on every read
		update := map[string]interface{}{
			"category_opt_ins": mapping.CategoryOptIns,
			"updated_at":       mapping.UpdatedAt,
		}
		if _, err := collection.UpdateOne(ctx, map[string]interface{}{"phone_number": storagePhone}, map[string]interface{}{"$set": update}); err != nil {
			logger.Warn("failed to persist initialized category opt-ins for phone", zap.String("phone_number", phoneNumber), zap.Error(err))
		}
	}

	response := models.PhoneNotificationPreferencesResponse{
		PhoneNumber:    phoneNumber,
		OptIn:          mapping.OptIn,
		CategoryOptIns: mapping.CategoryOptIns,
		UpdatedAt:      time.Now(),
	}
	if mapping.UpdatedAt != nil {
		response.UpdatedAt = *mapping.UpdatedAt
	}

	c.JSON(http.StatusOK, response)

	logger.Debug("GetPhoneNotificationPreferences completed",
		zap.Bool("opt_in", mapping.OptIn),
		zap.Duration("total_duration", time.Since(startTime)))
}

// UpdatePhonePreferences godoc
// @Summary Update phone notification preferences (admin only)
// @Description Update notification preferences for a phone number (synced with CPF preferences). Admin only.
// @Tags notification-preferences
// @Accept json
// @Produce json
// @Param phone_number path string true "Phone number"
// @Param data body models.UpdateNotificationPreferencesRequest true "Updated preferences"
// @Security BearerAuth
// @Success 200 {object} models.PhoneNotificationPreferencesResponse "Preferences updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid phone format or request body"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Admin access required"
// @Failure 404 {object} ErrorResponse "Phone not found"
// @Failure 422 {object} ErrorResponse "Invalid category ID"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /phone/{phone_number}/notification-preferences [put]
func (h *NotificationPreferencesHandlers) UpdatePhonePreferences(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdatePhoneNotificationPreferences")
	defer span.End()

	phoneNumber := c.Param("phone_number")
	logger := h.logger.With(zap.String("phone_number", phoneNumber))

	span.SetAttributes(
		attribute.String("phone_number", phoneNumber),
		attribute.String("operation", "update_phone_notification_preferences"),
		attribute.String("service", "notification_preferences"),
	)

	logger.Debug("UpdatePhoneNotificationPreferences called")

	// Validate phone number
	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Phone number is required"})
		return
	}

	// Parse phone number for storage format
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
		return
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	// Parse input
	var input models.UpdateNotificationPreferencesRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Validate category IDs exist and are active
	if len(input.CategoryOptIns) > 0 {
		for categoryID := range input.CategoryOptIns {
			if err := h.categoryService.ValidateCategoryExists(ctx, categoryID); err != nil {
				logger.Warn("invalid category ID", zap.String("category_id", categoryID), zap.Error(err))
				c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: fmt.Sprintf("Invalid category: %s", categoryID)})
				return
			}
		}
	}

	// Get current phone mapping
	collection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	var mapping models.PhoneCPFMapping
	err = collection.FindOne(ctx, map[string]interface{}{"phone_number": storagePhone}).Decode(&mapping)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			logger.Warn("phone mapping not found", zap.String("phone_number", phoneNumber))
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Phone number not found"})
		} else {
			logger.Error("failed to get phone mapping", zap.String("phone_number", phoneNumber), zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get preferences"})
		}
		return
	}

	// Track old values for audit
	oldGlobalOptIn := mapping.OptIn
	oldCategoryOptIns := make(map[string]bool)
	if mapping.CategoryOptIns != nil {
		for k, v := range mapping.CategoryOptIns {
			oldCategoryOptIns[k] = v
		}
	}

	// Update global opt-in if provided
	if input.OptIn != nil {
		mapping.OptIn = *input.OptIn
	}

	// Update category opt-ins if provided
	if len(input.CategoryOptIns) > 0 {
		if mapping.CategoryOptIns == nil {
			mapping.CategoryOptIns = make(map[string]bool)
		}
		for categoryID, opted := range input.CategoryOptIns {
			mapping.CategoryOptIns[categoryID] = opted
		}
	}

	now := time.Now()
	mapping.UpdatedAt = &now

	// Update phone mapping in database
	update := map[string]interface{}{
		"opt_in":           mapping.OptIn,
		"category_opt_ins": mapping.CategoryOptIns,
		"updated_at":       mapping.UpdatedAt,
	}
	_, err = collection.UpdateOne(ctx, map[string]interface{}{"phone_number": storagePhone}, map[string]interface{}{"$set": update})
	if err != nil {
		logger.Error("failed to update phone preferences", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update preferences"})
		return
	}

	// SYNC: Update CPF preferences if CPF is mapped
	if mapping.CPF != "" {
		logger.Debug("syncing preferences to CPF", zap.String("cpf", mapping.CPF))
		dataManager := services.NewDataManager(config.Redis, config.MongoDB, h.logger)
		var userConfig models.UserConfig
		err = dataManager.Read(ctx, mapping.CPF, config.AppConfig.UserConfigCollection, "user_config", &userConfig)
		configExists := err == nil

		if !configExists {
			defaultCategoryOptIns, err := h.categoryService.InitializeCategoryOptIns(ctx, mapping.OptIn)
			if err != nil {
				logger.Warn("failed to initialize category opt-ins for CPF sync", zap.Error(err))
			} else {
				userConfig = models.UserConfig{
					CPF:            mapping.CPF,
					OptIn:          mapping.OptIn,
					CategoryOptIns: defaultCategoryOptIns,
					FirstLogin:     false,
					UpdatedAt:      now,
				}
			}
		} else {
			// Sync values
			if input.OptIn != nil {
				userConfig.OptIn = *input.OptIn
			}
			if input.CategoryOptIns != nil {
				if userConfig.CategoryOptIns == nil {
					userConfig.CategoryOptIns = make(map[string]bool)
				}
				for categoryID, opted := range input.CategoryOptIns {
					userConfig.CategoryOptIns[categoryID] = opted
				}
			}
			userConfig.UpdatedAt = now
		}

		// Update CPF config
		cacheService := services.NewCacheService()
		if err := cacheService.UpdateUserConfig(ctx, mapping.CPF, &userConfig); err != nil {
			logger.Warn("failed to sync preferences to CPF", zap.String("cpf", mapping.CPF), zap.Error(err))
		} else {
			// Invalidate CPF cache
			cacheKey := fmt.Sprintf("user_config:%s", mapping.CPF)
			if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
				logger.Warn("failed to invalidate CPF cache", zap.String("cache_key", cacheKey), zap.Error(err))
			}
			logger.Debug("synced preferences to CPF successfully", zap.String("cpf", mapping.CPF))
		}
	}

	observability.DatabaseOperations.WithLabelValues("update", "success").Inc()

	// Log opt-in history for global preference change (if CPF is mapped)
	if mapping.CPF != "" {
		if input.OptIn != nil && *input.OptIn != oldGlobalOptIn {
			h.recordOptInHistoryWithPhone(ctx, storagePhone, mapping.CPF, oldGlobalOptIn, *input.OptIn, models.OptInScopeGlobal, nil, input.Channel, input.Reason)
		}

		// Log opt-in history for category preference changes
		for categoryID, newValue := range input.CategoryOptIns {
			oldValue, existed := oldCategoryOptIns[categoryID]
			if !existed || oldValue != newValue {
				h.recordOptInHistoryWithPhone(ctx, storagePhone, mapping.CPF, oldValue, newValue, models.OptInScopeCategory, &categoryID, input.Channel, input.Reason)
			}
		}
	}

	response := models.PhoneNotificationPreferencesResponse{
		PhoneNumber:    phoneNumber,
		OptIn:          mapping.OptIn,
		CategoryOptIns: mapping.CategoryOptIns,
		UpdatedAt:      now,
	}

	c.JSON(http.StatusOK, response)

	logger.Debug("UpdatePhoneNotificationPreferences completed",
		zap.Bool("old_global_opt_in", oldGlobalOptIn),
		zap.Bool("new_global_opt_in", mapping.OptIn),
		zap.Duration("total_duration", time.Since(startTime)))
}

// UpdatePhoneCategoryPreference godoc
// @Summary Update single category preference for phone (admin only)
// @Description Update opt-in preference for a single notification category for a phone number. Admin only.
// @Tags notification-preferences
// @Accept json
// @Produce json
// @Param phone_number path string true "Phone number"
// @Param category_id path string true "Category ID"
// @Param data body models.UpdateCategoryPreferenceRequest true "Category preference"
// @Security BearerAuth
// @Success 200 {object} models.PhoneNotificationPreferencesResponse "Preference updated successfully"
// @Failure 400 {object} ErrorResponse "Invalid phone format or request body"
// @Failure 401 {object} ErrorResponse "Unauthorized"
// @Failure 403 {object} ErrorResponse "Admin access required"
// @Failure 404 {object} ErrorResponse "Phone not found"
// @Failure 422 {object} ErrorResponse "Invalid category ID"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /phone/{phone_number}/notification-preferences/categories/{category_id} [patch]
func (h *NotificationPreferencesHandlers) UpdatePhoneCategoryPreference(c *gin.Context) {
	startTime := time.Now()
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "UpdatePhoneCategoryPreference")
	defer span.End()

	phoneNumber := c.Param("phone_number")
	categoryID := c.Param("category_id")
	logger := h.logger.With(zap.String("phone_number", phoneNumber), zap.String("category_id", categoryID))

	logger.Debug("UpdatePhoneCategoryPreference called")

	// Validate category exists
	if err := h.categoryService.ValidateCategoryExists(ctx, categoryID); err != nil {
		logger.Warn("invalid category ID", zap.Error(err))
		c.JSON(http.StatusUnprocessableEntity, ErrorResponse{Error: fmt.Sprintf("Invalid category: %s", categoryID)})
		return
	}

	// Parse input
	var input models.UpdateCategoryPreferenceRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request body: " + err.Error()})
		return
	}

	// Reuse full update with single category
	fullUpdate := models.UpdateNotificationPreferencesRequest{
		CategoryOptIns: map[string]bool{categoryID: input.OptIn},
	}

	// Parse phone and update
	components, err := utils.ParsePhoneNumber(phoneNumber)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid phone number format"})
		return
	}
	storagePhone := utils.FormatPhoneForStorage(components.DDI, components.DDD, components.Valor)

	collection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	var mapping models.PhoneCPFMapping
	err = collection.FindOne(ctx, map[string]interface{}{"phone_number": storagePhone}).Decode(&mapping)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			logger.Warn("phone mapping not found", zap.String("phone_number", phoneNumber))
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Phone number not found"})
		} else {
			logger.Error("failed to get phone mapping", zap.String("phone_number", phoneNumber), zap.Error(err))
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to get preferences"})
		}
		return
	}

	if mapping.CategoryOptIns == nil {
		mapping.CategoryOptIns = make(map[string]bool)
	}

	oldValue := mapping.CategoryOptIns[categoryID]
	mapping.CategoryOptIns[categoryID] = input.OptIn
	now := time.Now()
	mapping.UpdatedAt = &now

	update := map[string]interface{}{
		"category_opt_ins": mapping.CategoryOptIns,
		"updated_at":       mapping.UpdatedAt,
	}
	_, err = collection.UpdateOne(ctx, map[string]interface{}{"phone_number": storagePhone}, map[string]interface{}{"$set": update})
	if err != nil {
		logger.Error("failed to update preference", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update preference"})
		return
	}

	// Sync to CPF if mapped
	if mapping.CPF != "" {
		c.Set("update_request", fullUpdate)
		logger.Debug("syncing category preference to CPF", zap.String("cpf", mapping.CPF))

		dataManager := services.NewDataManager(config.Redis, config.MongoDB, h.logger)
		var userConfig models.UserConfig
		err = dataManager.Read(ctx, mapping.CPF, config.AppConfig.UserConfigCollection, "user_config", &userConfig)
		configExists := err == nil

		// Initialize user config if it doesn't exist
		if !configExists {
			defaultCategoryOptIns, err := h.categoryService.InitializeCategoryOptIns(ctx, mapping.OptIn)
			if err != nil {
				logger.Warn("failed to initialize category opt-ins for CPF sync", zap.String("cpf", mapping.CPF), zap.Error(err))
			} else {
				userConfig = models.UserConfig{
					CPF:            mapping.CPF,
					OptIn:          mapping.OptIn,
					CategoryOptIns: defaultCategoryOptIns,
					FirstLogin:     false,
					UpdatedAt:      now,
				}
			}
		}

		// Update the specific category
		if userConfig.CategoryOptIns == nil {
			userConfig.CategoryOptIns = make(map[string]bool)
		}
		userConfig.CategoryOptIns[categoryID] = input.OptIn
		userConfig.UpdatedAt = now

		// Persist the sync
		cacheService := services.NewCacheService()
		if err := cacheService.UpdateUserConfig(ctx, mapping.CPF, &userConfig); err != nil {
			logger.Warn("failed to sync category to CPF", zap.String("cpf", mapping.CPF), zap.Error(err))
		} else {
			if err := config.Redis.Del(ctx, fmt.Sprintf("user_config:%s", mapping.CPF)).Err(); err != nil {
				logger.Warn("failed to invalidate CPF cache", zap.String("cpf", mapping.CPF), zap.Error(err))
			}
		}

		// Log opt-in history for category preference change
		if oldValue != input.OptIn {
			h.recordOptInHistoryWithPhone(ctx, storagePhone, mapping.CPF, oldValue, input.OptIn, models.OptInScopeCategory, &categoryID, input.Channel, input.Reason)
		}
	}

	response := models.PhoneNotificationPreferencesResponse{
		PhoneNumber:    phoneNumber,
		OptIn:          mapping.OptIn,
		CategoryOptIns: mapping.CategoryOptIns,
		UpdatedAt:      now,
	}

	c.JSON(http.StatusOK, response)

	logger.Debug("UpdatePhoneCategoryPreference completed",
		zap.Bool("old_value", oldValue),
		zap.Bool("new_value", input.OptIn),
		zap.Duration("total_duration", time.Since(startTime)))
}

// recordOptInHistory records opt-in/opt-out history for notification preferences
func (h *NotificationPreferencesHandlers) recordOptInHistory(ctx context.Context, cpf string, oldValue, newValue bool, scope string, categoryID *string, channel string, reason *string) {
	h.recordOptInHistoryWithPhone(ctx, "", cpf, oldValue, newValue, scope, categoryID, channel, reason)
}

// recordOptInHistoryWithPhone records opt-in/opt-out history including phone number
func (h *NotificationPreferencesHandlers) recordOptInHistoryWithPhone(ctx context.Context, phoneNumber, cpf string, oldValue, newValue bool, scope string, categoryID *string, channel string, reason *string) {
	now := time.Now()

	// Determine action based on scope and values
	var action string
	if scope == models.OptInScopeGlobal {
		if newValue {
			action = models.OptInActionOptIn
		} else {
			action = models.OptInActionOptOut
		}
	} else {
		action = models.OptInActionCategoryUpdate
	}

	history := models.OptInHistory{
		CPF:       cpf,
		Action:    action,
		Scope:     scope,
		Category:  categoryID,
		Channel:   channel,
		Reason:    reason,
		OldValue:  &oldValue,
		NewValue:  &newValue,
		Timestamp: now,
	}

	// Add phone number if provided
	if phoneNumber != "" {
		history.PhoneNumber = phoneNumber
	}

	_, err := config.MongoDB.Collection(config.AppConfig.OptInHistoryCollection).InsertOne(ctx, history)
	if err != nil {
		h.logger.Error("failed to record opt-in history",
			zap.Error(err),
			zap.String("cpf", cpf),
			zap.String("scope", scope),
			zap.String("action", action))
		// Don't fail the main operation for this error
	}
}
