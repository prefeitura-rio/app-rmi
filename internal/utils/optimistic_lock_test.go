package utils

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// setupOptimisticLockTest initializes MongoDB for testing
func setupOptimisticLockTest(t *testing.T) func() {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping optimistic lock tests: MONGODB_URI not set")
	}

	logging.InitLogger()

	// Initialize config
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.SelfDeclaredCollection = "test_self_declared"
	config.AppConfig.UserConfigCollection = "test_user_config"

	// MongoDB setup
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	require.NoError(t, err, "Failed to connect to MongoDB")

	err = client.Ping(ctx, nil)
	require.NoError(t, err, "Failed to ping MongoDB")

	config.MongoDB = client.Database("rmi_test")

	return func() {
		// Clean up MongoDB
		config.MongoDB.Drop(ctx)
		client.Disconnect(ctx)
	}
}

// Test helper to create a test document
func createTestDocument(t *testing.T, ctx context.Context, collection string, cpf string, initialVersion int32) {
	doc := bson.M{
		"cpf":        cpf,
		"version":    initialVersion,
		"updated_at": time.Now(),
		"data":       "initial",
	}

	_, err := config.MongoDB.Collection(collection).InsertOne(ctx, doc)
	require.NoError(t, err, "Failed to insert test document")
}

func TestOptimisticLockError_Error(t *testing.T) {
	err := OptimisticLockError{
		Resource: "users",
		Message:  "version mismatch",
	}

	expected := "optimistic lock conflict for users: version mismatch"
	assert.Equal(t, expected, err.Error())
}

func TestOptimisticLockError_EmptyResource(t *testing.T) {
	err := OptimisticLockError{
		Resource: "",
		Message:  "test message",
	}

	assert.NotEmpty(t, err.Error())
}

func TestVersionedDocument(t *testing.T) {
	doc := VersionedDocument{
		Version:   1,
		UpdatedAt: time.Now(),
	}

	assert.Equal(t, int32(1), doc.Version)
	assert.False(t, doc.UpdatedAt.IsZero())
}

func TestOptimisticUpdateResult(t *testing.T) {
	result := OptimisticUpdateResult{
		ModifiedCount: 1,
		Version:       2,
		UpdatedAt:     time.Now(),
	}

	assert.Equal(t, int64(1), result.ModifiedCount)
	assert.Equal(t, int32(2), result.Version)
	assert.False(t, result.UpdatedAt.IsZero())
}

func TestUpdateWithOptimisticLock_SuccessfulUpdate(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"
	cpf := "12345678901"

	// Create initial document with version 1
	createTestDocument(t, ctx, collection, cpf, 1)

	// Update with correct version
	filter := bson.M{"cpf": cpf}
	update := bson.M{
		"$set": bson.M{
			"data": "updated",
		},
	}

	result, err := UpdateWithOptimisticLock(ctx, collection, filter, update, 1)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.ModifiedCount)
	assert.Equal(t, int32(2), result.Version) // Version should be incremented
	assert.False(t, result.UpdatedAt.IsZero())

	// Verify document was updated (use fresh filter since original was modified)
	var doc bson.M
	err = config.MongoDB.Collection(collection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&doc)
	require.NoError(t, err)
	assert.Equal(t, int32(2), doc["version"].(int32))
	assert.Equal(t, "updated", doc["data"].(string))
}

func TestUpdateWithOptimisticLock_VersionConflict(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"
	cpf := "12345678902"

	// Create initial document with version 2
	createTestDocument(t, ctx, collection, cpf, 2)

	// Try to update with wrong version (1)
	filter := bson.M{"cpf": cpf}
	update := bson.M{
		"$set": bson.M{
			"data": "should fail",
		},
	}

	result, err := UpdateWithOptimisticLock(ctx, collection, filter, update, 1)

	require.Error(t, err)
	assert.Nil(t, result)

	// Verify it's an OptimisticLockError
	var lockErr OptimisticLockError
	assert.True(t, errors.As(err, &lockErr))
	assert.Contains(t, lockErr.Error(), "expected version 1, but document has version 2")

	// Verify document was not modified (use fresh filter since original was modified)
	var doc bson.M
	err = config.MongoDB.Collection(collection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&doc)
	require.NoError(t, err)
	assert.Equal(t, int32(2), doc["version"].(int32))
	assert.Equal(t, "initial", doc["data"].(string))
}

