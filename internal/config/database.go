package config

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.opentelemetry.io/contrib/instrumentation/go.mongodb.org/mongo-driver/mongo/otelmongo"
	"go.uber.org/zap"
)

var (
	// MongoDB client
	MongoDB *mongo.Database
	// Redis client
	Redis *redisclient.Client
)

// InitMongoDB initializes the MongoDB connection
func InitMongoDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Configure MongoDB with optimizations
	opts := options.Client().
		ApplyURI(AppConfig.MongoURI).
		SetMonitor(otelmongo.NewMonitor()) // Add OpenTelemetry instrumentation
		// All MongoDB connection parameters are now configured via URI
		// This allows for easier tuning through environment variables

	// Add connection pool monitoring
	opts.SetPoolMonitor(&event.PoolMonitor{
		Event: func(evt *event.PoolEvent) {
			switch evt.Type {
			case event.GetSucceeded:
				logging.Logger.Info("MongoDB connection acquired",
					zap.Uint64("connection_id", evt.ConnectionID))
			case event.GetFailed:
				logging.Logger.Warn("MongoDB connection acquisition failed",
					zap.Uint64("connection_id", evt.ConnectionID))
			case event.ConnectionReturned:
				logging.Logger.Info("MongoDB connection returned to pool",
					zap.Uint64("connection_id", evt.ConnectionID))
			}
		},
	})

	client, err := mongo.Connect(ctx, opts)
	if err != nil {
		log.Fatal(err)
	}

	// Ping the database
	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatal(err)
	}

	MongoDB = client.Database(AppConfig.MongoDatabase)

	// Ensure indexes exist and start maintenance routine
	if err := ensureIndexes(); err != nil {
		logging.Logger.Error("failed to ensure indexes on startup", zap.Error(err))
	}
	startIndexMaintenance()

	logging.Logger.Info("Connected to MongoDB",
		zap.String("uri", maskMongoURI(AppConfig.MongoURI)),
		zap.String("database", AppConfig.MongoDatabase),
	)

	// Start connection pool monitoring
	go monitorConnectionPool()
}

// InitRedis initializes the Redis connection
func InitRedis() {
	// Initialize Redis client with production-optimized settings
	redisClient := redis.NewClient(&redis.Options{
		Addr:         AppConfig.RedisURI,
		Password:     AppConfig.RedisPassword,
		DB:           AppConfig.RedisDB,
		
		// Connection timeouts - configurable via environment variables
		DialTimeout:  AppConfig.RedisDialTimeout,
		ReadTimeout:  AppConfig.RedisReadTimeout,
		WriteTimeout: AppConfig.RedisWriteTimeout,
		
		// Connection pool optimization - configurable via environment variables
		PoolSize:     AppConfig.RedisPoolSize,
		MinIdleConns: AppConfig.RedisMinIdleConns,
		MaxRetries:   3,         // Retry failed commands
		
		// Connection health checks
		IdleTimeout:  5 * time.Minute,  // Close idle connections
		MaxConnAge:   30 * time.Minute, // Rotate connections
		
		// Circuit breaker for high load - configurable via environment variables
		PoolTimeout: AppConfig.RedisPoolTimeout,
	})

	// Wrap with traced client
	Redis = redisclient.NewClient(redisClient)

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := Redis.Ping(ctx).Err(); err != nil {
		logging.Logger.Error("failed to connect to Redis",
			zap.String("uri", AppConfig.RedisURI),
			zap.Error(err))
		return
	}

	logging.Logger.Info("connected to Redis",
		zap.String("uri", AppConfig.RedisURI),
		zap.Int("pool_size", AppConfig.RedisPoolSize),
		zap.Int("min_idle_conns", AppConfig.RedisMinIdleConns))

	// Start Redis connection pool monitoring
	go monitorRedisConnectionPool()
}

// maskMongoURI masks sensitive information in MongoDB URI
func maskMongoURI(uri string) string {
	// Implementation to mask username/password in URI
	return "mongodb://****:****@" + uri[strings.LastIndex(uri, "@")+1:]
}

