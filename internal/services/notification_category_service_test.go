package services

import (
	"context"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

func setupNotificationCategoryServiceTest(t *testing.T) (*NotificationCategoryService, func()) {
	// Use the shared MongoDB and Redis from common_test.go TestMain
	// Don't create new connections - use the global ones
	logging.InitLogger()

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.NotificationCategoryCollection = "test_notification_categories"
	config.AppConfig.NotificationCategoryCacheTTL = 5 * time.Minute

	ctx := context.Background()
	database := config.MongoDB

	service := NewNotificationCategoryService(logging.Logger)

	return service, func() {
		// Clean up Redis
		patterns := []string{"notification_categories:*"}
		for _, pattern := range patterns {
			keys, _ := config.Redis.Keys(ctx, pattern).Result()
			if len(keys) > 0 {
				config.Redis.Del(ctx, keys...)
			}
		}

		// Drop only the test collection, not the entire database
		database.Collection(config.AppConfig.NotificationCategoryCollection).Drop(ctx)
		// DO NOT disconnect the client - it's shared across all tests
	}
}

func TestNewNotificationCategoryService(t *testing.T) {
	service := NewNotificationCategoryService(logging.Logger)
	if service == nil {
		t.Error("NewNotificationCategoryService() returned nil")
	}
}

func TestListActive_Empty(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	categories, err := service.ListActive(ctx)
	if err != nil {
		t.Errorf("ListActive() error = %v", err)
	}

	if len(categories) != 0 {
		t.Errorf("ListActive() len(categories) = %v, want 0", len(categories))
	}
}

func TestListActive_WithData(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test categories
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	categories := []interface{}{
		bson.M{
			"_id":            "health",
			"name":           "Health",
			"description":    "Health notifications",
			"default_opt_in": true,
			"active":         true,
			"order":          1,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
		bson.M{
			"_id":            "education",
			"name":           "Education",
			"description":    "Education notifications",
			"default_opt_in": true,
			"active":         true,
			"order":          2,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
		bson.M{
			"_id":            "inactive",
			"name":           "Inactive",
			"description":    "Inactive category",
			"default_opt_in": false,
			"active":         false,
			"order":          3,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
	}

	_, err := collection.InsertMany(ctx, categories)
	if err != nil {
		t.Fatalf("Failed to insert categories: %v", err)
	}

	result, err := service.ListActive(ctx)
	if err != nil {
		t.Errorf("ListActive() error = %v", err)
	}

	// Should only return active categories
	if len(result) != 2 {
		t.Errorf("ListActive() len(categories) = %v, want 2", len(result))
	}

	// Verify they are sorted by order
	if len(result) >= 2 && result[0].Order > result[1].Order {
		t.Error("ListActive() categories not sorted by order")
	}
}

func TestListActive_MultipleCalls(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "health",
		"name":           "Health",
		"description":    "Health notifications",
		"default_opt_in": true,
		"active":         true,
		"order":          1,
		"created_at":     time.Now(),
		"updated_at":     time.Now(),
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	// First call
	firstResult, err := service.ListActive(ctx)
	if err != nil {
		t.Errorf("ListActive() first call error = %v", err)
	}

	if len(firstResult) != 1 {
		t.Fatalf("ListActive() first call len(categories) = %v, want 1", len(firstResult))
	}

	// Second call should also work correctly
	secondResult, err := service.ListActive(ctx)
	if err != nil {
		t.Errorf("ListActive() second call error = %v", err)
	}

	if len(secondResult) != 1 {
		t.Errorf("ListActive() second call len(categories) = %v, want 1", len(secondResult))
	}
}

func TestGetByID_Success(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "health",
		"name":           "Health",
		"description":    "Health notifications",
		"default_opt_in": true,
		"active":         true,
		"order":          1,
		"created_at":     time.Now(),
		"updated_at":     time.Now(),
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	result, err := service.GetByID(ctx, "health")
	if err != nil {
		t.Errorf("GetByID() error = %v", err)
	}

	if result == nil {
		t.Fatal("GetByID() returned nil")
	}

	if result.ID != "health" {
		t.Errorf("GetByID() ID = %v, want health", result.ID)
	}

	if result.Name != "Health" {
		t.Errorf("GetByID() Name = %v, want Health", result.Name)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	result, err := service.GetByID(ctx, "nonexistent")
	if err != nil {
		t.Errorf("GetByID() should not return error for non-existent category, got %v", err)
	}

	if result != nil {
		t.Error("GetByID() should return nil for non-existent category")
	}
}

func TestGetDefaults(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test categories with different defaults
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	categories := []interface{}{
		bson.M{
			"_id":            "health",
			"name":           "Health",
			"default_opt_in": true,
			"active":         true,
			"order":          1,
		},
		bson.M{
			"_id":            "marketing",
			"name":           "Marketing",
			"default_opt_in": false,
			"active":         true,
			"order":          2,
		},
	}

	_, err := collection.InsertMany(ctx, categories)
	if err != nil {
		t.Fatalf("Failed to insert categories: %v", err)
	}

	defaults, err := service.GetDefaults(ctx)
	if err != nil {
		t.Errorf("GetDefaults() error = %v", err)
	}

	if len(defaults) != 2 {
		t.Errorf("GetDefaults() len(defaults) = %v, want 2", len(defaults))
	}

	if defaults["health"] != true {
		t.Errorf("GetDefaults() health = %v, want true", defaults["health"])
	}

	if defaults["marketing"] != false {
		t.Errorf("GetDefaults() marketing = %v, want false", defaults["marketing"])
	}
}

func TestCreate_Success(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	req := models.CreateNotificationCategoryRequest{
		ID:           "new_category",
		Name:         "New Category",
		Description:  "Test description",
		DefaultOptIn: true,
		Active:       true,
		Order:        1,
	}

	result, err := service.Create(ctx, req)
	if err != nil {
		t.Errorf("Create() error = %v", err)
	}

	if result == nil {
		t.Fatal("Create() returned nil")
	}

	if result.ID != "new_category" {
		t.Errorf("Create() ID = %v, want new_category", result.ID)
	}

	if result.Name != "New Category" {
		t.Errorf("Create() Name = %v, want New Category", result.Name)
	}
}

func TestCreate_AlreadyExists(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert existing category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":    "existing",
		"name":   "Existing",
		"active": true,
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	req := models.CreateNotificationCategoryRequest{
		ID:   "existing",
		Name: "Duplicate",
	}

	_, err = service.Create(ctx, req)
	if err == nil {
		t.Error("Create() should return error for duplicate ID")
	}
}

func TestUpdate_Success(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "health",
		"name":           "Health",
		"description":    "Old description",
		"default_opt_in": true,
		"active":         true,
		"order":          1,
		"created_at":     time.Now(),
		"updated_at":     time.Now(),
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	newName := "Updated Health"
	newDesc := "New description"
	req := models.UpdateNotificationCategoryRequest{
		Name:        &newName,
		Description: &newDesc,
	}

	result, err := service.Update(ctx, "health", req)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	if result == nil {
		t.Fatal("Update() returned nil")
	}

	if result.Name != "Updated Health" {
		t.Errorf("Update() Name = %v, want Updated Health", result.Name)
	}

	if result.Description != "New description" {
		t.Errorf("Update() Description = %v, want New description", result.Description)
	}
}

func TestUpdate_NotFound(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	newName := "Updated"
	req := models.UpdateNotificationCategoryRequest{
		Name: &newName,
	}

	_, err := service.Update(ctx, "nonexistent", req)
	if err == nil {
		t.Error("Update() should return error for non-existent category")
	}
}

func TestDelete_Success(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":        "health",
		"name":       "Health",
		"active":     true,
		"created_at": time.Now(),
		"updated_at": time.Now(),
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	err = service.Delete(ctx, "health")
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify soft delete (active = false)
	result, err := service.GetByID(ctx, "health")
	if err != nil {
		t.Errorf("GetByID() after delete error = %v", err)
	}

	if result == nil {
		t.Fatal("GetByID() after delete returned nil")
	}

	if result.Active {
		t.Error("Delete() should set active to false (soft delete)")
	}
}

func TestDelete_NotFound(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.Delete(ctx, "nonexistent")
	if err == nil {
		t.Error("Delete() should return error for non-existent category")
	}
}

func TestInvalidateCache(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":    "health",
		"name":   "Health",
		"active": true,
		"order":  1,
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	// Populate cache
	_, err = service.ListActive(ctx)
	if err != nil {
		t.Errorf("ListActive() error = %v", err)
	}

	// Invalidate cache
	service.InvalidateCache(ctx)

	// Update MongoDB
	collection.UpdateOne(ctx, bson.M{"_id": "health"}, bson.M{"$set": bson.M{"name": "Updated"}})

	// Next call should fetch from MongoDB (not cache)
	result, err := service.ListActive(ctx)
	if err != nil {
		t.Errorf("ListActive() after invalidate error = %v", err)
	}

	if len(result) > 0 && result[0].Name != "Updated" {
		t.Error("InvalidateCache() did not clear cache properly")
	}
}

func TestInitializeCategoryOptIns_GlobalOptInTrue(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test categories with different defaults
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	categories := []interface{}{
		bson.M{
			"_id":            "health",
			"name":           "Health",
			"default_opt_in": true,
			"active":         true,
			"order":          1,
		},
		bson.M{
			"_id":            "marketing",
			"name":           "Marketing",
			"default_opt_in": false,
			"active":         true,
			"order":          2,
		},
	}

	_, err := collection.InsertMany(ctx, categories)
	if err != nil {
		t.Fatalf("Failed to insert categories: %v", err)
	}

	result, err := service.InitializeCategoryOptIns(ctx, true)
	if err != nil {
		t.Errorf("InitializeCategoryOptIns() error = %v", err)
	}

	if result["health"] != true {
		t.Errorf("InitializeCategoryOptIns() health = %v, want true (default)", result["health"])
	}

	if result["marketing"] != false {
		t.Errorf("InitializeCategoryOptIns() marketing = %v, want false (default)", result["marketing"])
	}
}

func TestInitializeCategoryOptIns_GlobalOptInFalse(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test categories
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	categories := []interface{}{
		bson.M{
			"_id":            "health",
			"name":           "Health",
			"default_opt_in": true,
			"active":         true,
			"order":          1,
		},
		bson.M{
			"_id":            "marketing",
			"name":           "Marketing",
			"default_opt_in": true,
			"active":         true,
			"order":          2,
		},
	}

	_, err := collection.InsertMany(ctx, categories)
	if err != nil {
		t.Fatalf("Failed to insert categories: %v", err)
	}

	result, err := service.InitializeCategoryOptIns(ctx, false)
	if err != nil {
		t.Errorf("InitializeCategoryOptIns() error = %v", err)
	}

	// All should be false when global opt-in is false
	if result["health"] != false {
		t.Errorf("InitializeCategoryOptIns() health = %v, want false (global opt-in false)", result["health"])
	}

	if result["marketing"] != false {
		t.Errorf("InitializeCategoryOptIns() marketing = %v, want false (global opt-in false)", result["marketing"])
	}
}

func TestValidateCategoryExists_Success(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":    "health",
		"name":   "Health",
		"active": true,
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	err = service.ValidateCategoryExists(ctx, "health")
	if err != nil {
		t.Errorf("ValidateCategoryExists() error = %v, want nil for active category", err)
	}
}

func TestValidateCategoryExists_NotFound(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.ValidateCategoryExists(ctx, "nonexistent")
	if err == nil {
		t.Error("ValidateCategoryExists() should return error for non-existent category")
	}
}

func TestValidateCategoryExists_Inactive(t *testing.T) {
	service, cleanup := setupNotificationCategoryServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert inactive category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":    "health",
		"name":   "Health",
		"active": false,
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	err = service.ValidateCategoryExists(ctx, "health")
	if err == nil {
		t.Error("ValidateCategoryExists() should return error for inactive category")
	}
}