func TestUpdateWithOptimisticLock_DocumentNotFound(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"
	cpf := "99999999999" // Non-existent document

	filter := bson.M{"cpf": cpf}
	update := bson.M{
		"$set": bson.M{
			"data": "new data",
		},
	}

	result, err := UpdateWithOptimisticLock(ctx, collection, filter, update, 1)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "document not found")
}

func TestUpdateWithOptimisticLock_ConcurrentUpdates(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"
	cpf := "12345678903"

	// Create initial document with version 1
	createTestDocument(t, ctx, collection, cpf, 1)

	// Simulate two concurrent updates
	var wg sync.WaitGroup
	results := make([]error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			filter := bson.M{"cpf": cpf}
			update := bson.M{
				"$set": bson.M{
					"data": "concurrent update",
				},
			}

			// Both try to update from version 1
			_, err := UpdateWithOptimisticLock(ctx, collection, filter, update, 1)
			results[idx] = err
		}(i)
	}

	wg.Wait()

	// One should succeed, one should fail
	successCount := 0
	failCount := 0

	for _, err := range results {
		if err == nil {
			successCount++
		} else {
			failCount++
			var lockErr OptimisticLockError
			assert.True(t, errors.As(err, &lockErr), "Error should be OptimisticLockError")
		}
	}

	assert.Equal(t, 1, successCount, "Exactly one update should succeed")
	assert.Equal(t, 1, failCount, "Exactly one update should fail")

	// Verify final version is 2
	var doc bson.M
	err := config.MongoDB.Collection(collection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&doc)
	require.NoError(t, err)
	assert.Equal(t, int32(2), doc["version"].(int32))
}

func TestUpdateWithOptimisticLock_UpdateWithoutSetOperation(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"
	cpf := "12345678904"

	// Create initial document with version 1
	createTestDocument(t, ctx, collection, cpf, 1)

	// Update without $set (should be added automatically)
	filter := bson.M{"cpf": cpf}
	update := bson.M{
		"$inc": bson.M{
			"counter": 1,
		},
	}

	result, err := UpdateWithOptimisticLock(ctx, collection, filter, update, 1)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int32(2), result.Version)

	// Verify version and updated_at were added (use fresh filter since original was modified)
	var doc bson.M
	err = config.MongoDB.Collection(collection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&doc)
	require.NoError(t, err)
	assert.Equal(t, int32(2), doc["version"].(int32))
	assert.NotNil(t, doc["updated_at"])
}

func TestGetDocumentVersion_Success(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"
	cpf := "12345678905"

	// Create document with version 5
	createTestDocument(t, ctx, collection, cpf, 5)

	// Get version
	filter := bson.M{"cpf": cpf}
	version, err := GetDocumentVersion(ctx, collection, filter)

	require.NoError(t, err)
	assert.Equal(t, int32(5), version)
}

func TestGetDocumentVersion_NotFound(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"

	// Try to get version of non-existent document
	filter := bson.M{"cpf": "99999999999"}
	version, err := GetDocumentVersion(ctx, collection, filter)

	require.Error(t, err)
	assert.Equal(t, int32(0), version)
	assert.Equal(t, mongo.ErrNoDocuments, err)
}

func TestGetDocumentVersion_NoVersionField(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"
	cpf := "12345678906"

	// Create document without version field
	doc := bson.M{
		"cpf":  cpf,
		"data": "no version",
	}
	_, err := config.MongoDB.Collection(collection).InsertOne(ctx, doc)
	require.NoError(t, err)

	// Get version should fail
	filter := bson.M{"cpf": cpf}
	version, err := GetDocumentVersion(ctx, collection, filter)

	require.Error(t, err)
	assert.Equal(t, int32(0), version)
	assert.Contains(t, err.Error(), "version field not found or invalid type")
}

func TestUpdateSelfDeclaredWithOptimisticLock(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	cpf := "12345678907"

	// Create initial document
	createTestDocument(t, ctx, config.AppConfig.SelfDeclaredCollection, cpf, 1)

	// Update using helper function
	update := bson.M{
		"$set": bson.M{
			"email": "test@example.com",
		},
	}

	result, err := UpdateSelfDeclaredWithOptimisticLock(ctx, cpf, update, 1)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int32(2), result.Version)
}

