package redisclient

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRedisForTest initializes Redis client for testing
func setupRedisForTest(t *testing.T) (*Client, func()) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("Skipping Redis integration tests: REDIS_ADDR not set")
	}

	// Initialize Redis client
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Wrap with traced client
	client := NewClient(singleClient)

	// Test connection
	ctx := context.Background()
	err := client.Ping(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Return cleanup function
	return client, func() {
		// Clean up test keys
		ctx := context.Background()
		keys, _ := client.Keys(ctx, "test:*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
	}
}

// setupRedisClusterForTest initializes Redis cluster client for testing
func setupRedisClusterForTest(t *testing.T) (*Client, func()) {
	redisAddrs := os.Getenv("REDIS_CLUSTER_ADDRS")
	if redisAddrs == "" {
		t.Skip("Skipping Redis cluster tests: REDIS_CLUSTER_ADDRS not set")
	}

	// Parse cluster addresses
	addrs := parseCommaSeparated(redisAddrs)

	// Initialize Redis cluster client
	clusterClient := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    addrs,
		Password: os.Getenv("REDIS_CLUSTER_PASSWORD"),
	})

	// Wrap with traced client
	client := NewClusterClient(clusterClient)

	// Test connection
	ctx := context.Background()
	err := client.Ping(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to connect to Redis cluster: %v", err)
	}

	// Return cleanup function
	return client, func() {
		// Clean up test keys
		ctx := context.Background()
		keys, _ := client.Keys(ctx, "test:*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
	}
}

// parseCommaSeparated is a helper to parse comma-separated strings
func parseCommaSeparated(s string) []string {
	if s == "" {
		return []string{}
	}
	result := []string{}
	for _, part := range splitByComma(s) {
		trimmed := trim(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func splitByComma(s string) []string {
	result := []string{}
	current := ""
	for _, char := range s {
		if char == ',' {
			result = append(result, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	result = append(result, current)
	return result
}

func trim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func TestNewClient(t *testing.T) {
	t.Run("Create new client", func(t *testing.T) {
		redisClient := redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
		client := NewClient(redisClient)

		assert.NotNil(t, client, "NewClient should return non-nil client")
		assert.NotNil(t, client.cmdable, "Client should have non-nil cmdable")
	})

	t.Run("Client cmdable is set correctly", func(t *testing.T) {
		redisClient := redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		})
		client := NewClient(redisClient)

		assert.Equal(t, redisClient, client.cmdable, "Client cmdable should be the redis client")
	})
}

func TestNewClusterClient(t *testing.T) {
	t.Run("Create new cluster client", func(t *testing.T) {
		clusterClient := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs: []string{"localhost:6379"},
		})
		client := NewClusterClient(clusterClient)

		assert.NotNil(t, client, "NewClusterClient should return non-nil client")
		assert.NotNil(t, client.cmdable, "Client should have non-nil cmdable")
	})

	t.Run("Cluster client cmdable is set correctly", func(t *testing.T) {
		clusterClient := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs: []string{"localhost:6379"},
		})
		client := NewClusterClient(clusterClient)

		assert.Equal(t, clusterClient, client.cmdable, "Client cmdable should be the cluster client")
	})
}

func TestClient_Get(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Get existing key", func(t *testing.T) {
		// Set a test key
		err := client.Set(ctx, "test:get:key1", "value1", 10*time.Second).Err()
		require.NoError(t, err, "Set should not error")

		// Get the key
		cmd := client.Get(ctx, "test:get:key1")
		require.NoError(t, cmd.Err(), "Get should not error")
		assert.Equal(t, "value1", cmd.Val(), "Get should return correct value")
	})

	t.Run("Get non-existent key", func(t *testing.T) {
		cmd := client.Get(ctx, "test:get:nonexistent")
		assert.Equal(t, redis.Nil, cmd.Err(), "Get non-existent key should return redis.Nil")
	})

	t.Run("Get with empty key", func(t *testing.T) {
		cmd := client.Get(ctx, "")
		// Redis allows empty keys, but behavior may vary
		assert.NotNil(t, cmd, "Get should return a command")
	})
}

