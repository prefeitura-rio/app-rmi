package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

var (
	// ErrMobilidadeNotImplemented is returned while Mobilidade services are under TDD (red phase).
	ErrMobilidadeNotImplemented = errors.New("mobilidade: not implemented")
	// ErrVehicleNotFound is returned when a vehicle does not exist or is soft-deleted.
	ErrVehicleNotFound = errors.New("vehicle not found")
	// ErrConductorNotFound is returned when a conductor link does not exist.
	ErrConductorNotFound = errors.New("conductor not found")
	// ErrMobilidadeForbidden is returned when the caller lacks the required role.
	ErrMobilidadeForbidden = errors.New("forbidden")
	// ErrMobilidadeConflict is returned on duplicate pending/accepted conductor links.
	ErrMobilidadeConflict = errors.New("conflict")
	// ErrMobilidadeInvalidInput is returned for business-rule validation failures.
	ErrMobilidadeInvalidInput = errors.New("invalid input")
	// ErrCatalogBrandNotFound is returned when a catalog brand does not exist or is soft-deleted.
	ErrCatalogBrandNotFound = errors.New("vehicle brand not found")
	// ErrCatalogModelNotFound is returned when a catalog model does not exist or is soft-deleted.
	ErrCatalogModelNotFound = errors.New("vehicle model not found")
)

// VehicleServiceInstance is the process-wide vehicle service used by handlers.
var VehicleServiceInstance *VehicleService

// VehicleService handles Mobilidade vehicle CRUD and wallet listing.
type VehicleService struct {
	database    *mongo.Database
	dataManager *DataManager
	logger      *logging.SafeLogger
}

// NewVehicleService creates a VehicleService.
func NewVehicleService(database *mongo.Database, dataManager *DataManager, logger *logging.SafeLogger) *VehicleService {
	return &VehicleService{database: database, dataManager: dataManager, logger: logger}
}

// InitVehicleService initializes VehicleServiceInstance from global MongoDB config.
func InitVehicleService() {
	logger := logging.GetLogger()
	dm := NewDataManager(config.Redis, config.MongoDB, logger)
	VehicleServiceInstance = NewVehicleService(config.MongoDB, dm, logger)
}

func (s *VehicleService) vehicles() *mongo.Collection {
	return s.database.Collection(config.AppConfig.MobilidadeVehicleCollection)
}

func (s *VehicleService) conductors() *mongo.Collection {
	return s.database.Collection(config.AppConfig.MobilidadeConductorCollection)
}

func (s *VehicleService) models() *mongo.Collection {
	return s.database.Collection(config.AppConfig.MobilidadeModelCollection)
}

func (s *VehicleService) brands() *mongo.Collection {
	return s.database.Collection(config.AppConfig.MobilidadeBrandCollection)
}

// ListVehicles returns vehicles where cpf is owner or accepted conductor.
func (s *VehicleService) ListVehicles(ctx context.Context, cpf string, page, perPage int) (*models.PaginatedVehicles, error) {
	ownedFilter := bson.M{"owner_cpf": cpf, "deleted_at": nil}
	ownedCursor, err := s.vehicles().Find(ctx, ownedFilter)
	if err != nil {
		return nil, fmt.Errorf("list owned vehicles: %w", err)
	}
	defer func() { _ = ownedCursor.Close(ctx) }()

	var owned []models.Vehicle
	if err := ownedCursor.All(ctx, &owned); err != nil {
		return nil, fmt.Errorf("decode owned vehicles: %w", err)
	}

	condCursor, err := s.conductors().Find(ctx, bson.M{
		"conductor_cpf": cpf,
		"status":        models.ConductorStatusAccepted,
	})
	if err != nil {
		return nil, fmt.Errorf("list conductor links: %w", err)
	}
	defer func() { _ = condCursor.Close(ctx) }()

	var links []models.VehicleConductor
	if err := condCursor.All(ctx, &links); err != nil {
		return nil, fmt.Errorf("decode conductor links: %w", err)
	}

	itemsByID := map[string]walletEntry{}
	for _, v := range owned {
		itemsByID[v.ID.Hex()] = walletEntry{
			item:      toListItem(v, models.VehicleRoleOwner, ""),
			createdAt: v.CreatedAt,
			id:        v.ID.Hex(),
		}
	}

	for _, link := range links {
		if _, exists := itemsByID[link.VehicleID.Hex()]; exists {
			continue
		}
		var v models.Vehicle
		err := s.vehicles().FindOne(ctx, bson.M{"_id": link.VehicleID, "deleted_at": nil}).Decode(&v)
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue
			}
			return nil, fmt.Errorf("load shared vehicle: %w", err)
		}
		itemsByID[v.ID.Hex()] = walletEntry{
			item:      toListItem(v, models.VehicleRoleConductor, link.ID.Hex()),
			createdAt: v.CreatedAt,
			id:        v.ID.Hex(),
		}
	}

	all := make([]walletEntry, 0, len(itemsByID))
	for _, entry := range itemsByID {
		all = append(all, entry)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].createdAt.Equal(all[j].createdAt) {
			return all[i].createdAt.After(all[j].createdAt)
		}
		return all[i].id > all[j].id
	})

	total := len(all)
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	pageItems := make([]models.VehicleListItem, 0, end-start)
	for _, entry := range all[start:end] {
		pageItems = append(pageItems, entry.item)
	}
	s.enrichListCatalogNames(ctx, pageItems)

	resp := &models.PaginatedVehicles{Data: pageItems}
	resp.Pagination.Page = page
	resp.Pagination.PerPage = perPage
	resp.Pagination.Total = total
	resp.Pagination.TotalPages = totalPages
	return resp, nil
}