// ensureIndexes creates required indexes if they don't exist
func ensureIndexes() error {
	logger := zap.L().Named("database")
	logger.Info("ensuring required indexes exist")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if we can write to the database (primary node or direct connection)
	if err := checkWriteAccess(ctx, logger); err != nil {
		logger.Warn("cannot write to database, skipping index creation", zap.Error(err))
		return nil
	}

	// Ensure citizen collection index
	if err := ensureCitizenIndex(ctx, logger); err != nil {
		return err
	}

	// Ensure maintenance request collection index
	if err := ensureMaintenanceRequestIndex(ctx, logger); err != nil {
		return err
	}

	// Ensure self_declared collection index
	if err := ensureSelfDeclaredIndex(ctx, logger); err != nil {
		return err
	}

	// Ensure phone_verifications collection index
	if err := ensurePhoneVerificationIndex(ctx, logger); err != nil {
		return err
	}

	// Ensure user_config collection index
	if err := ensureUserConfigIndex(ctx, logger); err != nil {
		return err
	}

	// Ensure audit_logs collection index
	if err := ensureAuditLogsIndex(ctx, logger); err != nil {
		return err
	}

	// Ensure phone_mapping collection index
	if err := ensurePhoneMappingIndex(ctx, logger); err != nil {
		return err
	}

	// Ensure opt_in_history collection index
	if err := ensureOptInHistoryIndex(ctx, logger); err != nil {
		return err
	}

	// Ensure beta_group collection index
	if err := ensureBetaGroupIndex(ctx, logger); err != nil {
		return err
	}

	logger.Info("all required indexes verified")
	return nil
}

// checkWriteAccess checks if we can write to the database
func checkWriteAccess(ctx context.Context, logger *zap.Logger) error {
	// Try to create a test document to verify write access
	testCollection := MongoDB.Collection("_test_write_access")
	
	// Use a unique test document ID to avoid conflicts
	testDoc := bson.M{
		"_id": primitive.NewObjectID(),
		"test": true,
		"timestamp": time.Now(),
	}
	
	_, err := testCollection.InsertOne(ctx, testDoc)
	if err != nil {
		return fmt.Errorf("cannot write to database: %w", err)
	}
	
	// Clean up the test document
	_, err = testCollection.DeleteOne(ctx, bson.M{"_id": testDoc["_id"]})
	if err != nil {
		logger.Warn("failed to clean up test document", zap.Error(err))
	}
	
	return nil
}

// ensureCitizenIndex creates the unique index on cpf for citizen collection
func ensureCitizenIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.CitizenCollection)
	
	// Check if index already exists
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	indexExists := false
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok && name == "cpf_1" {
			indexExists = true
			break
		}
	}

	if indexExists {
		logger.Debug("citizen collection index already exists", zap.String("collection", AppConfig.CitizenCollection))
		return nil
	}

	// Create unique index on cpf
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "cpf", Value: 1}},
		Options: options.Index().
			SetName("cpf_1").
			SetUnique(true),
	}

	_, err = collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		// Check if it's a duplicate key error (another instance created it)
		if mongo.IsDuplicateKeyError(err) {
			logger.Info("citizen index already exists (created by another instance)", 
				zap.String("collection", AppConfig.CitizenCollection))
			return nil
		}
		logger.Error("failed to create citizen index", 
			zap.String("collection", AppConfig.CitizenCollection),
			zap.Error(err))
		return err
	}

	logger.Info("created citizen collection index", 
		zap.String("collection", AppConfig.CitizenCollection),
		zap.String("index", "cpf_1"))
	return nil
}

// ensureMaintenanceRequestIndex creates the index on cpf for maintenance request collection
func ensureMaintenanceRequestIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.MaintenanceRequestCollection)
	
	// Check if index already exists
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	indexExists := false
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok && name == "cpf_1" {
			indexExists = true
			break
		}
	}

	if indexExists {
		logger.Debug("maintenance request collection index already exists", 
			zap.String("collection", AppConfig.MaintenanceRequestCollection))
		return nil
	}

	// Create non-unique index on cpf
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "cpf", Value: 1}},
		Options: options.Index().
			SetName("cpf_1"),
	}

	_, err = collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		// Check if it's a duplicate key error (another instance created it)
		if mongo.IsDuplicateKeyError(err) {
			logger.Info("maintenance request index already exists (created by another instance)", 
				zap.String("collection", AppConfig.MaintenanceRequestCollection))
			return nil
		}
		logger.Error("failed to create maintenance request index", 
			zap.String("collection", AppConfig.MaintenanceRequestCollection),
			zap.Error(err))
		return err
	}

	logger.Info("created maintenance request collection index", 
		zap.String("collection", AppConfig.MaintenanceRequestCollection),
		zap.String("index", "cpf_1"))
	return nil
}

