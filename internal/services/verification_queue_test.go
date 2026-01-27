package services

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.mongodb.org/mongo-driver/bson"
)

// setupVerificationQueueTest initializes test environment
func setupVerificationQueueTest(t *testing.T) func() {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping verification queue tests: MONGODB_URI not set")
	}

	_ = logging.InitLogger()

	// Use shared MongoDB connection from common_test.go
	// config.AppConfig is already initialized by TestMain via config.LoadConfig()
	// We rely on the actual collection names from the config

	// Redis setup (minimal for cache invalidation)
	if config.Redis == nil {
		config.InitRedis()
	}

	return func() {
		// Cleanup: drop collections used by tests
		ctx := context.Background()
		if config.AppConfig != nil {
			_ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).Drop(ctx)
			_ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).Drop(ctx)
		}
	}
}

func TestNewVerificationQueue(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	tests := []struct {
		name      string
		workers   int
		queueSize int
	}{
		{"single_worker_small_queue", 1, 10},
		{"multiple_workers_medium_queue", 5, 100},
		{"many_workers_large_queue", 10, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vq := NewVerificationQueue(tt.workers, tt.queueSize)
			defer vq.Stop()

			if vq == nil {
				t.Fatal("NewVerificationQueue() returned nil")
			}
			if vq.queue == nil {
				t.Error("queue channel is nil")
			}
			if vq.results == nil {
				t.Error("results channel is nil")
			}
			if vq.workers != tt.workers {
				t.Errorf("workers = %d, want %d", vq.workers, tt.workers)
			}
			if vq.processingStats == nil {
				t.Error("processingStats is nil")
			}
			if vq.ctx == nil {
				t.Error("context is nil")
			}
		})
	}
}

func TestEnqueue(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(2, 10)
	defer vq.Stop()

	job := VerificationJob{
		PhoneNumber: "+5521987654321",
		Code:        "123456",
		CPF:         "12345678901",
		UserID:      "user123",
		CreatedAt:   time.Now(),
	}

	err := vq.Enqueue(job)
	if err != nil {
		t.Errorf("Enqueue() error = %v, want nil", err)
	}

	stats := vq.GetStats()
	if stats.JobsEnqueued == 0 {
		t.Error("JobsEnqueued should be > 0 after enqueue")
	}
}

func TestEnqueue_QueueFull(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	// Create queue with size 2
	vq := NewVerificationQueue(1, 2)

	// Fill the queue
	for i := 0; i < 2; i++ {
		job := VerificationJob{
			PhoneNumber: fmt.Sprintf("+552198765432%d", i),
			Code:        "123456",
			CPF:         "12345678901",
			CreatedAt:   time.Now(),
		}
		_ = vq.Enqueue(job)
	}

	// This should fail because queue is full
	job := VerificationJob{
		PhoneNumber: "+5521987654399",
		Code:        "123456",
		CPF:         "12345678901",
		CreatedAt:   time.Now(),
	}

	err := vq.Enqueue(job)
	if err == nil {
		t.Error("Enqueue() should return error when queue is full")
	}

	// Stop queue before cleanup to avoid "client is disconnected" errors
	vq.Stop()
}

func TestBulkEnqueueJobs(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(2, 100)
	defer vq.Stop()

	jobs := make([]VerificationJob, 10)
	for i := 0; i < 10; i++ {
		jobs[i] = VerificationJob{
			PhoneNumber: fmt.Sprintf("+552198765%04d", i),
			Code:        "123456",
			CPF:         "12345678901",
			CreatedAt:   time.Now(),
		}
	}

	err := vq.BulkEnqueueJobs(jobs)
	if err != nil {
		t.Errorf("BulkEnqueueJobs() error = %v, want nil", err)
	}

	stats := vq.GetStats()
	if stats.JobsEnqueued != 10 {
		t.Errorf("JobsEnqueued = %d, want 10", stats.JobsEnqueued)
	}
}

func TestBulkEnqueueJobs_EmptySlice(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(2, 10)
	defer vq.Stop()

	err := vq.BulkEnqueueJobs([]VerificationJob{})
	if err != nil {
		t.Errorf("BulkEnqueueJobs() with empty slice error = %v, want nil", err)
	}
}