type walletEntry struct {
	item      models.VehicleListItem
	createdAt time.Time
	id        string
}

// GetVehicle returns vehicle detail if cpf is owner or accepted conductor.
func (s *VehicleService) GetVehicle(ctx context.Context, cpf, vehicleID string) (*models.VehicleDetail, error) {
	v, err := s.findActiveVehicle(ctx, vehicleID)
	if err != nil {
		return nil, err
	}

	role, conductorID, err := s.resolveRole(ctx, cpf, v)
	if err != nil {
		return nil, err
	}

	s.enrichOwnerFields(ctx, v)
	s.enrichVehicleCatalogNames(ctx, v)
	return &models.VehicleDetail{Vehicle: *v, Role: role, ConductorID: conductorID}, nil
}

// CreateVehicle registers a new vehicle owned by cpf.
// Owner contact is not persisted; GET/201 responses enrich name/phone/email live from RMI.
func (s *VehicleService) CreateVehicle(ctx context.Context, cpf string, req *models.VehicleCreateRequest) (*models.VehicleDetail, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMobilidadeInvalidInput, err.Error())
	}
	if !utils.ValidateCPF(cpf) {
		return nil, fmt.Errorf("%w: invalid cpf", ErrMobilidadeInvalidInput)
	}

	vehicleType, brandID, brandOther, modelID, modelOther, err := s.resolveCreateCatalogFields(ctx, req)
	if err != nil {
		return nil, err
	}

	regNumber, err := s.nextRegistrationNumber(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	vehicle := models.Vehicle{
		ID:                        primitive.NewObjectID(),
		OwnerCPF:                  cpf,
		DisplayName:               req.DisplayName,
		BrandID:                   brandID,
		BrandOther:                brandOther,
		ModelID:                   modelID,
		ModelOther:                modelOther,
		VehicleType:               vehicleType,
		Color:                     req.Color,
		SerialNumber:              req.SerialNumber,
		RegistrationNumber:        regNumber,
		SerialNumberPhotoURL:      req.SerialNumberPhotoURL,
		VehiclePhotoURL:           req.VehiclePhotoURL,
		InvoicePhotoURL:           req.InvoicePhotoURL,
		HasInvoice:                req.HasInvoiceValue(),
		SelfDeclaration:           true,
		SerialNumberPhotoFileName: req.SerialNumberPhotoFileName,
		SerialNumberPhotoFileSize: req.SerialNumberPhotoFileSize,
		VehiclePhotoFileName:      req.VehiclePhotoFileName,
		VehiclePhotoFileSize:      req.VehiclePhotoFileSize,
		InvoicePhotoFileName:      req.InvoicePhotoFileName,
		InvoicePhotoFileSize:      req.InvoicePhotoFileSize,
		CreatedAt:                 now,
		UpdatedAt:                 now,
	}

	const maxInsertAttempts = 3
	for attempt := 1; attempt <= maxInsertAttempts; attempt++ {
		if _, err = s.vehicles().InsertOne(ctx, vehicle); err == nil {
			break
		}
		if !mongo.IsDuplicateKeyError(err) || attempt == maxInsertAttempts {
			return nil, fmt.Errorf("insert vehicle: %w", err)
		}
		regNumber, allocErr := s.nextRegistrationNumber(ctx)
		if allocErr != nil {
			return nil, allocErr
		}
		vehicle.RegistrationNumber = regNumber
		vehicle.ID = primitive.NewObjectID()
	}
	s.enrichOwnerFields(ctx, &vehicle)
	s.enrichVehicleCatalogNames(ctx, &vehicle)
	return &models.VehicleDetail{Vehicle: vehicle, Role: models.VehicleRoleOwner}, nil
}

