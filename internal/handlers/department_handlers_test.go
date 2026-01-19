package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func setupDepartmentHandlersTest(t *testing.T) (*gin.Engine, func()) {
	// Use the shared MongoDB from common_test.go TestMain
	gin.SetMode(gin.TestMode)

	// Configure test collection
	config.AppConfig.DepartmentCollection = "test_departments"

	ctx := context.Background()
	database := config.MongoDB

	// Initialize global department service instance
	services.DepartmentServiceInstance = services.NewDepartmentService(database, logging.Logger)

	router := gin.New()
	router.GET("/departments", ListDepartments)
	router.GET("/departments/:cd_ua", GetDepartment)

	return router, func() {
		database.Drop(ctx)
		services.DepartmentServiceInstance = nil
	}
}

// Test ListDepartments - Empty Database
func TestListDepartments_Empty(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status for empty list")

	var response models.DepartmentListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 0, response.Pagination.Total, "Expected 0 departments in empty database")
	assert.Equal(t, int64(0), response.TotalCount, "Expected 0 total count")
	assert.Empty(t, response.Departments, "Expected empty departments array")
	assert.Equal(t, 1, response.Pagination.Page, "Expected page 1")
	assert.Equal(t, 10, response.Pagination.PerPage, "Expected per_page 10")
}

// Test ListDepartments - With Data
func TestListDepartments_WithData(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{
			"cd_ua":           "1000",
			"sigla_ua":        "PCRJ",
			"nome_ua":         "Prefeitura da Cidade do Rio de Janeiro",
			"nivel":           "1",
			"ordem_ua_basica": "001",
			"ordem_absoluta":  "001",
			"ordem_relativa":  "001",
		},
		bson.M{
			"cd_ua":           "2000",
			"sigla_ua":        "SMF",
			"nome_ua":         "Secretaria Municipal de Fazenda",
			"nivel":           "2",
			"cd_ua_pai":       "1000",
			"ordem_ua_basica": "002",
			"ordem_absoluta":  "002",
			"ordem_relativa":  "001",
		},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Pagination.Total, "Expected 2 departments")
	assert.Equal(t, int64(2), response.TotalCount, "Expected total count 2")
	assert.Len(t, response.Departments, 2, "Expected 2 departments in array")

	// Verify first department (ordered by nivel, ordem_absoluta)
	assert.Equal(t, "1000", response.Departments[0].CdUA, "Expected CdUA 1000")
	assert.Equal(t, "PCRJ", response.Departments[0].SiglaUA, "Expected sigla_ua PCRJ")
	assert.Equal(t, 1, response.Departments[0].Nivel, "Expected nivel 1")
}

