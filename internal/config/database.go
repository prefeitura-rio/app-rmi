package config

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/sony/gobreaker"
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
	Redis *redis.Client
)

// Circuit breaker for Redis
var redisBreaker = gobreaker.NewCircuitBreaker(gobreaker.Settings{
	Name:        "redis",
	MaxRequests: 3,
	Interval:    10 * time.Second,
	Timeout:     60 * time.Second,
	ReadyToTrip: func(counts gobreaker.Counts) bool {
		failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
		return counts.Requests >= 3 && failureRatio >= 0.6
	},
	OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
		observability.Logger.Warn("redis circuit breaker state changed",
			zap.String("from", from.String()),
			zap.String("to", to.String()),
		)
	},
})

// InitMongoDB initializes the MongoDB connection
func InitMongoDB() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Configure MongoDB with optimizations
	opts := options.Client().
		ApplyURI(AppConfig.MongoURI).
		SetMonitor(otelmongo.NewMonitor()). // Add OpenTelemetry instrumentation
		SetMaxPoolSize(100).                // Adjust based on your needs
		SetMinPoolSize(10).
		SetMaxConnIdleTime(5 * time.Minute).
		SetRetryWrites(true).
		SetRetryReads(true)

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

	// Create indexes
	createIndexes(ctx)

	observability.Logger.Info("Connected to MongoDB",
		zap.String("uri", maskMongoURI(AppConfig.MongoURI)),
		zap.String("database", AppConfig.MongoDatabase),
	)
}

// InitRedis initializes the Redis connection
func InitRedis() {
	Redis = redis.NewClient(&redis.Options{
		Addr:         AppConfig.RedisAddr,
		Password:     AppConfig.RedisPassword,
		DB:           AppConfig.RedisDB,
		PoolSize:     100,  // Adjust based on your needs
		MinIdleConns: 10,
		MaxRetries:   3,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})

	ctx := context.Background()
	_, err := redisBreaker.Execute(func() (interface{}, error) {
		return Redis.Ping(ctx).Result()
	})

	if err != nil {
		log.Fatal(err)
	}

	observability.Logger.Info("Connected to Redis",
		zap.String("addr", AppConfig.RedisAddr),
		zap.Int("db", AppConfig.RedisDB),
	)
}

// createIndexes creates MongoDB indexes
func createIndexes(ctx context.Context) {
	// Index for data_rmi collection
	_, err := MongoDB.Collection("data_rmi").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: map[string]interface{}{
			"cpf": 1,
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Index for self_declared collection
	_, err = MongoDB.Collection("self_declared").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: map[string]interface{}{
			"cpf": 1,
		},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		log.Fatal(err)
	}

	observability.Logger.Info("Created MongoDB indexes")
}

// maskMongoURI masks sensitive information in MongoDB URI
func maskMongoURI(uri string) string {
	// Implementation to mask username/password in URI
	return "mongodb://****:****@" + uri[strings.LastIndex(uri, "@")+1:]
} 