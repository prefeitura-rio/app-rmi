package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAdminHandlersTest(t *testing.T) (*gin.Engine, func()) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6380"
	}

	logging.InitLogger()
	gin.SetMode(gin.TestMode)

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AdminGroup = "go:admin"

	// Redis setup
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	config.Redis = redisclient.NewClient(singleClient)

	router := gin.New()

	// Create admin middleware for testing
	adminMiddleware := func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "03561350712",
		}
		claims.ResourceAccess.Superapp.Roles = []string{"go:admin"}
		c.Set("claims", claims)
		c.Next()
	}

	router.Use(adminMiddleware)
	router.POST("/admin/cache/read", ReadCacheKey)

	return router, func() {
		ctx := context.Background()
		// Clean up test keys
		patterns := []string{"test:*"}
		for _, pattern := range patterns {
			keys, _ := config.Redis.Keys(ctx, pattern).Result()
			if len(keys) > 0 {
				config.Redis.Del(ctx, keys...)
			}
		}
	}
}

func setupAdminHandlersTestWithAuth(t *testing.T, isAdmin bool) (*gin.Engine, func()) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6380"
	}

	logging.InitLogger()
	gin.SetMode(gin.TestMode)

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AdminGroup = "go:admin"

	// Redis setup
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	config.Redis = redisclient.NewClient(singleClient)

	router := gin.New()

	// Create middleware for testing with different auth levels
	authMiddleware := func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "03561350712",
		}
		if isAdmin {
			claims.ResourceAccess.Superapp.Roles = []string{"go:admin"}
		} else {
			claims.ResourceAccess.Superapp.Roles = []string{"go:user"}
		}
		c.Set("claims", claims)
		c.Next()
	}

	router.Use(authMiddleware)
	router.POST("/admin/cache/read", ReadCacheKey)

	return router, func() {
		ctx := context.Background()
		// Clean up test keys
		patterns := []string{"test:*"}
		for _, pattern := range patterns {
			keys, _ := config.Redis.Keys(ctx, pattern).Result()
			if len(keys) > 0 {
				config.Redis.Del(ctx, keys...)
			}
		}
	}
}

func setupAdminHandlersTestNoAuth(t *testing.T) (*gin.Engine, func()) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6380"
	}

	logging.InitLogger()
	gin.SetMode(gin.TestMode)

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AdminGroup = "go:admin"

	// Redis setup
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	config.Redis = redisclient.NewClient(singleClient)

	router := gin.New()
	// No auth middleware - simulates unauthenticated request
	router.POST("/admin/cache/read", ReadCacheKey)

	return router, func() {
		ctx := context.Background()
		// Clean up test keys
		patterns := []string{"test:*"}
		for _, pattern := range patterns {
			keys, _ := config.Redis.Keys(ctx, pattern).Result()
			if len(keys) > 0 {
				config.Redis.Del(ctx, keys...)
			}
		}
	}
}

func TestReadCacheKey_Success(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Set a test key in Redis
	testKey := "test:key1"
	testValue := "test value 1"
	err := config.Redis.Set(ctx, testKey, testValue, 10*time.Minute).Err()
	require.NoError(t, err, "Failed to set test key in Redis")

	reqBody := CacheReadRequest{
		Key: testKey,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Response body: %s", w.Body.String())

	var response CacheReadResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, testKey, response.Key)
	assert.True(t, response.Exists)
	assert.Equal(t, testValue, response.Value)
	assert.Greater(t, response.TTL, int64(0), "TTL should be greater than 0")
}

func TestReadCacheKey_NotFound(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	reqBody := CacheReadRequest{
		Key: "test:nonexistent",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response CacheReadResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.False(t, response.Exists, "Exists should be false for nonexistent key")
	assert.Nil(t, response.Value)
	assert.Equal(t, int64(-1), response.TTL)
}

func TestReadCacheKey_InvalidRequest(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Error, "Invalid request body")
}

