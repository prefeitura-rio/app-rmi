package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
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
)

// setupDatabaseUtilsTest initializes MongoDB for database utility testing
func setupDatabaseUtilsTest(t *testing.T) (*mongo.Collection, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping database utils tests: MONGODB_URI not set")
	}

	_ = logging.InitLogger()

	// Initialize config
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.CitizenCollection = "test_db_utils_citizens"
	config.AppConfig.SelfDeclaredCollection = "test_db_utils_self_declared"

	// MongoDB setup
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		_ = client.Disconnect(ctx)
		t.Skipf("MongoDB not available or authentication failed: %v", err)
	}

	config.MongoDB = client.Database("rmi_test_db_utils")
	testCollection := config.MongoDB.Collection("test_operations")

	// Clean up existing data
	_ = testCollection.Drop(ctx)

	return testCollection, func() {
		// Clean up MongoDB
		_ = config.MongoDB.Drop(ctx)
		_ = client.Disconnect(ctx)
	}
}

// TestExecuteWithTransaction tests transaction execution with success scenario
func TestExecuteWithTransaction_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	// Track which operations were executed
	var executed []int
	var mu sync.Mutex

	operations := []DatabaseOperation{
		{
			Operation: func() error {
				mu.Lock()
				executed = append(executed, 1)
				mu.Unlock()
				_, err := testCollection.InsertOne(ctx, bson.M{"_id": "doc1", "value": 100})
				return err
			},
			Rollback: func() error {
				_, err := testCollection.DeleteOne(ctx, bson.M{"_id": "doc1"})
				return err
			},
		},
		{
			Operation: func() error {
				mu.Lock()
				executed = append(executed, 2)
				mu.Unlock()
				_, err := testCollection.InsertOne(ctx, bson.M{"_id": "doc2", "value": 200})
				return err
			},
			Rollback: func() error {
				_, err := testCollection.DeleteOne(ctx, bson.M{"_id": "doc2"})
				return err
			},
		},
		{
			Operation: func() error {
				mu.Lock()
				executed = append(executed, 3)
				mu.Unlock()
				_, err := testCollection.InsertOne(ctx, bson.M{"_id": "doc3", "value": 300})
				return err
			},
			Rollback: func() error {
				_, err := testCollection.DeleteOne(ctx, bson.M{"_id": "doc3"})
				return err
			},
		},
	}

	err := ExecuteWithTransaction(ctx, operations)
	require.NoError(t, err, "Transaction should succeed")

	// Verify all operations were executed
	mu.Lock()
	assert.Equal(t, []int{1, 2, 3}, executed, "All operations should have been executed")
	mu.Unlock()

	// Verify documents were created
	count, err := testCollection.CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), count, "All 3 documents should exist")
}

// TestExecuteWithTransaction_RollbackOnError tests that rollback occurs when an operation fails
func TestExecuteWithTransaction_RollbackOnError(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	// Track operations and rollbacks
	var executed []int
	var rolledBack []int
	var mu sync.Mutex

	operations := []DatabaseOperation{
		{
			Operation: func() error {
				mu.Lock()
				executed = append(executed, 1)
				mu.Unlock()
				_, err := testCollection.InsertOne(ctx, bson.M{"_id": "doc1", "value": 100})
				return err
			},
			Rollback: func() error {
				mu.Lock()
				rolledBack = append(rolledBack, 1)
				mu.Unlock()
				_, err := testCollection.DeleteOne(ctx, bson.M{"_id": "doc1"})
				return err
			},
		},
		{
			Operation: func() error {
				mu.Lock()
				executed = append(executed, 2)
				mu.Unlock()
				_, err := testCollection.InsertOne(ctx, bson.M{"_id": "doc2", "value": 200})
				return err
			},
			Rollback: func() error {
				mu.Lock()
				rolledBack = append(rolledBack, 2)
				mu.Unlock()
				_, err := testCollection.DeleteOne(ctx, bson.M{"_id": "doc2"})
				return err
			},
		},
		{
			Operation: func() error {
				mu.Lock()
				executed = append(executed, 3)
				mu.Unlock()
				// This operation fails
				return errors.New("simulated error")
			},
			Rollback: func() error {
				mu.Lock()
				rolledBack = append(rolledBack, 3)
				mu.Unlock()
				return nil
			},
		},
	}

	err := ExecuteWithTransaction(ctx, operations)
	require.Error(t, err, "Transaction should fail")
	assert.Contains(t, err.Error(), "simulated error")

	// Verify operations were executed up to the failure
	mu.Lock()
	assert.Equal(t, []int{1, 2, 3}, executed, "Operations should have been executed until failure")

	// Verify rollbacks occurred in reverse order (operations 2 and 1)
	assert.ElementsMatch(t, []int{2, 1}, rolledBack, "Previous operations should have been rolled back")
	mu.Unlock()

	// Verify no documents remain (rollback successful)
	count, err := testCollection.CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "All documents should have been rolled back")
}

// TestExecuteWithTransaction_EmptyOperations tests transaction with no operations
func TestExecuteWithTransaction_EmptyOperations(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	err := ExecuteWithTransaction(ctx, []DatabaseOperation{})
	require.NoError(t, err, "Empty transaction should succeed")
}

// TestExecuteWithTransaction_RollbackError tests behavior when rollback itself fails
func TestExecuteWithTransaction_RollbackError(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	operations := []DatabaseOperation{
		{
			Operation: func() error {
				_, err := testCollection.InsertOne(ctx, bson.M{"_id": "doc1", "value": 100})
				return err
			},
			Rollback: func() error {
				// Rollback fails
				return errors.New("rollback failed")
			},
		},
		{
			Operation: func() error {
				// This operation fails, triggering rollback
				return errors.New("operation failed")
			},
			Rollback: func() error {
				return nil
			},
		},
	}

	err := ExecuteWithTransaction(ctx, operations)
	require.Error(t, err, "Transaction should fail")
	assert.Contains(t, err.Error(), "operation failed")
	// Note: Rollback errors are logged but don't prevent the main error from being returned
}

