package services

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MobilidadeCatalogServiceInstance is the process-wide catalog service used by handlers.
var MobilidadeCatalogServiceInstance *MobilidadeCatalogService

// MobilidadeCatalogService serves brand/model/color catalogs for Mobilidade forms.
type MobilidadeCatalogService struct {
	database *mongo.Database
	logger   *logging.SafeLogger
}

// NewMobilidadeCatalogService creates a MobilidadeCatalogService.
func NewMobilidadeCatalogService(database *mongo.Database, logger *logging.SafeLogger) *MobilidadeCatalogService {
	return &MobilidadeCatalogService{database: database, logger: logger}
}

// InitMobilidadeCatalogService initializes MobilidadeCatalogServiceInstance.
func InitMobilidadeCatalogService() {
	MobilidadeCatalogServiceInstance = NewMobilidadeCatalogService(config.MongoDB, logging.GetLogger())
}

func catalogActiveFilter() bson.M {
	return bson.M{
		"$or": []bson.M{
			{"deleted_at": nil},
			{"deleted_at": bson.M{"$exists": false}},
		},
	}
}

func (s *MobilidadeCatalogService) brandsColl() *mongo.Collection {
	return s.database.Collection(config.AppConfig.MobilidadeBrandCollection)
}

func (s *MobilidadeCatalogService) modelsColl() *mongo.Collection {
	return s.database.Collection(config.AppConfig.MobilidadeModelCollection)
}

func (s *MobilidadeCatalogService) vehiclesColl() *mongo.Collection {
	return s.database.Collection(config.AppConfig.MobilidadeVehicleCollection)
}

// ListBrands returns active vehicle brands ordered by name (citizen form).
func (s *MobilidadeCatalogService) ListBrands(ctx context.Context) ([]models.VehicleBrand, error) {
	return s.listBrands(ctx, catalogActiveFilter())
}

// ListBrandsAdmin returns all brands including soft-deleted (admin).
func (s *MobilidadeCatalogService) ListBrandsAdmin(ctx context.Context) ([]models.VehicleBrand, error) {
	return s.listBrands(ctx, bson.M{})
}

func (s *MobilidadeCatalogService) listBrands(ctx context.Context, filter bson.M) ([]models.VehicleBrand, error) {
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := s.brandsColl().Find(ctx, filter, opts)
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

// ListModelsByBrand returns active models for brandID (citizen form).
func (s *MobilidadeCatalogService) ListModelsByBrand(ctx context.Context, brandID string) ([]models.VehicleModel, error) {
	if brandID == "" {
		return nil, fmt.Errorf("%w: brand_id is required", ErrMobilidadeInvalidInput)
	}
	filter := bson.M{"brand_id": brandID}
	maps.Copy(filter, catalogActiveFilter())
	return s.listModels(ctx, filter)
}

// ListModelsAdmin returns all models, optionally filtered by brand_id (admin).
func (s *MobilidadeCatalogService) ListModelsAdmin(ctx context.Context, brandID string) ([]models.VehicleModel, error) {
	filter := bson.M{}
	if brandID != "" {
		filter["brand_id"] = brandID
	}
	return s.listModels(ctx, filter)
}

func (s *MobilidadeCatalogService) listModels(ctx context.Context, filter bson.M) ([]models.VehicleModel, error) {
	opts := options.Find().SetSort(bson.D{{Key: "name", Value: 1}})
	cursor, err := s.modelsColl().Find(ctx, filter, opts)
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
func (s *MobilidadeCatalogService) ListColors(ctx context.Context) ([]string, error) {
	return models.VehicleColors(), nil
}

// CreateBrand registers a new catalog brand (admin).
func (s *MobilidadeCatalogService) CreateBrand(ctx context.Context, req *models.VehicleBrandCreateRequest) (*models.VehicleBrand, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMobilidadeInvalidInput, err.Error())
	}
	name := strings.TrimSpace(req.Name)
	if err := s.ensureBrandNameUnique(ctx, name, ""); err != nil {
		return nil, err
	}

	id := utils.MobilidadeBrandIDFromName(name)
	var existing models.VehicleBrand
	err := s.brandsColl().FindOne(ctx, bson.M{"_id": id}).Decode(&existing)
	if err == nil {
		if existing.DeletedAt == nil {
			return nil, fmt.Errorf("%w: brand already exists", ErrMobilidadeConflict)
		}
		return nil, fmt.Errorf("%w: brand id already used by inactive entry", ErrMobilidadeConflict)
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("check brand id: %w", err)
	}

	brand := models.VehicleBrand{
		ID:      id,
		Name:    name,
		IsOther: false,
	}
	if _, err := s.brandsColl().InsertOne(ctx, brand); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrMobilidadeConflict
		}
		return nil, fmt.Errorf("insert brand: %w", err)
	}
	return &brand, nil
}