// UpdateVehicle updates a vehicle; only the owner may call this.
func (s *VehicleService) UpdateVehicle(ctx context.Context, cpf, vehicleID string, req *models.VehicleUpdateRequest) (*models.VehicleDetail, error) {
	v, err := s.findActiveVehicle(ctx, vehicleID)
	if err != nil {
		return nil, err
	}
	if v.OwnerCPF != cpf {
		return nil, ErrMobilidadeForbidden
	}
	if err := req.Validate(v); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMobilidadeInvalidInput, err.Error())
	}

	update := bson.M{"updated_at": time.Now().UTC()}
	if req.DisplayName != nil {
		update["display_name"] = *req.DisplayName
	}
	if req.Color != nil {
		update["color"] = *req.Color
	}
	if req.SerialNumber != nil {
		update["serial_number"] = *req.SerialNumber
	}
	if req.SerialNumberPhotoURL != nil {
		update["serial_number_photo_url"] = *req.SerialNumberPhotoURL
		// URL change without metadata would leave stale labels — clear unless replaced below.
		if req.SerialNumberPhotoFileName == nil {
			update["serial_number_photo_file_name"] = nil
		}
		if req.SerialNumberPhotoFileSize == nil {
			update["serial_number_photo_file_size"] = nil
		}
	}
	if req.VehiclePhotoURL != nil {
		update["vehicle_photo_url"] = *req.VehiclePhotoURL
		if req.VehiclePhotoFileName == nil {
			update["vehicle_photo_file_name"] = nil
		}
		if req.VehiclePhotoFileSize == nil {
			update["vehicle_photo_file_size"] = nil
		}
	}
	if req.SerialNumberPhotoFileName != nil {
		if strings.TrimSpace(*req.SerialNumberPhotoFileName) == "" {
			update["serial_number_photo_file_name"] = nil
		} else {
			update["serial_number_photo_file_name"] = *req.SerialNumberPhotoFileName
		}
	}
	if req.SerialNumberPhotoFileSize != nil {
		if *req.SerialNumberPhotoFileSize <= 0 {
			update["serial_number_photo_file_size"] = nil
		} else {
			update["serial_number_photo_file_size"] = *req.SerialNumberPhotoFileSize
		}
	}
	if req.VehiclePhotoFileName != nil {
		if strings.TrimSpace(*req.VehiclePhotoFileName) == "" {
			update["vehicle_photo_file_name"] = nil
		} else {
			update["vehicle_photo_file_name"] = *req.VehiclePhotoFileName
		}
	}
	if req.VehiclePhotoFileSize != nil {
		if *req.VehiclePhotoFileSize <= 0 {
			update["vehicle_photo_file_size"] = nil
		} else {
			update["vehicle_photo_file_size"] = *req.VehiclePhotoFileSize
		}
	}

	hasInvoice := v.HasInvoice
	if req.HasInvoice != nil {
		hasInvoice = *req.HasInvoice
		update["has_invoice"] = hasInvoice
	}
	if !hasInvoice {
		update["invoice_photo_url"] = nil
		update["invoice_photo_file_name"] = nil
		update["invoice_photo_file_size"] = nil
	} else {
		if req.InvoicePhotoURL != nil {
			update["invoice_photo_url"] = *req.InvoicePhotoURL
			if req.InvoicePhotoFileName == nil {
				update["invoice_photo_file_name"] = nil
			}
			if req.InvoicePhotoFileSize == nil {
				update["invoice_photo_file_size"] = nil
			}
		}
		if req.InvoicePhotoFileName != nil {
			if strings.TrimSpace(*req.InvoicePhotoFileName) == "" {
				update["invoice_photo_file_name"] = nil
			} else {
				update["invoice_photo_file_name"] = *req.InvoicePhotoFileName
			}
		}
		if req.InvoicePhotoFileSize != nil {
			if *req.InvoicePhotoFileSize <= 0 {
				update["invoice_photo_file_size"] = nil
			} else {
				update["invoice_photo_file_size"] = *req.InvoicePhotoFileSize
			}
		}
	}

	if err := s.applyUpdateCatalogFields(ctx, v, req, update); err != nil {
		return nil, err
	}

	_, err = s.vehicles().UpdateOne(ctx, bson.M{"_id": v.ID}, bson.M{"$set": update})
	if err != nil {
		return nil, fmt.Errorf("update vehicle: %w", err)
	}
	return s.GetVehicle(ctx, cpf, vehicleID)
}