// TestExecuteWriteOperation_Success tests successful write operation
func TestExecuteWriteOperation_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteWriteOperation(ctx, collectionName, "insert", func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "test1", "name": "Test Document", "value": 42})
		return err
	})

	require.NoError(t, err, "Write operation should succeed")

	// Verify document was created
	var result bson.M
	err = testCollection.FindOne(ctx, bson.M{"_id": "test1"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "Test Document", result["name"])
	assert.Equal(t, int32(42), result["value"])
}

// TestExecuteWriteOperation_Update tests update operation
func TestExecuteWriteOperation_Update(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Insert initial document
	_, err := testCollection.InsertOne(ctx, bson.M{"_id": "test2", "counter": 0})
	require.NoError(t, err)

	// Update document
	err = ExecuteWriteOperation(ctx, collectionName, "update", func(coll *mongo.Collection) error {
		_, err := coll.UpdateOne(ctx, bson.M{"_id": "test2"}, bson.M{"$inc": bson.M{"counter": 1}})
		return err
	})

	require.NoError(t, err, "Update operation should succeed")

	// Verify update
	var result bson.M
	err = testCollection.FindOne(ctx, bson.M{"_id": "test2"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, int32(1), result["counter"])
}

// TestExecuteWriteOperation_Delete tests delete operation
func TestExecuteWriteOperation_Delete(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Insert document to delete
	_, err := testCollection.InsertOne(ctx, bson.M{"_id": "test3", "to_delete": true})
	require.NoError(t, err)

	// Delete document
	err = ExecuteWriteOperation(ctx, collectionName, "delete", func(coll *mongo.Collection) error {
		_, err := coll.DeleteOne(ctx, bson.M{"_id": "test3"})
		return err
	})

	require.NoError(t, err, "Delete operation should succeed")

	// Verify deletion
	count, err := testCollection.CountDocuments(ctx, bson.M{"_id": "test3"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "Document should be deleted")
}

// TestExecuteWriteOperation_Error tests error handling
func TestExecuteWriteOperation_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Insert document to create duplicate key error
	_, err := testCollection.InsertOne(ctx, bson.M{"_id": "duplicate", "value": 1})
	require.NoError(t, err)

	// Try to insert duplicate
	err = ExecuteWriteOperation(ctx, collectionName, "insert", func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "duplicate", "value": 2})
		return err
	})

	require.Error(t, err, "Write operation should fail on duplicate key")
	assert.Contains(t, err.Error(), "write operation failed")
}

// TestExecuteWriteOperation_Concurrent tests concurrent write operations
func TestExecuteWriteOperation_Concurrent(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Number of concurrent operations
	const numOps = 10
	var wg sync.WaitGroup
	errChan := make(chan error, numOps)

	// Execute concurrent writes
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := ExecuteWriteOperation(ctx, collectionName, "insert", func(coll *mongo.Collection) error {
				_, err := coll.InsertOne(ctx, bson.M{"_id": id, "value": id * 100})
				return err
			})
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		t.Errorf("Concurrent operation failed: %v", err)
	}

	// Verify all documents were created
	count, err := testCollection.CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(numOps), count, "All concurrent operations should succeed")
}

// TestExecuteWriteOperation_BulkWrite tests bulk write operations
func TestExecuteWriteOperation_BulkWrite(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Create bulk write models
	models := []mongo.WriteModel{
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "bulk1", "type": "A"}),
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "bulk2", "type": "B"}),
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "bulk3", "type": "A"}),
	}

	err := ExecuteWriteOperation(ctx, collectionName, "bulk_insert", func(coll *mongo.Collection) error {
		_, err := coll.BulkWrite(ctx, models)
		return err
	})

	require.NoError(t, err, "Bulk write operation should succeed")

	// Verify documents were created
	count, err := testCollection.CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), count, "All bulk documents should be inserted")
}

// TestExecuteWriteOperation_Timeout tests operation with context timeout
func TestExecuteWriteOperation_Timeout(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	collectionName := testCollection.Name()

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	err := ExecuteWriteOperation(ctx, collectionName, "insert", func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "timeout_test", "value": 1})
		return err
	})

	// The operation should fail due to context timeout
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	}
}

func TestGetWriteConcernForOperation(t *testing.T) {
	tests := []struct {
		name          string
		operationType string
		wantW         interface{}
	}{
		{
			name:          "audit operation",
			operationType: "audit",
			wantW:         0,
		},
		{
			name:          "user_data operation",
			operationType: "user_data",
			wantW:         1,
		},
		{
			name:          "critical operation",
			operationType: "critical",
			wantW:         "majority",
		},
		{
			name:          "default operation",
			operationType: "unknown",
			wantW:         1,
		},
		{
			name:          "empty operation type",
			operationType: "",
			wantW:         1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wc := GetWriteConcernForOperation(tt.operationType)

			if wc == nil {
				t.Fatal("GetWriteConcernForOperation() returned nil")
			}

			if tt.wantW == "majority" {
				if wc.W != tt.wantW {
					t.Errorf("GetWriteConcernForOperation() W = %v, want %v", wc.W, tt.wantW)
				}
			} else {
				if wc.W != tt.wantW {
					t.Errorf("GetWriteConcernForOperation() W = %v, want %v", wc.W, tt.wantW)
				}
			}
		})
	}
}

