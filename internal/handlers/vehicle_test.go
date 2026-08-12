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
	handlerOwnerCPF     = "10000000019"
	handlerConductorCPF = "10000000108"
)

func setupMobilidadeHandlersTest(t *testing.T) (*gin.Engine, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	config.AppConfig.MobilidadeVehicleCollection = "test_mobilidade_vehicles_handlers"
	config.AppConfig.MobilidadeConductorCollection = "test_mobilidade_conductors_handlers"
	config.AppConfig.MobilidadeBrandCollection = "test_mobilidade_brands_handlers"
	config.AppConfig.MobilidadeModelCollection = "test_mobilidade_models_handlers"

	ctx := context.Background()
	db := config.MongoDB
	require.NotNil(t, db)

	services.VehicleServiceInstance = services.NewVehicleService(db, services.NewDataManager(config.Redis, db, logging.GetLogger()), logging.GetLogger())
	services.VehicleConductorServiceInstance = services.NewVehicleConductorService(db, services.NewDataManager(config.Redis, db, logging.GetLogger()), logging.GetLogger())
	services.MobilidadeCatalogServiceInstance = services.NewMobilidadeCatalogService(db, logging.GetLogger())

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
	_ = db.Collection(config.AppConfig.MobilidadeVehicleCollection).Drop(ctx)
	_ = db.Collection(config.AppConfig.MobilidadeConductorCollection).Drop(ctx)
	_ = db.Collection(config.AppConfig.MobilidadeBrandCollection).Drop(ctx)
	_ = db.Collection(config.AppConfig.MobilidadeModelCollection).Drop(ctx)

	seedHandlerCitizen(t, handlerOwnerCPF, "Ana Owner")
	seedHandlerCitizen(t, handlerConductorCPF, "João Condutor")
	seedHandlerCitizenEmail(t, handlerConductorCPF, "joao@example.com")

	cleanup := func() {
		_ = db.Collection(config.AppConfig.MobilidadeVehicleCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeConductorCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeBrandCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeModelCollection).Drop(ctx)
		services.VehicleServiceInstance = nil
		services.VehicleConductorServiceInstance = nil
		services.MobilidadeCatalogServiceInstance = nil
	}

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

func seedHandlerCitizenEmail(t *testing.T, cpf, email string) {
	t.Helper()
	ctx := context.Background()
	_, err := config.MongoDB.Collection(config.AppConfig.CitizenCollection).UpdateOne(ctx, bson.M{"cpf": cpf}, bson.M{
		"$set": bson.M{"email": bson.M{"principal": bson.M{"valor": email}}},
	})
	require.NoError(t, err)
}

func seedHandlerCatalog(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := config.MongoDB.Collection(config.AppConfig.MobilidadeBrandCollection).InsertMany(ctx, []interface{}{
		bson.M{"_id": "brand_caloi", "name": "Caloi", "is_other": false},
	})
	require.NoError(t, err)
	_, err = config.MongoDB.Collection(config.AppConfig.MobilidadeModelCollection).InsertMany(ctx, []interface{}{
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
		"has_invoice":             false,
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
		"email": "condutor@example.com",
		"name":  "Condutor Teste",
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
	assert.Equal(t, link.ID.Hex(), list.Data[0].ConductorID)

	detailReq := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerConductorCPF+"/vehicles/"+vehicle.ID.Hex(), nil)
	detailW := httptest.NewRecorder()
	router.ServeHTTP(detailW, detailReq)
	require.Equal(t, http.StatusOK, detailW.Code, "body: %s", detailW.Body.String())
	var detail models.VehicleDetail
	require.NoError(t, json.Unmarshal(detailW.Body.Bytes(), &detail))
	assert.Equal(t, models.VehicleRoleConductor, detail.Role)
	assert.Equal(t, link.ID.Hex(), detail.ConductorID)

	ownerListReq := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerOwnerCPF+"/vehicles?page=1&per_page=10", nil)
	ownerListW := httptest.NewRecorder()
	router.ServeHTTP(ownerListW, ownerListReq)
	require.Equal(t, http.StatusOK, ownerListW.Code)
	var ownerList models.PaginatedVehicles
	require.NoError(t, json.Unmarshal(ownerListW.Body.Bytes(), &ownerList))
	require.Equal(t, 1, ownerList.Pagination.Total)
	assert.Equal(t, models.VehicleRoleOwner, ownerList.Data[0].Role)
	assert.Empty(t, ownerList.Data[0].ConductorID)
}

func TestInviteVehicleConductor_DuplicateReturnsConflict(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	// Ensure unique index exists on the handler test collection too.
	ctx := context.Background()
	_, _ = config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "vehicle_id", Value: 1},
			{Key: "conductor_cpf", Value: 1},
		},
		Options: options.Index().
			SetName("idx_mobilidade_conductors_vehicle_cpf_active").
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
		"email": "condutor@example.com",
		"name":  "Condutor Teste",
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
	falseVal := false
	created, err := services.VehicleServiceInstance.CreateVehicle(context.Background(), handlerOwnerCPF, &models.VehicleCreateRequest{
		DisplayName: "Bike", BrandID: strPtr("brand_caloi"), ModelID: strPtr("model_e-vibe"),
		Color: "Preto", SerialNumber: "SN", SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg", VehiclePhotoURL: "https://storage.googleapis.com/v.jpg",
		HasInvoice: &falseVal, SelfDeclaration: true,
	})
	require.NoError(t, err)

	link, err := services.VehicleConductorServiceInstance.InviteConductor(context.Background(), handlerOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: handlerConductorCPF, Email: "condutor@example.com", Name: "Condutor",
	})
	require.NoError(t, err)
	_, err = services.VehicleConductorServiceInstance.RespondInvitation(context.Background(), handlerConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	patchBody, _ := json.Marshal(map[string]string{"display_name": "hack"})
	req := httptest.NewRequest(http.MethodPatch, "/citizen/"+handlerConductorCPF+"/vehicles/"+created.ID.Hex(), bytes.NewReader(patchBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetVehicle_AndDeleteVehicle_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	falseVal := false
	created, err := services.VehicleServiceInstance.CreateVehicle(context.Background(), handlerOwnerCPF, &models.VehicleCreateRequest{
		DisplayName: "Bike", BrandID: strPtr("brand_caloi"), ModelID: strPtr("model_e-vibe"),
		Color: "Preto", SerialNumber: "SN", SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg", VehiclePhotoURL: "https://storage.googleapis.com/v.jpg",
		HasInvoice: &falseVal, SelfDeclaration: true,
	})
	require.NoError(t, err)

	getReq := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerOwnerCPF+"/vehicles/"+created.ID.Hex(), nil)
	getW := httptest.NewRecorder()
	router.ServeHTTP(getW, getReq)
	require.Equal(t, http.StatusOK, getW.Code, "body: %s", getW.Body.String())

	var detail models.VehicleDetail
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &detail))
	assert.Equal(t, created.ID.Hex(), detail.ID.Hex())
	assert.Equal(t, models.VehicleRoleOwner, detail.Role)

	delReq := httptest.NewRequest(http.MethodDelete, "/citizen/"+handlerOwnerCPF+"/vehicles/"+created.ID.Hex(), nil)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)
	require.Equal(t, http.StatusNoContent, delW.Code)

	missingReq := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerOwnerCPF+"/vehicles/"+created.ID.Hex(), nil)
	missingW := httptest.NewRecorder()
	router.ServeHTTP(missingW, missingReq)
	assert.Equal(t, http.StatusNotFound, missingW.Code)
}

