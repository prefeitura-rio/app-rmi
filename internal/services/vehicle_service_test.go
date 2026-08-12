package services

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func boolPtr(v bool) *bool { return &v }

const (
	mobilidadeOwnerCPF     = "03561350712"
	mobilidadeConductorCPF = "45049725810"
	mobilidadeOtherCPF     = "11144477735"
)

func setupMobilidadeVehicleServiceTest(t *testing.T) (*VehicleService, *VehicleConductorService, *MobilidadeCatalogService, func()) {
	t.Helper()
	_ = logging.InitLogger()

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}
	config.AppConfig.MobilidadeVehicleCollection = "test_mobilidade_vehicles"
	config.AppConfig.MobilidadeConductorCollection = "test_mobilidade_vehicle_conductors"
	config.AppConfig.MobilidadeBrandCollection = "test_mobilidade_vehicle_brands"
	config.AppConfig.MobilidadeModelCollection = "test_mobilidade_vehicle_models"

	ctx := context.Background()
	db := config.MongoDB
	require.NotNil(t, db, "MongoDB must be initialized by TestMain")

	vehicleSvc := NewVehicleService(db, NewDataManager(config.Redis, db, logging.GetLogger()), logging.GetLogger())
	conductorSvc := NewVehicleConductorService(db, NewDataManager(config.Redis, db, logging.GetLogger()), logging.GetLogger())
	catalogSvc := NewMobilidadeCatalogService(db, logging.GetLogger())

	cleanup := func() {
		_ = db.Collection(config.AppConfig.MobilidadeVehicleCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeConductorCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeBrandCollection).Drop(ctx)
		_ = db.Collection(config.AppConfig.MobilidadeModelCollection).Drop(ctx)
		_ = db.Collection("mobilidade_registration_counters").Drop(ctx)
	}
	cleanup()

	// Seed citizens so GET enriches owner/conductor contact live from RMI.
	seedMobilidadeCitizen(t, mobilidadeOwnerCPF, "Ana Souza")
	seedMobilidadeCitizenEmail(t, mobilidadeOwnerCPF, "ana@example.com")
	seedMobilidadeCitizen(t, mobilidadeOtherCPF, "Outro Owner")
	seedMobilidadeCitizen(t, mobilidadeConductorCPF, "João Condutor")
	seedMobilidadeCitizenEmail(t, mobilidadeConductorCPF, "joao@example.com")
	// Clear leftover self-declared overlays for these CPFs so citizen fields win by default.
	_, _ = db.Collection(config.AppConfig.SelfDeclaredCollection).DeleteMany(ctx, bson.M{
		"cpf": bson.M{"$in": []string{mobilidadeOwnerCPF, mobilidadeOtherCPF, mobilidadeConductorCPF}},
	})
	invalidateMobilidadeContactCache(t, mobilidadeOwnerCPF)
	invalidateMobilidadeContactCache(t, mobilidadeOtherCPF)
	invalidateMobilidadeContactCache(t, mobilidadeConductorCPF)
	ensureMobilidadeConductorUniqueIndex(t)

	return vehicleSvc, conductorSvc, catalogSvc, cleanup
}

func ensureMobilidadeConductorUniqueIndex(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	coll := config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection)
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
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
	if err != nil {
		var cmdErr mongo.CommandError
		// 85 = IndexOptionsConflict, 86 = IndexKeySpecsConflict — identical/compatible index already present.
		if !errors.As(err, &cmdErr) || (cmdErr.Code != 85 && cmdErr.Code != 86) {
			require.NoError(t, err)
		}
	}

	cursor, err := coll.Indexes().List(ctx)
	require.NoError(t, err)
	defer cursor.Close(ctx)
	found := false
	for cursor.Next(ctx) {
		var idx bson.M
		require.NoError(t, cursor.Decode(&idx))
		if name, ok := idx["name"].(string); ok && name == "idx_mobilidade_conductors_vehicle_cpf_active" {
			found = true
			break
		}
	}
	require.True(t, found, "unique partial index idx_mobilidade_conductors_vehicle_cpf_active must exist")
}

func seedMobilidadeCitizen(t *testing.T, cpf, name string) {
	t.Helper()
	ctx := context.Background()
	invalidateMobilidadeContactCache(t, cpf)
	coll := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, _ = coll.DeleteMany(ctx, bson.M{"cpf": cpf})
	_, _ = coll.DeleteMany(ctx, bson.M{"_id": cpf})
	_, err := coll.InsertOne(ctx, bson.M{
		"_id":  cpf,
		"cpf":  cpf,
		"nome": name,
	})
	require.NoError(t, err)
}

