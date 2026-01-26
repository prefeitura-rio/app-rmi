package services

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/redis/go-redis/v9"
)

// setupCacheServiceTest initializes test environment and returns service with cleanup function
func setupCacheServiceTest(t *testing.T) (*CacheService, func()) {
	// Ensure test environment is set up (uses common_test.go)
	setupTestEnvironment()

	if config.MongoDB == nil {
		t.Skip("Skipping cache service tests: MongoDB not available")
	}

	if config.Redis == nil {
		t.Skip("Skipping cache service tests: Redis not available")
	}

	ctx := context.Background()

	// Create service
	service := NewCacheService()

	return service, func() {
		// Clean up Redis - remove all test keys
		patterns := []string{
			"citizen:*",
			"self_declared:*",
			"sync:queue:*",
			"sync:dlq:*",
			"phone_mapping:*",
			"user_config:*",
			"opt_in_history:*",
			"beta_group:*",
			"phone_verification:*",
			"maintenance_request:*",
		}

		for _, pattern := range patterns {
			keys, _ := config.Redis.Keys(ctx, pattern).Result()
			if len(keys) > 0 {
				config.Redis.Del(ctx, keys...)
			}
		}

		// Clean up MongoDB - drop only test collections
		collections := []string{
			"test_citizens",
			"test_self_declared",
			"phone_mappings",
			"user_configs",
			"opt_in_history",
			"beta_groups",
			"phone_verifications",
			"maintenance_requests",
		}

		for _, collName := range collections {
			config.MongoDB.Collection(collName).Drop(ctx)
		}
	}
}

func TestNewCacheService(t *testing.T) {
	service := NewCacheService()
	if service == nil {
		t.Error("NewCacheService() returned nil")
	}

	if service.citizenService == nil {
		t.Error("NewCacheService() citizenService is nil")
	}
}

func TestUpdateSelfDeclaredAddress(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	endereco := &models.Endereco{}

	err := service.UpdateSelfDeclaredAddress(ctx, "03561350712", endereco)
	if err != nil {
		t.Errorf("UpdateSelfDeclaredAddress() error = %v", err)
	}
	// Success - data was written to cache (verified by log output)
}

func TestUpdateSelfDeclaredEmail(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	emailData := &models.Email{}

	err := service.UpdateSelfDeclaredEmail(ctx, "03561350712", emailData)
	if err != nil {
		t.Errorf("UpdateSelfDeclaredEmail() error = %v", err)
	}
}

func TestUpdateSelfDeclaredPhone(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	telefone := &models.Telefone{}

	err := service.UpdateSelfDeclaredPhone(ctx, "03561350712", telefone)
	if err != nil {
		t.Errorf("UpdateSelfDeclaredPhone() error = %v", err)
	}
}

func TestUpdateSelfDeclaredRaca(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.UpdateSelfDeclaredRaca(ctx, "03561350712", "Branca")
	if err != nil {
		t.Errorf("UpdateSelfDeclaredRaca() error = %v", err)
	}
}

func TestUpdateSelfDeclaredNomeExibicao(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.UpdateSelfDeclaredNomeExibicao(ctx, "03561350712", "João Silva")
	if err != nil {
		t.Errorf("UpdateSelfDeclaredNomeExibicao() error = %v", err)
	}
}

func TestUpdateSelfDeclaredGenero(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.UpdateSelfDeclaredGenero(ctx, "03561350712", "Masculino")
	if err != nil {
		t.Errorf("UpdateSelfDeclaredGenero() error = %v", err)
	}
}

func TestUpdateSelfDeclaredRendaFamiliar(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.UpdateSelfDeclaredRendaFamiliar(ctx, "03561350712", "2 a 4 salários mínimos")
	if err != nil {
		t.Errorf("UpdateSelfDeclaredRendaFamiliar() error = %v", err)
	}
}

