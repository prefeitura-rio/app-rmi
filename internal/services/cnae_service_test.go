package services

import (
	"context"
	"os"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func setupCNAEServiceTest(t *testing.T) (*CNAEService, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping CNAE service tests: MONGODB_URI not set")
	}

	logging.InitLogger()

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.CNAECollection = "test_cnaes"

	ctx := context.Background()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	database := client.Database("rmi_test")
	config.MongoDB = database

	// Create text index for search
	collection := database.Collection(config.AppConfig.CNAECollection)
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "Denominacao", Value: "text"}},
	}
	_, _ = collection.Indexes().CreateOne(ctx, indexModel)

	service := NewCNAEService(database, logging.Logger)

	return service, func() {
		database.Drop(ctx)
		client.Disconnect(ctx)
	}
}

func TestNewCNAEService(t *testing.T) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		t.Skip("Skipping: MONGODB_URI not set")
	}

	ctx := context.Background()
	client, _ := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	defer client.Disconnect(ctx)

	database := client.Database("test")
	service := NewCNAEService(database, logging.Logger)

	if service == nil {
		t.Error("NewCNAEService() returned nil")
	}
}

func TestListCNAEs_Empty(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	filters := models.CNAEFilters{
		Page:    1,
		PerPage: 10,
	}

	result, err := service.ListCNAEs(ctx, filters)
	if err != nil {
		t.Errorf("ListCNAEs() error = %v", err)
		return
	}

	if result == nil {
		t.Error("ListCNAEs() returned nil result")
		return
	}

	if result.Pagination.Total != 0 {
		t.Errorf("ListCNAEs() Total = %v, want 0", result.Pagination.Total)
	}

	if len(result.CNAEs) != 0 {
		t.Errorf("ListCNAEs() len(CNAEs) = %v, want 0", len(result.CNAEs))
	}
}

func TestListCNAEs_WithData(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "011",
			"Classe":      "0111",
			"Subclasse":   "01111",
			"Denominacao": "Cultivo de arroz",
		},
		bson.M{
			"Secao":       "A",
			"Divisao":     "01",
			"Grupo":       "011",
			"Classe":      "0112",
			"Subclasse":   "01121",
			"Denominacao": "Cultivo de milho",
		},
		bson.M{
			"Secao":       "B",
			"Divisao":     "05",
			"Grupo":       "051",
			"Classe":      "0510",
			"Subclasse":   "05101",
			"Denominacao": "Extração de carvão mineral",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	if err != nil {
		t.Fatalf("Failed to insert CNAEs: %v", err)
	}

	filters := models.CNAEFilters{
		Page:    1,
		PerPage: 10,
	}

	result, err := service.ListCNAEs(ctx, filters)
	if err != nil {
		t.Errorf("ListCNAEs() error = %v", err)
	}

	if result.Pagination.Total != 3 {
		t.Errorf("ListCNAEs() Total = %v, want 3", result.Pagination.Total)
	}

	if len(result.CNAEs) != 3 {
		t.Errorf("ListCNAEs() len(CNAEs) = %v, want 3", len(result.CNAEs))
	}
}

func TestListCNAEs_FilterBySecao(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test CNAEs with different Secao values
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"Secao":       "A",
			"Divisao":     "01",
			"Classe":      "0111",
			"Denominacao": "Cultivo de arroz",
		},
		bson.M{
			"Secao":       "B",
			"Divisao":     "05",
			"Classe":      "0510",
			"Denominacao": "Extração de carvão",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	if err != nil {
		t.Fatalf("Failed to insert CNAEs: %v", err)
	}

	filters := models.CNAEFilters{
		Page:    1,
		PerPage: 10,
		Secao:   "A",
	}

	result, err := service.ListCNAEs(ctx, filters)
	if err != nil {
		t.Errorf("ListCNAEs() error = %v", err)
	}

	if result.Pagination.Total != 1 {
		t.Errorf("ListCNAEs() with Secao filter Total = %v, want 1", result.Pagination.Total)
	}

	if len(result.CNAEs) == 0 || result.CNAEs[0].Secao != "A" {
		t.Errorf("ListCNAEs() filtered CNAE Secao = %v, want A", result.CNAEs[0].Secao)
	}
}

