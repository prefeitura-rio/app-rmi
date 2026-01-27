package utils

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRedisForTest initializes Redis client for testing
func setupRedisForTest(t *testing.T) func() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("Skipping Redis integration tests: REDIS_ADDR not set")
	}

	// Initialize Redis client directly (no logging needed for tests)
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Wrap with traced client
	config.Redis = redisclient.NewClient(singleClient)

	// Test connection
	ctx := context.Background()
	err := config.Redis.Ping(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Return cleanup function
	return func() {
		// Clean up test keys
		ctx := context.Background()
		keys, _ := config.Redis.Keys(ctx, "test:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}
		// Note: redisclient.Client doesn't have Close method, the underlying client manages its own lifecycle
	}
}

func TestNewRedisPipeline(t *testing.T) {
	ctx := context.Background()

	t.Run("Create new pipeline", func(t *testing.T) {
		pipeline := NewRedisPipeline(ctx)
		assert.NotNil(t, pipeline, "NewRedisPipeline should return non-nil pipeline")
		assert.Equal(t, ctx, pipeline.ctx, "Pipeline should store the context")
	})

	t.Run("Create pipeline with background context", func(t *testing.T) {
		bgCtx := context.Background()
		pipeline := NewRedisPipeline(bgCtx)
		assert.NotNil(t, pipeline, "NewRedisPipeline should work with background context")
	})

	t.Run("Create pipeline with cancelled context", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately
		pipeline := NewRedisPipeline(cancelCtx)
		assert.NotNil(t, pipeline, "NewRedisPipeline should work with cancelled context")
	})
}

func TestRedisPipeline_BatchSet(t *testing.T) {
	cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()
	pipeline := NewRedisPipeline(ctx)

	t.Run("Set multiple key-value pairs", func(t *testing.T) {
		keyValues := map[string]interface{}{
			"test:key1": "value1",
			"test:key2": "value2",
			"test:key3": "value3",
		}

		err := pipeline.BatchSet(keyValues, 10*time.Second)
		require.NoError(t, err, "BatchSet should not error")

		// Verify all keys were set
		for key, expectedValue := range keyValues {
			val, err := config.Redis.Get(ctx, key).Result()
			require.NoError(t, err, "Failed to get key %s", key)
			assert.Equal(t, expectedValue, val, "Key %s should have correct value", key)
		}
	})

	t.Run("Set with TTL", func(t *testing.T) {
		keyValues := map[string]interface{}{
			"test:ttl1": "expiring_value",
		}

		err := pipeline.BatchSet(keyValues, 2*time.Second)
		require.NoError(t, err, "BatchSet should not error")

		// Verify key exists
		val, err := config.Redis.Get(ctx, "test:ttl1").Result()
		require.NoError(t, err, "Failed to get key")
		assert.Equal(t, "expiring_value", val, "Key should have correct value")

		// Verify TTL was set
		ttl, err := config.Redis.TTL(ctx, "test:ttl1").Result()
		require.NoError(t, err, "Failed to get TTL")
		assert.Greater(t, ttl, time.Duration(0), "TTL should be positive")
		assert.LessOrEqual(t, ttl, 2*time.Second, "TTL should not exceed set value")
	})

	t.Run("Empty map", func(t *testing.T) {
		err := pipeline.BatchSet(map[string]interface{}{}, 10*time.Second)
		assert.NoError(t, err, "BatchSet with empty map should not error")
	})

	t.Run("Set with different value types", func(t *testing.T) {
		keyValues := map[string]interface{}{
			"test:string": "string_value",
			"test:number": 12345,
			"test:float":  3.14159,
		}

		err := pipeline.BatchSet(keyValues, 10*time.Second)
		require.NoError(t, err, "BatchSet with different types should not error")

		// Verify values were set (they'll be stored as strings in Redis)
		for key := range keyValues {
			exists, err := config.Redis.Exists(ctx, key).Result()
			require.NoError(t, err, "Exists check should not error")
			assert.Equal(t, int64(1), exists, "Key %s should exist", key)
		}
	})

	t.Run("Set large batch", func(t *testing.T) {
		keyValues := make(map[string]interface{})
		for i := 0; i < 100; i++ {
			keyValues[fmt.Sprintf("test:batch:%d", i)] = fmt.Sprintf("value_%d", i)
		}

		err := pipeline.BatchSet(keyValues, 10*time.Second)
		require.NoError(t, err, "BatchSet with large batch should not error")

		// Verify all keys were set
		for key := range keyValues {
			exists, err := config.Redis.Exists(ctx, key).Result()
			require.NoError(t, err, "Exists check should not error")
			assert.Equal(t, int64(1), exists, "Key %s should exist", key)
		}
	})
}

