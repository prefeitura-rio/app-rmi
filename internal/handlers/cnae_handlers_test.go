package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func setupCNAEHandlersTest(t *testing.T) (*CNAEHandlers, *gin.Engine, func()) {
	// Use the shared MongoDB from common_test.go TestMain
	gin.SetMode(gin.TestMode)

	// Configure test collection
	config.AppConfig.CNAECollection = "test_cnaes"

	ctx := context.Background()
	database := config.MongoDB

	// Create text index for search
	collection := database.Collection(config.AppConfig.CNAECollection)
	// Note: We skip index creation in tests since it requires mongo.IndexModel
	// and we're avoiding importing mongo package
	_ = collection // Silence unused variable

	service := services.NewCNAEService(database, logging.Logger)
	handlers := NewCNAEHandlers(service, logging.Logger)

	router := gin.New()
	router.GET("/cnaes", handlers.ListCNAEs)

	return handlers, router, func() {
		database.Drop(ctx)
	}
}

func TestNewCNAEHandlers(t *testing.T) {
	service := services.NewCNAEService(nil, logging.Logger)
	handlers := NewCNAEHandlers(service, logging.Logger)

	assert.NotNil(t, handlers, "NewCNAEHandlers() returned nil")
	assert.NotNil(t, handlers.service, "NewCNAEHandlers() service is nil")
	assert.NotNil(t, handlers.logger, "NewCNAEHandlers() logger is nil")
}

func TestListCNAEs_Empty(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 0, response.Pagination.Total, "Expected 0 total CNAEs")
	assert.Empty(t, response.CNAEs, "Expected empty CNAEs list")
}

func TestListCNAEs_WithData(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "01011",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Cultivo de arroz",
		},
		bson.M{
			"_id":         "01012",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.12-1",
			"Subclasse":   "0101-1/02",
			"Denominacao": "Cultivo de milho",
		},
		bson.M{
			"_id":         "02011",
			"Secao":       "A",
			"Divisao":     "02",
			"Grupo":       "02.1",
			"Classe":      "02.11-5",
			"Subclasse":   "0201-1/01",
			"Denominacao": "Produção florestal",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 3, response.Pagination.Total, "Expected 3 total CNAEs")
	assert.Len(t, response.CNAEs, 3, "Expected 3 CNAEs in response")
	assert.Equal(t, 1, response.Pagination.Page, "Expected page 1")
	assert.Equal(t, 10, response.Pagination.PerPage, "Expected per_page 10")
	assert.Equal(t, 1, response.Pagination.TotalPages, "Expected 1 total page")
}

func TestListCNAEs_FilterBySecao(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs with different sections
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "a001",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Agricultura A",
		},
		bson.M{
			"_id":         "b001",
			"Secao":       "B",
			"Divisao":     "05",
			"Grupo":       "05.1",
			"Classe":      "05.11-1",
			"Subclasse":   "0501-1/01",
			"Denominacao": "Mineração B",
		},
		bson.M{
			"_id":         "c001",
			"Secao":       "C",
			"Divisao":     "10",
			"Grupo":       "10.1",
			"Classe":      "10.11-2",
			"Subclasse":   "1001-1/01",
			"Denominacao": "Indústria C",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Filter by Secao A
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10&secao=A", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 CNAE with Secao A")
	assert.Len(t, response.CNAEs, 1, "Expected 1 CNAE in response")
	assert.Equal(t, "A", response.CNAEs[0].Secao, "Expected Secao A")
}

func TestListCNAEs_FilterByDivisao(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs with different divisions
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "d01",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Divisão 01",
		},
		bson.M{
			"_id":         "d02",
			"Secao":       "A",
			"Divisao":     "02",
			"Grupo":       "02.1",
			"Classe":      "02.11-5",
			"Subclasse":   "0201-1/01",
			"Denominacao": "Divisão 02",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Filter by Divisao 01
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10&divisao=01", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 CNAE with Divisao 01")
	assert.Len(t, response.CNAEs, 1, "Expected 1 CNAE in response")
	assert.Equal(t, "01", response.CNAEs[0].Divisao, "Expected Divisao 01")
}

