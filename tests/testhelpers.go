package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestContainers holds references to test containers
type TestContainers struct {
	MongoContainer *mongodb.MongoDBContainer
	RedisContainer *redis.RedisContainer
	MongoDB        *mongo.Database
	Cleanup        func()
}

// SetupTestContainers starts MongoDB and Redis containers for testing
func SetupTestContainers(t *testing.T) *TestContainers {
	ctx := context.Background()

	// Start MongoDB container
	mongoContainer, err := mongodb.Run(ctx,
		"mongo:7.0",
		mongodb.WithUsername("root"),
		mongodb.WithPassword("password"),
	)
	require.NoError(t, err, "Failed to start MongoDB container")

	// Start Redis container
	redisContainer, err := redis.Run(ctx,
		"redis:7-alpine",
	)
	require.NoError(t, err, "Failed to start Redis container")

	// Get MongoDB connection string
	mongoURI, err := mongoContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get MongoDB connection string")

	// Get Redis connection string
	redisURI, err := redisContainer.ConnectionString(ctx)
	require.NoError(t, err, "Failed to get Redis connection string")

	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(mongoURI)
	mongoClient, err := mongo.Connect(ctx, clientOptions)
	require.NoError(t, err, "Failed to connect to MongoDB")

	// Ping MongoDB
	err = mongoClient.Ping(ctx, nil)
	require.NoError(t, err, "Failed to ping MongoDB")

	// Get test database
	database := mongoClient.Database("rmi_test")

	// Initialize config for tests
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	// Set test configuration
	config.AppConfig.MongoURI = mongoURI
	config.AppConfig.MongoDatabase = "rmi_test"
	config.AppConfig.RedisURI = redisURI
	config.AppConfig.CitizenCollection = "citizens"
	config.AppConfig.SelfDeclaredCollection = "self_declared"
	config.AppConfig.PhoneVerificationCollection = "phone_verifications"
	config.AppConfig.UserConfigCollection = "user_config"
	config.AppConfig.MaintenanceRequestCollection = "maintenance_requests"
	config.AppConfig.PhoneMappingCollection = "phone_cpf_mappings"
	config.AppConfig.OptInHistoryCollection = "opt_in_history"
	config.AppConfig.BetaGroupCollection = "beta_groups"
	config.AppConfig.AuditLogsCollection = "audit_logs"
	config.AppConfig.BairroCollection = "bairros"
	config.AppConfig.LogradouroCollection = "logradouros"
	config.AppConfig.AvatarsCollection = "avatars"
	config.AppConfig.LegalEntityCollection = "legal_entities"
	config.AppConfig.PetCollection = "pets"
	config.AppConfig.PetsSelfRegisteredCollection = "pets_self_registered"
	config.AppConfig.ChatMemoryCollection = "chat_memory"
	config.AppConfig.DepartmentCollection = "departments"
	config.AppConfig.NotificationCategoryCollection = "notification_categories"
	config.AppConfig.CNAECollection = "cnaes"
	config.AppConfig.CFLookupCollection = "cf_lookups"
	config.AppConfig.PhoneVerificationTTL = 5 * time.Minute
	config.AppConfig.PhoneQuarantineTTL = 180 * 24 * time.Hour
	config.AppConfig.BetaStatusCacheTTL = 24 * time.Hour
	config.AppConfig.SelfDeclaredOutdatedThreshold = 180 * 24 * time.Hour
	config.AppConfig.AddressCacheTTL = 6 * time.Hour
	config.AppConfig.AvatarCacheTTL = 1 * time.Hour
	config.AppConfig.NotificationCategoryCacheTTL = 6 * time.Hour
	config.AppConfig.CFLookupCacheTTL = 24 * time.Hour
	config.AppConfig.CFLookupRateLimit = 1 * time.Hour
	config.AppConfig.CFLookupGlobalRateLimit = 100
	config.AppConfig.CFLookupSyncTimeout = 8 * time.Second
	config.AppConfig.IndexMaintenanceInterval = 1 * time.Hour
	config.AppConfig.RedisTTL = 60 * time.Minute
	config.AppConfig.RedisDB = 0
	config.AppConfig.RedisPassword = ""
	config.AppConfig.RedisPoolSize = 10
	config.AppConfig.RedisMinIdleConns = 5
	config.AppConfig.RedisDialTimeout = 5 * time.Second
	config.AppConfig.RedisReadTimeout = 3 * time.Second
	config.AppConfig.RedisWriteTimeout = 3 * time.Second
	config.AppConfig.RedisPoolTimeout = 4 * time.Second
	config.AppConfig.AuditLogsEnabled = true
	config.AppConfig.AuditWorkerCount = 5
	config.AppConfig.AuditBufferSize = 100
	config.AppConfig.VerificationWorkerCount = 5
	config.AppConfig.VerificationQueueSize = 100
	config.AppConfig.DBWorkerCount = 5
	config.AppConfig.DBBatchSize = 50
	config.AppConfig.AdminGroup = "rmi-admin"
	config.AppConfig.WhatsAppEnabled = false
	config.AppConfig.CFLookupEnabled = false

	// Set global MongoDB reference
	config.MongoDB = database

	cleanup := func() {
		// Disconnect MongoDB
		if mongoClient != nil {
			ctx := context.Background()
			mongoClient.Disconnect(ctx)
		}

		// Terminate containers
		if mongoContainer != nil {
			mongoContainer.Terminate(ctx)
		}
		if redisContainer != nil {
			redisContainer.Terminate(ctx)
		}
	}

	return &TestContainers{
		MongoContainer: mongoContainer,
		RedisContainer: redisContainer,
		MongoDB:        database,
		Cleanup:        cleanup,
	}
}

// CleanupDatabase drops all collections in the test database
func CleanupDatabase(t *testing.T, db *mongo.Database) {
	ctx := context.Background()
	collections, err := db.ListCollectionNames(ctx, map[string]interface{}{})
	require.NoError(t, err, "Failed to list collections")

	for _, collection := range collections {
		err := db.Collection(collection).Drop(ctx)
		require.NoError(t, err, fmt.Sprintf("Failed to drop collection %s", collection))
	}
}
