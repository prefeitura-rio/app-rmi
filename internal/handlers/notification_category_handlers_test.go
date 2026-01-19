package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

func setupNotificationCategoryHandlersTest(t *testing.T) (*NotificationCategoryHandlers, *gin.Engine, func()) {
	// Use the shared MongoDB and Redis from common_test.go TestMain
	gin.SetMode(gin.TestMode)

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.NotificationCategoryCollection = "test_notification_categories"
	config.AppConfig.NotificationCategoryCacheTTL = 5 * time.Minute

	ctx := context.Background()
	database := config.MongoDB

	handlers := NewNotificationCategoryHandlers(logging.Logger)

	router := gin.New()
	router.GET("/notification-categories", handlers.ListCategories)
	router.POST("/admin/notification-categories", handlers.CreateCategory)
	router.PUT("/admin/notification-categories/:category_id", handlers.UpdateCategory)
	router.DELETE("/admin/notification-categories/:category_id", handlers.DeleteCategory)

	return handlers, router, func() {
		// Clean up Redis
		patterns := []string{"notification_categories:*"}
		for _, pattern := range patterns {
			keys, _ := config.Redis.Keys(ctx, pattern).Result()
			if len(keys) > 0 {
				config.Redis.Del(ctx, keys...)
			}
		}

		database.Drop(ctx)
	}
}

func TestNewNotificationCategoryHandlers(t *testing.T) {
	handlers := NewNotificationCategoryHandlers(logging.Logger)
	if handlers == nil {
		t.Error("NewNotificationCategoryHandlers() returned nil")
	}

	if handlers.service == nil {
		t.Error("NewNotificationCategoryHandlers() service is nil")
	}
}

func TestListCategories_Empty(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/notification-categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ListCategories() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategoriesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response.Categories) != 0 {
		t.Errorf("ListCategories() len(Categories) = %v, want 0", len(response.Categories))
	}
}