// startIndexMaintenance starts a goroutine that periodically ensures indexes exist
func startIndexMaintenance() {
	logger := zap.L().Named("database")
	
	go func() {
		ticker := time.NewTicker(AppConfig.IndexMaintenanceInterval)
		defer ticker.Stop()

		// Initial check
		if err := ensureIndexes(); err != nil {
			logger.Error("initial index check failed", zap.Error(err))
		}

		for range ticker.C {
			if err := ensureIndexes(); err != nil {
				logger.Error("periodic index check failed", zap.Error(err))
			}
		}
	}()

	logger.Info("started index maintenance routine", 
		zap.Duration("interval", AppConfig.IndexMaintenanceInterval))
}

// ensureSelfDeclaredIndex creates the unique index on cpf for self_declared collection
func ensureSelfDeclaredIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.SelfDeclaredCollection)
	
	// Check if index already exists
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	indexExists := false
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok && name == "cpf_1" {
			indexExists = true
			break
		}
	}

	if indexExists {
		logger.Debug("self_declared collection index already exists", 
			zap.String("collection", AppConfig.SelfDeclaredCollection))
		return nil
	}

	// Create unique index on cpf
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "cpf", Value: 1}},
		Options: options.Index().
			SetName("cpf_1").
			SetUnique(true),
	}

	_, err = collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		// Check if it's a duplicate key error (another instance created it)
		if mongo.IsDuplicateKeyError(err) {
			logger.Info("self_declared index already exists (created by another instance)", 
				zap.String("collection", AppConfig.SelfDeclaredCollection))
			return nil
		}
		logger.Error("failed to create self_declared index", 
			zap.String("collection", AppConfig.SelfDeclaredCollection),
			zap.Error(err))
		return err
	}

	logger.Info("created self_declared collection index", 
		zap.String("collection", AppConfig.SelfDeclaredCollection),
		zap.String("index", "cpf_1"))
	return nil
}

// ensurePhoneVerificationIndex creates the required indexes for phone_verifications collection
func ensurePhoneVerificationIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.PhoneVerificationCollection)
	
	// Check if indexes already exist
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	existingIndexes := make(map[string]bool)
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok {
			existingIndexes[name] = true
		}
	}

	// Create indexes that don't exist
	indexesToCreate := []mongo.IndexModel{}

	// 1. Unique compound index on cpf and phone_number
	if !existingIndexes["cpf_1_phone_number_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "cpf", Value: 1}, {Key: "phone_number", Value: 1}},
			Options: options.Index().
				SetName("cpf_1_phone_number_1").
				SetUnique(true),
		})
	}

	// 2. TTL index on expires_at for automatic cleanup
	if !existingIndexes["expires_at_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().
				SetName("expires_at_1").
				SetExpireAfterSeconds(0),
		})
	}

	// 3. Compound index for verification queries (cpf, code, phone_number, expires_at)
	if !existingIndexes["verification_query_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{
				{Key: "cpf", Value: 1},
				{Key: "code", Value: 1},
				{Key: "phone_number", Value: 1},
				{Key: "expires_at", Value: 1},
			},
			Options: options.Index().
				SetName("verification_query_1"),
		})
	}

	// Create all missing indexes
	for _, indexModel := range indexesToCreate {
		_, err = collection.Indexes().CreateOne(ctx, indexModel)
		if err != nil {
			// Check if it's a duplicate key error (another instance created it)
			if mongo.IsDuplicateKeyError(err) {
				logger.Info("phone_verifications index already exists (created by another instance)", 
					zap.String("collection", AppConfig.PhoneVerificationCollection))
				continue
			}
			logger.Error("failed to create phone_verifications index", 
				zap.String("collection", AppConfig.PhoneVerificationCollection),
				zap.Error(err))
			return err
		}
	}

	if len(indexesToCreate) > 0 {
		logger.Info("created phone_verifications collection indexes", 
			zap.String("collection", AppConfig.PhoneVerificationCollection),
			zap.Int("count", len(indexesToCreate)))
	} else {
		logger.Debug("phone_verifications collection indexes already exist", 
			zap.String("collection", AppConfig.PhoneVerificationCollection))
	}
	
	return nil
}