func TestGetUpdateOptionsWithWriteConcern(t *testing.T) {
	tests := []struct {
		name          string
		operationType string
		upsert        bool
	}{
		{
			name:          "audit with upsert",
			operationType: "audit",
			upsert:        true,
		},
		{
			name:          "user_data without upsert",
			operationType: "user_data",
			upsert:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetUpdateOptionsWithWriteConcern(tt.operationType, tt.upsert)

			if opts == nil {
				t.Fatal("GetUpdateOptionsWithWriteConcern() returned nil")
			}

			if *opts.Upsert != tt.upsert {
				t.Errorf("GetUpdateOptionsWithWriteConcern() Upsert = %v, want %v", *opts.Upsert, tt.upsert)
			}
		})
	}
}

func TestGetInsertOptionsWithWriteConcern(t *testing.T) {
	tests := []string{"audit", "user_data", "critical", "default"}

	for _, operationType := range tests {
		t.Run(operationType, func(t *testing.T) {
			opts := GetInsertOptionsWithWriteConcern(operationType)

			if opts == nil {
				t.Fatal("GetInsertOptionsWithWriteConcern() returned nil")
			}
		})
	}
}

func TestCreateBulkUpdateModels(t *testing.T) {
	updates := []BulkUpdateRequest{
		{
			Filter: bson.M{"_id": 1},
			Update: bson.M{"$set": bson.M{"name": "Test"}},
			Upsert: true,
		},
		{
			Filter: bson.M{"_id": 2},
			Update: bson.M{"$set": bson.M{"name": "Test2"}},
			Upsert: false,
		},
	}

	models := CreateBulkUpdateModels(updates)

	if len(models) != len(updates) {
		t.Errorf("CreateBulkUpdateModels() len = %d, want %d", len(models), len(updates))
	}

	for i, model := range models {
		if model == nil {
			t.Errorf("CreateBulkUpdateModels() model[%d] is nil", i)
		}
	}
}

func TestCreateOptimizedBulkUpdateModels(t *testing.T) {
	updates := []BulkUpdateRequest{
		{
			Filter: bson.M{"_id": 1},
			Update: bson.M{"$set": bson.M{"name": "Test"}},
			Upsert: true,
		},
		{
			Filter: bson.M{"_id": 2},
			Update: bson.M{"$set": bson.M{"name": "Test2"}},
			Upsert: false,
		},
		{
			Filter: bson.M{"_id": 3},
			Update: bson.M{"$set": bson.M{"name": "Test3"}},
			Upsert: true,
		},
	}

	models := CreateOptimizedBulkUpdateModels(updates)

	if len(models) != len(updates) {
		t.Errorf("CreateOptimizedBulkUpdateModels() len = %d, want %d", len(models), len(updates))
	}

	for i, model := range models {
		if model == nil {
			t.Errorf("CreateOptimizedBulkUpdateModels() model[%d] is nil", i)
		}
	}
}

func TestCreateBulkUpdateModels_Empty(t *testing.T) {
	updates := []BulkUpdateRequest{}

	models := CreateBulkUpdateModels(updates)

	if len(models) != 0 {
		t.Errorf("CreateBulkUpdateModels() with empty input len = %d, want 0", len(models))
	}
}

func TestBulkUpdateRequest_Structure(t *testing.T) {
	req := BulkUpdateRequest{
		Filter: bson.M{"cpf": "12345678901"},
		Update: bson.M{"$set": bson.M{"email": "test@example.com"}},
		Upsert: true,
	}

	if req.Filter == nil {
		t.Error("BulkUpdateRequest Filter is nil")
	}

	if req.Update == nil {
		t.Error("BulkUpdateRequest Update is nil")
	}

	if !req.Upsert {
		t.Error("BulkUpdateRequest Upsert = false, want true")
	}
}

func TestPhoneVerificationData_Structure(t *testing.T) {
	data := PhoneVerificationData{
		CPF:         "12345678901",
		DDI:         "55",
		DDD:         "21",
		Valor:       "999999999",
		PhoneNumber: "5521999999999",
		Code:        "123456",
	}

	if data.CPF == "" {
		t.Error("PhoneVerificationData CPF is empty")
	}

	if data.DDI == "" {
		t.Error("PhoneVerificationData DDI is empty")
	}

	if data.DDD == "" {
		t.Error("PhoneVerificationData DDD is empty")
	}

	if data.Valor == "" {
		t.Error("PhoneVerificationData Valor is empty")
	}

	if data.PhoneNumber == "" {
		t.Error("PhoneVerificationData PhoneNumber is empty")
	}

	if data.Code == "" {
		t.Error("PhoneVerificationData Code is empty")
	}
}

func TestDatabaseOperation_Structure(t *testing.T) {
	called := false

	op := DatabaseOperation{
		Operation: func() error {
			called = true
			return nil
		},
		Rollback: func() error {
			return nil
		},
	}

	if op.Operation == nil {
		t.Fatal("DatabaseOperation Operation is nil")
	}

	if op.Rollback == nil {
		t.Fatal("DatabaseOperation Rollback is nil")
	}

	err := op.Operation()
	if err != nil {
		t.Errorf("DatabaseOperation Operation() error = %v, want nil", err)
	}

	if !called {
		t.Error("DatabaseOperation Operation() was not called")
	}
}

func TestGetCollectionWithWriteConcern(t *testing.T) {
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	tests := []struct {
		name           string
		collectionName string
		operationType  string
	}{
		{
			name:           "audit collection",
			collectionName: "audit_logs",
			operationType:  "audit",
		},
		{
			name:           "user data collection",
			collectionName: "users",
			operationType:  "user_data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if config.MongoDB == nil {
				t.Skip("MongoDB not initialized")
			}

			coll := GetCollectionWithWriteConcern(tt.collectionName, tt.operationType)

			if coll == nil {
				t.Error("GetCollectionWithWriteConcern() returned nil")
			}
		})
	}
}