// DeleteVehicle soft-deletes a vehicle and revokes conductor links; owner only.
// Uses a Mongo transaction when available. On standalone Mongo, soft-deletes first
// then revokes conductors; the operation is idempotent so retries after partial failure
// still ensure conductors are revoked.
func (s *VehicleService) DeleteVehicle(ctx context.Context, cpf, vehicleID string) error {
	v, err := s.findVehicleForOwnerDelete(ctx, vehicleID)
	if err != nil {
		return err
	}
	if v.OwnerCPF != cpf {
		// Soft-deleted vehicles must not leak existence to non-owners (404, not 403).
		if v.DeletedAt != nil {
			return ErrVehicleNotFound
		}
		return ErrMobilidadeForbidden
	}

	now := time.Now().UTC()
	session, err := s.database.Client().StartSession()
	if err != nil {
		return s.deleteVehicleSequential(ctx, v, now)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, s.deleteVehicleOps(sessCtx, v, now)
	})
	if err != nil {
		if isMongoTransactionUnsupported(err) {
			return s.deleteVehicleSequential(ctx, v, now)
		}
		return err
	}
	return nil
}

func (s *VehicleService) deleteVehicleSequential(ctx context.Context, v *models.Vehicle, now time.Time) error {
	return s.deleteVehicleOps(ctx, v, now)
}

func (s *VehicleService) deleteVehicleOps(ctx context.Context, v *models.Vehicle, now time.Time) error {
	// Soft-delete first so the vehicle disappears from wallets even if revoke fails mid-way.
	if v.DeletedAt == nil {
		res, err := s.vehicles().UpdateOne(ctx,
			bson.M{"_id": v.ID, "deleted_at": nil},
			bson.M{"$set": bson.M{
				"deleted_at": now,
				"updated_at": now,
			}},
		)
		if err != nil {
			return fmt.Errorf("soft delete vehicle: %w", err)
		}
		// MatchedCount 0: concurrent delete already soft-deleted — continue to revoke.
		if res.MatchedCount > 0 {
			deletedAt := now
			v.DeletedAt = &deletedAt
		}
	}

	if _, err := s.conductors().UpdateMany(ctx,
		bson.M{
			"vehicle_id": v.ID,
			"status": bson.M{"$in": []models.ConductorStatus{
				models.ConductorStatusPending,
				models.ConductorStatusAccepted,
			}},
		},
		bson.M{"$set": bson.M{
			"status":     models.ConductorStatusRevoked,
			"updated_at": now,
		}},
	); err != nil {
		return fmt.Errorf("revoke conductors: %w", err)
	}
	return nil
}

func isMongoTransactionUnsupported(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) && cmdErr.Code == 20 {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Transaction numbers are only allowed") ||
		strings.Contains(msg, "transaction numbers are only allowed")
}

func (s *VehicleService) findActiveVehicle(ctx context.Context, vehicleID string) (*models.Vehicle, error) {
	oid, err := primitive.ObjectIDFromHex(vehicleID)
	if err != nil {
		return nil, ErrVehicleNotFound
	}
	var v models.Vehicle
	err = s.vehicles().FindOne(ctx, bson.M{"_id": oid, "deleted_at": nil}).Decode(&v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrVehicleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find vehicle: %w", err)
	}
	return &v, nil
}

// findVehicleForOwnerDelete loads a vehicle by id including soft-deleted docs so delete is idempotent.
func (s *VehicleService) findVehicleForOwnerDelete(ctx context.Context, vehicleID string) (*models.Vehicle, error) {
	oid, err := primitive.ObjectIDFromHex(vehicleID)
	if err != nil {
		return nil, ErrVehicleNotFound
	}
	var v models.Vehicle
	err = s.vehicles().FindOne(ctx, bson.M{"_id": oid}).Decode(&v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrVehicleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find vehicle: %w", err)
	}
	return &v, nil
}

func (s *VehicleService) resolveRole(ctx context.Context, cpf string, v *models.Vehicle) (models.VehicleRole, string, error) {
	if v.OwnerCPF == cpf {
		return models.VehicleRoleOwner, "", nil
	}
	var link models.VehicleConductor
	err := s.conductors().FindOne(ctx, bson.M{
		"vehicle_id":    v.ID,
		"conductor_cpf": cpf,
		"status":        models.ConductorStatusAccepted,
	}).Decode(&link)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", "", ErrVehicleNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("resolve role: %w", err)
	}
	return models.VehicleRoleConductor, link.ID.Hex(), nil
}

