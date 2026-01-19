package services

import (
	"context"
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
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// setupAddressServiceTest initializes MongoDB and Redis for testing
func setupAddressServiceTest(t *testing.T) (*AddressService, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping address service tests: MONGODB_URI not set")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	logging.InitLogger()

	// Initialize config
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.BairroCollection = "test_bairros"
	config.AppConfig.LogradouroCollection = "test_logradouros"
	config.AppConfig.AddressCacheTTL = 5 * time.Minute

	// MongoDB setup
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to ping MongoDB: %v", err)
	}

	database := client.Database("rmi_test")
	config.MongoDB = database

	// Redis setup
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	config.Redis = redisclient.NewClient(singleClient)

	// Test Redis connection
	err = config.Redis.Ping(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Create service
	logger := zap.L().Named("address_service_test")
	service := NewAddressService(client, database, logger)

	return service, func() {
		// Clean up Redis
		keys, _ := config.Redis.Keys(ctx, "address:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}

		// Clean up MongoDB
		database.Drop(ctx)
		client.Disconnect(ctx)
	}
}

func TestGetBairroByID_FromMongoDB(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test bairro
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "1234",
		"nome":      "Copacabana",
	})
	if err != nil {
		t.Fatalf("Failed to insert bairro: %v", err)
	}

	// Get bairro - should retrieve from MongoDB and cache
	nome, err := service.GetBairroByID(ctx, "1234")
	if err != nil {
		t.Errorf("GetBairroByID() error = %v", err)
	}

	if nome != "Copacabana" {
		t.Errorf("GetBairroByID() = %v, want Copacabana", nome)
	}

	// Verify it was cached
	cacheKey := "address:bairro:1234"
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	if err != nil {
		t.Errorf("Bairro should be cached: %v", err)
	}
	if cached != "Copacabana" {
		t.Errorf("Cached bairro = %v, want Copacabana", cached)
	}
}

func TestGetBairroByID_FromCache(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test bairro
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "1234",
		"nome":      "Copacabana",
	})
	if err != nil {
		t.Fatalf("Failed to insert bairro: %v", err)
	}

	// First call - populates cache
	_, err = service.GetBairroByID(ctx, "1234")
	if err != nil {
		t.Fatalf("First GetBairroByID() error = %v", err)
	}

	// Delete from MongoDB to verify second call uses cache
	_, err = bairroCollection.DeleteOne(ctx, bson.M{"id_bairro": "1234"})
	if err != nil {
		t.Fatalf("Failed to delete bairro: %v", err)
	}

	// Second call - should use cache
	nome, err := service.GetBairroByID(ctx, "1234")
	if err != nil {
		t.Errorf("GetBairroByID() from cache error = %v", err)
	}

	if nome != "Copacabana" {
		t.Errorf("GetBairroByID() from cache = %v, want Copacabana", nome)
	}
}

func TestGetBairroByID_Empty(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Empty ID should return empty string without error
	nome, err := service.GetBairroByID(ctx, "")
	if err != nil {
		t.Errorf("GetBairroByID(\"\") error = %v, want nil", err)
	}

	if nome != "" {
		t.Errorf("GetBairroByID(\"\") = %v, want empty string", nome)
	}
}

func TestGetBairroByID_NotFound(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Non-existent bairro should return empty string without error
	nome, err := service.GetBairroByID(ctx, "99999")
	if err != nil {
		t.Errorf("GetBairroByID() for non-existent error = %v, want nil", err)
	}

	if nome != "" {
		t.Errorf("GetBairroByID() for non-existent = %v, want empty string", nome)
	}
}

