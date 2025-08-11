package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// OptInHistory represents the history of opt-in and opt-out actions
type OptInHistory struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	PhoneNumber      string             `bson:"phone_number" json:"phone_number"`
	CPF              string             `bson:"cpf" json:"cpf"`
	Action           string             `bson:"action" json:"action"` // opt_in, opt_out
	Channel          string             `bson:"channel" json:"channel"`
	Reason           *string            `bson:"reason,omitempty" json:"reason,omitempty"` // only for opt_out
	ValidationResult *ValidationResult  `bson:"validation_result,omitempty" json:"validation_result,omitempty"`
	Timestamp        time.Time          `bson:"timestamp" json:"timestamp"`
}

// ValidationResult represents the result of a registration validation
type ValidationResult struct {
	Valid bool `bson:"valid" json:"valid"`
}

// OptInAction constants
const (
	OptInActionOptIn  = "opt_in"
	OptInActionOptOut = "opt_out"
)

// OptOutReason constants
const (
	OptOutReasonIrrelevantContent = "irrelevant_content"
	OptOutReasonNotFromRio        = "not_from_rio"
	OptOutReasonIncorrectPerson   = "incorrect_person"
	OptOutReasonTooManyMessages   = "too_many_messages"
)
