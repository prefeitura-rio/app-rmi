package services

import (
	"context"
	"fmt"
	"strconv"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// CNAEService handles CNAE business logic
type CNAEService struct {
	database *mongo.Database
	logger   *logging.SafeLogger
}

// NewCNAEService creates a new CNAE service instance
func NewCNAEService(database *mongo.Database, logger *logging.SafeLogger) *CNAEService {
	return &CNAEService{
		database: database,
		logger:   logger,
	}
}

// ListCNAEs retrieves a paginated list of CNAEs with optional filters
func (s *CNAEService) ListCNAEs(ctx context.Context, filters models.CNAEFilters) (*models.CNAEListResponse, error) {
	collection := s.database.Collection(config.AppConfig.CNAECollection)

	// Build filter query
	filter := bson.M{}

	// Text search filter on Denominacao
	if filters.Search != "" {
		filter["$text"] = bson.M{"$search": filters.Search}
	}

	// Field filters - all fields are strings
	if filters.Secao != "" {
		filter["Secao"] = filters.Secao
	}
	if filters.Divisao != "" {
		filter["Divisao"] = filters.Divisao
	}
	if filters.Grupo != "" {
		filter["Grupo"] = filters.Grupo
	}
	if filters.Classe != "" {
		filter["Classe"] = filters.Classe
	}
	if filters.Subclasse != "" {
		filter["Subclasse"] = filters.Subclasse
	}

	// Calculate pagination
	skip := (filters.Page - 1) * filters.PerPage

	// Get total count
	totalCount, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		s.logger.Error("failed to count CNAEs", zap.Error(err))
		return nil, fmt.Errorf("failed to count CNAEs: %w", err)
	}

	// Set up find options
	findOptions := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(filters.PerPage))

	// If using text search, sort by text score; otherwise sort by Classe
	if filters.Search != "" {
		findOptions.SetProjection(bson.M{"score": bson.M{"$meta": "textScore"}})
		findOptions.SetSort(bson.D{{Key: "score", Value: bson.M{"$meta": "textScore"}}})
	} else {
		findOptions.SetSort(bson.D{{Key: "Classe", Value: 1}})
	}

	// Execute query
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		s.logger.Error("failed to list CNAEs", zap.Error(err))
		return nil, fmt.Errorf("failed to list CNAEs: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode results with flexible bson.M to handle type variations
	var cnaes []models.CNAE
	for cursor.Next(ctx) {
		var rawDoc bson.M
		if err := cursor.Decode(&rawDoc); err != nil {
			s.logger.Warn("failed to decode CNAE document",
				zap.Error(err))
			continue
		}

		// Convert to CNAE model with type handling
		cnae := s.convertRawToCNAE(rawDoc)
		cnaes = append(cnaes, cnae)
	}

	if err := cursor.Err(); err != nil {
		s.logger.Error("cursor error while iterating CNAEs", zap.Error(err))
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	// Calculate pagination info
	totalPages := int(totalCount) / filters.PerPage
	if int(totalCount)%filters.PerPage > 0 {
		totalPages++
	}

	return &models.CNAEListResponse{
		CNAEs: cnaes,
		Pagination: models.PaginationInfo{
			Page:       filters.Page,
			PerPage:    filters.PerPage,
			Total:      int(totalCount),
			TotalPages: totalPages,
		},
	}, nil
}

// convertRawToCNAE converts a raw BSON document to a CNAE model with type handling
func (s *CNAEService) convertRawToCNAE(rawDoc bson.M) models.CNAE {
	cnae := models.CNAE{}

	// Handle _id field
	if id, ok := rawDoc["_id"]; ok {
		switch v := id.(type) {
		case string:
			cnae.ID = v
		case primitive.ObjectID:
			cnae.ID = v.Hex()
		}
	}

	// Handle Secao (string)
	if secao, ok := rawDoc["Secao"].(string); ok {
		cnae.Secao = secao
	}

	// Handle Divisao (string - but keep type handling for robustness)
	if divisao, ok := rawDoc["Divisao"]; ok {
		switch v := divisao.(type) {
		case string:
			cnae.Divisao = v
		case int:
			cnae.Divisao = strconv.Itoa(v)
		case int32:
			cnae.Divisao = strconv.Itoa(int(v))
		case int64:
			cnae.Divisao = strconv.FormatInt(v, 10)
		case float64:
			cnae.Divisao = strconv.FormatFloat(v, 'f', -1, 64)
		}
	}

	// Handle Grupo (string - but keep type handling for robustness)
	if grupo, ok := rawDoc["Grupo"]; ok {
		switch v := grupo.(type) {
		case string:
			cnae.Grupo = v
		case int:
			cnae.Grupo = strconv.Itoa(v)
		case int32:
			cnae.Grupo = strconv.Itoa(int(v))
		case int64:
			cnae.Grupo = strconv.FormatInt(v, 10)
		case float64:
			cnae.Grupo = strconv.FormatFloat(v, 'f', -1, 64)
		}
	}

	// Handle Classe (string)
	if classe, ok := rawDoc["Classe"].(string); ok {
		cnae.Classe = classe
	}

	// Handle Subclasse (string)
	if subclasse, ok := rawDoc["Subclasse"].(string); ok {
		cnae.Subclasse = subclasse
	}

	// Handle Denominacao (string)
	if denominacao, ok := rawDoc["Denominacao"].(string); ok {
		cnae.Denominacao = denominacao
	}

	return cnae
}
