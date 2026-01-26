package services

import (
	"context"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.mongodb.org/mongo-driver/bson"
)

func setupLegalEntityServiceTest(t *testing.T) (*LegalEntityService, func()) {
	if config.MongoDB == nil {
		t.Skip("Skipping legal entity service tests: MongoDB not initialized")
	}

	_ = logging.InitLogger()

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.LegalEntityCollection = "test_legal_entities"

	service := NewLegalEntityService(config.MongoDB, logging.Logger)

	return service, func() {
		ctx := context.Background()
		collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)
		_ = collection.Drop(ctx)
	}
}

func TestNewLegalEntityService(t *testing.T) {
	if config.MongoDB == nil {
		t.Skip("Skipping: MongoDB not initialized")
	}

	service := NewLegalEntityService(config.MongoDB, logging.Logger)

	if service == nil {
		t.Error("NewLegalEntityService() returned nil")
	}
}

func TestGetLegalEntitiesByCPF_Empty(t *testing.T) {
	service, cleanup := setupLegalEntityServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	result, err := service.GetLegalEntitiesByCPF(ctx, "03561350712", 1, 10, nil)
	if err != nil {
		t.Errorf("GetLegalEntitiesByCPF() error = %v", err)
		return
	}

	if result == nil {
		t.Error("GetLegalEntitiesByCPF() returned nil result")
		return
	}

	if result.Pagination.Total != 0 {
		t.Errorf("GetLegalEntitiesByCPF() Total = %v, want 0", result.Pagination.Total)
	}
}

func TestGetLegalEntitiesByCPF_WithData(t *testing.T) {
	service, cleanup := setupLegalEntityServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test legal entities
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)
	entities := []interface{}{
		bson.M{
			"cnpj":         "12345678000199",
			"razao_social": "Company A",
			"socios": []bson.M{
				{"cpf_socio": "03561350712", "nome_socio": "João Silva"},
			},
		},
		bson.M{
			"cnpj":         "98765432000188",
			"razao_social": "Company B",
			"socios": []bson.M{
				{"cpf_socio": "03561350712", "nome_socio": "João Silva"},
			},
		},
	}

	_, err := collection.InsertMany(ctx, entities)
	if err != nil {
		t.Fatalf("Failed to insert legal entities: %v", err)
	}

	result, err := service.GetLegalEntitiesByCPF(ctx, "03561350712", 1, 10, nil)
	if err != nil {
		t.Errorf("GetLegalEntitiesByCPF() error = %v", err)
	}

	if result.Pagination.Total != 2 {
		t.Errorf("GetLegalEntitiesByCPF() Total = %v, want 2", result.Pagination.Total)
	}

	if len(result.Data) != 2 {
		t.Errorf("GetLegalEntitiesByCPF() len(Data) = %v, want 2", len(result.Data))
	}
}

func TestGetLegalEntitiesByCPF_WithFilter(t *testing.T) {
	service, cleanup := setupLegalEntityServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert entities with different legal natures
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)
	nature1 := "2062"
	entities := []interface{}{
		bson.M{
			"cnpj":         "12345678000199",
			"razao_social": "Company A",
			"natureza_juridica": bson.M{
				"id": nature1,
			},
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		},
		bson.M{
			"cnpj":         "98765432000188",
			"razao_social": "Company B",
			"natureza_juridica": bson.M{
				"id": "2070",
			},
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		},
	}

	_, err := collection.InsertMany(ctx, entities)
	if err != nil {
		t.Fatalf("Failed to insert legal entities: %v", err)
	}

	// Filter by legal nature
	result, err := service.GetLegalEntitiesByCPF(ctx, "03561350712", 1, 10, &nature1)
	if err != nil {
		t.Errorf("GetLegalEntitiesByCPF() error = %v", err)
	}

	if result.Pagination.Total != 1 {
		t.Errorf("GetLegalEntitiesByCPF() with filter Total = %v, want 1", result.Pagination.Total)
	}
}

