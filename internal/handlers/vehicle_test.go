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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	handlerOwnerCPF     = "03561350712"
	handlerConductorCPF = "45049725810"
)

func setupMobilidadeHandlersTest(t *testing.T) (*gin.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	config.AppConfig.RioMobVehicleCollection = "test_riomob_vehicles_handlers"
	config.AppConfig.RioMobConductorCollection = "test_riomob_conductors_handlers"
	config.AppConfig.RioMobBrandCollection = "test_riomob_brands_handlers"
	config.AppConfig.RioMobModelCollection = "test_riomob_models_handlers"

	ctx := context.Background()
	db := config.MongoDB
	require.NotNil(t, db)

	services.VehicleServiceInstance = services.NewVehicleService(db, services.NewDataManager(config.Redis, db, logging.GetLogger()), logging.GetLogger())
	services.VehicleConductorServiceInstance = services.NewVehicleConductorService(db, logging.GetLogger())
	services.RioMobCatalogServiceInstance = services.NewRioMobCatalogService(db, logging.GetLogger())

	router := gin.New()
	router.GET("/citizen/:cpf/vehicles", GetVehicles)
	router.POST("/citizen/:cpf/vehicles", CreateVehicle)
	router.GET("/citizen/:cpf/vehicles/:vehicle_id", GetVehicle)
	router.PATCH("/citizen/:cpf/vehicles/:vehicle_id", UpdateVehicle)
	router.DELETE("/citizen/:cpf/vehicles/:vehicle_id", DeleteVehicle)
	router.GET("/citizen/:cpf/vehicle-invitations", GetVehicleInvitations)
	router.PATCH("/citizen/:cpf/vehicle-invitations/:conductor_id", RespondVehicleInvitation)
	router.GET("/citizen/:cpf/vehicles/:vehicle_id/conductors", GetVehicleConductors)
	router.POST("/citizen/:cpf/vehicles/:vehicle_id/conductors", InviteVehicleConductor)
	router.DELETE("/citizen/:cpf/vehicles/:vehicle_id/conductors/:conductor_id", RemoveVehicleConductor)
	router.GET("/mobilidade/vehicle-brands", GetMobilidadeVehicleBrands)
	router.GET("/mobilidade/vehicle-models", GetMobilidadeVehicleModels)
	router.GET("/mobilidade/vehicle-colors", GetMobilidadeVehicleColors)

	// Fresh collections for this test (do not nil service instances here).
	_ = db.Collection(config.AppConfig.RioMobVehicleCollection).Drop(ctx)
	_ = db.Collection(config.AppConfig.RioMobConductorCollection).Drop(ctx)
	_ = db.Collection(config.AppConfig.RioMobBrandCollection).Drop(ctx)
	_ = db.Collection(config.AppConfig.RioMobModelCollection).Drop(ctx)

	cleanup := func() {
		_ = db.Collection(config.AppConfig.RioMobVehicleCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.RioMobConductorCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.RioMobBrandCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.RioMobModelCollection).Drop(ctx)
		services.VehicleServiceInstance = nil
		services.VehicleConductorServiceInstance = nil
		services.RioMobCatalogServiceInstance = nil
	}
	seedHandlerCitizen(t, handlerOwnerCPF, "Ana Souza")

	return router, cleanup
}

func seedHandlerCitizen(t *testing.T, cpf, name string) {
	t.Helper()
	ctx := context.Background()
	coll := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, _ = coll.DeleteMany(ctx, bson.M{"cpf": cpf})
	_, err := coll.InsertOne(ctx, bson.M{"_id": cpf, "cpf": cpf, "nome": name})
	require.NoError(t, err)
}

func seedHandlerCatalog(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := config.MongoDB.Collection(config.AppConfig.RioMobBrandCollection).InsertMany(ctx, []interface{}{
		bson.M{"_id": "brand_caloi", "name": "Caloi", "is_other": false},
	})
	require.NoError(t, err)
	_, err = config.MongoDB.Collection(config.AppConfig.RioMobModelCollection).InsertMany(ctx, []interface{}{
		bson.M{
			"_id": "model_e-vibe", "brand_id": "brand_caloi", "name": "E-Vibe",
			"vehicle_type": "bicicleta_eletrica", "is_other": false,
		},
	})
	require.NoError(t, err)
}

func TestGetVehicles_InvalidCPF(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/citizen/12345/vehicles?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetVehicles_EmptyList(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerOwnerCPF+"/vehicles?page=1&per_page=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var resp models.PaginatedVehicles
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Pagination.Total)
	assert.Equal(t, 1, resp.Pagination.Page)
	assert.Equal(t, 10, resp.Pagination.PerPage)
}

