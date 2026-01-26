package services

import (
	"context"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
)

// setupAvatarServiceTest initializes MongoDB and Redis for testing
func setupAvatarServiceTest(t *testing.T) (*AvatarService, func()) {
	logging.InitLogger()

	// Initialize config
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AvatarsCollection = "test_avatars"
	config.AppConfig.AvatarCacheTTL = 5 * time.Minute

	// Use shared MongoDB connection
	if config.MongoDB == nil {
		t.Skip("Skipping avatar service tests: MongoDB not initialized")
	}

	ctx := context.Background()

	// Create service with nil client (uses shared connection)
	logger := zap.L().Named("avatar_service_test")
	service := NewAvatarService(nil, config.MongoDB, logger)

	return service, func() {
		// Clean up Redis
		if config.Redis != nil {
			keys, _ := config.Redis.Keys(ctx, "avatar*").Result()
			if len(keys) > 0 {
				config.Redis.Del(ctx, keys...)
			}
		}

		// Clean up only test_avatars collection
		config.MongoDB.Collection("test_avatars").Drop(ctx)
	}
}

func TestNewAvatarService(t *testing.T) {
	if config.MongoDB == nil {
		t.Skip("Skipping: MongoDB not initialized")
	}

	logger := zap.NewNop()

	service := NewAvatarService(nil, config.MongoDB, logger)
	if service == nil {
		t.Error("NewAvatarService() returned nil")
	}
}

func TestListAvatars_Empty(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	response, err := service.ListAvatars(ctx, 1, 20)
	if err != nil {
		t.Errorf("ListAvatars() error = %v", err)
	}

	if response == nil {
		t.Fatal("ListAvatars() returned nil")
	}

	if response.Total != 0 {
		t.Errorf("ListAvatars() Total = %v, want 0", response.Total)
	}

	if len(response.Data) != 0 {
		t.Errorf("ListAvatars() len(Data) = %v, want 0", len(response.Data))
	}
}

func TestListAvatars_WithData(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test avatars
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	now := time.Now()

	avatars := []interface{}{
		models.Avatar{
			ID:        primitive.NewObjectID(),
			Name:      "Avatar 1",
			URL:       "http://example.com/1.png",
			IsActive:  true,
			CreatedAt: now.Add(-2 * time.Hour),
		},
		models.Avatar{
			ID:        primitive.NewObjectID(),
			Name:      "Avatar 2",
			URL:       "http://example.com/2.png",
			IsActive:  true,
			CreatedAt: now.Add(-1 * time.Hour),
		},
		models.Avatar{
			ID:        primitive.NewObjectID(),
			Name:      "Avatar 3 Inactive",
			URL:       "http://example.com/3.png",
			IsActive:  false, // Inactive should not appear
			CreatedAt: now,
		},
	}

	_, err := collection.InsertMany(ctx, avatars)
	if err != nil {
		t.Fatalf("Failed to insert avatars: %v", err)
	}

	// List avatars
	response, err := service.ListAvatars(ctx, 1, 20)
	if err != nil {
		t.Errorf("ListAvatars() error = %v", err)
	}

	if response.Total != 2 {
		t.Errorf("ListAvatars() Total = %v, want 2 (only active)", response.Total)
	}

	if len(response.Data) != 2 {
		t.Errorf("ListAvatars() len(Data) = %v, want 2", len(response.Data))
	}

	// Should be sorted by created_at desc (latest first)
	if response.Data[0].Name != "Avatar 2" {
		t.Errorf("ListAvatars() first avatar = %v, want Avatar 2", response.Data[0].Name)
	}
}

func TestListAvatars_Pagination(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert 5 test avatars
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	var avatars []interface{}
	for i := 1; i <= 5; i++ {
		avatars = append(avatars, models.Avatar{
			ID:        primitive.NewObjectID(),
			Name:      "Avatar " + string(rune('0'+i)),
			URL:       "http://example.com/avatar.png",
			IsActive:  true,
			CreatedAt: time.Now().Add(time.Duration(-i) * time.Minute),
		})
	}

	_, err := collection.InsertMany(ctx, avatars)
	if err != nil {
		t.Fatalf("Failed to insert avatars: %v", err)
	}

	// Test pagination
	response, err := service.ListAvatars(ctx, 1, 2)
	if err != nil {
		t.Errorf("ListAvatars() page 1 error = %v", err)
	}

	if response.Total != 5 {
		t.Errorf("ListAvatars() Total = %v, want 5", response.Total)
	}

	if len(response.Data) != 2 {
		t.Errorf("ListAvatars() page 1 len(Data) = %v, want 2", len(response.Data))
	}

	if response.TotalPages != 3 {
		t.Errorf("ListAvatars() TotalPages = %v, want 3", response.TotalPages)
	}

	// Test page 2
	response2, err := service.ListAvatars(ctx, 2, 2)
	if err != nil {
		t.Errorf("ListAvatars() page 2 error = %v", err)
	}

	if len(response2.Data) != 2 {
		t.Errorf("ListAvatars() page 2 len(Data) = %v, want 2", len(response2.Data))
	}
}