func TestGetLegalEntitiesByCPF_Pagination(t *testing.T) {
	service, cleanup := setupLegalEntityServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert 5 entities
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)
	var entities []interface{}
	for i := 1; i <= 5; i++ {
		entities = append(entities, bson.M{
			"cnpj":         "1234567800019" + string(rune('0'+i)),
			"razao_social": "Company " + string(rune('A'+i-1)),
			"socios": []bson.M{
				{"cpf_socio": "03561350712"},
			},
		})
	}

	_, err := collection.InsertMany(ctx, entities)
	if err != nil {
		t.Fatalf("Failed to insert legal entities: %v", err)
	}

	// Page 1
	result, err := service.GetLegalEntitiesByCPF(ctx, "03561350712", 1, 2, nil)
	if err != nil {
		t.Errorf("GetLegalEntitiesByCPF() page 1 error = %v", err)
	}

	if len(result.Data) != 2 {
		t.Errorf("GetLegalEntitiesByCPF() page 1 len(Data) = %v, want 2", len(result.Data))
	}

	if result.Pagination.TotalPages != 3 {
		t.Errorf("GetLegalEntitiesByCPF() TotalPages = %v, want 3", result.Pagination.TotalPages)
	}
}

func TestGetLegalEntityByCNPJ_Success(t *testing.T) {
	service, cleanup := setupLegalEntityServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test entity
	collection := config.MongoDB.Collection(config.AppConfig.LegalEntityCollection)
	entity := bson.M{
		"cnpj":         "12345678000199",
		"razao_social": "Test Company",
	}

	_, err := collection.InsertOne(ctx, entity)
	if err != nil {
		t.Fatalf("Failed to insert legal entity: %v", err)
	}

	result, err := service.GetLegalEntityByCNPJ(ctx, "12345678000199")
	if err != nil {
		t.Errorf("GetLegalEntityByCNPJ() error = %v", err)
	}

	if result == nil {
		t.Fatal("GetLegalEntityByCNPJ() returned nil")
	}

	if result.CNPJ != "12345678000199" {
		t.Errorf("GetLegalEntityByCNPJ() CNPJ = %v, want 12345678000199", result.CNPJ)
	}
}

func TestGetLegalEntityByCNPJ_NotFound(t *testing.T) {
	service, cleanup := setupLegalEntityServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	_, err := service.GetLegalEntityByCNPJ(ctx, "99999999000199")
	if err == nil {
		t.Error("GetLegalEntityByCNPJ() should return error for non-existent CNPJ")
	}
}

func TestValidatePaginationParams_Defaults(t *testing.T) {
	page, perPage, err := ValidatePaginationParams("", "")
	if err != nil {
		t.Errorf("ValidatePaginationParams() error = %v", err)
	}

	if page != 1 {
		t.Errorf("ValidatePaginationParams() page = %v, want 1 (default)", page)
	}

	if perPage != 10 {
		t.Errorf("ValidatePaginationParams() perPage = %v, want 10 (default)", perPage)
	}
}

func TestValidatePaginationParams_Valid(t *testing.T) {
	page, perPage, err := ValidatePaginationParams("2", "20")
	if err != nil {
		t.Errorf("ValidatePaginationParams() error = %v", err)
	}

	if page != 2 {
		t.Errorf("ValidatePaginationParams() page = %v, want 2", page)
	}

	if perPage != 20 {
		t.Errorf("ValidatePaginationParams() perPage = %v, want 20", perPage)
	}
}

func TestValidatePaginationParams_InvalidPage(t *testing.T) {
	_, _, err := ValidatePaginationParams("invalid", "10")
	if err == nil {
		t.Error("ValidatePaginationParams() should return error for invalid page")
	}

	_, _, err = ValidatePaginationParams("0", "10")
	if err == nil {
		t.Error("ValidatePaginationParams() should return error for page < 1")
	}
}

func TestValidatePaginationParams_InvalidPerPage(t *testing.T) {
	_, _, err := ValidatePaginationParams("1", "invalid")
	if err == nil {
		t.Error("ValidatePaginationParams() should return error for invalid perPage")
	}

	_, _, err = ValidatePaginationParams("1", "0")
	if err == nil {
		t.Error("ValidatePaginationParams() should return error for perPage < 1")
	}

	_, _, err = ValidatePaginationParams("1", "101")
	if err == nil {
		t.Error("ValidatePaginationParams() should return error for perPage > 100")
	}
}