func TestUpdateUserConfigWithOptimisticLock(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	cpf := "12345678908"

	// Create initial document
	createTestDocument(t, ctx, config.AppConfig.UserConfigCollection, cpf, 1)

	// Update using helper function
	update := bson.M{
		"$set": bson.M{
			"theme": "dark",
		},
	}

	result, err := UpdateUserConfigWithOptimisticLock(ctx, cpf, update, 1)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int32(2), result.Version)
}

func TestGetSelfDeclaredVersion(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	cpf := "12345678909"

	// Create document with version 3
	createTestDocument(t, ctx, config.AppConfig.SelfDeclaredCollection, cpf, 3)

	// Get version using helper function
	version, err := GetSelfDeclaredVersion(ctx, cpf)

	require.NoError(t, err)
	assert.Equal(t, int32(3), version)
}

func TestGetUserConfigVersion(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	cpf := "12345678910"

	// Create document with version 7
	createTestDocument(t, ctx, config.AppConfig.UserConfigCollection, cpf, 7)

	// Get version using helper function
	version, err := GetUserConfigVersion(ctx, cpf)

	require.NoError(t, err)
	assert.Equal(t, int32(7), version)
}

func TestInitializeDocumentVersion(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"

	// Create documents without version field
	docs := []interface{}{
		bson.M{"cpf": "11111111111", "data": "doc1"},
		bson.M{"cpf": "22222222222", "data": "doc2"},
		bson.M{"cpf": "33333333333", "data": "doc3", "version": 5}, // Already has version
	}

	_, err := config.MongoDB.Collection(collection).InsertMany(ctx, docs)
	require.NoError(t, err)

	// Initialize versions
	err = InitializeDocumentVersion(ctx, collection, bson.M{})
	require.NoError(t, err)

	// Verify all documents have version field
	cursor, err := config.MongoDB.Collection(collection).Find(ctx, bson.M{})
	require.NoError(t, err)
	defer cursor.Close(ctx)

	count := 0
	for cursor.Next(ctx) {
		var doc bson.M
		err := cursor.Decode(&doc)
		require.NoError(t, err)

		version, ok := doc["version"].(int32)
		require.True(t, ok, "Document should have version field")

		// Documents without version should now have version 1
		// Document that already had version 5 should keep it
		if doc["cpf"].(string) == "33333333333" {
			assert.Equal(t, int32(5), version)
		} else {
			assert.Equal(t, int32(1), version)
		}

		count++
	}

	assert.Equal(t, 3, count, "Should have 3 documents")
}

func TestInitializeDocumentVersion_WithFilter(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"

	// Create documents with different types
	docs := []interface{}{
		bson.M{"cpf": "11111111111", "type": "A"},
		bson.M{"cpf": "22222222222", "type": "B"},
		bson.M{"cpf": "33333333333", "type": "A"},
	}

	_, err := config.MongoDB.Collection(collection).InsertMany(ctx, docs)
	require.NoError(t, err)

	// Initialize only type A documents
	err = InitializeDocumentVersion(ctx, collection, bson.M{"type": "A"})
	require.NoError(t, err)

	// Verify only type A documents have version
	var docA bson.M
	err = config.MongoDB.Collection(collection).FindOne(ctx, bson.M{"cpf": "11111111111"}).Decode(&docA)
	require.NoError(t, err)
	assert.Equal(t, int32(1), docA["version"].(int32))

	var docB bson.M
	err = config.MongoDB.Collection(collection).FindOne(ctx, bson.M{"cpf": "22222222222"}).Decode(&docB)
	require.NoError(t, err)
	_, hasVersion := docB["version"]
	assert.False(t, hasVersion, "Type B document should not have version")
}

func TestRetryWithOptimisticLock_Success(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		return nil
	}

	err := RetryWithOptimisticLock(ctx, 3, operation)

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestRetryWithOptimisticLock_NonOptimisticError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("database error")
	callCount := 0

	operation := func() error {
		callCount++
		return expectedErr
	}

	err := RetryWithOptimisticLock(ctx, 3, operation)

	assert.Equal(t, expectedErr, err)
	assert.Equal(t, 1, callCount, "Should not retry on non-optimistic lock errors")
}

func TestRetryWithOptimisticLock_OptimisticErrorRetry(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		if callCount < 3 {
			return OptimisticLockError{Resource: "test", Message: "conflict"}
		}
		return nil
	}

	err := RetryWithOptimisticLock(ctx, 5, operation)

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount)
}