func TestRedisPipeline_BatchGet(t *testing.T) {
	cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()
	pipeline := NewRedisPipeline(ctx)

	t.Run("Get multiple values", func(t *testing.T) {
		// Setup test data
		testData := map[string]interface{}{
			"test:get1": "value1",
			"test:get2": "value2",
			"test:get3": "value3",
		}
		err := pipeline.BatchSet(testData, 10*time.Second)
		require.NoError(t, err, "BatchSet should not error during setup")

		// Get values
		keys := []string{"test:get1", "test:get2", "test:get3"}
		results, err := pipeline.BatchGet(keys)
		require.NoError(t, err, "BatchGet should not error")

		assert.Len(t, results, 3, "Should return 3 results")

		for key, expectedValue := range testData {
			assert.Equal(t, expectedValue, results[key], "Key %s should have correct value", key)
		}
	})

	t.Run("Get with non-existent keys", func(t *testing.T) {
		keys := []string{"test:nonexistent1", "test:nonexistent2"}
		results, err := pipeline.BatchGet(keys)
		require.NoError(t, err, "BatchGet should not error for non-existent keys")

		// Non-existent keys should not be in results
		for _, key := range keys {
			_, exists := results[key]
			assert.False(t, exists, "Non-existent key %s should not be in results", key)
		}
	})

	t.Run("Empty key list", func(t *testing.T) {
		results, err := pipeline.BatchGet([]string{})
		require.NoError(t, err, "BatchGet with empty list should not error")
		assert.Empty(t, results, "BatchGet with empty list should return empty map")
	})

	t.Run("Get mixed existent and non-existent keys", func(t *testing.T) {
		// Setup test data
		testData := map[string]interface{}{
			"test:mixed1": "value1",
			"test:mixed3": "value3",
		}
		err := pipeline.BatchSet(testData, 10*time.Second)
		require.NoError(t, err, "BatchSet should not error during setup")

		// Get mixed keys (some exist, some don't)
		keys := []string{"test:mixed1", "test:mixed2", "test:mixed3", "test:mixed4"}
		results, err := pipeline.BatchGet(keys)
		require.NoError(t, err, "BatchGet should not error")

		assert.Len(t, results, 2, "Should return only existing keys")
		assert.Equal(t, "value1", results["test:mixed1"], "test:mixed1 should have correct value")
		assert.Equal(t, "value3", results["test:mixed3"], "test:mixed3 should have correct value")
		_, exists := results["test:mixed2"]
		assert.False(t, exists, "test:mixed2 should not be in results")
		_, exists = results["test:mixed4"]
		assert.False(t, exists, "test:mixed4 should not be in results")
	})

	t.Run("Get large batch", func(t *testing.T) {
		// Setup test data
		testData := make(map[string]interface{})
		for i := 0; i < 100; i++ {
			testData[fmt.Sprintf("test:getbatch:%d", i)] = fmt.Sprintf("value_%d", i)
		}
		err := pipeline.BatchSet(testData, 10*time.Second)
		require.NoError(t, err, "BatchSet should not error during setup")

		// Get all keys
		keys := make([]string, 0, 100)
		for key := range testData {
			keys = append(keys, key)
		}
		results, err := pipeline.BatchGet(keys)
		require.NoError(t, err, "BatchGet should not error")

		assert.Len(t, results, 100, "Should return all 100 keys")
	})
}

func TestRedisPipeline_BatchDelete(t *testing.T) {
	cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()
	pipeline := NewRedisPipeline(ctx)

	t.Run("Delete multiple keys", func(t *testing.T) {
		// Setup test data
		testData := map[string]interface{}{
			"test:del1": "value1",
			"test:del2": "value2",
			"test:del3": "value3",
		}
		err := pipeline.BatchSet(testData, 10*time.Second)
		require.NoError(t, err, "BatchSet should not error during setup")

		// Verify keys exist
		for key := range testData {
			exists, err := config.Redis.Exists(ctx, key).Result()
			require.NoError(t, err, "Exists check should not error")
			assert.Equal(t, int64(1), exists, "Key %s should exist after setup", key)
		}

		// Delete keys
		keys := []string{"test:del1", "test:del2", "test:del3"}
		err = pipeline.BatchDelete(keys)
		require.NoError(t, err, "BatchDelete should not error")

		// Verify keys were deleted
		for _, key := range keys {
			exists, err := config.Redis.Exists(ctx, key).Result()
			require.NoError(t, err, "Exists check should not error")
			assert.Equal(t, int64(0), exists, "Key %s should be deleted", key)
		}
	})

	t.Run("Delete non-existent keys", func(t *testing.T) {
		keys := []string{"test:noexist1", "test:noexist2"}
		err := pipeline.BatchDelete(keys)
		assert.NoError(t, err, "BatchDelete of non-existent keys should not error")
	})

	t.Run("Empty key list", func(t *testing.T) {
		err := pipeline.BatchDelete([]string{})
		assert.NoError(t, err, "BatchDelete with empty list should not error")
	})

	t.Run("Delete large batch", func(t *testing.T) {
		// Setup test data
		testData := make(map[string]interface{})
		for i := 0; i < 100; i++ {
			testData[fmt.Sprintf("test:delbatch:%d", i)] = fmt.Sprintf("value_%d", i)
		}
		err := pipeline.BatchSet(testData, 10*time.Second)
		require.NoError(t, err, "BatchSet should not error during setup")

		// Delete all keys
		keys := make([]string, 0, 100)
		for key := range testData {
			keys = append(keys, key)
		}
		err = pipeline.BatchDelete(keys)
		require.NoError(t, err, "BatchDelete should not error")

		// Verify all keys were deleted
		for _, key := range keys {
			exists, err := config.Redis.Exists(ctx, key).Result()
			require.NoError(t, err, "Exists check should not error")
			assert.Equal(t, int64(0), exists, "Key %s should be deleted", key)
		}
	})
}

