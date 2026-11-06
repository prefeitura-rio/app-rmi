package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
)

const (
	// avatarCleanupJobType marks a user_config sync job as an avatar cleanup (data.type).
	avatarCleanupJobType = "avatar_cleanup"
	// avatarCleanupQueue is the sync queue the cleanup job goes to; the worker consumes it,
	// and retries and the DLQ use the same name (job.Type).
	avatarCleanupQueue = "user_config"
	// avatarCleanupMaxRetries is the number of attempts before the cleanup job goes to the DLQ.
	avatarCleanupMaxRetries = 3
	// legacyAvatarCleanupListKey is the Redis list older versions pushed raw
	// {type, avatar_id, timestamp} payloads to. No worker consumes it.
	legacyAvatarCleanupListKey = "user_config:write"
	// legacyAvatarCleanupMigrateBatch bounds how many legacy payloads one worker start inspects.
	legacyAvatarCleanupMigrateBatch = 1000
	// userConfigDataType is the DataManager type of user configs: its cache keys are
	// user_config:cache:<cpf> and user_config:write:<cpf>.
	userConfigDataType = "user_config"
	// avatarCleanupBatchSize bounds how many user configs one cleanup step resets.
	avatarCleanupBatchSize = 500
	// avatarDeletedMarkerTTL keeps the deleted-avatar marker longer than a write buffer (6h)
	// and the retries of a pending user_config job, so none of them writes the avatar back.
	avatarDeletedMarkerTTL = 24 * time.Hour
	// writeBufferCASAttempts bounds the compare-and-set retries on a write buffer
	// that the API keeps rewriting.
	writeBufferCASAttempts = 3
	// avatarCleanupJobTimeout replaces the generic sync job timeout: one cleanup may reset
	// many user configs and invalidate their caches.
	avatarCleanupJobTimeout = 5 * time.Minute
	// avatarCacheInvalidationTimeout bounds the invalidation of one batch already reset in MongoDB.
	avatarCacheInvalidationTimeout = 30 * time.Second
)

// luaCompareAndSetKeepTTL replaces KEYS[1] by ARGV[2] only while it still holds ARGV[1],
// keeping its remaining TTL. Returns 1 when replaced, 0 otherwise.
const luaCompareAndSetKeepTTL = `
local current = redis.call('GET', KEYS[1])
if current ~= ARGV[1] then
  return 0
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl > 0 then
  redis.call('SET', KEYS[1], ARGV[2], 'PX', ttl)
else
  redis.call('SET', KEYS[1], ARGV[2])
end
return 1
`

// isAvatarCleanupJob reports whether a sync job is an avatar cleanup (data.type).
func isAvatarCleanupJob(job *SyncJob) bool {
	data, ok := job.Data.(map[string]interface{})
	return ok && data["type"] == avatarCleanupJobType
}

// avatarDeletedMarkerKey is set by the cleanup of a deleted avatar; while it exists, user_config
// sync jobs drop that avatar_id instead of writing it back to MongoDB and to the read cache.
func avatarDeletedMarkerKey(avatarID string) string {
	return "avatar_cleanup:deleted:" + avatarID
}

// stripAvatarFromUserConfigJSON removes avatar_id from a buffered user config JSON when it
// references avatarID. Other fields are kept byte for byte. It returns false when the
// document does not reference avatarID.
func stripAvatarFromUserConfigJSON(raw string, avatarID string) (string, bool, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", false, fmt.Errorf("invalid user config JSON: %w", err)
	}
	current, ok := doc["avatar_id"]
	if !ok {
		return "", false, nil
	}
	var currentID *string
	if err := json.Unmarshal(current, &currentID); err != nil || currentID == nil || *currentID != avatarID {
		return "", false, nil
	}
	delete(doc, "avatar_id")
	stripped, err := json.Marshal(doc)
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal user config JSON: %w", err)
	}
	return string(stripped), true, nil
}

// dropDeletedAvatarFromUserConfigJob removes avatar_id from a user_config sync job whose avatar
// was deleted (marker set by the cleanup), so a job queued before the cleanup does not write
// the avatar back to MongoDB nor to the read cache (handleSyncSuccess caches job.Data).
// It returns true when the avatar was dropped.
func (w *SyncWorker) dropDeletedAvatarFromUserConfigJob(ctx context.Context, job *SyncJob, bsonData map[string]interface{}) (bool, error) {
	avatarID, ok := bsonData["avatar_id"].(string)
	if !ok || avatarID == "" {
		return false, nil
	}
	deleted, err := w.redis.Exists(ctx, avatarDeletedMarkerKey(avatarID)).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check deleted avatar marker: %w", err)
	}
	if deleted == 0 {
		return false, nil
	}
	delete(bsonData, "avatar_id")
	if data, ok := job.Data.(map[string]interface{}); ok {
		delete(data, "avatar_id")
	}
	w.logger.Info("dropped deleted avatar from user_config sync job",
		zap.String("job_id", job.ID),
		zap.String("avatar_id", avatarID),
		observability.KeyField(job.Key))
	return true, nil
}

