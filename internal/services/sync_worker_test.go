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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// setupSyncWorkerTest initializes MongoDB and Redis for testing
func setupSyncWorkerTest(t *testing.T) (*SyncWorker, *mongo.Database, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping sync worker tests: MONGODB_URI not set")
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
	config.AppConfig.CitizenCollection = "test_citizens"
	config.AppConfig.SelfDeclaredCollection = "test_self_declared"
	config.AppConfig.UserConfigCollection = "test_user_config"

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

	db := client.Database("rmi_test_sync_worker")
	config.MongoDB = db

	// Redis setup
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	redisClient := redisclient.NewClient(singleClient)
	config.Redis = redisClient

	// Test Redis connection
	err = redisClient.Ping(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Create services
	metrics := NewMetrics()
	logger := logging.Logger
	degradedMode := NewDegradedMode(redisClient, db, metrics)

	worker := NewSyncWorker(redisClient, db, 1, logger, metrics, degradedMode)

	return worker, db, func() {
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
			"cf_lookup:*",
		}

		for _, pattern := range patterns {
			keys, _ := redisClient.Keys(ctx, pattern).Result()
			if len(keys) > 0 {
				redisClient.Del(ctx, keys...)
			}
		}

		// Clean up MongoDB
		db.Drop(ctx)
		client.Disconnect(ctx)
	}
}

// TestNewSyncWorker tests the constructor
func TestNewSyncWorker(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	assert.NotNil(t, worker)
	assert.Equal(t, 1, worker.id)
	assert.NotNil(t, worker.redis)
	assert.NotNil(t, worker.mongo)
	assert.NotNil(t, worker.logger)
	assert.NotNil(t, worker.metrics)
	assert.NotNil(t, worker.degradedMode)
	assert.NotNil(t, worker.stopChan)
	assert.NotEmpty(t, worker.queues)

	// Verify all expected queues are present
	expectedQueues := []string{
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
		"self_declared_genero",
		"self_declared_renda_familiar",
		"self_declared_escolaridade",
		"self_declared_deficiencia",
		"cf_lookup",
	}

	assert.Equal(t, len(expectedQueues), len(worker.queues))
	for _, expectedQueue := range expectedQueues {
		assert.Contains(t, worker.queues, expectedQueue)
	}
}

// TestSyncWorker_StartStop tests worker start and stop
func TestSyncWorker_StartStop(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	// Start worker in goroutine
	go worker.Start()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop worker
	worker.Stop()

	// Wait a bit to ensure it stopped
	time.Sleep(100 * time.Millisecond)

	// Verify worker stopped (stop channel should be closed)
	select {
	case <-worker.stopChan:
		// Expected - channel is closed
	default:
		t.Fatal("Worker stop channel should be closed")
	}
}

// TestSyncWorker_GetJobNonBlocking_EmptyQueue tests getting job from empty queue
func TestSyncWorker_GetJobNonBlocking_EmptyQueue(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	job, err := worker.getJobNonBlocking("citizen")
	assert.NoError(t, err)
	assert.Nil(t, job)
}

// TestSyncWorker_GetJobNonBlocking_ValidJob tests getting valid job from queue
func TestSyncWorker_GetJobNonBlocking_ValidJob(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a test job
	testJob := SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "citizens",
		Data: map[string]interface{}{
			"cpf":  "12345678901",
			"nome": "Test User",
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	// Push job to queue
	jobBytes, err := json.Marshal(testJob)
	require.NoError(t, err)

	queueKey := "sync:queue:citizen"
	err = worker.redis.LPush(ctx, queueKey, string(jobBytes)).Err()
	require.NoError(t, err)

	// Get job from queue
	job, err := worker.getJobNonBlocking("citizen")
	assert.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, testJob.ID, job.ID)
	assert.Equal(t, testJob.Type, job.Type)
	assert.Equal(t, testJob.Key, job.Key)
	assert.Equal(t, testJob.Collection, job.Collection)
}

