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
			item:      toListItem(v, models.VehicleRoleOwner),
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
			item:      toListItem(v, models.VehicleRoleConductor),
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

	role, err := s.resolveRole(ctx, cpf, v)
	if err != nil {
		return nil, err
	}

	s.enrichOwnerFields(ctx, v)
	return &models.VehicleDetail{Vehicle: *v, Role: role}, nil
}

// CreateVehicle registers a new vehicle owned by cpf.
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

	ownerName, ownerPhone, ownerEmail := s.loadOwnerSnapshot(ctx, cpf)
	now := time.Now().UTC()
	vehicle := models.Vehicle{
		ID:                        primitive.NewObjectID(),
		OwnerCPF:                  cpf,
		OwnerName:                 ownerName,
		OwnerPhone:                ownerPhone,
		OwnerEmail:                ownerEmail,
		DisplayName:               req.DisplayName,
		BrandID:                   brandID,
		BrandOther:                brandOther,
		ModelID:                   modelID,
		ModelOther:                modelOther,
		VehicleType:               vehicleType,
		Color:                     req.Color,
		SerialNumber:              req.SerialNumber,
		SerialNumberPhotoURL:      req.SerialNumberPhotoURL,
		VehiclePhotoURL:           req.VehiclePhotoURL,
		InvoicePhotoURL:           req.InvoicePhotoURL,
		HasInvoice:                req.HasInvoice,
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

	if _, err := s.vehicles().InsertOne(ctx, vehicle); err != nil {
		return nil, fmt.Errorf("insert vehicle: %w", err)
	}
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
	}
	if req.VehiclePhotoURL != nil {
		update["vehicle_photo_url"] = *req.VehiclePhotoURL
	}
	if req.SerialNumberPhotoFileName != nil {
		update["serial_number_photo_file_name"] = *req.SerialNumberPhotoFileName
	}
	if req.SerialNumberPhotoFileSize != nil {
		update["serial_number_photo_file_size"] = *req.SerialNumberPhotoFileSize
	}
	if req.VehiclePhotoFileName != nil {
		update["vehicle_photo_file_name"] = *req.VehiclePhotoFileName
	}
	if req.VehiclePhotoFileSize != nil {
		update["vehicle_photo_file_size"] = *req.VehiclePhotoFileSize
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
		}
		if req.InvoicePhotoFileName != nil {
			update["invoice_photo_file_name"] = *req.InvoicePhotoFileName
		}
		if req.InvoicePhotoFileSize != nil {
			update["invoice_photo_file_size"] = *req.InvoicePhotoFileSize
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

// DeleteVehicle soft-deletes a vehicle and revokes conductor links atomically; owner only.
// Uses a Mongo transaction when available; falls back to revoke-then-delete on standalone.
func (s *VehicleService) DeleteVehicle(ctx context.Context, cpf, vehicleID string) error {
	v, err := s.findActiveVehicle(ctx, vehicleID)
	if err != nil {
		return err
	}
	if v.OwnerCPF != cpf {
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
	if res.MatchedCount == 0 {
		return ErrVehicleNotFound
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

func (s *VehicleService) resolveRole(ctx context.Context, cpf string, v *models.Vehicle) (models.VehicleRole, error) {
	if v.OwnerCPF == cpf {
		return models.VehicleRoleOwner, nil
	}
	var link models.VehicleConductor
	err := s.conductors().FindOne(ctx, bson.M{
		"vehicle_id":    v.ID,
		"conductor_cpf": cpf,
		"status":        models.ConductorStatusAccepted,
	}).Decode(&link)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", ErrVehicleNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve role: %w", err)
	}
	return models.VehicleRoleConductor, nil
}

func (s *VehicleService) resolveCreateCatalogFields(ctx context.Context, req *models.VehicleCreateRequest) (
	vehicleType models.VehicleType,
	brandID, brandOther, modelID, modelOther *string,
	err error,
) {
	catalogFlow := req.BrandID != nil && *req.BrandID != "" && req.ModelID != nil && *req.ModelID != ""
	if catalogFlow {
		var model models.VehicleModel
		err := s.models().FindOne(ctx, bson.M{"_id": *req.ModelID, "brand_id": *req.BrandID}).Decode(&model)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", nil, nil, nil, nil, fmt.Errorf("%w: model not found for brand", ErrMobilidadeInvalidInput)
		}
		if err != nil {
			return "", nil, nil, nil, nil, fmt.Errorf("load model: %w", err)
		}
		var brand models.VehicleBrand
		err = s.brands().FindOne(ctx, bson.M{"_id": *req.BrandID}).Decode(&brand)
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

	if req.VehicleType == nil || !models.IsValidVehicleType(*req.VehicleType) {
		return "", nil, nil, nil, nil, fmt.Errorf("%w: vehicle_type is required for Outro flow", ErrMobilidadeInvalidInput)
	}
	return *req.VehicleType, nil, req.BrandOther, nil, req.ModelOther, nil
}

// applyUpdateCatalogFields merges PATCH catalog fields and, for catalog flow,
// re-derives vehicle_type from the model (same resilience as create).
func (s *VehicleService) applyUpdateCatalogFields(ctx context.Context, current *models.Vehicle, req *models.VehicleUpdateRequest, update bson.M) error {
	catalogTouched := req.BrandID != nil || req.ModelID != nil || req.BrandOther != nil || req.ModelOther != nil || req.VehicleType != nil
	if !catalogTouched {
		return nil
	}

	brandID := current.BrandID
	modelID := current.ModelID
	brandOther := current.BrandOther
	modelOther := current.ModelOther
	vehicleType := current.VehicleType

	if req.BrandID != nil {
		if *req.BrandID == "" {
			brandID = nil
		} else {
			b := *req.BrandID
			brandID = &b
		}
	}
	if req.ModelID != nil {
		if *req.ModelID == "" {
			modelID = nil
		} else {
			m := *req.ModelID
			modelID = &m
		}
	}
	if req.BrandOther != nil {
		if *req.BrandOther == "" {
			brandOther = nil
		} else {
			b := *req.BrandOther
			brandOther = &b
		}
	}
	if req.ModelOther != nil {
		if *req.ModelOther == "" {
			modelOther = nil
		} else {
			m := *req.ModelOther
			modelOther = &m
		}
	}
	if req.VehicleType != nil {
		vehicleType = *req.VehicleType
	}

	catalogFlow := brandID != nil && *brandID != "" && modelID != nil && *modelID != ""
	if catalogFlow {
		var model models.VehicleModel
		err := s.models().FindOne(ctx, bson.M{"_id": *modelID, "brand_id": *brandID}).Decode(&model)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return fmt.Errorf("%w: model not found for brand", ErrMobilidadeInvalidInput)
		}
		if err != nil {
			return fmt.Errorf("load model: %w", err)
		}
		var brand models.VehicleBrand
		err = s.brands().FindOne(ctx, bson.M{"_id": *brandID}).Decode(&brand)
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
	if req.VehicleType != nil && currentOther && !currentCatalog &&
		req.BrandID == nil && req.ModelID == nil && req.BrandOther == nil && req.ModelOther == nil {
		if !models.IsValidVehicleType(*req.VehicleType) {
			return fmt.Errorf("%w: invalid vehicle_type", ErrMobilidadeInvalidInput)
		}
		update["vehicle_type"] = *req.VehicleType
		return nil
	}

	return fmt.Errorf("%w: incomplete brand/model update; provide catalog brand_id+model_id or Outro fields", ErrMobilidadeInvalidInput)
}

func (s *VehicleService) loadOwnerSnapshot(ctx context.Context, cpf string) (name, phone, email string) {
	const maxAttempts = 3
	var citizen models.Citizen
	var err error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if s.dataManager != nil {
			err = s.dataManager.Read(ctx, cpf, config.AppConfig.CitizenCollection, "citizen", &citizen)
		} else {
			err = s.database.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
		}
		if err == nil {
			break
		}
		if s.logger != nil {
			s.logger.Warn("mobilidade owner snapshot miss",
				zap.String("cpf", cpf),
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
		}
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return "", "", ""
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	if err != nil {
		return "", "", ""
	}
	if citizen.Nome != nil {
		name = *citizen.Nome
	}
	phone = formatTelefonePrincipal(citizen.Telefone)
	if citizen.Email != nil && citizen.Email.Principal != nil && citizen.Email.Principal.Valor != nil {
		email = *citizen.Email.Principal.Valor
	}

	// Overlay self-declared display name / phone / email when present.
	var sd models.SelfDeclaredData
	sdErr := error(nil)
	if s.dataManager != nil {
		sdErr = s.dataManager.Read(ctx, cpf, config.AppConfig.SelfDeclaredCollection, "self_declared", &sd)
	} else {
		sdErr = s.database.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&sd)
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
	if v.OwnerName != "" && v.OwnerPhone != "" && v.OwnerEmail != "" {
		return
	}
	name, phone, email := s.loadOwnerSnapshot(ctx, v.OwnerCPF)
	changed := false
	if v.OwnerName == "" && name != "" {
		v.OwnerName = name
		changed = true
	}
	if v.OwnerPhone == "" && phone != "" {
		v.OwnerPhone = phone
		changed = true
	}
	if v.OwnerEmail == "" && email != "" {
		v.OwnerEmail = email
		changed = true
	}
	if changed {
		update := bson.M{"updated_at": time.Now().UTC()}
		if v.OwnerName != "" {
			update["owner_name"] = v.OwnerName
		}
		if v.OwnerPhone != "" {
			update["owner_phone"] = v.OwnerPhone
		}
		if v.OwnerEmail != "" {
			update["owner_email"] = v.OwnerEmail
		}
		if _, err := s.vehicles().UpdateOne(ctx, bson.M{"_id": v.ID}, bson.M{"$set": update}); err != nil {
			if s.logger != nil {
				s.logger.Warn("mobilidade owner backfill failed",
					zap.String("vehicle_id", v.ID.Hex()),
					zap.Error(err),
				)
			}
		}
	}
}

func toListItem(v models.Vehicle, role models.VehicleRole) models.VehicleListItem {
	return models.VehicleListItem{
		ID:              v.ID.Hex(),
		DisplayName:     v.DisplayName,
		BrandID:         v.BrandID,
		BrandOther:      v.BrandOther,
		ModelID:         v.ModelID,
		ModelOther:      v.ModelOther,
		VehicleType:     v.VehicleType,
		Color:           v.Color,
		VehiclePhotoURL: v.VehiclePhotoURL,
		Role:            role,
	}
}
