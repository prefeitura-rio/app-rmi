package services

import (
	"context"
	"fmt"
	"math"
	"strconv"

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

	logger.Info("legal entity service initialized successfully")
	logger.Info("indexes will be managed by global database maintenance system")
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

// GetLegalEntityByCNPJ retrieves a legal entity by CNPJ
func (s *LegalEntityService) GetLegalEntityByCNPJ(ctx context.Context, cnpj string) (*models.LegalEntity, error) {
	collection := s.database.Collection(config.AppConfig.LegalEntityCollection)

	var entity models.LegalEntity
	err := collection.FindOne(ctx, bson.M{"cnpj": cnpj}).Decode(&entity)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("legal entity not found")
		}
		return nil, fmt.Errorf("failed to find legal entity: %w", err)
	}

	s.logger.Debug("retrieved legal entity by CNPJ", zap.String("cnpj", cnpj))

	return &entity, nil
}