func TestGetLogradouroByID_FromMongoDB(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test logradouro
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err := logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "5678",
		"nome_completo": "Avenida Atlântica",
	})
	if err != nil {
		t.Fatalf("Failed to insert logradouro: %v", err)
	}

	// Get logradouro - should retrieve from MongoDB and cache
	nome, err := service.GetLogradouroByID(ctx, "5678")
	if err != nil {
		t.Errorf("GetLogradouroByID() error = %v", err)
	}

	if nome != "Avenida Atlântica" {
		t.Errorf("GetLogradouroByID() = %v, want Avenida Atlântica", nome)
	}

	// Verify it was cached
	cacheKey := "address:logradouro:5678"
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	if err != nil {
		t.Errorf("Logradouro should be cached: %v", err)
	}
	if cached != "Avenida Atlântica" {
		t.Errorf("Cached logradouro = %v, want Avenida Atlântica", cached)
	}
}

func TestGetLogradouroByID_FromCache(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test logradouro
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err := logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "5678",
		"nome_completo": "Avenida Atlântica",
	})
	if err != nil {
		t.Fatalf("Failed to insert logradouro: %v", err)
	}

	// First call - populates cache
	_, err = service.GetLogradouroByID(ctx, "5678")
	if err != nil {
		t.Fatalf("First GetLogradouroByID() error = %v", err)
	}

	// Delete from MongoDB to verify second call uses cache
	_, err = logradouroCollection.DeleteOne(ctx, bson.M{"id_logradouro": "5678"})
	if err != nil {
		t.Fatalf("Failed to delete logradouro: %v", err)
	}

	// Second call - should use cache
	nome, err := service.GetLogradouroByID(ctx, "5678")
	if err != nil {
		t.Errorf("GetLogradouroByID() from cache error = %v", err)
	}

	if nome != "Avenida Atlântica" {
		t.Errorf("GetLogradouroByID() from cache = %v, want Avenida Atlântica", nome)
	}
}

func TestGetLogradouroByID_Empty(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Empty ID should return empty string without error
	nome, err := service.GetLogradouroByID(ctx, "")
	if err != nil {
		t.Errorf("GetLogradouroByID(\"\") error = %v, want nil", err)
	}

	if nome != "" {
		t.Errorf("GetLogradouroByID(\"\") = %v, want empty string", nome)
	}
}

func TestGetLogradouroByID_NotFound(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Non-existent logradouro should return empty string without error
	nome, err := service.GetLogradouroByID(ctx, "99999")
	if err != nil {
		t.Errorf("GetLogradouroByID() for non-existent error = %v, want nil", err)
	}

	if nome != "" {
		t.Errorf("GetLogradouroByID() for non-existent = %v, want empty string", nome)
	}
}

func TestBuildAddress_Complete(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test data
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "1234",
		"nome":      "Copacabana",
	})
	if err != nil {
		t.Fatalf("Failed to insert bairro: %v", err)
	}

	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "5678",
		"nome_completo": "Avenida Atlântica",
	})
	if err != nil {
		t.Fatalf("Failed to insert logradouro: %v", err)
	}

	// Build complete address with number
	address, err := service.BuildAddress(ctx, "1234", "5678", "1500")
	if err != nil {
		t.Errorf("BuildAddress() error = %v", err)
	}

	if address == nil {
		t.Fatal("BuildAddress() returned nil")
	}

	expected := "Avenida Atlântica, 1500 - Copacabana"
	if *address != expected {
		t.Errorf("BuildAddress() = %v, want %v", *address, expected)
	}
}

func TestBuildAddress_WithoutNumber(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test data
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "1234",
		"nome":      "Copacabana",
	})
	if err != nil {
		t.Fatalf("Failed to insert bairro: %v", err)
	}

	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "5678",
		"nome_completo": "Avenida Atlântica",
	})
	if err != nil {
		t.Fatalf("Failed to insert logradouro: %v", err)
	}

	// Build address without number
	address, err := service.BuildAddress(ctx, "1234", "5678", nil)
	if err != nil {
		t.Errorf("BuildAddress() error = %v", err)
	}

	if address == nil {
		t.Fatal("BuildAddress() returned nil")
	}

	expected := "Avenida Atlântica - Copacabana"
	if *address != expected {
		t.Errorf("BuildAddress() = %v, want %v", *address, expected)
	}
}