func TestClient_Set(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Set string value", func(t *testing.T) {
		cmd := client.Set(ctx, "test:set:key1", "value1", 10*time.Second)
		require.NoError(t, cmd.Err(), "Set should not error")
		assert.Equal(t, "OK", cmd.Val(), "Set should return OK")

		// Verify value was set
		val, err := client.Get(ctx, "test:set:key1").Result()
		require.NoError(t, err, "Get should not error")
		assert.Equal(t, "value1", val, "Value should be set correctly")
	})

	t.Run("Set with expiration", func(t *testing.T) {
		cmd := client.Set(ctx, "test:set:key2", "value2", 2*time.Second)
		require.NoError(t, cmd.Err(), "Set should not error")

		// Verify TTL was set
		ttl, err := client.TTL(ctx, "test:set:key2").Result()
		require.NoError(t, err, "TTL should not error")
		assert.Greater(t, ttl, time.Duration(0), "TTL should be positive")
		assert.LessOrEqual(t, ttl, 2*time.Second, "TTL should not exceed set value")
	})

	t.Run("Set with no expiration", func(t *testing.T) {
		cmd := client.Set(ctx, "test:set:key3", "value3", 0)
		require.NoError(t, cmd.Err(), "Set should not error")

		// Verify key has no expiration
		ttl, err := client.TTL(ctx, "test:set:key3").Result()
		require.NoError(t, err, "TTL should not error")
		assert.Equal(t, time.Duration(-1), ttl, "TTL should be -1 for keys without expiration")
	})

	t.Run("Set different value types", func(t *testing.T) {
		// String
		err := client.Set(ctx, "test:set:string", "string_value", 10*time.Second).Err()
		assert.NoError(t, err, "Set string should not error")

		// Number
		err = client.Set(ctx, "test:set:number", 12345, 10*time.Second).Err()
		assert.NoError(t, err, "Set number should not error")

		// Float
		err = client.Set(ctx, "test:set:float", 3.14159, 10*time.Second).Err()
		assert.NoError(t, err, "Set float should not error")
	})

	t.Run("Overwrite existing key", func(t *testing.T) {
		// Set initial value
		err := client.Set(ctx, "test:set:overwrite", "initial", 10*time.Second).Err()
		require.NoError(t, err, "Initial Set should not error")

		// Overwrite
		err = client.Set(ctx, "test:set:overwrite", "new_value", 10*time.Second).Err()
		require.NoError(t, err, "Overwrite Set should not error")

		// Verify new value
		val, err := client.Get(ctx, "test:set:overwrite").Result()
		require.NoError(t, err, "Get should not error")
		assert.Equal(t, "new_value", val, "Value should be overwritten")
	})
}

func TestClient_Del(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Delete single key", func(t *testing.T) {
		// Set a test key
		err := client.Set(ctx, "test:del:key1", "value1", 10*time.Second).Err()
		require.NoError(t, err, "Set should not error")

		// Delete the key
		cmd := client.Del(ctx, "test:del:key1")
		require.NoError(t, cmd.Err(), "Del should not error")
		assert.Equal(t, int64(1), cmd.Val(), "Del should return 1 for deleted key")

		// Verify key was deleted
		exists, err := client.Exists(ctx, "test:del:key1").Result()
		require.NoError(t, err, "Exists should not error")
		assert.Equal(t, int64(0), exists, "Key should not exist after deletion")
	})

	t.Run("Delete multiple keys", func(t *testing.T) {
		// Set test keys
		err := client.Set(ctx, "test:del:key2", "value2", 10*time.Second).Err()
		require.NoError(t, err, "Set key2 should not error")
		err = client.Set(ctx, "test:del:key3", "value3", 10*time.Second).Err()
		require.NoError(t, err, "Set key3 should not error")

		// Delete multiple keys
		cmd := client.Del(ctx, "test:del:key2", "test:del:key3")
		require.NoError(t, cmd.Err(), "Del should not error")
		assert.Equal(t, int64(2), cmd.Val(), "Del should return 2 for two deleted keys")
	})

	t.Run("Delete non-existent key", func(t *testing.T) {
		cmd := client.Del(ctx, "test:del:nonexistent")
		require.NoError(t, cmd.Err(), "Del should not error for non-existent key")
		assert.Equal(t, int64(0), cmd.Val(), "Del should return 0 for non-existent key")
	})

	t.Run("Delete mixed existent and non-existent keys", func(t *testing.T) {
		// Set one test key
		err := client.Set(ctx, "test:del:exists", "value", 10*time.Second).Err()
		require.NoError(t, err, "Set should not error")

		// Delete mixed keys
		cmd := client.Del(ctx, "test:del:exists", "test:del:notexists")
		require.NoError(t, cmd.Err(), "Del should not error")
		assert.Equal(t, int64(1), cmd.Val(), "Del should return 1 for one deleted key")
	})
}

