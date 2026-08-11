package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestBuildMobilidadeInviteEmail(t *testing.T) {
	msg := BuildMobilidadeInviteEmail(MobilidadeInviteEmailPayload{
		NotifyEmail: "joao@example.com",
		OwnerName:   "Ana Souza",
		DisplayName: "Bike do trabalho",
	}, "https://pref.rio/")

	assert.Equal(t, "joao@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Ana Souza")
	assert.Contains(t, msg.Body, "Bike do trabalho")
	assert.Contains(t, msg.Body, "https://pref.rio/carteira?mobilidade=true")
}

func TestBuildMobilidadeInviteEmail_Defaults(t *testing.T) {
	msg := BuildMobilidadeInviteEmail(MobilidadeInviteEmailPayload{
		NotifyEmail: "x@example.com",
	}, "")

	assert.Contains(t, msg.Subject, "Alguém")
	assert.Contains(t, msg.Body, "um veículo")
	assert.Contains(t, msg.Body, "https://pref.rio/carteira?mobilidade=true")
}

func TestProcessMobilidadeInviteEmail_Success(t *testing.T) {
	sender := &RecordingEmailSender{}
	err := ProcessMobilidadeInviteEmail(context.Background(), sender, MobilidadeInviteEmailPayload{
		ConductorID: "cond1",
		NotifyEmail: "joao@example.com",
		OwnerName:   "Ana",
		DisplayName: "Bike",
	}, "https://pref.rio")
	require.NoError(t, err)
	require.Equal(t, 1, sender.SentCount())
	assert.Equal(t, "joao@example.com", sender.Messages[0].To)
}

func TestProcessMobilidadeInviteEmail_RequiresEmailAndConductorID(t *testing.T) {
	sender := &RecordingEmailSender{}
	err := ProcessMobilidadeInviteEmail(context.Background(), sender, MobilidadeInviteEmailPayload{
		ConductorID: "cond1",
	}, "https://pref.rio")
	require.Error(t, err)
	assert.Equal(t, 0, sender.SentCount())

	err = ProcessMobilidadeInviteEmail(context.Background(), sender, MobilidadeInviteEmailPayload{
		NotifyEmail: "a@example.com",
	}, "https://pref.rio")
	require.Error(t, err)
}

func TestProcessMobilidadeInviteEmail_SenderError(t *testing.T) {
	sender := &RecordingEmailSender{Err: errors.New("smtp down")}
	err := ProcessMobilidadeInviteEmail(context.Background(), sender, MobilidadeInviteEmailPayload{
		ConductorID: "cond1",
		NotifyEmail: "a@example.com",
	}, "https://pref.rio")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "smtp down")
}

func TestSyncWorker_HandleMobilidadeInviteEmailJob(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	condID := primitive.NewObjectID().Hex()
	job := &SyncJob{
		ID:         "job-1",
		Type:       MobilidadeInviteEmailQueue,
		Key:        condID,
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID:  condID,
			NotifyEmail:  "joao@example.com",
			VehicleID:    "veh-1",
			OwnerName:    "Ana Souza",
			DisplayName:  "Bike do trabalho",
			ConductorCPF: "45049725810",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}

	err := worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.NoError(t, err)
	require.Equal(t, 1, sender.SentCount())
	assert.Contains(t, sender.Messages[0].Body, "Ana Souza")
}

func TestSyncWorker_ProcessMobilidadeInviteEmailFromQueue(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	condID := primitive.NewObjectID().Hex()
	job := SyncJob{
		ID:         "job-queue-1",
		Type:       MobilidadeInviteEmailQueue,
		Key:        condID,
		Collection: MobilidadeInviteEmailQueue,
		Data: map[string]interface{}{
			"conductor_id":  condID,
			"notify_email":  "maria@example.com",
			"vehicle_id":    "veh-2",
			"owner_name":    "Pedro",
			"display_name":  "Patinete",
			"conductor_cpf": "11144477735",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}
	raw, err := json.Marshal(job)
	require.NoError(t, err)

	ctx := context.Background()
	queueKey := syncQueueKey(MobilidadeInviteEmailQueue)
	claimKey := syncProcessingClaimKey(MobilidadeInviteEmailQueue)
	_ = worker.redis.Del(ctx, queueKey, syncProcessingKey(MobilidadeInviteEmailQueue), claimKey)
	require.NoError(t, worker.redis.LPush(ctx, queueKey, raw).Err())

	got, err := worker.getJobNonBlocking(MobilidadeInviteEmailQueue)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MobilidadeInviteEmailQueue, got.Type)

	// Claim must be recorded atomically with RPOPLPUSH (same Lua script).
	score, err := worker.redis.ZScore(ctx, claimKey, got.rawRedisPayload).Result()
	require.NoError(t, err)
	assert.Greater(t, score, float64(0))

	worker.processJob(got)
	require.Equal(t, 1, sender.SentCount())
	assert.Equal(t, "maria@example.com", sender.Messages[0].To)
	assert.Contains(t, sender.Messages[0].Body, "Patinete")
}