func TestBuildAddress_BairroOnly(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test bairro
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "1234",
		"nome":      "Copacabana",
	})
	if err != nil {
		t.Fatalf("Failed to insert bairro: %v", err)
	}

	// Build address with only bairro
	address, err := service.BuildAddress(ctx, "1234", "", nil)
	if err != nil {
		t.Errorf("BuildAddress() error = %v", err)
	}

	if address == nil {
		t.Fatal("BuildAddress() returned nil")
	}

	expected := "Copacabana"
	if *address != expected {
		t.Errorf("BuildAddress() = %v, want %v", *address, expected)
	}
}

func TestBuildAddress_LogradouroOnly(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test logradouro
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err := logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "5678",
		"nome_completo": "Avenida Atlântica",
	})
	if err != nil {
		t.Fatalf("Failed to insert logradouro: %v", err)
	}

	// Build address with only logradouro and number
	address, err := service.BuildAddress(ctx, "", "5678", 1500)
	if err != nil {
		t.Errorf("BuildAddress() error = %v", err)
	}

	if address == nil {
		t.Fatal("BuildAddress() returned nil")
	}

	expected := "Avenida Atlântica, 1500"
	if *address != expected {
		t.Errorf("BuildAddress() = %v, want %v", *address, expected)
	}
}

func TestBuildAddress_Empty(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Build address with no data
	address, err := service.BuildAddress(ctx, "", "", nil)
	if err != nil {
		t.Errorf("BuildAddress() error = %v, want nil", err)
	}

	if address != nil {
		t.Errorf("BuildAddress() = %v, want nil", address)
	}
}

func TestBuildAddress_FromCache(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test data
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "1234",
		"nome":      "Copacabana",
	})
	if err != nil {
		t.Fatalf("Failed to insert bairro: %v", err)
	}

	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "5678",
		"nome_completo": "Avenida Atlântica",
	})
	if err != nil {
		t.Fatalf("Failed to insert logradouro: %v", err)
	}

	// First call - populates cache
	_, err = service.BuildAddress(ctx, "1234", "5678", "1500")
	if err != nil {
		t.Fatalf("First BuildAddress() error = %v", err)
	}

	// Delete from MongoDB
	bairroCollection.DeleteOne(ctx, bson.M{"id_bairro": "1234"})
	logradouroCollection.DeleteOne(ctx, bson.M{"id_logradouro": "5678"})

	// Second call - should use cache
	address, err := service.BuildAddress(ctx, "1234", "5678", "1500")
	if err != nil {
		t.Errorf("BuildAddress() from cache error = %v", err)
	}

	if address == nil {
		t.Fatal("BuildAddress() from cache returned nil")
	}

	expected := "Avenida Atlântica, 1500 - Copacabana"
	if *address != expected {
		t.Errorf("BuildAddress() from cache = %v, want %v", *address, expected)
	}
}

func TestBuildAddress_NumberTypes(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test logradouro
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err := logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "5678",
		"nome_completo": "Avenida Atlântica",
	})
	if err != nil {
		t.Fatalf("Failed to insert logradouro: %v", err)
	}

	tests := []struct {
		name     string
		numero   interface{}
		expected string
	}{
		{"int", 1500, "Avenida Atlântica, 1500"},
		{"int32", int32(1500), "Avenida Atlântica, 1500"},
		{"int64", int64(1500), "Avenida Atlântica, 1500"},
		{"float64", float64(1500), "Avenida Atlântica, 1500"},
		{"string", "1500", "Avenida Atlântica, 1500"},
		{"zero string", "0", "Avenida Atlântica"},
		{"empty string", "", "Avenida Atlântica"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache
			config.Redis.Del(ctx, "address:full::5678:"+fmt.Sprint(tt.numero))

			address, err := service.BuildAddress(ctx, "", "5678", tt.numero)
			if err != nil {
				t.Errorf("BuildAddress() error = %v", err)
			}

			if address == nil {
				t.Fatal("BuildAddress() returned nil")
			}

			if *address != tt.expected {
				t.Errorf("BuildAddress() = %v, want %v", *address, tt.expected)
			}
		})
	}
}

