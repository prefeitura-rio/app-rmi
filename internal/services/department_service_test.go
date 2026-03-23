package services

import (
	"context"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func setupDepartmentServiceTest(t *testing.T) (*DepartmentService, func()) {
	if config.MongoDB == nil || config.Redis == nil {
		t.Skip("Skipping department service tests: MongoDB or Redis not initialized")
	}

	_ = logging.InitLogger()

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.DepartmentCollection = "test_departments"

	// Create DataManager for cache-aware operations
	dataManager := NewDataManager(config.Redis, config.MongoDB, logging.Logger)
	service := NewDepartmentService(config.MongoDB, dataManager, logging.Logger)

	// Clear Redis cache before test
	ctx := context.Background()
	keys, err := config.Redis.Keys(ctx, "department:*").Result()
	if err == nil && len(keys) > 0 {
		_ = config.Redis.Del(ctx, keys...).Err()
	}

	return service, func() {
		ctx := context.Background()
		_ = config.MongoDB.Collection(config.AppConfig.DepartmentCollection).Drop(ctx)
		// Clear Redis cache after test
		keys, err := config.Redis.Keys(ctx, "department:*").Result()
		if err == nil && len(keys) > 0 {
			_ = config.Redis.Del(ctx, keys...).Err()
		}
	}
}

func TestNewDepartmentService(t *testing.T) {
	if config.MongoDB == nil || config.Redis == nil {
		t.Skip("Skipping: MongoDB or Redis not initialized")
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, logging.Logger)
	service := NewDepartmentService(config.MongoDB, dataManager, logging.Logger)

	if service == nil {
		t.Error("NewDepartmentService() returned nil")
	}
}

func TestGetDepartmentByID_Success(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test department
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	dept := bson.M{
		"cd_ua":    "1000",
		"sigla_ua": "PCRJ",
		"nome_ua":  "Prefeitura da Cidade do Rio de Janeiro",
		"nivel":    "1",
	}

	_, err := collection.InsertOne(ctx, dept)
	if err != nil {
		t.Fatalf("Failed to insert department: %v", err)
	}

	result, err := service.GetDepartmentByID(ctx, "1000")
	if err != nil {
		t.Errorf("GetDepartmentByID() error = %v", err)
	}

	if result == nil {
		t.Fatal("GetDepartmentByID() returned nil")
	}

	if result.CdUA != "1000" {
		t.Errorf("GetDepartmentByID() CdUA = %v, want 1000", result.CdUA)
	}

	if result.SiglaUA != "PCRJ" {
		t.Errorf("GetDepartmentByID() SiglaUA = %v, want PCRJ", result.SiglaUA)
	}
}

func TestGetDepartmentByID_NotFound(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	_, err := service.GetDepartmentByID(ctx, "9999")
	if err == nil {
		t.Error("GetDepartmentByID() should return error for non-existent department")
	}
}

func TestGetDepartmentByID_CacheHit(t *testing.T) {
	t.Parallel() // Run in parallel but use unique cd_ua to avoid conflicts

	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Use unique cd_ua for parallel test execution
	uniqueCdUA := "cache_test_1000"

	// Insert test department with updated_at field
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	now := time.Now().UTC()
	dept := bson.M{
		"cd_ua":      uniqueCdUA,
		"sigla_ua":   "PCRJ",
		"nome_ua":    "Prefeitura da Cidade do Rio de Janeiro",
		"nivel":      1,
		"updated_at": primitive.NewDateTimeFromTime(now),
	}

	_, err := collection.InsertOne(ctx, dept)
	if err != nil {
		t.Fatalf("Failed to insert department: %v", err)
	}
	defer func() {
		_, _ = collection.DeleteOne(ctx, bson.M{"cd_ua": uniqueCdUA})
	}()

	// First call - populates cache from MongoDB
	result1, err := service.GetDepartmentByID(ctx, uniqueCdUA)
	if err != nil {
		t.Fatalf("First GetDepartmentByID() error = %v", err)
	}

	if result1 == nil {
		t.Fatal("First GetDepartmentByID() returned nil")
	}

	if result1.CdUA != uniqueCdUA {
		t.Errorf("First GetDepartmentByID() CdUA = %v, want %s", result1.CdUA, uniqueCdUA)
	}

	// Delete the document from MongoDB (cache should still have it)
	_, err = collection.DeleteOne(ctx, bson.M{"cd_ua": uniqueCdUA})
	if err != nil {
		t.Fatalf("Failed to delete department from MongoDB: %v", err)
	}

	// Verify document is gone from MongoDB
	var checkDoc bson.M
	err = collection.FindOne(ctx, bson.M{"cd_ua": uniqueCdUA}).Decode(&checkDoc)
	if err == nil {
		t.Fatal("Document should have been deleted from MongoDB")
	}

	// Second call - should return from cache even though MongoDB document is gone
	result2, err := service.GetDepartmentByID(ctx, uniqueCdUA)
	if err != nil {
		t.Fatalf("Second GetDepartmentByID() error = %v (should return from cache)", err)
	}

	if result2 == nil {
		t.Fatal("Second GetDepartmentByID() returned nil (should return from cache)")
	}

	if result2.CdUA != uniqueCdUA {
		t.Errorf("Second GetDepartmentByID() CdUA = %v, want %s (from cache)", result2.CdUA, uniqueCdUA)
	}

	if result2.SiglaUA != "PCRJ" {
		t.Errorf("Second GetDepartmentByID() SiglaUA = %v, want PCRJ (from cache)", result2.SiglaUA)
	}

	// Verify nivel field was preserved through cache round-trip (critical: JSON deserializes numbers as float64)
	if result2.Nivel != 1 {
		t.Errorf("Second GetDepartmentByID() Nivel = %v, want 1 (from cache, must handle float64)", result2.Nivel)
	}

	// Verify updated_at field was preserved through cache round-trip
	if result2.UpdatedAt == nil {
		t.Error("Second GetDepartmentByID() UpdatedAt = nil (should be preserved from cache)")
	} else {
		// Allow for slight time precision differences due to JSON serialization
		diff := result2.UpdatedAt.Sub(now).Abs()
		if diff > time.Second {
			t.Errorf("Second GetDepartmentByID() UpdatedAt time diff = %v, want < 1s (from cache)", diff)
		}
	}
}

func TestListDepartments_Empty(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	filters := DepartmentFilters{
		Page:    1,
		PerPage: 10,
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() error = %v", err)
		return
	}

	if result == nil {
		t.Error("ListDepartments() returned nil result")
		return
	}

	if result.Pagination.Total != 0 {
		t.Errorf("ListDepartments() Total = %v, want 0", result.Pagination.Total)
	}
}

func TestListDepartments_WithData(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{
			"cd_ua":    "1000",
			"sigla_ua": "PCRJ",
			"nome_ua":  "Prefeitura",
			"nivel":    "1",
		},
		bson.M{
			"cd_ua":     "2000",
			"sigla_ua":  "SMF",
			"nome_ua":   "Secretaria Municipal de Fazenda",
			"nivel":     "2",
			"cd_ua_pai": "1000",
		},
		bson.M{
			"cd_ua":     "3000",
			"sigla_ua":  "SME",
			"nome_ua":   "Secretaria Municipal de Educação",
			"nivel":     "2",
			"cd_ua_pai": "1000",
		},
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}

	filters := DepartmentFilters{
		Page:    1,
		PerPage: 10,
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() error = %v", err)
	}

	if result.Pagination.Total != 3 {
		t.Errorf("ListDepartments() Total = %v, want 3", result.Pagination.Total)
	}

	if len(result.Departments) != 3 {
		t.Errorf("ListDepartments() len(Departments) = %v, want 3", len(result.Departments))
	}
}

