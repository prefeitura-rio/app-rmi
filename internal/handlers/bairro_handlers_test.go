package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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

func setupBairroHandlersTest(t *testing.T) (*BairroHandlers, *gin.Engine, func()) {
	if config.MongoDB == nil {
		t.Skip("Skipping bairro handler tests: MongoDB not initialized")
	}

	gin.SetMode(gin.TestMode)

	// Use a dedicated test collection
	config.AppConfig.BairroCollection = "test_bairros"

	ctx := context.Background()
	database := config.MongoDB

	// Drop collection before test for fresh state
	_ = database.Collection(config.AppConfig.BairroCollection).Drop(ctx)

	cleanup := func() {
		_ = database.Collection(config.AppConfig.BairroCollection).Drop(ctx)
	}

	service := services.NewBairroService(database, logging.GetLogger())
	h := NewBairroHandlers(service, logging.GetLogger())

	router := gin.New()
	router.GET("/bairros", h.ListBairros)

	return h, router, cleanup
}

func TestNewBairroHandlers(t *testing.T) {
	service := services.NewBairroService(nil, logging.GetLogger())
	h := NewBairroHandlers(service, logging.GetLogger())

	assert.NotNil(t, h, "NewBairroHandlers() returned nil")
	assert.NotNil(t, h.service, "NewBairroHandlers() service is nil")
	assert.NotNil(t, h.logger, "NewBairroHandlers() logger is nil")
}

func TestListBairros_Empty(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/bairros", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, int64(0), response.Total, "Expected 0 total bairros")
	assert.Empty(t, response.Bairros, "Expected empty bairros list")
	assert.Equal(t, 1, response.Page, "Expected page 1")
	assert.Equal(t, 50, response.Limit, "Expected default limit 50")
}

func TestListBairros_WithData(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.BairroCollection)

	bairros := []interface{}{
		bson.M{"id_bairro": "001", "nome": "Copacabana", "subprefeitura": "Subprefeitura da Zona Sul"},
		bson.M{"id_bairro": "002", "nome": "Ipanema", "subprefeitura": "Subprefeitura da Zona Sul"},
		bson.M{"id_bairro": "003", "nome": "Leblon", "subprefeitura": "Subprefeitura da Zona Sul"},
	}

	_, err := collection.InsertMany(ctx, bairros)
	require.NoError(t, err, "Failed to insert test bairros")

	req, _ := http.NewRequest("GET", "/bairros", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, int64(3), response.Total, "Expected 3 total bairros")
	assert.Len(t, response.Bairros, 3, "Expected 3 bairros in response")
	assert.Equal(t, 1, response.Page, "Expected page 1")
	assert.Equal(t, 50, response.Limit, "Expected default limit 50")
}

func TestListBairros_ResponseFields(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.BairroCollection)

	_, err := collection.InsertOne(ctx, bson.M{"id_bairro": "001", "nome": "Copacabana", "subprefeitura": "Subprefeitura da Zona Sul"})
	require.NoError(t, err, "Failed to insert test bairro")

	req, _ := http.NewRequest("GET", "/bairros", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	require.Len(t, response.Bairros, 1, "Expected 1 bairro")
	assert.Equal(t, "001", response.Bairros[0].ID, "Expected id '001'")
	assert.Equal(t, "Copacabana", response.Bairros[0].Nome, "Expected nome 'Copacabana'")
	assert.Equal(t, "Subprefeitura da Zona Sul", response.Bairros[0].Subprefeitura, "Expected subprefeitura 'Subprefeitura da Zona Sul'")
}

func TestListBairros_CustomPagination(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.BairroCollection)

	var bairros []interface{}
	for i := 1; i <= 10; i++ {
		bairros = append(bairros, bson.M{
			"id_bairro": fmt.Sprintf("%03d", i),
			"nome":      fmt.Sprintf("Bairro %d", i),
		})
	}
	_, err := collection.InsertMany(ctx, bairros)
	require.NoError(t, err, "Failed to insert test bairros")

	// Request page 2 with limit 3
	req, _ := http.NewRequest("GET", "/bairros?page=2&limit=3", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, int64(10), response.Total, "Expected 10 total bairros")
	assert.Len(t, response.Bairros, 3, "Expected 3 bairros on page 2")
	assert.Equal(t, 2, response.Page, "Expected page 2")
	assert.Equal(t, 3, response.Limit, "Expected limit 3")
}

