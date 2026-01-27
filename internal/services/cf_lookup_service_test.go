package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// setupCFLookupTest initializes MongoDB and Redis for CF lookup service tests
func setupCFLookupTest(t *testing.T) (*CFLookupService, func()) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	// Initialize logging
	_ = logging.InitLogger()

	// Check if MongoDB is initialized (via TestMain in common_test.go)
	ctx := context.Background()
	if config.MongoDB == nil {
		t.Skip("MongoDB not initialized")
	}

	// Initialize Redis
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})
	config.Redis = redisclient.NewClient(singleClient)

	// Test Redis connection
	err := config.Redis.Ping(ctx).Err()
	if err != nil {
		t.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Initialize config
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.CFLookupCollection = "test_cf_lookups"
	config.AppConfig.CFLookupCacheTTL = 5 * time.Minute
	config.AppConfig.CFLookupEnabled = true
	config.AppConfig.CFLookupSyncTimeout = 8 * time.Second

	// Create service (without MCP client for most tests)
	logger := &logging.SafeLogger{}
	service := NewCFLookupService(config.MongoDB, nil, logger)

	// Return cleanup function
	return service, func() {
		// Clean up Redis keys
		keys, _ := config.Redis.Keys(ctx, "cf_lookup:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}
		// Drop only test collection
		_ = config.MongoDB.Collection(config.AppConfig.CFLookupCollection).Drop(ctx)
	}
}

func TestNewCFLookupService(t *testing.T) {
	logger := &logging.SafeLogger{}
	database := &mongo.Database{}
	mcpClient := &MCPClient{}

	service := NewCFLookupService(database, mcpClient, logger)

	assert.NotNil(t, service)
	assert.Equal(t, database, service.database)
	assert.Equal(t, mcpClient, service.mcpClient)
	assert.Equal(t, logger, service.logger)
}

func TestGenerateAddressHash(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	tests := []struct {
		name     string
		address1 string
		address2 string
		wantSame bool
	}{
		{
			name:     "identical addresses",
			address1: "Rua A, 123 - Bairro X",
			address2: "Rua A, 123 - Bairro X",
			wantSame: true,
		},
		{
			name:     "case insensitive",
			address1: "Rua A, 123 - Bairro X",
			address2: "RUA A, 123 - BAIRRO X",
			wantSame: true,
		},
		{
			name:     "whitespace differences",
			address1: "Rua A, 123 - Bairro X",
			address2: "  Rua A, 123 - Bairro X  ",
			wantSame: true,
		},
		{
			name:     "different addresses",
			address1: "Rua A, 123 - Bairro X",
			address2: "Rua B, 456 - Bairro Y",
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := service.GenerateAddressHash(tt.address1)
			hash2 := service.GenerateAddressHash(tt.address2)

			assert.NotEmpty(t, hash1)
			assert.NotEmpty(t, hash2)

			if tt.wantSame {
				assert.Equal(t, hash1, hash2, "hashes should be equal")
			} else {
				assert.NotEqual(t, hash1, hash2, "hashes should be different")
			}
		})
	}
}

func TestExtractAddress(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	tests := []struct {
		name         string
		citizenData  *models.Citizen
		wantAddress  string
		wantContains string
	}{
		{
			name: "self-declared address",
			citizenData: &models.Citizen{
				Endereco: &models.Endereco{
					Principal: &models.EnderecoPrincipal{
						Logradouro: strPtr("Rua Test"),
						Numero:     strPtr("123"),
						Bairro:     strPtr("Copacabana"),
						Municipio:  strPtr("Rio de Janeiro"),
						Estado:     strPtr("RJ"),
						Origem:     strPtr("self-declared"),
					},
				},
			},
			wantContains: "Rua Test",
		},
		{
			name: "base data address",
			citizenData: &models.Citizen{
				Endereco: &models.Endereco{
					Principal: &models.EnderecoPrincipal{
						Logradouro: strPtr("Avenida Atlântica"),
						Numero:     strPtr("1500"),
						Bairro:     strPtr("Copacabana"),
						Municipio:  strPtr("Rio de Janeiro"),
						Estado:     strPtr("RJ"),
					},
				},
			},
			wantContains: "Avenida Atlântica",
		},
		{
			name: "no address",
			citizenData: &models.Citizen{
				Endereco: nil,
			},
			wantAddress: "",
		},
		{
			name: "empty logradouro",
			citizenData: &models.Citizen{
				Endereco: &models.Endereco{
					Principal: &models.EnderecoPrincipal{
						Logradouro: strPtr(""),
						Numero:     strPtr("123"),
					},
				},
			},
			wantAddress: "",
		},
		{
			name: "nil logradouro",
			citizenData: &models.Citizen{
				Endereco: &models.Endereco{
					Principal: &models.EnderecoPrincipal{
						Logradouro: nil,
					},
				},
			},
			wantAddress: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			address := service.ExtractAddress(tt.citizenData)

			if tt.wantAddress != "" {
				assert.Equal(t, tt.wantAddress, address)
			}

			if tt.wantContains != "" {
				assert.Contains(t, address, tt.wantContains)
			}
		})
	}
}