func TestListCNAEs_FilterByGrupo(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs with different groups
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "g11",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Grupo 01.1",
		},
		bson.M{
			"_id":         "g12",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.2",
			"Classe":      "01.21-0",
			"Subclasse":   "0102-1/01",
			"Denominacao": "Grupo 01.2",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Filter by Grupo 01.1
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10&grupo=01.1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 CNAE with Grupo 01.1")
	assert.Len(t, response.CNAEs, 1, "Expected 1 CNAE in response")
	assert.Equal(t, "01.1", response.CNAEs[0].Grupo, "Expected Grupo 01.1")
}

func TestListCNAEs_FilterByClasse(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs with different classes
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "c111",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Classe 01.11-3",
		},
		bson.M{
			"_id":         "c121",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.12-1",
			"Subclasse":   "0101-1/02",
			"Denominacao": "Classe 01.12-1",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Filter by Classe 01.11-3
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10&classe=01.11-3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 CNAE with Classe 01.11-3")
	assert.Len(t, response.CNAEs, 1, "Expected 1 CNAE in response")
	assert.Equal(t, "01.11-3", response.CNAEs[0].Classe, "Expected Classe 01.11-3")
}

func TestListCNAEs_FilterBySubclasse(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs with different subclasses
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "s01",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Subclasse 0101-1/01",
		},
		bson.M{
			"_id":         "s02",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/02",
			"Denominacao": "Subclasse 0101-1/02",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Filter by Subclasse 0101-1/01
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10&subclasse=0101-1/01", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 CNAE with Subclasse 0101-1/01")
	assert.Len(t, response.CNAEs, 1, "Expected 1 CNAE in response")
	assert.Equal(t, "0101-1/01", response.CNAEs[0].Subclasse, "Expected Subclasse 0101-1/01")
}

func TestListCNAEs_TextSearch(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs with searchable denominations
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "search01",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Cultivo de arroz",
		},
		bson.M{
			"_id":         "search02",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.12-1",
			"Subclasse":   "0101-1/02",
			"Denominacao": "Cultivo de milho",
		},
		bson.M{
			"_id":         "search03",
			"Secao":       "C",
			"Divisao":     "10",
			"Grupo":       "10.1",
			"Classe":      "10.11-2",
			"Subclasse":   "1001-1/01",
			"Denominacao": "Fabricação de produtos alimentícios",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Search for "cultivo"
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10&search=cultivo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 2, response.Pagination.Total, "Expected 2 CNAEs matching 'cultivo'")
	assert.Len(t, response.CNAEs, 2, "Expected 2 CNAEs in response")
}

func TestListCNAEs_Pagination(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert 25 CNAEs to test pagination
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	var cnaes []interface{}
	for i := 1; i <= 25; i++ {
		cnaes = append(cnaes, bson.M{
			"_id":         bson.M{},
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      bson.M{},
			"Subclasse":   bson.M{},
			"Denominacao": bson.M{},
		})
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Test page 1
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 25, response.Pagination.Total, "Expected 25 total CNAEs")
	assert.Len(t, response.CNAEs, 10, "Expected 10 CNAEs on page 1")
	assert.Equal(t, 3, response.Pagination.TotalPages, "Expected 3 total pages")

	// Test page 2
	req, _ = http.NewRequest("GET", "/cnaes?page=2&per_page=10", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 25, response.Pagination.Total, "Expected 25 total CNAEs")
	assert.Len(t, response.CNAEs, 10, "Expected 10 CNAEs on page 2")
	assert.Equal(t, 2, response.Pagination.Page, "Expected page 2")

	// Test page 3 (last page with fewer items)
	req, _ = http.NewRequest("GET", "/cnaes?page=3&per_page=10", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 25, response.Pagination.Total, "Expected 25 total CNAEs")
	assert.Len(t, response.CNAEs, 5, "Expected 5 CNAEs on page 3")
	assert.Equal(t, 3, response.Pagination.Page, "Expected page 3")
}

func TestListCNAEs_InvalidPagination(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	testCases := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{
			name:       "Invalid page - negative",
			url:        "/cnaes?page=-1&per_page=10",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid page - zero",
			url:        "/cnaes?page=0&per_page=10",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid page - non-numeric",
			url:        "/cnaes?page=abc&per_page=10",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid per_page - negative",
			url:        "/cnaes?page=1&per_page=-10",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid per_page - zero",
			url:        "/cnaes?page=1&per_page=0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid per_page - too large",
			url:        "/cnaes?page=1&per_page=101",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Invalid per_page - non-numeric",
			url:        "/cnaes?page=1&per_page=xyz",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tc.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code, "Expected status %d for %s", tc.wantStatus, tc.name)
		})
	}
}