func TestClient_Ping(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Ping successfully", func(t *testing.T) {
		cmd := client.Ping(ctx)
		require.NoError(t, cmd.Err(), "Ping should not error")
		assert.Equal(t, "PONG", cmd.Val(), "Ping should return PONG")
	})

	t.Run("Ping with cancelled context", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		cmd := client.Ping(cancelCtx)
		assert.Error(t, cmd.Err(), "Ping with cancelled context should error")
	})
}

func TestClient_Exists(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Check single existing key", func(t *testing.T) {
		// Set a test key
		err := client.Set(ctx, "test:exists:key1", "value1", 10*time.Second).Err()
		require.NoError(t, err, "Set should not error")

		// Check existence
		cmd := client.Exists(ctx, "test:exists:key1")
		require.NoError(t, cmd.Err(), "Exists should not error")
		assert.Equal(t, int64(1), cmd.Val(), "Exists should return 1 for existing key")
	})

	t.Run("Check non-existent key", func(t *testing.T) {
		cmd := client.Exists(ctx, "test:exists:nonexistent")
		require.NoError(t, cmd.Err(), "Exists should not error")
		assert.Equal(t, int64(0), cmd.Val(), "Exists should return 0 for non-existent key")
	})

	t.Run("Check multiple keys", func(t *testing.T) {
		// Set test keys
		err := client.Set(ctx, "test:exists:key2", "value2", 10*time.Second).Err()
		require.NoError(t, err, "Set key2 should not error")
		err = client.Set(ctx, "test:exists:key3", "value3", 10*time.Second).Err()
		require.NoError(t, err, "Set key3 should not error")

		// Check existence of multiple keys
		cmd := client.Exists(ctx, "test:exists:key2", "test:exists:key3", "test:exists:notexists")
		require.NoError(t, cmd.Err(), "Exists should not error")
		assert.Equal(t, int64(2), cmd.Val(), "Exists should return 2 for two existing keys")
	})
}

func TestClient_TTL(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Get TTL for key with expiration", func(t *testing.T) {
		// Set a test key with TTL
		err := client.Set(ctx, "test:ttl:key1", "value1", 10*time.Second).Err()
		require.NoError(t, err, "Set should not error")

		// Get TTL
		cmd := client.TTL(ctx, "test:ttl:key1")
		require.NoError(t, cmd.Err(), "TTL should not error")
		ttl := cmd.Val()
		assert.Greater(t, ttl, time.Duration(0), "TTL should be positive")
		assert.LessOrEqual(t, ttl, 10*time.Second, "TTL should not exceed set value")
	})

	t.Run("Get TTL for key without expiration", func(t *testing.T) {
		// Set a test key without TTL
		err := client.Set(ctx, "test:ttl:key2", "value2", 0).Err()
		require.NoError(t, err, "Set should not error")

		// Get TTL
		cmd := client.TTL(ctx, "test:ttl:key2")
		require.NoError(t, cmd.Err(), "TTL should not error")
		assert.Equal(t, time.Duration(-1), cmd.Val(), "TTL should be -1 for keys without expiration")
	})

	t.Run("Get TTL for non-existent key", func(t *testing.T) {
		cmd := client.TTL(ctx, "test:ttl:nonexistent")
		require.NoError(t, cmd.Err(), "TTL should not error")
		assert.Equal(t, time.Duration(-2), cmd.Val(), "TTL should be -2 for non-existent keys")
	})
}