func TestGetCollectionForReadOperation(t *testing.T) {
	if config.MongoDB == nil {
		t.Skip("MongoDB not initialized")
	}

	coll := GetCollectionForReadOperation("test_collection")

	if coll == nil {
		t.Error("GetCollectionForReadOperation() returned nil")
	}
}

func TestGetCollectionForWriteOperation(t *testing.T) {
	if config.MongoDB == nil {
		t.Skip("MongoDB not initialized")
	}

	coll := GetCollectionForWriteOperation("test_collection")

	if coll == nil {
		t.Error("GetCollectionForWriteOperation() returned nil")
	}
}

func TestGetCollectionWithLoadDistribution(t *testing.T) {
	if config.MongoDB == nil {
		t.Skip("MongoDB not initialized")
	}

	tests := []struct {
		name           string
		collectionName string
		operationType  string
	}{
		{
			name:           "read operation",
			collectionName: "users",
			operationType:  "read",
		},
		{
			name:           "write operation",
			collectionName: "users",
			operationType:  "write",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coll := GetCollectionWithLoadDistribution(tt.collectionName, tt.operationType)

			if coll == nil {
				t.Error("GetCollectionWithLoadDistribution() returned nil")
			}
		})
	}
}

func TestShouldUseBulkOperations(t *testing.T) {
	result := shouldUseBulkOperations()

	// Currently always returns false
	if result != false {
		t.Errorf("shouldUseBulkOperations() = %v, want false", result)
	}
}

func TestWriteConcern_FireAndForget(t *testing.T) {
	wc := GetWriteConcernForOperation("audit")

	if wc.W != 0 {
		t.Errorf("Fire-and-forget write concern W = %v, want 0", wc.W)
	}
}

func TestWriteConcern_Acknowledged(t *testing.T) {
	wc := GetWriteConcernForOperation("user_data")

	if wc.W != 1 {
		t.Errorf("Acknowledged write concern W = %v, want 1", wc.W)
	}
}

func TestWriteConcern_Majority(t *testing.T) {
	wc := GetWriteConcernForOperation("critical")

	if wc.W != "majority" {
		t.Errorf("Majority write concern W = %v, want majority", wc.W)
	}
}

func TestCreateBulkUpdateModels_MultipleOperations(t *testing.T) {
	updates := []BulkUpdateRequest{
		{
			Filter: bson.M{"cpf": "11111111111"},
			Update: bson.M{"$set": bson.M{"email": "test1@example.com"}},
			Upsert: true,
		},
		{
			Filter: bson.M{"cpf": "22222222222"},
			Update: bson.M{"$set": bson.M{"email": "test2@example.com"}},
			Upsert: false,
		},
		{
			Filter: bson.M{"cpf": "33333333333"},
			Update: bson.M{"$inc": bson.M{"count": 1}},
			Upsert: true,
		},
	}

	models := CreateBulkUpdateModels(updates)

	if len(models) != 3 {
		t.Errorf("CreateBulkUpdateModels() len = %d, want 3", len(models))
	}

	// Verify each model is an UpdateOneModel
	for i, model := range models {
		if _, ok := model.(*mongo.UpdateOneModel); !ok {
			t.Errorf("CreateBulkUpdateModels() model[%d] is not *mongo.UpdateOneModel", i)
		}
	}
}

func TestGetUpdateOptionsWithWriteConcern_BothUpsertValues(t *testing.T) {
	// Test with upsert = true
	optsTrue := GetUpdateOptionsWithWriteConcern("user_data", true)
	if optsTrue.Upsert == nil {
		t.Error("GetUpdateOptionsWithWriteConcern() Upsert is nil")
	} else if !*optsTrue.Upsert {
		t.Errorf("GetUpdateOptionsWithWriteConcern() Upsert = %v, want true", *optsTrue.Upsert)
	}

	// Test with upsert = false
	optsFalse := GetUpdateOptionsWithWriteConcern("user_data", false)
	if optsFalse.Upsert == nil {
		t.Error("GetUpdateOptionsWithWriteConcern() Upsert is nil")
	} else if *optsFalse.Upsert {
		t.Errorf("GetUpdateOptionsWithWriteConcern() Upsert = %v, want false", *optsFalse.Upsert)
	}
}

func TestPhoneVerificationData_AllFields(t *testing.T) {
	data := PhoneVerificationData{
		CPF:         "12345678901",
		DDI:         "55",
		DDD:         "21",
		Valor:       "999999999",
		PhoneNumber: "5521999999999",
		Code:        "123456",
	}

	// Test all fields are correctly set
	if data.CPF != "12345678901" {
		t.Errorf("PhoneVerificationData CPF = %v, want 12345678901", data.CPF)
	}

	if data.DDI != "55" {
		t.Errorf("PhoneVerificationData DDI = %v, want 55", data.DDI)
	}

	if data.DDD != "21" {
		t.Errorf("PhoneVerificationData DDD = %v, want 21", data.DDD)
	}

	if data.Valor != "999999999" {
		t.Errorf("PhoneVerificationData Valor = %v, want 999999999", data.Valor)
	}

	if data.PhoneNumber != "5521999999999" {
		t.Errorf("PhoneVerificationData PhoneNumber = %v, want 5521999999999", data.PhoneNumber)
	}

	if data.Code != "123456" {
		t.Errorf("PhoneVerificationData Code = %v, want 123456", data.Code)
	}
}

func TestGetCollectionWithReadPreference(t *testing.T) {
	if config.MongoDB == nil {
		t.Skip("MongoDB not initialized")
	}

	coll := GetCollectionWithReadPreference("test_collection", nil)

	if coll == nil {
		t.Error("GetCollectionWithReadPreference() returned nil")
	}
}