func TestListBairros_SearchFilter(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.BairroCollection)

	bairros := []interface{}{
		bson.M{"id_bairro": "001", "nome": "Copacabana"},
		bson.M{"id_bairro": "002", "nome": "Ipanema"},
		bson.M{"id_bairro": "003", "nome": "Copa do Mundo"},
	}
	_, err := collection.InsertMany(ctx, bairros)
	require.NoError(t, err, "Failed to insert test bairros")

	req, _ := http.NewRequest("GET", "/bairros?search=Copa", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, int64(2), response.Total, "Expected 2 bairros matching 'Copa'")
	assert.Len(t, response.Bairros, 2, "Expected 2 bairros in response")
}

func TestListBairros_SearchCaseInsensitive(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.BairroCollection)

	bairros := []interface{}{
		bson.M{"id_bairro": "001", "nome": "Copacabana"},
		bson.M{"id_bairro": "002", "nome": "Ipanema"},
	}
	_, err := collection.InsertMany(ctx, bairros)
	require.NoError(t, err, "Failed to insert test bairros")

	// Search with lowercase
	req, _ := http.NewRequest("GET", "/bairros?search=copacabana", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, int64(1), response.Total, "Expected 1 bairro matching case-insensitive search")
	assert.Len(t, response.Bairros, 1, "Expected 1 bairro in response")
	assert.Equal(t, "Copacabana", response.Bairros[0].Nome, "Expected Copacabana")
}

func TestListBairros_SearchNoResults(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.BairroCollection)

	_, err := collection.InsertOne(ctx, bson.M{"id_bairro": "001", "nome": "Copacabana"})
	require.NoError(t, err, "Failed to insert test bairro")

	req, _ := http.NewRequest("GET", "/bairros?search=NonExistentBairro", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, int64(0), response.Total, "Expected 0 total bairros")
	assert.Empty(t, response.Bairros, "Expected empty bairros list")
}

func TestListBairros_InvalidPagination(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	testCases := []struct {
		name       string
		url        string
		wantStatus int
	}{
		{
			name:       "Negative page",
			url:        "/bairros?page=-1",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Zero page",
			url:        "/bairros?page=0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Non-numeric page",
			url:        "/bairros?page=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Limit exceeds 100",
			url:        "/bairros?limit=101",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Zero limit",
			url:        "/bairros?limit=0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Negative limit",
			url:        "/bairros?limit=-5",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Non-numeric limit",
			url:        "/bairros?limit=xyz",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tc.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tc.wantStatus, w.Code, "Expected status %d for %s", tc.wantStatus, tc.name)

			var errResp ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &errResp)
			require.NoError(t, err, "Failed to unmarshal error response")
			assert.NotEmpty(t, errResp.Error, "Expected error message")
		})
	}
}

func TestListBairros_DefaultPagination(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/bairros", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 1, response.Page, "Expected default page 1")
	assert.Equal(t, 50, response.Limit, "Expected default limit 50")
}

func TestListBairros_MaxLimit(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/bairros?limit=100", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200 for limit=100 (max allowed)")

	var response models.BairroListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	assert.Equal(t, 100, response.Limit, "Expected limit 100")
}

func TestListBairros_AlphabeticalOrdering(t *testing.T) {
	_, router, cleanup := setupBairroHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.BairroCollection)

	// Insert bairros in non-alphabetical order
	bairros := []interface{}{
		bson.M{"id_bairro": "003", "nome": "Zebra", "subprefeitura": "Test"},
		bson.M{"id_bairro": "001", "nome": "Alpha", "subprefeitura": "Test"},
		bson.M{"id_bairro": "004", "nome": "Delta", "subprefeitura": "Test"},
		bson.M{"id_bairro": "002", "nome": "Bravo", "subprefeitura": "Test"},
	}
	_, err := collection.InsertMany(ctx, bairros)
	require.NoError(t, err, "Failed to insert test bairros")

	req, _ := http.NewRequest("GET", "/bairros", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Expected status 200")

	var response models.BairroListResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err, "Failed to unmarshal response")

	require.Len(t, response.Bairros, 4, "Expected 4 bairros")

	// Verify alphabetical ordering
	assert.Equal(t, "Alpha", response.Bairros[0].Nome, "Expected first bairro to be Alpha")
	assert.Equal(t, "Bravo", response.Bairros[1].Nome, "Expected second bairro to be Bravo")
	assert.Equal(t, "Delta", response.Bairros[2].Nome, "Expected third bairro to be Delta")
	assert.Equal(t, "Zebra", response.Bairros[3].Nome, "Expected fourth bairro to be Zebra")
}