// Test ListDepartments - Pagination
func TestListDepartments_Pagination(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert 15 test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	var departments []interface{}
	for i := 1; i <= 15; i++ {
		departments = append(departments, bson.M{
			"cd_ua":    string(rune(1000 + i)),
			"nome_ua":  "Department " + string(rune(i)),
			"nivel":    "1",
			"sigla_ua": "D" + string(rune(i)),
		})
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	// Test page 1
	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 15, response.Pagination.Total, "Expected 15 total departments")
	assert.Len(t, response.Departments, 10, "Expected 10 departments on page 1")
	assert.Equal(t, 1, response.Pagination.Page, "Expected page 1")
	assert.Equal(t, 2, response.Pagination.TotalPages, "Expected 2 total pages")

	// Test page 2
	req, _ = http.NewRequest("GET", "/departments?page=2&per_page=10", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Len(t, response.Departments, 5, "Expected 5 departments on page 2")
	assert.Equal(t, 2, response.Pagination.Page, "Expected page 2")
}

// Test ListDepartments - Filter by ParentID
func TestListDepartments_FilterByParentID(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments with hierarchy
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Root", "nivel": "1"},
		bson.M{"cd_ua": "2000", "nome_ua": "Child of 1000", "nivel": "2", "cd_ua_pai": "1000"},
		bson.M{"cd_ua": "3000", "nome_ua": "Another child of 1000", "nivel": "2", "cd_ua_pai": "1000"},
		bson.M{"cd_ua": "4000", "nome_ua": "Child of 2000", "nivel": "3", "cd_ua_pai": "2000"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&parent_id=1000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Pagination.Total, "Expected 2 departments with parent_id=1000")
	assert.Len(t, response.Departments, 2, "Expected 2 departments in response")

	// Verify all returned departments have correct parent
	for _, dept := range response.Departments {
		assert.Equal(t, "1000", dept.CdUAPai, "Expected all departments to have parent 1000")
	}
}

// Test ListDepartments - Filter by ExactLevel
func TestListDepartments_FilterByExactLevel(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments at different levels
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Level 1 Dept 1", "nivel": "1"},
		bson.M{"cd_ua": "1001", "nome_ua": "Level 1 Dept 2", "nivel": "1"},
		bson.M{"cd_ua": "2000", "nome_ua": "Level 2 Dept", "nivel": "2"},
		bson.M{"cd_ua": "3000", "nome_ua": "Level 3 Dept", "nivel": "3"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&exact_level=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Pagination.Total, "Expected 2 departments at level 1")
	assert.Len(t, response.Departments, 2, "Expected 2 departments in response")

	// Verify all returned departments are level 1
	for _, dept := range response.Departments {
		assert.Equal(t, 1, dept.Nivel, "Expected all departments to be level 1")
	}
}

// Test ListDepartments - Filter by MinLevel and MaxLevel
func TestListDepartments_FilterByMinMaxLevel(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments at different levels
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Level 1", "nivel": "1"},
		bson.M{"cd_ua": "2000", "nome_ua": "Level 2", "nivel": "2"},
		bson.M{"cd_ua": "3000", "nome_ua": "Level 3", "nivel": "3"},
		bson.M{"cd_ua": "4000", "nome_ua": "Level 4", "nivel": "4"},
		bson.M{"cd_ua": "5000", "nome_ua": "Level 5", "nivel": "5"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&min_level=2&max_level=4", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 3, response.Pagination.Total, "Expected 3 departments in range 2-4")
	assert.Len(t, response.Departments, 3, "Expected 3 departments in response")

	// Verify all returned departments are in range 2-4
	for _, dept := range response.Departments {
		assert.GreaterOrEqual(t, dept.Nivel, 2, "Expected nivel >= 2")
		assert.LessOrEqual(t, dept.Nivel, 4, "Expected nivel <= 4")
	}
}

// Test ListDepartments - Filter by MinLevel only
func TestListDepartments_FilterByMinLevel(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Level 1", "nivel": "1"},
		bson.M{"cd_ua": "2000", "nome_ua": "Level 2", "nivel": "2"},
		bson.M{"cd_ua": "3000", "nome_ua": "Level 3", "nivel": "3"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&min_level=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Pagination.Total, "Expected 2 departments with min_level=2")
}

// Test ListDepartments - Filter by MaxLevel only
func TestListDepartments_FilterByMaxLevel(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Level 1", "nivel": "1"},
		bson.M{"cd_ua": "2000", "nome_ua": "Level 2", "nivel": "2"},
		bson.M{"cd_ua": "3000", "nome_ua": "Level 3", "nivel": "3"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&max_level=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Pagination.Total, "Expected 2 departments with max_level=2")
}

// Test ListDepartments - Filter by SiglaUA
func TestListDepartments_FilterBySiglaUA(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "sigla_ua": "SMF", "nome_ua": "Secretaria de Fazenda", "nivel": "1"},
		bson.M{"cd_ua": "2000", "sigla_ua": "SME", "nome_ua": "Secretaria de Educação", "nivel": "1"},
		bson.M{"cd_ua": "3000", "sigla_ua": "SMS", "nome_ua": "Secretaria de Saúde", "nivel": "1"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&sigla_ua=SMF", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 department with sigla_ua=SMF")
	assert.Len(t, response.Departments, 1, "Expected 1 department in response")
	assert.Equal(t, "SMF", response.Departments[0].SiglaUA, "Expected sigla_ua to be SMF")
	assert.Equal(t, "1000", response.Departments[0].CdUA, "Expected cd_ua to be 1000")
}

// Test ListDepartments - Search by name (case insensitive)
func TestListDepartments_SearchByName(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Secretaria de Fazenda", "nivel": "1"},
		bson.M{"cd_ua": "2000", "nome_ua": "Secretaria de Educação", "nivel": "1"},
		bson.M{"cd_ua": "3000", "nome_ua": "Departamento de Saúde", "nivel": "2"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&search=Secretaria", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Pagination.Total, "Expected 2 departments with 'Secretaria' in name")
	assert.Len(t, response.Departments, 2, "Expected 2 departments in response")
}

// Test ListDepartments - Search case insensitive
func TestListDepartments_SearchCaseInsensitive(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Secretaria de Fazenda", "nivel": "1"},
		bson.M{"cd_ua": "2000", "sigla_ua": "SME", "nome_ua": "Educação", "nivel": "1"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	// Test lowercase search
	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&search=secretaria", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 department with case-insensitive search")
}

// Test ListDepartments - Search by sigla_ua
func TestListDepartments_SearchBySigla(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "sigla_ua": "SMFP", "nome_ua": "Fazenda e Planejamento", "nivel": "1"},
		bson.M{"cd_ua": "2000", "sigla_ua": "SME", "nome_ua": "Educação", "nivel": "1"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&search=SMF", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 department matching sigla search")
	assert.Equal(t, "SMFP", response.Departments[0].SiglaUA, "Expected SMFP sigla")
}

// Test ListDepartments - Combined filters
func TestListDepartments_CombinedFilters(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "1000", "nome_ua": "Root", "nivel": "1"},
		bson.M{"cd_ua": "2000", "nome_ua": "Secretaria A", "nivel": "2", "cd_ua_pai": "1000"},
		bson.M{"cd_ua": "3000", "nome_ua": "Secretaria B", "nivel": "2", "cd_ua_pai": "1000"},
		bson.M{"cd_ua": "4000", "nome_ua": "Departamento X", "nivel": "3", "cd_ua_pai": "2000"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	// Filter by parent_id and search
	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&parent_id=1000&search=Secretaria", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Pagination.Total, "Expected 2 departments matching combined filters")
}

// Test ListDepartments - Invalid pagination parameters
func TestListDepartments_InvalidPagination(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name    string
		page    string
		perPage string
	}{
		{"invalid page", "invalid", "10"},
		{"invalid per_page", "1", "invalid"},
		{"page zero", "0", "10"},
		{"negative page", "-1", "10"},
		{"per_page zero", "1", "0"},
		{"negative per_page", "1", "-10"},
		{"per_page too large", "1", "101"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/departments?page="+tt.page+"&per_page="+tt.perPage, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "Expected BadRequest status for "+tt.name)

			var errorResp ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &errorResp)
			require.NoError(t, err, "Failed to unmarshal error response")
			assert.NotEmpty(t, errorResp.Error, "Expected error message")
		})
	}
}