// UpdateBrand updates an active brand name (admin).
func (s *MobilidadeCatalogService) UpdateBrand(ctx context.Context, brandID string, req *models.VehicleBrandUpdateRequest) (*models.VehicleBrand, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMobilidadeInvalidInput, err.Error())
	}
	if err := s.rejectCatalogSentinelBrand(brandID); err != nil {
		return nil, err
	}

	brand, err := s.findActiveBrand(ctx, brandID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureBrandReferencedByVehicle(ctx, brandID); err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.Name)
	if err := s.ensureBrandNameUnique(ctx, name, brandID); err != nil {
		return nil, err
	}

	res, err := s.brandsColl().UpdateOne(ctx,
		bson.M{"_id": brandID, "deleted_at": nil},
		bson.M{"$set": bson.M{"name": name}},
	)
	if err != nil {
		return nil, fmt.Errorf("update brand: %w", err)
	}
	if res.MatchedCount == 0 {
		return nil, ErrCatalogBrandNotFound
	}
	brand.Name = name
	return brand, nil
}

// DeleteBrand soft-deletes a brand when it has no active models or vehicle references (admin).
func (s *MobilidadeCatalogService) DeleteBrand(ctx context.Context, brandID string) error {
	if err := s.rejectCatalogSentinelBrand(brandID); err != nil {
		return err
	}

	if _, err := s.findActiveBrand(ctx, brandID); err != nil {
		return err
	}
	if err := s.ensureBrandReferencedByVehicle(ctx, brandID); err != nil {
		return err
	}

	activeModels, err := s.modelsColl().CountDocuments(ctx, bson.M{
		"brand_id": brandID,
		"$or": []bson.M{
			{"deleted_at": nil},
			{"deleted_at": bson.M{"$exists": false}},
		},
	})
	if err != nil {
		return fmt.Errorf("count brand models: %w", err)
	}
	if activeModels > 0 {
		return fmt.Errorf("%w: brand still has active models", ErrMobilidadeConflict)
	}

	now := time.Now().UTC()
	res, err := s.brandsColl().UpdateOne(ctx,
		bson.M{"_id": brandID, "deleted_at": nil},
		bson.M{"$set": bson.M{"deleted_at": now}},
	)
	if err != nil {
		return fmt.Errorf("delete brand: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrCatalogBrandNotFound
	}
	return nil
}

// CreateModel registers a new catalog model under a brand (admin).
func (s *MobilidadeCatalogService) CreateModel(ctx context.Context, req *models.VehicleModelCreateRequest) (*models.VehicleModel, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMobilidadeInvalidInput, err.Error())
	}
	brandID := strings.TrimSpace(req.BrandID)
	if brandID == models.VehicleBrandOutroID {
		return nil, fmt.Errorf("%w: cannot add models to sentinel brand", ErrMobilidadeInvalidInput)
	}

	brand, err := s.findActiveBrand(ctx, brandID)
	if err != nil {
		return nil, err
	}
	if brand.IsOther {
		return nil, fmt.Errorf("%w: cannot add models to sentinel brand", ErrMobilidadeInvalidInput)
	}

	name := strings.TrimSpace(req.Name)
	if err := s.ensureModelNameUnique(ctx, brandID, name, ""); err != nil {
		return nil, err
	}

	id := utils.MobilidadeModelIDFromBrandAndName(brand.Name, name)
	var existing models.VehicleModel
	err = s.modelsColl().FindOne(ctx, bson.M{"_id": id}).Decode(&existing)
	if err == nil {
		if existing.DeletedAt == nil {
			return nil, fmt.Errorf("%w: model already exists", ErrMobilidadeConflict)
		}
		return nil, fmt.Errorf("%w: model id already used by inactive entry", ErrMobilidadeConflict)
	}
	if !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, fmt.Errorf("check model id: %w", err)
	}

	model := models.VehicleModel{
		ID:          id,
		BrandID:     brandID,
		Name:        name,
		VehicleType: req.VehicleType,
		IsOther:     false,
	}
	if _, err := s.modelsColl().InsertOne(ctx, model); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, ErrMobilidadeConflict
		}
		return nil, fmt.Errorf("insert model: %w", err)
	}
	return &model, nil
}