func TestListCNAEs_DefaultPagination(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	var cnaes []interface{}
	for i := 1; i <= 15; i++ {
		cnaes = append(cnaes, bson.M{
			"_id":         bson.M{},
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      bson.M{},
			"Subclasse":   bson.M{},
			"Denominacao": bson.M{},
		})
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Test without pagination params (should use defaults)
	req, _ := http.NewRequest("GET", "/cnaes", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 15, response.Pagination.Total, "Expected 15 total CNAEs")
	assert.Equal(t, 1, response.Pagination.Page, "Expected default page 1")
	assert.Equal(t, 10, response.Pagination.PerPage, "Expected default per_page 10")
	assert.Len(t, response.CNAEs, 10, "Expected 10 CNAEs with default per_page")
}

func TestListCNAEs_MultipleFilters(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "multi01",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Agricultura A",
		},
		bson.M{
			"_id":         "multi02",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.2",
			"Classe":      "01.21-0",
			"Subclasse":   "0102-1/01",
			"Denominacao": "Agricultura B",
		},
		bson.M{
			"_id":         "multi03",
			"Secao":       "B",
			"Divisao":     "05",
			"Grupo":       "05.1",
			"Classe":      "05.11-1",
			"Subclasse":   "0501-1/01",
			"Denominacao": "Mineração",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Filter by Secao A and Divisao 01 and Grupo 01.1
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10&secao=A&divisao=01&grupo=01.1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Pagination.Total, "Expected 1 CNAE matching all filters")
	assert.Len(t, response.CNAEs, 1, "Expected 1 CNAE in response")
	assert.Equal(t, "A", response.CNAEs[0].Secao, "Expected Secao A")
	assert.Equal(t, "01", response.CNAEs[0].Divisao, "Expected Divisao 01")
	assert.Equal(t, "01.1", response.CNAEs[0].Grupo, "Expected Grupo 01.1")
}

func TestListCNAEs_NoResults(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "no01",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Agricultura",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Filter with non-existent values
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10&secao=Z", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 0, response.Pagination.Total, "Expected 0 total CNAEs")
	assert.Empty(t, response.CNAEs, "Expected empty CNAEs list")
}

func TestListCNAEs_SortOrder(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs in random order
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "sort03",
			"Secao":       "A",
			"Divisao":     "03",
			"Grupo":       "03.1",
			"Classe":      "03.11-5",
			"Subclasse":   "0301-1/01",
			"Denominacao": "Terceiro",
		},
		bson.M{
			"_id":         "sort01",
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "01.1",
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Primeiro",
		},
		bson.M{
			"_id":         "sort02",
			"Secao":       "A",
			"Divisao":     "02",
			"Grupo":       "02.1",
			"Classe":      "02.11-5",
			"Subclasse":   "0201-1/01",
			"Denominacao": "Segundo",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAEs")

	// Get all CNAEs
	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Len(t, response.CNAEs, 3, "Expected 3 CNAEs")

	// Verify results are sorted by Classe
	assert.Equal(t, "01.11-3", response.CNAEs[0].Classe, "Expected first CNAE to have Classe 01.11-3")
	assert.Equal(t, "02.11-5", response.CNAEs[1].Classe, "Expected second CNAE to have Classe 02.11-5")
	assert.Equal(t, "03.11-5", response.CNAEs[2].Classe, "Expected third CNAE to have Classe 03.11-5")
}

func TestListCNAEs_TypeConversion(t *testing.T) {
	_, router, cleanup := setupCNAEHandlersTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert CNAE with numeric fields (testing type conversion in service)
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"_id":         "type01",
			"Secao":       "A",
			"Divisao":     1,   // numeric instead of string
			"Grupo":       1.1, // numeric instead of string
			"Classe":      "01.11-3",
			"Subclasse":   "0101-1/01",
			"Denominacao": "Test Type Conversion",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	require.NoError(t, err, "Failed to insert test CNAE")

	req, _ := http.NewRequest("GET", "/cnaes?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.CNAEListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Len(t, response.CNAEs, 1, "Expected 1 CNAE")
	assert.Equal(t, "1", response.CNAEs[0].Divisao, "Expected Divisao to be converted to string '1'")
	assert.Equal(t, "1.1", response.CNAEs[0].Grupo, "Expected Grupo to be converted to string '1.1'")
}