// TestSyncWorker_GetJobNonBlocking_InvalidJSON tests getting invalid JSON from queue
func TestSyncWorker_GetJobNonBlocking_InvalidJSON(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Push invalid JSON to queue
	queueKey := "sync:queue:citizen"
	err := worker.redis.LPush(ctx, queueKey, "invalid json {{{").Err()
	require.NoError(t, err)

	// Get job from queue should fail
	job, err := worker.getJobNonBlocking("citizen")
	assert.Error(t, err)
	assert.Nil(t, job)
}

// TestSyncWorker_ProcessQueuesParallel_EmptyQueues tests processing empty queues
func TestSyncWorker_ProcessQueuesParallel_EmptyQueues(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	// Process queues - should not panic or error
	worker.processQueuesParallel()

	// No assertions needed - just verify it doesn't panic
}

// TestSyncWorker_ProcessQueuesParallel_DegradedMode tests skipping when in degraded mode
func TestSyncWorker_ProcessQueuesParallel_DegradedMode(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add job to queue
	testJob := SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "citizens",
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
	err = worker.redis.LPush(ctx, queueKey, string(jobBytes)).Err()
	require.NoError(t, err)

	// Activate degraded mode
	worker.degradedMode.Activate("test_reason")

	// Process queues - should skip all processing
	worker.processQueuesParallel()

	// Job should still be in queue (not processed)
	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen)
}