func seedMobilidadeCitizenEmail(t *testing.T, cpf, email string) {
	t.Helper()
	ctx := context.Background()
	invalidateMobilidadeContactCache(t, cpf)
	coll := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := coll.UpdateOne(ctx, bson.M{"cpf": cpf}, bson.M{
		"$set": bson.M{
			"email": bson.M{
				"principal": bson.M{"valor": email},
			},
		},
	})
	require.NoError(t, err)
}

func seedMobilidadeSelfDeclaredContact(t *testing.T, cpf, displayName, email, ddi, ddd, phone string) {
	t.Helper()
	ctx := context.Background()
	invalidateMobilidadeContactCache(t, cpf)
	coll := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection)
	_, _ = coll.DeleteMany(ctx, bson.M{"cpf": cpf})
	_, err := coll.InsertOne(ctx, bson.M{
		"cpf":           cpf,
		"nome_exibicao": displayName,
		"email": bson.M{
			"principal": bson.M{"valor": email},
		},
		"telefone": bson.M{
			"principal": bson.M{
				"ddi":   ddi,
				"ddd":   ddd,
				"valor": phone,
			},
		},
		"updated_at": time.Now().UTC(),
	})
	require.NoError(t, err)
}

func invalidateMobilidadeContactCache(t *testing.T, cpf string) {
	t.Helper()
	if config.Redis == nil {
		return
	}
	ctx := context.Background()
	_ = config.Redis.Del(ctx,
		"citizen:cache:"+cpf,
		"citizen:write:"+cpf,
		"self_declared:cache:"+cpf,
		"self_declared:write:"+cpf,
	).Err()
}

func seedMobilidadeCatalog(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	brands := config.MongoDB.Collection(config.AppConfig.MobilidadeBrandCollection)
	modelsCol := config.MongoDB.Collection(config.AppConfig.MobilidadeModelCollection)

	_, err := brands.InsertMany(ctx, []interface{}{
		bson.M{"_id": "brand_caloi", "name": "Caloi", "is_other": false},
		bson.M{"_id": "brand_outro", "name": "Outro", "is_other": true},
	})
	require.NoError(t, err)

	_, err = modelsCol.InsertMany(ctx, []interface{}{
		bson.M{
			"_id": "model_e-vibe", "brand_id": "brand_caloi", "name": "E-Vibe",
			"vehicle_type": "bicicleta_eletrica", "is_other": false,
		},
		bson.M{
			"_id": "model_other_caloi", "brand_id": "brand_caloi", "name": "Outro",
			"vehicle_type": "bicicleta_eletrica", "is_other": true,
		},
		bson.M{
			"_id": "model_outro", "brand_id": "brand_outro", "name": "Outro",
			"vehicle_type": "autopropelido", "is_other": true,
		},
	})
	require.NoError(t, err)
}

func catalogCreateRequest() *models.VehicleCreateRequest {
	brandID := "brand_caloi"
	modelID := "model_e-vibe"
	invoiceURL := "https://storage.googleapis.com/bucket/nf.pdf"
	return &models.VehicleCreateRequest{
		DisplayName:          "Bike do trabalho",
		BrandID:              &brandID,
		ModelID:              &modelID,
		Color:                "Preto",
		SerialNumber:         "SN-ABC-123456",
		SerialNumberPhotoURL: "https://storage.googleapis.com/serial.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/vehicle.jpg",
		HasInvoice:           boolPtr(true),
		InvoicePhotoURL:      &invoiceURL,
		SelfDeclaration:      true,
	}
}

func otherCreateRequest() *models.VehicleCreateRequest {
	brandOther := "Xiaomi"
	modelOther := "Mi Electric Scooter 4"
	vt := models.VehicleTypeAutopropelido
	return &models.VehicleCreateRequest{
		DisplayName:          "Meu patinete",
		BrandOther:           &brandOther,
		ModelOther:           &modelOther,
		VehicleType:          &vt,
		Color:                "Branco",
		SerialNumber:         "XM-999888",
		SerialNumberPhotoURL: "https://storage.googleapis.com/serial.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/scooter.jpg",
		HasInvoice:           boolPtr(false),
		SelfDeclaration:      true,
	}
}

func TestMobilidadeCatalog_ListBrands(t *testing.T) {
	_, _, catalog, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	brands, err := catalog.ListBrands(context.Background())
	require.NoError(t, err)
	require.Len(t, brands, 2)
	assert.Equal(t, "brand_caloi", brands[0].ID)
	assert.Equal(t, "Caloi", brands[0].Name)
}