func (s *VehicleService) resolveCreateCatalogFields(ctx context.Context, req *models.VehicleCreateRequest) (
	vehicleType models.VehicleType,
	brandID, brandOther, modelID, modelOther *string,
	err error,
) {
	catalogFlow := req.BrandID != nil && *req.BrandID != "" && req.ModelID != nil && *req.ModelID != ""
	if catalogFlow {
		var model models.VehicleModel
		err := s.models().FindOne(ctx, withCatalogActive(bson.M{"_id": *req.ModelID, "brand_id": *req.BrandID})).Decode(&model)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", nil, nil, nil, nil, fmt.Errorf("%w: model not found for brand", ErrMobilidadeInvalidInput)
		}
		if err != nil {
			return "", nil, nil, nil, nil, fmt.Errorf("load model: %w", err)
		}
		var brand models.VehicleBrand
		err = s.brands().FindOne(ctx, withCatalogActive(bson.M{"_id": *req.BrandID})).Decode(&brand)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", nil, nil, nil, nil, fmt.Errorf("%w: brand not found", ErrMobilidadeInvalidInput)
		}
		if err != nil {
			return "", nil, nil, nil, nil, fmt.Errorf("load brand: %w", err)
		}

		b := *req.BrandID
		m := *req.ModelID
		if brand.IsOther || model.IsOther {
			if brand.IsOther && (req.BrandOther == nil || *req.BrandOther == "") {
				return "", nil, nil, nil, nil, fmt.Errorf("%w: brand_other is required for Outro brand", ErrMobilidadeInvalidInput)
			}
			if model.IsOther && (req.ModelOther == nil || *req.ModelOther == "") {
				return "", nil, nil, nil, nil, fmt.Errorf("%w: model_other is required for Outro model", ErrMobilidadeInvalidInput)
			}
			vt := model.VehicleType
			if model.IsOther {
				if req.VehicleType == nil || !models.IsValidVehicleType(*req.VehicleType) {
					return "", nil, nil, nil, nil, fmt.Errorf("%w: vehicle_type is required for Outro model", ErrMobilidadeInvalidInput)
				}
				vt = *req.VehicleType
			}
			return vt, &b, req.BrandOther, &m, req.ModelOther, nil
		}
		return model.VehicleType, &b, nil, &m, nil, nil
	}

	if req.BrandID != nil && *req.BrandID != "" && (req.ModelID == nil || *req.ModelID == "") {
		if req.ModelOther == nil || strings.TrimSpace(*req.ModelOther) == "" {
			return "", nil, nil, nil, nil, fmt.Errorf("%w: model_other is required when model_id is empty", ErrMobilidadeInvalidInput)
		}
		if req.VehicleType == nil || !models.IsValidVehicleType(*req.VehicleType) {
			return "", nil, nil, nil, nil, fmt.Errorf("%w: vehicle_type is required for Outro flow", ErrMobilidadeInvalidInput)
		}
		var brand models.VehicleBrand
		err := s.brands().FindOne(ctx, withCatalogActive(bson.M{"_id": *req.BrandID})).Decode(&brand)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", nil, nil, nil, nil, fmt.Errorf("%w: brand not found", ErrMobilidadeInvalidInput)
		}
		if err != nil {
			return "", nil, nil, nil, nil, fmt.Errorf("load brand: %w", err)
		}
		b := *req.BrandID
		if brand.IsOther && (req.BrandOther == nil || *req.BrandOther == "") {
			return "", nil, nil, nil, nil, fmt.Errorf("%w: brand_other is required for Outro brand", ErrMobilidadeInvalidInput)
		}
		keptBrandOther := req.BrandOther
		if !brand.IsOther {
			keptBrandOther = nil
		}
		return *req.VehicleType, &b, keptBrandOther, nil, req.ModelOther, nil
	}

	if req.VehicleType == nil || !models.IsValidVehicleType(*req.VehicleType) {
		return "", nil, nil, nil, nil, fmt.Errorf("%w: vehicle_type is required for Outro flow", ErrMobilidadeInvalidInput)
	}
	return *req.VehicleType, nil, req.BrandOther, nil, req.ModelOther, nil
}