func TestBuildFullAddress(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	tests := []struct {
		name        string
		logradouro  *string
		numero      *string
		complemento *string
		bairro      *string
		cidade      *string
		estado      *string
		wantEmpty   bool
		wantContain []string
	}{
		{
			name:        "complete address",
			logradouro:  strPtr("Rua Test"),
			numero:      strPtr("123"),
			complemento: strPtr("Apto 101"),
			bairro:      strPtr("Copacabana"),
			cidade:      strPtr("Rio de Janeiro"),
			estado:      strPtr("RJ"),
			wantContain: []string{"Rua Test", "123", "Apto 101", "Copacabana", "Rio de Janeiro", "RJ"},
		},
		{
			name:        "without numero",
			logradouro:  strPtr("Rua Test"),
			bairro:      strPtr("Copacabana"),
			wantContain: []string{"Rua Test", "Copacabana"},
		},
		{
			name:        "default city and state",
			logradouro:  strPtr("Rua Test"),
			numero:      strPtr("123"),
			wantContain: []string{"Rua Test", "123", "Rio de Janeiro", "RJ"},
		},
		{
			name:      "no logradouro",
			numero:    strPtr("123"),
			wantEmpty: true,
		},
		{
			name:       "empty logradouro",
			logradouro: strPtr(""),
			numero:     strPtr("123"),
			wantEmpty:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			address := service.buildFullAddress(tt.logradouro, tt.numero, tt.complemento, tt.bairro, tt.cidade, tt.estado)

			if tt.wantEmpty {
				assert.Empty(t, address)
			} else {
				for _, want := range tt.wantContain {
					assert.Contains(t, address, want)
				}
			}
		})
	}
}

func TestCategorizeError(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	tests := []struct {
		name     string
		err      error
		wantType string
	}{
		{
			name:     "timeout error",
			err:      fmt.Errorf("context deadline exceeded"),
			wantType: "timeout",
		},
		{
			name:     "network error",
			err:      fmt.Errorf("connection refused"),
			wantType: "network",
		},
		{
			name:     "unauthorized error",
			err:      fmt.Errorf("401 unauthorized"),
			wantType: "authorization",
		},
		{
			name:     "validation error",
			err:      fmt.Errorf("invalid address format"),
			wantType: "validation",
		},
		{
			name:     "server error",
			err:      fmt.Errorf("500 internal server error"),
			wantType: "server",
		},
		{
			name:     "unknown error",
			err:      fmt.Errorf("something went wrong"),
			wantType: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorType := service.categorizeError(tt.err)
			assert.Equal(t, tt.wantType, errorType)
		})
	}
}

func TestStoreCFLookup(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	cfLookup := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		AddressUsed: "Rua Test, 123 - Copacabana",
		CFData: models.CFInfo{
			NomeOficial: "CF Copacabana",
			NomePopular: "Clínica Copacabana",
			Logradouro:  "Rua da Clínica",
			Numero:      "456",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		DistanceMeters: 500,
		LookupSource:   "mcp",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		IsActive:       true,
	}

	err := service.storeCFLookup(ctx, cfLookup)
	assert.NoError(t, err)

	// Verify it was stored
	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	var stored models.CFLookup
	err = collection.FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&stored)
	assert.NoError(t, err)
	assert.Equal(t, cfLookup.CPF, stored.CPF)
	assert.Equal(t, cfLookup.AddressHash, stored.AddressHash)
	assert.Equal(t, cfLookup.CFData.NomePopular, stored.CFData.NomePopular)
}