func TestSyncWorker_MobilidadeInviteEmailFailure(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{Err: errors.New("provider unavailable")}
	worker.SetEmailSender(sender)

	condID := primitive.NewObjectID().Hex()
	job := &SyncJob{
		ID:         "job-fail-1",
		Type:       MobilidadeInviteEmailQueue,
		Key:        condID,
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID: condID,
			NotifyEmail: "fail@example.com",
			OwnerName:   "Ana",
			DisplayName: "Bike",
		},
		Timestamp:  time.Now().UTC(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err := worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider unavailable")
	assert.Equal(t, 0, sender.SentCount())
}

func TestSyncWorker_MobilidadeInviteEmailSkippedWhenLoggingSender(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	worker.SetEmailSender(NewLoggingEmailSender(nil))

	condID := primitive.NewObjectID().Hex()
	job := &SyncJob{
		ID:         "job-skip-1",
		Type:       MobilidadeInviteEmailQueue,
		Key:        condID,
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID: condID,
			NotifyEmail: "skip@example.com",
			OwnerName:   "Ana",
			DisplayName: "Bike",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}

	err := worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.NoError(t, err, "skipped delivery must not retry as failure")
}

func TestSyncWorker_SpecialJobSkipsWriteBufferCleanup(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	condID := primitive.NewObjectID().Hex()
	job := &SyncJob{
		ID:         "job-no-wb",
		Type:       MobilidadeInviteEmailQueue,
		Key:        condID,
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID: condID,
			NotifyEmail: "nowb@example.com",
			OwnerName:   "Ana",
			DisplayName: "Bike",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}

	ctx := context.Background()
	cacheKey := fmt.Sprintf("%s:cache:%s", job.Type, job.Key)
	_ = worker.redis.Del(ctx, cacheKey)

	worker.processJob(job)
	require.Equal(t, 1, sender.SentCount())

	exists, err := worker.redis.Exists(ctx, cacheKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "invite email jobs must not write spurious cache keys")
}

func TestSyncWorker_MobilidadeInviteEmailInflightAck(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	condID := primitive.NewObjectID().Hex()
	job := SyncJob{
		ID:         "job-inflight-1",
		Type:       MobilidadeInviteEmailQueue,
		Key:        condID,
		Collection: MobilidadeInviteEmailQueue,
		Data: map[string]interface{}{
			"conductor_id":  condID,
			"notify_email":  "inflight@example.com",
			"vehicle_id":    "veh-1",
			"owner_name":    "Ana",
			"display_name":  "Bike",
			"conductor_cpf": "11144477735",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}
	raw, err := json.Marshal(job)
	require.NoError(t, err)

	ctx := context.Background()
	queueKey := syncQueueKey(MobilidadeInviteEmailQueue)
	processingKey := syncProcessingKey(MobilidadeInviteEmailQueue)
	claimKey := syncProcessingClaimKey(MobilidadeInviteEmailQueue)
	_ = worker.redis.Del(ctx, queueKey, processingKey, claimKey)
	require.NoError(t, worker.redis.LPush(ctx, queueKey, raw).Err())

	got, err := worker.getJobNonBlocking(MobilidadeInviteEmailQueue)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.fromInflight)
	assert.NotEmpty(t, got.rawRedisPayload)

	processingLen, err := worker.redis.LLen(ctx, processingKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), processingLen)

	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), queueLen)

	worker.processJob(got)
	require.Equal(t, 1, sender.SentCount())

	processingLen, err = worker.redis.LLen(ctx, processingKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), processingLen)

	claimCount, err := worker.redis.Eval(ctx, `return redis.call('ZCARD', KEYS[1])`, []string{claimKey}).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(0), claimCount, "ack must clear claim atomically with LREM")
}