func TestListCategories_WithData(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
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

	req, _ := http.NewRequest("GET", "/notification-categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ListCategories() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategoriesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Should only return active categories
	if len(response.Categories) != 2 {
		t.Errorf("ListCategories() len(Categories) = %v, want 2", len(response.Categories))
	}
}

func TestCreateCategory_Success(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	reqBody := models.CreateNotificationCategoryRequest{
		ID:           "new_category",
		Name:         "New Category",
		Description:  "Test description",
		DefaultOptIn: true,
		Active:       true,
		Order:        1,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/notification-categories", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("CreateCategory() status = %v, want %v", w.Code, http.StatusCreated)
	}

	var response models.NotificationCategory
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.ID != "new_category" {
		t.Errorf("CreateCategory() ID = %v, want new_category", response.ID)
	}

	if response.Name != "New Category" {
		t.Errorf("CreateCategory() Name = %v, want New Category", response.Name)
	}
}

func TestCreateCategory_InvalidRequest(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/admin/notification-categories", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("CreateCategory() with invalid JSON status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestCreateCategory_Duplicate(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
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

	reqBody := models.CreateNotificationCategoryRequest{
		ID:           "existing",
		Name:         "Duplicate",
		Description:  "Duplicate description",
		DefaultOptIn: true,
		Active:       true,
		Order:        1,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/notification-categories", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("CreateCategory() duplicate status = %v, want %v", w.Code, http.StatusConflict)
	}
}

func TestUpdateCategory_Success(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
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
	reqBody := models.UpdateNotificationCategoryRequest{
		Name:        &newName,
		Description: &newDesc,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/admin/notification-categories/health", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("UpdateCategory() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategory
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Name != "Updated Health" {
		t.Errorf("UpdateCategory() Name = %v, want Updated Health", response.Name)
	}

	if response.Description != "New description" {
		t.Errorf("UpdateCategory() Description = %v, want New description", response.Description)
	}
}

func TestUpdateCategory_InvalidRequest(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("PUT", "/admin/notification-categories/health", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("UpdateCategory() with invalid JSON status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateCategory_NotFound(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	newName := "Updated"
	reqBody := models.UpdateNotificationCategoryRequest{
		Name: &newName,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/admin/notification-categories/nonexistent", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("UpdateCategory() not found status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestDeleteCategory_Success(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
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

	req, _ := http.NewRequest("DELETE", "/admin/notification-categories/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("DeleteCategory() status = %v, want %v", w.Code, http.StatusNoContent)
	}

	// Verify soft delete
	var result bson.M
	err = collection.FindOne(ctx, bson.M{"_id": "health"}).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to find category after delete: %v", err)
	}

	if result["active"].(bool) {
		t.Error("DeleteCategory() should set active to false (soft delete)")
	}
}

func TestDeleteCategory_NotFound(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("DELETE", "/admin/notification-categories/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("DeleteCategory() not found status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

// Additional comprehensive tests for better coverage

func TestCreateCategory_MissingRequiredFields(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	testCases := []struct {
		name     string
		reqBody  map[string]interface{}
		wantCode int
	}{
		{
			name:     "missing ID",
			reqBody:  map[string]interface{}{"name": "Test", "description": "Test desc"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing name",
			reqBody:  map[string]interface{}{"id": "test", "description": "Test desc"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "missing description",
			reqBody:  map[string]interface{}{"id": "test", "name": "Test"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty ID",
			reqBody:  map[string]interface{}{"id": "", "name": "Test", "description": "Test desc"},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty name",
			reqBody:  map[string]interface{}{"id": "test", "name": "", "description": "Test desc"},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.reqBody)
			req, _ := http.NewRequest("POST", "/admin/notification-categories", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tc.wantCode {
				t.Errorf("%s: status = %v, want %v (body: %s)", tc.name, w.Code, tc.wantCode, w.Body.String())
			}
		})
	}
}

func TestCreateCategory_WithAllFields(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	reqBody := models.CreateNotificationCategoryRequest{
		ID:           "full_category",
		Name:         "Full Category",
		Description:  "Complete test description",
		DefaultOptIn: true,
		Active:       true,
		Order:        5,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/notification-categories", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("CreateCategory() status = %v, want %v (body: %s)", w.Code, http.StatusCreated, w.Body.String())
	}

	var response models.NotificationCategory
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.ID != "full_category" {
		t.Errorf("ID = %v, want full_category", response.ID)
	}
	if response.Name != "Full Category" {
		t.Errorf("Name = %v, want Full Category", response.Name)
	}
	if response.Description != "Complete test description" {
		t.Errorf("Description = %v, want Complete test description", response.Description)
	}
	if response.DefaultOptIn != true {
		t.Errorf("DefaultOptIn = %v, want true", response.DefaultOptIn)
	}
	if response.Active != true {
		t.Errorf("Active = %v, want true", response.Active)
	}
	if response.Order != 5 {
		t.Errorf("Order = %v, want 5", response.Order)
	}
	if response.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if response.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestCreateCategory_DefaultValues(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	reqBody := models.CreateNotificationCategoryRequest{
		ID:          "default_category",
		Name:        "Default Category",
		Description: "Category with defaults",
		// DefaultOptIn, Active, Order will use zero values
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/notification-categories", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("CreateCategory() status = %v, want %v", w.Code, http.StatusCreated)
	}

	var response models.NotificationCategory
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.DefaultOptIn != false {
		t.Errorf("DefaultOptIn = %v, want false (default)", response.DefaultOptIn)
	}
	if response.Active != false {
		t.Errorf("Active = %v, want false (default)", response.Active)
	}
	if response.Order != 0 {
		t.Errorf("Order = %v, want 0 (default)", response.Order)
	}
}

func TestUpdateCategory_PartialUpdate_Name(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "test_partial",
		"name":           "Original Name",
		"description":    "Original Description",
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

	newName := "Updated Name Only"
	reqBody := models.UpdateNotificationCategoryRequest{
		Name: &newName,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/admin/notification-categories/test_partial", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("UpdateCategory() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategory
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Name != "Updated Name Only" {
		t.Errorf("Name = %v, want Updated Name Only", response.Name)
	}
	// Other fields should remain unchanged
	if response.Description != "Original Description" {
		t.Errorf("Description = %v, want Original Description", response.Description)
	}
	if response.DefaultOptIn != true {
		t.Errorf("DefaultOptIn = %v, want true", response.DefaultOptIn)
	}
}

func TestUpdateCategory_AllFields(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "test_all_fields",
		"name":           "Original Name",
		"description":    "Original Description",
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

	newName := "New Name"
	newDesc := "New Description"
	newDefaultOptIn := false
	newActive := false
	newOrder := 10
	reqBody := models.UpdateNotificationCategoryRequest{
		Name:         &newName,
		Description:  &newDesc,
		DefaultOptIn: &newDefaultOptIn,
		Active:       &newActive,
		Order:        &newOrder,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/admin/notification-categories/test_all_fields", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("UpdateCategory() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategory
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Name != "New Name" {
		t.Errorf("Name = %v, want New Name", response.Name)
	}
	if response.Description != "New Description" {
		t.Errorf("Description = %v, want New Description", response.Description)
	}
	if response.DefaultOptIn != false {
		t.Errorf("DefaultOptIn = %v, want false", response.DefaultOptIn)
	}
	if response.Active != false {
		t.Errorf("Active = %v, want false", response.Active)
	}
	if response.Order != 10 {
		t.Errorf("Order = %v, want 10", response.Order)
	}
}

func TestUpdateCategory_EmptyRequest(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "test_empty",
		"name":           "Original Name",
		"description":    "Original Description",
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

	// Empty update request (only updates UpdatedAt)
	reqBody := models.UpdateNotificationCategoryRequest{}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/admin/notification-categories/test_empty", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("UpdateCategory() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategory
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// All fields should remain the same
	if response.Name != "Original Name" {
		t.Errorf("Name = %v, want Original Name", response.Name)
	}
}

func TestUpdateCategory_ToggleActiveState(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "toggle_test",
		"name":           "Toggle Test",
		"description":    "Test toggling active state",
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

	newActive := false
	reqBody := models.UpdateNotificationCategoryRequest{
		Active: &newActive,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/admin/notification-categories/toggle_test", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("UpdateCategory() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategory
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Active != false {
		t.Errorf("Active = %v, want false", response.Active)
	}
}

func TestListCategories_Ordering(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test categories in random order
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	categories := []interface{}{
		bson.M{
			"_id":            "third",
			"name":           "Third",
			"description":    "Should be third",
			"default_opt_in": true,
			"active":         true,
			"order":          3,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
		bson.M{
			"_id":            "first",
			"name":           "First",
			"description":    "Should be first",
			"default_opt_in": true,
			"active":         true,
			"order":          1,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
		bson.M{
			"_id":            "second",
			"name":           "Second",
			"description":    "Should be second",
			"default_opt_in": true,
			"active":         true,
			"order":          2,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
	}

	_, err := collection.InsertMany(ctx, categories)
	if err != nil {
		t.Fatalf("Failed to insert categories: %v", err)
	}

	req, _ := http.NewRequest("GET", "/notification-categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ListCategories() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategoriesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(response.Categories) != 3 {
		t.Fatalf("len(Categories) = %v, want 3", len(response.Categories))
	}

	// Verify ordering
	if response.Categories[0].ID != "first" {
		t.Errorf("Categories[0].ID = %v, want first", response.Categories[0].ID)
	}
	if response.Categories[1].ID != "second" {
		t.Errorf("Categories[1].ID = %v, want second", response.Categories[1].ID)
	}
	if response.Categories[2].ID != "third" {
		t.Errorf("Categories[2].ID = %v, want third", response.Categories[2].ID)
	}
}

func TestListCategories_CacheHit(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "cache_test",
		"name":           "Cache Test",
		"description":    "Test cache behavior",
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

	// First request - should cache the result
	req1, _ := http.NewRequest("GET", "/notification-categories", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Errorf("First ListCategories() status = %v, want %v", w1.Code, http.StatusOK)
	}

	// Second request - should hit cache
	req2, _ := http.NewRequest("GET", "/notification-categories", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Second ListCategories() status = %v, want %v", w2.Code, http.StatusOK)
	}

	var response1, response2 models.NotificationCategoriesResponse
	json.Unmarshal(w1.Body.Bytes(), &response1)
	json.Unmarshal(w2.Body.Bytes(), &response2)

	if len(response1.Categories) != len(response2.Categories) {
		t.Errorf("Cache responses differ: %v vs %v", len(response1.Categories), len(response2.Categories))
	}
}

func TestDeleteCategory_InvalidatesCache(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "cache_invalidate",
		"name":           "Cache Invalidate",
		"description":    "Test cache invalidation",
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

	// First request - should cache the result
	req1, _ := http.NewRequest("GET", "/notification-categories", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	var response1 models.NotificationCategoriesResponse
	json.Unmarshal(w1.Body.Bytes(), &response1)

	if len(response1.Categories) != 1 {
		t.Fatalf("Expected 1 category before delete, got %v", len(response1.Categories))
	}

	// Delete the category
	reqDelete, _ := http.NewRequest("DELETE", "/admin/notification-categories/cache_invalidate", nil)
	wDelete := httptest.NewRecorder()
	router.ServeHTTP(wDelete, reqDelete)

	if wDelete.Code != http.StatusNoContent {
		t.Errorf("DeleteCategory() status = %v, want %v", wDelete.Code, http.StatusNoContent)
	}

	// Second request - cache should be invalidated, should not return deleted category
	req2, _ := http.NewRequest("GET", "/notification-categories", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	var response2 models.NotificationCategoriesResponse
	json.Unmarshal(w2.Body.Bytes(), &response2)

	if len(response2.Categories) != 0 {
		t.Errorf("Expected 0 categories after delete (soft delete sets active=false), got %v", len(response2.Categories))
	}
}

func TestCreateCategory_InvalidatesCache(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	// First request - empty list, should cache empty result
	req1, _ := http.NewRequest("GET", "/notification-categories", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	var response1 models.NotificationCategoriesResponse
	json.Unmarshal(w1.Body.Bytes(), &response1)

	if len(response1.Categories) != 0 {
		t.Fatalf("Expected 0 categories initially, got %v", len(response1.Categories))
	}

	// Create a new category
	reqBody := models.CreateNotificationCategoryRequest{
		ID:           "new_cache",
		Name:         "New Cache",
		Description:  "Test cache invalidation on create",
		DefaultOptIn: true,
		Active:       true,
		Order:        1,
	}

	body, _ := json.Marshal(reqBody)
	reqCreate, _ := http.NewRequest("POST", "/admin/notification-categories", bytes.NewBuffer(body))
	reqCreate.Header.Set("Content-Type", "application/json")
	wCreate := httptest.NewRecorder()
	router.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusCreated {
		t.Errorf("CreateCategory() status = %v, want %v", wCreate.Code, http.StatusCreated)
	}

	// Second request - should show new category (cache invalidated)
	req2, _ := http.NewRequest("GET", "/notification-categories", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	var response2 models.NotificationCategoriesResponse
	json.Unmarshal(w2.Body.Bytes(), &response2)

	if len(response2.Categories) != 1 {
		t.Errorf("Expected 1 category after create, got %v", len(response2.Categories))
	}
}

func TestUpdateCategory_InvalidatesCache(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":            "update_cache",
		"name":           "Update Cache",
		"description":    "Original",
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

	// First request - should cache the result
	req1, _ := http.NewRequest("GET", "/notification-categories", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	var response1 models.NotificationCategoriesResponse
	json.Unmarshal(w1.Body.Bytes(), &response1)

	if response1.Categories[0].Description != "Original" {
		t.Fatalf("Expected 'Original' description, got %v", response1.Categories[0].Description)
	}

	// Update the category
	newDesc := "Updated"
	reqBody := models.UpdateNotificationCategoryRequest{
		Description: &newDesc,
	}

	body, _ := json.Marshal(reqBody)
	reqUpdate, _ := http.NewRequest("PUT", "/admin/notification-categories/update_cache", bytes.NewBuffer(body))
	reqUpdate.Header.Set("Content-Type", "application/json")
	wUpdate := httptest.NewRecorder()
	router.ServeHTTP(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Errorf("UpdateCategory() status = %v, want %v", wUpdate.Code, http.StatusOK)
	}

	// Second request - should reflect update (cache invalidated)
	req2, _ := http.NewRequest("GET", "/notification-categories", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	var response2 models.NotificationCategoriesResponse
	json.Unmarshal(w2.Body.Bytes(), &response2)

	if response2.Categories[0].Description != "Updated" {
		t.Errorf("Expected 'Updated' description after update, got %v", response2.Categories[0].Description)
	}
}

func TestListCategories_MixedActiveInactive(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test categories with mixed active states
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	categories := []interface{}{
		bson.M{
			"_id":            "active1",
			"name":           "Active 1",
			"description":    "Active category 1",
			"default_opt_in": true,
			"active":         true,
			"order":          1,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
		bson.M{
			"_id":            "inactive1",
			"name":           "Inactive 1",
			"description":    "Inactive category 1",
			"default_opt_in": false,
			"active":         false,
			"order":          2,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
		bson.M{
			"_id":            "active2",
			"name":           "Active 2",
			"description":    "Active category 2",
			"default_opt_in": true,
			"active":         true,
			"order":          3,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
		bson.M{
			"_id":            "inactive2",
			"name":           "Inactive 2",
			"description":    "Inactive category 2",
			"default_opt_in": false,
			"active":         false,
			"order":          4,
			"created_at":     time.Now(),
			"updated_at":     time.Now(),
		},
	}

	_, err := collection.InsertMany(ctx, categories)
	if err != nil {
		t.Fatalf("Failed to insert categories: %v", err)
	}

	req, _ := http.NewRequest("GET", "/notification-categories", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ListCategories() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.NotificationCategoriesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Should only return active categories
	if len(response.Categories) != 2 {
		t.Errorf("ListCategories() len(Categories) = %v, want 2 (only active)", len(response.Categories))
	}

	// Verify all returned categories are active
	for _, cat := range response.Categories {
		if !cat.Active {
			t.Errorf("ListCategories() returned inactive category: %v", cat.ID)
		}
	}
}

func TestDeleteCategory_AlreadyInactive(t *testing.T) {
	_, router, cleanup := setupNotificationCategoryHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert already inactive category
	collection := config.MongoDB.Collection(config.AppConfig.NotificationCategoryCollection)
	category := bson.M{
		"_id":        "already_inactive",
		"name":       "Already Inactive",
		"active":     false,
		"created_at": time.Now(),
		"updated_at": time.Now(),
	}

	_, err := collection.InsertOne(ctx, category)
	if err != nil {
		t.Fatalf("Failed to insert category: %v", err)
	}

	req, _ := http.NewRequest("DELETE", "/admin/notification-categories/already_inactive", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should still succeed (soft delete is idempotent)
	if w.Code != http.StatusNoContent {
		t.Errorf("DeleteCategory() status = %v, want %v", w.Code, http.StatusNoContent)
	}

	// Verify still inactive
	var result bson.M
	err = collection.FindOne(ctx, bson.M{"_id": "already_inactive"}).Decode(&result)
	if err != nil {
		t.Fatalf("Failed to find category after delete: %v", err)
	}

	if result["active"].(bool) {
		t.Error("DeleteCategory() should keep active=false")
	}
}