// Test ListDepartments - Invalid level parameters
func TestListDepartments_InvalidLevelParams(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name  string
		param string
		value string
	}{
		{"invalid exact_level", "exact_level", "invalid"},
		{"invalid min_level", "min_level", "not_a_number"},
		{"invalid max_level", "max_level", "xyz"},
		{"float exact_level", "exact_level", "2.5"},
		{"float min_level", "min_level", "1.5"},
		{"float max_level", "max_level", "3.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&"+tt.param+"="+tt.value, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "Expected BadRequest status for "+tt.name)

			var errorResp ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &errorResp)
			require.NoError(t, err, "Failed to unmarshal error response")
			assert.NotEmpty(t, errorResp.Error, "Expected error message")
		})
	}
}

// Test ListDepartments - Default pagination values
func TestListDepartments_DefaultPagination(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test department
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	_, err := collection.InsertOne(ctx, bson.M{"cd_ua": "1000", "nome_ua": "Test", "nivel": "1"})
	require.NoError(t, err, "Failed to insert department")

	req, _ := http.NewRequest("GET", "/departments", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Page, "Expected default page 1")
	assert.Equal(t, 10, response.Pagination.PerPage, "Expected default per_page 10")
}

// Test ListDepartments - Service unavailable
func TestListDepartments_ServiceUnavailable(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	// Temporarily set service to nil
	originalService := services.DepartmentServiceInstance
	services.DepartmentServiceInstance = nil

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "Expected InternalServerError when service unavailable")

	var errorResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errorResp)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResp.Error, "Department service unavailable", "Expected service unavailable error message")

	// Restore service
	services.DepartmentServiceInstance = originalService
}

