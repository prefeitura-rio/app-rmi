package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/redis/go-redis/v9"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reliabilityCPF = "11144477735" // synthetic, check-digit valid

// reliabilityRedisDB keeps these queues away from Redis DB 0, where other tests of this
// and of the handlers package (running in parallel) enqueue jobs and delete sync:queue:*.
const reliabilityRedisDB = 11

// setupReliabilityWorker returns a sync worker on its own Redis database, emptied before
// and after the test. It does not touch the package-wide config.
func setupReliabilityWorker(t *testing.T) *SyncWorker {
	t.Helper()
	setupTestEnvironment()
	if config.MongoDB == nil {
		t.Skip("Skipping sync worker reliability tests: MongoDB not initialized")
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rc := redisclient.NewClient(redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_PASSWORD"), DB: reliabilityRedisDB}))
	ctx := context.Background()
	require.NoError(t, rc.Ping(ctx).Err())
	require.NoError(t, rc.FlushDB(ctx).Err())
	t.Cleanup(func() { _ = rc.FlushDB(context.Background()).Err() })
	metrics := NewMetrics()
	return NewSyncWorker(rc, config.MongoDB, 1, logging.GetLogger(), metrics, NewDegradedMode(rc, config.MongoDB, metrics))
}

func reliableQueueLens(t *testing.T, worker *SyncWorker, queue string) (queued, processing, claimed, dlq int64) {
	t.Helper()
	ctx := context.Background()
	var err error
	queued, err = worker.redis.LLen(ctx, syncQueueKey(queue)).Result()
	require.NoError(t, err)
	processing, err = worker.redis.LLen(ctx, syncProcessingKey(queue)).Result()
	require.NoError(t, err)
	claimed, err = worker.redis.Eval(ctx, "return redis.call('ZCARD', KEYS[1])", []string{syncProcessingClaimKey(queue)}).Int64()
	require.NoError(t, err)
	dlq, err = worker.redis.LLen(ctx, syncDLQKey(queue)).Result()
	require.NoError(t, err)
	return
}

func firstListItem(t *testing.T, worker *SyncWorker, key string) string {
	t.Helper()
	items, err := worker.redis.LRange(context.Background(), key, 0, 0).Result()
	require.NoError(t, err)
	require.Len(t, items, 1)
	return items[0]
}

// claimSalesforceSyncJob enqueues an anonimizacao job and claims it from the reliable queue,
// leaving it in flight (processing list + claim set) as the worker does.
func claimSalesforceSyncJob(t *testing.T, worker *SyncWorker) *SyncJob {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, EnqueueSalesforceSyncJob(ctx, worker.redis, reliabilityCPF, SalesforceWebhookEventAnonimizacao, "2026-08-27T17:00:00Z", nil))
	job, err := worker.getJobNonBlocking(SalesforceSyncQueue)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.True(t, job.fromInflight)
	require.NotEmpty(t, job.rawRedisPayload)
	queued, processing, claimed, _ := reliableQueueLens(t, worker, SalesforceSyncQueue)
	require.Zero(t, queued)
	require.Equal(t, int64(1), processing)
	require.Equal(t, int64(1), claimed)
	return job
}

func TestReliableQueue_RetryableFailureMovesInflightJobBackToQueue(t *testing.T) {
	worker := setupReliabilityWorker(t)

	job := claimSalesforceSyncJob(t, worker)
	worker.handleSyncFailure(job, fmt.Errorf("mongo unavailable"))

	queued, processing, claimed, dlq := reliableQueueLens(t, worker, SalesforceSyncQueue)
	assert.Equal(t, int64(1), queued, "job is back on the queue")
	assert.Zero(t, processing, "inflight entry removed atomically")
	assert.Zero(t, claimed)
	assert.Zero(t, dlq)

	raw := firstListItem(t, worker, syncQueueKey(SalesforceSyncQueue))
	var requeued SyncJob
	require.NoError(t, json.Unmarshal([]byte(raw), &requeued))
	assert.Equal(t, job.ID, requeued.ID)
	assert.Equal(t, 1, requeued.RetryCount)
	assert.True(t, requeued.AvailableAt.After(time.Now()), "retry is deferred by the backoff")
}

func TestReliableQueue_DeferredJobIsReturnedUntilAvailable(t *testing.T) {
	worker := setupReliabilityWorker(t)

	job := claimSalesforceSyncJob(t, worker)
	worker.handleSyncFailure(job, fmt.Errorf("transient"))

	// The requeued job is not yet available: processJob returns it to the queue untouched.
	deferred, err := worker.getJobNonBlocking(SalesforceSyncQueue)
	require.NoError(t, err)
	require.NotNil(t, deferred)
	require.True(t, deferred.AvailableAt.After(time.Now()))
	worker.processJob(deferred)

	queued, processing, claimed, dlq := reliableQueueLens(t, worker, SalesforceSyncQueue)
	assert.Equal(t, int64(1), queued)
	assert.Zero(t, processing)
	assert.Zero(t, claimed)
	assert.Zero(t, dlq)

	raw := firstListItem(t, worker, syncQueueKey(SalesforceSyncQueue))
	var returned SyncJob
	require.NoError(t, json.Unmarshal([]byte(raw), &returned))
	assert.Equal(t, 1, returned.RetryCount, "deferral is not a new attempt")
}

func TestReliableQueue_ExhaustedRetriesGoToDLQWithoutBearer(t *testing.T) {
	worker := setupReliabilityWorker(t)

	job := claimSalesforceSyncJob(t, worker)
	job.RetryCount = job.MaxRetries - 1
	job.BearerToken = "enc:v1:sealed-token"
	worker.handleSyncFailure(job, fmt.Errorf("still failing"))

	queued, processing, claimed, dlq := reliableQueueLens(t, worker, SalesforceSyncQueue)
	assert.Zero(t, queued)
	assert.Zero(t, processing)
	assert.Zero(t, claimed)
	require.Equal(t, int64(1), dlq)

	raw := firstListItem(t, worker, syncDLQKey(SalesforceSyncQueue))
	assert.NotContains(t, raw, "sealed-token", "bearer must never reach the DLQ")
	var dlqJob DLQJob
	require.NoError(t, json.Unmarshal([]byte(raw), &dlqJob))
	assert.Equal(t, job.ID, dlqJob.OriginalJob.ID)
	assert.Equal(t, "still failing", dlqJob.Error)
	assert.Empty(t, dlqJob.OriginalJob.BearerToken)
}

func TestReliableQueue_NonRetryableFailureGoesStraightToDLQ(t *testing.T) {
	worker := setupReliabilityWorker(t)

	job := claimSalesforceSyncJob(t, worker)
	worker.handleSyncFailure(job, newNonRetryableSyncError(fmt.Errorf("salesforce 401")))

	queued, processing, _, dlq := reliableQueueLens(t, worker, SalesforceSyncQueue)
	assert.Zero(t, queued)
	assert.Zero(t, processing)
	assert.Equal(t, int64(1), dlq)
	assert.Equal(t, 0, job.RetryCount, "non-retryable failures do not consume retries")
}

func TestReliableQueue_MissingInflightPayloadIsNotPersisted(t *testing.T) {
	worker := setupReliabilityWorker(t)

	job := claimSalesforceSyncJob(t, worker)
	// Another worker recovered/acked the payload in the meantime.
	require.NoError(t, worker.ackReliableJob(context.Background(), SalesforceSyncQueue, job.rawRedisPayload))

	assert.False(t, worker.requeueJob(job), "requeue must report failure when the inflight payload is gone")
	assert.False(t, worker.moveToDLQ(job, fmt.Errorf("boom")), "DLQ must report failure when the inflight payload is gone")
	assert.ErrorContains(t, worker.returnJobToQueue(job), "inflight payload missing")

	queued, processing, _, dlq := reliableQueueLens(t, worker, SalesforceSyncQueue)
	assert.Zero(t, queued, "no duplicate is created")
	assert.Zero(t, processing)
	assert.Zero(t, dlq)
}

func TestReturnJobToQueue_PlainQueue(t *testing.T) {
	worker := setupReliabilityWorker(t)
	ctx := context.Background()
	queue := "self_declared_email"

	job := &SyncJob{ID: "deferred-plain", Type: queue, Key: reliabilityCPF, AvailableAt: time.Now().Add(time.Minute), MaxRetries: 3}
	worker.processJob(job)

	raw, err := worker.redis.RPop(ctx, syncQueueKey(queue)).Result()
	require.NoError(t, err)
	var returned SyncJob
	require.NoError(t, json.Unmarshal([]byte(raw), &returned))
	assert.Equal(t, "deferred-plain", returned.ID)
	assert.Equal(t, 0, returned.RetryCount)
}

func TestRedactBearerFromJobData(t *testing.T) {
	assert.Nil(t, redactBearerFromJobData(nil))

	byValue := redactBearerFromJobData(SalesforcePushPayload{CPF: reliabilityCPF, BearerToken: "secret"})
	assert.Equal(t, SalesforcePushPayload{CPF: reliabilityCPF}, byValue)

	original := &SalesforcePushPayload{CPF: reliabilityCPF, BearerToken: "secret"}
	byPointer := redactBearerFromJobData(original)
	assert.Equal(t, SalesforcePushPayload{CPF: reliabilityCPF}, byPointer)
	assert.Equal(t, "secret", original.BearerToken, "the caller's payload is not mutated")

	var nilPayload *SalesforcePushPayload
	assert.Equal(t, nilPayload, redactBearerFromJobData(nilPayload))

	asMap := redactBearerFromJobData(map[string]interface{}{"cpf": reliabilityCPF, "bearer_token": "secret"})
	assert.Equal(t, map[string]interface{}{"cpf": reliabilityCPF}, asMap)

	type otherPayload struct {
		CPF         string `json:"cpf"`
		BearerToken string `json:"bearer_token"`
	}
	asStruct := redactBearerFromJobData(otherPayload{CPF: reliabilityCPF, BearerToken: "secret"})
	assert.Equal(t, map[string]interface{}{"cpf": reliabilityCPF}, asStruct)

	unmarshalable := redactBearerFromJobData([]string{"a"})
	assert.Equal(t, []string{"a"}, unmarshalable, "non-object payloads are kept as they are")

	job := redactSyncJobSecrets(SyncJob{BearerToken: "enc:v1:x", Data: map[string]interface{}{"bearer_token": "y"}})
	assert.Empty(t, job.BearerToken)
	assert.NotContains(t, job.Data, "bearer_token")
}