func TestSyncWorker_InviteEmailStatusPersistFailureRetriesWithoutResend(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	// Invalid ObjectID makes SetConductorInviteEmailStatus fail after a successful send.
	job := &SyncJob{
		ID:         "job-persist-fail",
		Type:       MobilidadeInviteEmailQueue,
		Key:        "not-a-valid-object-id",
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID: "not-a-valid-object-id",
			NotifyEmail: "persist@example.com",
			OwnerName:   "Ana",
			DisplayName: "Bike",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}

	err := worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist invite email status")
	require.Equal(t, 1, sender.SentCount())

	payload, err := parseMobilidadeInviteEmailPayload(job.Data)
	require.NoError(t, err)
	assert.Equal(t, string(models.InviteEmailStatusSent), payload.DeliveryOutcome)

	// Retry with DeliveryOutcome set must not call the provider again.
	err = worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.Error(t, err)
	assert.Equal(t, 1, sender.SentCount(), "must not resend after delivery_outcome is set")

	// Durable success path: fix conductor id and persist without resending.
	condID := primitive.NewObjectID().Hex()
	payload.ConductorID = condID
	job.Key = condID
	job.Data = payload
	err = worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.NoError(t, err)
	assert.Equal(t, 1, sender.SentCount())
}

func TestSyncWorker_ProcessJobPersistFailureRequeuesWithoutAckOrResend(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	job := SyncJob{
		ID:         "job-process-persist-fail",
		Type:       MobilidadeInviteEmailQueue,
		Key:        "not-a-valid-object-id",
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID: "not-a-valid-object-id",
			NotifyEmail: "process-persist@example.com",
			OwnerName:   "Ana",
			DisplayName: "Bike",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}
	raw, err := json.Marshal(job)
	require.NoError(t, err)

	ctx := context.Background()
	queueKey := syncQueueKey(MobilidadeInviteEmailQueue)
	processingKey := syncProcessingKey(MobilidadeInviteEmailQueue)
	claimKey := syncProcessingClaimKey(MobilidadeInviteEmailQueue)
	_ = worker.redis.Del(ctx, queueKey, processingKey, claimKey)
	require.NoError(t, worker.redis.LPush(ctx, queueKey, raw).Err())

	got, err := worker.getJobNonBlocking(MobilidadeInviteEmailQueue)
	require.NoError(t, err)
	require.NotNil(t, got)

	worker.processJob(got)
	require.Equal(t, 1, sender.SentCount())

	// Persist failure must not leave a silent success ack: job leaves processing via requeue.
	processingLen, err := worker.redis.LLen(ctx, processingKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), processingLen, "failed persist must not ack-away the job without requeue")

	claimCount, err := worker.redis.Eval(ctx, `return redis.call('ZCARD', KEYS[1])`, []string{claimKey}).Int64()
	require.NoError(t, err)
	assert.Equal(t, int64(0), claimCount)

	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen, "job must return to the main queue for retry")

	requeuedRaw, err := worker.redis.RPop(ctx, queueKey).Result()
	require.NoError(t, err)
	var requeued SyncJob
	require.NoError(t, json.Unmarshal([]byte(requeuedRaw), &requeued))
	assert.Equal(t, 1, requeued.RetryCount)
	payload, err := parseMobilidadeInviteEmailPayload(requeued.Data)
	require.NoError(t, err)
	assert.Equal(t, string(models.InviteEmailStatusSent), payload.DeliveryOutcome)

	// Second processJob: clear deferral, fix conductor id, must persist without resending.
	condID := primitive.NewObjectID().Hex()
	payload.ConductorID = condID
	requeued.Key = condID
	requeued.Data = payload
	requeued.AvailableAt = time.Time{}
	requeuedRaw2, err := json.Marshal(requeued)
	require.NoError(t, err)
	require.NoError(t, worker.redis.LPush(ctx, queueKey, requeuedRaw2).Err())

	got2, err := worker.getJobNonBlocking(MobilidadeInviteEmailQueue)
	require.NoError(t, err)
	require.NotNil(t, got2)
	worker.processJob(got2)

	require.Equal(t, 1, sender.SentCount(), "requeued job with delivery_outcome must not resend")
	processingLen, err = worker.redis.LLen(ctx, processingKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), processingLen, "successful persist retry must ack inflight")
	queueLen, err = worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), queueLen)
}

// countingSkipSender records Send calls and always reports delivery skipped.
type countingSkipSender struct {
	calls int
}

