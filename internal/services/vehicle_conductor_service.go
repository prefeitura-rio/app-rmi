package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// VehicleConductorServiceInstance is the process-wide conductor service used by handlers.
var VehicleConductorServiceInstance *VehicleConductorService

// VehicleConductorService manages invites and conductor links.
type VehicleConductorService struct {
	database    *mongo.Database
	dataManager *DataManager
	logger      *logging.SafeLogger
}

// NewVehicleConductorService creates a VehicleConductorService.
func NewVehicleConductorService(database *mongo.Database, logger *logging.SafeLogger) *VehicleConductorService {
	return &VehicleConductorService{database: database, logger: logger}
}

// InitVehicleConductorService initializes VehicleConductorServiceInstance.
func InitVehicleConductorService() {
	logger := logging.GetLogger()
	dm := NewDataManager(config.Redis, config.MongoDB, logger)
	svc := NewVehicleConductorService(config.MongoDB, logger)
	svc.dataManager = dm
	VehicleConductorServiceInstance = svc
}

func (s *VehicleConductorService) vehicles() *mongo.Collection {
	return s.database.Collection(config.AppConfig.RioMobVehicleCollection)
}

func (s *VehicleConductorService) conductors() *mongo.Collection {
	return s.database.Collection(config.AppConfig.RioMobConductorCollection)
}

func (s *VehicleConductorService) brands() *mongo.Collection {
	return s.database.Collection(config.AppConfig.RioMobBrandCollection)
}

func (s *VehicleConductorService) modelsColl() *mongo.Collection {
	return s.database.Collection(config.AppConfig.RioMobModelCollection)
}

// ListInvitations returns pending invitations for conductor_cpf = cpf.
func (s *VehicleConductorService) ListInvitations(ctx context.Context, cpf string) (*models.VehicleInvitationsResponse, error) {
	cursor, err := s.conductors().Find(ctx, bson.M{
		"conductor_cpf": cpf,
		"status":        models.ConductorStatusPending,
	})
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer cursor.Close(ctx)

	var links []models.VehicleConductor
	if err := cursor.All(ctx, &links); err != nil {
		return nil, fmt.Errorf("decode invitations: %w", err)
	}

	items := make([]models.VehicleInvitationItem, 0, len(links))
	for _, link := range links {
		var v models.Vehicle
		err := s.vehicles().FindOne(ctx, bson.M{"_id": link.VehicleID, "deleted_at": nil}).Decode(&v)
		if err != nil {
			continue
		}
		brandLabel, modelLabel := s.resolveBrandModelLabels(ctx, &v)
		items = append(items, models.VehicleInvitationItem{
			ID:           link.ID.Hex(),
			VehicleID:    v.ID.Hex(),
			Status:       link.Status,
			InvitedByCPF: link.InvitedByCPF,
			OwnerName:    v.OwnerName,
			Vehicle: models.VehicleInvitationSummary{
				DisplayName:     v.DisplayName,
				BrandLabel:      brandLabel,
				ModelLabel:      modelLabel,
				VehicleType:     v.VehicleType,
				Color:           v.Color,
				VehiclePhotoURL: v.VehiclePhotoURL,
			},
			CreatedAt: link.CreatedAt,
		})
	}

	return &models.VehicleInvitationsResponse{Data: items}, nil
}

// RespondInvitation accepts or rejects a pending invitation for cpf.
func (s *VehicleConductorService) RespondInvitation(ctx context.Context, cpf, conductorID string, req *models.RespondInvitationRequest) (*models.VehicleConductor, error) {
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRioMobInvalidInput, err.Error())
	}

	link, err := s.findConductorByID(ctx, conductorID)
	if err != nil {
		return nil, err
	}
	if link.ConductorCPF != cpf {
		return nil, ErrRioMobForbidden
	}
	if link.Status != models.ConductorStatusPending {
		return nil, fmt.Errorf("%w: invitation is not pending", ErrRioMobInvalidInput)
	}

	// Reject accept/reject against soft-deleted vehicles.
	if _, err := s.findActiveVehicle(ctx, link.VehicleID.Hex()); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	res, err := s.conductors().UpdateOne(ctx,
		bson.M{
			"_id":           link.ID,
			"conductor_cpf": cpf,
			"status":        models.ConductorStatusPending,
		},
		bson.M{"$set": bson.M{
			"status":       req.Status,
			"responded_at": now,
			"updated_at":   now,
		}},
	)
	if err != nil {
		return nil, fmt.Errorf("respond invitation: %w", err)
	}
	if res.MatchedCount == 0 {
		return nil, fmt.Errorf("%w: invitation is not pending", ErrRioMobInvalidInput)
	}

	link.Status = req.Status
	link.RespondedAt = &now
	link.UpdatedAt = now
	return link, nil
}