func TestListDepartments_FilterByParentID(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{
			"cd_ua":    "1000",
			"sigla_ua": "PCRJ",
			"nome_ua":  "Prefeitura",
			"nivel":    "1",
		},
		bson.M{
			"cd_ua":     "2000",
			"sigla_ua":  "SMF",
			"nome_ua":   "Secretaria de Fazenda",
			"nivel":     "2",
			"cd_ua_pai": "1000",
		},
		bson.M{
			"cd_ua":     "3000",
			"sigla_ua":  "SME",
			"nome_ua":   "Secretaria de Educação",
			"nivel":     "2",
			"cd_ua_pai": "1000",
		},
		bson.M{
			"cd_ua":     "4000",
			"sigla_ua":  "SUBSECRETARIA",
			"nome_ua":   "Subsecretaria",
			"nivel":     "3",
			"cd_ua_pai": "2000",
		},
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}

	filters := DepartmentFilters{
		Page:     1,
		PerPage:  10,
		ParentID: "1000",
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() error = %v", err)
	}

	if result.Pagination.Total != 2 {
		t.Errorf("ListDepartments() with ParentID filter Total = %v, want 2", result.Pagination.Total)
	}
}

func TestListDepartments_FilterByExactLevel(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nivel": "1", "nome_ua": "Nível 1"},
		bson.M{"cd_ua": "2000", "nivel": "2", "nome_ua": "Nível 2"},
		bson.M{"cd_ua": "3000", "nivel": "3", "nome_ua": "Nível 3"},
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}

	level := 2
	filters := DepartmentFilters{
		Page:       1,
		PerPage:    10,
		ExactLevel: &level,
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() error = %v", err)
	}

	if result.Pagination.Total != 1 {
		t.Errorf("ListDepartments() with ExactLevel filter Total = %v, want 1", result.Pagination.Total)
	}

	if len(result.Departments) > 0 && result.Departments[0].Nivel != 2 {
		t.Errorf("ListDepartments() filtered department Nivel = %v, want 2", result.Departments[0].Nivel)
	}
}