func TestNewAddressService(t *testing.T) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping: MONGODB_URI not set")
	}

	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	database := client.Database("test")
	logger := zap.NewNop()

	service := NewAddressService(client, database, logger)
	assert.NotNil(t, service)
	assert.Equal(t, client, service.mongoClient)
	assert.Equal(t, database, service.database)
	assert.Equal(t, logger, service.logger)
}

func TestGetBairroByID_WithTestify(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test bairro
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "testify-001",
		"nome":      "Ipanema",
	})
	require.NoError(t, err)

	// Test successful retrieval
	nome, err := service.GetBairroByID(ctx, "testify-001")
	assert.NoError(t, err)
	assert.Equal(t, "Ipanema", nome)

	// Verify caching
	cacheKey := "address:bairro:testify-001"
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, "Ipanema", cached)
}

func TestGetLogradouroByID_WithTestify(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test logradouro
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err := logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "testify-002",
		"nome_completo": "Rua Visconde de Pirajá",
	})
	require.NoError(t, err)

	// Test successful retrieval
	nome, err := service.GetLogradouroByID(ctx, "testify-002")
	assert.NoError(t, err)
	assert.Equal(t, "Rua Visconde de Pirajá", nome)

	// Verify caching
	cacheKey := "address:logradouro:testify-002"
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, "Rua Visconde de Pirajá", cached)
}

func TestBuildAddress_CompleteWithTestify(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test data
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "testify-bairro",
		"nome":      "Leblon",
	})
	require.NoError(t, err)

	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "testify-logradouro",
		"nome_completo": "Avenida Ataulfo de Paiva",
	})
	require.NoError(t, err)

	tests := []struct {
		name         string
		bairroID     string
		logradouroID string
		numero       interface{}
		expected     string
	}{
		{
			name:         "Complete address with string number",
			bairroID:     "testify-bairro",
			logradouroID: "testify-logradouro",
			numero:       "500",
			expected:     "Avenida Ataulfo de Paiva, 500 - Leblon",
		},
		{
			name:         "Complete address with int number",
			bairroID:     "testify-bairro",
			logradouroID: "testify-logradouro",
			numero:       600,
			expected:     "Avenida Ataulfo de Paiva, 600 - Leblon",
		},
		{
			name:         "Without number",
			bairroID:     "testify-bairro",
			logradouroID: "testify-logradouro",
			numero:       nil,
			expected:     "Avenida Ataulfo de Paiva - Leblon",
		},
		{
			name:         "Bairro only",
			bairroID:     "testify-bairro",
			logradouroID: "",
			numero:       nil,
			expected:     "Leblon",
		},
		{
			name:         "Logradouro with number only",
			bairroID:     "",
			logradouroID: "testify-logradouro",
			numero:       700,
			expected:     "Avenida Ataulfo de Paiva, 700",
		},
		{
			name:         "Logradouro only without number",
			bairroID:     "",
			logradouroID: "testify-logradouro",
			numero:       nil,
			expected:     "Avenida Ataulfo de Paiva",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache
			cacheKey := fmt.Sprintf("address:full:%s:%s:%v", tt.bairroID, tt.logradouroID, tt.numero)
			config.Redis.Del(ctx, cacheKey)

			address, err := service.BuildAddress(ctx, tt.bairroID, tt.logradouroID, tt.numero)
			assert.NoError(t, err)
			require.NotNil(t, address)
			assert.Equal(t, tt.expected, *address)
		})
	}
}

