package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ConductorStatus covers the full invite/conductor lifecycle on one resource.
type ConductorStatus string

const (
	ConductorStatusPending  ConductorStatus = "pending"
	ConductorStatusAccepted ConductorStatus = "accepted"
	ConductorStatusRejected ConductorStatus = "rejected"
	ConductorStatusRevoked  ConductorStatus = "revoked"
)

// IsValidConductorStatus reports whether s is a known status.
func IsValidConductorStatus(s ConductorStatus) bool {
	switch s {
	case ConductorStatusPending, ConductorStatusAccepted, ConductorStatusRejected, ConductorStatusRevoked:
		return true
	default:
		return false
	}
}

// InviteEmailStatus tracks invite notification delivery on the conductor link (handoff).
type InviteEmailStatus string

const (
	InviteEmailStatusPending InviteEmailStatus = "pending"
	InviteEmailStatusQueued  InviteEmailStatus = "queued"
	InviteEmailStatusSent    InviteEmailStatus = "sent"
	InviteEmailStatusFailed  InviteEmailStatus = "failed"
)

// VehicleConductor is the invite/active-conductor link for a vehicle.
type VehicleConductor struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	VehicleID     primitive.ObjectID `bson:"vehicle_id" json:"vehicle_id"`
	ConductorCPF  string             `bson:"conductor_cpf" json:"conductor_cpf"`
	ConductorName string             `bson:"conductor_name" json:"conductor_name"`
	NotifyEmail   string             `bson:"notify_email" json:"notify_email"`
	Status        ConductorStatus    `bson:"status" json:"status"`
	InvitedByCPF  string             `bson:"invited_by_cpf" json:"invited_by_cpf"`

	// Invite email delivery (handoff: register attempt/status on the link).
	EmailStatus      InviteEmailStatus `bson:"email_status" json:"email_status"`
	EmailAttemptedAt *time.Time        `bson:"email_attempted_at,omitempty" json:"email_attempted_at,omitempty"`
	EmailLastError   *string           `bson:"email_last_error,omitempty" json:"-"`

	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at" json:"updated_at"`
	RespondedAt *time.Time `bson:"responded_at,omitempty" json:"responded_at"`
}

// InviteConductorRequest is the body for POST .../conductors.
type InviteConductorRequest struct {
	CPF   string `json:"cpf" binding:"required"`
	Name  string `json:"name"`
	Email string `json:"email" binding:"required,email"`
}

// RespondInvitationRequest is the body for PATCH .../vehicle-invitations/{conductor_id}.
type RespondInvitationRequest struct {
	Status ConductorStatus `json:"status" binding:"required"`
}

// Validate ensures only pending → accepted|rejected transitions are requested.
func (r *RespondInvitationRequest) Validate() error {
	if r.Status != ConductorStatusAccepted && r.Status != ConductorStatusRejected {
		return fmt.Errorf("status must be accepted or rejected")
	}
	return nil
}

// VehicleInvitationSummary is vehicle info embedded in pending-invitation cards.
type VehicleInvitationSummary struct {
	DisplayName     string      `json:"display_name"`
	BrandLabel      string      `json:"brand_label"`
	ModelLabel      string      `json:"model_label"`
	VehicleType     VehicleType `json:"vehicle_type"`
	Color           string      `json:"color"`
	VehiclePhotoURL string      `json:"vehicle_photo_url"`
}

// VehicleInvitationItem is one pending invitation for GET .../vehicle-invitations.
type VehicleInvitationItem struct {
	ID           string                   `json:"id"`
	VehicleID    string                   `json:"vehicle_id"`
	Status       ConductorStatus          `json:"status"`
	InvitedByCPF string                   `json:"invited_by_cpf"`
	OwnerName    string                   `json:"owner_name"`
	Vehicle      VehicleInvitationSummary `json:"vehicle"`
	CreatedAt    time.Time                `json:"created_at"`
}

// VehicleInvitationsResponse wraps pending invitations.
type VehicleInvitationsResponse struct {
	Data []VehicleInvitationItem `json:"data"`
}

// ConductorsListResponse wraps conductors for a vehicle (owner-only).
type ConductorsListResponse struct {
	Data []VehicleConductor `json:"data"`
}