func TestListAvatars_FromCache(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test avatar
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	avatar := models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "Cached Avatar",
		URL:       "http://example.com/cached.png",
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	_, err := collection.InsertOne(ctx, avatar)
	if err != nil {
		t.Fatalf("Failed to insert avatar: %v", err)
	}

	// First call - populates cache
	_, err = service.ListAvatars(ctx, 1, 20)
	if err != nil {
		t.Fatalf("First ListAvatars() error = %v", err)
	}

	// Delete from MongoDB
	collection.DeleteOne(ctx, bson.M{"_id": avatar.ID})

	// Second call - should use cache
	response, err := service.ListAvatars(ctx, 1, 20)
	if err != nil {
		t.Errorf("ListAvatars() from cache error = %v", err)
	}

	if response.Total != 1 {
		t.Errorf("ListAvatars() from cache Total = %v, want 1", response.Total)
	}
}

func TestListAvatars_InvalidPagination(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test with invalid page (should default to 1)
	response, err := service.ListAvatars(ctx, 0, 20)
	if err != nil {
		t.Errorf("ListAvatars() error = %v", err)
	}

	if response.Page != 1 {
		t.Errorf("ListAvatars() Page = %v, want 1 (default)", response.Page)
	}

	// Test with invalid perPage (should default to 20)
	response, err = service.ListAvatars(ctx, 1, 0)
	if err != nil {
		t.Errorf("ListAvatars() error = %v", err)
	}

	if response.PerPage != 20 {
		t.Errorf("ListAvatars() PerPage = %v, want 20 (default)", response.PerPage)
	}

	// Test with perPage > 100 (should cap to 20)
	response, err = service.ListAvatars(ctx, 1, 150)
	if err != nil {
		t.Errorf("ListAvatars() error = %v", err)
	}

	if response.PerPage != 20 {
		t.Errorf("ListAvatars() PerPage = %v, want 20 (capped)", response.PerPage)
	}
}

func TestGetAvatarByID_Success(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test avatar
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	avatar := models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "Test Avatar",
		URL:       "http://example.com/test.png",
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	_, err := collection.InsertOne(ctx, avatar)
	if err != nil {
		t.Fatalf("Failed to insert avatar: %v", err)
	}

	// Get avatar
	result, err := service.GetAvatarByID(ctx, avatar.ID.Hex())
	if err != nil {
		t.Errorf("GetAvatarByID() error = %v", err)
	}

	if result == nil {
		t.Fatal("GetAvatarByID() returned nil")
	}

	if result.Name != "Test Avatar" {
		t.Errorf("GetAvatarByID() Name = %v, want Test Avatar", result.Name)
	}
}

func TestGetAvatarByID_FromCache(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test avatar
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	avatar := models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "Cached Avatar",
		URL:       "http://example.com/cached.png",
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	_, err := collection.InsertOne(ctx, avatar)
	if err != nil {
		t.Fatalf("Failed to insert avatar: %v", err)
	}

	// First call - populates cache
	_, err = service.GetAvatarByID(ctx, avatar.ID.Hex())
	if err != nil {
		t.Fatalf("First GetAvatarByID() error = %v", err)
	}

	// Delete from MongoDB
	collection.DeleteOne(ctx, bson.M{"_id": avatar.ID})

	// Second call - should use cache
	result, err := service.GetAvatarByID(ctx, avatar.ID.Hex())
	if err != nil {
		t.Errorf("GetAvatarByID() from cache error = %v", err)
	}

	if result == nil {
		t.Fatal("GetAvatarByID() from cache returned nil")
	}

	if result.Name != "Cached Avatar" {
		t.Errorf("GetAvatarByID() from cache Name = %v, want Cached Avatar", result.Name)
	}
}

func TestGetAvatarByID_NotFound(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get non-existent avatar
	nonExistentID := primitive.NewObjectID().Hex()
	result, err := service.GetAvatarByID(ctx, nonExistentID)

	if err != nil {
		t.Errorf("GetAvatarByID() error = %v, want nil", err)
	}

	if result != nil {
		t.Errorf("GetAvatarByID() = %v, want nil for non-existent", result)
	}
}

func TestGetAvatarByID_InvalidID(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try with invalid ObjectID
	_, err := service.GetAvatarByID(ctx, "invalid-id")
	if err == nil {
		t.Error("GetAvatarByID() should return error for invalid ID")
	}
}