func TestBuildAddress_NotFoundCases(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name         string
		bairroID     string
		logradouroID string
		numero       interface{}
		expectNil    bool
	}{
		{
			name:         "Both IDs empty",
			bairroID:     "",
			logradouroID: "",
			numero:       nil,
			expectNil:    true,
		},
		{
			name:         "Non-existent bairro ID",
			bairroID:     "nonexistent-bairro",
			logradouroID: "",
			numero:       nil,
			expectNil:    true,
		},
		{
			name:         "Non-existent logradouro ID",
			bairroID:     "",
			logradouroID: "nonexistent-logradouro",
			numero:       nil,
			expectNil:    true,
		},
		{
			name:         "Both non-existent",
			bairroID:     "nonexistent-bairro",
			logradouroID: "nonexistent-logradouro",
			numero:       nil,
			expectNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			address, err := service.BuildAddress(ctx, tt.bairroID, tt.logradouroID, tt.numero)
			assert.NoError(t, err)
			if tt.expectNil {
				assert.Nil(t, address)
			}
		})
	}
}

func TestGetBairroByID_CacheFailureHandling(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test bairro
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "cache-test-001",
		"nome":      "Botafogo",
	})
	require.NoError(t, err)

	// Get bairro - should retrieve from MongoDB even if cache is not working
	nome, err := service.GetBairroByID(ctx, "cache-test-001")
	assert.NoError(t, err)
	assert.Equal(t, "Botafogo", nome)
}

func TestGetLogradouroByID_CacheFailureHandling(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test logradouro
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err := logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "cache-test-002",
		"nome_completo": "Praia de Botafogo",
	})
	require.NoError(t, err)

	// Get logradouro - should retrieve from MongoDB even if cache is not working
	nome, err := service.GetLogradouroByID(ctx, "cache-test-002")
	assert.NoError(t, err)
	assert.Equal(t, "Praia de Botafogo", nome)
}

func TestBuildAddress_ComponentCaching(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test data
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "comp-cache-001",
		"nome":      "Tijuca",
	})
	require.NoError(t, err)

	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "comp-cache-002",
		"nome_completo": "Rua Conde de Bonfim",
	})
	require.NoError(t, err)

	// Build address - should cache all components
	address, err := service.BuildAddress(ctx, "comp-cache-001", "comp-cache-002", "100")
	assert.NoError(t, err)
	require.NotNil(t, address)
	assert.Equal(t, "Rua Conde de Bonfim, 100 - Tijuca", *address)

	// Verify individual components are cached
	bairroCache, err := config.Redis.Get(ctx, "address:bairro:comp-cache-001").Result()
	assert.NoError(t, err)
	assert.Equal(t, "Tijuca", bairroCache)

	logradouroCache, err := config.Redis.Get(ctx, "address:logradouro:comp-cache-002").Result()
	assert.NoError(t, err)
	assert.Equal(t, "Rua Conde de Bonfim", logradouroCache)

	// Verify full address is cached
	fullCache, err := config.Redis.Get(ctx, "address:full:comp-cache-001:comp-cache-002:100").Result()
	assert.NoError(t, err)
	assert.Equal(t, "Rua Conde de Bonfim, 100 - Tijuca", fullCache)
}

func TestBuildAddress_NumberEdgeCases(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test logradouro
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err := logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "edge-case-001",
		"nome_completo": "Rua Test",
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		numero   interface{}
		expected string
	}{
		{
			name:     "Zero int",
			numero:   0,
			expected: "Rua Test",
		},
		{
			name:     "Zero string",
			numero:   "0",
			expected: "Rua Test",
		},
		{
			name:     "Empty string",
			numero:   "",
			expected: "Rua Test",
		},
		{
			name:     "Negative number",
			numero:   -5,
			expected: "Rua Test, -5",
		},
		{
			name:     "Large number",
			numero:   999999,
			expected: "Rua Test, 999999",
		},
		{
			name:     "Float with decimals",
			numero:   123.456,
			expected: "Rua Test, 123",
		},
		{
			name:     "String with letters",
			numero:   "123A",
			expected: "Rua Test, 123A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache
			cacheKey := fmt.Sprintf("address:full::edge-case-001:%v", tt.numero)
			config.Redis.Del(ctx, cacheKey)

			address, err := service.BuildAddress(ctx, "", "edge-case-001", tt.numero)
			assert.NoError(t, err)
			require.NotNil(t, address)
			assert.Equal(t, tt.expected, *address)
		})
	}
}