// TestSyncWorker_ProcessQueuesParallel_MaxJobsLimit tests max jobs per cycle limit
func TestSyncWorker_ProcessQueuesParallel_MaxJobsLimit(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add 5 jobs to queue (more than max 3 per cycle)
	for i := 0; i < 5; i++ {
		testJob := SyncJob{
			ID:         uuid.New().String(),
			Type:       "citizen",
			Key:        fmt.Sprintf("1234567890%d", i),
			Collection: "citizens",
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
		err = worker.redis.LPush(ctx, queueKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// Process queues - should process max 3 jobs per iteration
	// Since we have 11 queues and max 3 jobs total per processQueuesParallel,
	// we should process at most 3 jobs from the citizen queue
	worker.processQueuesParallel()

	// Should have at least some jobs left in queue (not all 5 should be processed)
	queueKey := "sync:queue:citizen"
	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Greater(t, queueLen, int64(0), "Should have some jobs remaining after processing")
}

// TestSyncWorker_SyncToMongoDB_CitizenSuccess tests successful citizen sync
func TestSyncWorker_SyncToMongoDB_CitizenSuccess(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
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

	err := worker.syncToMongoDB(testJob)
	assert.NoError(t, err)

	// Verify data was written to MongoDB
	var result bson.M
	err = db.Collection("test_citizens").FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "12345678901", result["cpf"])
	assert.Equal(t, "Test User", result["nome"])
}

// TestSyncWorker_SyncToMongoDB_PhoneMappingSuccess tests successful phone mapping sync
func TestSyncWorker_SyncToMongoDB_PhoneMappingSuccess(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "phone_mapping",
		Key:        "5521999999999",
		Collection: "phone_cpf_mappings",
		Data: map[string]interface{}{
			"phone_number": "5521999999999",
			"cpf":          "12345678901",
			"status":       "active",
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err := worker.syncToMongoDB(testJob)
	assert.NoError(t, err)

	// Verify data was written to MongoDB
	var result bson.M
	err = db.Collection("phone_cpf_mappings").FindOne(ctx, bson.M{"phone_number": "5521999999999"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "5521999999999", result["phone_number"])
	assert.Equal(t, "12345678901", result["cpf"])
	assert.Equal(t, "active", result["status"])
}

// TestSyncWorker_SyncToMongoDB_SelfDeclaredFieldSpecific tests field-specific self_declared updates
func TestSyncWorker_SyncToMongoDB_SelfDeclaredFieldSpecific(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// First, create a self_declared document with multiple fields
	collection := db.Collection("self_declared")
	_, err := collection.InsertOne(ctx, bson.M{
		"cpf":        "12345678901",
		"endereco":   "Old Address",
		"email":      "old@example.com",
		"telefone":   "5521888888888",
		"created_at": time.Now(),
	})
	require.NoError(t, err)

	// Now sync only the email field
	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "self_declared_email",
		Key:        "12345678901",
		Collection: "self_declared",
		Data: map[string]interface{}{
			"email":      "new@example.com",
			"updated_at": time.Now(),
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err = worker.syncToMongoDB(testJob)
	assert.NoError(t, err)

	// Verify only email was updated, other fields preserved
	var result bson.M
	err = collection.FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "new@example.com", result["email"])
	assert.Equal(t, "Old Address", result["endereco"])
	assert.Equal(t, "5521888888888", result["telefone"])
}

// TestSyncWorker_SyncToMongoDB_AllSelfDeclaredFields tests all self_declared field types
func TestSyncWorker_SyncToMongoDB_AllSelfDeclaredFields(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := db.Collection("self_declared")

	// Test all self_declared field types
	testCases := []struct {
		jobType    string
		fieldName  string
		fieldValue interface{}
	}{
		{"self_declared_address", "endereco", "Rua Test, 123"},
		{"self_declared_email", "email", "test@example.com"},
		{"self_declared_phone", "telefone", "5521999999999"},
		{"self_declared_raca", "raca", "parda"},
		{"self_declared_nome_exibicao", "nome_exibicao", "Test User"},
		{"self_declared_genero", "genero", "masculino"},
		{"self_declared_renda_familiar", "renda_familiar", "2-4 salários"},
		{"self_declared_escolaridade", "escolaridade", "superior completo"},
		{"self_declared_deficiencia", "deficiencia", false},
	}

	for _, tc := range testCases {
		t.Run(tc.jobType, func(t *testing.T) {
			cpf := fmt.Sprintf("1234567890%d", len(tc.jobType))

			testJob := &SyncJob{
				ID:         uuid.New().String(),
				Type:       tc.jobType,
				Key:        cpf,
				Collection: "self_declared",
				Data: map[string]interface{}{
					tc.fieldName: tc.fieldValue,
					"updated_at": time.Now(),
				},
				Timestamp:  time.Now(),
				RetryCount: 0,
				MaxRetries: 3,
			}

			err := worker.syncToMongoDB(testJob)
			assert.NoError(t, err)

			// Verify field was written
			var result bson.M
			err = collection.FindOne(ctx, bson.M{"cpf": cpf}).Decode(&result)
			require.NoError(t, err)
			assert.Equal(t, tc.fieldValue, result[tc.fieldName])
		})
	}
}

// TestSyncWorker_SyncToMongoDB_DuplicateKey tests handling duplicate key errors
func TestSyncWorker_SyncToMongoDB_DuplicateKey(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// First sync
	testJob := &SyncJob{
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

	err := worker.syncToMongoDB(testJob)
	assert.NoError(t, err)

	// Second sync with same key - should not error (upsert)
	testJob.Data = map[string]interface{}{
		"cpf":  "12345678901",
		"nome": "Updated User",
	}

	err = worker.syncToMongoDB(testJob)
	assert.NoError(t, err)

	// Verify data was updated
	var result bson.M
	err = db.Collection("test_citizens").FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Updated User", result["nome"])
}

// TestSyncWorker_SyncToMongoDB_InvalidData tests handling invalid data
func TestSyncWorker_SyncToMongoDB_InvalidData(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	// Create job with invalid data (circular reference would cause marshaling issues)
	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data:       make(chan int), // Channels can't be marshaled to JSON
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err := worker.syncToMongoDB(testJob)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal job data")
}

// TestSyncWorker_HandleSyncSuccess tests successful sync handling
func TestSyncWorker_HandleSyncSuccess(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
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

	// Create write buffer
	writeKey := fmt.Sprintf("%s:write:%s", testJob.Type, testJob.Key)
	dataBytes, _ := json.Marshal(testJob.Data)
	err := worker.redis.Set(ctx, writeKey, string(dataBytes), 1*time.Hour).Err()
	require.NoError(t, err)

	// Handle success
	worker.handleSyncSuccess(testJob)

	// Verify write buffer was cleaned up
	exists, err := worker.redis.Exists(ctx, writeKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)

	// Verify read cache was updated
	cacheKey := fmt.Sprintf("%s:cache:%s", testJob.Type, testJob.Key)
	cached, err := worker.redis.Get(ctx, cacheKey).Result()
	require.NoError(t, err)
	assert.NotEmpty(t, cached)

	// Verify TTL is set (should be 3 hours)
	ttl, err := worker.redis.TTL(ctx, cacheKey).Result()
	require.NoError(t, err)
	assert.True(t, ttl > 2*time.Hour && ttl <= 3*time.Hour)
}

// TestSyncWorker_HandleSyncFailure_Retry tests retry logic
func TestSyncWorker_HandleSyncFailure_Retry(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
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

	testErr := fmt.Errorf("test error")

	// Handle failure
	worker.handleSyncFailure(testJob, testErr)

	// Job should be re-queued
	queueKey := fmt.Sprintf("sync:queue:%s", testJob.Type)

	// Wait for requeue (has backoff delay)
	time.Sleep(100 * time.Millisecond)

	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen)

	// Verify retry count was incremented
	jobJSON, err := worker.redis.RPop(ctx, queueKey).Result()
	require.NoError(t, err)

	var requeuedJob SyncJob
	err = json.Unmarshal([]byte(jobJSON), &requeuedJob)
	require.NoError(t, err)
	assert.Equal(t, 1, requeuedJob.RetryCount)
}

// TestSyncWorker_HandleSyncFailure_MaxRetries tests DLQ movement
func TestSyncWorker_HandleSyncFailure_MaxRetries(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data: map[string]interface{}{
			"cpf":  "12345678901",
			"nome": "Test User",
		},
		Timestamp:  time.Now(),
		RetryCount: 3, // Already at max retries
		MaxRetries: 3,
	}

	testErr := fmt.Errorf("persistent error")

	// Handle failure
	worker.handleSyncFailure(testJob, testErr)

	// Job should be in DLQ
	dlqKey := fmt.Sprintf("sync:dlq:%s", testJob.Type)
	dlqLen, err := worker.redis.LLen(ctx, dlqKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), dlqLen)

	// Verify DLQ job structure
	dlqJobJSON, err := worker.redis.RPop(ctx, dlqKey).Result()
	require.NoError(t, err)

	var dlqJob DLQJob
	err = json.Unmarshal([]byte(dlqJobJSON), &dlqJob)
	require.NoError(t, err)
	assert.Equal(t, testJob.ID, dlqJob.OriginalJob.ID)
	assert.Equal(t, testErr.Error(), dlqJob.Error)
	assert.False(t, dlqJob.FailedAt.IsZero())
}

// TestSyncWorker_MoveToDLQ tests moving job to dead letter queue
func TestSyncWorker_MoveToDLQ(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data: map[string]interface{}{
			"cpf":  "12345678901",
			"nome": "Test User",
		},
		Timestamp:  time.Now(),
		RetryCount: 3,
		MaxRetries: 3,
	}

	testErr := fmt.Errorf("fatal error")

	worker.moveToDLQ(testJob, testErr)

	// Verify job in DLQ
	dlqKey := fmt.Sprintf("sync:dlq:%s", testJob.Type)
	dlqLen, err := worker.redis.LLen(ctx, dlqKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), dlqLen)
}

