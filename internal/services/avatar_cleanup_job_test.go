package services

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestStripAvatarFromUserConfigJSON(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantChanged bool
		wantErr     bool
	}{
		{"references deleted avatar", `{"cpf":"00000000191","opt_in":false,"avatar_id":"del","category_opt_ins":{"a":true}}`, true, false},
		{"other avatar", `{"cpf":"00000000191","avatar_id":"other"}`, false, false},
		{"null avatar", `{"cpf":"00000000191","avatar_id":null}`, false, false},
		{"no avatar", `{"cpf":"00000000191","opt_in":true}`, false, false},
		{"not json", `not-json`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripped, changed, err := stripAvatarFromUserConfigJSON(tt.raw, "del")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantChanged, changed)
			if !changed {
				return
			}
			var doc map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(stripped), &doc))
			_, hasAvatar := doc["avatar_id"]
			assert.False(t, hasAvatar)
			assert.Equal(t, false, doc["opt_in"], "other buffered fields must be kept")
			assert.Equal(t, map[string]interface{}{"a": true}, doc["category_opt_ins"])
		})
	}
}

// userConfigJSON is the JSON DataManager keeps in the user_config read cache and write buffer.
func userConfigJSON(t *testing.T, cfg models.UserConfig) string {
	t.Helper()
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	return string(raw)
}

func TestHandleAvatarCleanup_InvalidatesUserConfigCacheAndWriteBuffer(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	avatarID := primitive.NewObjectID().Hex()
	otherAvatarID := primitive.NewObjectID().Hex()
	const (
		cachedCPF   = "00000000191"
		bufferedCPF = "00000000272"
		otherCPF    = "00000000353"
	)

	userConfigs := db.Collection(config.AppConfig.UserConfigCollection)
	_, err := userConfigs.InsertMany(ctx, []interface{}{
		bson.M{"cpf": cachedCPF, "opt_in": true, "avatar_id": avatarID},
		bson.M{"cpf": bufferedCPF, "opt_in": true, "avatar_id": avatarID},
		bson.M{"cpf": otherCPF, "opt_in": true, "avatar_id": otherAvatarID},
	})
	require.NoError(t, err)

	cachedKey := "user_config:cache:" + cachedCPF
	bufferKey := "user_config:write:" + bufferedCPF
	otherKey := "user_config:cache:" + otherCPF
	require.NoError(t, worker.redis.Set(ctx, cachedKey,
		userConfigJSON(t, models.UserConfig{CPF: cachedCPF, OptIn: true, AvatarID: &avatarID}), 3*time.Hour).Err())
	// Pending write (opt-out not yet synced) carrying the deleted avatar.
	require.NoError(t, worker.redis.Set(ctx, bufferKey,
		userConfigJSON(t, models.UserConfig{CPF: bufferedCPF, OptIn: false, AvatarID: &avatarID}), 6*time.Hour).Err())
	require.NoError(t, worker.redis.Set(ctx, otherKey,
		userConfigJSON(t, models.UserConfig{CPF: otherCPF, OptIn: true, AvatarID: &otherAvatarID}), 3*time.Hour).Err())

	dm := NewDataManager(worker.redis, db, worker.logger)
	readAvatar := func(cpf string) (*string, models.UserConfig) {
		var cfg models.UserConfig
		require.NoError(t, dm.Read(ctx, cpf, config.AppConfig.UserConfigCollection, userConfigDataType, &cfg))
		return cfg.AvatarID, cfg
	}

	// Before the cleanup the stale copies are what reads return.
	before, _ := readAvatar(cachedCPF)
	require.NotNil(t, before)
	require.Equal(t, avatarID, *before)

	worker.processJob(func() *SyncJob { j := newAvatarCleanupSyncJob(avatarID, time.Now()); return &j }())

	after, _ := readAvatar(cachedCPF)
	assert.Nil(t, after, "the read cache of an affected citizen must not return the deleted avatar")

	afterBuffered, buffered := readAvatar(bufferedCPF)
	assert.Nil(t, afterBuffered, "the pending write buffer must not return the deleted avatar")
	assert.False(t, buffered.OptIn, "the pending write must keep its other fields")
	ttl, err := worker.redis.TTL(ctx, bufferKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, 5*time.Hour, "the write buffer keeps its TTL")

	afterOther, _ := readAvatar(otherCPF)
	require.NotNil(t, afterOther)
	assert.Equal(t, otherAvatarID, *afterOther, "citizens with another avatar are untouched")
	exists, err := worker.redis.Exists(ctx, otherKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists, "the cache of an unaffected citizen is kept")

	count, err := userConfigs.CountDocuments(ctx, bson.M{"avatar_id": avatarID})
	require.NoError(t, err)
	assert.Zero(t, count)

	dlqLen, err := worker.redis.LLen(ctx, syncDLQKey(avatarCleanupQueue)).Result()
	require.NoError(t, err)
	assert.Zero(t, dlqLen)
}