// ensureUserConfigIndex creates the unique index on cpf for user_config collection
func ensureUserConfigIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.UserConfigCollection)
	
	// Check if index already exists
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	indexExists := false
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok && name == "cpf_1" {
			indexExists = true
			break
		}
	}

	if indexExists {
		logger.Debug("user_config collection index already exists", 
			zap.String("collection", AppConfig.UserConfigCollection))
		return nil
	}

	// Create unique index on cpf
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "cpf", Value: 1}},
		Options: options.Index().
			SetName("cpf_1").
			SetUnique(true),
	}

	_, err = collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		// Check if it's a duplicate key error (another instance created it)
		if mongo.IsDuplicateKeyError(err) {
			logger.Info("user_config index already exists (created by another instance)", 
				zap.String("collection", AppConfig.UserConfigCollection))
			return nil
		}
		logger.Error("failed to create user_config index", 
			zap.String("collection", AppConfig.UserConfigCollection),
			zap.Error(err))
		return err
	}

	logger.Info("created user_config collection index", 
		zap.String("collection", AppConfig.UserConfigCollection),
		zap.String("index", "cpf_1"))
	return nil
}

// ensureAuditLogsIndex creates the required indexes for audit_logs collection
func ensureAuditLogsIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.AuditLogsCollection)
	
	// Check if indexes already exist
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	existingIndexes := make(map[string]bool)
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok {
			existingIndexes[name] = true
		}
	}

	// Create indexes that don't exist
	indexesToCreate := []mongo.IndexModel{}

	// 1. Index on cpf for quick citizen lookups
	if !existingIndexes["cpf_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "cpf", Value: 1}},
			Options: options.Index().
				SetName("cpf_1"),
		})
	}

	// 2. Index on timestamp for time-based queries
	if !existingIndexes["timestamp_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "timestamp", Value: -1}},
			Options: options.Index().
				SetName("timestamp_1"),
		})
	}

	// 3. Compound index on action and resource
	if !existingIndexes["action_1_resource_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "action", Value: 1}, {Key: "resource", Value: 1}},
			Options: options.Index().
				SetName("action_1_resource_1"),
		})
	}

	// 4. TTL index for automatic cleanup (keep audit logs for 1 year)
	if !existingIndexes["timestamp_ttl"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "timestamp", Value: 1}},
			Options: options.Index().
				SetName("timestamp_ttl").
				SetExpireAfterSeconds(365 * 24 * 60 * 60), // 1 year
		})
	}

	// Create all missing indexes
	for _, indexModel := range indexesToCreate {
		_, err = collection.Indexes().CreateOne(ctx, indexModel)
		if err != nil {
			// Check if it's a duplicate key error (another instance created it)
			if mongo.IsDuplicateKeyError(err) {
						logger.Info("audit_logs index already exists (created by another instance)",
			zap.String("collection", AppConfig.AuditLogsCollection))
				continue
			}
					logger.Error("failed to create audit_logs index",
			zap.String("collection", AppConfig.AuditLogsCollection),
				zap.Error(err))
			return err
		}
	}

	if len(indexesToCreate) > 0 {
				logger.Info("created audit_logs collection indexes",
			zap.String("collection", AppConfig.AuditLogsCollection),
			zap.Int("count", len(indexesToCreate)))
	} else {
				logger.Debug("audit_logs collection indexes already exist",
			zap.String("collection", AppConfig.AuditLogsCollection))
	}
	
	return nil
} 