func TestStoreCFLookup_Upsert(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Store initial CF lookup
	cfLookup1 := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		AddressUsed: "Rua Test, 123 - Copacabana",
		CFData: models.CFInfo{
			NomeOficial: "CF Original",
			NomePopular: "Clínica Original",
			Logradouro:  "Rua Original",
			Numero:      "100",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		LookupSource: "mcp",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		IsActive:     true,
	}

	err := service.storeCFLookup(ctx, cfLookup1)
	assert.NoError(t, err)

	// Update with new address
	cfLookup2 := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash456",
		AddressUsed: "Rua Nova, 456 - Ipanema",
		CFData: models.CFInfo{
			NomeOficial: "CF Nova",
			NomePopular: "Clínica Nova",
			Logradouro:  "Rua Nova",
			Numero:      "200",
			Bairro:      "Ipanema",
			Ativo:       true,
		},
		LookupSource: "mcp",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		IsActive:     true,
	}

	err = service.storeCFLookup(ctx, cfLookup2)
	assert.NoError(t, err)

	// Verify only one document exists with updated data
	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	count, err := collection.CountDocuments(ctx, bson.M{"cpf": "12345678901"})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)

	var stored models.CFLookup
	err = collection.FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&stored)
	assert.NoError(t, err)
	assert.Equal(t, "hash456", stored.AddressHash)
	assert.Equal(t, "CF Nova", stored.CFData.NomeOficial)
}

func TestGetActiveCFLookup(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CF lookup
	cfLookup := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		AddressUsed: "Rua Test, 123 - Copacabana",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Logradouro:  "Rua Test",
			Numero:      "123",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	_, err := collection.InsertOne(ctx, cfLookup)
	assert.NoError(t, err)

	// Retrieve CF lookup
	retrieved, err := service.getActiveCFLookup(ctx, "12345678901")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, cfLookup.CPF, retrieved.CPF)
	assert.Equal(t, cfLookup.AddressHash, retrieved.AddressHash)
}

func TestGetActiveCFLookup_NotFound(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get non-existent CF lookup
	retrieved, err := service.getActiveCFLookup(ctx, "99999999999")
	assert.Error(t, err)
	assert.Equal(t, mongo.ErrNoDocuments, err)
	assert.Nil(t, retrieved)
}

func TestGetExistingCFLookup(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	addressHash := "hash123"

	// Insert test CF lookup
	cfLookup := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: addressHash,
		AddressUsed: "Rua Test, 123 - Copacabana",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	_, err := collection.InsertOne(ctx, cfLookup)
	assert.NoError(t, err)

	// Get existing CF lookup with same address hash
	existing, err := service.getExistingCFLookup(ctx, "12345678901", addressHash)
	assert.NoError(t, err)
	assert.NotNil(t, existing)
	assert.Equal(t, cfLookup.AddressHash, existing.AddressHash)
}

func TestGetExistingCFLookup_DifferentHash(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CF lookup with one address hash
	cfLookup := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		AddressUsed: "Rua Test, 123 - Copacabana",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	_, err := collection.InsertOne(ctx, cfLookup)
	assert.NoError(t, err)

	// Try to get with different address hash
	existing, err := service.getExistingCFLookup(ctx, "12345678901", "hash456")
	assert.NoError(t, err)
	assert.Nil(t, existing)
}

func TestCacheCFData(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	err := service.cacheCFData(ctx, "12345678901", cfData)
	assert.NoError(t, err)

	// Verify it was cached
	cacheKey := "cf_lookup:cpf:12345678901"
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, cached)

	// Verify cached data is correct
	var cachedData models.CFLookup
	err = json.Unmarshal([]byte(cached), &cachedData)
	assert.NoError(t, err)
	assert.Equal(t, cfData.CPF, cachedData.CPF)
	assert.Equal(t, cfData.CFData.NomePopular, cachedData.CFData.NomePopular)
}

func TestGetCachedCFData(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	// Cache the data
	err := service.cacheCFData(ctx, "12345678901", cfData)
	assert.NoError(t, err)

	// Retrieve from cache
	retrieved, err := service.getCachedCFData(ctx, "12345678901")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, cfData.CPF, retrieved.CPF)
	assert.Equal(t, cfData.CFData.NomePopular, retrieved.CFData.NomePopular)
}