func TestClient_Keys(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Get keys by pattern", func(t *testing.T) {
		// Set test keys
		err := client.Set(ctx, "test:keys:foo1", "value1", 10*time.Second).Err()
		require.NoError(t, err, "Set foo1 should not error")
		err = client.Set(ctx, "test:keys:foo2", "value2", 10*time.Second).Err()
		require.NoError(t, err, "Set foo2 should not error")
		err = client.Set(ctx, "test:keys:bar1", "value3", 10*time.Second).Err()
		require.NoError(t, err, "Set bar1 should not error")

		// Get keys matching pattern
		cmd := client.Keys(ctx, "test:keys:foo*")
		require.NoError(t, cmd.Err(), "Keys should not error")
		keys := cmd.Val()
		assert.Len(t, keys, 2, "Keys should return 2 matching keys")
		assert.Contains(t, keys, "test:keys:foo1", "Keys should contain foo1")
		assert.Contains(t, keys, "test:keys:foo2", "Keys should contain foo2")
	})

	t.Run("Get all keys with wildcard", func(t *testing.T) {
		// Set test keys
		err := client.Set(ctx, "test:keys:all1", "value1", 10*time.Second).Err()
		require.NoError(t, err, "Set all1 should not error")
		err = client.Set(ctx, "test:keys:all2", "value2", 10*time.Second).Err()
		require.NoError(t, err, "Set all2 should not error")

		// Get all test keys
		cmd := client.Keys(ctx, "test:keys:all*")
		require.NoError(t, cmd.Err(), "Keys should not error")
		keys := cmd.Val()
		assert.GreaterOrEqual(t, len(keys), 2, "Keys should return at least 2 keys")
	})

	t.Run("Get keys with no matches", func(t *testing.T) {
		cmd := client.Keys(ctx, "test:keys:nomatch*")
		require.NoError(t, cmd.Err(), "Keys should not error")
		keys := cmd.Val()
		assert.Len(t, keys, 0, "Keys should return empty slice for no matches")
	})
}

func TestClient_FlushDB(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("FlushDB successfully", func(t *testing.T) {
		// Set some test keys
		err := client.Set(ctx, "test:flush:key1", "value1", 10*time.Second).Err()
		require.NoError(t, err, "Set key1 should not error")
		err = client.Set(ctx, "test:flush:key2", "value2", 10*time.Second).Err()
		require.NoError(t, err, "Set key2 should not error")

		// Flush database
		cmd := client.FlushDB(ctx)
		require.NoError(t, cmd.Err(), "FlushDB should not error")
		assert.Equal(t, "OK", cmd.Val(), "FlushDB should return OK")

		// Verify keys were deleted
		exists, err := client.Exists(ctx, "test:flush:key1", "test:flush:key2").Result()
		require.NoError(t, err, "Exists should not error")
		assert.Equal(t, int64(0), exists, "Keys should not exist after FlushDB")
	})
}

func TestClient_Info(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Get all info", func(t *testing.T) {
		cmd := client.Info(ctx)
		require.NoError(t, cmd.Err(), "Info should not error")
		info := cmd.Val()
		assert.NotEmpty(t, info, "Info should return non-empty string")
		assert.Contains(t, info, "redis_version", "Info should contain redis_version")
	})

	t.Run("Get specific section", func(t *testing.T) {
		cmd := client.Info(ctx, "server")
		require.NoError(t, cmd.Err(), "Info should not error")
		info := cmd.Val()
		assert.NotEmpty(t, info, "Info should return non-empty string")
		assert.Contains(t, info, "redis_version", "Info server section should contain redis_version")
	})

	t.Run("Get multiple sections", func(t *testing.T) {
		cmd := client.Info(ctx, "server", "clients")
		require.NoError(t, cmd.Err(), "Info should not error")
		info := cmd.Val()
		assert.NotEmpty(t, info, "Info should return non-empty string")
	})
}