func TestMobilidadeCatalog_ListModelsByBrand(t *testing.T) {
	_, _, catalog, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	modelsList, err := catalog.ListModelsByBrand(context.Background(), "brand_caloi")
	require.NoError(t, err)
	require.Len(t, modelsList, 2)
	assert.Equal(t, models.VehicleTypeBicicletaEletrica, modelsList[0].VehicleType)
	assert.Equal(t, "brand_caloi", modelsList[0].BrandID)
}

func TestMobilidadeCatalog_ListModelsByBrand_RequiresBrandID(t *testing.T) {
	_, _, catalog, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()

	_, err := catalog.ListModelsByBrand(context.Background(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)
}

func TestMobilidadeCatalog_ListColors(t *testing.T) {
	_, _, catalog, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()

	colors, err := catalog.ListColors(context.Background())
	require.NoError(t, err)
	assert.Equal(t, models.VehicleColors(), colors)
	assert.Contains(t, colors, "Preto")
}

func TestVehicleService_CreateVehicle_CatalogFlow(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	require.NotNil(t, created)

	assert.Equal(t, mobilidadeOwnerCPF, created.OwnerCPF)
	assert.Equal(t, "Bike do trabalho", created.DisplayName)
	assert.Equal(t, models.VehicleTypeBicicletaEletrica, created.VehicleType)
	assert.Equal(t, models.VehicleRoleOwner, created.Role)
	assert.True(t, created.SelfDeclaration)
	assert.True(t, created.HasInvoice)
	require.NotNil(t, created.InvoicePhotoURL)
	assert.Equal(t, "https://storage.googleapis.com/bucket/nf.pdf", *created.InvoicePhotoURL)
	assert.False(t, created.ID.IsZero())
	assert.Regexp(t, `^RJ-E-\d{6}$`, created.RegistrationNumber)
	assert.Equal(t, "Ana Souza", created.OwnerName)
	assert.Equal(t, "ana@example.com", created.OwnerEmail)

	// registration_number and owner_* are not frozen on the document for UI — owner contact is bson:"-".
	var stored bson.M
	err = config.MongoDB.Collection(config.AppConfig.MobilidadeVehicleCollection).FindOne(
		context.Background(), bson.M{"_id": created.ID},
	).Decode(&stored)
	require.NoError(t, err)
	assert.Equal(t, created.RegistrationNumber, stored["registration_number"])
	_, hasOwnerName := stored["owner_name"]
	assert.False(t, hasOwnerName)
}

func TestVehicleService_CreateVehicle_OtherFlow(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, otherCreateRequest())
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, models.VehicleTypeAutopropelido, created.VehicleType)
	assert.Equal(t, "Xiaomi", *created.BrandOther)
	assert.Nil(t, created.BrandID)
	assert.False(t, created.HasInvoice)
	assert.Nil(t, created.InvoicePhotoURL)
}

func TestVehicleService_UpdateVehicle_InvoiceFields(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	falseVal := false
	updated, err := vehicleSvc.UpdateVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.VehicleUpdateRequest{
		HasInvoice: &falseVal,
	})
	require.NoError(t, err)
	assert.False(t, updated.HasInvoice)
	assert.Nil(t, updated.InvoicePhotoURL)

	trueVal := true
	newURL := "https://storage.googleapis.com/bucket/new-nf.pdf"
	fileName := "new-nf.pdf"
	fileSize := int64(2048)
	updated, err = vehicleSvc.UpdateVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.VehicleUpdateRequest{
		HasInvoice:           &trueVal,
		InvoicePhotoURL:      &newURL,
		InvoicePhotoFileName: &fileName,
		InvoicePhotoFileSize: &fileSize,
	})
	require.NoError(t, err)
	assert.True(t, updated.HasInvoice)
	require.NotNil(t, updated.InvoicePhotoURL)
	assert.Equal(t, newURL, *updated.InvoicePhotoURL)
	require.NotNil(t, updated.InvoicePhotoFileName)
	assert.Equal(t, fileName, *updated.InvoicePhotoFileName)
	require.NotNil(t, updated.InvoicePhotoFileSize)
	assert.Equal(t, fileSize, *updated.InvoicePhotoFileSize)
}

