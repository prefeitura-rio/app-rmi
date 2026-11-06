package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/prefeitura-rio/app-rmi/internal/config"
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
)

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