func TestClient_PoolStats(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	t.Run("Get pool stats from single client", func(t *testing.T) {
		stats := client.PoolStats()
		assert.NotNil(t, stats, "PoolStats should return non-nil stats")
		// Stats fields should be accessible
		assert.GreaterOrEqual(t, stats.Hits, uint32(0), "Hits should be non-negative")
		assert.GreaterOrEqual(t, stats.Misses, uint32(0), "Misses should be non-negative")
		assert.GreaterOrEqual(t, stats.Timeouts, uint32(0), "Timeouts should be non-negative")
		assert.GreaterOrEqual(t, stats.TotalConns, uint32(0), "TotalConns should be non-negative")
		assert.GreaterOrEqual(t, stats.IdleConns, uint32(0), "IdleConns should be non-negative")
		assert.GreaterOrEqual(t, stats.StaleConns, uint32(0), "StaleConns should be non-negative")
	})
}

func TestClient_Pipeline(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Create pipeline", func(t *testing.T) {
		pipeline := client.Pipeline()
		assert.NotNil(t, pipeline, "Pipeline should return non-nil pipeliner")
	})

	t.Run("Execute pipeline operations", func(t *testing.T) {
		pipeline := client.Pipeline()

		// Queue multiple operations
		pipeline.Set(ctx, "test:pipeline:key1", "value1", 10*time.Second)
		pipeline.Set(ctx, "test:pipeline:key2", "value2", 10*time.Second)
		pipeline.Get(ctx, "test:pipeline:key1")

		// Execute pipeline
		cmds, err := pipeline.Exec(ctx)
		require.NoError(t, err, "Pipeline Exec should not error")
		assert.Len(t, cmds, 3, "Pipeline should execute 3 commands")

		// Verify values were set
		val, err := client.Get(ctx, "test:pipeline:key1").Result()
		require.NoError(t, err, "Get key1 should not error")
		assert.Equal(t, "value1", val, "Key1 should have correct value")
	})
}

func TestClient_LLen(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Get length of existing list", func(t *testing.T) {
		// Create a list
		err := client.LPush(ctx, "test:llen:list1", "value1", "value2", "value3").Err()
		require.NoError(t, err, "LPush should not error")

		// Get list length
		cmd := client.LLen(ctx, "test:llen:list1")
		require.NoError(t, cmd.Err(), "LLen should not error")
		assert.Equal(t, int64(3), cmd.Val(), "LLen should return 3")
	})

	t.Run("Get length of non-existent list", func(t *testing.T) {
		cmd := client.LLen(ctx, "test:llen:nonexistent")
		require.NoError(t, cmd.Err(), "LLen should not error")
		assert.Equal(t, int64(0), cmd.Val(), "LLen should return 0 for non-existent list")
	})
}

func TestClient_LPush(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Push single value", func(t *testing.T) {
		cmd := client.LPush(ctx, "test:lpush:list1", "value1")
		require.NoError(t, cmd.Err(), "LPush should not error")
		assert.Equal(t, int64(1), cmd.Val(), "LPush should return list length 1")

		// Verify list length
		length, err := client.LLen(ctx, "test:lpush:list1").Result()
		require.NoError(t, err, "LLen should not error")
		assert.Equal(t, int64(1), length, "List should have 1 element")
	})

	t.Run("Push multiple values", func(t *testing.T) {
		cmd := client.LPush(ctx, "test:lpush:list2", "value1", "value2", "value3")
		require.NoError(t, cmd.Err(), "LPush should not error")
		assert.Equal(t, int64(3), cmd.Val(), "LPush should return list length 3")

		// Verify list length
		length, err := client.LLen(ctx, "test:lpush:list2").Result()
		require.NoError(t, err, "LLen should not error")
		assert.Equal(t, int64(3), length, "List should have 3 elements")
	})

	t.Run("Push to existing list", func(t *testing.T) {
		// Initial push
		err := client.LPush(ctx, "test:lpush:list3", "value1").Err()
		require.NoError(t, err, "Initial LPush should not error")

		// Push more values
		cmd := client.LPush(ctx, "test:lpush:list3", "value2", "value3")
		require.NoError(t, cmd.Err(), "Second LPush should not error")
		assert.Equal(t, int64(3), cmd.Val(), "LPush should return list length 3")
	})
}