func TestHandleAvatarCleanup_ResetsEveryBatch(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	avatarID := primitive.NewObjectID().Hex()
	userConfigs := db.Collection(config.AppConfig.UserConfigCollection)

	total := avatarCleanupBatchSize + 7
	docs := make([]interface{}, 0, total)
	for i := 0; i < total; i++ {
		docs = append(docs, bson.M{"cpf": fmt.Sprintf("9%010d", i), "avatar_id": avatarID})
	}
	// A document without cpf must not stall the batches.
	docs = append(docs, bson.M{"avatar_id": avatarID})
	_, err := userConfigs.InsertMany(ctx, docs)
	require.NoError(t, err)
	lastKey := fmt.Sprintf("user_config:cache:9%010d", total-1)
	require.NoError(t, worker.redis.Set(ctx, lastKey, `{"avatar_id":"`+avatarID+`"}`, time.Hour).Err())

	modified, err := worker.resetUserConfigsForDeletedAvatar(ctx, avatarID)
	require.NoError(t, err)
	assert.Equal(t, int64(total+1), modified)

	count, err := userConfigs.CountDocuments(ctx, bson.M{"avatar_id": avatarID})
	require.NoError(t, err)
	assert.Zero(t, count)
	exists, err := worker.redis.Exists(ctx, lastKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "caches of the last batch are invalidated too")
}

func TestUserConfigSyncJob_DropsAvatarDeletedBeforeSync(t *testing.T) {
	worker, db, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	avatarID := primitive.NewObjectID().Hex()
	const cpf = "00000000434"

	// The API queued a user_config write that still carries the avatar; the cleanup ran first.
	dm := NewDataManager(worker.redis, db, worker.logger)
	require.NoError(t, dm.Write(ctx, &UserConfigDataOperation{
		UserID: cpf,
		Data:   &models.UserConfig{CPF: cpf, OptIn: false, AvatarID: &avatarID, UpdatedAt: time.Now()},
	}))
	require.NoError(t, worker.handleAvatarCleanup(ctx, map[string]interface{}{"avatar_id": avatarID}))

	var job *SyncJob
	items, err := worker.redis.LRange(ctx, syncQueueKey(userConfigDataType), 0, -1).Result()
	require.NoError(t, err)
	for _, raw := range items {
		var candidate SyncJob
		if json.Unmarshal([]byte(raw), &candidate) == nil && candidate.Key == cpf {
			job = &candidate
			require.NoError(t, worker.redis.LRem(ctx, syncQueueKey(userConfigDataType), 1, raw).Err())
			break
		}
	}
	require.NotNil(t, job, "the user_config write must be queued")

	worker.processJob(job)

	var stored bson.M
	require.NoError(t, db.Collection(config.AppConfig.UserConfigCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&stored))
	_, hasAvatar := stored["avatar_id"]
	assert.False(t, hasAvatar, "a pending job must not write the deleted avatar back to MongoDB")
	assert.Equal(t, false, stored["opt_in"], "the rest of the pending write is synced")

	var cfg models.UserConfig
	require.NoError(t, dm.Read(ctx, cpf, config.AppConfig.UserConfigCollection, userConfigDataType, &cfg))
	assert.Nil(t, cfg.AvatarID, "the read cache refreshed by the sync must not carry the deleted avatar")

	// A user_config job with an avatar that was not deleted keeps it.
	keptAvatar := primitive.NewObjectID().Hex()
	keptJob := &SyncJob{
		ID: "kept", Type: userConfigDataType, Key: "00000000515", Collection: config.AppConfig.UserConfigCollection,
		Data: map[string]interface{}{"cpf": "00000000515", "avatar_id": keptAvatar}, MaxRetries: 3,
	}
	worker.processJob(keptJob)
	var kept bson.M
	require.NoError(t, db.Collection(config.AppConfig.UserConfigCollection).FindOne(ctx, bson.M{"cpf": "00000000515"}).Decode(&kept))
	assert.Equal(t, keptAvatar, kept["avatar_id"])
}

func TestSyncJobTimeout_AvatarCleanupHasItsOwnBudget(t *testing.T) {
	job := newAvatarCleanupSyncJob("avatar-1", time.Now())
	assert.Equal(t, avatarCleanupJobTimeout, syncJobTimeout(&job))
	assert.Greater(t, syncJobTimeout(&job), defaultSyncJobTimeout)

	userConfigWrite := &SyncJob{Type: userConfigDataType, Collection: "user_config",
		Data: map[string]interface{}{"cpf": "00000000191", "avatar_id": "avatar-1"}}
	assert.Equal(t, defaultSyncJobTimeout, syncJobTimeout(userConfigWrite))
}

func TestInvalidateUserConfigAvatarCaches_OutlivesJobDeadline(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	avatarID := primitive.NewObjectID().Hex()
	const cpf = "00000000604"
	cacheKey := "user_config:cache:" + cpf
	require.NoError(t, worker.redis.Set(context.Background(), cacheKey, `{"avatar_id":"`+avatarID+`"}`, time.Hour).Err())

	// The batch is already reset in MongoDB when the job deadline expires.
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	worker.invalidateUserConfigAvatarCaches(expired, []userConfigRef{{CPF: cpf}}, avatarID)

	exists, err := worker.redis.Exists(context.Background(), cacheKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "the cache of a reset user config is invalidated even after the job deadline")
}