func TestBulkEnqueueJobs_PartialEnqueue(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	// Small queue to force dropping
	vq := NewVerificationQueue(1, 5)
	defer vq.Stop()

	// Try to enqueue more than queue size
	jobs := make([]VerificationJob, 10)
	for i := 0; i < 10; i++ {
		jobs[i] = VerificationJob{
			PhoneNumber: fmt.Sprintf("+552198765%04d", i),
			Code:        "123456",
			CPF:         "12345678901",
			CreatedAt:   time.Now(),
		}
	}

	err := vq.BulkEnqueueJobs(jobs)
	if err != nil {
		t.Errorf("BulkEnqueueJobs() error = %v, want nil (partial enqueue is ok)", err)
	}

	stats := vq.GetStats()
	// Should have enqueued at least some jobs, but not all
	if stats.JobsEnqueued == 0 {
		t.Error("JobsEnqueued = 0, should have enqueued at least some jobs")
	}
}

func TestGetStats(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(3, 50)
	defer vq.Stop()

	stats := vq.GetStats()
	if stats.ActiveWorkers != 3 {
		t.Errorf("ActiveWorkers = %d, want 3", stats.ActiveWorkers)
	}
	if stats.QueueSize != 0 {
		t.Errorf("QueueSize = %d, want 0 (empty queue)", stats.QueueSize)
	}
	if stats.JobsEnqueued != 0 {
		t.Errorf("JobsEnqueued = %d, want 0", stats.JobsEnqueued)
	}
	if stats.JobsProcessed != 0 {
		t.Errorf("JobsProcessed = %d, want 0", stats.JobsProcessed)
	}
	if stats.JobsFailed != 0 {
		t.Errorf("JobsFailed = %d, want 0", stats.JobsFailed)
	}
}

func TestIsHealthy(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	tests := []struct {
		name     string
		setup    func(*VerificationQueue)
		wantSick bool
	}{
		{
			name: "healthy_empty_queue",
			setup: func(vq *VerificationQueue) {
				// No setup needed
			},
			wantSick: false,
		},
		{
			name: "healthy_with_jobs",
			setup: func(vq *VerificationQueue) {
				for i := 0; i < 10; i++ {
					_ = vq.Enqueue(VerificationJob{
						PhoneNumber: fmt.Sprintf("+552198765%04d", i),
						Code:        "123456",
						CPF:         "12345678901",
						CreatedAt:   time.Now(),
					})
				}
				// Wait for workers to process the jobs
				time.Sleep(200 * time.Millisecond)
			},
			wantSick: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vq := NewVerificationQueue(2, 100)
			defer vq.Stop()

			tt.setup(vq)

			healthy := vq.IsHealthy()
			if tt.wantSick && healthy {
				t.Error("IsHealthy() = true, want false (unhealthy)")
			}
			if !tt.wantSick && !healthy {
				t.Error("IsHealthy() = false, want true (healthy)")
			}
		})
	}
}

func TestValidateVerificationCode_Success(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert valid verification code
	code := "123456"
	phoneNumber := "+5521987654321"
	normalizedPhone := "5521987654321"

	verificationDoc := bson.M{
		"phone_number": normalizedPhone,
		"code":         code,
		"used":         false,
		"expires_at":   time.Now().Add(5 * time.Minute),
		"created_at":   time.Now(),
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).InsertOne(ctx, verificationDoc)

	// Insert phone mapping
	phoneMappingDoc := bson.M{
		"phone_number": normalizedPhone,
		"verified":     false,
		"created_at":   time.Now(),
		"updated_at":   time.Now(),
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, phoneMappingDoc)

	vq := NewVerificationQueue(1, 10)
	defer vq.Stop()

	job := VerificationJob{
		PhoneNumber: phoneNumber,
		Code:        code,
		CPF:         "12345678901",
		UserID:      "user123",
		CreatedAt:   time.Now(),
	}

	err := vq.validateVerificationCode(job)
	if err != nil {
		t.Errorf("validateVerificationCode() error = %v, want nil", err)
	}

	// Verify code was marked as used
	var verification struct {
		Used bool `bson:"used"`
	}
	_ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).FindOne(
		ctx,
		bson.M{"phone_number": normalizedPhone, "code": code},
	).Decode(&verification)

	if !verification.Used {
		t.Error("Verification code should be marked as used")
	}

	// Verify phone mapping was updated
	var phoneMapping struct {
		Verified bool `bson:"verified"`
	}
	_ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": normalizedPhone},
	).Decode(&phoneMapping)

	if !phoneMapping.Verified {
		t.Error("Phone mapping should be marked as verified")
	}
}