func TestVehicleService_UpdateVehicle_ReResolvesTypeFromCatalog(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	assert.Equal(t, models.VehicleTypeBicicletaEletrica, created.VehicleType)

	// Seed a ciclomotor model under Caloi for this test.
	ctx := context.Background()
	_, err = config.MongoDB.Collection(config.AppConfig.MobilidadeModelCollection).InsertOne(ctx, bson.M{
		"_id": "model_ciclomotor", "brand_id": "brand_caloi", "name": "Ciclomotor X",
		"vehicle_type": "ciclomotor", "is_other": false,
	})
	require.NoError(t, err)

	modelID := "model_ciclomotor"
	updated, err := vehicleSvc.UpdateVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.VehicleUpdateRequest{
		ModelID: &modelID,
	})
	require.NoError(t, err)
	assert.Equal(t, models.VehicleTypeCiclomotor, updated.VehicleType)
	require.NotNil(t, updated.ModelID)
	assert.Equal(t, modelID, *updated.ModelID)
}

func TestVehicleService_UpdateVehicle_RejectsIncompleteCatalogPatch(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	empty := ""
	_, err = vehicleSvc.UpdateVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.VehicleUpdateRequest{
		BrandID: &empty,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)
	assert.Contains(t, err.Error(), "incomplete")
}

func TestVehicleService_UpdateVehicle_CatalogToOtherViaJSONNull(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	require.NotNil(t, created.BrandID)
	assert.Equal(t, models.VehicleTypeBicicletaEletrica, created.VehicleType)

	// Client sends brand_id/model_id as JSON null when switching to free-text Outro.
	var req models.VehicleUpdateRequest
	err = json.Unmarshal([]byte(`{
		"display_name": "Coelho",
		"brand_id": null,
		"brand_other": "teste",
		"model_id": null,
		"model_other": "teste",
		"vehicle_type": "ciclomotor",
		"color": "Amarelo",
		"serial_number": "TESTE"
	}`), &req)
	require.NoError(t, err)

	updated, err := vehicleSvc.UpdateVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &req)
	require.NoError(t, err)
	assert.Equal(t, "Coelho", updated.DisplayName)
	assert.Equal(t, "Amarelo", updated.Color)
	assert.Equal(t, "TESTE", updated.SerialNumber)
	assert.Equal(t, models.VehicleTypeCiclomotor, updated.VehicleType)
	assert.Nil(t, updated.BrandID)
	assert.Nil(t, updated.ModelID)
	require.NotNil(t, updated.BrandOther)
	assert.Equal(t, "teste", *updated.BrandOther)
	require.NotNil(t, updated.ModelOther)
	assert.Equal(t, "teste", *updated.ModelOther)
}

func TestVehicleService_CreateVehicle_OtherCatalogRequiresFreeText(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	brandID := "brand_outro"
	modelID := "model_outro"
	req := &models.VehicleCreateRequest{
		DisplayName:          "Custom",
		BrandID:              &brandID,
		ModelID:              &modelID,
		Color:                "Preto",
		SerialNumber:         "SN-OTHER",
		SerialNumberPhotoURL: "https://storage.googleapis.com/serial.jpg",
		VehiclePhotoURL:      "https://storage.googleapis.com/vehicle.jpg",
		HasInvoice:           boolPtr(false),
		SelfDeclaration:      true,
	}
	_, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	brandOther := "Minha Marca"
	modelOther := "Meu Modelo"
	vt := models.VehicleTypeCiclomotor
	req.BrandOther = &brandOther
	req.ModelOther = &modelOther
	req.VehicleType = &vt
	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, req)
	require.NoError(t, err)
	assert.Equal(t, models.VehicleTypeCiclomotor, created.VehicleType)
	require.NotNil(t, created.BrandOther)
	assert.Equal(t, brandOther, *created.BrandOther)
}

func TestVehicleConductorService_RespondInvitation_RejectsAfterRevoke(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)

	require.NoError(t, conductorSvc.RemoveConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), link.ID.Hex()))

	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)
}

func TestVehicleConductorService_RespondInvitation_RejectsDeletedVehicle(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)

	require.NoError(t, vehicleSvc.DeleteVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex()))

	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.Error(t, err)
}

func TestVehicleService_ListVehicles_StableSort(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	first, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	second, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, otherCreateRequest())
	require.NoError(t, err)

	list, err := vehicleSvc.ListVehicles(context.Background(), mobilidadeOwnerCPF, 1, 10)
	require.NoError(t, err)
	require.Len(t, list.Data, 2)
	assert.Equal(t, second.ID.Hex(), list.Data[0].ID)
	assert.Equal(t, first.ID.Hex(), list.Data[1].ID)
}

func TestVehicleService_CreateVehicle_RejectsFalseSelfDeclaration(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	req := catalogCreateRequest()
	req.SelfDeclaration = false
	_, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, req)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)
}