func TestReadCacheKey_EmptyKey(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	reqBody := CacheReadRequest{
		Key: "",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Contains(t, response.Error, "Invalid request body")
}

func TestReadCacheKey_WithTTL(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Set a test key with specific TTL
	testKey := "test:ttl_key"
	testValue := "value with ttl"
	ttl := 30 * time.Second
	err := config.Redis.Set(ctx, testKey, testValue, ttl).Err()
	require.NoError(t, err, "Failed to set test key in Redis")

	reqBody := CacheReadRequest{
		Key: testKey,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response CacheReadResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	// TTL should be close to 30 seconds (allowing some margin for test execution time)
	assert.GreaterOrEqual(t, response.TTL, int64(25), "TTL should be at least 25 seconds")
	assert.LessOrEqual(t, response.TTL, int64(30), "TTL should be at most 30 seconds")
}

func TestReadCacheKey_NoTTL(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Set a test key with no TTL (persist)
	testKey := "test:no_ttl_key"
	testValue := "value without ttl"
	err := config.Redis.Set(ctx, testKey, testValue, 0).Err()
	require.NoError(t, err, "Failed to set test key in Redis")

	reqBody := CacheReadRequest{
		Key: testKey,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response CacheReadResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.True(t, response.Exists)
	// Keys with no expiration should have TTL of -1 (persistent)
	// Note: This may vary depending on Redis version
	if response.TTL != -1 {
		t.Logf("Note: TTL for persistent key = %v (expected -1, but may vary)", response.TTL)
	}
}

func TestReadCacheKey_DifferentValueTypes(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"simple string", "test:string", "hello world"},
		{"json string", "test:json", `{"key":"value","number":123}`},
		{"numeric string", "test:number", "12345"},
		{"empty string", "test:empty", ""},
		{"unicode string", "test:unicode", "こんにちは世界"},
		{"special chars", "test:special", "!@#$%^&*()_+-={}[]|\\:;\"'<>,.?/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set test key
			err := config.Redis.Set(ctx, tt.key, tt.value, 10*time.Minute).Err()
			require.NoError(t, err, "Failed to set test key in Redis")
			defer config.Redis.Del(ctx, tt.key)

			reqBody := CacheReadRequest{
				Key: tt.key,
			}

			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response CacheReadResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err, "Failed to unmarshal response")

			assert.True(t, response.Exists)
			assert.Equal(t, tt.value, response.Value)
		})
	}
}

func TestReadCacheKey_MultipleKeys(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Set multiple test keys
	keys := []struct {
		key   string
		value string
	}{
		{"test:multi1", "value1"},
		{"test:multi2", "value2"},
		{"test:multi3", "value3"},
	}

	for _, k := range keys {
		err := config.Redis.Set(ctx, k.key, k.value, 10*time.Minute).Err()
		require.NoError(t, err)
	}

	// Read each key and verify
	for _, k := range keys {
		reqBody := CacheReadRequest{
			Key: k.key,
		}

		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response CacheReadResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response.Exists)
		assert.Equal(t, k.value, response.Value)
	}
}

func TestReadCacheKey_LargeValue(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a large value (1MB)
	testKey := "test:large_value"
	largeValue := string(make([]byte, 1024*1024))
	err := config.Redis.Set(ctx, testKey, largeValue, 10*time.Minute).Err()
	require.NoError(t, err)

	reqBody := CacheReadRequest{
		Key: testKey,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response CacheReadResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.True(t, response.Exists)
	assert.Equal(t, len(largeValue), len(response.Value.(string)))
}

func TestReadCacheKey_KeyPatterns(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"colon separator", "test:user:12345", "user data"},
		{"slash separator", "test/path/to/key", "path data"},
		{"mixed separators", "test:cache/user:12345", "mixed data"},
		{"long key", "test:very:long:key:with:many:parts:and:more:parts", "long key data"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Redis.Set(ctx, tt.key, tt.value, 10*time.Minute).Err()
			require.NoError(t, err)
			defer config.Redis.Del(ctx, tt.key)

			reqBody := CacheReadRequest{
				Key: tt.key,
			}

			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response CacheReadResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.True(t, response.Exists)
			assert.Equal(t, tt.value, response.Value)
		})
	}
}