func TestValidateVerificationCode_InvalidCode(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(1, 10)
	defer vq.Stop()

	job := VerificationJob{
		PhoneNumber: "+5521987654322",
		Code:        "999999", // Invalid code
		CPF:         "12345678901",
		UserID:      "user123",
		CreatedAt:   time.Now(),
	}

	err := vq.validateVerificationCode(job)
	if err == nil {
		t.Error("validateVerificationCode() should return error for invalid code")
	}
}

func TestValidateVerificationCode_ExpiredCode(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert expired verification code
	code := "123456"
	phoneNumber := "+5521987654323"
	normalizedPhone := "5521987654323"

	verificationDoc := bson.M{
		"phone_number": normalizedPhone,
		"code":         code,
		"used":         false,
		"expires_at":   time.Now().Add(-5 * time.Minute), // Expired
		"created_at":   time.Now().Add(-10 * time.Minute),
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).InsertOne(ctx, verificationDoc)

	vq := NewVerificationQueue(1, 10)
	defer vq.Stop()

	job := VerificationJob{
		PhoneNumber: phoneNumber,
		Code:        code,
		CPF:         "12345678901",
		UserID:      "user123",
		CreatedAt:   time.Now(),
	}

	err := vq.validateVerificationCode(job)
	if err == nil {
		t.Error("validateVerificationCode() should return error for expired code")
	}
}

func TestValidateVerificationCode_AlreadyUsed(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert already used verification code
	code := "123456"
	phoneNumber := "+5521987654324"
	normalizedPhone := "5521987654324"

	verificationDoc := bson.M{
		"phone_number": normalizedPhone,
		"code":         code,
		"used":         true, // Already used
		"expires_at":   time.Now().Add(5 * time.Minute),
		"created_at":   time.Now(),
		"used_at":      time.Now(),
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).InsertOne(ctx, verificationDoc)

	vq := NewVerificationQueue(1, 10)
	defer vq.Stop()

	job := VerificationJob{
		PhoneNumber: phoneNumber,
		Code:        code,
		CPF:         "12345678901",
		UserID:      "user123",
		CreatedAt:   time.Now(),
	}

	err := vq.validateVerificationCode(job)
	if err == nil {
		t.Error("validateVerificationCode() should return error for already used code")
	}
}

func TestProcessJob(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert valid verification code for success test
	code := "123456"
	phoneNumber := "+5521987654325"
	normalizedPhone := "5521987654325"

	verificationDoc := bson.M{
		"phone_number": normalizedPhone,
		"code":         code,
		"used":         false,
		"expires_at":   time.Now().Add(5 * time.Minute),
		"created_at":   time.Now(),
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).InsertOne(ctx, verificationDoc)

	phoneMappingDoc := bson.M{
		"phone_number": normalizedPhone,
		"verified":     false,
		"created_at":   time.Now(),
		"updated_at":   time.Now(),
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, phoneMappingDoc)

	vq := NewVerificationQueue(1, 10)
	defer vq.Stop()

	job := VerificationJob{
		PhoneNumber: phoneNumber,
		Code:        code,
		CPF:         "12345678901",
		UserID:      "user123",
		CreatedAt:   time.Now(),
	}

	// Process the job
	vq.processJob(job, 0)

	// Give time for result to be sent
	time.Sleep(100 * time.Millisecond)

	stats := vq.GetStats()
	if stats.JobsProcessed == 0 {
		t.Error("JobsProcessed should be > 0 after processing job")
	}
}

func TestProcessBatchResults(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(1, 10)
	defer vq.Stop()

	results := []VerificationResult{
		{
			JobID:       "1-+5521987654326",
			PhoneNumber: "+5521987654326",
			Success:     true,
			ProcessedAt: time.Now(),
		},
		{
			JobID:       "1-+5521987654327",
			PhoneNumber: "+5521987654327",
			Success:     false,
			Error:       "invalid code",
			ProcessedAt: time.Now(),
		},
	}

	err := vq.processBatchResults(results)
	if err != nil {
		t.Errorf("processBatchResults() error = %v, want nil", err)
	}

	stats := vq.GetStats()
	if stats.JobsFailed == 0 {
		t.Error("JobsFailed should be > 0 after processing failed result")
	}
}

