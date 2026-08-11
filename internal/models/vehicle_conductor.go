package models

import (
	"fmt"
	"net/mail"
	"strings"
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

// InvitationResponseStatus is the restricted body enum for PATCH invitations (accepted|rejected only).
type InvitationResponseStatus string

const (
	InvitationResponseAccepted InvitationResponseStatus = "accepted"
	InvitationResponseRejected InvitationResponseStatus = "rejected"
)

// InviteEmailStatus tracks invite notification delivery on the conductor link (handoff).
type InviteEmailStatus string

const (
	InviteEmailStatusPending InviteEmailStatus = "pending"
	InviteEmailStatusQueued  InviteEmailStatus = "queued"
	InviteEmailStatusSent    InviteEmailStatus = "sent"
	InviteEmailStatusFailed  InviteEmailStatus = "failed"
	// InviteEmailStatusSkipped means no outbound provider was configured (logging-only); not a delivery success.
	InviteEmailStatusSkipped InviteEmailStatus = "skipped"
)

// VehicleConductor is the invite/active-conductor link for a vehicle.
// Pending responses expose invite snapshot fields (name/email/phone from the invite form).
// Accepted responses enrich name/email/phone live from the invitee's RMI profile.
type VehicleConductor struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	VehicleID    primitive.ObjectID `bson:"vehicle_id" json:"vehicle_id"`
	ConductorCPF string             `bson:"conductor_cpf" json:"conductor_cpf"`

	// Invite snapshot (persisted). For accepted links, GET overlays live RMI contact on these JSON fields.
	ConductorName string `bson:"conductor_name,omitempty" json:"conductor_name"`
	NotifyEmail   string `bson:"notify_email" json:"notify_email"`
	Phone         string `bson:"phone,omitempty" json:"phone,omitempty"`

	Status       ConductorStatus `bson:"status" json:"status"`
	InvitedByCPF string          `bson:"invited_by_cpf" json:"invited_by_cpf"`

	// Invite email delivery (handoff: register attempt/status on the link).
	EmailStatus      InviteEmailStatus `bson:"email_status" json:"email_status"`
	EmailAttemptedAt *time.Time        `bson:"email_attempted_at,omitempty" json:"email_attempted_at,omitempty"`
	EmailLastError   *string           `bson:"email_last_error,omitempty" json:"-"`

	CreatedAt   time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at" json:"updated_at"`
	RespondedAt *time.Time `bson:"responded_at,omitempty" json:"responded_at"`
}

// InviteConductorRequest is the body for POST .../conductors.
// email is required for messaging (notify_email); name/phone are optional display hints while pending.
type InviteConductorRequest struct {
	// CPF must be exactly 11 digits (no punctuation), matching RMI/JWT form.
	CPF   string `json:"cpf" binding:"required" example:"11144477735"`
	Email string `json:"email" binding:"required"`
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// Validate checks invite body rules.
// CPF must be exactly 11 digits (literal RMI form) — formatted values like
// "111.444.777-35" are rejected so stored conductor_cpf matches JWT/path lookups.
func (r *InviteConductorRequest) Validate() error {
	r.CPF = strings.TrimSpace(r.CPF)
	r.Email = strings.TrimSpace(r.Email)
	r.Name = strings.TrimSpace(r.Name)
	r.Phone = strings.TrimSpace(r.Phone)
	if len(r.CPF) != 11 {
		return fmt.Errorf("cpf must be exactly 11 digits")
	}
	for _, c := range r.CPF {
		if c < '0' || c > '9' {
			return fmt.Errorf("cpf must be exactly 11 digits")
		}
	}
	if r.Email == "" {
		return fmt.Errorf("email is required")
	}
	if _, err := mail.ParseAddress(r.Email); err != nil {
		return fmt.Errorf("email is invalid")
	}
	return nil
}

// RespondInvitationRequest is the body for PATCH .../vehicle-invitations/{conductor_id}.
type RespondInvitationRequest struct {
	// Status must be accepted or rejected (pending/revoked are rejected with 400).
	Status InvitationResponseStatus `json:"status" binding:"required"`
}

// Validate ensures only pending → accepted|rejected transitions are requested.
func (r *RespondInvitationRequest) Validate() error {
	if r.Status != InvitationResponseAccepted && r.Status != InvitationResponseRejected {
		return fmt.Errorf("status must be accepted or rejected")
	}
	return nil
}

// AsConductorStatus maps the restricted response enum to ConductorStatus.
func (r *RespondInvitationRequest) AsConductorStatus() ConductorStatus {
	return ConductorStatus(r.Status)
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