func TestGetWriteConcernForOperation_AllTypes(t *testing.T) {
	types := map[string]interface{}{
		"audit":     0,
		"user_data": 1,
		"critical":  "majority",
		"default":   1,
		"":          1,
		"unknown":   1,
		"random":    1,
	}

	for opType, expectedW := range types {
		t.Run(opType, func(t *testing.T) {
			wc := GetWriteConcernForOperation(opType)

			if wc == nil {
				t.Fatal("GetWriteConcernForOperation() returned nil")
			}

			if wc.W != expectedW {
				t.Errorf("GetWriteConcernForOperation(%s) W = %v, want %v", opType, wc.W, expectedW)
			}
		})
	}
}

// TestExecuteWithWriteConcern tests transaction execution with specific write concerns
func TestExecuteWithWriteConcern_Success(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name          string
		operationType string
	}{
		{"audit operation", "audit"},
		{"user_data operation", "user_data"},
		{"critical operation", "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executed := false
			err := ExecuteWithWriteConcern(ctx, tt.operationType, func(sessCtx mongo.SessionContext) error {
				executed = true
				return nil
			})

			require.NoError(t, err, "ExecuteWithWriteConcern should succeed")
			assert.True(t, executed, "Operation should have been executed")
		})
	}
}

// TestExecuteWithWriteConcern_Error tests error handling
func TestExecuteWithWriteConcern_Error(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	err := ExecuteWithWriteConcern(ctx, "user_data", func(sessCtx mongo.SessionContext) error {
		return errors.New("operation failed")
	})

	require.Error(t, err, "ExecuteWithWriteConcern should fail")
	assert.Contains(t, err.Error(), "operation failed")
}

// TestBulkWriteWithWriteConcern tests bulk write operations
func TestBulkWriteWithWriteConcern_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	models := []mongo.WriteModel{
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "bulk1", "value": 100}),
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "bulk2", "value": 200}),
		mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": "bulk1"}).
			SetUpdate(bson.M{"$set": bson.M{"updated": true}}),
	}

	result, err := BulkWriteWithWriteConcern(ctx, collectionName, models, "user_data")

	require.NoError(t, err, "BulkWriteWithWriteConcern should succeed")
	assert.NotNil(t, result, "Result should not be nil")
	assert.Equal(t, int64(2), result.InsertedCount, "Should insert 2 documents")
	assert.Equal(t, int64(1), result.ModifiedCount, "Should modify 1 document")
}

// TestBulkWriteWithWriteConcern_Error tests bulk write error handling
func TestBulkWriteWithWriteConcern_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Insert a document first
	_, err := testCollection.InsertOne(ctx, bson.M{"_id": "duplicate", "value": 1})
	require.NoError(t, err)

	// Try to insert duplicate
	models := []mongo.WriteModel{
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "duplicate", "value": 2}),
	}

	result, err := BulkWriteWithWriteConcern(ctx, collectionName, models, "user_data")

	require.Error(t, err, "BulkWriteWithWriteConcern should fail on duplicate key")
	assert.Nil(t, result, "Result should be nil on error")
}

// TestBulkWriteWithWriteConcern_EmptyOperations tests bulk write with no operations
func TestBulkWriteWithWriteConcern_EmptyOperations(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	models := []mongo.WriteModel{}

	result, err := BulkWriteWithWriteConcern(ctx, collectionName, models, "user_data")

	require.NoError(t, err, "Empty bulk write should succeed")
	assert.NotNil(t, result, "Result should not be nil")
	assert.Equal(t, int64(0), result.InsertedCount, "Should insert 0 documents")
}

// TestExecuteReadWithLoadDistribution tests read operations with load distribution
func TestExecuteReadWithLoadDistribution_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Insert test document
	_, err := testCollection.InsertOne(ctx, bson.M{"_id": "read_test", "value": 123})
	require.NoError(t, err)

	var result bson.M
	err = ExecuteReadWithLoadDistribution(ctx, collectionName, func(coll *mongo.Collection) error {
		return coll.FindOne(ctx, bson.M{"_id": "read_test"}).Decode(&result)
	})

	require.NoError(t, err, "Read operation should succeed")
	assert.Equal(t, int32(123), result["value"], "Should read correct value")
}

// TestExecuteReadWithLoadDistribution_Error tests read operation error handling
func TestExecuteReadWithLoadDistribution_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteReadWithLoadDistribution(ctx, collectionName, func(coll *mongo.Collection) error {
		return errors.New("read failed")
	})

	require.Error(t, err, "Read operation should fail")
	assert.Contains(t, err.Error(), "read failed")
}

// TestExecuteWriteWithOptimizedConcern tests write operations with optimized concern
func TestExecuteWriteWithOptimizedConcern_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteWriteWithOptimizedConcern(ctx, collectionName, "user_data", func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "optimized_test", "value": 999})
		return err
	})

	require.NoError(t, err, "Write operation should succeed")

	// Verify document was created
	var result bson.M
	err = testCollection.FindOne(ctx, bson.M{"_id": "optimized_test"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, int32(999), result["value"])
}

// TestExecuteWriteWithOptimizedConcern_Error tests write operation error handling
func TestExecuteWriteWithOptimizedConcern_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteWriteWithOptimizedConcern(ctx, collectionName, "user_data", func(coll *mongo.Collection) error {
		return errors.New("write failed")
	})

	require.Error(t, err, "Write operation should fail")
	assert.Contains(t, err.Error(), "write failed")
}

// TestExecuteWithLoadDistribution tests operations with load distribution
func TestExecuteWithLoadDistribution_Read(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	executed := false
	err := ExecuteWithLoadDistribution(ctx, "read", func(sessCtx mongo.SessionContext) error {
		executed = true
		return nil
	})

	require.NoError(t, err, "Read operation should succeed")
	assert.True(t, executed, "Operation should have been executed")
}