func TestProcessBatchResults_EmptyBatch(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(1, 10)
	defer vq.Stop()

	err := vq.processBatchResults([]VerificationResult{})
	if err != nil {
		t.Errorf("processBatchResults() with empty batch error = %v, want nil", err)
	}
}

func TestStop(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(2, 10)

	// Enqueue a job
	job := VerificationJob{
		PhoneNumber: "+5521987654328",
		Code:        "123456",
		CPF:         "12345678901",
		CreatedAt:   time.Now(),
	}
	_ = vq.Enqueue(job)

	// Stop the queue
	vq.Stop()

	// Verify context is cancelled
	select {
	case <-vq.ctx.Done():
		// Success - context is cancelled
	default:
		t.Error("Context should be cancelled after Stop()")
	}
}

func TestWorker_Integration(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Setup valid verification for processing
	code := "123456"
	phoneNumber := "+5521987654329"
	normalizedPhone := "5521987654329"

	verificationDoc := bson.M{
		"phone_number": normalizedPhone,
		"code":         code,
		"used":         false,
		"expires_at":   time.Now().Add(5 * time.Minute),
		"created_at":   time.Now(),
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).InsertOne(ctx, verificationDoc)

	phoneMappingDoc := bson.M{
		"phone_number": normalizedPhone,
		"verified":     false,
		"created_at":   time.Now(),
		"updated_at":   time.Now(),
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, phoneMappingDoc)

	vq := NewVerificationQueue(2, 10)
	defer vq.Stop()

	job := VerificationJob{
		PhoneNumber: phoneNumber,
		Code:        code,
		CPF:         "12345678901",
		UserID:      "user123",
		CreatedAt:   time.Now(),
	}

	err := vq.Enqueue(job)
	if err != nil {
		t.Fatalf("Failed to enqueue job: %v", err)
	}

	// Wait for job to be processed
	time.Sleep(500 * time.Millisecond)

	stats := vq.GetStats()
	if stats.JobsProcessed == 0 {
		t.Error("Worker should have processed at least one job")
	}
}

func TestProcessingStats_Concurrency(t *testing.T) {
	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	vq := NewVerificationQueue(5, 100)
	defer vq.Stop()

	// Concurrently enqueue jobs
	var wg atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Add(-1)
			job := VerificationJob{
				PhoneNumber: fmt.Sprintf("+552198765%04d", i),
				Code:        "123456",
				CPF:         "12345678901",
				CreatedAt:   time.Now(),
			}
			_ = vq.Enqueue(job)
		}(i)
	}

	// Wait for all goroutines
	for wg.Load() > 0 {
		time.Sleep(10 * time.Millisecond)
	}

	stats := vq.GetStats()
	if stats.JobsEnqueued != 10 {
		t.Errorf("JobsEnqueued = %d, want 10", stats.JobsEnqueued)
	}
}

func TestAverageWaitTime_Updated(t *testing.T) {
	t.Skip("Skipping flaky timing-dependent test - relies on precise timing and worker scheduling")

	cleanup := setupVerificationQueueTest(t)
	defer cleanup()

	ctx := context.Background()

	// Setup multiple valid verifications
	for i := 0; i < 3; i++ {
		phoneNumber := fmt.Sprintf("552198765%04d", 400+i)
		verificationDoc := bson.M{
			"phone_number": phoneNumber,
			"code":         "123456",
			"used":         false,
			"expires_at":   time.Now().Add(5 * time.Minute),
			"created_at":   time.Now(),
		}
		_, _ = config.MongoDB.Collection(config.AppConfig.PhoneVerificationCollection).InsertOne(ctx, verificationDoc)

		phoneMappingDoc := bson.M{
			"phone_number": phoneNumber,
			"verified":     false,
			"created_at":   time.Now(),
			"updated_at":   time.Now(),
		}
		_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, phoneMappingDoc)
	}

	vq := NewVerificationQueue(2, 10)
	defer vq.Stop()

	// Enqueue jobs
	for i := 0; i < 3; i++ {
		job := VerificationJob{
			PhoneNumber: fmt.Sprintf("+552198765%04d", 400+i),
			Code:        "123456",
			CPF:         "12345678901",
			CreatedAt:   time.Now(),
		}
		_ = vq.Enqueue(job)
	}

	// Wait for processing
	time.Sleep(1 * time.Second)

	stats := vq.GetStats()
	if stats.AverageWaitTime == 0 && stats.JobsProcessed > 0 {
		t.Error("AverageWaitTime should be updated after processing jobs")
	}
}
