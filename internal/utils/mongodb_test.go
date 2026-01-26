package utils

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// setupMongoDBUtilsTest initializes MongoDB connection for tests
func setupMongoDBUtilsTest(t *testing.T) (*mongo.Database, *mongo.Collection, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping MongoDB integration tests: MONGODB_URI not set")
	}

	// Connect to MongoDB
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err, "Failed to connect to MongoDB")

	// Ping to verify connection
	err = client.Ping(ctx, nil)
	if err != nil {
		_ = client.Disconnect(ctx)
		t.Skipf("MongoDB not available or authentication failed: %v", err)
	}

	// Use a test database and collection
	db := client.Database("test_rmi_utils")
	collection := db.Collection("test_mongodb_utils")

	// Cleanup existing data
	_ = collection.Drop(ctx)

	// Return cleanup function
	cleanup := func() {
		_ = collection.Drop(ctx)
		_ = client.Disconnect(ctx)
	}

	return db, collection, cleanup
}

func TestFindOneWithTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		// Insert test document
		testDoc := bson.M{"_id": "test1", "name": "Test User", "age": 25}
		_, err := collection.InsertOne(ctx, testDoc)
		require.NoError(t, err)

		// Test FindOneWithTimeout
		var result bson.M
		err = FindOneWithTimeout(ctx, collection, bson.M{"_id": "test1"}, &result, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, "Test User", result["name"])
		assert.Equal(t, int32(25), result["age"])
	})

	t.Run("NotFound", func(t *testing.T) {
		var result bson.M
		err := FindOneWithTimeout(ctx, collection, bson.M{"_id": "nonexistent"}, &result, 5*time.Second)
		assert.Equal(t, mongo.ErrNoDocuments, err)
	})

	t.Run("WithTimeout", func(t *testing.T) {
		// Insert document
		testDoc := bson.M{"_id": "timeout_test", "data": "value"}
		_, _ = collection.InsertOne(ctx, testDoc)

		// Test with reasonable timeout
		var result bson.M
		err := FindOneWithTimeout(ctx, collection, bson.M{"_id": "timeout_test"}, &result, DefaultQueryTimeout)
		require.NoError(t, err)
		assert.Equal(t, "value", result["data"])
	})
}

func TestFindOneWithProjectionAndTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success - Include Fields", func(t *testing.T) {
		// Insert test document
		testDoc := bson.M{
			"_id":   "test2",
			"name":  "John Doe",
			"age":   30,
			"email": "john@example.com",
			"phone": "123456789",
		}
		_, err := collection.InsertOne(ctx, testDoc)
		require.NoError(t, err)

		// Test with projection (only name and age)
		var result bson.M
		projection := bson.M{"name": 1, "age": 1}
		err = FindOneWithProjectionAndTimeout(ctx, collection, bson.M{"_id": "test2"}, projection, &result, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, "John Doe", result["name"])
		assert.Equal(t, int32(30), result["age"])
		assert.Nil(t, result["email"], "Email should be excluded from projection")
		assert.Nil(t, result["phone"], "Phone should be excluded from projection")
	})

	t.Run("Success - Exclude Fields", func(t *testing.T) {
		// Insert test document
		testDoc := bson.M{
			"_id":      "test3",
			"name":     "Jane Doe",
			"password": "secret123",
			"salt":     "random",
		}
		_, _ = collection.InsertOne(ctx, testDoc)

		// Test with exclusion projection
		var result bson.M
		projection := bson.M{"password": 0, "salt": 0}
		err := FindOneWithProjectionAndTimeout(ctx, collection, bson.M{"_id": "test3"}, projection, &result, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, "Jane Doe", result["name"])
		assert.Nil(t, result["password"], "Password should be excluded")
		assert.Nil(t, result["salt"], "Salt should be excluded")
	})

	t.Run("NotFound", func(t *testing.T) {
		var result bson.M
		projection := bson.M{"name": 1}
		err := FindOneWithProjectionAndTimeout(ctx, collection, bson.M{"_id": "nonexistent"}, projection, &result, 5*time.Second)
		assert.Equal(t, mongo.ErrNoDocuments, err)
	})
}

func TestFindWithTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success - Multiple Documents", func(t *testing.T) {
		// Insert multiple documents
		docs := []interface{}{
			bson.M{"_id": "find1", "type": "A", "age": 25},
			bson.M{"_id": "find2", "type": "A", "age": 30},
			bson.M{"_id": "find3", "type": "B", "age": 35},
		}
		_, err := collection.InsertMany(ctx, docs)
		require.NoError(t, err)

		// Find all documents
		cursor, err := FindWithTimeout(ctx, collection, bson.M{}, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 3, len(results))
	})

	t.Run("Success - With Filter", func(t *testing.T) {
		// Find documents with filter
		cursor, err := FindWithTimeout(ctx, collection, bson.M{"type": "A"}, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 2, len(results))
		for _, doc := range results {
			assert.Equal(t, "A", doc["type"])
		}
	})

	t.Run("Success - Empty Result", func(t *testing.T) {
		cursor, err := FindWithTimeout(ctx, collection, bson.M{"type": "Z"}, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 0, len(results))
	})
}

func TestFindWithProjectionAndTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success - With Projection", func(t *testing.T) {
		// Insert test documents
		docs := []interface{}{
			bson.M{"_id": "proj1", "name": "User1", "email": "user1@test.com", "password": "secret1"},
			bson.M{"_id": "proj2", "name": "User2", "email": "user2@test.com", "password": "secret2"},
		}
		_, err := collection.InsertMany(ctx, docs)
		require.NoError(t, err)

		// Find with projection (exclude password)
		projection := bson.M{"password": 0}
		cursor, err := FindWithProjectionAndTimeout(ctx, collection, bson.M{}, projection, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 2, len(results))
		for _, doc := range results {
			assert.NotNil(t, doc["name"])
			assert.NotNil(t, doc["email"])
			assert.Nil(t, doc["password"], "Password should be excluded")
		}
	})

	t.Run("Success - Include Specific Fields", func(t *testing.T) {
		// Find with projection (only name)
		projection := bson.M{"name": 1}
		cursor, err := FindWithProjectionAndTimeout(ctx, collection, bson.M{}, projection, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 2, len(results))
		for _, doc := range results {
			assert.NotNil(t, doc["name"])
			assert.Nil(t, doc["email"], "Email should be excluded")
			assert.Nil(t, doc["password"], "Password should be excluded")
		}
	})
}

func TestFindWithLimitAndTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success - Limit Results", func(t *testing.T) {
		// Insert test documents
		docs := []interface{}{
			bson.M{"_id": "limit1", "order": 1},
			bson.M{"_id": "limit2", "order": 2},
			bson.M{"_id": "limit3", "order": 3},
			bson.M{"_id": "limit4", "order": 4},
		}
		_, err := collection.InsertMany(ctx, docs)
		require.NoError(t, err)

		// Find with limit
		cursor, err := FindWithLimitAndTimeout(ctx, collection, bson.M{}, 2, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 2, len(results))
	})

	t.Run("Success - Limit Greater Than Count", func(t *testing.T) {
		// Find with limit greater than available documents
		cursor, err := FindWithLimitAndTimeout(ctx, collection, bson.M{}, 100, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 4, len(results))
	})

	t.Run("Success - Limit Zero", func(t *testing.T) {
		// Limit of 0 in MongoDB means no limit
		cursor, err := FindWithLimitAndTimeout(ctx, collection, bson.M{}, 0, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 4, len(results))
	})
}

func TestUpdateOneWithTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success - Update Existing", func(t *testing.T) {
		// Insert test document
		testDoc := bson.M{"_id": "update1", "name": "Jane Doe", "age": 28}
		_, err := collection.InsertOne(ctx, testDoc)
		require.NoError(t, err)

		// Update document
		update := bson.M{"$set": bson.M{"age": 29, "city": "Rio de Janeiro"}}
		updateResult, err := UpdateOneWithTimeout(ctx, collection, bson.M{"_id": "update1"}, update, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, int64(1), updateResult.MatchedCount)
		assert.Equal(t, int64(1), updateResult.ModifiedCount)

		// Verify update
		var result bson.M
		err = FindOneWithTimeout(ctx, collection, bson.M{"_id": "update1"}, &result, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, int32(29), result["age"])
		assert.Equal(t, "Rio de Janeiro", result["city"])
	})

	t.Run("Success - No Match", func(t *testing.T) {
		update := bson.M{"$set": bson.M{"age": 30}}
		updateResult, err := UpdateOneWithTimeout(ctx, collection, bson.M{"_id": "nonexistent"}, update, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, int64(0), updateResult.MatchedCount)
		assert.Equal(t, int64(0), updateResult.ModifiedCount)
	})

	t.Run("Success - No Modification", func(t *testing.T) {
		// Update with same value
		update := bson.M{"$set": bson.M{"age": 29}}
		updateResult, err := UpdateOneWithTimeout(ctx, collection, bson.M{"_id": "update1"}, update, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, int64(1), updateResult.MatchedCount)
		assert.Equal(t, int64(0), updateResult.ModifiedCount, "Should not modify when value is the same")
	})
}

func TestUpsertOneWithTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success - Insert New", func(t *testing.T) {
		// Upsert non-existent document
		update := bson.M{"$set": bson.M{"name": "New User", "age": 22}}
		upsertResult, err := UpsertOneWithTimeout(ctx, collection, bson.M{"_id": "upsert1"}, update, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, int64(1), upsertResult.UpsertedCount)
		assert.NotNil(t, upsertResult.UpsertedID)

		// Verify document was created
		var result bson.M
		err = FindOneWithTimeout(ctx, collection, bson.M{"_id": "upsert1"}, &result, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, "New User", result["name"])
		assert.Equal(t, int32(22), result["age"])
	})

	t.Run("Success - Update Existing", func(t *testing.T) {
		// Upsert existing document
		update := bson.M{"$set": bson.M{"age": 23, "city": "São Paulo"}}
		upsertResult, err := UpsertOneWithTimeout(ctx, collection, bson.M{"_id": "upsert1"}, update, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, int64(1), upsertResult.MatchedCount)
		assert.Equal(t, int64(1), upsertResult.ModifiedCount)
		assert.Nil(t, upsertResult.UpsertedID, "Should not have UpsertedID when updating")

		// Verify update
		var result bson.M
		err = FindOneWithTimeout(ctx, collection, bson.M{"_id": "upsert1"}, &result, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, int32(23), result["age"])
		assert.Equal(t, "São Paulo", result["city"])
	})
}

func TestInsertOneWithTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		testDoc := bson.M{"_id": "insert1", "name": "Insert Test", "value": 100}
		insertResult, err := InsertOneWithTimeout(ctx, collection, testDoc, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, "insert1", insertResult.InsertedID)

		// Verify insertion
		var result bson.M
		err = FindOneWithTimeout(ctx, collection, bson.M{"_id": "insert1"}, &result, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, "Insert Test", result["name"])
		assert.Equal(t, int32(100), result["value"])
	})

	t.Run("Error - Duplicate Key", func(t *testing.T) {
		// Try to insert duplicate
		testDoc := bson.M{"_id": "insert1", "name": "Duplicate"}
		_, err := InsertOneWithTimeout(ctx, collection, testDoc, 5*time.Second)
		assert.Error(t, err)
		assert.True(t, mongo.IsDuplicateKeyError(err))
	})
}

func TestDeleteOneWithTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		// Insert document to delete
		testDoc := bson.M{"_id": "delete1", "name": "To Delete"}
		_, err := collection.InsertOne(ctx, testDoc)
		require.NoError(t, err)

		// Delete document
		deleteResult, err := DeleteOneWithTimeout(ctx, collection, bson.M{"_id": "delete1"}, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, int64(1), deleteResult.DeletedCount)

		// Verify deletion
		var result bson.M
		err = FindOneWithTimeout(ctx, collection, bson.M{"_id": "delete1"}, &result, 5*time.Second)
		assert.Equal(t, mongo.ErrNoDocuments, err)
	})

	t.Run("Success - No Match", func(t *testing.T) {
		deleteResult, err := DeleteOneWithTimeout(ctx, collection, bson.M{"_id": "nonexistent"}, 5*time.Second)
		require.NoError(t, err)

		assert.Equal(t, int64(0), deleteResult.DeletedCount)
	})
}