// applyUpdateCatalogFields merges PATCH catalog fields and, for catalog flow,
// re-derives vehicle_type from the model (same resilience as create).
// Explicit JSON null (or "") on brand_id/model_id clears catalog IDs so a switch
// to free-text Outro (brand_other/model_other) is not overridden by stale IDs.
func (s *VehicleService) applyUpdateCatalogFields(ctx context.Context, current *models.Vehicle, req *models.VehicleUpdateRequest, update bson.M) error {
	catalogTouched := req.BrandIDProvided() || req.ModelIDProvided() || req.BrandOtherProvided() ||
		req.ModelOtherProvided() || req.VehicleTypeProvided()
	if !catalogTouched {
		return nil
	}

	brandID := current.BrandID
	modelID := current.ModelID
	brandOther := current.BrandOther
	modelOther := current.ModelOther
	vehicleType := current.VehicleType

	if req.BrandIDProvided() {
		if req.BrandID == nil || *req.BrandID == "" {
			brandID = nil
		} else {
			b := *req.BrandID
			brandID = &b
		}
	}
	if req.ModelIDProvided() {
		if req.ModelID == nil || *req.ModelID == "" {
			modelID = nil
		} else {
			m := *req.ModelID
			modelID = &m
		}
	}
	if req.BrandOtherProvided() {
		if req.BrandOther == nil || *req.BrandOther == "" {
			brandOther = nil
		} else {
			b := *req.BrandOther
			brandOther = &b
		}
	}
	if req.ModelOtherProvided() {
		if req.ModelOther == nil || *req.ModelOther == "" {
			modelOther = nil
		} else {
			m := *req.ModelOther
			modelOther = &m
		}
	}
	if req.VehicleTypeProvided() && req.VehicleType != nil {
		vehicleType = *req.VehicleType
	}

	catalogFlow := brandID != nil && *brandID != "" && modelID != nil && *modelID != ""
	if catalogFlow {
		var model models.VehicleModel
		err := s.models().FindOne(ctx, withCatalogActive(bson.M{"_id": *modelID, "brand_id": *brandID})).Decode(&model)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("%w: model not found for brand", ErrMobilidadeInvalidInput)
		}
		if err != nil {
			return fmt.Errorf("load model: %w", err)
		}
		var brand models.VehicleBrand
		err = s.brands().FindOne(ctx, withCatalogActive(bson.M{"_id": *brandID})).Decode(&brand)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("%w: brand not found", ErrMobilidadeInvalidInput)
		}
		if err != nil {
			return fmt.Errorf("load brand: %w", err)
		}

		update["brand_id"] = *brandID
		update["model_id"] = *modelID
		if brand.IsOther || model.IsOther {
			if brand.IsOther && (brandOther == nil || *brandOther == "") {
				return fmt.Errorf("%w: brand_other is required for Outro brand", ErrMobilidadeInvalidInput)
			}
			if model.IsOther && (modelOther == nil || *modelOther == "") {
				return fmt.Errorf("%w: model_other is required for Outro model", ErrMobilidadeInvalidInput)
			}
			vt := model.VehicleType
			if model.IsOther {
				if !models.IsValidVehicleType(vehicleType) {
					return fmt.Errorf("%w: vehicle_type is required for Outro model", ErrMobilidadeInvalidInput)
				}
				vt = vehicleType
			}
			update["brand_other"] = brandOther
			update["model_other"] = modelOther
			update["vehicle_type"] = vt
			return nil
		}
		update["brand_other"] = nil
		update["model_other"] = nil
		update["vehicle_type"] = model.VehicleType
		return nil
	}

	if brandID != nil && *brandID != "" && (modelID == nil || *modelID == "") {
		if modelOther == nil || strings.TrimSpace(*modelOther) == "" {
			return fmt.Errorf("%w: model_other is required when model_id is empty", ErrMobilidadeInvalidInput)
		}
		if !models.IsValidVehicleType(vehicleType) {
			return fmt.Errorf("%w: vehicle_type is required for Outro flow", ErrMobilidadeInvalidInput)
		}
		var brand models.VehicleBrand
		err := s.brands().FindOne(ctx, withCatalogActive(bson.M{"_id": *brandID})).Decode(&brand)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("%w: brand not found", ErrMobilidadeInvalidInput)
		}
		if err != nil {
			return fmt.Errorf("load brand: %w", err)
		}
		if brand.IsOther && (brandOther == nil || *brandOther == "") {
			return fmt.Errorf("%w: brand_other is required for Outro brand", ErrMobilidadeInvalidInput)
		}
		update["brand_id"] = *brandID
		update["model_id"] = nil
		if brand.IsOther {
			update["brand_other"] = brandOther
		} else {
			update["brand_other"] = nil
		}
		update["model_other"] = modelOther
		update["vehicle_type"] = vehicleType
		return nil
	}

	otherFlow := (brandOther != nil && *brandOther != "") || (modelOther != nil && *modelOther != "")
	if otherFlow {
		if !models.IsValidVehicleType(vehicleType) {
			return fmt.Errorf("%w: vehicle_type is required for Outro flow", ErrMobilidadeInvalidInput)
		}
		update["brand_id"] = nil
		update["model_id"] = nil
		update["brand_other"] = brandOther
		update["model_other"] = modelOther
		update["vehicle_type"] = vehicleType
		return nil
	}

	// Type-only change is allowed only when vehicle is already on free-text Outro flow.
	currentOther := (current.BrandOther != nil && *current.BrandOther != "") || (current.ModelOther != nil && *current.ModelOther != "")
	currentCatalog := current.BrandID != nil && *current.BrandID != "" && current.ModelID != nil && *current.ModelID != ""
	if req.VehicleTypeProvided() && req.VehicleType != nil && currentOther && !currentCatalog &&
		!req.BrandIDProvided() && !req.ModelIDProvided() && !req.BrandOtherProvided() && !req.ModelOtherProvided() {
		if !models.IsValidVehicleType(*req.VehicleType) {
			return fmt.Errorf("%w: invalid vehicle_type", ErrMobilidadeInvalidInput)
		}
		update["vehicle_type"] = *req.VehicleType
		return nil
	}

	return fmt.Errorf("%w: incomplete brand/model update; provide catalog brand_id+model_id or Outro fields", ErrMobilidadeInvalidInput)
}