func TestVehicleService_ListVehicles_OwnerAndAcceptedConductor(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	owned, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	shared, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOtherCPF, otherCreateRequest())
	require.NoError(t, err)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOtherCPF, shared.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeOwnerCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)

	invites, err := conductorSvc.ListInvitations(context.Background(), mobilidadeOwnerCPF)
	require.NoError(t, err)
	require.Len(t, invites.Data, 1)

	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeOwnerCPF, invites.Data[0].ID, &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	list, err := vehicleSvc.ListVehicles(context.Background(), mobilidadeOwnerCPF, 1, 10)
	require.NoError(t, err)
	require.Equal(t, 2, list.Pagination.Total)

	roles := map[string]models.VehicleRole{}
	conductorIDs := map[string]string{}
	for _, item := range list.Data {
		roles[item.ID] = item.Role
		conductorIDs[item.ID] = item.ConductorID
	}
	assert.Equal(t, models.VehicleRoleOwner, roles[owned.ID.Hex()])
	assert.Empty(t, conductorIDs[owned.ID.Hex()])
	assert.Equal(t, models.VehicleRoleConductor, roles[shared.ID.Hex()])
	assert.Equal(t, invites.Data[0].ID, conductorIDs[shared.ID.Hex()])
}

func TestVehicleService_ListVehicles_ExcludesPendingOnly(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	shared, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOtherCPF, catalogCreateRequest())
	require.NoError(t, err)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOtherCPF, shared.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeOwnerCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)

	list, err := vehicleSvc.ListVehicles(context.Background(), mobilidadeOwnerCPF, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, list.Pagination.Total)
}

func TestVehicleService_GetVehicle_OwnerAndConductor(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	detail, err := vehicleSvc.GetVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)
	assert.Equal(t, models.VehicleRoleOwner, detail.Role)
	assert.Empty(t, detail.ConductorID)
	assert.Equal(t, "SN-ABC-123456", detail.SerialNumber)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)
	invites, err := conductorSvc.ListInvitations(context.Background(), mobilidadeConductorCPF)
	require.NoError(t, err)
	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, invites.Data[0].ID, &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	asConductor, err := vehicleSvc.GetVehicle(context.Background(), mobilidadeConductorCPF, created.ID.Hex())
	require.NoError(t, err)
	assert.Equal(t, models.VehicleRoleConductor, asConductor.Role)
	assert.Equal(t, invites.Data[0].ID, asConductor.ConductorID)
}

func TestVehicleService_GetVehicle_ForbiddenForStranger(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	_, err = vehicleSvc.GetVehicle(context.Background(), mobilidadeOtherCPF, created.ID.Hex())
	require.Error(t, err)
	// Privacy: 404 preferred; 403 also acceptable.
	assert.True(t,
		errors.Is(err, ErrVehicleNotFound) || errors.Is(err, ErrMobilidadeForbidden),
		"want not found or forbidden, got %v", err,
	)
}

func TestVehicleService_UpdateVehicle_OwnerOnly(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	newName := "Bike nova"
	updated, err := vehicleSvc.UpdateVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.VehicleUpdateRequest{
		DisplayName: &newName,
	})
	require.NoError(t, err)
	assert.Equal(t, "Bike nova", updated.DisplayName)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)
	invites, _ := conductorSvc.ListInvitations(context.Background(), mobilidadeConductorCPF)
	_, _ = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, invites.Data[0].ID, &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})

	_, err = vehicleSvc.UpdateVehicle(context.Background(), mobilidadeConductorCPF, created.ID.Hex(), &models.VehicleUpdateRequest{
		DisplayName: &newName,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeForbidden)
}

func TestVehicleService_DeleteVehicle_CascadesConductors(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)
	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	err = vehicleSvc.DeleteVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)

	_, err = vehicleSvc.GetVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.Error(t, err)

	list, err := vehicleSvc.ListVehicles(context.Background(), mobilidadeConductorCPF, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, list.Pagination.Total)

	// Conductor link should be revoked in storage
	var stored models.VehicleConductor
	err = config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection).FindOne(
		context.Background(), bson.M{"_id": link.ID},
	).Decode(&stored)
	require.NoError(t, err)
	assert.Equal(t, models.ConductorStatusRevoked, stored.Status)
}