// TestSyncWorker_RequeueJob tests job requeuing with backoff
func TestSyncWorker_RequeueJob(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data: map[string]interface{}{
			"cpf":  "12345678901",
			"nome": "Test User",
		},
		Timestamp:  time.Now(),
		RetryCount: 1,
		MaxRetries: 3,
	}

	start := time.Now()
	worker.requeueJob(testJob)
	duration := time.Since(start)

	// Should have backoff delay of 5 seconds (1 * 5s)
	assert.True(t, duration >= 5*time.Second)

	// Verify job in queue
	queueKey := fmt.Sprintf("sync:queue:%s", testJob.Type)
	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen)
}

// TestSyncWorker_RequeueJob_BackoffCap tests backoff cap at 60 seconds
func TestSyncWorker_RequeueJob_BackoffCap(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data: map[string]interface{}{
			"cpf":  "12345678901",
			"nome": "Test User",
		},
		Timestamp:  time.Now(),
		RetryCount: 20, // Would calculate to 100 seconds, but should cap at 60
		MaxRetries: 25,
	}

	start := time.Now()
	worker.requeueJob(testJob)
	duration := time.Since(start)

	// Should be capped at 60 seconds
	assert.True(t, duration >= 60*time.Second)
	assert.True(t, duration < 65*time.Second)

	// Verify job in queue
	queueKey := fmt.Sprintf("sync:queue:%s", testJob.Type)
	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen)
}