func TestCountDocumentsWithTimeout(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Success - All Documents", func(t *testing.T) {
		// Insert multiple documents
		docs := []interface{}{
			bson.M{"_id": "count1", "type": "A"},
			bson.M{"_id": "count2", "type": "A"},
			bson.M{"_id": "count3", "type": "B"},
		}
		_, err := collection.InsertMany(ctx, docs)
		require.NoError(t, err)

		// Count all documents
		count, err := CountDocumentsWithTimeout(ctx, collection, bson.M{}, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
	})

	t.Run("Success - With Filter", func(t *testing.T) {
		// Count with filter
		count, err := CountDocumentsWithTimeout(ctx, collection, bson.M{"type": "A"}, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, int64(2), count)
	})

	t.Run("Success - No Match", func(t *testing.T) {
		count, err := CountDocumentsWithTimeout(ctx, collection, bson.M{"type": "Z"}, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, int64(0), count)
	})
}

func TestDefaultQueryTimeout(t *testing.T) {
	// Verify the default timeout constant
	assert.Equal(t, 10*time.Second, DefaultQueryTimeout)
}

func TestTimeoutBehavior(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Reasonable Timeout Works", func(t *testing.T) {
		// Insert document
		testDoc := bson.M{"_id": "timeout_test", "data": "value"}
		_, err := collection.InsertOne(ctx, testDoc)
		require.NoError(t, err)

		// Test with reasonable timeout
		var result bson.M
		err = FindOneWithTimeout(ctx, collection, bson.M{"_id": "timeout_test"}, &result, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, "value", result["data"])
	})

	t.Run("DefaultQueryTimeout Works", func(t *testing.T) {
		// Test with default timeout
		var result bson.M
		err := FindOneWithTimeout(ctx, collection, bson.M{"_id": "timeout_test"}, &result, DefaultQueryTimeout)
		require.NoError(t, err)
		assert.Equal(t, "value", result["data"])
	})
}

func TestComplexQueries(t *testing.T) {
	_, collection, cleanup := setupMongoDBUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Complex Filter With Update", func(t *testing.T) {
		// Insert test documents
		docs := []interface{}{
			bson.M{"_id": "complex1", "category": "A", "status": "active", "score": 100},
			bson.M{"_id": "complex2", "category": "A", "status": "inactive", "score": 50},
			bson.M{"_id": "complex3", "category": "B", "status": "active", "score": 75},
		}
		_, err := collection.InsertMany(ctx, docs)
		require.NoError(t, err)

		// Update with complex filter
		filter := bson.M{"category": "A", "status": "active"}
		update := bson.M{"$inc": bson.M{"score": 10}}
		result, err := UpdateOneWithTimeout(ctx, collection, filter, update, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, int64(1), result.ModifiedCount)

		// Verify update
		var doc bson.M
		err = FindOneWithTimeout(ctx, collection, bson.M{"_id": "complex1"}, &doc, 5*time.Second)
		require.NoError(t, err)
		assert.Equal(t, int32(110), doc["score"])
	})

	t.Run("Complex Projection With Multiple Fields", func(t *testing.T) {
		// Find with complex projection
		projection := bson.M{"category": 1, "score": 1, "_id": 0}
		cursor, err := FindWithProjectionAndTimeout(ctx, collection, bson.M{"status": "active"}, projection, 5*time.Second)
		require.NoError(t, err)
		defer cursor.Close(ctx)

		var results []bson.M
		err = cursor.All(ctx, &results)
		require.NoError(t, err)

		assert.Equal(t, 2, len(results))
		for _, doc := range results {
			assert.NotNil(t, doc["category"])
			assert.NotNil(t, doc["score"])
			assert.Nil(t, doc["_id"], "_id should be excluded")
			assert.Nil(t, doc["status"], "status should be excluded")
		}
	})
}