func TestCreateVehicle_AndList(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	body := map[string]interface{}{
		"display_name":            "Bike do trabalho",
		"brand_id":                "brand_caloi",
		"model_id":                "model_e-vibe",
		"color":                   "Preto",
		"serial_number":           "SN-1",
		"serial_number_photo_url": "https://storage.googleapis.com/s.jpg",
		"vehicle_photo_url":       "https://storage.googleapis.com/v.jpg",
		"has_invoice":             true,
		"invoice_photo_url":       "https://storage.googleapis.com/bucket/nf.pdf",
		"self_declaration":        true,
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/citizen/"+handlerOwnerCPF+"/vehicles", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	var created models.VehicleDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, handlerOwnerCPF, created.OwnerCPF)
	assert.Equal(t, models.VehicleTypeBicicletaEletrica, created.VehicleType)
	assert.Equal(t, models.VehicleRoleOwner, created.Role)
	require.NotNil(t, created.InvoicePhotoURL)
	assert.Equal(t, "https://storage.googleapis.com/bucket/nf.pdf", *created.InvoicePhotoURL)
	assert.True(t, created.HasInvoice)

	listReq := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerOwnerCPF+"/vehicles?page=1&per_page=10", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	require.Equal(t, http.StatusOK, listW.Code)
	var list models.PaginatedVehicles
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &list))
	require.Equal(t, 1, list.Pagination.Total)
	assert.Equal(t, models.VehicleRoleOwner, list.Data[0].Role)
}