func TestGetCachedCFData_NotFound(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get non-existent cached data
	retrieved, err := service.getCachedCFData(ctx, "99999999999")
	assert.Error(t, err)
	assert.Nil(t, retrieved)
}

func TestInvalidateCFCache(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	// Cache the data
	err := service.cacheCFData(ctx, "12345678901", cfData)
	assert.NoError(t, err)

	// Invalidate cache
	err = service.invalidateCFCache(ctx, "12345678901")
	assert.NoError(t, err)

	// Verify cache is invalidated
	_, err = service.getCachedCFData(ctx, "12345678901")
	assert.Error(t, err)
}

func TestGetCFDataForCitizen_FromCache(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	// Cache the data
	err := service.cacheCFData(ctx, "12345678901", cfData)
	assert.NoError(t, err)

	// Retrieve from cache via GetCFDataForCitizen
	retrieved, err := service.GetCFDataForCitizen(ctx, "12345678901")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, cfData.CPF, retrieved.CPF)
}

func TestGetCFDataForCitizen_FromDatabase(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert into database
	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	_, err := collection.InsertOne(ctx, cfData)
	assert.NoError(t, err)

	// Retrieve from database
	retrieved, err := service.GetCFDataForCitizen(ctx, "12345678901")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, cfData.CPF, retrieved.CPF)

	// Verify it was cached
	cached, err := service.getCachedCFData(ctx, "12345678901")
	assert.NoError(t, err)
	assert.NotNil(t, cached)
}

func TestGetCFDataForCitizen_NotFound(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get non-existent CF data
	retrieved, err := service.GetCFDataForCitizen(ctx, "99999999999")
	assert.NoError(t, err)
	assert.Nil(t, retrieved)
}

func TestInvalidateCFDataForAddress_AddressChanged(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert CF lookup with old address
	oldAddressHash := service.GenerateAddressHash("Rua Old, 123")
	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: oldAddressHash,
		AddressUsed: "Rua Old, 123",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	_, err := collection.InsertOne(ctx, cfData)
	assert.NoError(t, err)

	// Cache the data
	err = service.cacheCFData(ctx, "12345678901", cfData)
	assert.NoError(t, err)

	// Invalidate with new address hash
	newAddressHash := service.GenerateAddressHash("Rua New, 456")
	err = service.InvalidateCFDataForAddress(ctx, "12345678901", newAddressHash)
	assert.NoError(t, err)

	// Verify CF data was deleted
	var deleted models.CFLookup
	err = collection.FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&deleted)
	assert.Error(t, err)
	assert.Equal(t, mongo.ErrNoDocuments, err)

	// Verify cache was invalidated
	_, err = service.getCachedCFData(ctx, "12345678901")
	assert.Error(t, err)
}

func TestInvalidateCFDataForAddress_AddressUnchanged(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert CF lookup
	addressHash := service.GenerateAddressHash("Rua Test, 123")
	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: addressHash,
		AddressUsed: "Rua Test, 123",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	_, err := collection.InsertOne(ctx, cfData)
	assert.NoError(t, err)

	// Try to invalidate with same address hash
	err = service.InvalidateCFDataForAddress(ctx, "12345678901", addressHash)
	assert.NoError(t, err)

	// Verify CF data still exists
	var stillThere models.CFLookup
	err = collection.FindOne(ctx, bson.M{"cpf": "12345678901"}).Decode(&stillThere)
	assert.NoError(t, err)
	assert.Equal(t, cfData.CPF, stillThere.CPF)
}

func TestInvalidateCFDataForAddress_NoExisting(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to invalidate non-existent CF data
	err := service.InvalidateCFDataForAddress(ctx, "99999999999", "hash123")
	assert.NoError(t, err) // Should not error when no data exists
}

func TestShouldLookupCF_AlreadyHasCFData(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	indicador := true
	citizenData := &models.Citizen{
		Saude: &models.Saude{
			ClinicaFamilia: &models.ClinicaFamilia{
				Indicador: &indicador,
			},
		},
	}

	shouldLookup, address, err := service.ShouldLookupCF(ctx, "12345678901", citizenData)
	assert.NoError(t, err)
	assert.False(t, shouldLookup)
	assert.Empty(t, address)
}

