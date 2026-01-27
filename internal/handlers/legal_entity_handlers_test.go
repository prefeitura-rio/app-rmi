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

func setupLegalEntityHandlersTest(t *testing.T) (*gin.Engine, func()) {
	// Use the shared MongoDB from common_test.go TestMain
	gin.SetMode(gin.TestMode)

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.LegalEntityCollection = "test_legal_entities"

	ctx := context.Background()
	database := config.MongoDB

	// Initialize global legal entity service instance
	services.LegalEntityServiceInstance = services.NewLegalEntityService(database, logging.Logger)

	router := gin.New()
	router.GET("/citizen/:cpf/legal-entities", GetLegalEntities)
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	return router, func() {
		_ = database.Drop(ctx)
		services.LegalEntityServiceInstance = nil
	}
}

// Helper function to create admin middleware
func adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "03561350712",
		}
		claims.ResourceAccess.Superapp.Roles = []string{"go:admin"}
		c.Set("claims", claims)
		c.Next()
	}
}

// Helper function to create user middleware with specific CPF
func userMiddleware(cpf string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: cpf,
		}
		claims.ResourceAccess.Superapp.Roles = []string{"user"}
		c.Set("claims", claims)
		c.Next()
	}
}

// Helper function to create test legal entity
//
//nolint:unused // Keeping for potential future use
func createTestLegalEntity(cnpj, companyName, responsibleCPF string, partners []string) bson.M {
	entity := bson.M{
		"cnpj":            cnpj,
		"razao_social":    companyName,
		"responsavel_cpf": responsibleCPF,
		"socios":          []bson.M{},
	}

	var partnerDocs []bson.M
	for _, partnerCPF := range partners {
		partnerDocs = append(partnerDocs, bson.M{"cpf_socio": partnerCPF})
	}
	entity["socios"] = partnerDocs

	return entity
}

// Test GetLegalEntities handler

func TestGetLegalEntities_Success_Empty(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PaginatedLegalEntities
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Pagination.Total)
	assert.Equal(t, 0, len(response.Data))
	assert.Equal(t, 1, response.Pagination.Page)
	assert.Equal(t, 10, response.Pagination.PerPage)
}

func TestGetLegalEntities_Success_WithData(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test legal entities
	entities := []interface{}{
		bson.M{
			"cnpj":         "12345678000199",
			"razao_social": "Empresa A LTDA",
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		},
		bson.M{
			"cnpj":         "98765432000188",
			"razao_social": "Empresa B SA",
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		},
	}

	_, err := collection.InsertMany(ctx, entities)
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PaginatedLegalEntities
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.Pagination.Total)
	assert.Equal(t, 2, len(response.Data))
	assert.Equal(t, 1, response.Pagination.Page)
	assert.Equal(t, 10, response.Pagination.PerPage)
	assert.Equal(t, 1, response.Pagination.TotalPages)
}

func TestGetLegalEntities_InvalidCPF(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name string
		cpf  string
	}{
		{"empty CPF", ""},
		{"short CPF", "12345"},
		{"letters in CPF", "abcdefghijk"},
		{"invalid check digits", "11111111111"},
		{"too long", "123456789012345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/citizen/"+tt.cpf+"/legal-entities?page=1&per_page=10", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "Expected bad request for %s", tt.name)
		})
	}
}

func TestGetLegalEntities_InvalidPagination(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	tests := []struct {
		name    string
		page    string
		perPage string
	}{
		{"invalid page string", "invalid", "10"},
		{"page zero", "0", "10"},
		{"negative page", "-1", "10"},
		{"invalid per_page string", "1", "invalid"},
		{"per_page zero", "1", "0"},
		{"per_page too large", "1", "101"},
		{"negative per_page", "1", "-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page="+tt.page+"&per_page="+tt.perPage, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "Expected bad request for %s", tt.name)
		})
	}
}

func TestGetLegalEntities_DefaultPagination(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PaginatedLegalEntities
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Default pagination should be page=1, per_page=10
	assert.Equal(t, 1, response.Pagination.Page)
	assert.Equal(t, 10, response.Pagination.PerPage)
}

func TestGetLegalEntities_WithFilter(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test legal entities with different legal natures
	entities := []interface{}{
		bson.M{
			"cnpj":              "12345678000199",
			"razao_social":      "Empresa A",
			"natureza_juridica": bson.M{"id": "2062", "descricao": "LTDA"},
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		},
		bson.M{
			"cnpj":              "98765432000188",
			"razao_social":      "Empresa B",
			"natureza_juridica": bson.M{"id": "2070", "descricao": "SA"},
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		},
		bson.M{
			"cnpj":              "11222333000144",
			"razao_social":      "Empresa C",
			"natureza_juridica": bson.M{"id": "2062", "descricao": "LTDA"},
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		},
	}

	_, err := collection.InsertMany(ctx, entities)
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=1&per_page=10&natureza_juridica_id=2062", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PaginatedLegalEntities
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.Pagination.Total, "Should only return entities with legal nature 2062")
	assert.Equal(t, 2, len(response.Data))
}