// ensurePhoneMappingIndex creates the required indexes for phone_cpf_mappings collection
func ensurePhoneMappingIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.PhoneMappingCollection)
	
	// Check if indexes already exist
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	existingIndexes := make(map[string]bson.M)
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok {
			existingIndexes[name] = index
		}
	}

	// Check if we need to drop the old unique index
	if oldIndex, exists := existingIndexes["phone_number_1"]; exists {
		if unique, ok := oldIndex["unique"].(bool); ok && unique {
			logger.Info("dropping old unique phone_number index to allow multiple CPFs", 
				zap.String("collection", AppConfig.PhoneMappingCollection))
			
			_, err = collection.Indexes().DropOne(ctx, "phone_number_1")
			if err != nil {
				logger.Error("failed to drop old unique phone_number index", zap.Error(err))
				return err
			}
			
			// Remove from existing indexes map since we're recreating it
			delete(existingIndexes, "phone_number_1")
		}
	}

	// Create phone_number index (non-unique to allow multiple CPFs)
	if _, exists := existingIndexes["phone_number_1"]; !exists {
		logger.Info("creating phone_number index", zap.String("collection", AppConfig.PhoneMappingCollection))
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "phone_number", Value: 1}},
					Options: options.Index().
			SetName("phone_number_1"),
		})
		if err != nil {
			logger.Error("failed to create phone_number index", zap.Error(err))
			return err
		}
	}

	// Create cpf index
	if _, exists := existingIndexes["cpf_1"]; !exists {
		logger.Info("creating cpf index", zap.String("collection", AppConfig.PhoneMappingCollection))
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "cpf", Value: 1}},
					Options: options.Index().
			SetName("cpf_1"),
		})
		if err != nil {
			logger.Error("failed to create cpf index", zap.Error(err))
			return err
		}
	}

	// Create status index
	if _, exists := existingIndexes["status_1"]; !exists {
		logger.Info("creating status index", zap.String("collection", AppConfig.PhoneMappingCollection))
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "status", Value: 1}},
					Options: options.Index().
			SetName("status_1"),
		})
		if err != nil {
			logger.Error("failed to create status index", zap.Error(err))
			return err
		}
	}

	// Create phone_number + status compound index
	if _, exists := existingIndexes["phone_number_1_status_1"]; !exists {
		logger.Info("creating phone_number + status compound index", zap.String("collection", AppConfig.PhoneMappingCollection))
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "phone_number", Value: 1},
				{Key: "status", Value: 1},
			},
					Options: options.Index().
			SetName("phone_number_1_status_1"),
		})
		if err != nil {
			logger.Error("failed to create phone_number + status compound index", zap.Error(err))
			return err
		}
	}

	// Create quarantine_until index for quarantine queries
	if _, exists := existingIndexes["quarantine_until_1"]; !exists {
		logger.Info("creating quarantine_until index", zap.String("collection", AppConfig.PhoneMappingCollection))
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "quarantine_until", Value: 1}},
					Options: options.Index().
			SetName("quarantine_until_1"),
		})
		if err != nil {
			logger.Error("failed to create quarantine_until index", zap.Error(err))
			return err
		}
	}

	// Create compound index for quarantine queries (quarantine_until + cpf)
	if _, exists := existingIndexes["quarantine_until_1_cpf_1"]; !exists {
		logger.Info("creating quarantine_until + cpf compound index", zap.String("collection", AppConfig.PhoneMappingCollection))
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "quarantine_until", Value: 1},
				{Key: "cpf", Value: 1},
			},
					Options: options.Index().
			SetName("quarantine_until_1_cpf_1"),
		})
		if err != nil {
			logger.Error("failed to create quarantine_until + cpf compound index", zap.Error(err))
			return err
		}
	}

	// Create created_at index for sorting
	if _, exists := existingIndexes["created_at_1"]; !exists {
		logger.Info("creating created_at index", zap.String("collection", AppConfig.PhoneMappingCollection))
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "created_at", Value: 1}},
					Options: options.Index().
			SetName("created_at_1"),
		})
		if err != nil {
			logger.Error("failed to create created_at index", zap.Error(err))
			return err
		}
	}

	// Create beta_group_id index for beta whitelist queries
	if _, exists := existingIndexes["beta_group_id_1"]; !exists {
		logger.Info("creating beta_group_id index", zap.String("collection", AppConfig.PhoneMappingCollection))
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{{Key: "beta_group_id", Value: 1}},
					Options: options.Index().
			SetName("beta_group_id_1"),
		})
		if err != nil {
			logger.Error("failed to create beta_group_id index", zap.Error(err))
			return err
		}
	}

	return nil
}

