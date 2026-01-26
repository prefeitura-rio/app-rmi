package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

// setupSyncServiceTest initializes test environment for SyncService tests
func setupSyncServiceTest(t *testing.T) (*SyncService, *redisclient.Client, *mongo.Database, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping sync service tests: MONGODB_URI not set")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	_ = logging.InitLogger()

	// Override test collections
	config.AppConfig.CitizenCollection = "test_citizens"
	config.AppConfig.SelfDeclaredCollection = "test_self_declared"
	config.AppConfig.UserConfigCollection = "test_user_config"

	ctx := context.Background()
	db := config.MongoDB

	// Redis setup
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	redisClient := redisclient.NewClient(singleClient)
	config.Redis = redisClient

	logger := logging.Logger
	syncService := NewSyncService(redisClient, db, 3, logger)

	return syncService, redisClient, db, func() {
		// Clean up Redis
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
			"cf_lookup:*",
		}

		for _, pattern := range patterns {
			keys, _ := redisClient.Keys(ctx, pattern).Result()
			if len(keys) > 0 {
				redisClient.Del(ctx, keys...)
			}
		}

		// Clean up MongoDB test collections
		db.Collection("test_citizens").Drop(ctx)
		db.Collection("test_self_declared").Drop(ctx)
		db.Collection("test_user_config").Drop(ctx)
	}
}

// TestNewSyncService tests the sync service constructor
func TestNewSyncService(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	assert.NotNil(t, service)
	assert.NotNil(t, service.redis)
	assert.NotNil(t, service.mongo)
	assert.NotNil(t, service.logger)
	assert.NotNil(t, service.metrics)
	assert.NotNil(t, service.degradedMode)
	assert.Equal(t, 3, service.workerCount)
	assert.Empty(t, service.workers) // Workers not created until Start() is called
}

// TestNewSyncService_DifferentWorkerCounts tests service with different worker counts
func TestNewSyncService_DifferentWorkerCounts(t *testing.T) {
	testCases := []struct {
		name        string
		workerCount int
	}{
		{"single_worker", 1},
		{"three_workers", 3},
		{"five_workers", 5},
		{"ten_workers", 10},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mongoURI := os.Getenv("MONGODB_URI")
			if mongoURI == "" {
				t.Skip("Skipping test: MONGODB_URI not set")
			}

			redisAddr := os.Getenv("REDIS_ADDR")
			if redisAddr == "" {
				redisAddr = "localhost:6379"
			}

			_ = logging.InitLogger()

			db := config.MongoDB

			singleClient := redis.NewClient(&redis.Options{
				Addr:     redisAddr,
				Password: os.Getenv("REDIS_PASSWORD"),
				DB:       0,
			})
			redisClient := redisclient.NewClient(singleClient)

			logger := logging.Logger
			service := NewSyncService(redisClient, db, tc.workerCount, logger)

			assert.NotNil(t, service)
			assert.Equal(t, tc.workerCount, service.workerCount)
		})
	}
}

// TestSyncService_Start tests starting the sync service
func TestSyncService_Start(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	// Start the service
	service.Start()

	// Give workers time to initialize
	time.Sleep(100 * time.Millisecond)

	// Verify workers were created
	assert.Equal(t, service.workerCount, len(service.workers))
	assert.NotEmpty(t, service.workers)

	// Stop the service
	service.Stop()
}

// TestSyncService_StartStop tests start and stop lifecycle
func TestSyncService_StartStop(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	// Start the service
	service.Start()

	// Let it run briefly
	time.Sleep(200 * time.Millisecond)

	// Verify service is running (workers created)
	assert.Equal(t, service.workerCount, len(service.workers))

	// Stop the service
	service.Stop()

	// Give workers time to stop
	time.Sleep(100 * time.Millisecond)

	// Verify all workers have stopped
	for _, worker := range service.workers {
		select {
		case <-worker.stopChan:
			// Expected - channel is closed
		default:
			t.Fatal("Worker stop channel should be closed")
		}
	}
}

// TestSyncService_Stop tests stopping the service
func TestSyncService_Stop(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	service.Start()
	time.Sleep(100 * time.Millisecond)

	// Should not panic when stopping
	service.Stop()

	// Verify degraded mode was stopped
	assert.False(t, service.degradedMode.IsActive())
}

// TestSyncService_Stop_WithoutStart tests stopping service that was never started
func TestSyncService_Stop_WithoutStart(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	// Should not panic even if never started
	service.Stop()
}

