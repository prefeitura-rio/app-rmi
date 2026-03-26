package services

import (
	"context"
	"fmt"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// BairroService handles bairro business logic
type BairroService struct {
	database *mongo.Database
	logger   *logging.SafeLogger
}

// NewBairroService creates a new BairroService instance
func NewBairroService(database *mongo.Database, logger *logging.SafeLogger) *BairroService {
	return &BairroService{
		database: database,
		logger:   logger,
	}
}

// ListBairros retrieves a paginated list of bairros with optional search filter
func (s *BairroService) ListBairros(ctx context.Context, filters models.BairroFilters) (*models.BairroListResponse, error) {
	collection := s.database.Collection(config.AppConfig.BairroCollection)

	// Build filter query
	filter := bson.M{}

	// Case-insensitive substring search on the nome field
	if filters.Search != "" {
		filter["nome"] = bson.M{
			"$regex":   filters.Search,
			"$options": "i",
		}
	}

	// Get total count matching the filter
	totalCount, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		s.logger.Error("failed to count bairros", zap.Error(err))
		return nil, fmt.Errorf("failed to count bairros: %w", err)
	}

	// Calculate pagination offset
	skip := (filters.Page - 1) * filters.Limit

	// Set up find options with pagination and sorting by nome
	findOptions := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(filters.Limit)).
		SetSort(bson.D{{Key: "nome", Value: 1}})

	// Execute query
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		s.logger.Error("failed to list bairros", zap.Error(err))
		return nil, fmt.Errorf("failed to list bairros: %w", err)
	}
	defer cursor.Close(ctx)

	var bairros []models.Bairro
	for cursor.Next(ctx) {
		var rawDoc bson.M
		if err := cursor.Decode(&rawDoc); err != nil {
			s.logger.Warn("failed to decode bairro document", zap.Error(err))
			continue
		}
		bairro := convertRawToBairro(rawDoc)
		bairros = append(bairros, bairro)
	}

	if err := cursor.Err(); err != nil {
		s.logger.Error("cursor error while iterating bairros", zap.Error(err))
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	// Ensure bairros is never nil in the response
	if bairros == nil {
		bairros = []models.Bairro{}
	}

	return &models.BairroListResponse{
		Bairros: bairros,
		Total:   totalCount,
		Page:    filters.Page,
		Limit:   filters.Limit,
	}, nil
}

// convertRawToBairro converts a raw BSON document to a Bairro model
func convertRawToBairro(rawDoc bson.M) models.Bairro {
	bairro := models.Bairro{}

	// Handle id_bairro field
	if idVal, ok := rawDoc["id_bairro"]; ok {
		switch v := idVal.(type) {
		case string:
			bairro.ID = v
		case primitive.ObjectID:
			bairro.ID = v.Hex()
		}
	}

	// Handle nome field
	if nome, ok := rawDoc["nome"].(string); ok {
		bairro.Nome = nome
	}

	return bairro
}