func (s *countingSkipSender) Send(ctx context.Context, msg EmailMessage) error {
	s.calls++
	return ErrEmailDeliverySkipped
}

func TestSyncWorker_InviteEmailSkippedPersistFailureRetriesWithoutResend(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &countingSkipSender{}
	worker.SetEmailSender(sender)

	job := &SyncJob{
		ID:         "job-skip-persist-fail",
		Type:       MobilidadeInviteEmailQueue,
		Key:        "not-a-valid-object-id",
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID: "not-a-valid-object-id",
			NotifyEmail: "skip-persist@example.com",
			OwnerName:   "Ana",
			DisplayName: "Bike",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}

	err := worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist invite email status")
	assert.Contains(t, err.Error(), string(models.InviteEmailStatusSkipped))
	require.Equal(t, 1, sender.calls)

	payload, err := parseMobilidadeInviteEmailPayload(job.Data)
	require.NoError(t, err)
	assert.Equal(t, string(models.InviteEmailStatusSkipped), payload.DeliveryOutcome)

	err = worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.Error(t, err)
	assert.Equal(t, 1, sender.calls, "must not invoke sender again after skipped delivery_outcome")

	condID := primitive.NewObjectID().Hex()
	payload.ConductorID = condID
	job.Key = condID
	job.Data = payload
	err = worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.NoError(t, err)
	assert.Equal(t, 1, sender.calls)
}

func TestSyncWorker_ClaimAckRecoverAreAtomicAgainstPartialFailureWindows(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	queueKey := syncQueueKey(MobilidadeInviteEmailQueue)
	processingKey := syncProcessingKey(MobilidadeInviteEmailQueue)
	claimKey := syncProcessingClaimKey(MobilidadeInviteEmailQueue)
	_ = worker.redis.Del(ctx, queueKey, processingKey, claimKey)

	payload := `{"id":"atomic-1","type":"mobilidade_invite_email","key":"cond"}`
	require.NoError(t, worker.redis.LPush(ctx, queueKey, payload).Err())

	claimed, err := worker.claimReliableJob(ctx, MobilidadeInviteEmailQueue)
	require.NoError(t, err)
	assert.Equal(t, payload, claimed)

	// After atomic claim: in processing AND in claim zset (no RPOPLPUSH-without-ZADD window).
	processingLen, err := worker.redis.LLen(ctx, processingKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), processingLen)
	_, err = worker.redis.ZScore(ctx, claimKey, payload).Result()
	require.NoError(t, err)

	require.NoError(t, worker.ackReliableJob(ctx, MobilidadeInviteEmailQueue, payload))
	processingLen, err = worker.redis.LLen(ctx, processingKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), processingLen)
	_, err = worker.redis.ZScore(ctx, claimKey, payload).Result()
	assert.ErrorIs(t, err, redis.Nil)

	// Stale recover: LREM+ZREM+LPUSH in one script — second recover is a no-op.
	stale := `{"id":"stale-atomic","type":"mobilidade_invite_email","key":"cond"}`
	require.NoError(t, worker.redis.LPush(ctx, processingKey, stale).Err())
	require.NoError(t, worker.redis.ZAdd(ctx, claimKey, redis.Z{
		Score: float64(time.Now().Add(-10 * time.Minute).Unix()), Member: stale,
	}).Err())

	moved, err := worker.recoverStaleInflightItem(ctx, MobilidadeInviteEmailQueue, stale, time.Now().Unix())
	require.NoError(t, err)
	assert.True(t, moved)

	movedAgain, err := worker.recoverStaleInflightItem(ctx, MobilidadeInviteEmailQueue, stale, time.Now().Unix())
	require.NoError(t, err)
	assert.False(t, movedAgain, "second recover must not duplicate requeue")

	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen)
}

