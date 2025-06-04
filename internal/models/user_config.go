package models

import "time"

// UserConfig represents user configuration and preferences
type UserConfig struct {
	CPF        string    `bson:"cpf" json:"cpf"`
	FirstLogin bool      `bson:"first_login" json:"first_login"`
	OptIn      bool      `bson:"opt_in" json:"opt_in"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
}

// UserConfigResponse represents the response format for user config endpoints
type UserConfigResponse struct {
	FirstLogin bool `json:"firstlogin"`
}

// UserConfigOptInResponse represents the response format for opt-in endpoints
type UserConfigOptInResponse struct {
	OptIn bool `json:"optin"`
} 