// UpdateModel updates an active model (admin).
func (s *MobilidadeCatalogService) UpdateModel(ctx context.Context, modelID string, req *models.VehicleModelUpdateRequest) (*models.VehicleModel, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMobilidadeInvalidInput, err.Error())
	}
	if err := s.rejectCatalogSentinelModel(modelID); err != nil {
		return nil, err
	}

	model, err := s.findActiveModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureModelReferencedByVehicle(ctx, modelID); err != nil {
		return nil, err
	}

	targetBrandID := model.BrandID
	if req.BrandID != nil {
		targetBrandID = strings.TrimSpace(*req.BrandID)
		if targetBrandID == models.VehicleBrandOutroID {
			return nil, fmt.Errorf("%w: cannot move model to sentinel brand", ErrMobilidadeInvalidInput)
		}
		brand, err := s.findActiveBrand(ctx, targetBrandID)
		if err != nil {
			return nil, err
		}
		if brand.IsOther {
			return nil, fmt.Errorf("%w: cannot move model to sentinel brand", ErrMobilidadeInvalidInput)
		}
	}

	targetName := model.Name
	if req.Name != nil {
		targetName = strings.TrimSpace(*req.Name)
	}
	if err := s.ensureModelNameUnique(ctx, targetBrandID, targetName, modelID); err != nil {
		return nil, err
	}

	set := bson.M{}
	if req.Name != nil {
		set["name"] = targetName
	}
	if req.BrandID != nil {
		set["brand_id"] = targetBrandID
	}
	if req.VehicleType != nil {
		set["vehicle_type"] = *req.VehicleType
	}

	res, err := s.modelsColl().UpdateOne(ctx,
		bson.M{"_id": modelID, "deleted_at": nil},
		bson.M{"$set": set},
	)
	if err != nil {
		return nil, fmt.Errorf("update model: %w", err)
	}
	if res.MatchedCount == 0 {
		return nil, ErrCatalogModelNotFound
	}

	updated, err := s.findActiveModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteModel soft-deletes a model when not referenced by vehicles (admin).
func (s *MobilidadeCatalogService) DeleteModel(ctx context.Context, modelID string) error {
	if err := s.rejectCatalogSentinelModel(modelID); err != nil {
		return err
	}

	if _, err := s.findActiveModel(ctx, modelID); err != nil {
		return err
	}
	if err := s.ensureModelReferencedByVehicle(ctx, modelID); err != nil {
		return err
	}

	now := time.Now().UTC()
	res, err := s.modelsColl().UpdateOne(ctx,
		bson.M{"_id": modelID, "deleted_at": nil},
		bson.M{"$set": bson.M{"deleted_at": now}},
	)
	if err != nil {
		return fmt.Errorf("delete model: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrCatalogModelNotFound
	}
	return nil
}

func (s *MobilidadeCatalogService) findActiveBrand(ctx context.Context, brandID string) (*models.VehicleBrand, error) {
	var brand models.VehicleBrand
	filter := bson.M{"_id": brandID}
	maps.Copy(filter, catalogActiveFilter())
	err := s.brandsColl().FindOne(ctx, filter).Decode(&brand)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrCatalogBrandNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find brand: %w", err)
	}
	return &brand, nil
}

