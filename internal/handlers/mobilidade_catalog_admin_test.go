package handlers

import (
	"bytes"
	"context"
	"encoding/json"
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

func setupMobilidadeCatalogAdminHandlersTest(t *testing.T) (*gin.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	config.AppConfig.MobilidadeBrandCollection = "test_mobilidade_admin_handlers_brands"
	config.AppConfig.MobilidadeModelCollection = "test_mobilidade_admin_handlers_models"
	config.AppConfig.MobilidadeVehicleCollection = "test_mobilidade_admin_handlers_vehicles"

	ctx := context.Background()
	db := config.MongoDB
	require.NotNil(t, db)

	services.MobilidadeCatalogServiceInstance = services.NewMobilidadeCatalogService(db, logging.GetLogger())
	services.VehicleServiceInstance = services.NewVehicleService(db, services.NewDataManager(config.Redis, db, logging.GetLogger()), logging.GetLogger())

	router := gin.New()
	router.GET("/mobilidade/vehicle-brands", GetMobilidadeVehicleBrands)
	router.GET("/mobilidade/vehicle-models", GetMobilidadeVehicleModels)
	router.GET("/admin/mobilidade/vehicle-brands", AdminListMobilidadeVehicleBrands)
	router.POST("/admin/mobilidade/vehicle-brands", CreateMobilidadeVehicleBrand)
	router.PATCH("/admin/mobilidade/vehicle-brands/:brand_id", UpdateMobilidadeVehicleBrand)
	router.DELETE("/admin/mobilidade/vehicle-brands/:brand_id", DeleteMobilidadeVehicleBrand)
	router.GET("/admin/mobilidade/vehicle-models", AdminListMobilidadeVehicleModels)
	router.POST("/admin/mobilidade/vehicle-models", CreateMobilidadeVehicleModel)
	router.PATCH("/admin/mobilidade/vehicle-models/:model_id", UpdateMobilidadeVehicleModel)
	router.DELETE("/admin/mobilidade/vehicle-models/:model_id", DeleteMobilidadeVehicleModel)

	cleanupDB := func() {
		_ = db.Collection(config.AppConfig.MobilidadeBrandCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeModelCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeVehicleCollection).Drop(ctx)
	}
	cleanupDB()
	seedAdminHandlerCatalog(t)

	cleanup := func() {
		cleanupDB()
		services.MobilidadeCatalogServiceInstance = nil
		services.VehicleServiceInstance = nil
	}
	return router, cleanup
}

func seedAdminHandlerCatalog(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := config.MongoDB.Collection(config.AppConfig.MobilidadeBrandCollection).InsertMany(ctx, []interface{}{
		bson.M{"_id": "brand_caloi", "name": "Caloi", "is_other": false},
		bson.M{"_id": models.VehicleBrandOutroID, "name": "Outro", "is_other": true},
	})
	require.NoError(t, err)
	_, err = config.MongoDB.Collection(config.AppConfig.MobilidadeModelCollection).InsertMany(ctx, []interface{}{
		bson.M{
			"_id": "model_e-vibe", "brand_id": "brand_caloi", "name": "E-Vibe",
			"vehicle_type": "bicicleta_eletrica", "is_other": false,
		},
		bson.M{
			"_id": models.VehicleModelOutroID, "brand_id": models.VehicleBrandOutroID, "name": "Outro",
			"vehicle_type": "autopropelido", "is_other": true,
		},
	})
	require.NoError(t, err)
}

func TestAdminMobilidadeCatalog_CreateBrand_Model_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	createBrandBody, _ := json.Marshal(models.VehicleBrandCreateRequest{Name: "Monark"})
	req := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-brands", bytes.NewReader(createBrandBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var brand models.VehicleBrand
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &brand))
	assert.Equal(t, "brand_monark", brand.ID)

	createModelBody, _ := json.Marshal(models.VehicleModelCreateRequest{
		BrandID: brand.ID, Name: "City", VehicleType: models.VehicleTypeAutopropelido,
	})
	req2 := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-models", bytes.NewReader(createModelBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusCreated, w2.Code, w2.Body.String())

	publicReq := httptest.NewRequest(http.MethodGet, "/mobilidade/vehicle-brands", nil)
	publicW := httptest.NewRecorder()
	router.ServeHTTP(publicW, publicReq)
	require.Equal(t, http.StatusOK, publicW.Code)
	var public models.VehicleBrandsResponse
	require.NoError(t, json.Unmarshal(publicW.Body.Bytes(), &public))
	found := false
	for _, b := range public.Data {
		if b.ID == brand.ID {
			found = true
		}
	}
	assert.True(t, found)
}