func TestListDepartments_FilterByMinMaxLevel(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nivel": "1", "nome_ua": "Nível 1"},
		bson.M{"cd_ua": "2000", "nivel": "2", "nome_ua": "Nível 2"},
		bson.M{"cd_ua": "3000", "nivel": "3", "nome_ua": "Nível 3"},
		bson.M{"cd_ua": "4000", "nivel": "4", "nome_ua": "Nível 4"},
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}

	minLevel := 2
	maxLevel := 3
	filters := DepartmentFilters{
		Page:     1,
		PerPage:  10,
		MinLevel: &minLevel,
		MaxLevel: &maxLevel,
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() error = %v", err)
	}

	if result.Pagination.Total != 2 {
		t.Errorf("ListDepartments() with MinLevel/MaxLevel filter Total = %v, want 2", result.Pagination.Total)
	}
}

func TestListDepartments_FilterBySiglaUA(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "sigla_ua": "SMF", "nome_ua": "Fazenda"},
		bson.M{"cd_ua": "2000", "sigla_ua": "SME", "nome_ua": "Educação"},
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}

	filters := DepartmentFilters{
		Page:    1,
		PerPage: 10,
		SiglaUA: "SMF",
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() error = %v", err)
	}

	if result.Pagination.Total != 1 {
		t.Errorf("ListDepartments() with SiglaUA filter Total = %v, want 1", result.Pagination.Total)
	}

	if len(result.Departments) > 0 && result.Departments[0].SiglaUA != "SMF" {
		t.Errorf("ListDepartments() filtered department SiglaUA = %v, want SMF", result.Departments[0].SiglaUA)
	}
}

func TestListDepartments_SearchByName(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Secretaria de Fazenda"},
		bson.M{"cd_ua": "2000", "nome_ua": "Secretaria de Educação"},
		bson.M{"cd_ua": "3000", "nome_ua": "Departamento de Saúde"},
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}

	filters := DepartmentFilters{
		Page:    1,
		PerPage: 10,
		Search:  "Secretaria",
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() with search error = %v", err)
	}

	if result.Pagination.Total != 2 {
		t.Errorf("ListDepartments() with search Total = %v, want 2", result.Pagination.Total)
	}
}