func TestRedisPipeline_BatchExpire(t *testing.T) {
	cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()
	pipeline := NewRedisPipeline(ctx)

	t.Run("Set expiration on multiple keys", func(t *testing.T) {
		// Setup test data without expiration
		testData := map[string]interface{}{
			"test:exp1": "value1",
			"test:exp2": "value2",
		}
		err := pipeline.BatchSet(testData, 0) // No expiration initially
		require.NoError(t, err, "BatchSet should not error during setup")

		// Set expiration
		keys := []string{"test:exp1", "test:exp2"}
		err = pipeline.BatchExpire(keys, 5*time.Second)
		require.NoError(t, err, "BatchExpire should not error")

		// Verify TTL was set
		for _, key := range keys {
			ttl, err := config.Redis.TTL(ctx, key).Result()
			require.NoError(t, err, "Failed to get TTL for %s", key)
			assert.Greater(t, ttl, time.Duration(0), "Key %s TTL should be positive", key)
			assert.LessOrEqual(t, ttl, 5*time.Second, "Key %s TTL should not exceed set value", key)
		}
	})

	t.Run("Empty key list", func(t *testing.T) {
		err := pipeline.BatchExpire([]string{}, 5*time.Second)
		assert.NoError(t, err, "BatchExpire with empty list should not error")
	})
}

func TestRedisPipeline_BatchExists(t *testing.T) {
	cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()
	pipeline := NewRedisPipeline(ctx)

	t.Run("Check existence of multiple keys", func(t *testing.T) {
		// Setup some test data
		testData := map[string]interface{}{
			"test:exists1": "value1",
			"test:exists2": "value2",
		}
		err := pipeline.BatchSet(testData, 10*time.Second)
		require.NoError(t, err, "BatchSet should not error during setup")

		// Check existence
		keys := []string{"test:exists1", "test:exists2", "test:notexists"}
		results, err := pipeline.BatchExists(keys)
		require.NoError(t, err, "BatchExists should not error")

		assert.True(t, results["test:exists1"], "test:exists1 should exist")
		assert.True(t, results["test:exists2"], "test:exists2 should exist")
		assert.False(t, results["test:notexists"], "test:notexists should not exist")
	})

	t.Run("Empty key list", func(t *testing.T) {
		results, err := pipeline.BatchExists([]string{})
		require.NoError(t, err, "BatchExists with empty list should not error")
		assert.Empty(t, results, "BatchExists with empty list should return empty map")
	})
}

func TestRedisPipeline_BatchIncr(t *testing.T) {
	cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()
	pipeline := NewRedisPipeline(ctx)

	t.Run("Increment multiple keys", func(t *testing.T) {
		// Setup initial values
		err := config.Redis.Set(ctx, "test:incr1", 10, 0).Err()
		require.NoError(t, err, "Set test:incr1 should not error")
		err = config.Redis.Set(ctx, "test:incr2", 20, 0).Err()
		require.NoError(t, err, "Set test:incr2 should not error")

		// Increment keys
		keys := []string{"test:incr1", "test:incr2", "test:incr3"} // incr3 doesn't exist
		results, err := pipeline.BatchIncr(keys)
		require.NoError(t, err, "BatchIncr should not error")

		assert.Equal(t, int64(11), results["test:incr1"], "test:incr1 should be incremented to 11")
		assert.Equal(t, int64(21), results["test:incr2"], "test:incr2 should be incremented to 21")
		assert.Equal(t, int64(1), results["test:incr3"], "test:incr3 should be incremented from 0 to 1")
	})

	t.Run("Empty key list", func(t *testing.T) {
		results, err := pipeline.BatchIncr([]string{})
		require.NoError(t, err, "BatchIncr with empty list should not error")
		assert.Empty(t, results, "BatchIncr with empty list should return empty map")
	})
}
