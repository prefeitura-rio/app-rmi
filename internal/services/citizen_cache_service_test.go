package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// setupCitizenCacheTest initializes MongoDB and Redis for testing
func setupCitizenCacheTest(t *testing.T) (*CitizenCacheService, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping citizen cache service tests: MONGODB_URI not set")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	logging.InitLogger()

	// Initialize config
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// MongoDB setup
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to ping MongoDB: %v", err)
	}

	config.MongoDB = client.Database("rmi_test")
	config.AppConfig.CitizenCollection = "test_citizens"

	// Redis setup
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	config.Redis = redisclient.NewClient(singleClient)

	// Test Redis connection
	err = config.Redis.Ping(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Create service
	service := NewCitizenCacheService()

	return service, func() {
		// Clean up Redis
		keys, _ := config.Redis.Keys(ctx, "citizen:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}

		// Clean up MongoDB
		config.MongoDB.Drop(ctx)
		client.Disconnect(ctx)
	}
}

func TestGetCitizen_FromMongoDB(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert citizen directly into MongoDB
	nome := "João da Silva"
	citizen := models.Citizen{
		CPF:  "03561350712",
		Nome: &nome,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert citizen: %v", err)
	}

	// Get citizen - should retrieve from MongoDB and cache
	result, err := service.GetCitizen(ctx, "03561350712")
	if err != nil {
		t.Errorf("GetCitizen() error = %v", err)
	}

	if result.CPF != "03561350712" {
		t.Errorf("GetCitizen() CPF = %v, want 03561350712", result.CPF)
	}

	if result.Nome == nil || *result.Nome != "João da Silva" {
		var nome string
		if result.Nome != nil {
			nome = *result.Nome
		}
		t.Errorf("GetCitizen() Nome = %v, want João da Silva", nome)
	}

	// Verify data was cached
	if !service.IsCitizenInCache(ctx, "03561350712") {
		t.Error("Citizen should be cached after retrieval")
	}
}

func TestGetCitizen_FromCache(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert citizen into MongoDB
	nome := "João da Silva"
	citizen := models.Citizen{
		CPF:  "03561350712",
		Nome: &nome,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert citizen: %v", err)
	}

	// First retrieval - caches the data
	_, err = service.GetCitizen(ctx, "03561350712")
	if err != nil {
		t.Fatalf("First GetCitizen() error = %v", err)
	}

	// Delete from MongoDB to verify second retrieval uses cache
	_, err = collection.DeleteOne(ctx, bson.M{"cpf": "03561350712"})
	if err != nil {
		t.Fatalf("Failed to delete citizen: %v", err)
	}

	// Second retrieval - should use cache
	result, err := service.GetCitizen(ctx, "03561350712")
	if err != nil {
		t.Errorf("GetCitizen() from cache error = %v", err)
	}

	if result.CPF != "03561350712" {
		t.Errorf("GetCitizen() from cache CPF = %v, want 03561350712", result.CPF)
	}
}

func TestGetCitizen_NotFound(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get non-existent citizen
	_, err := service.GetCitizen(ctx, "99999999999")
	if err == nil {
		t.Error("GetCitizen() should return error for non-existent citizen")
	}
}

func TestUpdateCitizen_Success(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	nome := "João da Silva"
	citizen := &models.Citizen{
		CPF:  "03561350712",
		Nome: &nome,
	}

	// Update citizen - should write to cache
	err := service.UpdateCitizen(ctx, "03561350712", citizen)
	if err != nil {
		t.Errorf("UpdateCitizen() error = %v", err)
	}

	// Verify data is in write buffer
	if !service.IsCitizenInCache(ctx, "03561350712") {
		t.Error("Citizen should be in cache after update")
	}

	// Retrieve from cache only
	cached, err := service.GetCitizenFromCacheOnly(ctx, "03561350712")
	if err != nil {
		t.Errorf("GetCitizenFromCacheOnly() error = %v", err)
	}

	if cached.CPF != "03561350712" {
		t.Errorf("GetCitizenFromCacheOnly() CPF = %v, want 03561350712", cached.CPF)
	}
}

func TestUpdateCitizen_Overwrite(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// First update
	nome1 := "João da Silva"
	citizen1 := &models.Citizen{
		CPF:  "03561350712",
		Nome: &nome1,
	}

	err := service.UpdateCitizen(ctx, "03561350712", citizen1)
	if err != nil {
		t.Fatalf("First UpdateCitizen() error = %v", err)
	}

	// Second update (overwrite) - should log but succeed
	nome2 := "João Santos"
	citizen2 := &models.Citizen{
		CPF:  "03561350712",
		Nome: &nome2,
	}

	err = service.UpdateCitizen(ctx, "03561350712", citizen2)
	if err != nil {
		t.Errorf("Second UpdateCitizen() error = %v", err)
	}

	// Verify latest data is in cache
	cached, err := service.GetCitizenFromCacheOnly(ctx, "03561350712")
	if err != nil {
		t.Errorf("GetCitizenFromCacheOnly() error = %v", err)
	}

	if cached.Nome == nil || *cached.Nome != "João Santos" {
		var nome string
		if cached.Nome != nil {
			nome = *cached.Nome
		}
		t.Errorf("GetCitizenFromCacheOnly() Nome = %v, want João Santos (latest update)", nome)
	}
}

func TestDeleteCitizen_Success(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// First, create a citizen in MongoDB
	nome := "João da Silva"
	citizen := models.Citizen{
		CPF:  "03561350712",
		Nome: &nome,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert citizen: %v", err)
	}

	// Cache it
	_, err = service.GetCitizen(ctx, "03561350712")
	if err != nil {
		t.Fatalf("GetCitizen() error = %v", err)
	}

	// Verify it's cached
	if !service.IsCitizenInCache(ctx, "03561350712") {
		t.Fatal("Citizen should be cached before delete")
	}

	// Delete citizen
	err = service.DeleteCitizen(ctx, "03561350712")
	if err != nil {
		t.Errorf("DeleteCitizen() error = %v", err)
	}

	// Give it a moment for the deletion to propagate
	time.Sleep(100 * time.Millisecond)

	// Verify cache is cleared
	if service.IsCitizenInCache(ctx, "03561350712") {
		t.Error("Citizen should not be in cache after delete")
	}
}

func TestGetCitizenFromCacheOnly_WriteBuffer(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	nome := "João da Silva"
	citizen := &models.Citizen{
		CPF:  "03561350712",
		Nome: &nome,
	}

	// Update citizen (writes to write buffer)
	err := service.UpdateCitizen(ctx, "03561350712", citizen)
	if err != nil {
		t.Fatalf("UpdateCitizen() error = %v", err)
	}

	// Should retrieve from write buffer
	result, err := service.GetCitizenFromCacheOnly(ctx, "03561350712")
	if err != nil {
		t.Errorf("GetCitizenFromCacheOnly() error = %v", err)
	}

	if result.CPF != "03561350712" {
		t.Errorf("GetCitizenFromCacheOnly() CPF = %v, want 03561350712", result.CPF)
	}
}

func TestGetCitizenFromCacheOnly_ReadCache(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert citizen into MongoDB
	nome := "João da Silva"
	citizen := models.Citizen{
		CPF:  "03561350712",
		Nome: &nome,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert citizen: %v", err)
	}

	// Get citizen (populates read cache)
	_, err = service.GetCitizen(ctx, "03561350712")
	if err != nil {
		t.Fatalf("GetCitizen() error = %v", err)
	}

	// Should retrieve from read cache
	result, err := service.GetCitizenFromCacheOnly(ctx, "03561350712")
	if err != nil {
		t.Errorf("GetCitizenFromCacheOnly() error = %v", err)
	}

	if result.CPF != "03561350712" {
		t.Errorf("GetCitizenFromCacheOnly() CPF = %v, want 03561350712", result.CPF)
	}
}

func TestGetCitizenFromCacheOnly_NotFound(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get from cache when not cached
	_, err := service.GetCitizenFromCacheOnly(ctx, "99999999999")
	if err == nil {
		t.Error("GetCitizenFromCacheOnly() should return error when not in cache")
	}
}

func TestIsCitizenInCache_WriteBuffer(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Initially not in cache
	if service.IsCitizenInCache(ctx, "03561350712") {
		t.Error("Citizen should not be in cache initially")
	}

	// Update citizen (writes to write buffer)
	nome := "João da Silva"
	citizen := &models.Citizen{
		CPF:  "03561350712",
		Nome: &nome,
	}

	err := service.UpdateCitizen(ctx, "03561350712", citizen)
	if err != nil {
		t.Fatalf("UpdateCitizen() error = %v", err)
	}

	// Should now be in cache
	if !service.IsCitizenInCache(ctx, "03561350712") {
		t.Error("Citizen should be in write buffer")
	}
}

func TestIsCitizenInCache_ReadCache(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert citizen into MongoDB
	nome := "João da Silva"
	citizen := models.Citizen{
		CPF:  "03561350712",
		Nome: &nome,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert citizen: %v", err)
	}

	// Get citizen (populates read cache)
	_, err = service.GetCitizen(ctx, "03561350712")
	if err != nil {
		t.Fatalf("GetCitizen() error = %v", err)
	}

	// Should be in cache
	if !service.IsCitizenInCache(ctx, "03561350712") {
		t.Error("Citizen should be in read cache")
	}
}

func TestIsCitizenInCache_NotFound(t *testing.T) {
	service, cleanup := setupCitizenCacheTest(t)
	defer cleanup()

	ctx := context.Background()

	// Check non-existent citizen
	if service.IsCitizenInCache(ctx, "99999999999") {
		t.Error("Non-existent citizen should not be in cache")
	}
}