func TestBuildAddress_PartialData(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert only bairro
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "partial-001",
		"nome":      "Maracanã",
	})
	require.NoError(t, err)

	// Test with valid bairro but invalid logradouro
	address, err := service.BuildAddress(ctx, "partial-001", "invalid-logradouro", "50")
	assert.NoError(t, err)
	require.NotNil(t, address)
	assert.Equal(t, "Maracanã", *address)

	// Insert only logradouro
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "partial-002",
		"nome_completo": "Rua São Francisco Xavier",
	})
	require.NoError(t, err)

	// Test with invalid bairro but valid logradouro
	address, err = service.BuildAddress(ctx, "invalid-bairro", "partial-002", "200")
	assert.NoError(t, err)
	require.NotNil(t, address)
	assert.Equal(t, "Rua São Francisco Xavier, 200", *address)
}

func TestAddressService_ConcurrentAccess(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test data
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "concurrent-001",
		"nome":      "Centro",
	})
	require.NoError(t, err)

	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "concurrent-002",
		"nome_completo": "Avenida Rio Branco",
	})
	require.NoError(t, err)

	// Test concurrent access
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(num int) {
			address, err := service.BuildAddress(ctx, "concurrent-001", "concurrent-002", num)
			assert.NoError(t, err)
			assert.NotNil(t, address)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestAddressService_EmptyNames(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert bairro with empty name
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "empty-name-001",
		"nome":      "",
	})
	require.NoError(t, err)

	// Test with empty bairro name
	nome, err := service.GetBairroByID(ctx, "empty-name-001")
	assert.NoError(t, err)
	assert.Equal(t, "", nome)

	// Insert logradouro with empty name
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "empty-name-002",
		"nome_completo": "",
	})
	require.NoError(t, err)

	// Test with empty logradouro name
	nomeLogradouro, err := service.GetLogradouroByID(ctx, "empty-name-002")
	assert.NoError(t, err)
	assert.Equal(t, "", nomeLogradouro)

	// Build address with empty names should return nil
	address, err := service.BuildAddress(ctx, "empty-name-001", "empty-name-002", nil)
	assert.NoError(t, err)
	assert.Nil(t, address)
}

func TestAddressService_SpecialCharacters(t *testing.T) {
	service, cleanup := setupAddressServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert bairro with special characters
	bairroCollection := config.MongoDB.Collection(config.AppConfig.BairroCollection)
	_, err := bairroCollection.InsertOne(ctx, bson.M{
		"id_bairro": "special-001",
		"nome":      "São Cristóvão",
	})
	require.NoError(t, err)

	// Insert logradouro with special characters
	logradouroCollection := config.MongoDB.Collection(config.AppConfig.LogradouroCollection)
	_, err = logradouroCollection.InsertOne(ctx, bson.M{
		"id_logradouro": "special-002",
		"nome_completo": "Avenida Pedro Álvares Cabral",
	})
	require.NoError(t, err)

	// Build address with special characters
	address, err := service.BuildAddress(ctx, "special-001", "special-002", "150")
	assert.NoError(t, err)
	require.NotNil(t, address)
	assert.Equal(t, "Avenida Pedro Álvares Cabral, 150 - São Cristóvão", *address)

	// Verify caching works with special characters
	cached, err := config.Redis.Get(ctx, "address:full:special-001:special-002:150").Result()
	assert.NoError(t, err)
	assert.Equal(t, "Avenida Pedro Álvares Cabral, 150 - São Cristóvão", cached)
}