func TestShouldLookupCF_NoAddress(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	citizenData := &models.Citizen{
		Endereco: nil,
	}

	shouldLookup, address, err := service.ShouldLookupCF(ctx, "12345678901", citizenData)
	assert.NoError(t, err)
	assert.False(t, shouldLookup)
	assert.Empty(t, address)
}

func TestShouldLookupCF_ExistingLookupSameAddress(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Use the same address format that buildFullAddress creates
	address := "Rua Test, 123, Copacabana, Rio de Janeiro, RJ"
	addressHash := service.GenerateAddressHash(address)

	// Insert existing CF lookup
	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: addressHash,
		AddressUsed: address,
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	_, err := collection.InsertOne(ctx, cfData)
	assert.NoError(t, err)

	// Create citizen with same address
	citizenData := &models.Citizen{
		Endereco: &models.Endereco{
			Principal: &models.EnderecoPrincipal{
				Logradouro: strPtr("Rua Test"),
				Numero:     strPtr("123"),
				Bairro:     strPtr("Copacabana"),
				Municipio:  strPtr("Rio de Janeiro"),
				Estado:     strPtr("RJ"),
			},
		},
	}

	shouldLookup, returnedAddress, err := service.ShouldLookupCF(ctx, "12345678901", citizenData)
	assert.NoError(t, err)
	assert.False(t, shouldLookup)
	assert.NotEmpty(t, returnedAddress)
}

func TestShouldLookupCF_NewAddress(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	citizenData := &models.Citizen{
		Endereco: &models.Endereco{
			Principal: &models.EnderecoPrincipal{
				Logradouro: strPtr("Rua Test"),
				Numero:     strPtr("123"),
				Bairro:     strPtr("Copacabana"),
				Municipio:  strPtr("Rio de Janeiro"),
				Estado:     strPtr("RJ"),
			},
		},
	}

	shouldLookup, address, err := service.ShouldLookupCF(ctx, "12345678901", citizenData)
	assert.NoError(t, err)
	assert.True(t, shouldLookup)
	assert.NotEmpty(t, address)
	assert.Contains(t, address, "Rua Test")
}

func TestQueueCFLookupJob(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	service.queueCFLookupJob(ctx, "12345678901", "Rua Test, 123")

	// Verify job was queued
	queueKey := "sync:queue:cf_lookup"
	jobJSON, err := config.Redis.RPop(ctx, queueKey).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, jobJSON)

	// Verify job data
	var job SyncJob
	err = json.Unmarshal([]byte(jobJSON), &job)
	assert.NoError(t, err)
	assert.Equal(t, "cf_lookup", job.Type)
	assert.Equal(t, "cf_lookup", job.Collection)

	// Convert Data to map for access
	dataMap, ok := job.Data.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "12345678901", dataMap["cpf"])
	assert.Equal(t, "Rua Test, 123", dataMap["address"])
}

func TestTrySynchronousCFLookup_NilService(t *testing.T) {
	var service *CFLookupService = nil
	ctx := context.Background()

	result, err := service.TrySynchronousCFLookup(ctx, "12345678901", "Rua Test, 123")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not available")
}

func TestTrySynchronousCFLookup_NilMCPClient(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Service has nil MCP client
	result, err := service.TrySynchronousCFLookup(ctx, "12345678901", "Rua Test, 123")
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "MCP client not available")
}

func TestTrySynchronousCFLookup_CachedData(t *testing.T) {
	service, cleanup := setupCFLookupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create and cache CF data
	cfData := &models.CFLookup{
		ID:          primitive.NewObjectID(),
		CPF:         "12345678901",
		AddressHash: "hash123",
		CFData: models.CFInfo{
			NomeOficial: "CF Test",
			NomePopular: "Clínica Test",
			Bairro:      "Copacabana",
			Ativo:       true,
		},
		IsActive: true,
	}

	collection := config.MongoDB.Collection(config.AppConfig.CFLookupCollection)
	_, err := collection.InsertOne(ctx, cfData)
	assert.NoError(t, err)

	err = service.cacheCFData(ctx, "12345678901", cfData)
	assert.NoError(t, err)

	// Create mock MCP client
	logger := &logging.SafeLogger{}
	service.mcpClient = NewMCPClient(config.AppConfig, logger)

	// Should return cached data without calling MCP
	result, err := service.TrySynchronousCFLookup(ctx, "12345678901", "Rua Test, 123")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "12345678901", result.CPF)
}

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}