func TestListDepartments_SearchBySigla(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "sigla_ua": "SMF", "nome_ua": "Fazenda"},
		bson.M{"cd_ua": "2000", "sigla_ua": "SME", "nome_ua": "Educação"},
		bson.M{"cd_ua": "3000", "sigla_ua": "SMS", "nome_ua": "Saúde"},
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}

	filters := DepartmentFilters{
		Page:    1,
		PerPage: 10,
		Search:  "SM",
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() with search error = %v", err)
	}

	if result.Pagination.Total != 3 {
		t.Errorf("ListDepartments() with search Total = %v, want 3", result.Pagination.Total)
	}
}

func TestListDepartments_Pagination(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert 5 departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	var departments []interface{}
	for i := 1; i <= 5; i++ {
		departments = append(departments, bson.M{
			"cd_ua":   "100" + string(rune('0'+i)),
			"nome_ua": "Department " + string(rune('A'+i-1)),
			"nivel":   "1",
		})
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}

	// Page 1
	filters := DepartmentFilters{
		Page:    1,
		PerPage: 2,
	}

	result, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Errorf("ListDepartments() page 1 error = %v", err)
	}

	if len(result.Departments) != 2 {
		t.Errorf("ListDepartments() page 1 len(Departments) = %v, want 2", len(result.Departments))
	}

	if result.Pagination.TotalPages != 3 {
		t.Errorf("ListDepartments() TotalPages = %v, want 3", result.Pagination.TotalPages)
	}
}

func TestConvertRawToDepartment_StringID(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	rawDoc := bson.M{
		"_id":       "test-id-123",
		"cd_ua":     "1000",
		"sigla_ua":  "SMF",
		"nome_ua":   "Secretaria de Fazenda",
		"cd_ua_pai": "999",
		"nivel":     "2",
	}

	dept := service.convertRawToDepartment(rawDoc)

	if dept.ID != "test-id-123" {
		t.Errorf("convertRawToDepartment() ID = %v, want test-id-123", dept.ID)
	}

	if dept.CdUA != "1000" {
		t.Errorf("convertRawToDepartment() CdUA = %v, want 1000", dept.CdUA)
	}

	if dept.Nivel != 2 {
		t.Errorf("convertRawToDepartment() Nivel = %v, want 2", dept.Nivel)
	}
}

func TestConvertRawToDepartment_ObjectID(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	objectID := primitive.NewObjectID()

	rawDoc := bson.M{
		"_id":     objectID,
		"cd_ua":   "1000",
		"nome_ua": "Test Department",
	}

	dept := service.convertRawToDepartment(rawDoc)

	if dept.ID != objectID.Hex() {
		t.Errorf("convertRawToDepartment() ID = %v, want %v", dept.ID, objectID.Hex())
	}
}

func TestConvertRawToDepartment_IntegerNivel(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	tests := []struct {
		name     string
		nivel    interface{}
		expected int
	}{
		{"int", int(1), 1},
		{"int32", int32(2), 2},
		{"int64", int64(3), 3},
		{"string", "4", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawDoc := bson.M{
				"nivel": tt.nivel,
			}

			dept := service.convertRawToDepartment(rawDoc)

			if dept.Nivel != tt.expected {
				t.Errorf("convertRawToDepartment() Nivel = %v, want %v", dept.Nivel, tt.expected)
			}
		})
	}
}