// userConfigRef is the part of a user config the avatar cleanup reads.
type userConfigRef struct {
	ID  interface{} `bson:"_id"`
	CPF string      `bson:"cpf"`
}

// resetUserConfigsForDeletedAvatar unsets avatar_id on every user config that references
// avatarID and invalidates the Redis copies of each affected user config so no read returns
// the deleted avatar. One cursor reads the collection once (avatar_id has no index); the
// updates go in batches by _id. It returns how many documents were modified.
func (w *SyncWorker) resetUserConfigsForDeletedAvatar(ctx context.Context, avatarID string) (int64, error) {
	collection := w.mongo.Collection(config.AppConfig.UserConfigCollection)
	cursor, err := collection.Find(ctx, bson.M{"avatar_id": avatarID}, options.Find().
		SetProjection(bson.M{"_id": 1, "cpf": 1}).
		SetBatchSize(avatarCleanupBatchSize))
	if err != nil {
		return 0, fmt.Errorf("failed to find user configs of deleted avatar: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()

	var modified int64
	batch := make([]userConfigRef, 0, avatarCleanupBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		ids := make([]interface{}, 0, len(batch))
		for _, ref := range batch {
			ids = append(ids, ref.ID)
		}
		result, err := collection.UpdateMany(ctx,
			bson.M{"_id": bson.M{"$in": ids}, "avatar_id": avatarID},
			bson.M{
				"$unset": bson.M{"avatar_id": ""},
				"$set":   bson.M{"updated_at": time.Now()},
			})
		if err != nil {
			return fmt.Errorf("failed to cleanup avatar references: %w", err)
		}
		modified += result.ModifiedCount

		// Invalidate only after MongoDB is clean, so a concurrent read cannot cache the old value again.
		w.invalidateUserConfigAvatarCaches(ctx, batch, avatarID)
		batch = batch[:0]
		return nil
	}

	for cursor.Next(ctx) {
		var ref userConfigRef
		if err := cursor.Decode(&ref); err != nil {
			return modified, fmt.Errorf("failed to read user config of deleted avatar: %w", err)
		}
		batch = append(batch, ref)
		if len(batch) == avatarCleanupBatchSize {
			if err := flush(); err != nil {
				return modified, err
			}
		}
	}
	if err := cursor.Err(); err != nil {
		return modified, fmt.Errorf("failed to read user configs of deleted avatar: %w", err)
	}
	return modified, flush()
}

// invalidateUserConfigAvatarCaches invalidates the caches of a batch already reset in MongoDB.
// It runs detached from the job deadline: a retry would not find these documents again, so an
// interrupted invalidation would leave their caches with the deleted avatar until they expire.
func (w *SyncWorker) invalidateUserConfigAvatarCaches(ctx context.Context, batch []userConfigRef, avatarID string) {
	invalidateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), avatarCacheInvalidationTimeout)
	defer cancel()
	for _, ref := range batch {
		if ref.CPF != "" {
			w.invalidateUserConfigAvatarCache(invalidateCtx, ref.CPF, avatarID)
		}
	}
}

// invalidateUserConfigAvatarCache deletes user_config:cache:<cpf> and removes avatarID from a
// pending user_config:write:<cpf>, keeping the other buffered fields and the buffer TTL.
// Failures are logged: the read cache expires on its own (3h) and the pending job drops the avatar.
func (w *SyncWorker) invalidateUserConfigAvatarCache(ctx context.Context, cpf string, avatarID string) {
	cacheKey := fmt.Sprintf("%s:cache:%s", userConfigDataType, cpf)
	if err := w.redis.Del(ctx, cacheKey).Err(); err != nil {
		w.logger.Error("failed to invalidate user config cache after avatar cleanup",
			zap.String("avatar_id", avatarID),
			observability.CacheKeyField(cacheKey),
			zap.Error(err))
	}

	writeKey := fmt.Sprintf("%s:write:%s", userConfigDataType, cpf)
	for attempt := 0; attempt < writeBufferCASAttempts; attempt++ {
		raw, err := w.redis.Get(ctx, writeKey).Result()
		if errors.Is(err, redis.Nil) {
			return
		}
		if err != nil {
			w.logger.Error("failed to read user config write buffer after avatar cleanup",
				zap.String("avatar_id", avatarID),
				observability.CacheKeyField(writeKey),
				zap.Error(err))
			return
		}
		stripped, changed, err := stripAvatarFromUserConfigJSON(raw, avatarID)
		if err != nil {
			w.logger.Warn("unreadable user config write buffer left as is after avatar cleanup",
				zap.String("avatar_id", avatarID),
				observability.CacheKeyField(writeKey),
				zap.Error(err))
			return
		}
		if !changed {
			return
		}
		replaced, err := w.redis.Eval(ctx, luaCompareAndSetKeepTTL, []string{writeKey}, raw, stripped).Int()
		if err != nil {
			w.logger.Error("failed to update user config write buffer after avatar cleanup",
				zap.String("avatar_id", avatarID),
				observability.CacheKeyField(writeKey),
				zap.Error(err))
			return
		}
		if replaced == 1 {
			return
		}
		// The API rewrote the buffer meanwhile: read it again.
	}
	w.logger.Warn("user config write buffer kept changing during avatar cleanup",
		zap.String("avatar_id", avatarID),
		observability.CacheKeyField(writeKey))
}