func TestSyncWorker_InviteEmailCheckpointPreventsResendOnRecover(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	condID := primitive.NewObjectID().Hex()
	job := SyncJob{
		ID:         "job-checkpoint",
		Type:       MobilidadeInviteEmailQueue,
		Key:        condID,
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID: condID,
			NotifyEmail: "checkpoint@example.com",
			OwnerName:   "Ana",
			DisplayName: "Bike",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}
	raw, err := json.Marshal(job)
	require.NoError(t, err)

	ctx := context.Background()
	queueKey := syncQueueKey(MobilidadeInviteEmailQueue)
	processingKey := syncProcessingKey(MobilidadeInviteEmailQueue)
	claimKey := syncProcessingClaimKey(MobilidadeInviteEmailQueue)
	_ = worker.redis.Del(ctx, queueKey, processingKey, claimKey)
	require.NoError(t, worker.redis.LPush(ctx, queueKey, string(raw)).Err())

	got, err := worker.getJobNonBlocking(MobilidadeInviteEmailQueue)
	require.NoError(t, err)
	require.NotNil(t, got)

	// Break Mongo so status persist fails after send; checkpoint should still rewrite inflight.
	origMongo := worker.mongo
	worker.mongo = nil
	err = worker.handleMobilidadeInviteEmailJob(ctx, got)
	require.Error(t, err)
	assert.Equal(t, 1, sender.SentCount())
	worker.mongo = origMongo

	// Simulate worker crash: recover stale inflight (force claim score old).
	require.NoError(t, worker.redis.ZAdd(ctx, claimKey, redis.Z{
		Score: float64(time.Now().Add(-10 * time.Minute).Unix()), Member: got.rawRedisPayload,
	}).Err())
	prevTimeout := inflightVisibilityTimeout
	inflightVisibilityTimeout = time.Minute
	defer func() { inflightVisibilityTimeout = prevTimeout }()
	worker.recoverStaleInflightJobs()

	reclaimed, err := worker.getJobNonBlocking(MobilidadeInviteEmailQueue)
	require.NoError(t, err)
	require.NotNil(t, reclaimed)

	payload, err := parseMobilidadeInviteEmailPayload(reclaimed.Data)
	require.NoError(t, err)
	assert.Equal(t, string(models.InviteEmailStatusSent), payload.DeliveryOutcome)

	err = worker.handleMobilidadeInviteEmailJob(ctx, reclaimed)
	require.NoError(t, err)
	assert.Equal(t, 1, sender.SentCount(), "recovered job with delivery_outcome must not resend")
}

func TestSyncWorker_RecoverStaleInflightMobilidadeInviteJobs(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	prevTimeout := inflightVisibilityTimeout
	inflightVisibilityTimeout = time.Minute
	defer func() { inflightVisibilityTimeout = prevTimeout }()

	ctx := context.Background()
	queueKey := syncQueueKey(MobilidadeInviteEmailQueue)
	processingKey := syncProcessingKey(MobilidadeInviteEmailQueue)
	claimKey := syncProcessingClaimKey(MobilidadeInviteEmailQueue)
	_ = worker.redis.Del(ctx, queueKey, processingKey, claimKey)

	stalePayload := `{"id":"stale-1","type":"mobilidade_invite_email","key":"cond-stale"}`
	freshPayload := `{"id":"fresh-1","type":"mobilidade_invite_email","key":"cond-fresh"}`
	require.NoError(t, worker.redis.LPush(ctx, processingKey, stalePayload, freshPayload).Err())

	// Fresh claim (now) must not be stolen; stale claim (older than visibility) must be requeued.
	require.NoError(t, worker.redis.ZAdd(ctx, claimKey,
		redis.Z{Score: float64(time.Now().Unix()), Member: freshPayload},
		redis.Z{Score: float64(time.Now().Add(-2 * time.Minute).Unix()), Member: stalePayload},
	).Err())

	worker.recoverStaleInflightJobs()

	processingLen, err := worker.redis.LLen(ctx, processingKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), processingLen, "fresh in-flight job must stay in processing")

	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen, "only stale job should be requeued")

	queued, err := worker.redis.RPop(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, stalePayload, queued)

	stillProcessing, err := worker.redis.LRange(ctx, processingKey, 0, -1).Result()
	require.NoError(t, err)
	require.Len(t, stillProcessing, 1)
	assert.Equal(t, freshPayload, stillProcessing[0])
}

func TestSyncWorker_RecoverInflightSkipsFreshClaimsOnStart(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	queueKey := syncQueueKey(MobilidadeInviteEmailQueue)
	processingKey := syncProcessingKey(MobilidadeInviteEmailQueue)
	claimKey := syncProcessingClaimKey(MobilidadeInviteEmailQueue)
	_ = worker.redis.Del(ctx, queueKey, processingKey, claimKey)

	payload := `{"id":"active-1","type":"mobilidade_invite_email","key":"cond-active"}`
	require.NoError(t, worker.redis.LPush(ctx, processingKey, payload).Err())
	require.NoError(t, worker.redis.ZAdd(ctx, claimKey, redis.Z{
		Score: float64(time.Now().Unix()), Member: payload,
	}).Err())

	worker.recoverStaleInflightJobs()

	processingLen, err := worker.redis.LLen(ctx, processingKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), processingLen)

	queueLen, err := worker.redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), queueLen)
}

