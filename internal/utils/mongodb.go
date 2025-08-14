package utils

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DefaultQueryTimeout is the default timeout for MongoDB queries
const DefaultQueryTimeout = 10 * time.Second

// FindOneWithTimeout performs a MongoDB FindOne operation with timeout
func FindOneWithTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, result interface{}, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return collection.FindOne(ctx, filter).Decode(result)
}

// FindOneWithProjectionAndTimeout performs a MongoDB FindOne operation with projection and timeout
func FindOneWithProjectionAndTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, projection bson.M, result interface{}, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := options.FindOne().SetProjection(projection)
	return collection.FindOne(ctx, filter, opts).Decode(result)
}

// FindWithTimeout performs a MongoDB Find operation with timeout
func FindWithTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, timeout time.Duration) (*mongo.Cursor, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return collection.Find(ctx, filter)
}

// FindWithProjectionAndTimeout performs a MongoDB Find operation with projection and timeout
func FindWithProjectionAndTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, projection bson.M, timeout time.Duration) (*mongo.Cursor, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := options.Find().SetProjection(projection)
	return collection.Find(ctx, filter, opts)
}

// FindWithLimitAndTimeout performs a MongoDB Find operation with limit and timeout
func FindWithLimitAndTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, limit int64, timeout time.Duration) (*mongo.Cursor, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := options.Find().SetLimit(limit)
	return collection.Find(ctx, filter, opts)
}

// UpdateOneWithTimeout performs a MongoDB UpdateOne operation with timeout
func UpdateOneWithTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, update bson.M, timeout time.Duration) (*mongo.UpdateResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return collection.UpdateOne(ctx, filter, update)
}

// UpsertOneWithTimeout performs a MongoDB Upsert operation with timeout
func UpsertOneWithTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, update bson.M, timeout time.Duration) (*mongo.UpdateResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := options.Update().SetUpsert(true)
	return collection.UpdateOne(ctx, filter, update, opts)
}

// InsertOneWithTimeout performs a MongoDB InsertOne operation with timeout
func InsertOneWithTimeout(ctx context.Context, collection *mongo.Collection, document interface{}, timeout time.Duration) (*mongo.InsertOneResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return collection.InsertOne(ctx, document)
}

// DeleteOneWithTimeout performs a MongoDB DeleteOne operation with timeout
func DeleteOneWithTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, timeout time.Duration) (*mongo.DeleteResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return collection.DeleteOne(ctx, filter)
}

// CountDocumentsWithTimeout performs a MongoDB CountDocuments operation with timeout
func CountDocumentsWithTimeout(ctx context.Context, collection *mongo.Collection, filter bson.M, timeout time.Duration) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return collection.CountDocuments(ctx, filter)
}