func TestGetLegalEntities_Pagination(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert 15 legal entities
	var entities []interface{}
	for i := 1; i <= 15; i++ {
		cnpj := "1234567800019" + string(rune('0'+i%10))
		if i >= 10 {
			cnpj = "1234567800018" + string(rune('0'+(i-10)))
		}
		entities = append(entities, bson.M{
			"cnpj":         cnpj,
			"razao_social": "Empresa " + string(rune('A'+i-1)),
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		})
	}

	_, err := collection.InsertMany(ctx, entities)
	require.NoError(t, err)

	// Test first page
	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=1&per_page=5", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PaginatedLegalEntities
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 15, response.Pagination.Total)
	assert.Equal(t, 5, len(response.Data))
	assert.Equal(t, 1, response.Pagination.Page)
	assert.Equal(t, 5, response.Pagination.PerPage)
	assert.Equal(t, 3, response.Pagination.TotalPages)

	// Test second page
	req, _ = http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=2&per_page=5", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 15, response.Pagination.Total)
	assert.Equal(t, 5, len(response.Data))
	assert.Equal(t, 2, response.Pagination.Page)
}

func TestGetLegalEntities_ServiceUnavailable(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	// Temporarily set service to nil to simulate service unavailability
	originalService := services.LegalEntityServiceInstance
	services.LegalEntityServiceInstance = nil
	defer func() {
		services.LegalEntityServiceInstance = originalService
	}()

	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response.Error, "Legal entity service unavailable")
}

// Test GetLegalEntityByCNPJ handler

func TestGetLegalEntityByCNPJ_Success_AdminAccess(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Test Company Admin",
		"responsavel": bson.M{
			"cpf": "99999999999",
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router with admin middleware
	router := gin.New()
	router.Use(adminMiddleware())
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.LegalEntity
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "11222333000181", response.CNPJ)
	assert.Equal(t, "Test Company Admin", response.CompanyName)
}

func TestGetLegalEntityByCNPJ_Success_ResponsiblePersonAccess(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Test Company Responsible",
		"responsavel": bson.M{
			"cpf": "03561350712",
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router with user middleware
	router := gin.New()
	router.Use(userMiddleware("03561350712"))
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.LegalEntity
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "11222333000181", response.CNPJ)
	assert.Equal(t, "Test Company Responsible", response.CompanyName)
}

func TestGetLegalEntityByCNPJ_Success_PartnerAccess(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity
	partnerCPF := "03561350712"
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Test Company Partner",
		"responsavel": bson.M{
			"cpf": "99999999999",
		},
		"socios": []bson.M{
			{"cpf_socio": &partnerCPF},
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router with user middleware
	router := gin.New()
	router.Use(userMiddleware("03561350712"))
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.LegalEntity
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "11222333000181", response.CNPJ)
	assert.Equal(t, "Test Company Partner", response.CompanyName)
}

func TestGetLegalEntityByCNPJ_InvalidCNPJ(t *testing.T) {
	router := gin.New()
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	tests := []struct {
		name string
		cnpj string
	}{
		{"invalid format short", "12345"},
		{"letters in CNPJ", "abcdefghijklmn"},
		{"invalid check digits", "00000000000000"},
		{"all same digits", "11111111111111"},
		{"too long", "123456789012345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/legal-entity/"+tt.cnpj, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code, "Expected bad request for %s", tt.name)
		})
	}
}

func TestGetLegalEntityByCNPJ_NotFound(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	// Create router with user middleware
	router := gin.New()
	router.Use(userMiddleware("03561350712"))
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	// Use a valid CNPJ format that doesn't exist in DB
	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response.Error, "Legal entity not found")
}