func TestInviteConductor_EnqueuesSyncJobForEmail(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	if config.Redis == nil {
		t.Skip("Redis not available")
	}
	_ = logging.InitLogger()

	ctx := context.Background()
	queueKey := syncQueueKey(MobilidadeInviteEmailQueue)
	_ = config.Redis.Del(ctx, queueKey)

	created, err := vehicleSvc.CreateVehicle(ctx, mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(ctx, mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor",
	})
	require.NoError(t, err)

	raw, err := config.Redis.RPop(ctx, queueKey).Result()
	require.NoError(t, err)

	var job SyncJob
	require.NoError(t, json.Unmarshal([]byte(raw), &job))
	assert.Equal(t, MobilidadeInviteEmailQueue, job.Type)
	assert.Equal(t, link.ID.Hex(), job.Key)
	assert.Equal(t, 3, job.MaxRetries)
	assert.Equal(t, models.InviteEmailStatusQueued, link.EmailStatus)

	// Persist status should be queued on the document too.
	var stored models.VehicleConductor
	require.NoError(t, config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection).FindOne(ctx, bson.M{"_id": link.ID}).Decode(&stored))
	assert.Equal(t, models.InviteEmailStatusQueued, stored.EmailStatus)

	payload, err := parseMobilidadeInviteEmailPayload(job.Data)
	require.NoError(t, err)
	assert.Equal(t, "joao@example.com", payload.NotifyEmail)
	assert.Equal(t, link.ID.Hex(), payload.ConductorID)
	assert.Equal(t, created.ID.Hex(), payload.VehicleID)
	assert.Equal(t, "Ana Souza", payload.OwnerName)
}

func TestResolveDefaultEmailSender_LoggingWhenUnconfigured(t *testing.T) {
	prevSender := DefaultEmailSender
	prevCfg := config.AppConfig
	defer func() {
		DefaultEmailSender = prevSender
		config.AppConfig = prevCfg
	}()

	DefaultEmailSender = nil
	config.AppConfig = &config.Config{}
	sender := ResolveDefaultEmailSender(nil)
	_, ok := sender.(*LoggingEmailSender)
	assert.True(t, ok)
}

func TestResolveDefaultEmailSender_DataRelayWhenConfigured(t *testing.T) {
	prevSender := DefaultEmailSender
	prevCfg := config.AppConfig
	defer func() {
		DefaultEmailSender = prevSender
		config.AppConfig = prevCfg
	}()

	DefaultEmailSender = nil
	config.AppConfig = &config.Config{
		DataRelayBaseURL: "https://relay.example.com",
		DataRelayAPIKey:  "secret",
		DataRelayTimeout: time.Second,
	}
	sender := ResolveDefaultEmailSender(nil)
	_, ok := sender.(*DataRelayEmailSender)
	assert.True(t, ok)
}

func TestResolveDefaultEmailSender_OverrideWins(t *testing.T) {
	prevSender := DefaultEmailSender
	prevCfg := config.AppConfig
	defer func() {
		DefaultEmailSender = prevSender
		config.AppConfig = prevCfg
	}()

	rec := &RecordingEmailSender{}
	DefaultEmailSender = rec
	config.AppConfig = &config.Config{
		DataRelayBaseURL: "https://relay.example.com",
		DataRelayAPIKey:  "secret",
	}
	assert.Equal(t, rec, ResolveDefaultEmailSender(nil))
}

func TestDataRelayEmailSender_Send(t *testing.T) {
	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/data/mailman", r.URL.Path)
		assert.Equal(t, "k", r.Header.Get("X-Api-Key"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := clients.NewDataRelayClient(srv.URL, "k", time.Second)
	sender := NewDataRelayEmailSender(client, nil)
	err := sender.Send(context.Background(), EmailMessage{
		To:      "joao@example.com",
		Subject: "Convite",
		Body:    "texto",
	})
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"joao@example.com"}, got["to_addresses"])
	assert.Equal(t, "Convite", got["subject"])
	assert.Equal(t, false, got["is_html_body"])
}