// TestExecuteWithLoadDistribution_Write tests write operations with load distribution
func TestExecuteWithLoadDistribution_Write(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	executed := false
	err := ExecuteWithLoadDistribution(ctx, "write", func(sessCtx mongo.SessionContext) error {
		executed = true
		return nil
	})

	require.NoError(t, err, "Write operation should succeed")
	assert.True(t, executed, "Operation should have been executed")
}

// TestExecuteWithLoadDistribution_Audit tests audit operations with load distribution
func TestExecuteWithLoadDistribution_Audit(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	executed := false
	err := ExecuteWithLoadDistribution(ctx, "audit", func(sessCtx mongo.SessionContext) error {
		executed = true
		return nil
	})

	require.NoError(t, err, "Audit operation should succeed")
	assert.True(t, executed, "Operation should have been executed")
}

// TestExecuteWithLoadDistribution_Error tests error handling with load distribution
func TestExecuteWithLoadDistribution_Error(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	err := ExecuteWithLoadDistribution(ctx, "read", func(sessCtx mongo.SessionContext) error {
		return errors.New("operation failed")
	})

	require.Error(t, err, "Operation should fail")
	assert.Contains(t, err.Error(), "operation failed")
}

// TestExecuteReadWithSecondaryPreference tests read operations preferring secondary nodes
func TestExecuteReadWithSecondaryPreference_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Insert test document
	_, err := testCollection.InsertOne(ctx, bson.M{"_id": "secondary_test", "value": 456})
	require.NoError(t, err)

	var result bson.M
	err = ExecuteReadWithSecondaryPreference(ctx, collectionName, func(coll *mongo.Collection) error {
		return coll.FindOne(ctx, bson.M{"_id": "secondary_test"}).Decode(&result)
	})

	require.NoError(t, err, "Read operation should succeed")
	assert.Equal(t, int32(456), result["value"], "Should read correct value")
}

// TestExecuteReadWithSecondaryPreference_Error tests error handling
func TestExecuteReadWithSecondaryPreference_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteReadWithSecondaryPreference(ctx, collectionName, func(coll *mongo.Collection) error {
		return errors.New("read failed")
	})

	require.Error(t, err, "Read operation should fail")
	assert.Contains(t, err.Error(), "read failed")
}

// TestExecuteWriteWithPrimaryOptimization tests write operations with primary node optimization
func TestExecuteWriteWithPrimaryOptimization_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteWriteWithPrimaryOptimization(ctx, collectionName, func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "primary_test", "value": 789})
		return err
	})

	require.NoError(t, err, "Write operation should succeed")

	// Verify document was created
	var result bson.M
	err = testCollection.FindOne(ctx, bson.M{"_id": "primary_test"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, int32(789), result["value"])
}

// TestExecuteWriteWithPrimaryOptimization_Error tests error handling
func TestExecuteWriteWithPrimaryOptimization_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteWriteWithPrimaryOptimization(ctx, collectionName, func(coll *mongo.Collection) error {
		return errors.New("write failed")
	})

	require.Error(t, err, "Write operation should fail")
	assert.Contains(t, err.Error(), "write failed")
}

// TestExecuteBulkWriteOptimized tests optimized bulk write operations
func TestExecuteBulkWriteOptimized_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	models := []mongo.WriteModel{
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "opt_bulk1", "value": 100}),
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "opt_bulk2", "value": 200}),
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "opt_bulk3", "value": 300}),
	}

	result, err := ExecuteBulkWriteOptimized(ctx, collectionName, models, "user_data")

	require.NoError(t, err, "Optimized bulk write should succeed")
	assert.NotNil(t, result, "Result should not be nil")
	assert.Equal(t, int64(3), result.InsertedCount, "Should insert 3 documents")
}

// TestExecuteBulkWriteOptimized_WithUpdates tests optimized bulk write with mixed operations
func TestExecuteBulkWriteOptimized_WithUpdates(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Insert initial documents
	_, err := testCollection.InsertOne(ctx, bson.M{"_id": "opt_update1", "counter": 0})
	require.NoError(t, err)

	models := []mongo.WriteModel{
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "opt_new1", "value": 100}),
		mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": "opt_update1"}).
			SetUpdate(bson.M{"$inc": bson.M{"counter": 1}}),
		mongo.NewDeleteOneModel().SetFilter(bson.M{"_id": "opt_update1"}),
	}

	result, err := ExecuteBulkWriteOptimized(ctx, collectionName, models, "user_data")

	require.NoError(t, err, "Optimized bulk write should succeed")
	assert.NotNil(t, result, "Result should not be nil")
	assert.Equal(t, int64(1), result.InsertedCount, "Should insert 1 document")
	assert.Equal(t, int64(1), result.ModifiedCount, "Should modify 1 document")
	assert.Equal(t, int64(1), result.DeletedCount, "Should delete 1 document")
}

// TestExecuteBulkWriteOptimized_Error tests error handling
func TestExecuteBulkWriteOptimized_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	// Insert a document first
	_, err := testCollection.InsertOne(ctx, bson.M{"_id": "opt_dup", "value": 1})
	require.NoError(t, err)

	// Try to insert duplicate
	models := []mongo.WriteModel{
		mongo.NewInsertOneModel().SetDocument(bson.M{"_id": "opt_dup", "value": 2}),
	}

	result, err := ExecuteBulkWriteOptimized(ctx, collectionName, models, "user_data")

	require.Error(t, err, "Optimized bulk write should fail on duplicate key")
	assert.Nil(t, result, "Result should be nil on error")
}

