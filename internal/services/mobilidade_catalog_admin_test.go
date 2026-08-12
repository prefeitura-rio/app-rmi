package services

import (
	"context"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func setupMobilidadeCatalogAdminTest(t *testing.T) (*MobilidadeCatalogService, *VehicleService, func()) {
	t.Helper()
	_ = logging.InitLogger()
	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.MobilidadeVehicleCollection = "test_mobilidade_catalog_admin_vehicles"
	config.AppConfig.MobilidadeBrandCollection = "test_mobilidade_catalog_admin_brands"
	config.AppConfig.MobilidadeModelCollection = "test_mobilidade_catalog_admin_models"

	ctx := context.Background()
	db := config.MongoDB
	require.NotNil(t, db)

	catalog := NewMobilidadeCatalogService(db, logging.GetLogger())
	vehicleSvc := NewVehicleService(db, NewDataManager(config.Redis, db, logging.GetLogger()), logging.GetLogger())

	cleanup := func() {
		_ = db.Collection(config.AppConfig.MobilidadeBrandCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeModelCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeVehicleCollection).Drop(ctx)
	}
	cleanup()
	seedMobilidadeCatalog(t)

	return catalog, vehicleSvc, cleanup
}

func TestMobilidadeCatalogAdmin_CreateBrandAndModel(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	brand, err := catalog.CreateBrand(context.Background(), &models.VehicleBrandCreateRequest{Name: "Monark"})
	require.NoError(t, err)
	assert.Equal(t, "brand_monark", brand.ID)
	assert.False(t, brand.IsOther)

	model, err := catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID:     brand.ID,
		Name:        "E-Bike X",
		VehicleType: models.VehicleTypeBicicletaEletrica,
	})
	require.NoError(t, err)
	assert.Equal(t, "model_monark_e_bike_x", model.ID)
	assert.Equal(t, brand.ID, model.BrandID)

	brands, err := catalog.ListBrands(context.Background())
	require.NoError(t, err)
	ids := map[string]bool{}
	for _, b := range brands {
		ids[b.ID] = true
	}
	assert.True(t, ids["brand_monark"])

	modelsList, err := catalog.ListModelsByBrand(context.Background(), brand.ID)
	require.NoError(t, err)
	require.Len(t, modelsList, 1)
	assert.Equal(t, model.ID, modelsList[0].ID)
}

func TestMobilidadeCatalogAdmin_UniqueNames(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	_, err := catalog.CreateBrand(context.Background(), &models.VehicleBrandCreateRequest{Name: "Caloi"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeConflict)

	_, err = catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID: "brand_caloi", Name: "E-Vibe", VehicleType: models.VehicleTypeBicicletaEletrica,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeConflict)
}

