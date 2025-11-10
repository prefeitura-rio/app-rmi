package services

import (
	"context"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

type NotificationCategoryService struct {
	logger *logging.SafeLogger
}

func NewNotificationCategoryService(logger *logging.SafeLogger) *NotificationCategoryService {
	return &NotificationCategoryService{
		logger: logger,
	}
}

// ListActive returns all active notification categories
func (s *NotificationCategoryService) ListActive(ctx context.Context) ([]models.NotificationCategory, error) {
	// Try to get from cache first
	cacheKey := "notification_categories:active"
	var categories []models.NotificationCategory

	// Check cache
	cachedData, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil && cachedData != "" {
		// Parse cached data
		var cached []models.NotificationCategory
		if err := bson.UnmarshalExtJSON([]byte(cachedData), false, &cached); err == nil {
			s.logger.Debug("notification categories cache hit", zap.String("cache_key", cacheKey))
			return cached, nil
		}
		s.logger.Warn("failed to unmarshal cached categories", zap.Error(err))
	}

	// Cache miss - fetch from database
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	filter := bson.M{"active": true}
	opts := options.Find().SetSort(bson.D{{Key: "order", Value: 1}})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		s.logger.Error("failed to list active categories", zap.Error(err))
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &categories); err != nil {
		s.logger.Error("failed to decode categories", zap.Error(err))
		return nil, fmt.Errorf("failed to decode categories: %w", err)
	}

	// Cache the result
	if len(categories) > 0 {
		jsonData, err := bson.MarshalExtJSON(categories, false, false)
		if err == nil {
			config.Redis.Set(ctx, cacheKey, string(jsonData), config.AppConfig.NotificationCategoryCacheTTL)
		}
	}

	return categories, nil
}

// GetByID returns a notification category by ID
func (s *NotificationCategoryService) GetByID(ctx context.Context, id string) (*models.NotificationCategory, error) {
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)

	var category models.NotificationCategory
	err := collection.FindOne(ctx, bson.M{"_id": id}).Decode(&category)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		s.logger.Error("failed to get category", zap.Error(err), zap.String("id", id))
		return nil, fmt.Errorf("failed to get category: %w", err)
	}

	return &category, nil
}

// GetDefaults returns default opt-in values for all active categories
func (s *NotificationCategoryService) GetDefaults(ctx context.Context) (map[string]bool, error) {
	categories, err := s.ListActive(ctx)
	if err != nil {
		return nil, err
	}

	defaults := make(map[string]bool)
	for _, cat := range categories {
		defaults[cat.ID] = cat.DefaultOptIn
	}

	return defaults, nil
}

// Create creates a new notification category (admin only)
func (s *NotificationCategoryService) Create(ctx context.Context, req models.CreateNotificationCategoryRequest) (*models.NotificationCategory, error) {
	// Check if category already exists
	existing, err := s.GetByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("category with ID %s already exists", req.ID)
	}

	category := models.NotificationCategory{
		ID:           req.ID,
		Name:         req.Name,
		Description:  req.Description,
		DefaultOptIn: req.DefaultOptIn,
		Active:       req.Active,
		Order:        req.Order,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	_, err = collection.InsertOne(ctx, category)
	if err != nil {
		s.logger.Error("failed to create category", zap.Error(err), zap.String("id", req.ID))
		return nil, fmt.Errorf("failed to create category: %w", err)
	}

	// Invalidate cache
	s.InvalidateCache(ctx)

	s.logger.Info("created notification category", zap.String("id", category.ID))
	return &category, nil
}

// Update updates a notification category (admin only)
func (s *NotificationCategoryService) Update(ctx context.Context, id string, req models.UpdateNotificationCategoryRequest) (*models.NotificationCategory, error) {
	// Check if category exists
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("category with ID %s not found", id)
	}

	update := bson.M{
		"updated_at": time.Now(),
	}

	if req.Name != nil {
		update["name"] = *req.Name
	}
	if req.Description != nil {
		update["description"] = *req.Description
	}
	if req.DefaultOptIn != nil {
		update["default_opt_in"] = *req.DefaultOptIn
	}
	if req.Active != nil {
		update["active"] = *req.Active
	}
	if req.Order != nil {
		update["order"] = *req.Order
	}

	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": update},
	)
	if err != nil {
		s.logger.Error("failed to update category", zap.Error(err), zap.String("id", id))
		return nil, fmt.Errorf("failed to update category: %w", err)
	}

	// Invalidate cache
	s.InvalidateCache(ctx)

	// Fetch updated category
	updated, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	s.logger.Info("updated notification category", zap.String("id", id))
	return updated, nil
}

// Delete soft-deletes a notification category by setting active=false (admin only)
func (s *NotificationCategoryService) Delete(ctx context.Context, id string) error {
	// Check if category exists
	existing, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("category with ID %s not found", id)
	}

	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	_, err = collection.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"active":     false,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		s.logger.Error("failed to delete category", zap.Error(err), zap.String("id", id))
		return fmt.Errorf("failed to delete category: %w", err)
	}

	// Invalidate cache
	s.InvalidateCache(ctx)

	s.logger.Info("deleted notification category", zap.String("id", id))
	return nil
}

// InvalidateCache invalidates the notification categories cache
func (s *NotificationCategoryService) InvalidateCache(ctx context.Context) {
	cacheKey := "notification_categories:active"
	if err := config.Redis.Del(ctx, cacheKey).Err(); err != nil {
		s.logger.Warn("failed to invalidate categories cache", zap.Error(err))
	}
}

// InitializeCategoryOptIns initializes category opt-ins for a new user with default values
func (s *NotificationCategoryService) InitializeCategoryOptIns(ctx context.Context, globalOptIn bool) (map[string]bool, error) {
	defaults, err := s.GetDefaults(ctx)
	if err != nil {
		return nil, err
	}

	// If global opt-in is false, all categories should be false
	if !globalOptIn {
		categoryOptIns := make(map[string]bool)
		for categoryID := range defaults {
			categoryOptIns[categoryID] = false
		}
		return categoryOptIns, nil
	}

	// Otherwise, use category defaults
	return defaults, nil
}

// ValidateCategoryExists checks if a category exists and is active
func (s *NotificationCategoryService) ValidateCategoryExists(ctx context.Context, categoryID string) error {
	category, err := s.GetByID(ctx, categoryID)
	if err != nil {
		return err
	}
	if category == nil {
		return fmt.Errorf("category %s not found", categoryID)
	}
	if !category.Active {
		return fmt.Errorf("category %s is not active", categoryID)
	}
	return nil
}