func TestVehicleService_DeleteVehicle_IdempotentRevokesConductors(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor",
	})
	require.NoError(t, err)

	// Simulate partial failure: vehicle already soft-deleted, conductor still pending.
	now := time.Now().UTC()
	_, err = config.MongoDB.Collection(config.AppConfig.MobilidadeVehicleCollection).UpdateOne(
		context.Background(),
		bson.M{"_id": created.ID},
		bson.M{"$set": bson.M{"deleted_at": now, "updated_at": now}},
	)
	require.NoError(t, err)

	err = vehicleSvc.DeleteVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)

	var stored models.VehicleConductor
	err = config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection).FindOne(
		context.Background(), bson.M{"_id": link.ID},
	).Decode(&stored)
	require.NoError(t, err)
	assert.Equal(t, models.ConductorStatusRevoked, stored.Status)
}

func TestVehicleService_DeleteVehicle_SoftDeletedNonOwnerGetsNotFound(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	require.NoError(t, vehicleSvc.DeleteVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex()))

	err = vehicleSvc.DeleteVehicle(context.Background(), mobilidadeOtherCPF, created.ID.Hex())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVehicleNotFound)
	assert.NotErrorIs(t, err, ErrMobilidadeForbidden)
}

func TestVehicleConductorService_InviteRejectsSelfAndDuplicate(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeOwnerCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeConflict)
}

func TestVehicleConductorService_InviteRejectsFormattedCPF(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF:   "111.444.777-35",
		Email: "joao@example.com",
		Name:  "João Condutor",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeInvalidInput)
	assert.Contains(t, err.Error(), "11 digits")
}

func TestVehicleService_UpdateVehicle_ClearsStaleFileMetadataOnURLChange(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	req := catalogCreateRequest()
	name := "serial-old.jpg"
	size := int64(1234)
	req.SerialNumberPhotoFileName = &name
	req.SerialNumberPhotoFileSize = &size
	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, req)
	require.NoError(t, err)
	require.NotNil(t, created.SerialNumberPhotoFileName)

	newURL := "https://storage.googleapis.com/bucket/serial-new.jpg"
	updated, err := vehicleSvc.UpdateVehicle(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.VehicleUpdateRequest{
		SerialNumberPhotoURL: &newURL,
	})
	require.NoError(t, err)
	assert.Equal(t, newURL, updated.SerialNumberPhotoURL)
	assert.Nil(t, updated.SerialNumberPhotoFileName)
	assert.Nil(t, updated.SerialNumberPhotoFileSize)
}

func TestMobilidadeConductorUniqueIndex_RejectsDuplicateActiveLink(t *testing.T) {
	vehicleSvc, _, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	ctx := context.Background()
	coll := config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection)
	now := time.Now().UTC()
	first := models.VehicleConductor{
		ID: primitive.NewObjectID(), VehicleID: created.ID, ConductorCPF: mobilidadeConductorCPF,
		NotifyEmail: "a@example.com", Status: models.ConductorStatusPending,
		InvitedByCPF: mobilidadeOwnerCPF, CreatedAt: now, UpdatedAt: now,
	}
	_, err = coll.InsertOne(ctx, first)
	require.NoError(t, err)

	// Same vehicle + CPF while pending must violate unique partial index.
	second := first
	second.ID = primitive.NewObjectID()
	second.NotifyEmail = "b@example.com"
	_, err = coll.InsertOne(ctx, second)
	require.Error(t, err)
	assert.True(t, mongo.IsDuplicateKeyError(err), "want duplicate key from unique index, got %v", err)
	assert.ErrorIs(t, mapConductorInsertError(err), ErrMobilidadeConflict)

	// Revoked links are outside the partial filter — a new pending invite is allowed.
	_, err = coll.UpdateOne(ctx, bson.M{"_id": first.ID}, bson.M{"$set": bson.M{"status": models.ConductorStatusRevoked}})
	require.NoError(t, err)
	_, err = coll.InsertOne(ctx, second)
	require.NoError(t, err)
}

