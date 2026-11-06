package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

func TestNewAvatarCleanupSyncJob_IsConsumedFromUserConfigQueue(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	job := newAvatarCleanupSyncJob("avatar-1", now)

	assert.NotEmpty(t, job.ID)
	assert.Equal(t, avatarCleanupQueue, job.Type)
	assert.Equal(t, "avatar-1", job.Key)
	assert.Equal(t, avatarCleanupMaxRetries, job.MaxRetries)
	assert.Equal(t, "sync:queue:user_config", syncQueueKey(job.Type), "retries and DLQ must use the consumed queue")

	worker := NewSyncWorker(nil, nil, 1, nil, nil, nil)
	assert.Contains(t, worker.queues, job.Type, "the worker must consume the queue the job is pushed to")

	// The worker unmarshals the Redis payload into a SyncJob before dispatching it.
	raw, err := json.Marshal(job)
	require.NoError(t, err)
	var decoded SyncJob
	require.NoError(t, json.Unmarshal(raw, &decoded))
	data, ok := decoded.Data.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, avatarCleanupJobType, data["type"])
	assert.Equal(t, "avatar-1", data["avatar_id"])
}

func TestParseLegacyAvatarCleanupPayload(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		wantID string
		wantOK bool
	}{
		{"legacy payload", `{"type":"avatar_cleanup","avatar_id":"abc","timestamp":1700000000}`, "abc", true},
		{"other type", `{"type":"user_config","avatar_id":"abc"}`, "", false},
		{"missing avatar id", `{"type":"avatar_cleanup"}`, "", false},
		{"blank avatar id", `{"type":"avatar_cleanup","avatar_id":"  "}`, "", false},
		{"not json", `not-json`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := parseLegacyAvatarCleanupPayload(tt.raw)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantID, id)
		})
	}
}

func TestEnqueueAvatarCleanupJob_RejectsInvalidInput(t *testing.T) {
	_, err := enqueueAvatarCleanupJob(context.Background(), nil, "abc")
	assert.Error(t, err)
}

// findAvatarCleanupJob returns the queued cleanup job for avatarID, ignoring jobs of other tests.
func findAvatarCleanupJob(t *testing.T, ctx context.Context, worker *SyncWorker, avatarID string) (*SyncJob, string) {
	t.Helper()
	items, err := worker.redis.LRange(ctx, syncQueueKey(avatarCleanupQueue), 0, -1).Result()
	require.NoError(t, err)
	for _, raw := range items {
		var job SyncJob
		if json.Unmarshal([]byte(raw), &job) != nil {
			continue
		}
		data, ok := job.Data.(map[string]interface{})
		if !ok || data["type"] != avatarCleanupJobType || data["avatar_id"] != avatarID {
			continue
		}
		return &job, raw
	}
	return nil, ""
}

func TestDeleteAvatar_QueuesCleanupProcessedBySyncWorker(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()
	service, avatarCleanup := setupAvatarServiceTest(t)
	defer avatarCleanup()

	ctx := context.Background()

	avatar := models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "In use",
		URL:       "http://example.com/in-use.png",
		IsActive:  true,
		CreatedAt: time.Now(),
	}
	_, err := db.Collection(config.AppConfig.AvatarsCollection).InsertOne(ctx, avatar)
	require.NoError(t, err)
	avatarID := avatar.ID.Hex()

	userConfigs := db.Collection(config.AppConfig.UserConfigCollection)
	_, err = userConfigs.InsertMany(ctx, []interface{}{
		bson.M{"cpf": "00000000191", "avatar_id": avatarID},
		bson.M{"cpf": "00000000272", "avatar_id": "other-avatar"},
	})
	require.NoError(t, err)

	legacyBefore, err := worker.redis.LLen(ctx, legacyAvatarCleanupListKey).Result()
	require.NoError(t, err)

	require.NoError(t, service.DeleteAvatar(ctx, avatarID))

	legacyAfter, err := worker.redis.LLen(ctx, legacyAvatarCleanupListKey).Result()
	require.NoError(t, err)
	assert.Equal(t, legacyBefore, legacyAfter, "nothing may be pushed to the unconsumed legacy list")

	job, raw := findAvatarCleanupJob(t, ctx, worker, avatarID)
	require.NotNil(t, job, "cleanup job must be queued on sync:queue:user_config")
	require.NoError(t, worker.redis.LRem(ctx, syncQueueKey(avatarCleanupQueue), 1, raw).Err())

	worker.processJob(job)

	var affected bson.M
	require.NoError(t, userConfigs.FindOne(ctx, bson.M{"cpf": "00000000191"}).Decode(&affected))
	_, hasAvatar := affected["avatar_id"]
	assert.False(t, hasAvatar, "avatar_id of the deleted avatar must be removed")

	var untouched bson.M
	require.NoError(t, userConfigs.FindOne(ctx, bson.M{"cpf": "00000000272"}).Decode(&untouched))
	assert.Equal(t, "other-avatar", untouched["avatar_id"])

	// A special job never creates a user_config document keyed by the avatar id.
	count, err := userConfigs.CountDocuments(ctx, bson.M{"cpf": avatarID})
	require.NoError(t, err)
	assert.Zero(t, count)
	dlqLen, err := worker.redis.LLen(ctx, syncDLQKey(avatarCleanupQueue)).Result()
	require.NoError(t, err)
	assert.Zero(t, dlqLen)
}

func TestMigrateLegacyAvatarCleanupJobs(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	avatarID := primitive.NewObjectID().Hex()
	legacy := `{"avatar_id":"` + avatarID + `","timestamp":1700000000,"type":"avatar_cleanup"}`
	unrecognized := `{"type":"something_else"}`

	require.NoError(t, worker.redis.Del(ctx, legacyAvatarCleanupListKey).Err())
	require.NoError(t, worker.redis.LPush(ctx, legacyAvatarCleanupListKey, legacy, unrecognized).Err())

	worker.migrateLegacyAvatarCleanupJobs(ctx)

	remaining, err := worker.redis.LRange(ctx, legacyAvatarCleanupListKey, 0, -1).Result()
	require.NoError(t, err)
	assert.Equal(t, []string{unrecognized}, remaining, "only unrecognized payloads stay in the legacy list")

	job, raw := findAvatarCleanupJob(t, ctx, worker, avatarID)
	require.NotNil(t, job, "legacy payload must become a SyncJob on sync:queue:user_config")
	assert.Equal(t, avatarCleanupQueue, job.Type)
	assert.Equal(t, avatarID, job.Key)

	// Running again (another worker start) must not queue the same payload twice.
	require.NoError(t, worker.redis.LRem(ctx, syncQueueKey(avatarCleanupQueue), 1, raw).Err())
	worker.migrateLegacyAvatarCleanupJobs(ctx)
	again, _ := findAvatarCleanupJob(t, ctx, worker, avatarID)
	assert.Nil(t, again)

	require.NoError(t, worker.redis.Del(ctx, legacyAvatarCleanupListKey).Err())
}