// TestExecuteWriteWithLoadOptimization tests write operations with load optimization
func TestExecuteWriteWithLoadOptimization_Success(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteWriteWithLoadOptimization(ctx, collectionName, "user_data", func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "load_opt_test", "value": 111})
		return err
	})

	require.NoError(t, err, "Write with load optimization should succeed")

	// Verify document was created
	var result bson.M
	err = testCollection.FindOne(ctx, bson.M{"_id": "load_opt_test"}).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, int32(111), result["value"])
}

// TestExecuteWriteWithLoadOptimization_SlowOperation tests slow operation warning
func TestExecuteWriteWithLoadOptimization_SlowOperation(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteWriteWithLoadOptimization(ctx, collectionName, "user_data", func(coll *mongo.Collection) error {
		// Simulate slow operation
		time.Sleep(150 * time.Millisecond)
		_, err := coll.InsertOne(ctx, bson.M{"_id": "slow_test", "value": 222})
		return err
	})

	require.NoError(t, err, "Slow write operation should succeed")

	// Verify document was created
	count, err := testCollection.CountDocuments(ctx, bson.M{"_id": "slow_test"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

// TestExecuteWriteWithLoadOptimization_Error tests error handling
func TestExecuteWriteWithLoadOptimization_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	collectionName := testCollection.Name()

	err := ExecuteWriteWithLoadOptimization(ctx, collectionName, "user_data", func(coll *mongo.Collection) error {
		return errors.New("load optimization write failed")
	})

	require.Error(t, err, "Write with load optimization should fail")
	assert.Contains(t, err.Error(), "load optimization write failed")
}

// TestExecuteWriteWithCollectionOptimization tests write operations with collection-specific optimization
func TestExecuteWriteWithCollectionOptimization_AuditCollection(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	// Set up audit collection
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.AuditLogsCollection = testCollection.Name()

	err := ExecuteWriteWithCollectionOptimization(ctx, testCollection.Name(), "audit", func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "audit_test", "action": "test"})
		return err
	})

	require.NoError(t, err, "Audit write should succeed")

	// Verify document was created
	count, err := testCollection.CountDocuments(ctx, bson.M{"_id": "audit_test"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

// TestExecuteWriteWithCollectionOptimization_CitizenCollection tests citizen collection optimization
func TestExecuteWriteWithCollectionOptimization_CitizenCollection(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	// Use the citizen collection configured in setup
	err := ExecuteWriteWithCollectionOptimization(ctx, config.AppConfig.CitizenCollection, "user_data", func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "citizen_test", "cpf": "12345678901"})
		return err
	})

	require.NoError(t, err, "Citizen write should succeed")
}

// TestExecuteWriteWithCollectionOptimization_DefaultCollection tests default collection optimization
func TestExecuteWriteWithCollectionOptimization_DefaultCollection(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	err := ExecuteWriteWithCollectionOptimization(ctx, "unknown_collection", "user_data", func(coll *mongo.Collection) error {
		_, err := coll.InsertOne(ctx, bson.M{"_id": "default_test", "value": 333})
		return err
	})

	require.NoError(t, err, "Default collection write should succeed")
}

// TestExecuteWriteWithCollectionOptimization_SlowOperation tests slow operation detection
func TestExecuteWriteWithCollectionOptimization_SlowOperation(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	err := ExecuteWriteWithCollectionOptimization(ctx, testCollection.Name(), "user_data", func(coll *mongo.Collection) error {
		// Simulate slow operation
		time.Sleep(150 * time.Millisecond)
		_, err := coll.InsertOne(ctx, bson.M{"_id": "slow_coll_test", "value": 444})
		return err
	})

	require.NoError(t, err, "Slow collection write should succeed")
}

// TestExecuteWriteWithCollectionOptimization_Error tests error handling
func TestExecuteWriteWithCollectionOptimization_Error(t *testing.T) {
	testCollection, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()

	err := ExecuteWriteWithCollectionOptimization(ctx, testCollection.Name(), "user_data", func(coll *mongo.Collection) error {
		return errors.New("collection optimization write failed")
	})

	require.Error(t, err, "Collection optimization write should fail")
	assert.Contains(t, err.Error(), "collection optimization write failed")
}