// TestSyncWorker_ProcessJob_Success tests successful job processing
func TestSyncWorker_ProcessJob_Success(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
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

	worker.processJob(testJob)

	// Verify data in MongoDB
	var result bson.M
	err := db.Collection("test_citizens").FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "12345678901", result["cpf"])

	// Verify metrics
	assert.Greater(t, worker.metrics.syncOperations["citizen"], int64(0))
}

// TestSyncWorker_ProcessJob_Failure tests failed job processing
func TestSyncWorker_ProcessJob_Failure(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create job with invalid data
	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data:       make(chan int), // Can't be marshaled
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	worker.processJob(testJob)

	// Verify metrics show failure
	assert.Greater(t, worker.metrics.syncFailures["citizen"], int64(0))

	// Job should be requeued or in DLQ
	queueKey := fmt.Sprintf("sync:queue:%s", testJob.Type)

	// Wait for requeue
	time.Sleep(100 * time.Millisecond)

	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen)
}

// TestSyncWorker_HandleAvatarCleanup tests avatar cleanup job handling
func TestSyncWorker_HandleAvatarCleanup(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create test user configs with avatar references
	userConfigCollection := db.Collection(config.AppConfig.UserConfigCollection)
	_, err := userConfigCollection.InsertMany(ctx, []interface{}{
		bson.M{"cpf": "11111111111", "avatar_id": "avatar-to-delete", "created_at": time.Now()},
		bson.M{"cpf": "22222222222", "avatar_id": "avatar-to-delete", "created_at": time.Now()},
		bson.M{"cpf": "33333333333", "avatar_id": "other-avatar", "created_at": time.Now()},
	})
	require.NoError(t, err)

	// Create avatar cleanup job
	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "avatar_cleanup",
		Key:        "avatar-to-delete",
		Collection: "special",
		Data: map[string]interface{}{
			"type":      "avatar_cleanup",
			"avatar_id": "avatar-to-delete",
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err = worker.syncToMongoDB(testJob)
	assert.NoError(t, err)

	// Verify avatar_id was removed from affected users
	var user1, user2, user3 bson.M
	err = userConfigCollection.FindOne(ctx, bson.M{"cpf": "11111111111"}).Decode(&user1)
	require.NoError(t, err)
	_, hasAvatarID := user1["avatar_id"]
	assert.False(t, hasAvatarID, "avatar_id should be removed")

	err = userConfigCollection.FindOne(ctx, bson.M{"cpf": "22222222222"}).Decode(&user2)
	require.NoError(t, err)
	_, hasAvatarID = user2["avatar_id"]
	assert.False(t, hasAvatarID, "avatar_id should be removed")

	// User with different avatar should not be affected
	err = userConfigCollection.FindOne(ctx, bson.M{"cpf": "33333333333"}).Decode(&user3)
	require.NoError(t, err)
	assert.Equal(t, "other-avatar", user3["avatar_id"])
}

// TestSyncWorker_HandleAvatarCleanup_InvalidData tests invalid avatar cleanup data
func TestSyncWorker_HandleAvatarCleanup_InvalidData(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Missing avatar_id
	data := map[string]interface{}{
		"type": "avatar_cleanup",
	}

	err := worker.handleAvatarCleanup(ctx, data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid avatar_id")
}

// TestSyncWorker_HandleCFLookupJob tests CF lookup job handling
func TestSyncWorker_HandleCFLookupJob(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Note: CF lookup will fail because service is not initialized in tests
	// This is expected - we're testing the job handling, not the actual lookup
	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "cf_lookup",
		Key:        "12345678901",
		Collection: "cf_lookup",
		Data: map[string]interface{}{
			"cpf":     "12345678901",
			"address": "Rua Test, 123",
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err := worker.handleCFLookupJob(ctx, testJob)
	// Should error because CFLookupServiceInstance is nil in tests
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CF lookup service disabled")
}

// TestSyncWorker_HandleCFLookupJob_InvalidData tests invalid CF lookup data
func TestSyncWorker_HandleCFLookupJob_InvalidData(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Missing CPF
	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "cf_lookup",
		Key:        "",
		Collection: "cf_lookup",
		Data: map[string]interface{}{
			"address": "Rua Test, 123",
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err := worker.handleCFLookupJob(ctx, testJob)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing or invalid CPF")

	// Missing address
	testJob.Data = map[string]interface{}{
		"cpf": "12345678901",
	}

	err = worker.handleCFLookupJob(ctx, testJob)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing or invalid address")
}

// TestGetFieldNameFromJobType tests job type to field name mapping
func TestGetFieldNameFromJobType(t *testing.T) {
	testCases := []struct {
		jobType  string
		expected string
	}{
		{"self_declared_address", "endereco"},
		{"self_declared_email", "email"},
		{"self_declared_phone", "telefone"},
		{"self_declared_raca", "raca"},
		{"self_declared_nome_exibicao", "nome_exibicao"},
		{"self_declared_genero", "genero"},
		{"self_declared_renda_familiar", "renda_familiar"},
		{"self_declared_escolaridade", "escolaridade"},
		{"self_declared_deficiencia", "deficiencia"},
		{"unknown_type", ""},
		{"citizen", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.jobType, func(t *testing.T) {
			result := getFieldNameFromJobType(tc.jobType)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestSyncWorker_HandleSpecialJobTypes_NotSpecial tests non-special job handling
func TestSyncWorker_HandleSpecialJobTypes_NotSpecial(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
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

	err := worker.handleSpecialJobTypes(ctx, testJob)
	assert.Error(t, err)
	assert.Equal(t, "not_special_job", err.Error())
}

// TestSyncWorker_RoundRobinProcessing tests round-robin queue processing
func TestSyncWorker_RoundRobinProcessing(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add jobs to different queues
	queues := []string{"citizen", "phone_mapping", "user_config"}
	processedQueues := make(map[string]bool)

	for _, queue := range queues {
		testJob := SyncJob{
			ID:         uuid.New().String(),
			Type:       queue,
			Key:        fmt.Sprintf("key-%s", queue),
			Collection: fmt.Sprintf("test_%s", queue),
			Data: map[string]interface{}{
				"id":   fmt.Sprintf("key-%s", queue),
				"data": "test",
			},
			Timestamp:  time.Now(),
			RetryCount: 0,
			MaxRetries: 3,
		}

		jobBytes, _ := json.Marshal(testJob)
		queueKey := fmt.Sprintf("sync:queue:%s", queue)
		err := worker.redis.LPush(ctx, queueKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// Process queues - should process jobs from different queues
	worker.processQueuesParallel()

	// Check which queues were processed
	for _, queue := range queues {
		queueKey := fmt.Sprintf("sync:queue:%s", queue)
		queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
		require.NoError(t, err)
		if queueLen == 0 {
			processedQueues[queue] = true
		}
	}

	// At least one queue should have been processed
	assert.NotEmpty(t, processedQueues)
}

// TestSyncWorker_ConcurrentJobProcessing tests concurrent job processing
func TestSyncWorker_ConcurrentJobProcessing(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add multiple jobs across different queues to test concurrent processing
	numJobs := 6
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
		err := worker.redis.LPush(ctx, queueKey, string(jobBytes)).Err()
		require.NoError(t, err)
	}

	// Process all jobs (multiple cycles) - max 3 jobs per cycle, so need at least 2 cycles
	for i := 0; i < 3; i++ {
		worker.processQueuesParallel()
		time.Sleep(10 * time.Millisecond)
	}

	// Verify jobs were processed - with 3 cycles and max 3 jobs/cycle, should get all 6
	count, err := db.Collection("test_citizens").CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(3), "Should have processed multiple jobs")
}

// TestSyncWorker_MetricsCollection tests metrics are collected during processing
func TestSyncWorker_MetricsCollection(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add successful job
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

	jobBytes, _ := json.Marshal(testJob)
	queueKey := "sync:queue:citizen"
	err := worker.redis.LPush(ctx, queueKey, string(jobBytes)).Err()
	require.NoError(t, err)

	// Process job
	worker.processQueuesParallel()

	// Verify metrics were updated
	assert.Greater(t, worker.metrics.syncOperations["citizen"], int64(0))
}

// TestSyncWorker_MetricsFailureCollection tests failure metrics collection
func TestSyncWorker_MetricsFailureCollection(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	// Add job with invalid data
	testJob := SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data:       "invalid", // Invalid data type
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	jobBytes, _ := json.Marshal(testJob)
	queueKey := "sync:queue:citizen"
	err := worker.redis.LPush(ctx, queueKey, string(jobBytes)).Err()
	require.NoError(t, err)

	// Process job
	worker.processQueuesParallel()

	// Wait for requeue
	time.Sleep(100 * time.Millisecond)

	// Verify failure metrics were updated
	assert.Greater(t, worker.metrics.syncFailures["citizen"], int64(0))
}

// TestSyncWorker_DataValidation tests data validation during sync
func TestSyncWorker_DataValidation(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	testCases := []struct {
		name        string
		job         *SyncJob
		shouldError bool
	}{
		{
			name: "valid_citizen_data",
			job: &SyncJob{
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
			},
			shouldError: false,
		},
		{
			name: "invalid_data_type",
			job: &SyncJob{
				ID:         uuid.New().String(),
				Type:       "citizen",
				Key:        "12345678901",
				Collection: "test_citizens",
				Data:       []int{1, 2, 3}, // Arrays at root level work, but not ideal
				Timestamp:  time.Now(),
				RetryCount: 0,
				MaxRetries: 3,
			},
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := worker.syncToMongoDB(tc.job)
			if tc.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSyncWorker_EmptyDataHandling tests handling of empty data
func TestSyncWorker_EmptyDataHandling(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testJob := &SyncJob{
		ID:         uuid.New().String(),
		Type:       "citizen",
		Key:        "12345678901",
		Collection: "test_citizens",
		Data:       map[string]interface{}{},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err := worker.syncToMongoDB(testJob)
	assert.NoError(t, err)

	// Verify empty document was created
	var result bson.M
	err = db.Collection("test_citizens").FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&result)
	// Should not find document with just CPF filter since we only stored empty data
	assert.Error(t, err)
}

// TestSyncWorker_MultipleWorkers tests multiple workers processing concurrently
func TestSyncWorker_MultipleWorkers(t *testing.T) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping test: MONGODB_URI not set")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	logging.InitLogger()

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.CitizenCollection = "test_citizens"
	config.AppConfig.SelfDeclaredCollection = "test_self_declared"
	config.AppConfig.UserConfigCollection = "test_user_config"

	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err)

	db := client.Database("rmi_test_multi_worker")
	defer db.Drop(ctx)
	defer client.Disconnect(ctx)

	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	redisClient := redisclient.NewClient(singleClient)

	metrics := NewMetrics()
	logger := logging.Logger
	degradedMode := NewDegradedMode(redisClient, db, metrics)

	// Create 3 workers
	workers := make([]*SyncWorker, 3)
	for i := 0; i < 3; i++ {
		workers[i] = NewSyncWorker(redisClient, db, i+1, logger, metrics, degradedMode)
	}

	// Add jobs to queue
	numJobs := 15
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

	// Start workers
	for _, worker := range workers {
		go worker.Start()
	}

	// Let workers process
	time.Sleep(2 * time.Second)

	// Stop workers
	for _, worker := range workers {
		worker.Stop()
	}

	time.Sleep(200 * time.Millisecond)

	// Verify jobs were processed
	count, err := db.Collection("test_citizens").CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Greater(t, count, int64(10), "Multiple workers should have processed jobs")

	// Clean up Redis
	patterns := []string{
		"citizen:*",
		"sync:queue:*",
		"sync:dlq:*",
	}
	for _, pattern := range patterns {
		keys, _ := redisClient.Keys(ctx, pattern).Result()
		if len(keys) > 0 {
			redisClient.Del(ctx, keys...)
		}
	}
}

// TestSyncWorker_AllCollectionTypes tests all collection types
func TestSyncWorker_AllCollectionTypes(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		name       string
		jobType    string
		collection string
		key        string
		data       map[string]interface{}
		filterKey  string
	}{
		{
			name:       "citizen",
			jobType:    "citizen",
			collection: "test_citizens",
			key:        "12345678901",
			data:       map[string]interface{}{"cpf": "12345678901", "nome": "Test"},
			filterKey:  "cpf",
		},
		{
			name:       "self_declared",
			jobType:    "self_declared_email",
			collection: "self_declared",
			key:        "12345678901",
			data:       map[string]interface{}{"cpf": "12345678901", "email": "test@example.com", "updated_at": time.Now()},
			filterKey:  "cpf",
		},
		{
			name:       "user_config",
			jobType:    "user_config",
			collection: "test_user_config",
			key:        "12345678901",
			data:       map[string]interface{}{"cpf": "12345678901", "theme": "dark"},
			filterKey:  "cpf",
		},
		{
			name:       "phone_mapping",
			jobType:    "phone_mapping",
			collection: "phone_cpf_mappings",
			key:        "5521999999999",
			data:       map[string]interface{}{"phone_number": "5521999999999", "cpf": "12345678901"},
			filterKey:  "phone_number",
		},
		{
			name:       "opt_in_history",
			jobType:    "opt_in_history",
			collection: "opt_in_histories",
			key:        "12345678901",
			data:       map[string]interface{}{"cpf": "12345678901", "action": "opt_in"},
			filterKey:  "cpf",
		},
		{
			name:       "beta_group",
			jobType:    "beta_group",
			collection: "beta_groups",
			key:        "12345678901",
			data:       map[string]interface{}{"cpf": "12345678901", "group": "test_group"},
			filterKey:  "cpf",
		},
		{
			name:       "phone_verification",
			jobType:    "phone_verification",
			collection: "phone_verifications",
			key:        "5521999999999",
			data:       map[string]interface{}{"phone_number": "5521999999999", "verified": true},
			filterKey:  "phone_number",
		},
		{
			name:       "maintenance_request",
			jobType:    "maintenance_request",
			collection: "maintenance_requests",
			key:        "12345678901",
			data:       map[string]interface{}{"cpf": "12345678901", "request": "test"},
			filterKey:  "cpf",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testJob := &SyncJob{
				ID:         uuid.New().String(),
				Type:       tc.jobType,
				Key:        tc.key,
				Collection: tc.collection,
				Data:       tc.data,
				Timestamp:  time.Now(),
				RetryCount: 0,
				MaxRetries: 3,
			}

			err := worker.syncToMongoDB(testJob)
			assert.NoError(t, err)

			// Verify data was written
			var result bson.M
			err = db.Collection(tc.collection).FindOne(ctx, bson.M{tc.filterKey: tc.key}).Decode(&result)
			require.NoError(t, err)
			assert.Equal(t, tc.key, result[tc.filterKey])
		})
	}
}