func TestRetryWithOptimisticLock_MaxRetriesExceeded(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		return OptimisticLockError{Resource: "test", Message: "conflict"}
	}

	err := RetryWithOptimisticLock(ctx, 2, operation)

	assert.Error(t, err)
	assert.Equal(t, 3, callCount, "Should call maxRetries + 1 times (initial attempt + retries)")

	var lockErr OptimisticLockError
	assert.True(t, errors.As(err, &lockErr))
}

func TestRetryWithOptimisticLock_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	operation := func() error {
		callCount++
		if callCount == 2 {
			cancel()
		}
		return OptimisticLockError{Resource: "test", Message: "conflict"}
	}

	err := RetryWithOptimisticLock(ctx, 10, operation)

	assert.Error(t, err)
	assert.LessOrEqual(t, callCount, 5, "Should stop retrying when context is canceled")
}

func TestRetryWithOptimisticLock_ExponentialBackoff(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	callTimes := []time.Time{}

	operation := func() error {
		callCount++
		callTimes = append(callTimes, time.Now())
		if callCount < 4 {
			return OptimisticLockError{Resource: "test", Message: "conflict"}
		}
		return nil
	}

	start := time.Now()
	err := RetryWithOptimisticLock(ctx, 5, operation)

	assert.NoError(t, err)

	// Check that total duration is reasonable (exponential backoff: 100ms, 200ms, 400ms)
	duration := time.Since(start)
	expectedMin := 700 * time.Millisecond // 100 + 200 + 400
	expectedMax := 1000 * time.Millisecond

	assert.GreaterOrEqual(t, duration, expectedMin, "Duration should respect exponential backoff")
	if duration > expectedMax {
		t.Logf("Duration %v exceeded expected max %v (acceptable, system may be busy)", duration, expectedMax)
	}
}

func TestRetryWithOptimisticLock_ZeroRetries(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		return OptimisticLockError{Resource: "test", Message: "conflict"}
	}

	err := RetryWithOptimisticLock(ctx, 0, operation)

	assert.Error(t, err)
	assert.Equal(t, 1, callCount, "With 0 retries should call exactly once")
}

func TestRetryWithOptimisticLock_AlternatingErrors(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	operation := func() error {
		callCount++
		if callCount%2 == 0 {
			return errors.New("non-optimistic error")
		}
		return OptimisticLockError{Resource: "test", Message: "conflict"}
	}

	err := RetryWithOptimisticLock(ctx, 5, operation)

	assert.Error(t, err)
	assert.Equal(t, 2, callCount, "Should stop on non-optimistic error")
}

func TestRetryWithOptimisticLock_RealWorldScenario(t *testing.T) {
	cleanup := setupOptimisticLockTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := "test_self_declared"
	cpf := "12345678999"

	// Create initial document
	createTestDocument(t, ctx, collection, cpf, 1)

	attempt := 0
	err := RetryWithOptimisticLock(ctx, 5, func() error {
		attempt++

		// Get current version
		version, err := GetDocumentVersion(ctx, collection, bson.M{"cpf": cpf})
		if err != nil {
			return err
		}

		// Try to update with optimistic lock
		filter := bson.M{"cpf": cpf}
		update := bson.M{
			"$set": bson.M{
				"data": "retry attempt",
			},
		}

		_, err = UpdateWithOptimisticLock(ctx, collection, filter, update, version)
		return err
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, attempt, "Should succeed on first attempt in non-concurrent scenario")

	// Verify final state
	var doc bson.M
	err = config.MongoDB.Collection(collection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&doc)
	require.NoError(t, err)
	assert.Equal(t, int32(2), doc["version"].(int32))
	assert.Equal(t, "retry attempt", doc["data"].(string))
}

func TestOptimisticLockError_IsError(t *testing.T) {
	var err error = OptimisticLockError{
		Resource: "test",
		Message:  "conflict",
	}

	assert.NotNil(t, err)

	var lockErr OptimisticLockError
	assert.True(t, errors.As(err, &lockErr))
}

func TestVersionedDocument_ZeroValues(t *testing.T) {
	doc := VersionedDocument{}

	assert.Equal(t, int32(0), doc.Version)
	assert.True(t, doc.UpdatedAt.IsZero())
}

func TestOptimisticUpdateResult_ZeroValues(t *testing.T) {
	result := OptimisticUpdateResult{}

	assert.Equal(t, int64(0), result.ModifiedCount)
	assert.Equal(t, int32(0), result.Version)
	assert.True(t, result.UpdatedAt.IsZero())
}