func TestGetAvatarByID_Inactive(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert inactive avatar
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	avatar := models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "Inactive Avatar",
		URL:       "http://example.com/inactive.png",
		IsActive:  false,
		CreatedAt: time.Now(),
	}

	_, err := collection.InsertOne(ctx, avatar)
	if err != nil {
		t.Fatalf("Failed to insert avatar: %v", err)
	}

	// Try to get inactive avatar
	result, err := service.GetAvatarByID(ctx, avatar.ID.Hex())
	if err != nil {
		t.Errorf("GetAvatarByID() error = %v", err)
	}

	if result != nil {
		t.Errorf("GetAvatarByID() should return nil for inactive avatar")
	}
}

func TestCreateAvatar_Success(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	request := &models.AvatarRequest{
		Name: "New Avatar",
		URL:  "http://example.com/new.png",
	}

	avatar, err := service.CreateAvatar(ctx, request)
	if err != nil {
		t.Errorf("CreateAvatar() error = %v", err)
	}

	if avatar == nil {
		t.Fatal("CreateAvatar() returned nil")
	}

	if avatar.Name != "New Avatar" {
		t.Errorf("CreateAvatar() Name = %v, want New Avatar", avatar.Name)
	}

	if !avatar.IsActive {
		t.Error("CreateAvatar() IsActive should be true")
	}

	// Verify it was saved to MongoDB
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	var found models.Avatar
	err = collection.FindOne(ctx, bson.M{"_id": avatar.ID}).Decode(&found)
	if err != nil {
		t.Errorf("Created avatar not found in MongoDB: %v", err)
	}
}

func TestDeleteAvatar_Success(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create avatar first
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	avatar := models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "To Delete",
		URL:       "http://example.com/delete.png",
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	_, err := collection.InsertOne(ctx, avatar)
	if err != nil {
		t.Fatalf("Failed to insert avatar: %v", err)
	}

	// Delete avatar
	err = service.DeleteAvatar(ctx, avatar.ID.Hex())
	if err != nil {
		t.Errorf("DeleteAvatar() error = %v", err)
	}

	// Verify it's soft deleted (is_active = false)
	var deleted models.Avatar
	err = collection.FindOne(ctx, bson.M{"_id": avatar.ID}).Decode(&deleted)
	if err != nil {
		t.Errorf("Failed to find deleted avatar: %v", err)
	}

	if deleted.IsActive {
		t.Error("DeleteAvatar() should set is_active to false")
	}
}

func TestDeleteAvatar_NotFound(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to delete non-existent avatar
	nonExistentID := primitive.NewObjectID().Hex()
	err := service.DeleteAvatar(ctx, nonExistentID)

	// Should return error for non-existent avatar
	if err == nil {
		t.Error("DeleteAvatar() should return error for non-existent avatar")
	}
}

func TestDeleteAvatar_InvalidID(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try with invalid ObjectID
	err := service.DeleteAvatar(ctx, "invalid-id")
	if err == nil {
		t.Error("DeleteAvatar() should return error for invalid ID")
	}
}

func TestValidateAvatarExists_True(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test avatar
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	avatar := models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "Exists",
		URL:       "http://example.com/exists.png",
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	_, err := collection.InsertOne(ctx, avatar)
	if err != nil {
		t.Fatalf("Failed to insert avatar: %v", err)
	}

	// Validate avatar exists
	exists, err := service.ValidateAvatarExists(ctx, avatar.ID.Hex())
	if err != nil {
		t.Errorf("ValidateAvatarExists() error = %v", err)
	}

	if !exists {
		t.Error("ValidateAvatarExists() = false, want true")
	}
}

func TestValidateAvatarExists_False(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Check non-existent avatar
	nonExistentID := primitive.NewObjectID().Hex()
	exists, err := service.ValidateAvatarExists(ctx, nonExistentID)
	if err != nil {
		t.Errorf("ValidateAvatarExists() error = %v", err)
	}

	if exists {
		t.Error("ValidateAvatarExists() = true, want false")
	}
}

func TestValidateAvatarExists_Inactive(t *testing.T) {
	service, cleanup := setupAvatarServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert inactive avatar
	collection := config.MongoDB.Collection(config.AppConfig.AvatarsCollection)
	avatar := models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "Inactive",
		URL:       "http://example.com/inactive.png",
		IsActive:  false,
		CreatedAt: time.Now(),
	}

	_, err := collection.InsertOne(ctx, avatar)
	if err != nil {
		t.Fatalf("Failed to insert avatar: %v", err)
	}

	// Validate - should return false for inactive
	exists, err := service.ValidateAvatarExists(ctx, avatar.ID.Hex())
	if err != nil {
		t.Errorf("ValidateAvatarExists() error = %v", err)
	}

	if exists {
		t.Error("ValidateAvatarExists() should return false for inactive avatar")
	}
}
