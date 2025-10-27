package models

import (
	"time"
)

// PhoneCPFMapping represents the mapping between phone numbers and CPFs
type PhoneCPFMapping struct {
	PhoneNumber       string            `bson:"phone_number" json:"phone_number"`
	CPF               string            `bson:"cpf,omitempty" json:"cpf,omitempty"`
	Status            string            `bson:"status" json:"status"`
	QuarantineUntil   *time.Time        `bson:"quarantine_until,omitempty" json:"quarantine_until,omitempty"`
	QuarantineHistory []QuarantineEvent `bson:"quarantine_history,omitempty" json:"quarantine_history,omitempty"`
	ValidationAttempt ValidationAttempt `bson:"validation_attempt,omitempty" json:"validation_attempt,omitempty"`
	Channel           string            `bson:"channel,omitempty" json:"channel,omitempty"`
	BetaGroupID       string            `bson:"beta_group_id,omitempty" json:"beta_group_id,omitempty"`
	CreatedAt         *time.Time        `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt         *time.Time        `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// QuarantineEvent represents a quarantine event in the history
type QuarantineEvent struct {
	QuarantinedAt   time.Time  `bson:"quarantined_at" json:"quarantined_at"`
	QuarantineUntil time.Time  `bson:"quarantine_until" json:"quarantine_until"`
	ReleasedAt      *time.Time `bson:"released_at,omitempty" json:"released_at,omitempty"`
}

// ValidationAttempt represents validation attempt details
type ValidationAttempt struct {
	AttemptedAt time.Time `bson:"attempted_at" json:"attempted_at"`
	Valid       bool      `bson:"valid" json:"valid"`
	Channel     string    `bson:"channel" json:"channel"`
}

// PhoneStatusResponse represents the response for phone status check
type PhoneStatusResponse struct {
	PhoneNumber     string     `json:"phone_number"`
	Found           bool       `json:"found"`
	Quarantined     bool       `json:"quarantined"`
	OptedOut        bool       `json:"opted_out"`
	CPF             string     `json:"cpf,omitempty"`
	Name            string     `json:"name,omitempty"`
	QuarantineUntil *time.Time `json:"quarantine_until,omitempty"`
	BetaWhitelisted bool       `json:"beta_whitelisted"`
	BetaGroupID     string     `json:"beta_group_id,omitempty"`
	BetaGroupName   string     `json:"beta_group_name,omitempty"`
}

// QuarantineRequest represents the request to quarantine a phone number
type QuarantineRequest struct {
	// Empty - no additional data needed for quarantine
}

// QuarantineResponse represents the response for quarantine operations
type QuarantineResponse struct {
	Status          string    `json:"status"`
	PhoneNumber     string    `json:"phone_number"`
	QuarantineUntil time.Time `json:"quarantine_until"`
	Message         string    `json:"message"`
}

// BindRequest represents the request to bind a phone number to a CPF
type BindRequest struct {
	CPF     string `json:"cpf" binding:"required"`
	Channel string `json:"channel" binding:"required"`
}

// BindResponse represents the response for binding operations
type BindResponse struct {
	Status      string `json:"status"`
	PhoneNumber string `json:"phone_number"`
	CPF         string `json:"cpf"`
	OptIn       bool   `json:"opt_in"`
	Message     string `json:"message"`
}

// QuarantinedPhone represents a quarantined phone number for admin endpoints
type QuarantinedPhone struct {
	PhoneNumber     string    `json:"phone_number"`
	CPF             string    `json:"cpf,omitempty"`
	QuarantineUntil time.Time `json:"quarantine_until"`
	Expired         bool      `json:"expired"`
}

// QuarantinedListResponse represents the paginated response for quarantined phones
type QuarantinedListResponse struct {
	Data       []QuarantinedPhone `json:"data"`
	Pagination PaginationInfo     `json:"pagination"`
}

// QuarantineStats represents quarantine statistics
type QuarantineStats struct {
	TotalQuarantined       int `json:"total_quarantined"`
	ExpiredQuarantines     int `json:"expired_quarantines"`
	ActiveQuarantines      int `json:"active_quarantines"`
	QuarantinesWithCPF     int `json:"quarantines_with_cpf"`
	QuarantinesWithoutCPF  int `json:"quarantines_without_cpf"`
	QuarantineHistoryTotal int `json:"quarantine_history_total"`
}

// PaginationInfo represents pagination information
type PaginationInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// Mapping status constants
const (
	MappingStatusActive      = "active"
	MappingStatusBlocked     = "blocked"
	MappingStatusQuarantined = "quarantined"
)

// Channel constants
const (
	ChannelWhatsApp = "whatsapp"
	ChannelWeb      = "web"
	ChannelMobile   = "mobile"
)