func (s *VehicleService) loadOwnerSnapshot(ctx context.Context, cpf string) (name, phone, email string) {
	return loadCitizenContactProfile(ctx, s.database, s.dataManager, s.logger, cpf)
}

// loadCitizenContactProfile resolves name/phone/email from citizen + self-declared overlay.
func loadCitizenContactProfile(ctx context.Context, db *mongo.Database, dm *DataManager, logger *logging.SafeLogger, cpf string) (name, phone, email string) {
	cpf = utils.NormalizeCPF(cpf)
	if cpf == "" {
		return "", "", ""
	}

	citizen, err := readCitizenContact(ctx, db, dm, logger, cpf)
	if err != nil {
		return "", "", ""
	}
	if citizen.NomeExibicao != nil && *citizen.NomeExibicao != "" {
		name = *citizen.NomeExibicao
	} else if citizen.NomeSocial != nil && *citizen.NomeSocial != "" {
		name = *citizen.NomeSocial
	} else if citizen.Nome != nil {
		name = *citizen.Nome
	}
	phone = formatTelefonePrincipal(citizen.Telefone)
	if citizen.Email != nil && citizen.Email.Principal != nil && citizen.Email.Principal.Valor != nil {
		email = *citizen.Email.Principal.Valor
	}

	var sd models.SelfDeclaredData
	var sdErr error
	if dm != nil {
		sdErr = dm.Read(ctx, cpf, config.AppConfig.SelfDeclaredCollection, "self_declared", &sd)
	} else if db != nil {
		sdErr = db.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&sd)
	}
	if sdErr == nil {
		if sd.NomeExibicao != nil && *sd.NomeExibicao != "" {
			name = *sd.NomeExibicao
		}
		if p := formatTelefonePrincipal(sd.Telefone); p != "" {
			phone = p
		}
		if sd.Email != nil && sd.Email.Principal != nil && sd.Email.Principal.Valor != nil && *sd.Email.Principal.Valor != "" {
			email = *sd.Email.Principal.Valor
		}
	}
	return name, phone, email
}

func readCitizenContact(ctx context.Context, db *mongo.Database, dm *DataManager, logger *logging.SafeLogger, cpf string) (*models.Citizen, error) {
	const maxAttempts = 3
	var citizen models.Citizen
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if dm != nil {
			err = dm.Read(ctx, cpf, config.AppConfig.CitizenCollection, "citizen", &citizen)
		} else if db != nil {
			err = db.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
		} else {
			return nil, fmt.Errorf("no data source")
		}
		if err == nil {
			return &citizen, nil
		}
		if logger != nil {
			logger.Warn("mobilidade citizen contact profile miss",
				zap.String("cpf", utils.MaskCPF(cpf)),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
		}
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	if db != nil {
		var byID models.Citizen
		if idErr := db.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"_id": cpf}).Decode(&byID); idErr == nil {
			return &byID, nil
		}
	}
	return nil, err
}

func formatTelefonePrincipal(tel *models.Telefone) string {
	if tel == nil || tel.Principal == nil || tel.Principal.Valor == nil || *tel.Principal.Valor == "" {
		return ""
	}
	ddi, ddd, valor := "", "", *tel.Principal.Valor
	if tel.Principal.DDI != nil {
		ddi = *tel.Principal.DDI
	}
	if tel.Principal.DDD != nil {
		ddd = *tel.Principal.DDD
	}
	if ddi != "" || ddd != "" {
		return utils.FormatPhoneForStorage(ddi, ddd, valor)
	}
	return valor
}

func (s *VehicleService) enrichOwnerFields(ctx context.Context, v *models.Vehicle) {
	if v == nil || v.OwnerCPF == "" {
		return
	}
	name, phone, email := s.loadOwnerSnapshot(ctx, v.OwnerCPF)
	v.OwnerName = name
	v.OwnerPhone = phone
	v.OwnerEmail = email
}

// enrichVehicleCatalogNames fills brand_name/model_name from the catalog when IDs are set
// and free-text Outro fields are empty (front uses *_other when present).
func (s *VehicleService) enrichVehicleCatalogNames(ctx context.Context, v *models.Vehicle) {
	if v == nil {
		return
	}
	v.BrandName, v.ModelName = s.resolveCatalogNames(ctx, v.BrandID, v.BrandOther, v.ModelID, v.ModelOther)
}

