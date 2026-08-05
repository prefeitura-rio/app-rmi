package services

import (
	"context"
	"fmt"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RioMobCatalogServiceInstance is the process-wide catalog service used by handlers.
var RioMobCatalogServiceInstance *RioMobCatalogService

// RioMobCatalogService serves brand/model/color catalogs for RioMob forms.
type RioMobCatalogService struct {
	database *mongo.Database
	logger   *logging.SafeLogger
}

// NewRioMobCatalogService creates a RioMobCatalogService.
func NewRioMobCatalogService(database *mongo.Database, logger *logging.SafeLogger) *RioMobCatalogService {
	return &RioMobCatalogService{database: database, logger: logger}
}

// InitRioMobCatalogService initializes RioMobCatalogServiceInstance.
func InitRioMobCatalogService() {
	RioMobCatalogServiceInstance = NewRioMobCatalogService(config.MongoDB, logging.GetLogger())
}

// ListBrands returns seeded vehicle brands ordered by name.
func (s *RioMobCatalogService) ListBrands(ctx context.Context) ([]models.VehicleBrand, error) {
	coll := s.database.Collection(config.AppConfig.RioMobBrandCollection)
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := coll.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("list brands: %w", err)
	}
	defer cursor.Close(ctx)

	var brands []models.VehicleBrand
	if err := cursor.All(ctx, &brands); err != nil {
		return nil, fmt.Errorf("decode brands: %w", err)
	}
	if brands == nil {
		brands = []models.VehicleBrand{}
	}
	return brands, nil
}

// ListModelsByBrand returns models for brandID (required).
func (s *RioMobCatalogService) ListModelsByBrand(ctx context.Context, brandID string) ([]models.VehicleModel, error) {
	if brandID == "" {
		return nil, fmt.Errorf("%w: brand_id is required", ErrRioMobInvalidInput)
	}
	coll := s.database.Collection(config.AppConfig.RioMobModelCollection)
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := coll.Find(ctx, bson.M{"brand_id": brandID}, opts)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer cursor.Close(ctx)

	var list []models.VehicleModel
	if err := cursor.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("decode models: %w", err)
	}
	if list == nil {
		list = []models.VehicleModel{}
	}
	return list, nil
}

// ListColors returns the fixed color catalog.
func (s *RioMobCatalogService) ListColors(ctx context.Context) ([]string, error) {
	return models.VehicleColors(), nil
}