// ensureOptInHistoryIndex creates the required indexes for opt_in_history collection
func ensureOptInHistoryIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.OptInHistoryCollection)
	
	// Check if indexes already exist
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	existingIndexes := make(map[string]bool)
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok {
			existingIndexes[name] = true
		}
	}

	// Create indexes that don't exist
	indexesToCreate := []mongo.IndexModel{}

	// 1. Index on phone_number for phone-based queries
	if !existingIndexes["phone_number_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "phone_number", Value: 1}},
			Options: options.Index().
				SetName("phone_number_1"),
		})
	}

	// 2. Index on cpf for CPF-based queries
	if !existingIndexes["cpf_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "cpf", Value: 1}},
			Options: options.Index().
				SetName("cpf_1"),
		})
	}

	// 3. Index on action for filtering opt-in/opt-out
	if !existingIndexes["action_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "action", Value: 1}},
			Options: options.Index().
				SetName("action_1"),
		})
	}

	// 4. Index on channel for channel-based queries
	if !existingIndexes["channel_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "channel", Value: 1}},
			Options: options.Index().
				SetName("channel_1"),
		})
	}

	// 5. Index on timestamp for time-based queries
	if !existingIndexes["timestamp_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "timestamp", Value: -1}},
			Options: options.Index().
				SetName("timestamp_1"),
		})
	}

	// 6. Compound index on phone_number and timestamp for chronological history
	if !existingIndexes["phone_number_1_timestamp_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "phone_number", Value: 1}, {Key: "timestamp", Value: -1}},
			Options: options.Index().
				SetName("phone_number_1_timestamp_1"),
		})
	}

	// Create all missing indexes
	for _, indexModel := range indexesToCreate {
		_, err = collection.Indexes().CreateOne(ctx, indexModel)
		if err != nil {
			// Check if it's a duplicate key error (another instance created it)
			if mongo.IsDuplicateKeyError(err) {
				logger.Info("opt_in_history index already exists (created by another instance)", 
					zap.String("collection", AppConfig.OptInHistoryCollection))
				continue
			}
			logger.Error("failed to create opt_in_history index", 
				zap.String("collection", AppConfig.OptInHistoryCollection),
				zap.Error(err))
			return err
		}
	}

	if len(indexesToCreate) > 0 {
		logger.Info("created opt_in_history collection indexes", 
			zap.String("collection", AppConfig.OptInHistoryCollection),
			zap.Int("count", len(indexesToCreate)))
	} else {
		logger.Debug("opt_in_history collection indexes already exist", 
			zap.String("collection", AppConfig.OptInHistoryCollection))
	}
	
	return nil
}

// ensureBetaGroupIndex creates the indexes for beta_group collection
func ensureBetaGroupIndex(ctx context.Context, logger *zap.Logger) error {
	collection := MongoDB.Collection(AppConfig.BetaGroupCollection)
	
	// Check if indexes already exist
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		logger.Error("failed to list indexes", zap.Error(err))
		return err
	}
	defer cursor.Close(ctx)

	existingIndexes := make(map[string]bool)
	for cursor.Next(ctx) {
		var index bson.M
		if err := cursor.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok {
			existingIndexes[name] = true
		}
	}

	// Create indexes that don't exist
	indexesToCreate := []mongo.IndexModel{}

	// 1. Index on name for group name queries (case-insensitive)
	if !existingIndexes["name_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "name", Value: 1}},
			Options: options.Index().
				SetName("name_1").
				SetUnique(true),
		})
	}

	// 2. Index on created_at for time-based queries
	if !existingIndexes["created_at_1"] {
		indexesToCreate = append(indexesToCreate, mongo.IndexModel{
			Keys: bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().
				SetName("created_at_1"),
		})
	}

	// Create all missing indexes
	for _, indexModel := range indexesToCreate {
		_, err = collection.Indexes().CreateOne(ctx, indexModel)
		if err != nil {
			// Check if it's a duplicate key error (another instance created it)
			if mongo.IsDuplicateKeyError(err) {
				logger.Info("beta_group index already exists (created by another instance)", 
					zap.String("collection", AppConfig.BetaGroupCollection))
				continue
			}
			logger.Error("failed to create beta_group index", 
				zap.String("collection", AppConfig.BetaGroupCollection),
				zap.Error(err))
			return err
		}
	}

	if len(indexesToCreate) > 0 {
		logger.Info("created beta_group collection indexes", 
			zap.String("collection", AppConfig.BetaGroupCollection),
			zap.Int("count", len(indexesToCreate)))
	} else {
		logger.Debug("beta_group collection indexes already exist", 
			zap.String("collection", AppConfig.BetaGroupCollection))
	}
	
	return nil
}

