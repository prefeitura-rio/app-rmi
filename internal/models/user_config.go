package models

import "time"

// UserConfig represents user configuration and preferences
type UserConfig struct {
	CPF            string          `bson:"cpf" json:"cpf"`
	FirstLogin     bool            `bson:"first_login" json:"first_login"`
	OptIn          bool            `bson:"opt_in" json:"opt_in"`
	CategoryOptIns map[string]bool `bson:"category_opt_ins,omitempty" json:"category_opt_ins,omitempty"`
	AvatarID       *string         `bson:"avatar_id,omitempty" json:"avatar_id,omitempty"`
	Version        int32           `bson:"version,omitempty" json:"version,omitempty"`
	UpdatedAt      time.Time       `bson:"updated_at" json:"updated_at"`
}

// UserConfigResponse represents the response format for user config endpoints
type UserConfigResponse struct {
	FirstLogin bool `json:"firstlogin"`
}

// UserConfigOptInResponse represents the response format for opt-in endpoints
type UserConfigOptInResponse struct {
	OptIn          *bool           `json:"opt_in" binding:"required"`
	CategoryOptIns map[string]bool `json:"category_opt_ins,omitempty"`
}