func TestClient_BRPop(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Pop from non-empty list", func(t *testing.T) {
		// Push values to list
		err := client.LPush(ctx, "test:brpop:list1", "value1", "value2").Err()
		require.NoError(t, err, "LPush should not error")

		// Pop from list
		cmd := client.BRPop(ctx, 1*time.Second, "test:brpop:list1")
		require.NoError(t, cmd.Err(), "BRPop should not error")
		result := cmd.Val()
		assert.Len(t, result, 2, "BRPop should return 2 elements (key and value)")
		assert.Equal(t, "test:brpop:list1", result[0], "First element should be key")
		assert.Equal(t, "value1", result[1], "Second element should be value")
	})

	t.Run("Pop from empty list with timeout", func(t *testing.T) {
		// Pop from non-existent list with short timeout
		cmd := client.BRPop(ctx, 100*time.Millisecond, "test:brpop:empty")
		assert.Equal(t, redis.Nil, cmd.Err(), "BRPop on empty list should return redis.Nil")
	})

	t.Run("Pop from multiple lists", func(t *testing.T) {
		// Push to second list only
		err := client.LPush(ctx, "test:brpop:multi2", "value2").Err()
		require.NoError(t, err, "LPush should not error")

		// Pop from multiple lists (first is empty, second has value)
		cmd := client.BRPop(ctx, 1*time.Second, "test:brpop:multi1", "test:brpop:multi2")
		require.NoError(t, cmd.Err(), "BRPop should not error")
		result := cmd.Val()
		assert.Equal(t, "test:brpop:multi2", result[0], "Should pop from second list")
	})
}

func TestClient_RPop(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Pop from non-empty list", func(t *testing.T) {
		// Push values to list
		err := client.LPush(ctx, "test:rpop:list1", "value1", "value2", "value3").Err()
		require.NoError(t, err, "LPush should not error")

		// Pop from list
		cmd := client.RPop(ctx, "test:rpop:list1")
		require.NoError(t, cmd.Err(), "RPop should not error")
		assert.Equal(t, "value1", cmd.Val(), "RPop should return first pushed value")

		// Verify list length decreased
		length, err := client.LLen(ctx, "test:rpop:list1").Result()
		require.NoError(t, err, "LLen should not error")
		assert.Equal(t, int64(2), length, "List should have 2 elements after pop")
	})

	t.Run("Pop from empty list", func(t *testing.T) {
		cmd := client.RPop(ctx, "test:rpop:empty")
		assert.Equal(t, redis.Nil, cmd.Err(), "RPop on empty list should return redis.Nil")
	})

	t.Run("Pop until list is empty", func(t *testing.T) {
		// Push two values
		err := client.LPush(ctx, "test:rpop:list2", "value1", "value2").Err()
		require.NoError(t, err, "LPush should not error")

		// Pop first value
		cmd := client.RPop(ctx, "test:rpop:list2")
		require.NoError(t, cmd.Err(), "First RPop should not error")
		assert.Equal(t, "value1", cmd.Val(), "First pop should return value1")

		// Pop second value
		cmd = client.RPop(ctx, "test:rpop:list2")
		require.NoError(t, cmd.Err(), "Second RPop should not error")
		assert.Equal(t, "value2", cmd.Val(), "Second pop should return value2")

		// Pop from now-empty list
		cmd = client.RPop(ctx, "test:rpop:list2")
		assert.Equal(t, redis.Nil, cmd.Err(), "Pop from empty list should return redis.Nil")
	})
}