// monitorConnectionPool monitors MongoDB connection pool health
func monitorConnectionPool() {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Get connection pool stats
			stats := MongoDB.Client().NumberSessionsInProgress()
			
			// Log connection pool status
			logging.Logger.Info("MongoDB connection pool status",
				zap.Int("sessions_in_progress", stats),
				zap.String("database", AppConfig.MongoDatabase))
			
			// Alert if too many connections are in use
			if stats > 100 {
				logging.Logger.Warn("High MongoDB connection usage detected",
					zap.Int("sessions_in_progress", stats),
					zap.String("database", AppConfig.MongoDatabase))
			}
			
			// Critical alert if approaching connection limit
			if stats > 400 { // 80% of maxPoolSize=500
				logging.Logger.Error("Critical MongoDB connection usage - approaching limit",
					zap.Int("sessions_in_progress", stats),
					zap.String("database", AppConfig.MongoDatabase),
					zap.String("recommendation", "Check for connection leaks or increase maxPoolSize"))
			}
			
			// Check if we're experiencing connection pool exhaustion
			if stats > 300 && stats > 0 {
				logging.Logger.Warn("MongoDB connection pool pressure detected",
					zap.Int("sessions_in_progress", stats),
					zap.String("database", AppConfig.MongoDatabase),
					zap.String("recommendation", "Consider increasing maxPoolSize or optimizing queries"))
			}
		}
	}
}

// monitorRedisConnectionPool monitors Redis connection pool health
func monitorRedisConnectionPool() {
	ticker := time.NewTicker(15 * time.Second) // Check every 15 seconds for Redis
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if Redis == nil {
				continue
			}

			// Get Redis pool stats
			poolStats := Redis.PoolStats()
			
			// Log Redis connection pool status
			logging.Logger.Info("Redis connection pool status",
				zap.Int("total_connections", int(poolStats.TotalConns)),
				zap.Int("idle_connections", int(poolStats.IdleConns)),
				zap.Int("stale_connections", int(poolStats.StaleConns)),
				zap.String("uri", AppConfig.RedisURI))
			
			// Alert if connection pool is getting full
			if poolStats.TotalConns > uint32(float64(AppConfig.RedisPoolSize)*0.8) { // 80% of max pool size
				logging.Logger.Warn("High Redis connection usage detected",
					zap.Int("total_connections", int(poolStats.TotalConns)),
					zap.Int("max_pool_size", AppConfig.RedisPoolSize),
					zap.Int("idle_connections", int(poolStats.IdleConns)),
					zap.String("uri", AppConfig.RedisURI))
			}
			
			// Alert if no idle connections available
			if poolStats.IdleConns == 0 {
				logging.Logger.Warn("No idle Redis connections available",
					zap.Int("total_connections", int(poolStats.TotalConns)),
					zap.String("uri", AppConfig.RedisURI))
			}
			
			// Critical alert if approaching connection limit
			if poolStats.TotalConns > uint32(float64(AppConfig.RedisPoolSize)*0.9) { // 90% of max pool size
				logging.Logger.Error("Critical Redis connection usage - approaching limit",
					zap.Int("total_connections", int(poolStats.TotalConns)),
					zap.Int("max_pool_size", AppConfig.RedisPoolSize),
					zap.String("uri", AppConfig.RedisURI),
					zap.String("recommendation", "Increase REDIS_POOL_SIZE or check for connection leaks"))
			}
		}
	}
} 