func TestListCNAEs_FilterByDivisao(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"Secao":       "A",
			"Divisao":     "01",
			"Classe":      "0111",
			"Denominacao": "Cultivo de arroz",
		},
		bson.M{
			"Secao":       "A",
			"Divisao":     "02",
			"Classe":      "0210",
			"Denominacao": "Produção florestal",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	if err != nil {
		t.Fatalf("Failed to insert CNAEs: %v", err)
	}

	filters := models.CNAEFilters{
		Page:    1,
		PerPage: 10,
		Divisao: "01",
	}

	result, err := service.ListCNAEs(ctx, filters)
	if err != nil {
		t.Errorf("ListCNAEs() error = %v", err)
	}

	if result.Pagination.Total != 1 {
		t.Errorf("ListCNAEs() with Divisao filter Total = %v, want 1", result.Pagination.Total)
	}
}

func TestListCNAEs_FilterByClasse(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"Secao":       "A",
			"Classe":      "0111",
			"Denominacao": "Cultivo de arroz",
		},
		bson.M{
			"Secao":       "A",
			"Classe":      "0112",
			"Denominacao": "Cultivo de milho",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	if err != nil {
		t.Fatalf("Failed to insert CNAEs: %v", err)
	}

	filters := models.CNAEFilters{
		Page:    1,
		PerPage: 10,
		Classe:  "0111",
	}

	result, err := service.ListCNAEs(ctx, filters)
	if err != nil {
		t.Errorf("ListCNAEs() error = %v", err)
	}

	if result.Pagination.Total != 1 {
		t.Errorf("ListCNAEs() with Classe filter Total = %v, want 1", result.Pagination.Total)
	}

	if len(result.CNAEs) == 0 || result.CNAEs[0].Classe != "0111" {
		t.Errorf("ListCNAEs() filtered CNAE Classe = %v, want 0111", result.CNAEs[0].Classe)
	}
}

func TestListCNAEs_FilterBySubclasse(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"Subclasse":   "01111",
			"Denominacao": "Cultivo de arroz",
		},
		bson.M{
			"Subclasse":   "01121",
			"Denominacao": "Cultivo de milho",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	if err != nil {
		t.Fatalf("Failed to insert CNAEs: %v", err)
	}

	filters := models.CNAEFilters{
		Page:      1,
		PerPage:   10,
		Subclasse: "01111",
	}

	result, err := service.ListCNAEs(ctx, filters)
	if err != nil {
		t.Errorf("ListCNAEs() error = %v", err)
	}

	if result.Pagination.Total != 1 {
		t.Errorf("ListCNAEs() with Subclasse filter Total = %v, want 1", result.Pagination.Total)
	}
}

func TestListCNAEs_TextSearch(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	cnaes := []interface{}{
		bson.M{
			"Classe":      "0111",
			"Denominacao": "Cultivo de arroz",
		},
		bson.M{
			"Classe":      "0112",
			"Denominacao": "Cultivo de milho",
		},
		bson.M{
			"Classe":      "0510",
			"Denominacao": "Extração de carvão mineral",
		},
	}

	_, err := collection.InsertMany(ctx, cnaes)
	if err != nil {
		t.Fatalf("Failed to insert CNAEs: %v", err)
	}

	filters := models.CNAEFilters{
		Page:    1,
		PerPage: 10,
		Search:  "cultivo",
	}

	result, err := service.ListCNAEs(ctx, filters)
	if err != nil {
		t.Errorf("ListCNAEs() with search error = %v", err)
	}

	if result.Pagination.Total != 2 {
		t.Errorf("ListCNAEs() with search Total = %v, want 2", result.Pagination.Total)
	}
}