func TestInviteConductor_ConcurrentDuplicateHitsUniqueIndex(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	// Barrier so both goroutines pass CountDocuments before either InsertOne commits.
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, inviteErr := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
			errs <- inviteErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrMobilidadeConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	assert.Equal(t, 1, successes, "exactly one invite should succeed")
	assert.Equal(t, 1, conflicts, "the other invite should hit unique index → conflict")

	count, err := config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection).CountDocuments(
		context.Background(),
		bson.M{
			"vehicle_id":    created.ID,
			"conductor_cpf": mobilidadeConductorCPF,
			"status":        models.ConductorStatusPending,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestVehicleConductorService_ListInvitations_IncludesVehicleSummary(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)

	invites, err := conductorSvc.ListInvitations(context.Background(), mobilidadeConductorCPF)
	require.NoError(t, err)
	require.Len(t, invites.Data, 1)
	assert.Equal(t, models.ConductorStatusPending, invites.Data[0].Status)
	assert.Equal(t, "Bike do trabalho", invites.Data[0].Vehicle.DisplayName)
	assert.Equal(t, "Caloi", invites.Data[0].Vehicle.BrandLabel)
	assert.NotEmpty(t, invites.Data[0].OwnerName)
}

func TestVehicleConductorService_RespondInvitation_AcceptAndReject(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	v1, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	v2, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, otherCreateRequest())
	require.NoError(t, err)

	link1, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, v1.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)
	link2, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, v2.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)

	accepted, err := conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link1.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)
	assert.Equal(t, models.ConductorStatusAccepted, accepted.Status)
	require.NotNil(t, accepted.RespondedAt)

	rejected, err := conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link2.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseRejected,
	})
	require.NoError(t, err)
	assert.Equal(t, models.ConductorStatusRejected, rejected.Status)

	list, err := vehicleSvc.ListVehicles(context.Background(), mobilidadeConductorCPF, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, list.Pagination.Total)
}

func TestVehicleConductorService_ListConductors_OwnerOnly(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)
	_, err = conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)

	list, err := conductorSvc.ListConductors(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)
	require.Len(t, list.Data, 1)

	_, err = conductorSvc.ListConductors(context.Background(), mobilidadeConductorCPF, created.ID.Hex())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMobilidadeForbidden)
}

func TestVehicleConductorService_RemoveConductor_OwnerAndSelfLeave(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)
	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	err = conductorSvc.RemoveConductor(context.Background(), mobilidadeConductorCPF, created.ID.Hex(), link.ID.Hex())
	require.NoError(t, err)

	list, err := vehicleSvc.ListVehicles(context.Background(), mobilidadeConductorCPF, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, list.Pagination.Total)

	// Re-invite and owner revokes
	link2, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor"})
	require.NoError(t, err)
	err = conductorSvc.RemoveConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), link2.ID.Hex())
	require.NoError(t, err)
}

func TestVehicleConductorService_InvitePersistsFormSnapshot(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "convite@example.com", Name: "Nome Digitado", Phone: "21999990000",
	})
	require.NoError(t, err)
	assert.Equal(t, "Nome Digitado", link.ConductorName)
	assert.Equal(t, "convite@example.com", link.NotifyEmail)
	assert.Equal(t, "21999990000", link.Phone)
	assert.Equal(t, models.ConductorStatusPending, link.Status)
	assert.Equal(t, mobilidadeOwnerCPF, link.InvitedByCPF)
	assert.False(t, link.ID.IsZero())

	var stored bson.M
	err = config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection).FindOne(
		context.Background(), bson.M{"_id": link.ID},
	).Decode(&stored)
	require.NoError(t, err)
	assert.Equal(t, mobilidadeConductorCPF, stored["conductor_cpf"])
	assert.Equal(t, "Nome Digitado", stored["conductor_name"])
	assert.Equal(t, "convite@example.com", stored["notify_email"])
	assert.Equal(t, "21999990000", stored["phone"])

	// Pending list uses invite snapshot, not RMI profile.
	list, err := conductorSvc.ListConductors(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.Equal(t, "Nome Digitado", list.Data[0].ConductorName)
	assert.Equal(t, "convite@example.com", list.Data[0].NotifyEmail)
	assert.Equal(t, "21999990000", list.Data[0].Phone)
}

func TestListConductors_AcceptedReplacesInviteSnapshotWithCitizenProfile(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "convite@example.com", Name: "Nome Digitado", Phone: "21999990000",
	})
	require.NoError(t, err)
	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	list, err := conductorSvc.ListConductors(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.Equal(t, models.ConductorStatusAccepted, list.Data[0].Status)
	assert.Equal(t, "João Condutor", list.Data[0].ConductorName)
	assert.Equal(t, "joao@example.com", list.Data[0].NotifyEmail)
	assert.Equal(t, "21999990000", list.Data[0].Phone) // RMI sem telefone → mantém snapshot
	assert.NotEqual(t, "Nome Digitado", list.Data[0].ConductorName)
	assert.NotEqual(t, "convite@example.com", list.Data[0].NotifyEmail)
}