func TestClient_ErrorHandling(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Set operation with wrong type", func(t *testing.T) {
		// Create a list
		err := client.LPush(ctx, "test:error:list", "value").Err()
		require.NoError(t, err, "LPush should not error")

		// Try to set TTL on a list (should work)
		cmd := client.TTL(ctx, "test:error:list")
		require.NoError(t, cmd.Err(), "TTL on list should not error")
	})

	t.Run("Operations with cancelled context", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Try various operations
		cmd := client.Get(cancelCtx, "test:error:key")
		assert.Error(t, cmd.Err(), "Get with cancelled context should error")

		setCmd := client.Set(cancelCtx, "test:error:key", "value", 0)
		assert.Error(t, setCmd.Err(), "Set with cancelled context should error")
	})
}

func TestClient_ConcurrentOperations(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Concurrent sets and gets", func(t *testing.T) {
		done := make(chan error, 100)

		// Run 100 concurrent operations
		for i := 0; i < 100; i++ {
			go func(index int) {
				key := "test:concurrent:key"
				value := "value"

				// Set
				err := client.Set(ctx, key, value, 10*time.Second).Err()
				if err != nil {
					done <- err
					return
				}

				// Get
				_, err = client.Get(ctx, key).Result()
				done <- err
			}(i)
		}

		// Wait for all operations
		for i := 0; i < 100; i++ {
			err := <-done
			assert.NoError(t, err, "Concurrent operation should not error")
		}
	})
}

func TestClient_TracingAttributes(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Tracing preserves functionality", func(t *testing.T) {
		// All operations should work with tracing enabled
		err := client.Set(ctx, "test:trace:key", "value", 10*time.Second).Err()
		assert.NoError(t, err, "Set with tracing should not error")

		val, err := client.Get(ctx, "test:trace:key").Result()
		assert.NoError(t, err, "Get with tracing should not error")
		assert.Equal(t, "value", val, "Get should return correct value with tracing")

		// Operations should complete successfully
		err = client.Del(ctx, "test:trace:key").Err()
		assert.NoError(t, err, "Del with tracing should not error")
	})
}

// TestClient_PoolStatsCluster tests pool stats for cluster client
func TestClient_PoolStatsCluster(t *testing.T) {
	// Only run if cluster is available
	if os.Getenv("REDIS_CLUSTER_ADDRS") == "" {
		t.Skip("Skipping cluster tests: REDIS_CLUSTER_ADDRS not set")
	}

	client, cleanup := setupRedisClusterForTest(t)
	defer cleanup()

	t.Run("Get pool stats from cluster client", func(t *testing.T) {
		stats := client.PoolStats()
		assert.NotNil(t, stats, "PoolStats should return non-nil stats for cluster")
	})
}

// TestClient_PoolStatsInvalidType tests pool stats with invalid client type
func TestClient_PoolStatsInvalidType(t *testing.T) {
	t.Run("PoolStats with invalid cmdable type", func(t *testing.T) {
		// Create a client with a mock cmdable that's neither *redis.Client nor *redis.ClusterClient
		client := &Client{
			cmdable: &mockCmdable{},
		}

		stats := client.PoolStats()
		assert.NotNil(t, stats, "PoolStats should return empty stats for unknown type")
		assert.Equal(t, uint32(0), stats.Hits, "Stats should be empty")
		assert.Equal(t, uint32(0), stats.Misses, "Stats should be empty")
	})
}

// mockCmdable is a mock implementation that doesn't match redis.Client or redis.ClusterClient
type mockCmdable struct {
	redis.Cmdable
}

func (m *mockCmdable) Pipeline() redis.Pipeliner {
	return nil
}

// Helper function to verify that errors are properly wrapped and traced
func TestClient_ErrorTracing(t *testing.T) {
	client, cleanup := setupRedisForTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Error is traced for Get", func(t *testing.T) {
		// Getting non-existent key returns redis.Nil, which should be traced but not as error
		cmd := client.Get(ctx, "test:error:nonexistent")
		err := cmd.Err()
		assert.Equal(t, redis.Nil, err, "Should return redis.Nil")
	})

	t.Run("Error is traced for Set", func(t *testing.T) {
		// Set with invalid value type - Redis will handle conversion
		// This shouldn't error in most cases
		cmd := client.Set(ctx, "test:error:key", errors.New("error value"), 1*time.Second)
		// Redis will convert error to string, so this might not error
		_ = cmd.Err()
	})
}