// ListConductors lists links for a vehicle; owner only.
func (s *VehicleConductorService) ListConductors(ctx context.Context, cpf, vehicleID string) (*models.ConductorsListResponse, error) {
	v, err := s.requireOwnerVehicle(ctx, cpf, vehicleID)
	if err != nil {
		return nil, err
	}

	cursor, err := s.conductors().Find(ctx, bson.M{
		"vehicle_id": v.ID,
		"status": bson.M{"$in": []models.ConductorStatus{
			models.ConductorStatusPending,
			models.ConductorStatusAccepted,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("list conductors: %w", err)
	}
	defer cursor.Close(ctx)

	var list []models.VehicleConductor
	if err := cursor.All(ctx, &list); err != nil {
		return nil, fmt.Errorf("decode conductors: %w", err)
	}
	if list == nil {
		list = []models.VehicleConductor{}
	}
	return &models.ConductorsListResponse{Data: list}, nil
}

// InviteConductor creates a pending link and enqueues email notification; owner only.
func (s *VehicleConductorService) InviteConductor(ctx context.Context, cpf, vehicleID string, req *models.InviteConductorRequest) (*models.VehicleConductor, error) {
	if !utils.ValidateCPF(req.CPF) {
		return nil, fmt.Errorf("%w: invalid conductor cpf", ErrRioMobInvalidInput)
	}
	if req.Email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrRioMobInvalidInput)
	}

	v, err := s.requireOwnerVehicle(ctx, cpf, vehicleID)
	if err != nil {
		return nil, err
	}
	if req.CPF == v.OwnerCPF {
		return nil, fmt.Errorf("%w: cannot invite vehicle owner", ErrRioMobInvalidInput)
	}

	existingCount, err := s.conductors().CountDocuments(ctx, bson.M{
		"vehicle_id":    v.ID,
		"conductor_cpf": req.CPF,
		"status": bson.M{"$in": []models.ConductorStatus{
			models.ConductorStatusPending,
			models.ConductorStatusAccepted,
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("check duplicate invite: %w", err)
	}
	if existingCount > 0 {
		return nil, ErrRioMobConflict
	}

	conductorName := req.Name
	if conductorName == "" {
		conductorName = s.lookupConductorName(ctx, req.CPF)
	}

	now := time.Now().UTC()
	link := models.VehicleConductor{
		ID:            primitive.NewObjectID(),
		VehicleID:     v.ID,
		ConductorCPF:  req.CPF,
		ConductorName: conductorName,
		NotifyEmail:   req.Email,
		Status:        models.ConductorStatusPending,
		InvitedByCPF:  cpf,
		EmailStatus:   models.InviteEmailStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if _, err := s.conductors().InsertOne(ctx, link); err != nil {
		return nil, mapConductorInsertError(err)
	}

	s.enqueueInviteEmail(ctx, &link, v)
	return &link, nil
}

func (s *VehicleConductorService) lookupConductorName(ctx context.Context, cpf string) string {
	var citizen models.Citizen
	var err error
	if s.dataManager != nil {
		err = s.dataManager.Read(ctx, cpf, config.AppConfig.CitizenCollection, "citizen", &citizen)
	} else {
		err = s.database.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
	}
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("riomob conductor name lookup miss", zap.String("cpf", cpf), zap.Error(err))
		}
		return ""
	}
	if citizen.Nome != nil {
		return *citizen.Nome
	}
	return ""
}

func mapConductorInsertError(err error) error {
	if mongo.IsDuplicateKeyError(err) {
		return ErrRioMobConflict
	}
	return fmt.Errorf("insert conductor: %w", err)
}

// RemoveConductor revokes a link (owner any link, or accepted conductor self-leave).
func (s *VehicleConductorService) RemoveConductor(ctx context.Context, cpf, vehicleID, conductorID string) error {
	v, err := s.findActiveVehicle(ctx, vehicleID)
	if err != nil {
		return err
	}
	link, err := s.findConductorByID(ctx, conductorID)
	if err != nil {
		return err
	}
	if link.VehicleID != v.ID {
		return ErrConductorNotFound
	}

	isOwner := v.OwnerCPF == cpf
	isSelfLeave := link.ConductorCPF == cpf && link.Status == models.ConductorStatusAccepted
	if !isOwner && !isSelfLeave {
		return ErrRioMobForbidden
	}
	if isOwner && link.Status != models.ConductorStatusPending && link.Status != models.ConductorStatusAccepted {
		return fmt.Errorf("%w: link cannot be revoked", ErrRioMobInvalidInput)
	}

	now := time.Now().UTC()
	_, err = s.conductors().UpdateOne(ctx, bson.M{"_id": link.ID}, bson.M{"$set": bson.M{
		"status":     models.ConductorStatusRevoked,
		"updated_at": now,
	}})
	if err != nil {
		return fmt.Errorf("revoke conductor: %w", err)
	}
	return nil
}

func (s *VehicleConductorService) requireOwnerVehicle(ctx context.Context, cpf, vehicleID string) (*models.Vehicle, error) {
	v, err := s.findActiveVehicle(ctx, vehicleID)
	if err != nil {
		return nil, err
	}
	if v.OwnerCPF != cpf {
		return nil, ErrRioMobForbidden
	}
	return v, nil
}

func (s *VehicleConductorService) findActiveVehicle(ctx context.Context, vehicleID string) (*models.Vehicle, error) {
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

func (s *VehicleConductorService) findConductorByID(ctx context.Context, conductorID string) (*models.VehicleConductor, error) {
	oid, err := primitive.ObjectIDFromHex(conductorID)
	if err != nil {
		return nil, ErrConductorNotFound
	}
	var link models.VehicleConductor
	err = s.conductors().FindOne(ctx, bson.M{"_id": oid}).Decode(&link)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrConductorNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find conductor: %w", err)
	}
	return &link, nil
}

func (s *VehicleConductorService) resolveBrandModelLabels(ctx context.Context, v *models.Vehicle) (brandLabel, modelLabel string) {
	if v.BrandOther != nil && *v.BrandOther != "" {
		brandLabel = *v.BrandOther
	} else if v.BrandID != nil {
		var brand models.VehicleBrand
		if err := s.brands().FindOne(ctx, bson.M{"_id": *v.BrandID}).Decode(&brand); err == nil {
			brandLabel = brand.Name
		}
	}
	if v.ModelOther != nil && *v.ModelOther != "" {
		modelLabel = *v.ModelOther
	} else if v.ModelID != nil {
		var model models.VehicleModel
		if err := s.modelsColl().FindOne(ctx, bson.M{"_id": *v.ModelID}).Decode(&model); err == nil {
			modelLabel = model.Name
		}
	}
	return brandLabel, modelLabel
}

func (s *VehicleConductorService) enqueueInviteEmail(ctx context.Context, link *models.VehicleConductor, v *models.Vehicle) {
	if config.Redis == nil {
		s.persistInviteEmailStatus(ctx, link, models.InviteEmailStatusFailed, "redis unavailable")
		return
	}

	payload := RioMobInviteEmailPayload{
		ConductorID:  link.ID.Hex(),
		NotifyEmail:  link.NotifyEmail,
		VehicleID:    v.ID.Hex(),
		OwnerName:    v.OwnerName,
		DisplayName:  v.DisplayName,
		ConductorCPF: link.ConductorCPF,
	}

	job := SyncJob{
		ID:         primitive.NewObjectID().Hex(),
		Type:       RioMobInviteEmailQueue,
		Key:        link.ID.Hex(),
		Collection: RioMobInviteEmailQueue,
		Data:       payload,
		Timestamp:  time.Now().UTC(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	jobBytes, err := json.Marshal(job)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("riomob invite email marshal failed", zap.Error(err))
		}
		s.persistInviteEmailStatus(ctx, link, models.InviteEmailStatusFailed, err.Error())
		return
	}

	queueKey := fmt.Sprintf("sync:queue:%s", RioMobInviteEmailQueue)
	if err := config.Redis.LPush(ctx, queueKey, jobBytes).Err(); err != nil {
		if s.logger != nil {
			s.logger.Warn("riomob invite email enqueue failed", zap.Error(err), zap.String("queue", queueKey))
		}
		s.persistInviteEmailStatus(ctx, link, models.InviteEmailStatusFailed, err.Error())
		return
	}

	s.persistInviteEmailStatus(ctx, link, models.InviteEmailStatusQueued, "")
}

func (s *VehicleConductorService) persistInviteEmailStatus(ctx context.Context, link *models.VehicleConductor, status models.InviteEmailStatus, lastError string) {
	if err := SetConductorInviteEmailStatus(ctx, s.database, link.ID.Hex(), status, lastError); err != nil {
		if s.logger != nil {
			s.logger.Warn("riomob invite email status update failed",
				zap.String("conductor_id", link.ID.Hex()),
				zap.String("email_status", string(status)),
				zap.Error(err))
		}
		return
	}
	link.EmailStatus = status
	now := time.Now().UTC()
	link.EmailAttemptedAt = &now
	if lastError != "" {
		link.EmailLastError = &lastError
	} else {
		link.EmailLastError = nil
	}
}

// SetConductorInviteEmailStatus persists invite email delivery status on the conductor link.
func SetConductorInviteEmailStatus(ctx context.Context, db *mongo.Database, conductorID string, status models.InviteEmailStatus, lastError string) error {
	if db == nil || config.AppConfig == nil {
		return fmt.Errorf("database not configured")
	}
	oid, err := primitive.ObjectIDFromHex(conductorID)
	if err != nil {
		return fmt.Errorf("invalid conductor id: %w", err)
	}
	now := time.Now().UTC()
	set := bson.M{
		"email_status":       status,
		"email_attempted_at": now,
		"updated_at":         now,
	}
	if lastError != "" {
		set["email_last_error"] = lastError
	} else {
		set["email_last_error"] = nil
	}
	_, err = db.Collection(config.AppConfig.RioMobConductorCollection).UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": set})
	return err
}
