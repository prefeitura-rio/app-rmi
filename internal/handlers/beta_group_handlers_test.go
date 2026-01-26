package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func setupBetaGroupHandlersTest(t *testing.T) (*BetaGroupHandlers, *gin.Engine, func()) {
	// Use the shared MongoDB and Redis from common_test.go TestMain
	gin.SetMode(gin.TestMode)

	// Configure test collections
	config.AppConfig.BetaGroupCollection = "test_beta_groups"
	config.AppConfig.AdminGroup = "go:admin"

	ctx := context.Background()
	database := config.MongoDB

	// Initialize service
	betaGroupService := services.NewBetaGroupService(logging.Logger)
	handlers := NewBetaGroupHandlers(logging.Logger, betaGroupService)

	router := gin.New()

	// Create admin middleware for testing
	adminMiddleware := func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "03561350712",
		}
		claims.RealmAccess.Roles = []string{"go:admin"}
		c.Set("claims", claims)
		c.Next()
	}

	router.Use(adminMiddleware)
	router.POST("/admin/beta/groups", handlers.CreateGroup)
	router.GET("/admin/beta/groups/:group_id", handlers.GetGroup)
	router.GET("/admin/beta/groups", handlers.ListGroups)
	router.PUT("/admin/beta/groups/:group_id", handlers.UpdateGroup)
	router.DELETE("/admin/beta/groups/:group_id", handlers.DeleteGroup)
	router.GET("/beta/status", handlers.GetBetaStatus)
	router.POST("/admin/beta/groups/:group_id/whitelist", handlers.AddToWhitelist)
	router.DELETE("/admin/beta/groups/:group_id/whitelist", handlers.RemoveFromWhitelist)
	router.GET("/admin/beta/groups/:group_id/whitelist", handlers.ListWhitelistedPhones)

	return handlers, router, func() {
		_ = database.Drop(ctx)
	}
}

func TestNewBetaGroupHandlers(t *testing.T) {
	handlers := NewBetaGroupHandlers(logging.Logger, nil)
	if handlers == nil {
		t.Error("NewBetaGroupHandlers() returned nil")
		return
	}

	if handlers.logger == nil {
		t.Error("NewBetaGroupHandlers() logger is nil")
	}
}

func TestCreateGroup_Success(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	reqBody := models.BetaGroupRequest{
		Name: "test_group",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/beta/groups", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("CreateGroup() status = %v, want %v (body: %s)", w.Code, http.StatusCreated, w.Body.String())
	}

	var response models.BetaGroupResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Name != "test_group" {
		t.Errorf("CreateGroup() Name = %v, want test_group", response.Name)
	}
}

func TestCreateGroup_InvalidJSON(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/admin/beta/groups", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("CreateGroup() with invalid JSON status = %v, want %v", w.Code, http.StatusBadRequest)
	}
}

func TestGetGroup_NotFound(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	// Use a valid ObjectID format that doesn't exist
	req, _ := http.NewRequest("GET", "/admin/beta/groups/507f1f77bcf86cd799439011", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GetGroup() not found status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestGetGroup_Success(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test group with valid ObjectID
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
	testID := "507f1f77bcf86cd799439011"
	objectID, _ := primitive.ObjectIDFromHex(testID)
	group := bson.M{
		"_id":       objectID,
		"name":      "test_group",
		"is_active": true,
		"whitelist": []string{},
	}

	_, err := collection.InsertOne(ctx, group)
	if err != nil {
		t.Fatalf("Failed to insert beta group: %v", err)
	}

	req, _ := http.NewRequest("GET", "/admin/beta/groups/"+testID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("GetGroup() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.BetaGroupResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Name != "test_group" {
		t.Errorf("GetGroup() Name = %v, want test_group", response.Name)
	}
}

func TestListGroups_Empty(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/admin/beta/groups?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ListGroups() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.BetaGroupListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.TotalGroups != 0 {
		t.Errorf("ListGroups() TotalGroups = %v, want 0", response.TotalGroups)
	}
}

func TestListGroups_WithData(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test groups
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
	groups := []interface{}{
		bson.M{"_id": "group1", "name": "group1", "is_active": true, "whitelist": []string{}},
		bson.M{"_id": "group2", "name": "group2", "is_active": true, "whitelist": []string{}},
	}

	_, err := collection.InsertMany(ctx, groups)
	if err != nil {
		t.Fatalf("Failed to insert beta groups: %v", err)
	}

	req, _ := http.NewRequest("GET", "/admin/beta/groups?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ListGroups() status = %v, want %v", w.Code, http.StatusOK)
	}

	var response models.BetaGroupListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.TotalGroups != 2 {
		t.Errorf("ListGroups() TotalGroups = %v, want 2", response.TotalGroups)
	}
}

func TestUpdateGroup_NotFound(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	reqBody := models.BetaGroupRequest{
		Name: "updated_group",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("PUT", "/admin/beta/groups/507f1f77bcf86cd799439012", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("UpdateGroup() not found status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestDeleteGroup_NotFound(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("DELETE", "/admin/beta/groups/507f1f77bcf86cd799439013", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("DeleteGroup() not found status = %v, want %v", w.Code, http.StatusNotFound)
	}
}

func TestDeleteGroup_Success(t *testing.T) {
	_, router, cleanup := setupBetaGroupHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test group with valid ObjectID
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
	testID := "507f1f77bcf86cd799439014"
	objectID, _ := primitive.ObjectIDFromHex(testID)
	group := bson.M{
		"_id":       objectID,
		"name":      "test_group",
		"is_active": true,
		"whitelist": []string{},
	}

	_, err := collection.InsertOne(ctx, group)
	if err != nil {
		t.Fatalf("Failed to insert beta group: %v", err)
	}

	req, _ := http.NewRequest("DELETE", "/admin/beta/groups/"+testID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("DeleteGroup() status = %v, want %v", w.Code, http.StatusNoContent)
	}

	// Verify group is deleted
	var result bson.M
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&result)
	if err == nil {
		t.Error("DeleteGroup() group still exists after deletion")
	}
}

// Helper function
//
//nolint:unused // Keeping for potential future use
func boolPtr(b bool) *bool {
	return &b
}