// setupRedisForDatabaseTest initializes Redis client for testing
func setupRedisForDatabaseTest(t *testing.T) func() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("Skipping Redis integration tests: REDIS_ADDR not set")
	}

	// Initialize Redis client directly
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// Wrap with traced client
	config.Redis = redisclient.NewClient(singleClient)

	// Test connection
	ctx := context.Background()
	err := config.Redis.Ping(ctx).Err()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Return cleanup function
	return func() {
		// Clean up test keys
		ctx := context.Background()
		keys, _ := config.Redis.Keys(ctx, "citizen:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}
		keys, _ = config.Redis.Keys(ctx, "citizen_wallet:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}
		keys, _ = config.Redis.Keys(ctx, "maintenance_requests:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}
	}
}

// setupDatabaseWithRedisTest sets up both MongoDB and Redis
// UNUSED: Commented out to fix linting errors - function is not called anywhere
// nolint:unused
/*
func setupDatabaseWithRedisTest(t *testing.T) (*mongo.Collection, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping database utils tests: MONGODB_URI not set")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		t.Skip("Skipping Redis integration tests: REDIS_ADDR not set")
	}

	logging.InitLogger()

	// Initialize config
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.CitizenCollection = "test_db_utils_citizens"
	config.AppConfig.SelfDeclaredCollection = "test_db_utils_self_declared"
	config.AppConfig.PhoneVerificationCollection = "test_db_utils_phone_verification"

	// MongoDB setup
	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		_ = client.Disconnect(ctx)
		t.Skipf("MongoDB not available or authentication failed: %v", err)
	}

	config.MongoDB = client.Database("rmi_test_db_utils_redis")
	testCollection := config.MongoDB.Collection("test_operations")

	// Clean up existing data
	_ = testCollection.Drop(ctx)

	// Redis setup
	singleClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	config.Redis = redisclient.NewClient(singleClient)

	err = config.Redis.Ping(ctx).Err()
	if err != nil {
		_ = config.MongoDB.Drop(ctx)
		_ = client.Disconnect(ctx)
		t.Skipf("Redis not available: %v", err)
	}

	return testCollection, func() {
		// Clean up Redis
		keys, _ := config.Redis.Keys(ctx, "citizen:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}
		keys, _ = config.Redis.Keys(ctx, "citizen_wallet:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}
		keys, _ = config.Redis.Keys(ctx, "maintenance_requests:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}

		// Clean up MongoDB
		_ = config.MongoDB.Drop(ctx)
		_ = client.Disconnect(ctx)
	}
}
*/

// TestUpdateSelfDeclaredPendingPhone tests updating pending phone in self-declared collection
func TestUpdateSelfDeclaredPendingPhone_Success(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	cpf := "12345678901"

	data := PhoneVerificationData{
		CPF:         cpf,
		DDI:         "55",
		DDD:         "21",
		Valor:       "999999999",
		PhoneNumber: "5521999999999",
		Code:        "123456",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}

	err := UpdateSelfDeclaredPendingPhone(ctx, cpf, data)
	require.NoError(t, err, "UpdateSelfDeclaredPendingPhone should succeed")

	// Verify the document was created/updated
	var result bson.M
	err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(ctx, bson.M{"cpf": cpf}).Decode(&result)
	require.NoError(t, err, "Should find the updated document")

	// Verify pending phone was set
	assert.NotNil(t, result["telefone_pending"], "telefone_pending should be set")
}

// TestUpdateSelfDeclaredPendingPhone_Upsert tests upsert functionality
func TestUpdateSelfDeclaredPendingPhone_Upsert(t *testing.T) {
	_, cleanup := setupDatabaseUtilsTest(t)
	defer cleanup()

	ctx := context.Background()
	cpf := "98765432100"

	// Count documents before
	countBefore, err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).
		CountDocuments(ctx, bson.M{"cpf": cpf})
	require.NoError(t, err)
	assert.Equal(t, int64(0), countBefore, "Document should not exist initially")

	data := PhoneVerificationData{
		CPF:         cpf,
		DDI:         "55",
		DDD:         "11",
		Valor:       "888888888",
		PhoneNumber: "5511888888888",
		Code:        "654321",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
	}

	err = UpdateSelfDeclaredPendingPhone(ctx, cpf, data)
	require.NoError(t, err, "UpdateSelfDeclaredPendingPhone should succeed with upsert")

	// Count documents after
	countAfter, err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).
		CountDocuments(ctx, bson.M{"cpf": cpf})
	require.NoError(t, err)
	assert.Equal(t, int64(1), countAfter, "Document should be created via upsert")
}

// TestInvalidateCitizenCache tests cache invalidation
func TestInvalidateCitizenCache_Success(t *testing.T) {
	cleanup := setupRedisForDatabaseTest(t)
	defer cleanup()

	ctx := context.Background()
	cpf := "11122233344"

	// Set up cache entries
	citizenKey := fmt.Sprintf("citizen:%s", cpf)
	walletKey := fmt.Sprintf("citizen_wallet:%s", cpf)
	maintenanceKey := fmt.Sprintf("maintenance_requests:%s", cpf)

	err := config.Redis.Set(ctx, citizenKey, "citizen_data", 0).Err()
	require.NoError(t, err)
	err = config.Redis.Set(ctx, walletKey, "wallet_data", 0).Err()
	require.NoError(t, err)
	err = config.Redis.Set(ctx, maintenanceKey, "maintenance_data", 0).Err()
	require.NoError(t, err)

	// Verify keys exist
	exists, err := config.Redis.Exists(ctx, citizenKey, walletKey, maintenanceKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(3), exists, "All cache keys should exist")

	// Invalidate cache
	err = InvalidateCitizenCache(ctx, cpf)
	require.NoError(t, err, "InvalidateCitizenCache should succeed")

	// Verify keys were deleted
	exists, err = config.Redis.Exists(ctx, citizenKey, walletKey, maintenanceKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "All cache keys should be deleted")
}

// TestInvalidateCitizenCache_PartialFailure tests partial failure handling
func TestInvalidateCitizenCache_PartialFailure(t *testing.T) {
	cleanup := setupRedisForDatabaseTest(t)
	defer cleanup()

	ctx := context.Background()
	cpf := "55566677788"

	// Set up only citizen cache entry (not wallet or maintenance)
	citizenKey := fmt.Sprintf("citizen:%s", cpf)
	err := config.Redis.Set(ctx, citizenKey, "citizen_data", 0).Err()
	require.NoError(t, err)

	// Invalidate cache - should succeed even if some keys don't exist
	err = InvalidateCitizenCache(ctx, cpf)
	require.NoError(t, err, "InvalidateCitizenCache should succeed even with partial keys")

	// Verify citizen key was deleted
	exists, err := config.Redis.Exists(ctx, citizenKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "Citizen cache key should be deleted")
}

// TestInvalidateCitizenCache_EmptyCPF tests handling of empty CPF
func TestInvalidateCitizenCache_EmptyCPF(t *testing.T) {
	cleanup := setupRedisForDatabaseTest(t)
	defer cleanup()

	ctx := context.Background()

	// This should not error, just won't find any keys to delete
	err := InvalidateCitizenCache(ctx, "")
	require.NoError(t, err, "InvalidateCitizenCache should handle empty CPF gracefully")
}