func TestListCNAEs_Pagination(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert 5 CNAEs
	collection := config.MongoDB.Collection(config.AppConfig.CNAECollection)
	var cnaes []interface{}
	for i := 1; i <= 5; i++ {
		cnaes = append(cnaes, bson.M{
			"Classe":      "011" + string(rune('0'+i)),
			"Denominacao": "CNAE " + string(rune('A'+i-1)),
		})
	}

	_, err := collection.InsertMany(ctx, cnaes)
	if err != nil {
		t.Fatalf("Failed to insert CNAEs: %v", err)
	}

	// Page 1
	filters := models.CNAEFilters{
		Page:    1,
		PerPage: 2,
	}

	result, err := service.ListCNAEs(ctx, filters)
	if err != nil {
		t.Errorf("ListCNAEs() page 1 error = %v", err)
	}

	if len(result.CNAEs) != 2 {
		t.Errorf("ListCNAEs() page 1 len(CNAEs) = %v, want 2", len(result.CNAEs))
	}

	if result.Pagination.TotalPages != 3 {
		t.Errorf("ListCNAEs() TotalPages = %v, want 3", result.Pagination.TotalPages)
	}
}

func TestConvertRawToCNAE_StringID(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	rawDoc := bson.M{
		"_id":         "test-id-123",
		"Secao":       "A",
		"Divisao":     "01",
		"Grupo":       "011",
		"Classe":      "0111",
		"Subclasse":   "01111",
		"Denominacao": "Test CNAE",
	}

	cnae := service.convertRawToCNAE(rawDoc)

	if cnae.ID != "test-id-123" {
		t.Errorf("convertRawToCNAE() ID = %v, want test-id-123", cnae.ID)
	}

	if cnae.Secao != "A" {
		t.Errorf("convertRawToCNAE() Secao = %v, want A", cnae.Secao)
	}

	if cnae.Denominacao != "Test CNAE" {
		t.Errorf("convertRawToCNAE() Denominacao = %v, want Test CNAE", cnae.Denominacao)
	}
}

func TestConvertRawToCNAE_ObjectID(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	objectID := primitive.NewObjectID()

	rawDoc := bson.M{
		"_id":         objectID,
		"Secao":       "B",
		"Denominacao": "Test CNAE with ObjectID",
	}

	cnae := service.convertRawToCNAE(rawDoc)

	if cnae.ID != objectID.Hex() {
		t.Errorf("convertRawToCNAE() ID = %v, want %v", cnae.ID, objectID.Hex())
	}
}

func TestConvertRawToCNAE_IntegerDivisao(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	tests := []struct {
		name     string
		divisao  interface{}
		expected string
	}{
		{"int", int(1), "1"},
		{"int32", int32(2), "2"},
		{"int64", int64(3), "3"},
		{"float64", float64(4), "4"},
		{"string", "05", "05"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawDoc := bson.M{
				"Divisao": tt.divisao,
			}

			cnae := service.convertRawToCNAE(rawDoc)

			if cnae.Divisao != tt.expected {
				t.Errorf("convertRawToCNAE() Divisao = %v, want %v", cnae.Divisao, tt.expected)
			}
		})
	}
}

func TestConvertRawToCNAE_IntegerGrupo(t *testing.T) {
	service, cleanup := setupCNAEServiceTest(t)
	defer cleanup()

	tests := []struct {
		name     string
		grupo    interface{}
		expected string
	}{
		{"int", int(11), "11"},
		{"int32", int32(12), "12"},
		{"int64", int64(13), "13"},
		{"float64", float64(14), "14"},
		{"string", "015", "015"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawDoc := bson.M{
				"Grupo": tt.grupo,
			}

			cnae := service.convertRawToCNAE(rawDoc)

			if cnae.Grupo != tt.expected {
				t.Errorf("convertRawToCNAE() Grupo = %v, want %v", cnae.Grupo, tt.expected)
			}
		})
	}
}