func TestConvertRawToDepartment_OptionalFields(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	msg := "Test message"
	now := time.Now()

	rawDoc := bson.M{
		"cd_ua":      "1000",
		"msg":        msg,
		"updated_at": primitive.NewDateTimeFromTime(now),
	}

	dept := service.convertRawToDepartment(rawDoc)

	if dept.Msg == nil {
		t.Error("convertRawToDepartment() Msg should not be nil")
	} else if *dept.Msg != msg {
		t.Errorf("convertRawToDepartment() Msg = %v, want %v", *dept.Msg, msg)
	}

	if dept.UpdatedAt == nil {
		t.Error("convertRawToDepartment() UpdatedAt should not be nil")
	}
}

func TestConvertRawToDepartment_TimeTime(t *testing.T) {
	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	now := time.Now()

	rawDoc := bson.M{
		"cd_ua":      "1000",
		"updated_at": now,
	}

	dept := service.convertRawToDepartment(rawDoc)

	if dept.UpdatedAt == nil {
		t.Error("convertRawToDepartment() UpdatedAt should not be nil")
	} else if !dept.UpdatedAt.Equal(now) {
		t.Errorf("convertRawToDepartment() UpdatedAt = %v, want %v", *dept.UpdatedAt, now)
	}
}

func TestListDepartments_CacheHit(t *testing.T) {
	t.Parallel() // Run in parallel but use unique cd_ua values to avoid conflicts

	service, cleanup := setupDepartmentServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Use unique cd_ua values for parallel test execution
	uniqueID1 := "cache_list_1000"
	uniqueID2 := "cache_list_2000"

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{
			"cd_ua":    uniqueID1,
			"sigla_ua": "PCRJ",
			"nome_ua":  "Prefeitura",
			"nivel":    1,
		},
		bson.M{
			"cd_ua":     uniqueID2,
			"sigla_ua":  "SMF",
			"nome_ua":   "Secretaria Municipal de Fazenda",
			"nivel":     2,
			"cd_ua_pai": uniqueID1,
		},
	}

	_, err := collection.InsertMany(ctx, departments)
	if err != nil {
		t.Fatalf("Failed to insert departments: %v", err)
	}
	defer func() {
		_, _ = collection.DeleteOne(ctx, bson.M{"cd_ua": uniqueID1})
		_, _ = collection.DeleteOne(ctx, bson.M{"cd_ua": uniqueID2})
	}()

	// Use a filter that matches only our test departments
	filters := DepartmentFilters{
		ParentID: uniqueID1,
		Page:     1,
		PerPage:  10,
	}

	// First call - populates cache from MongoDB
	result1, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Fatalf("First ListDepartments() error = %v", err)
	}

	if result1.Pagination.Total != 1 {
		t.Errorf("First ListDepartments() Total = %v, want 1", result1.Pagination.Total)
	}

	// Delete the child department from MongoDB (cache should still have it)
	_, err = collection.DeleteOne(ctx, bson.M{"cd_ua": uniqueID2})
	if err != nil {
		t.Fatalf("Failed to delete department from MongoDB: %v", err)
	}

	// Verify document is gone from MongoDB
	count, err := collection.CountDocuments(ctx, bson.M{"cd_ua": uniqueID2})
	if err != nil {
		t.Fatalf("Failed to count documents: %v", err)
	}
	if count != 0 {
		t.Fatal("Document should have been deleted from MongoDB")
	}

	// Second call - should return from cache even though MongoDB document is gone
	result2, err := service.ListDepartments(ctx, filters)
	if err != nil {
		t.Fatalf("Second ListDepartments() error = %v (should return from cache)", err)
	}

	if result2.Pagination.Total != 1 {
		t.Errorf("Second ListDepartments() Total = %v, want 1 (from cache)", result2.Pagination.Total)
	}

	if len(result2.Departments) != 1 {
		t.Errorf("Second ListDepartments() len(Departments) = %v, want 1 (from cache)", len(result2.Departments))
	}

	// Verify nivel field was preserved through cache round-trip (float64 handling)
	for _, dept := range result2.Departments {
		if dept.Nivel == 0 {
			t.Errorf("Second ListDepartments() department has Nivel = 0 (should be 2 from cache)")
		}
	}
}