func TestMobilidadeCatalogAdmin_SentinelProtection(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	_, err := catalog.UpdateBrand(context.Background(), models.VehicleBrandOutroID, &models.VehicleBrandUpdateRequest{Name: "X"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	err = catalog.DeleteBrand(context.Background(), models.VehicleBrandOutroID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	_, err = catalog.UpdateModel(context.Background(), models.VehicleModelOutroID, &models.VehicleModelUpdateRequest{
		Name: func() *string { s := "X"; return &s }(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	err = catalog.DeleteModel(context.Background(), models.VehicleModelOutroID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)
}

func TestMobilidadeCatalogAdmin_DeleteBrandWithModelsConflict(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	err := catalog.DeleteBrand(context.Background(), "brand_caloi")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeConflict)
}

func TestMobilidadeCatalogAdmin_SoftDeleteHidesFromPublicList(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	brand, err := catalog.CreateBrand(context.Background(), &models.VehicleBrandCreateRequest{Name: "Temp Brand"})
	require.NoError(t, err)

	require.NoError(t, catalog.DeleteBrand(context.Background(), brand.ID))

	public, err := catalog.ListBrands(context.Background())
	require.NoError(t, err)
	for _, b := range public {
		assert.NotEqual(t, brand.ID, b.ID)
	}

	admin, err := catalog.ListBrandsAdmin(context.Background())
	require.NoError(t, err)
	found := false
	for _, b := range admin {
		if b.ID == brand.ID {
			found = true
			assert.NotNil(t, b.DeletedAt)
		}
	}
	assert.True(t, found)
}

func TestMobilidadeCatalogAdmin_BlockMutationsWhenReferencedByVehicle(t *testing.T) {
	catalog, vehicleSvc, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()
	seedMobilidadeCitizen(t, mobilidadeOwnerCPF, "Ana Souza")

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	require.NotNil(t, created.BrandID)
	require.NotNil(t, created.ModelID)

	_, err = catalog.UpdateBrand(context.Background(), *created.BrandID, &models.VehicleBrandUpdateRequest{Name: "Caloi Nova"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeConflict)

	err = catalog.DeleteBrand(context.Background(), *created.BrandID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeConflict)

	newName := "E-Vibe Pro"
	_, err = catalog.UpdateModel(context.Background(), *created.ModelID, &models.VehicleModelUpdateRequest{Name: &newName})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeConflict)

	err = catalog.DeleteModel(context.Background(), *created.ModelID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeConflict)
}

func TestMobilidadeCatalogAdmin_DeleteBrandAfterModelsRemoved(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	brand, err := catalog.CreateBrand(context.Background(), &models.VehicleBrandCreateRequest{Name: "Solo Brand"})
	require.NoError(t, err)
	model, err := catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID: brand.ID, Name: "Solo Model", VehicleType: models.VehicleTypeCiclomotor,
	})
	require.NoError(t, err)

	require.NoError(t, catalog.DeleteModel(context.Background(), model.ID))
	require.NoError(t, catalog.DeleteBrand(context.Background(), brand.ID))

	_, err = catalog.findActiveBrand(context.Background(), brand.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogBrandNotFound)

	var stored bson.M
	err = config.MongoDB.Collection(config.AppConfig.MobilidadeBrandCollection).FindOne(
		context.Background(), bson.M{"_id": brand.ID},
	).Decode(&stored)
	require.NoError(t, err)
	assert.NotNil(t, stored["deleted_at"])
}

func TestMobilidadeCatalogAdmin_NotFoundAndValidation(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	_, err := catalog.UpdateBrand(context.Background(), "brand_missing", &models.VehicleBrandUpdateRequest{Name: "X"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogBrandNotFound)

	err = catalog.DeleteBrand(context.Background(), "brand_missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogBrandNotFound)

	_, err = catalog.UpdateModel(context.Background(), "model_missing", &models.VehicleModelUpdateRequest{
		Name: func() *string { s := "X"; return &s }(),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogModelNotFound)

	err = catalog.DeleteModel(context.Background(), "model_missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogModelNotFound)

	_, err = catalog.CreateBrand(context.Background(), &models.VehicleBrandCreateRequest{Name: "   "})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	_, err = catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID: "brand_caloi", Name: "Bad", VehicleType: models.VehicleType("invalid"),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	_, err = catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID: "brand_does_not_exist", Name: "X", VehicleType: models.VehicleTypeCiclomotor,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogBrandNotFound)

	_, err = catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID: models.VehicleBrandOutroID, Name: "X", VehicleType: models.VehicleTypeCiclomotor,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	outroBrand := models.VehicleBrandOutroID
	_, err = catalog.UpdateModel(context.Background(), "model_e-vibe", &models.VehicleModelUpdateRequest{
		BrandID: &outroBrand,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	emptyName := "   "
	_, err = catalog.UpdateModel(context.Background(), "model_e-vibe", &models.VehicleModelUpdateRequest{
		Name: &emptyName,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	_, err = catalog.UpdateModel(context.Background(), "model_e-vibe", &models.VehicleModelUpdateRequest{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)
}

func TestMobilidadeCatalogAdmin_SoftDeletedNotFoundOnMutate(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	brand, err := catalog.CreateBrand(context.Background(), &models.VehicleBrandCreateRequest{Name: "Ghost Brand"})
	require.NoError(t, err)
	require.NoError(t, catalog.DeleteBrand(context.Background(), brand.ID))

	_, err = catalog.UpdateBrand(context.Background(), brand.ID, &models.VehicleBrandUpdateRequest{Name: "Still Ghost"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogBrandNotFound)

	err = catalog.DeleteBrand(context.Background(), brand.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogBrandNotFound)

	model, err := catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID: "brand_caloi", Name: "Ghost Model", VehicleType: models.VehicleTypeAutopropelido,
	})
	require.NoError(t, err)
	require.NoError(t, catalog.DeleteModel(context.Background(), model.ID))

	newName := "Still Ghost"
	_, err = catalog.UpdateModel(context.Background(), model.ID, &models.VehicleModelUpdateRequest{Name: &newName})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogModelNotFound)

	err = catalog.DeleteModel(context.Background(), model.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCatalogModelNotFound)
}

func TestMobilidadeCatalogAdmin_SoftDeleteModelHidesFromPublicList(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	model, err := catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID: "brand_caloi", Name: "Temp Model", VehicleType: models.VehicleTypeCiclomotor,
	})
	require.NoError(t, err)
	require.NoError(t, catalog.DeleteModel(context.Background(), model.ID))

	public, err := catalog.ListModelsByBrand(context.Background(), "brand_caloi")
	require.NoError(t, err)
	for _, m := range public {
		assert.NotEqual(t, model.ID, m.ID)
	}

	admin, err := catalog.ListModelsAdmin(context.Background(), "brand_caloi")
	require.NoError(t, err)
	found := false
	for _, m := range admin {
		if m.ID == model.ID {
			found = true
			assert.NotNil(t, m.DeletedAt)
		}
	}
	assert.True(t, found)
}

func TestMobilidadeCatalogAdmin_UpdateBrandAndModelSuccess(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	brand, err := catalog.CreateBrand(context.Background(), &models.VehicleBrandCreateRequest{Name: "Oggi"})
	require.NoError(t, err)
	updatedBrand, err := catalog.UpdateBrand(context.Background(), brand.ID, &models.VehicleBrandUpdateRequest{Name: "Oggi Bikes"})
	require.NoError(t, err)
	assert.Equal(t, "Oggi Bikes", updatedBrand.Name)

	model, err := catalog.CreateModel(context.Background(), &models.VehicleModelCreateRequest{
		BrandID: brand.ID, Name: "Hacker", VehicleType: models.VehicleTypeBicicletaEletrica,
	})
	require.NoError(t, err)

	newType := models.VehicleTypeCiclomotor
	newName := "Hacker Pro"
	updatedModel, err := catalog.UpdateModel(context.Background(), model.ID, &models.VehicleModelUpdateRequest{
		Name: &newName, VehicleType: &newType, BrandID: &brand.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, "Hacker Pro", updatedModel.Name)
	assert.Equal(t, models.VehicleTypeCiclomotor, updatedModel.VehicleType)
	assert.Equal(t, brand.ID, updatedModel.BrandID)
}

func TestMobilidadeCatalogAdmin_MoveModelToOtherBrand(t *testing.T) {
	catalog, _, cleanup := setupMobilidadeCatalogAdminTest(t)
	defer cleanup()

	target, err := catalog.CreateBrand(context.Background(), &models.VehicleBrandCreateRequest{Name: "Sense"})
	require.NoError(t, err)

	moved, err := catalog.UpdateModel(context.Background(), "model_e-vibe", &models.VehicleModelUpdateRequest{
		BrandID: &target.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, target.ID, moved.BrandID)

	caloiModels, err := catalog.ListModelsByBrand(context.Background(), "brand_caloi")
	require.NoError(t, err)
	for _, m := range caloiModels {
		assert.NotEqual(t, "model_e-vibe", m.ID)
	}
}
