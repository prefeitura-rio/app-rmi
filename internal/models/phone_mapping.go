package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PhoneCPFMapping represents the mapping between a phone number and a CPF
type PhoneCPFMapping struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	PhoneNumber       string            `bson:"phone_number" json:"phone_number"`
	CPF               string            `bson:"cpf" json:"cpf"`
	Status            string            `bson:"status" json:"status"` // active, blocked, pending
	IsSelfDeclared    bool              `bson:"is_self_declared" json:"is_self_declared"`
	Channel           string            `bson:"channel" json:"channel"`
	ValidationAttempts []ValidationAttempt `bson:"validation_attempts" json:"validation_attempts"`
	CreatedAt         time.Time         `bson:"created_at" json:"created_at"`
	UpdatedAt         time.Time         `bson:"updated_at" json:"updated_at"`
}

// ValidationAttempt represents a single validation attempt
type ValidationAttempt struct {
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
	Valid     bool      `bson:"valid" json:"valid"`
	Channel   string    `bson:"channel" json:"channel"`
}

// PhoneMappingStatus constants
const (
	PhoneMappingStatusActive   = "active"
	PhoneMappingStatusBlocked  = "blocked"
	PhoneMappingStatusPending  = "pending"
)

// Channel constants
const (
	ChannelWhatsApp = "whatsapp"
	ChannelWeb      = "web"
	ChannelMobile   = "mobile"
) 