func TestUpdateSelfDeclaredEscolaridade(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.UpdateSelfDeclaredEscolaridade(ctx, "03561350712", "Ensino Superior Completo")
	if err != nil {
		t.Errorf("UpdateSelfDeclaredEscolaridade() error = %v", err)
	}
}

func TestUpdateSelfDeclaredDeficiencia(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	err := service.UpdateSelfDeclaredDeficiencia(ctx, "03561350712", "Nenhuma")
	if err != nil {
		t.Errorf("UpdateSelfDeclaredDeficiencia() error = %v", err)
	}
}

func TestUpdateUserConfig(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	userConfig := &models.UserConfig{
		CPF: "03561350712",
	}

	err := service.UpdateUserConfig(ctx, "user123", userConfig)
	if err != nil {
		t.Errorf("UpdateUserConfig() error = %v", err)
	}
}

func TestUpdateOptInHistory(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	optInHistory := &models.OptInHistory{
		CPF:       "03561350712",
		Timestamp: time.Now(),
	}

	err := service.UpdateOptInHistory(ctx, "history123", optInHistory)
	if err != nil {
		t.Errorf("UpdateOptInHistory() error = %v", err)
	}
}

func TestUpdatePhoneMapping(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	phoneMapping := &models.PhoneCPFMapping{
		PhoneNumber: "5521987654321",
		CPF:         "03561350712",
		Status:      models.MappingStatusActive,
	}

	err := service.UpdatePhoneMapping(ctx, "5521987654321", phoneMapping)
	if err != nil {
		t.Errorf("UpdatePhoneMapping() error = %v", err)
	}
}

func TestUpdateBetaGroup(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	betaGroup := &models.BetaGroup{
		Name: "Test Group",
	}

	err := service.UpdateBetaGroup(ctx, "group123", betaGroup)
	if err != nil {
		t.Errorf("UpdateBetaGroup() error = %v", err)
	}
}

func TestUpdatePhoneVerification(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	verification := &models.PhoneVerification{
		PhoneNumber: "5521987654321",
		Code:        "123456",
		ExpiresAt:   time.Now().Add(5 * time.Minute),
	}

	err := service.UpdatePhoneVerification(ctx, "5521987654321", verification)
	if err != nil {
		t.Errorf("UpdatePhoneVerification() error = %v", err)
	}
}

func TestUpdateMaintenanceRequest(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	request := &models.MaintenanceRequest{
		CPF: "03561350712",
	}

	err := service.UpdateMaintenanceRequest(ctx, "request123", request)
	if err != nil {
		t.Errorf("UpdateMaintenanceRequest() error = %v", err)
	}
}

func TestGetCitizen_Delegation(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// This should delegate to CitizenCacheService
	// Non-existent citizen should return error
	_, err := service.GetCitizen(ctx, "99999999999")
	if err == nil {
		t.Error("GetCitizen() should return error for non-existent citizen")
	}
}

func TestGetCitizenFromCacheOnly_Delegation(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Should delegate to CitizenCacheService
	_, err := service.GetCitizenFromCacheOnly(ctx, "99999999999")
	if err == nil {
		t.Error("GetCitizenFromCacheOnly() should return error when not in cache")
	}
}

func TestIsCitizenInCache_Delegation(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Should delegate to CitizenCacheService
	exists := service.IsCitizenInCache(ctx, "99999999999")
	if exists {
		t.Error("IsCitizenInCache() should return false for non-existent citizen")
	}
}

func TestDeleteCitizen_Delegation(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Should delegate to CitizenCacheService
	err := service.DeleteCitizen(ctx, "99999999999")
	if err != nil {
		t.Errorf("DeleteCitizen() error = %v", err)
	}
}

