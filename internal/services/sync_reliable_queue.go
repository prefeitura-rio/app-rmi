package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Lua scripts keep queue / processing / claim transitions atomic on a single Redis Cluster slot
// (keys share the same hash tag via syncQueueKey / syncProcessingKey / syncProcessingClaimKey).

const luaClaimInflight = `
local payload = redis.call('RPOPLPUSH', KEYS[1], KEYS[2])
if not payload then
  return false
end
redis.call('ZADD', KEYS[3], ARGV[1], payload)
return payload
`

const luaAckInflight = `
redis.call('ZREM', KEYS[2], ARGV[1])
return redis.call('LREM', KEYS[1], 1, ARGV[1])
`

const luaRecoverStaleInflight = `
local score = redis.call('ZSCORE', KEYS[3], ARGV[1])
if score ~= false and tonumber(score) > tonumber(ARGV[2]) then
  return 0
end
local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
redis.call('ZREM', KEYS[3], ARGV[1])
if removed == 0 then
  return 0
end
redis.call('LPUSH', KEYS[2], ARGV[1])
return 1
`

// Move a claimed payload out of processing into another list (queue or DLQ), optionally replacing body.
const luaCompleteInflightToList = `
local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
redis.call('ZREM', KEYS[3], ARGV[1])
if removed == 0 then
  return 0
end
redis.call('LPUSH', KEYS[2], ARGV[2])
return 1
`

// Replace the processing list entry + claim member after a side effect (e.g. email sent)
// so a crash/recovery sees the updated payload without redoing the side effect.
const luaReplaceInflightPayload = `
local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
if removed == 0 then
  return 0
end
redis.call('ZREM', KEYS[2], ARGV[1])
redis.call('LPUSH', KEYS[1], ARGV[2])
redis.call('ZADD', KEYS[2], ARGV[3], ARGV[2])
return 1
`

func (w *SyncWorker) claimReliableJob(ctx context.Context, queue string) (string, error) {
	queueKey := syncQueueKey(queue)
	processingKey := syncProcessingKey(queue)
	claimKey := syncProcessingClaimKey(queue)
	score := fmt.Sprintf("%d", time.Now().Unix())

	res, err := w.redis.Eval(ctx, luaClaimInflight, []string{queueKey, processingKey, claimKey}, score).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	if res == nil || res == false {
		return "", nil
	}
	payload, ok := res.(string)
	if !ok {
		return "", fmt.Errorf("claim inflight: unexpected result type %T", res)
	}
	return payload, nil
}

func (w *SyncWorker) ackReliableJob(ctx context.Context, queue, payload string) error {
	if payload == "" {
		return nil
	}
	processingKey := syncProcessingKey(queue)
	claimKey := syncProcessingClaimKey(queue)
	return w.redis.Eval(ctx, luaAckInflight, []string{processingKey, claimKey}, payload).Err()
}

func (w *SyncWorker) recoverStaleInflightItem(ctx context.Context, queue, payload string, cutoffUnix int64) (bool, error) {
	processingKey := syncProcessingKey(queue)
	queueKey := syncQueueKey(queue)
	claimKey := syncProcessingClaimKey(queue)

	res, err := w.redis.Eval(ctx, luaRecoverStaleInflight,
		[]string{processingKey, queueKey, claimKey},
		payload, fmt.Sprintf("%d", cutoffUnix),
	).Result()
	if err != nil {
		return false, err
	}
	n, ok := res.(int64)
	if !ok {
		// go-redis may return int depending on version
		if i, okInt := res.(int); okInt {
			return i == 1, nil
		}
		return false, fmt.Errorf("recover inflight: unexpected result type %T", res)
	}
	return n == 1, nil
}

func (w *SyncWorker) completeReliableJobToList(ctx context.Context, queue, destKey, oldPayload, newPayload string) (bool, error) {
	processingKey := syncProcessingKey(queue)
	claimKey := syncProcessingClaimKey(queue)
	res, err := w.redis.Eval(ctx, luaCompleteInflightToList,
		[]string{processingKey, destKey, claimKey},
		oldPayload, newPayload,
	).Result()
	if err != nil {
		return false, err
	}
	switch v := res.(type) {
	case int64:
		return v == 1, nil
	case int:
		return v == 1, nil
	default:
		return false, fmt.Errorf("complete inflight: unexpected result type %T", res)
	}
}

func (w *SyncWorker) replaceReliableInflightPayload(ctx context.Context, queue, oldPayload, newPayload string) error {
	processingKey := syncProcessingKey(queue)
	claimKey := syncProcessingClaimKey(queue)
	score := fmt.Sprintf("%d", time.Now().Unix())
	res, err := w.redis.Eval(ctx, luaReplaceInflightPayload,
		[]string{processingKey, claimKey},
		oldPayload, newPayload, score,
	).Result()
	if err != nil {
		return err
	}
	switch v := res.(type) {
	case int64:
		if v != 1 {
			return fmt.Errorf("replace inflight payload: item not found in processing")
		}
	case int:
		if v != 1 {
			return fmt.Errorf("replace inflight payload: item not found in processing")
		}
	default:
		return fmt.Errorf("replace inflight payload: unexpected result type %T", res)
	}
	return nil
}
