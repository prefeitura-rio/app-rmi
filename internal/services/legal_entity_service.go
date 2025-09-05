package services

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// LegalEntityService handles legal entity business logic
type LegalEntityService struct {
	database *mongo.Database
	logger   *logging.SafeLogger
}

// NewLegalEntityService creates a new legal entity service instance
func NewLegalEntityService(database *mongo.Database, logger *logging.SafeLogger) *LegalEntityService {
	return &LegalEntityService{
		database: database,
		logger:   logger,
	}
}

// Global legal entity service instance
var LegalEntityServiceInstance *LegalEntityService

// InitLegalEntityService initializes the global legal entity service instance
func InitLegalEntityService() {
	logger := zap.L().Named("legal_entity_service")
	
	LegalEntityServiceInstance = NewLegalEntityService(config.MongoDB, &logging.SafeLogger{})
	
	// Create MongoDB indexes for efficient queries
	err := LegalEntityServiceInstance.ensureIndexes()
	if err != nil {
		logger.Error("failed to create legal entity indexes", zap.Error(err))
	} else {
		logger.Info("legal entity service initialized successfully with indexes")
	}
}

// ensureIndexes creates the necessary MongoDB indexes for efficient CPF and legal nature queries
func (s *LegalEntityService) ensureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	collection := s.database.Collection(config.AppConfig.LegalEntityCollection)

	// Index 1: Compound index on socios.cpf_socio for CPF matching (most important)
	// This supports queries like: {"socios.cpf_socio": "12345678901"}
	partnerCPFIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "socios.cpf_socio", Value: 1},
		},
		Options: options.Index().SetName("idx_partners_cpf"),
	}

	// Index 2: Index on natureza_juridica.id for legal nature filtering
	// This supports queries like: {"natureza_juridica.id": "2062"}
	legalNatureIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "natureza_juridica.id", Value: 1},
		},
		Options: options.Index().SetName("idx_legal_nature_id"),
	}

	// Index 3: Compound index for combined filtering (CPF + legal nature)
	// This supports queries like: {"socios.cpf_socio": "12345678901", "natureza_juridica.id": "2062"}
	combinedIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "socios.cpf_socio", Value: 1},
			{Key: "natureza_juridica.id", Value: 1},
		},
		Options: options.Index().SetName("idx_partners_cpf_legal_nature"),
	}

	// Index 4: Additional useful indexes for general queries
	cnpjIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "cnpj", Value: 1},
		},
		Options: options.Index().SetName("idx_cnpj").SetUnique(true),
	}

	// Index 5: Compound index for pagination performance (sorted by company name)
	paginationIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "socios.cpf_socio", Value: 1},
			{Key: "razao_social", Value: 1},
		},
		Options: options.Index().SetName("idx_partners_cpf_company_name"),
	}

	// Create all indexes
	indexes := []mongo.IndexModel{
		partnerCPFIndex,
		legalNatureIndex,
		combinedIndex,
		cnpjIndex,
		paginationIndex,
	}

	indexNames, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create legal entity indexes: %w", err)
	}

	s.logger.Info("created legal entity indexes successfully",
		zap.Strings("index_names", indexNames),
		zap.String("collection", config.AppConfig.LegalEntityCollection))

	return nil
}

// GetLegalEntitiesByCPF retrieves legal entities associated with a CPF with pagination
func (s *LegalEntityService) GetLegalEntitiesByCPF(ctx context.Context, cpf string, page, perPage int, legalNatureID *string) (*models.PaginatedLegalEntities, error) {
	collection := s.database.Collection(config.AppConfig.LegalEntityCollection)

	// Build filter query
	filter := bson.M{
		"socios.cpf_socio": cpf,
	}

	// Add legal nature filter if provided
	if legalNatureID != nil && *legalNatureID != "" {
		filter["natureza_juridica.id"] = *legalNatureID
	}

	// Get total count
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count legal entities: %w", err)
	}

	// Calculate pagination
	skip := (page - 1) * perPage
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	// Set up find options with pagination and sorting
	findOptions := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPage)).
		SetSort(bson.D{{Key: "razao_social", Value: 1}}) // Sort by company name

	// Execute query
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find legal entities: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode results
	var entities []models.LegalEntity
	if err = cursor.All(ctx, &entities); err != nil {
		return nil, fmt.Errorf("failed to decode legal entities: %w", err)
	}

	// Build paginated response
	response := &models.PaginatedLegalEntities{
		Data: entities,
	}
	response.Pagination.Page = page
	response.Pagination.PerPage = perPage
	response.Pagination.Total = int(total)
	response.Pagination.TotalPages = totalPages

	s.logger.Debug("retrieved legal entities for CPF",
		zap.String("cpf", cpf),
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int("total", int(total)),
		zap.Int("returned", len(entities)),
		zap.Any("legal_nature_filter", legalNatureID))

	return response, nil
}

// ValidatePaginationParams validates and normalizes pagination parameters
func ValidatePaginationParams(pageStr, perPageStr string) (int, int, error) {
	page := 1
	if pageStr != "" {
		p, err := strconv.Atoi(pageStr)
		if err != nil || p < 1 {
			return 0, 0, fmt.Errorf("invalid page parameter: must be a positive integer")
		}
		page = p
	}

	perPage := 10
	if perPageStr != "" {
		pp, err := strconv.Atoi(perPageStr)
		if err != nil || pp < 1 || pp > 100 {
			return 0, 0, fmt.Errorf("invalid per_page parameter: must be between 1 and 100")
		}
		perPage = pp
	}

	return page, perPage, nil
}