func TestGetQueueDepth(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create raw Redis client for test setup
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rawClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Initially should be 0
	depth, err := service.GetQueueDepth(ctx, "citizen")
	if err != nil {
		t.Errorf("GetQueueDepth() error = %v", err)
	}

	if depth != 0 {
		t.Errorf("GetQueueDepth() = %v, want 0 (empty queue)", depth)
	}

	// Add items to queue
	queueKey := "sync:queue:citizen"
	rawClient.RPush(ctx, queueKey, "item1", "item2", "item3")

	depth, err = service.GetQueueDepth(ctx, "citizen")
	if err != nil {
		t.Errorf("GetQueueDepth() error = %v", err)
	}

	if depth != 3 {
		t.Errorf("GetQueueDepth() = %v, want 3", depth)
	}
}

func TestGetDLQDepth(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create raw Redis client for test setup
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rawClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Initially should be 0
	depth, err := service.GetDLQDepth(ctx, "citizen")
	if err != nil {
		t.Errorf("GetDLQDepth() error = %v", err)
	}

	if depth != 0 {
		t.Errorf("GetDLQDepth() = %v, want 0 (empty DLQ)", depth)
	}

	// Add items to DLQ
	dlqKey := "sync:dlq:citizen"
	rawClient.RPush(ctx, dlqKey, "failed1", "failed2")

	depth, err = service.GetDLQDepth(ctx, "citizen")
	if err != nil {
		t.Errorf("GetDLQDepth() error = %v", err)
	}

	if depth != 2 {
		t.Errorf("GetDLQDepth() = %v, want 2", depth)
	}
}

func TestGetCacheStats(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create raw Redis client for test setup
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rawClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Add some test data to queues
	rawClient.RPush(ctx, "sync:queue:citizen", "item1", "item2")
	rawClient.RPush(ctx, "sync:dlq:phone_mapping", "failed1")
	config.Redis.Set(ctx, "citizen:write:test1", "data", 0)
	config.Redis.Set(ctx, "citizen:write:test2", "data", 0)
	config.Redis.Set(ctx, "citizen:cache:test3", "data", 0)

	stats := service.GetCacheStats(ctx)

	if stats == nil {
		t.Fatal("GetCacheStats() returned nil")
	}

	// Check queue depth for citizen
	if qd, ok := stats["queue_depth_citizen"].(int64); ok {
		if qd != 2 {
			t.Errorf("queue_depth_citizen = %v, want 2", qd)
		}
	} else {
		t.Error("queue_depth_citizen not found in stats")
	}

	// Check DLQ depth for phone_mapping
	if dlq, ok := stats["dlq_depth_phone_mapping"].(int64); ok {
		if dlq != 1 {
			t.Errorf("dlq_depth_phone_mapping = %v, want 1", dlq)
		}
	} else {
		t.Error("dlq_depth_phone_mapping not found in stats")
	}

	// Check write buffer count
	if wb, ok := stats["write_buffer_citizen"].(int); ok {
		if wb != 2 {
			t.Errorf("write_buffer_citizen = %v, want 2", wb)
		}
	} else {
		t.Error("write_buffer_citizen not found in stats")
	}

	// Check read cache count
	if rc, ok := stats["read_cache_citizen"].(int); ok {
		if rc != 1 {
			t.Errorf("read_cache_citizen = %v, want 1", rc)
		}
	} else {
		t.Error("read_cache_citizen not found in stats")
	}
}

func TestGetCacheStats_Empty(t *testing.T) {
	service, cleanup := setupCacheServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	stats := service.GetCacheStats(ctx)

	if stats == nil {
		t.Fatal("GetCacheStats() returned nil")
	}

	// All queue depths should be 0
	if qd, ok := stats["queue_depth_citizen"].(int64); ok {
		if qd != 0 {
			t.Errorf("queue_depth_citizen = %v, want 0", qd)
		}
	}

	// All write buffers should be 0
	if wb, ok := stats["write_buffer_citizen"].(int); ok {
		if wb != 0 {
			t.Errorf("write_buffer_citizen = %v, want 0", wb)
		}
	}
}