func TestGetLegalEntityByCNPJ_NoAuthClaims(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Test Entity",
		"responsavel": bson.M{
			"cpf": "03561350712",
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router without claims middleware
	router := gin.New()
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetLegalEntityByCNPJ_InvalidClaimsType(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Test Entity",
		"responsavel": bson.M{
			"cpf": "03561350712",
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router with invalid claims type
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("claims", "invalid_claims_type")
		c.Next()
	})
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGetLegalEntityByCNPJ_UnauthorizedAccess(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Test Entity",
		"responsavel": bson.M{
			"cpf": "99999999999", // Different user
		},
		"socios": []bson.M{
			{"cpf_socio": stringPtr("88888888888")}, // User not a partner
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router with unauthorized user claims
	router := gin.New()
	router.Use(userMiddleware("03561350712"))
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var response ErrorResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response.Error, "Access denied")
}

func TestGetLegalEntityByCNPJ_MissingCPFInToken(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Test Entity",
		"responsavel": bson.M{
			"cpf": "03561350712",
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router with claims missing CPF
	router := gin.New()
	router.Use(func(c *gin.Context) {
		claims := &models.JWTClaims{
			PreferredUsername: "", // Empty CPF
		}
		claims.ResourceAccess.Superapp.Roles = []string{"user"}
		c.Set("claims", claims)
		c.Next()
	})
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetLegalEntityByCNPJ_ServiceUnavailable(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	// Create router with user middleware
	router := gin.New()
	router.Use(userMiddleware("03561350712"))
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	// Temporarily set service to nil to simulate service unavailability
	originalService := services.LegalEntityServiceInstance
	services.LegalEntityServiceInstance = nil
	defer func() {
		services.LegalEntityServiceInstance = originalService
	}()

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response.Error, "Legal entity service unavailable")
}

// Additional comprehensive tests

func TestGetLegalEntities_MultiplePartnersInEntity(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert entity with multiple partners
	entity := bson.M{
		"cnpj":         "12345678000199",
		"razao_social": "Multi Partner Company",
		"socios": []bson.M{
			{"cpf_socio": "03561350712"},
			{"cpf_socio": "99999999999"},
			{"cpf_socio": "88888888888"},
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PaginatedLegalEntities
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 1, response.Pagination.Total)
	assert.Equal(t, 1, len(response.Data))
}

func TestGetLegalEntities_CPFNotInAnyEntity(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert entities not containing the search CPF
	entities := []interface{}{
		bson.M{
			"cnpj":         "12345678000199",
			"razao_social": "Company A",
			"socios": []bson.M{
				{"cpf_socio": "99999999999"},
			},
		},
		bson.M{
			"cnpj":         "98765432000188",
			"razao_social": "Company B",
			"socios": []bson.M{
				{"cpf_socio": "88888888888"},
			},
		},
	}

	_, err := collection.InsertMany(ctx, entities)
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PaginatedLegalEntities
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Pagination.Total)
	assert.Equal(t, 0, len(response.Data))
}

func TestGetLegalEntityByCNPJ_PartnerWithNilCPF(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity with partner having nil CPF
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Test Company Nil Partner",
		"responsavel": bson.M{
			"cpf": "99999999999",
		},
		"socios": []bson.M{
			{"cpf_socio": nil}, // Partner with nil CPF
			{"cpf_socio": stringPtr("88888888888")},
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router with user middleware
	router := gin.New()
	router.Use(userMiddleware("03561350712"))
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should deny access since user is not in partners list
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetLegalEntities_LargeDataset(t *testing.T) {
	router, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert 50 legal entities
	var entities []interface{}
	for i := 1; i <= 50; i++ {
		entities = append(entities, bson.M{
			"cnpj":         generateCNPJ(i),
			"razao_social": "Company " + string(rune('A'+(i%26))),
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		})
	}

	_, err := collection.InsertMany(ctx, entities)
	require.NoError(t, err)

	req, _ := http.NewRequest("GET", "/citizen/03561350712/legal-entities?page=1&per_page=100", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.PaginatedLegalEntities
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 50, response.Pagination.Total)
	// per_page is capped at 100, so all 50 should be returned
	assert.Equal(t, 50, len(response.Data))
}

func TestGetLegalEntityByCNPJ_MultiplePartners_OnlyOneMatch(t *testing.T) {
	_, cleanup := setupLegalEntityHandlersTest(t)
	defer cleanup()

	ctx := context.Background()
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)

	// Insert test entity with multiple partners, one matching user
	entity := bson.M{
		"cnpj":         "11222333000181",
		"razao_social": "Multi Partner Company",
		"responsavel": bson.M{
			"cpf": "99999999999",
		},
		"socios": []bson.M{
			{"cpf_socio": stringPtr("88888888888")},
			{"cpf_socio": stringPtr("03561350712")}, // User is here
			{"cpf_socio": stringPtr("77777777777")},
		},
	}

	_, err := collection.InsertOne(ctx, entity)
	require.NoError(t, err)

	// Create router with user middleware
	router := gin.New()
	router.Use(userMiddleware("03561350712"))
	router.GET("/legal-entity/:cnpj", GetLegalEntityByCNPJ)

	req, _ := http.NewRequest("GET", "/legal-entity/11222333000181", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response models.LegalEntity
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "11222333000181", response.CNPJ)
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

func generateCNPJ(seed int) string {
	// Simple CNPJ generator for testing
	// Format: base number with seed appended
	// Note: These are not validated CNPJs, just for test data
	return fmt.Sprintf("123456780001%02d", seed%100)
}