// newAvatarCleanupSyncJob builds the SyncJob that resets user configs referencing a deleted avatar.
func newAvatarCleanupSyncJob(avatarID string, now time.Time) SyncJob {
	collection := "user_config"
	if config.AppConfig != nil && config.AppConfig.UserConfigCollection != "" {
		collection = config.AppConfig.UserConfigCollection
	}
	return SyncJob{
		ID:         uuid.New().String(),
		Type:       avatarCleanupQueue,
		Key:        avatarID,
		Collection: collection,
		Data: map[string]interface{}{
			"type":      avatarCleanupJobType,
			"avatar_id": avatarID,
			"timestamp": now.Unix(),
		},
		Timestamp:  now,
		RetryCount: 0,
		MaxRetries: avatarCleanupMaxRetries,
	}
}

// enqueueAvatarCleanupJob pushes an avatar cleanup SyncJob to the queue the sync worker consumes.
func enqueueAvatarCleanupJob(ctx context.Context, rdb *redisclient.Client, avatarID string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("redis client not initialized")
	}
	if strings.TrimSpace(avatarID) == "" {
		return "", fmt.Errorf("empty avatar_id for cleanup job")
	}

	job := newAvatarCleanupSyncJob(avatarID, time.Now())
	//nolint:gosec // G117: internal job payload serialization; no token on this job
	jobJSON, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cleanup job: %w", err)
	}

	if err := rdb.LPush(ctx, syncQueueKey(avatarCleanupQueue), string(jobJSON)).Err(); err != nil {
		return "", fmt.Errorf("failed to queue cleanup job: %w", err)
	}
	return job.ID, nil
}

// parseLegacyAvatarCleanupPayload returns the avatar id of a raw payload left in
// legacyAvatarCleanupListKey, or false when the payload is not an avatar cleanup.
func parseLegacyAvatarCleanupPayload(raw string) (string, bool) {
	var payload struct {
		Type     string `json:"type"`
		AvatarID string `json:"avatar_id"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", false
	}
	if payload.Type != avatarCleanupJobType || strings.TrimSpace(payload.AvatarID) == "" {
		return "", false
	}
	return payload.AvatarID, true
}

// migrateLegacyAvatarCleanupJobs moves the avatar cleanup payloads older versions left in
// legacyAvatarCleanupListKey to the sync queue as SyncJobs. Each payload is removed (LREM)
// before its job is queued, so concurrent workers never queue the same payload twice;
// the cleanup itself is idempotent. Unrecognized payloads stay in the list.
func (w *SyncWorker) migrateLegacyAvatarCleanupJobs(ctx context.Context) {
	items, err := w.redis.LRange(ctx, legacyAvatarCleanupListKey, 0, legacyAvatarCleanupMigrateBatch-1).Result()
	if err != nil {
		w.logger.Warn("failed to list legacy avatar cleanup jobs", zap.Error(err))
		return
	}
	if len(items) == 0 {
		return
	}

	migrated, unrecognized := 0, 0
	for _, raw := range items {
		avatarID, ok := parseLegacyAvatarCleanupPayload(raw)
		if !ok {
			unrecognized++
			continue
		}

		removed, err := w.redis.LRem(ctx, legacyAvatarCleanupListKey, 1, raw).Result()
		if err != nil {
			w.logger.Warn("failed to remove legacy avatar cleanup job",
				zap.String("avatar_id", avatarID),
				zap.Error(err))
			continue
		}
		if removed == 0 {
			// Another worker already took this payload.
			continue
		}

		jobID, err := enqueueAvatarCleanupJob(ctx, w.redis, avatarID)
		if err != nil {
			w.logger.Error("failed to queue migrated avatar cleanup job",
				zap.String("avatar_id", avatarID),
				zap.Error(err))
			if pushErr := w.redis.LPush(ctx, legacyAvatarCleanupListKey, raw).Err(); pushErr != nil {
				w.logger.Error("failed to restore legacy avatar cleanup job",
					zap.String("avatar_id", avatarID),
					zap.Error(pushErr))
			}
			continue
		}
		migrated++
		w.logger.Info("legacy avatar cleanup job migrated to sync queue",
			zap.String("avatar_id", avatarID),
			zap.String("job_id", jobID))
	}

	if migrated > 0 || unrecognized > 0 {
		w.logger.Info("legacy avatar cleanup list processed",
			zap.Int("worker_id", w.id),
			zap.Int("inspected", len(items)),
			zap.Int("migrated", migrated),
			zap.Int("unrecognized", unrecognized))
	}
}