func (s *MobilidadeCatalogService) findActiveModel(ctx context.Context, modelID string) (*models.VehicleModel, error) {
	var model models.VehicleModel
	filter := bson.M{"_id": modelID}
	maps.Copy(filter, catalogActiveFilter())
	err := s.modelsColl().FindOne(ctx, filter).Decode(&model)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrCatalogModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find model: %w", err)
	}
	return &model, nil
}

func (s *MobilidadeCatalogService) rejectCatalogSentinelBrand(brandID string) error {
	if brandID == models.VehicleBrandOutroID {
		return fmt.Errorf("%w: sentinel brand cannot be modified", ErrMobilidadeInvalidInput)
	}
	return nil
}

func (s *MobilidadeCatalogService) rejectCatalogSentinelModel(modelID string) error {
	if modelID == models.VehicleModelOutroID {
		return fmt.Errorf("%w: sentinel model cannot be modified", ErrMobilidadeInvalidInput)
	}
	return nil
}

func (s *MobilidadeCatalogService) ensureBrandNameUnique(ctx context.Context, name, excludeID string) error {
	filter := bson.M{
		"name": caseInsensitiveExactRegex(name),
	}
	maps.Copy(filter, catalogActiveFilter())
	if excludeID != "" {
		filter["_id"] = bson.M{"$ne": excludeID}
	}
	count, err := s.brandsColl().CountDocuments(ctx, filter)
	if err != nil {
		return fmt.Errorf("check brand name: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: brand name already exists", ErrMobilidadeConflict)
	}
	return nil
}

func (s *MobilidadeCatalogService) ensureModelNameUnique(ctx context.Context, brandID, name, excludeID string) error {
	filter := bson.M{
		"brand_id": brandID,
		"name":     caseInsensitiveExactRegex(name),
	}
	maps.Copy(filter, catalogActiveFilter())
	if excludeID != "" {
		filter["_id"] = bson.M{"$ne": excludeID}
	}
	count, err := s.modelsColl().CountDocuments(ctx, filter)
	if err != nil {
		return fmt.Errorf("check model name: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: model name already exists for brand", ErrMobilidadeConflict)
	}
	return nil
}

func (s *MobilidadeCatalogService) ensureBrandReferencedByVehicle(ctx context.Context, brandID string) error {
	count, err := s.countActiveVehicles(ctx, bson.M{"brand_id": brandID})
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: brand is referenced by registered vehicles", ErrMobilidadeConflict)
	}
	return nil
}

func (s *MobilidadeCatalogService) ensureModelReferencedByVehicle(ctx context.Context, modelID string) error {
	count, err := s.countActiveVehicles(ctx, bson.M{"model_id": modelID})
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: model is referenced by registered vehicles", ErrMobilidadeConflict)
	}
	return nil
}

func (s *MobilidadeCatalogService) countActiveVehicles(ctx context.Context, catalogFilter bson.M) (int64, error) {
	filter := bson.M{"deleted_at": nil}
	maps.Copy(filter, catalogFilter)
	count, err := s.vehiclesColl().CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("count vehicles: %w", err)
	}
	return count, nil
}

func caseInsensitiveExactRegex(value string) bson.M {
	escaped := regexp.QuoteMeta(strings.TrimSpace(value))
	return bson.M{"$regex": fmt.Sprintf("^%s$", escaped), "$options": "i"}
}