// Test GetDepartment - Success
func TestGetDepartment_Success(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test department with all fields
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	now := time.Now()
	msg := "Test message"
	department := bson.M{
		"cd_ua":           "1000",
		"sigla_ua":        "PCRJ",
		"nome_ua":         "Prefeitura da Cidade do Rio de Janeiro",
		"nivel":           "1",
		"cd_ua_pai":       "",
		"ordem_ua_basica": "001",
		"ordem_absoluta":  "001",
		"ordem_relativa":  "001",
		"msg":             msg,
		"updated_at":      now,
	}

	_, err := collection.InsertOne(ctx, department)
	require.NoError(t, err, "Failed to insert department")

	req, _ := http.NewRequest("GET", "/departments/1000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, "1000", response.CdUA, "Expected cd_ua 1000")
	assert.Equal(t, "PCRJ", response.SiglaUA, "Expected sigla_ua PCRJ")
	assert.Equal(t, "Prefeitura da Cidade do Rio de Janeiro", response.NomeUA, "Expected correct nome_ua")
	assert.Equal(t, 1, response.Nivel, "Expected nivel 1")
	assert.Equal(t, "001", response.OrdemUABasica, "Expected ordem_ua_basica 001")
	assert.Equal(t, "001", response.OrdemAbsoluta, "Expected ordem_absoluta 001")
	assert.Equal(t, "001", response.OrdemRelativa, "Expected ordem_relativa 001")
	assert.NotNil(t, response.Msg, "Expected msg to be present")
	assert.Equal(t, msg, *response.Msg, "Expected correct msg value")
	assert.NotNil(t, response.UpdatedAt, "Expected updated_at to be present")
}

// Test GetDepartment - Not Found
func TestGetDepartment_NotFound(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/departments/9999", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "Expected NotFound status")

	var errorResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errorResp)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResp.Error, "Department not found", "Expected 'not found' error message")
}

// Test GetDepartment - Empty cd_ua (should redirect to list)
func TestGetDepartment_EmptyCdUA(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/departments/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Empty cd_ua will redirect or route to list endpoint
	// Accept 200 (list), 301 (redirect), or 404 (not found)
	acceptableStatuses := []int{http.StatusOK, http.StatusNotFound, http.StatusMovedPermanently}
	assert.Contains(t, acceptableStatuses, w.Code, "Expected acceptable status for empty cd_ua")
}

// Test GetDepartment - Special characters in cd_ua
func TestGetDepartment_SpecialCharactersInCdUA(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert department with special characters
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	department := bson.M{
		"cd_ua":    "TEST-001",
		"sigla_ua": "TST",
		"nome_ua":  "Test Department",
		"nivel":    "1",
	}

	_, err := collection.InsertOne(ctx, department)
	require.NoError(t, err, "Failed to insert department")

	req, _ := http.NewRequest("GET", "/departments/TEST-001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status for special characters in cd_ua")

	var response models.DepartmentResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, "TEST-001", response.CdUA, "Expected cd_ua with special characters")
}