func TestGetAndRemoveVehicleConductors_HTTP(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	falseVal := false
	created, err := services.VehicleServiceInstance.CreateVehicle(context.Background(), handlerOwnerCPF, &models.VehicleCreateRequest{
		DisplayName: "Bike", BrandID: strPtr("brand_caloi"), ModelID: strPtr("model_e-vibe"),
		Color: "Preto", SerialNumber: "SN", SerialNumberPhotoURL: "https://storage.googleapis.com/s.jpg", VehiclePhotoURL: "https://storage.googleapis.com/v.jpg",
		HasInvoice: &falseVal, SelfDeclaration: true,
	})
	require.NoError(t, err)

	link, err := services.VehicleConductorServiceInstance.InviteConductor(context.Background(), handlerOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: handlerConductorCPF, Email: "condutor@example.com", Name: "Condutor",
	})
	require.NoError(t, err)

	listReq := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerOwnerCPF+"/vehicles/"+created.ID.Hex()+"/conductors", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)
	require.Equal(t, http.StatusOK, listW.Code, "body: %s", listW.Body.String())
	var list models.ConductorsListResponse
	require.NoError(t, json.Unmarshal(listW.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	assert.Equal(t, link.ID.Hex(), list.Data[0].ID.Hex())
	assert.Equal(t, models.ConductorStatusPending, list.Data[0].Status)
	assert.Equal(t, "Condutor", list.Data[0].ConductorName)
	assert.Equal(t, "condutor@example.com", list.Data[0].NotifyEmail)

	patchBody, _ := json.Marshal(models.RespondInvitationRequest{Status: models.InvitationResponseAccepted})
	patchReq := httptest.NewRequest(http.MethodPatch, "/citizen/"+handlerConductorCPF+"/vehicle-invitations/"+link.ID.Hex(), bytes.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchW := httptest.NewRecorder()
	router.ServeHTTP(patchW, patchReq)
	require.Equal(t, http.StatusOK, patchW.Code, "body: %s", patchW.Body.String())

	listReqAccepted := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerOwnerCPF+"/vehicles/"+created.ID.Hex()+"/conductors", nil)
	listWAccepted := httptest.NewRecorder()
	router.ServeHTTP(listWAccepted, listReqAccepted)
	require.Equal(t, http.StatusOK, listWAccepted.Code, "body: %s", listWAccepted.Body.String())
	var listAccepted models.ConductorsListResponse
	require.NoError(t, json.Unmarshal(listWAccepted.Body.Bytes(), &listAccepted))
	require.Len(t, listAccepted.Data, 1)
	assert.Equal(t, models.ConductorStatusAccepted, listAccepted.Data[0].Status)
	assert.Equal(t, "João Condutor", listAccepted.Data[0].ConductorName)
	assert.Equal(t, "joao@example.com", listAccepted.Data[0].NotifyEmail)
	assert.NotEqual(t, "Condutor", listAccepted.Data[0].ConductorName)
	assert.NotEqual(t, "condutor@example.com", listAccepted.Data[0].NotifyEmail)

	delReq := httptest.NewRequest(http.MethodDelete, "/citizen/"+handlerOwnerCPF+"/vehicles/"+created.ID.Hex()+"/conductors/"+link.ID.Hex(), nil)
	delW := httptest.NewRecorder()
	router.ServeHTTP(delW, delReq)
	require.Equal(t, http.StatusNoContent, delW.Code)

	listReq2 := httptest.NewRequest(http.MethodGet, "/citizen/"+handlerOwnerCPF+"/vehicles/"+created.ID.Hex()+"/conductors", nil)
	listW2 := httptest.NewRecorder()
	router.ServeHTTP(listW2, listReq2)
	require.Equal(t, http.StatusOK, listW2.Code)
	var list2 models.ConductorsListResponse
	require.NoError(t, json.Unmarshal(listW2.Body.Bytes(), &list2))
	assert.Empty(t, list2.Data)
}

func TestMapMobilidadeError_Branches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"not_found", services.ErrVehicleNotFound, http.StatusNotFound},
		{"conductor_not_found", services.ErrConductorNotFound, http.StatusNotFound},
		{"brand_not_found", services.ErrCatalogBrandNotFound, http.StatusNotFound},
		{"model_not_found", services.ErrCatalogModelNotFound, http.StatusNotFound},
		{"forbidden", services.ErrMobilidadeForbidden, http.StatusForbidden},
		{"conflict", services.ErrMobilidadeConflict, http.StatusConflict},
		{"invalid", services.ErrMobilidadeInvalidInput, http.StatusBadRequest},
		{"not_implemented", services.ErrMobilidadeNotImplemented, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
			mapMobilidadeError(c, tc.err)
			assert.Equal(t, tc.code, w.Code)
		})
	}
}

func TestGetVehicleModels_WithBrand(t *testing.T) {
	router, cleanup := setupMobilidadeHandlersTest(t)
	defer cleanup()
	seedHandlerCatalog(t)

	req := httptest.NewRequest(http.MethodGet, "/mobilidade/vehicle-models?brand_id=brand_caloi", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	var resp models.VehicleModelsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "E-Vibe", resp.Data[0].Name)
}