func TestListConductors_AcceptedEnrichesLiveRMI(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "convite@example.com", Name: "Nome Digitado",
	})
	require.NoError(t, err)
	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	seedMobilidadeSelfDeclaredContact(t, mobilidadeConductorCPF, "João Atualizado", "joao.novo@example.com", "+55", "21", "988887777")

	list, err := conductorSvc.ListConductors(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.Equal(t, "João Atualizado", list.Data[0].ConductorName)
	assert.Equal(t, "joao.novo@example.com", list.Data[0].NotifyEmail)
	assert.NotEmpty(t, list.Data[0].Phone)
	assert.Contains(t, list.Data[0].Phone, "988887777")
}

func TestListConductors_AcceptedKeepsSnapshotWhenRMIEmpty(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "convite@example.com", Name: "Nome Digitado", Phone: "21999990000",
	})
	require.NoError(t, err)
	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	// Remove citizen so live profile is empty — overlay must keep invite snapshot fields.
	_, err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).DeleteMany(context.Background(), bson.M{"cpf": mobilidadeConductorCPF})
	require.NoError(t, err)
	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).DeleteMany(context.Background(), bson.M{"cpf": mobilidadeConductorCPF})
	require.NoError(t, err)
	invalidateMobilidadeContactCache(t, mobilidadeConductorCPF)

	list, err := conductorSvc.ListConductors(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.Equal(t, models.ConductorStatusAccepted, list.Data[0].Status)
	assert.Equal(t, "Nome Digitado", list.Data[0].ConductorName)
	assert.Equal(t, "convite@example.com", list.Data[0].NotifyEmail)
	assert.Equal(t, "21999990000", list.Data[0].Phone)
}

func TestListConductors_AcceptedOverlaysPartialRMI(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "convite@example.com", Name: "Nome Digitado", Phone: "21999990000",
	})
	require.NoError(t, err)
	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	// Citizen has name but no email/phone — RMI name overlays; invite email/phone remain.
	invalidateMobilidadeContactCache(t, mobilidadeConductorCPF)
	_, err = config.MongoDB.Collection(config.AppConfig.CitizenCollection).UpdateOne(context.Background(), bson.M{"cpf": mobilidadeConductorCPF}, bson.M{
		"$unset": bson.M{"email": "", "telefone": ""},
		"$set":   bson.M{"nome": "João Condutor"},
	})
	require.NoError(t, err)
	_, err = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).DeleteMany(context.Background(), bson.M{"cpf": mobilidadeConductorCPF})
	require.NoError(t, err)
	invalidateMobilidadeContactCache(t, mobilidadeConductorCPF)

	list, err := conductorSvc.ListConductors(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.Equal(t, "João Condutor", list.Data[0].ConductorName)
	assert.Equal(t, "convite@example.com", list.Data[0].NotifyEmail)
	assert.Equal(t, "21999990000", list.Data[0].Phone)
}

func TestListConductors_AcceptedReadsCitizenByIDFallback(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	created, err := vehicleSvc.CreateVehicle(context.Background(), mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(context.Background(), mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "convite@example.com", Name: "Nome Digitado",
	})
	require.NoError(t, err)
	_, err = conductorSvc.RespondInvitation(context.Background(), mobilidadeConductorCPF, link.ID.Hex(), &models.RespondInvitationRequest{
		Status: models.InvitationResponseAccepted,
	})
	require.NoError(t, err)

	invalidateMobilidadeContactCache(t, mobilidadeConductorCPF)
	coll := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err = coll.DeleteMany(context.Background(), bson.M{"cpf": mobilidadeConductorCPF})
	require.NoError(t, err)
	_, err = coll.DeleteMany(context.Background(), bson.M{"_id": mobilidadeConductorCPF})
	require.NoError(t, err)
	_, err = coll.InsertOne(context.Background(), bson.M{
		"_id":  mobilidadeConductorCPF,
		"nome": "Só Pelo ID",
		"email": bson.M{
			"principal": bson.M{"valor": "id@example.com"},
		},
	})
	require.NoError(t, err)
	invalidateMobilidadeContactCache(t, mobilidadeConductorCPF)

	list, err := conductorSvc.ListConductors(context.Background(), mobilidadeOwnerCPF, created.ID.Hex())
	require.NoError(t, err)
	require.Len(t, list.Data, 1)
	assert.Equal(t, "Só Pelo ID", list.Data[0].ConductorName)
	assert.Equal(t, "id@example.com", list.Data[0].NotifyEmail)

	// Restore a normal citizen doc so later tests in the same process are not polluted.
	_, _ = coll.DeleteMany(context.Background(), bson.M{"_id": mobilidadeConductorCPF})
	seedMobilidadeCitizen(t, mobilidadeConductorCPF, "João Condutor")
	seedMobilidadeCitizenEmail(t, mobilidadeConductorCPF, "joao@example.com")
}
