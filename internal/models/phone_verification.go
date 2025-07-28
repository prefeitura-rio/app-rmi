package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PhoneVerification represents a phone verification request
type PhoneVerification struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CPF        string            `bson:"cpf" json:"cpf"`
	Telefone   *Telefone         `bson:"telefone" json:"telefone"`
	PhoneNumber string           `bson:"phone_number" json:"phone_number"`
	Code       string            `bson:"code" json:"code"`
	CreatedAt  time.Time         `bson:"created_at" json:"created_at"`
	ExpiresAt  time.Time         `bson:"expires_at" json:"expires_at"`
}

// PhoneVerificationValidateRequest represents the request body for validating a phone verification
type PhoneVerificationValidateRequest struct {
	Code  string `json:"code" binding:"required"`
	DDI   string `json:"ddi" binding:"required"`
	DDD   string `json:"ddd" binding:"required"`
	Valor string `json:"valor" binding:"required"`
}

// Constants for verification status
const (
	VerificationStatusPending   = "pending"
	VerificationStatusVerified  = "verified"
	VerificationStatusExpired   = "expired"
	VerificationStatusFailed    = "failed"
)

// Constants for verification configuration
const (
	VerificationCodeLength    = 6
	MaxVerificationAttempts   = 3
) 