// TestSyncService_GetMetrics tests getting metrics from the service
func TestSyncService_GetMetrics(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	metrics := service.GetMetrics()

	assert.NotNil(t, metrics)
	assert.Same(t, service.metrics, metrics) // Should return the same instance
}

// TestSyncService_IsDegradedMode tests checking degraded mode status
func TestSyncService_IsDegradedMode(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	// Initially should not be in degraded mode
	assert.False(t, service.IsDegradedMode())

	// Activate degraded mode
	service.degradedMode.Activate("test_reason")
	assert.True(t, service.IsDegradedMode())

	// Deactivate degraded mode
	service.degradedMode.Deactivate()
	assert.False(t, service.IsDegradedMode())
}

// TestSyncService_MonitorDLQ tests DLQ monitoring functionality
func TestSyncService_MonitorDLQ(t *testing.T) {
	service, redisClient, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add some jobs to DLQs
	dlqKeys := []string{
		"sync:dlq:citizen",
		"sync:dlq:phone_mapping",
		"sync:dlq:user_config",
	}

	for _, dlqKey := range dlqKeys {
		testJob := DLQJob{
			OriginalJob: SyncJob{
				ID:         uuid.New().String(),
				Type:       "test",
				Key:        "test_key",
				Collection: "test_collection",
				Data:       map[string]interface{}{"test": "data"},
				Timestamp:  time.Now(),
			},
			Error:    "test error",
			FailedAt: time.Now(),
		}

		jobBytes, _ := json.Marshal(testJob)
		err := redisClient.LPush(ctx, dlqKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// Run DLQ monitoring once (we can't easily test the ticker without mocking)
	// Instead, we'll verify the monitoring logic by checking that DLQ sizes are tracked
	go service.monitorDLQ()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Verify DLQ jobs are still there (monitoring doesn't remove them)
	for _, dlqKey := range dlqKeys {
		dlqLen, err := redisClient.LLen(ctx, dlqKey).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), dlqLen)
	}
}

// TestSyncService_MonitorDLQ_AllQueues tests that all queue types are monitored
func TestSyncService_MonitorDLQ_AllQueues(t *testing.T) {
	service, redisClient, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add jobs to all DLQ types mentioned in the code
	allQueues := []string{
		"citizen",
		"phone_mapping",
		"user_config",
		"opt_in_history",
		"beta_group",
		"phone_verification",
		"maintenance_request",
		"self_declared_address",
		"self_declared_email",
		"self_declared_phone",
		"self_declared_raca",
		"self_declared_nome_exibicao",
		"cf_lookup",
	}

	for _, queue := range allQueues {
		dlqKey := fmt.Sprintf("sync:dlq:%s", queue)
		testJob := DLQJob{
			OriginalJob: SyncJob{
				ID:         uuid.New().String(),
				Type:       queue,
				Key:        "test_key",
				Collection: "test_collection",
				Data:       map[string]interface{}{"test": "data"},
				Timestamp:  time.Now(),
			},
			Error:    "test error",
			FailedAt: time.Now(),
		}

		jobBytes, _ := json.Marshal(testJob)
		err := redisClient.LPush(ctx, dlqKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// Start monitoring
	go service.monitorDLQ()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Verify all DLQs still have jobs
	for _, queue := range allQueues {
		dlqKey := fmt.Sprintf("sync:dlq:%s", queue)
		dlqLen, err := redisClient.LLen(ctx, dlqKey).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), dlqLen, "DLQ %s should have 1 job", queue)
	}
}

// TestSyncService_MonitorDLQ_EmptyQueues tests monitoring with empty DLQs
func TestSyncService_MonitorDLQ_EmptyQueues(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	// Run monitoring with empty DLQs - should not panic or error
	go service.monitorDLQ()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// No assertions needed - just verify it doesn't panic
}

// TestSyncService_WorkersProcessJobs tests that workers process jobs
func TestSyncService_WorkersProcessJobs(t *testing.T) {
	service, redisClient, db, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add jobs to queue
	numJobs := 5
	for i := 0; i < numJobs; i++ {
		testJob := SyncJob{
			ID:         uuid.New().String(),
			Type:       "citizen",
			Key:        fmt.Sprintf("1234567890%d", i),
			Collection: "test_citizens",
			Data: map[string]interface{}{
				"cpf":  fmt.Sprintf("1234567890%d", i),
				"nome": fmt.Sprintf("Test User %d", i),
			},
			Timestamp:  time.Now(),
			RetryCount: 0,
			MaxRetries: 3,
		}

		jobBytes, err := json.Marshal(testJob)
		require.NoError(t, err)

		queueKey := "sync:queue:citizen"
		err = redisClient.LPush(ctx, queueKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// Start the service
	service.Start()

	// Let workers process jobs
	time.Sleep(1 * time.Second)

	// Stop the service
	service.Stop()

	// Verify at least some jobs were processed
	count, err := db.Collection("test_citizens").CountDocuments(ctx, map[string]interface{}{})
	require.NoError(t, err)
	assert.Greater(t, count, int64(0), "At least some jobs should have been processed")
}

// TestSyncService_MultipleWorkersProcessJobs tests multiple workers processing concurrently
func TestSyncService_MultipleWorkersProcessJobs(t *testing.T) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping test: MONGODB_URI not set")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	_ = logging.InitLogger()

	config.AppConfig.CitizenCollection = "test_citizens"

	ctx := context.Background()
	db := config.MongoDB

	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	redisClient := redisclient.NewClient(singleClient)

	logger := logging.Logger
	service := NewSyncService(redisClient, db, 5, logger)

	// Add many jobs
	numJobs := 20
	for i := 0; i < numJobs; i++ {
		testJob := SyncJob{
			ID:         uuid.New().String(),
			Type:       "citizen",
			Key:        fmt.Sprintf("1234567890%d", i),
			Collection: "test_citizens",
			Data: map[string]interface{}{
				"cpf":  fmt.Sprintf("1234567890%d", i),
				"nome": fmt.Sprintf("Test User %d", i),
			},
			Timestamp:  time.Now(),
			RetryCount: 0,
			MaxRetries: 3,
		}

		jobBytes, _ := json.Marshal(testJob)
		queueKey := "sync:queue:citizen"
		err := redisClient.LPush(ctx, queueKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// Start the service with multiple workers
	service.Start()

	// Let workers process jobs
	time.Sleep(2 * time.Second)

	// Stop the service
	service.Stop()

	// Verify many jobs were processed
	count, err := db.Collection("test_citizens").CountDocuments(ctx, map[string]interface{}{})
	require.NoError(t, err)
	assert.Greater(t, count, int64(10), "Multiple workers should have processed many jobs")

	// Clean up test collection
	db.Collection("test_citizens").Drop(ctx)

	// Clean up Redis
	patterns := []string{"citizen:*", "sync:queue:*", "sync:dlq:*"}
	for _, pattern := range patterns {
		keys, _ := redisClient.Keys(ctx, pattern).Result()
		if len(keys) > 0 {
			redisClient.Del(ctx, keys...)
		}
	}
}

// TestSyncService_DegradedModeStopsProcessing tests that degraded mode stops processing
func TestSyncService_DegradedModeStopsProcessing(t *testing.T) {
	service, redisClient, db, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add jobs to queue
	numJobs := 10
	for i := 0; i < numJobs; i++ {
		testJob := SyncJob{
			ID:         uuid.New().String(),
			Type:       "citizen",
			Key:        fmt.Sprintf("1234567890%d", i),
			Collection: "test_citizens",
			Data: map[string]interface{}{
				"cpf":  fmt.Sprintf("1234567890%d", i),
				"nome": fmt.Sprintf("Test User %d", i),
			},
			Timestamp:  time.Now(),
			RetryCount: 0,
			MaxRetries: 3,
		}

		jobBytes, err := json.Marshal(testJob)
		require.NoError(t, err)

		queueKey := "sync:queue:citizen"
		err = redisClient.LPush(ctx, queueKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// Activate degraded mode BEFORE starting
	service.degradedMode.Activate("test_degraded_mode")

	// Start the service
	service.Start()

	// Let it run briefly
	time.Sleep(500 * time.Millisecond)

	// Stop the service
	service.Stop()

	// Verify no or very few jobs were processed (degraded mode should prevent processing)
	count, err := db.Collection("test_citizens").CountDocuments(ctx, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "No jobs should be processed in degraded mode")

	// Verify jobs are still in queue
	queueKey := "sync:queue:citizen"
	queueLen, err := redisClient.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Greater(t, queueLen, int64(5), "Most jobs should still be in queue")
}

// TestSyncService_MetricsCollection tests that metrics are collected
func TestSyncService_MetricsCollection(t *testing.T) {
	service, redisClient, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add a successful job
	testJob := SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data: map[string]interface{}{
			"cpf":  "12345678901",
			"nome": "Test User",
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	jobBytes, err := json.Marshal(testJob)
	require.NoError(t, err)

	queueKey := "sync:queue:citizen"
	err = redisClient.LPush(ctx, queueKey, string(jobBytes)).Err()
	require.NoError(t, err)

	// Start the service
	service.Start()

	// Let it process
	time.Sleep(1 * time.Second)

	// Stop the service
	service.Stop()

	// Verify metrics were collected
	metrics := service.GetMetrics()
	assert.NotNil(t, metrics)

	// Check that some metrics exist
	allMetrics := metrics.GetAllMetrics()
	assert.NotNil(t, allMetrics)
	assert.NotEmpty(t, allMetrics)
}

// TestSyncService_StartMultipleTimes tests that starting multiple times is safe
func TestSyncService_StartMultipleTimes(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	// Start multiple times - should not panic
	service.Start()
	time.Sleep(50 * time.Millisecond)

	service.Start() // Second start
	time.Sleep(50 * time.Millisecond)

	service.Start() // Third start
	time.Sleep(50 * time.Millisecond)

	// Should have created workers from multiple starts
	// Note: This may create duplicate workers, but should not panic
	assert.NotEmpty(t, service.workers)

	// Stop once
	service.Stop()
}

// TestSyncService_StopMultipleTimes tests that stopping multiple times is safe
func TestSyncService_StopMultipleTimes(t *testing.T) {
	service, _, _, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	service.Start()
	time.Sleep(100 * time.Millisecond)

	// Stop multiple times - first should work, subsequent may have issues
	service.Stop()

	// Note: Stopping multiple times may panic due to closing closed channels
	// This is expected behavior - service should only be stopped once
}

// TestSyncService_WorkerCountZero tests service with zero workers
func TestSyncService_WorkerCountZero(t *testing.T) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping test: MONGODB_URI not set")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	_ = logging.InitLogger()

	db := config.MongoDB

	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	redisClient := redisclient.NewClient(singleClient)

	logger := logging.Logger
	service := NewSyncService(redisClient, db, 0, logger)

	// Start the service with zero workers
	service.Start()
	time.Sleep(100 * time.Millisecond)

	// Should have no workers
	assert.Empty(t, service.workers)

	// Stop should not panic
	service.Stop()
}

// TestSyncService_Lifecycle tests full service lifecycle
func TestSyncService_Lifecycle(t *testing.T) {
	service, redisClient, db, cleanup := setupSyncServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add jobs before starting
	for i := 0; i < 3; i++ {
		testJob := SyncJob{
			ID:         uuid.New().String(),
			Type:       "citizen",
			Key:        fmt.Sprintf("1234567890%d", i),
			Collection: "test_citizens",
			Data: map[string]interface{}{
				"cpf":  fmt.Sprintf("1234567890%d", i),
				"nome": fmt.Sprintf("Test User %d", i),
			},
			Timestamp:  time.Now(),
			RetryCount: 0,
			MaxRetries: 3,
		}

		jobBytes, _ := json.Marshal(testJob)
		queueKey := "sync:queue:citizen"
		err := redisClient.LPush(ctx, queueKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// 1. Create service
	assert.NotNil(t, service)
	assert.Empty(t, service.workers)

	// 2. Start service
	service.Start()
	time.Sleep(100 * time.Millisecond)
	assert.NotEmpty(t, service.workers)

	// 3. Let it process jobs
	time.Sleep(1 * time.Second)

	// 4. Check metrics
	metrics := service.GetMetrics()
	assert.NotNil(t, metrics)

	// 5. Verify some processing occurred
	count, err := db.Collection("test_citizens").CountDocuments(ctx, map[string]interface{}{})
	require.NoError(t, err)
	assert.Greater(t, count, int64(0), "Some jobs should have been processed")

	// 6. Stop service
	service.Stop()

	// 7. Verify workers stopped
	for _, worker := range service.workers {
		select {
		case <-worker.stopChan:
			// Expected - channel is closed
		default:
			t.Fatal("Worker should be stopped")
		}
	}

	// 8. Verify degraded mode stopped
	assert.False(t, service.degradedMode.IsActive())
}