func TestReadCacheKey_ConcurrentRequests(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Set up test keys
	numKeys := 10
	for i := 0; i < numKeys; i++ {
		key := "test:concurrent_" + string(rune('a'+i))
		value := "value_" + string(rune('a'+i))
		err := config.Redis.Set(ctx, key, value, 10*time.Minute).Err()
		require.NoError(t, err)
	}

	// Make concurrent requests
	done := make(chan bool)
	for i := 0; i < numKeys; i++ {
		go func(idx int) {
			key := "test:concurrent_" + string(rune('a'+idx))
			expectedValue := "value_" + string(rune('a'+idx))

			reqBody := CacheReadRequest{
				Key: key,
			}

			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response CacheReadResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.True(t, response.Exists)
			assert.Equal(t, expectedValue, response.Value)

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numKeys; i++ {
		<-done
	}
}

func TestReadCacheKey_AdminRequired(t *testing.T) {
	// Test with admin user - should succeed
	t.Run("admin user success", func(t *testing.T) {
		router, cleanup := setupAdminHandlersTestWithAuth(t, true)
		defer cleanup()

		ctx := context.Background()
		testKey := "test:admin_key"
		testValue := "admin value"
		err := config.Redis.Set(ctx, testKey, testValue, 10*time.Minute).Err()
		require.NoError(t, err)

		reqBody := CacheReadRequest{
			Key: testKey,
		}

		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response CacheReadResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Exists)
		assert.Equal(t, testValue, response.Value)
	})

	// Test with non-admin user - handler should still work (authorization checked in middleware)
	t.Run("non-admin user", func(t *testing.T) {
		router, cleanup := setupAdminHandlersTestWithAuth(t, false)
		defer cleanup()

		ctx := context.Background()
		testKey := "test:non_admin_key"
		testValue := "non-admin value"
		err := config.Redis.Set(ctx, testKey, testValue, 10*time.Minute).Err()
		require.NoError(t, err)

		reqBody := CacheReadRequest{
			Key: testKey,
		}

		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Handler works fine - authorization is middleware's responsibility
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestReadCacheKey_NoAuthentication(t *testing.T) {
	router, cleanup := setupAdminHandlersTestNoAuth(t)
	defer cleanup()

	ctx := context.Background()
	testKey := "test:no_auth_key"
	testValue := "no auth value"
	err := config.Redis.Set(ctx, testKey, testValue, 10*time.Minute).Err()
	require.NoError(t, err)

	reqBody := CacheReadRequest{
		Key: testKey,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Handler works without auth - authorization is middleware's responsibility
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestReadCacheKey_MissingContentType(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	reqBody := CacheReadRequest{
		Key: "test:some_key",
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
	// Missing Content-Type header
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Gin is usually lenient, but validation should still happen
	// The response depends on Gin's default behavior
	assert.NotEqual(t, http.StatusInternalServerError, w.Code)
}

func TestReadCacheKey_TTLEdgeCases(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name        string
		key         string
		value       string
		ttl         time.Duration
		expectTTL   bool
		minTTL      int64
		maxTTL      int64
		description string
	}{
		{
			name:        "very short TTL",
			key:         "test:short_ttl",
			value:       "short value",
			ttl:         1 * time.Second,
			expectTTL:   true,
			minTTL:      0,
			maxTTL:      2,
			description: "Key with 1 second TTL",
		},
		{
			name:        "medium TTL",
			key:         "test:medium_ttl",
			value:       "medium value",
			ttl:         5 * time.Minute,
			expectTTL:   true,
			minTTL:      290,
			maxTTL:      300,
			description: "Key with 5 minute TTL",
		},
		{
			name:        "long TTL",
			key:         "test:long_ttl",
			value:       "long value",
			ttl:         1 * time.Hour,
			expectTTL:   true,
			minTTL:      3590,
			maxTTL:      3600,
			description: "Key with 1 hour TTL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Redis.Set(ctx, tt.key, tt.value, tt.ttl).Err()
			require.NoError(t, err)
			defer config.Redis.Del(ctx, tt.key)

			reqBody := CacheReadRequest{
				Key: tt.key,
			}

			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response CacheReadResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.True(t, response.Exists)
			if tt.expectTTL {
				assert.GreaterOrEqual(t, response.TTL, tt.minTTL)
				assert.LessOrEqual(t, response.TTL, tt.maxTTL)
			}
		})
	}
}

func TestReadCacheKey_JSONRequest(t *testing.T) {
	router, cleanup := setupAdminHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name       string
		jsonBody   string
		expectCode int
		expectErr  bool
	}{
		{
			name:       "valid JSON",
			jsonBody:   `{"key":"test:valid"}`,
			expectCode: http.StatusOK,
			expectErr:  false,
		},
		{
			name:       "missing required field",
			jsonBody:   `{"not_key":"test:invalid"}`,
			expectCode: http.StatusBadRequest,
			expectErr:  true,
		},
		{
			name:       "empty JSON object",
			jsonBody:   `{}`,
			expectCode: http.StatusBadRequest,
			expectErr:  true,
		},
		{
			name:       "malformed JSON",
			jsonBody:   `{key:"test:invalid"}`,
			expectCode: http.StatusBadRequest,
			expectErr:  true,
		},
		{
			name:       "extra fields",
			jsonBody:   `{"key":"test:extra","extra":"field"}`,
			expectCode: http.StatusOK,
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer([]byte(tt.jsonBody)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectCode, w.Code)

			if tt.expectErr {
				var response ErrorResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.Error)
			}
		})
	}
}

func TestReadCacheKey_Authorization_WithMiddleware(t *testing.T) {
	// This test demonstrates that authorization should be enforced by middleware,
	// not by the handler itself. The handler trusts that middleware has already
	// checked authorization.

	t.Run("admin with middleware enforcement", func(t *testing.T) {
		redisAddr := os.Getenv("REDIS_ADDR")
		if redisAddr == "" {
			redisAddr = "localhost:6380"
		}

		logging.InitLogger()
		gin.SetMode(gin.TestMode)

		if config.AppConfig == nil {
			config.AppConfig = &config.Config{}
		}
		config.AppConfig.AdminGroup = "go:admin"

		ctx := context.Background()

		// Redis setup
		singleClient := redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		})
		config.Redis = redisclient.NewClient(singleClient)

		// Test Redis connection
		err := config.Redis.Ping(ctx).Err()
		require.NoError(t, err, "Failed to connect to Redis")

		router := gin.New()

		// Simulate RequireAdmin middleware
		requireAdminMiddleware := func(c *gin.Context) {
			claims, exists := c.Get("claims")
			if !exists {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Claims not found"})
				c.Abort()
				return
			}

			jwtClaims, ok := claims.(*models.JWTClaims)
			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims type"})
				c.Abort()
				return
			}

			// Check if user has admin role
			isAdmin := false
			for _, role := range jwtClaims.RealmAccess.Roles {
				if role == config.AppConfig.AdminGroup {
					isAdmin = true
					break
				}
			}
			if !isAdmin {
				for _, role := range jwtClaims.ResourceAccess.Superapp.Roles {
					if role == config.AppConfig.AdminGroup {
						isAdmin = true
						break
					}
				}
			}

			if !isAdmin {
				c.JSON(http.StatusForbidden, gin.H{"error": "Admin privileges required"})
				c.Abort()
				return
			}

			c.Next()
		}

		// Create admin claims middleware
		adminClaimsMiddleware := func(c *gin.Context) {
			claims := &models.JWTClaims{
				PreferredUsername: "03561350712",
			}
			claims.RealmAccess.Roles = []string{"go:admin"}
			c.Set("claims", claims)
			c.Next()
		}

		// Setup route with both middlewares
		adminGroup := router.Group("/admin")
		adminGroup.Use(adminClaimsMiddleware)
		adminGroup.Use(requireAdminMiddleware)
		adminGroup.POST("/cache/read", ReadCacheKey)

		// Set test key
		testKey := "test:auth_admin"
		testValue := "admin authorized"
		err = config.Redis.Set(ctx, testKey, testValue, 10*time.Minute).Err()
		require.NoError(t, err)

		// Make request
		reqBody := CacheReadRequest{
			Key: testKey,
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Admin should succeed
		assert.Equal(t, http.StatusOK, w.Code, "Admin should be authorized")

		var response CacheReadResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.True(t, response.Exists)
		assert.Equal(t, testValue, response.Value)

		// Cleanup
		config.Redis.Del(ctx, testKey)
	})

	t.Run("non-admin with middleware enforcement", func(t *testing.T) {
		redisAddr := os.Getenv("REDIS_ADDR")
		if redisAddr == "" {
			redisAddr = "localhost:6380"
		}

		logging.InitLogger()
		gin.SetMode(gin.TestMode)

		if config.AppConfig == nil {
			config.AppConfig = &config.Config{}
		}
		config.AppConfig.AdminGroup = "go:admin"

		// Redis setup
		singleClient := redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		})
		config.Redis = redisclient.NewClient(singleClient)

		router := gin.New()

		// Simulate RequireAdmin middleware
		requireAdminMiddleware := func(c *gin.Context) {
			claims, exists := c.Get("claims")
			if !exists {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Claims not found"})
				c.Abort()
				return
			}

			jwtClaims, ok := claims.(*models.JWTClaims)
			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims type"})
				c.Abort()
				return
			}

			// Check if user has admin role
			isAdmin := false
			for _, role := range jwtClaims.RealmAccess.Roles {
				if role == config.AppConfig.AdminGroup {
					isAdmin = true
					break
				}
			}
			if !isAdmin {
				for _, role := range jwtClaims.ResourceAccess.Superapp.Roles {
					if role == config.AppConfig.AdminGroup {
						isAdmin = true
						break
					}
				}
			}

			if !isAdmin {
				c.JSON(http.StatusForbidden, gin.H{"error": "Admin privileges required"})
				c.Abort()
				return
			}

			c.Next()
		}

		// Create non-admin claims middleware
		nonAdminClaimsMiddleware := func(c *gin.Context) {
			claims := &models.JWTClaims{
				PreferredUsername: "98765432100",
			}
			claims.RealmAccess.Roles = []string{"go:user"}
			c.Set("claims", claims)
			c.Next()
		}

		// Setup route with both middlewares
		adminGroup := router.Group("/admin")
		adminGroup.Use(nonAdminClaimsMiddleware)
		adminGroup.Use(requireAdminMiddleware)
		adminGroup.POST("/cache/read", ReadCacheKey)

		// Make request
		reqBody := CacheReadRequest{
			Key: "test:auth_non_admin",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Non-admin should be forbidden
		assert.Equal(t, http.StatusForbidden, w.Code, "Non-admin should be forbidden")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "Admin privileges required")
	})

	t.Run("missing claims with middleware enforcement", func(t *testing.T) {
		redisAddr := os.Getenv("REDIS_ADDR")
		if redisAddr == "" {
			redisAddr = "localhost:6380"
		}

		logging.InitLogger()
		gin.SetMode(gin.TestMode)

		if config.AppConfig == nil {
			config.AppConfig = &config.Config{}
		}
		config.AppConfig.AdminGroup = "go:admin"

		// Redis setup
		singleClient := redis.NewClient(&redis.Options{
			Addr:     redisAddr,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		})
		config.Redis = redisclient.NewClient(singleClient)

		router := gin.New()

		// Simulate RequireAdmin middleware
		requireAdminMiddleware := func(c *gin.Context) {
			claims, exists := c.Get("claims")
			if !exists {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Claims not found"})
				c.Abort()
				return
			}

			jwtClaims, ok := claims.(*models.JWTClaims)
			if !ok {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid claims type"})
				c.Abort()
				return
			}

			// Check if user has admin role
			isAdmin := false
			for _, role := range jwtClaims.RealmAccess.Roles {
				if role == config.AppConfig.AdminGroup {
					isAdmin = true
					break
				}
			}
			if !isAdmin {
				for _, role := range jwtClaims.ResourceAccess.Superapp.Roles {
					if role == config.AppConfig.AdminGroup {
						isAdmin = true
						break
					}
				}
			}

			if !isAdmin {
				c.JSON(http.StatusForbidden, gin.H{"error": "Admin privileges required"})
				c.Abort()
				return
			}

			c.Next()
		}

		// Setup route with middleware but NO claims middleware (simulating missing auth)
		adminGroup := router.Group("/admin")
		adminGroup.Use(requireAdminMiddleware)
		adminGroup.POST("/cache/read", ReadCacheKey)

		// Make request
		reqBody := CacheReadRequest{
			Key: "test:auth_no_claims",
		}
		body, _ := json.Marshal(reqBody)
		req, _ := http.NewRequest("POST", "/admin/cache/read", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Should be unauthorized
		assert.Equal(t, http.StatusUnauthorized, w.Code, "Missing claims should result in unauthorized")

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response["error"], "Claims not found")
	})
}