// Test GetDepartment - Nivel as integer in database
func TestGetDepartment_NivelAsInteger(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert department with nivel as integer
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	department := bson.M{
		"cd_ua":    "2000",
		"sigla_ua": "SMF",
		"nome_ua":  "Secretaria de Fazenda",
		"nivel":    2, // Integer instead of string
	}

	_, err := collection.InsertOne(ctx, department)
	require.NoError(t, err, "Failed to insert department")

	req, _ := http.NewRequest("GET", "/departments/2000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Nivel, "Expected nivel to be converted to int")
}

// Test GetDepartment - Service unavailable
func TestGetDepartment_ServiceUnavailable(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	// Temporarily set service to nil
	originalService := services.DepartmentServiceInstance
	services.DepartmentServiceInstance = nil

	req, _ := http.NewRequest("GET", "/departments/1000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, "Expected InternalServerError when service unavailable")

	var errorResp ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &errorResp)
	require.NoError(t, err, "Failed to unmarshal error response")
	assert.Contains(t, errorResp.Error, "Department service unavailable", "Expected service unavailable error message")

	// Restore service
	services.DepartmentServiceInstance = originalService
}

// Test GetDepartment - Minimal department data
func TestGetDepartment_MinimalData(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert department with minimal required fields
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	department := bson.M{
		"cd_ua":   "3000",
		"nome_ua": "Minimal Dept",
		"nivel":   "1",
	}

	_, err := collection.InsertOne(ctx, department)
	require.NoError(t, err, "Failed to insert department")

	req, _ := http.NewRequest("GET", "/departments/3000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, "3000", response.CdUA, "Expected cd_ua 3000")
	assert.Equal(t, "Minimal Dept", response.NomeUA, "Expected nome_ua")
	assert.Equal(t, 1, response.Nivel, "Expected nivel 1")
	assert.Nil(t, response.Msg, "Expected nil msg for minimal data")
	assert.Nil(t, response.UpdatedAt, "Expected nil updated_at for minimal data")
}

// Test ListDepartments - Verify sorting order
func TestListDepartments_SortingOrder(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert departments in non-sorted order
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	departments := []interface{}{
		bson.M{"cd_ua": "3000", "nome_ua": "Dept C", "nivel": "2", "ordem_absoluta": "003"},
		bson.M{"cd_ua": "1000", "nome_ua": "Dept A", "nivel": "1", "ordem_absoluta": "001"},
		bson.M{"cd_ua": "4000", "nome_ua": "Dept D", "nivel": "2", "ordem_absoluta": "002"},
		bson.M{"cd_ua": "2000", "nome_ua": "Dept B", "nivel": "1", "ordem_absoluta": "002"},
	}

	_, err := collection.InsertMany(ctx, departments)
	require.NoError(t, err, "Failed to insert departments")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Len(t, response.Departments, 4, "Expected 4 departments")

	// Verify sorted by nivel first (as string), then ordem_absoluta
	assert.Equal(t, "1000", response.Departments[0].CdUA, "First should be nivel 1, ordem 001")
	assert.Equal(t, "2000", response.Departments[1].CdUA, "Second should be nivel 1, ordem 002")
	assert.Equal(t, "4000", response.Departments[2].CdUA, "Third should be nivel 2, ordem 002")
	assert.Equal(t, "3000", response.Departments[3].CdUA, "Fourth should be nivel 2, ordem 003")
}

// Test ListDepartments - No results with filter
func TestListDepartments_NoResultsWithFilter(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test departments
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	_, err := collection.InsertOne(ctx, bson.M{"cd_ua": "1000", "nome_ua": "Test", "nivel": "1"})
	require.NoError(t, err, "Failed to insert department")

	req, _ := http.NewRequest("GET", "/departments?page=1&per_page=10&search=NonExistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status even with no results")

	var response models.DepartmentListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 0, response.Pagination.Total, "Expected 0 departments with non-matching filter")
	assert.Empty(t, response.Departments, "Expected empty departments array")
}

// Test GetDepartment - URL encoding
func TestGetDepartment_URLEncoding(t *testing.T) {
	router, cleanup := setupDepartmentHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert department
	collection := config.MongoDB.Collection(config.AppConfig.DepartmentCollection)
	department := bson.M{
		"cd_ua":    "TEST 001",
		"nome_ua":  "Test Department",
		"nivel":    "1",
		"sigla_ua": "TST",
	}

	_, err := collection.InsertOne(ctx, department)
	require.NoError(t, err, "Failed to insert department")

	// URL encode the space
	req, _ := http.NewRequest("GET", "/departments/TEST%20001", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected OK status with URL encoded cd_ua")

	var response models.DepartmentResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, "TEST 001", response.CdUA, "Expected decoded cd_ua")
}