func TestCreateVehicle_RejectsFalseSelfDeclaration(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	body := map[string]interface{}{
		"display_name":            "Bike",
		"brand_id":                "brand_caloi",
		"model_id":                "model_e-vibe",
		"color":                   "Preto",
		"serial_number":           "SN-1",
		"serial_number_photo_url": "https://storage.googleapis.com/s.jpg",
		"vehicle_photo_url":       "https://storage.googleapis.com/v.jpg",
		"self_declaration":        false,
	}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/citizen/"+handlerOwnerCPF+"/vehicles", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetVehicleModels_RequiresBrandID(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/mobilidade/vehicle-models", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetVehicleColors(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/mobilidade/vehicle-colors", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		Data []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, models.VehicleColors(), resp.Data)
}

func TestGetVehicleBrands(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	req := httptest.NewRequest(http.MethodGet, "/mobilidade/vehicle-brands", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		Data []models.VehicleBrand `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "Caloi", resp.Data[0].Name)
}

func TestConductorInviteAcceptFlow_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	createBody, _ := json.Marshal(map[string]interface{}{
		"display_name":            "Bike",
		"brand_id":                "brand_caloi",
		"model_id":                "model_e-vibe",
		"color":                   "Preto",
		"serial_number":           "SN-1",
		"serial_number_photo_url": "https://storage.googleapis.com/s.jpg",
		"vehicle_photo_url":       "https://storage.googleapis.com/v.jpg",
		"has_invoice":             false,
		"self_declaration":        true,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/citizen/"+handlerOwnerCPF+"/vehicles", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code, "body: %s", createW.Body.String())

	var vehicle models.VehicleDetail
	require.NoError(t, json.Unmarshal(createW.Body.Bytes(), &vehicle))

	inviteBody, _ := json.Marshal(map[string]string{
		"cpf":   handlerConductorCPF,
		"name":  "João",
		"email": "joao@example.com",
	})
	inviteReq := httptest.NewRequest(http.MethodPost, "/citizen/"+handlerOwnerCPF+"/vehicles/"+vehicle.ID.Hex()+"/conductors", bytes.NewReader(inviteBody))
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteW := httptest.NewRecorder()
	router.ServeHTTP(inviteW, inviteReq)
	require.Equal(t, http.StatusCreated, inviteW.Code, "body: %s", inviteW.Body.String())

	var link models.VehicleConductor
	require.NoError(t, json.Unmarshal(inviteW.Body.Bytes(), &link))
	assert.Equal(t, models.ConductorStatusPending, link.Status)

	invitesReq := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerConductorCPF+"/vehicle-invitations", nil)
	invitesW := httptest.NewRecorder()
	router.ServeHTTP(invitesW, invitesReq)
	require.Equal(t, http.StatusOK, invitesW.Code, "body: %s", invitesW.Body.String())
	var invites models.VehicleInvitationsResponse
	require.NoError(t, json.Unmarshal(invitesW.Body.Bytes(), &invites))
	require.Len(t, invites.Data, 1)

	acceptBody, _ := json.Marshal(map[string]string{"status": "accepted"})
	acceptReq := httptest.NewRequest(http.MethodPatch, "/citizen/"+handlerConductorCPF+"/vehicle-invitations/"+link.ID.Hex(), bytes.NewReader(acceptBody))
	acceptReq.Header.Set("Content-Type", "application/json")
	acceptW := httptest.NewRecorder()
	router.ServeHTTP(acceptW, acceptReq)
	require.Equal(t, http.StatusOK, acceptW.Code, "body: %s", acceptW.Body.String())

	listReq := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerConductorCPF+"/vehicles?page=1&per_page=10", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	require.Equal(t, http.StatusOK, listW.Code)
	var list models.PaginatedVehicles
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &list))
	require.Equal(t, 1, list.Pagination.Total)
	assert.Equal(t, models.VehicleRoleConductor, list.Data[0].Role)
}

func TestInviteVehicleConductor_DuplicateReturnsConflict(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	// Ensure unique index exists on the handler test collection too.
	ctx := context.Background()
	_, _ = config.MongoDB.Collection(config.AppConfig.RioMobConductorCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "vehicle_id", Value: 1},
			{Key: "conductor_cpf", Value: 1},
		},
		Options: options.Index().
			SetName("idx_riomob_conductors_vehicle_cpf_active").
			SetUnique(true).
			SetPartialFilterExpression(bson.M{
				"status": bson.M{"$in": bson.A{"pending", "accepted"}},
			}),
	})

	createBody, _ := json.Marshal(map[string]interface{}{
		"display_name":            "Bike",
		"brand_id":                "brand_caloi",
		"model_id":                "model_e-vibe",
		"color":                   "Preto",
		"serial_number":           "SN-1",
		"serial_number_photo_url": "https://storage.googleapis.com/s.jpg",
		"vehicle_photo_url":       "https://storage.googleapis.com/v.jpg",
		"has_invoice":             false,
		"self_declaration":        true,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/citizen/"+handlerOwnerCPF+"/vehicles", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	router.ServeHTTP(createW, createReq)
	require.Equal(t, http.StatusCreated, createW.Code, "body: %s", createW.Body.String())

	var vehicle models.VehicleDetail
	require.NoError(t, json.Unmarshal(createW.Body.Bytes(), &vehicle))

	inviteBody, _ := json.Marshal(map[string]string{
		"cpf":   handlerConductorCPF,
		"name":  "João",
		"email": "joao@example.com",
	})
	inviteURL := "/citizen/" + handlerOwnerCPF + "/vehicles/" + vehicle.ID.Hex() + "/conductors"

	firstReq := httptest.NewRequest(http.MethodPost, inviteURL, bytes.NewReader(inviteBody))
	firstReq.Header.Set("Content-Type", "application/json")
	firstW := httptest.NewRecorder()
	router.ServeHTTP(firstW, firstReq)
	require.Equal(t, http.StatusCreated, firstW.Code, "body: %s", firstW.Body.String())

	secondReq := httptest.NewRequest(http.MethodPost, inviteURL, bytes.NewReader(inviteBody))
	secondReq.Header.Set("Content-Type", "application/json")
	secondW := httptest.NewRecorder()
	router.ServeHTTP(secondW, secondReq)
	assert.Equal(t, http.StatusConflict, secondW.Code, "body: %s", secondW.Body.String())
}

func TestUpdateVehicle_ConductorForbidden(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	// Create as owner via service for setup speed
	created, err := services.VehicleServiceInstance.CreateVehicle(context.Background(), handlerOwnerCPF, &models.VehicleCreateRequest{
		DisplayName: "Bike", BrandID: strPtr("brand_caloi"), ModelID: strPtr("model_e-vibe"),
		Color: "Preto", SerialNumber: "SN", SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg", VehiclePhotoURL: "https://storage.googleapis.com/v.jpg",
		SelfDeclaration: true,
	})
	require.NoError(t, err)

	link, err := services.VehicleConductorServiceInstance.InviteConductor(context.Background(), handlerOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: handlerConductorCPF, Email: "c@example.com",
	})
	require.NoError(t, err)
	_, err = services.VehicleConductorServiceInstance.RespondInvitation(context.Background(), handlerConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.ConductorStatusAccepted,
	})
	require.NoError(t, err)

	patchBody, _ := json.Marshal(map[string]string{"display_name": "hack"})
	req := httptest.NewRequest(http.MethodPatch, "/citizen/"+handlerConductorCPF+"/vehicles/"+created.ID.Hex(), bytes.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}