func (s *VehicleService) enrichListCatalogNames(ctx context.Context, items []models.VehicleListItem) {
	if len(items) == 0 {
		return
	}
	brandIDs := make([]string, 0, len(items))
	modelIDs := make([]string, 0, len(items))
	seenBrand := map[string]struct{}{}
	seenModel := map[string]struct{}{}
	for i := range items {
		if items[i].BrandID != nil && *items[i].BrandID != "" && !hasFreeText(items[i].BrandOther) {
			id := *items[i].BrandID
			if _, ok := seenBrand[id]; !ok {
				seenBrand[id] = struct{}{}
				brandIDs = append(brandIDs, id)
			}
		}
		if items[i].ModelID != nil && *items[i].ModelID != "" && !hasFreeText(items[i].ModelOther) {
			id := *items[i].ModelID
			if _, ok := seenModel[id]; !ok {
				seenModel[id] = struct{}{}
				modelIDs = append(modelIDs, id)
			}
		}
	}
	brandNames := s.loadBrandNames(ctx, brandIDs)
	modelNames := s.loadModelNames(ctx, modelIDs)
	for i := range items {
		if items[i].BrandID != nil && !hasFreeText(items[i].BrandOther) {
			if name, ok := brandNames[*items[i].BrandID]; ok {
				n := name
				items[i].BrandName = &n
			}
		}
		if items[i].ModelID != nil && !hasFreeText(items[i].ModelOther) {
			if name, ok := modelNames[*items[i].ModelID]; ok {
				n := name
				items[i].ModelName = &n
			}
		}
	}
}

func (s *VehicleService) resolveCatalogNames(ctx context.Context, brandID, brandOther, modelID, modelOther *string) (brandName, modelName *string) {
	if brandID != nil && *brandID != "" && !hasFreeText(brandOther) {
		names := s.loadBrandNames(ctx, []string{*brandID})
		if name, ok := names[*brandID]; ok {
			n := name
			brandName = &n
		}
	}
	if modelID != nil && *modelID != "" && !hasFreeText(modelOther) {
		names := s.loadModelNames(ctx, []string{*modelID})
		if name, ok := names[*modelID]; ok {
			n := name
			modelName = &n
		}
	}
	return brandName, modelName
}

func hasFreeText(other *string) bool {
	return other != nil && strings.TrimSpace(*other) != ""
}

func (s *VehicleService) loadBrandNames(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	cursor, err := s.brands().Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return out
	}
	defer func() { _ = cursor.Close(ctx) }()
	var brands []models.VehicleBrand
	if err := cursor.All(ctx, &brands); err != nil {
		return out
	}
	for _, b := range brands {
		if b.Name != "" {
			out[b.ID] = b.Name
		}
	}
	return out
}

func (s *VehicleService) loadModelNames(ctx context.Context, ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	cursor, err := s.models().Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return out
	}
	defer func() { _ = cursor.Close(ctx) }()
	var modelsList []models.VehicleModel
	if err := cursor.All(ctx, &modelsList); err != nil {
		return out
	}
	for _, m := range modelsList {
		if m.Name != "" {
			out[m.ID] = m.Name
		}
	}
	return out
}

// nextRegistrationNumber allocates a short unique wallet id (format RJ-E-XXXXXX).
func (s *VehicleService) nextRegistrationNumber(ctx context.Context) (string, error) {
	counters := s.database.Collection("mobilidade_registration_counters")
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var result struct {
		Seq int64 `bson:"seq"`
	}
	err := counters.FindOneAndUpdate(
		ctx,
		bson.M{"_id": "vehicle"},
		bson.M{"$inc": bson.M{"seq": 1}},
		opts,
	).Decode(&result)
	if err != nil {
		return "", fmt.Errorf("allocate registration_number: %w", err)
	}
	// Prefer RJ-E-XXXXXX (6 digits); allow growth past 999999 without truncating uniqueness.
	if result.Seq > 999999 {
		return fmt.Sprintf("RJ-E-%d", result.Seq), nil
	}
	return fmt.Sprintf("RJ-E-%06d", result.Seq), nil
}

func toListItem(v models.Vehicle, role models.VehicleRole, conductorID string) models.VehicleListItem {
	return models.VehicleListItem{
		ID:                 v.ID.Hex(),
		DisplayName:        v.DisplayName,
		RegistrationNumber: v.RegistrationNumber,
		BrandID:            v.BrandID,
		BrandOther:         v.BrandOther,
		BrandName:          nil,
		ModelID:            v.ModelID,
		ModelOther:         v.ModelOther,
		ModelName:          nil,
		VehicleType:        v.VehicleType,
		Color:              v.Color,
		VehiclePhotoURL:    v.VehiclePhotoURL,
		Role:               role,
		ConductorID:        conductorID,
	}
}