func TestAdminMobilidadeCatalog_DeleteSentinel_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-brands/"+models.VehicleBrandOutroID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	req2 := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-models/"+models.VehicleModelOutroID, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)
}

func TestAdminMobilidadeCatalog_DuplicateBrand_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	body, _ := json.Marshal(models.VehicleBrandCreateRequest{Name: "Caloi"})
	req := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-brands", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAdminMobilidadeCatalog_InvalidBodies_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-brands", bytes.NewReader([]byte(`{not-json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	emptyName, _ := json.Marshal(map[string]string{"name": ""})
	req2 := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-brands", bytes.NewReader(emptyName))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusBadRequest, w2.Code)

	badType, _ := json.Marshal(models.VehicleModelCreateRequest{
		BrandID: "brand_caloi", Name: "X", VehicleType: models.VehicleType("nope"),
	})
	req3 := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-models", bytes.NewReader(badType))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusBadRequest, w3.Code)
}

func TestAdminMobilidadeCatalog_NotFound_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	body, _ := json.Marshal(models.VehicleBrandUpdateRequest{Name: "X"})
	req := httptest.NewRequest(http.MethodPatch, "/admin/mobilidade/vehicle-brands/brand_missing", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	del := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-brands/brand_missing", nil)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, del)
	assert.Equal(t, http.StatusNotFound, delW.Code)

	modelBody, _ := json.Marshal(map[string]string{"name": "X"})
	req2 := httptest.NewRequest(http.MethodPatch, "/admin/mobilidade/vehicle-models/model_missing", bytes.NewReader(modelBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code)

	del2 := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-models/model_missing", nil)
	delW2 := httptest.NewRecorder()
	router.ServeHTTP(delW2, del2)
	assert.Equal(t, http.StatusNotFound, delW2.Code)

	createOnMissingBrand, _ := json.Marshal(models.VehicleModelCreateRequest{
		BrandID: "brand_missing", Name: "X", VehicleType: models.VehicleTypeCiclomotor,
	})
	req3 := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-models", bytes.NewReader(createOnMissingBrand))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusNotFound, w3.Code)
}

func TestAdminMobilidadeCatalog_DeleteBrandWithModels_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-brands/brand_caloi", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestAdminMobilidadeCatalog_PatchDeleteModelAndSoftDeleteVisibility_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	createBrand, _ := json.Marshal(models.VehicleBrandCreateRequest{Name: "Oggi"})
	req := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-brands", bytes.NewReader(createBrand))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var brand models.VehicleBrand
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &brand))

	patchBrand, _ := json.Marshal(models.VehicleBrandUpdateRequest{Name: "Oggi Bikes"})
	patchReq := httptest.NewRequest(http.MethodPatch, "/admin/mobilidade/vehicle-brands/"+brand.ID, bytes.NewReader(patchBrand))
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	router.ServeHTTP(patchW, patchReq)
	require.Equal(t, http.StatusOK, patchW.Code, patchW.Body.String())

	createModel, _ := json.Marshal(models.VehicleModelCreateRequest{
		BrandID: brand.ID, Name: "Hacker", VehicleType: models.VehicleTypeBicicletaEletrica,
	})
	modelReq := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-models", bytes.NewReader(createModel))
	modelReq.Header.Set("Content-Type", "application/json")
	modelW := httptest.NewRecorder()
	router.ServeHTTP(modelW, modelReq)
	require.Equal(t, http.StatusCreated, modelW.Code, modelW.Body.String())
	var model models.VehicleModel
	require.NoError(t, json.Unmarshal(modelW.Body.Bytes(), &model))

	patchModel, _ := json.Marshal(map[string]interface{}{
		"name":         "Hacker Pro",
		"vehicle_type": string(models.VehicleTypeCiclomotor),
	})
	pmReq := httptest.NewRequest(http.MethodPatch, "/admin/mobilidade/vehicle-models/"+model.ID, bytes.NewReader(patchModel))
	pmReq.Header.Set("Content-Type", "application/json")
	pmW := httptest.NewRecorder()
	router.ServeHTTP(pmW, pmReq)
	require.Equal(t, http.StatusOK, pmW.Code, pmW.Body.String())

	delModel := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-models/"+model.ID, nil)
	delModelW := httptest.NewRecorder()
	router.ServeHTTP(delModelW, delModel)
	require.Equal(t, http.StatusNoContent, delModelW.Code)

	publicModels := httptest.NewRequest(http.MethodGet, "/mobilidade/vehicle-models?brand_id="+brand.ID, nil)
	publicMW := httptest.NewRecorder()
	router.ServeHTTP(publicMW, publicModels)
	require.Equal(t, http.StatusOK, publicMW.Code)
	var modelsResp models.VehicleModelsResponse
	require.NoError(t, json.Unmarshal(publicMW.Body.Bytes(), &modelsResp))
	assert.Empty(t, modelsResp.Data)

	adminModels := httptest.NewRequest(http.MethodGet, "/admin/mobilidade/vehicle-models?brand_id="+brand.ID, nil)
	adminMW := httptest.NewRecorder()
	router.ServeHTTP(adminMW, adminModels)
	require.Equal(t, http.StatusOK, adminMW.Code)
	var adminResp models.VehicleModelsResponse
	require.NoError(t, json.Unmarshal(adminMW.Body.Bytes(), &adminResp))
	require.Len(t, adminResp.Data, 1)
	assert.NotNil(t, adminResp.Data[0].DeletedAt)

	delBrand := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-brands/"+brand.ID, nil)
	delBrandW := httptest.NewRecorder()
	router.ServeHTTP(delBrandW, delBrand)
	require.Equal(t, http.StatusNoContent, delBrandW.Code)

	publicBrands := httptest.NewRequest(http.MethodGet, "/mobilidade/vehicle-brands", nil)
	publicBW := httptest.NewRecorder()
	router.ServeHTTP(publicBW, publicBrands)
	require.Equal(t, http.StatusOK, publicBW.Code)
	var brandsResp models.VehicleBrandsResponse
	require.NoError(t, json.Unmarshal(publicBW.Body.Bytes(), &brandsResp))
	for _, b := range brandsResp.Data {
		assert.NotEqual(t, brand.ID, b.ID)
	}
}

func TestAdminMobilidadeCatalog_ConflictWhenReferencedByVehicle_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	seedHandlerCitizen(t, handlerOwnerCPF, "Ana Owner")

	falseVal := false
	created, err := services.VehicleServiceInstance.CreateVehicle(context.Background(), handlerOwnerCPF, &models.VehicleCreateRequest{
		DisplayName: "Bike", BrandID: strPtr("brand_caloi"), ModelID: strPtr("model_e-vibe"),
		Color: "Preto", SerialNumber: "SN", SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg",
		VehiclePhotoURL: "https://storage.googleapis.com/v.jpg", HasInvoice: &falseVal, SelfDeclaration: true,
	})
	require.NoError(t, err)
	require.NotNil(t, created.ModelID)

	patchBrand, _ := json.Marshal(models.VehicleBrandUpdateRequest{Name: "Caloi Nova"})
	req := httptest.NewRequest(http.MethodPatch, "/admin/mobilidade/vehicle-brands/brand_caloi", bytes.NewReader(patchBrand))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)

	delBrand := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-brands/brand_caloi", nil)
	delBW := httptest.NewRecorder()
	router.ServeHTTP(delBW, delBrand)
	assert.Equal(t, http.StatusConflict, delBW.Code)

	patchModel, _ := json.Marshal(map[string]string{"name": "E-Vibe Pro"})
	pmReq := httptest.NewRequest(http.MethodPatch, "/admin/mobilidade/vehicle-models/model_e-vibe", bytes.NewReader(patchModel))
	pmReq.Header.Set("Content-Type", "application/json")
	pmW := httptest.NewRecorder()
	router.ServeHTTP(pmW, pmReq)
	assert.Equal(t, http.StatusConflict, pmW.Code)

	delModel := httptest.NewRequest(http.MethodDelete, "/admin/mobilidade/vehicle-models/model_e-vibe", nil)
	delMW := httptest.NewRecorder()
	router.ServeHTTP(delMW, delModel)
	assert.Equal(t, http.StatusConflict, delMW.Code)
}

func TestAdminMobilidadeCatalog_CreateModelOnSentinelBrand_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeCatalogAdminHandlersTest(t)
	defer cleanup()

	body, _ := json.Marshal(models.VehicleModelCreateRequest{
		BrandID: models.VehicleBrandOutroID, Name: "X", VehicleType: models.VehicleTypeAutopropelido,
	})
	req := httptest.NewRequest(http.MethodPost, "/admin/mobilidade/vehicle-models", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
