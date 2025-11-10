package models

import "time"

// NotificationCategory represents a notification category for opt-in/opt-out
type NotificationCategory struct {
	ID           string    `bson:"_id" json:"id"`
	Name         string    `bson:"name" json:"name"`
	Description  string    `bson:"description" json:"description"`
	DefaultOptIn bool      `bson:"default_opt_in" json:"default_opt_in"`
	Active       bool      `bson:"active" json:"active"`
	Order        int       `bson:"order" json:"order"`
	CreatedAt    time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at" json:"updated_at"`
}

// NotificationCategoriesResponse represents the response for listing categories
type NotificationCategoriesResponse struct {
	Categories []NotificationCategory `json:"categories"`
}

// CreateNotificationCategoryRequest represents the request to create a category
type CreateNotificationCategoryRequest struct {
	ID           string `json:"id" binding:"required"`
	Name         string `json:"name" binding:"required"`
	Description  string `json:"description" binding:"required"`
	DefaultOptIn bool   `json:"default_opt_in"`
	Active       bool   `json:"active"`
	Order        int    `json:"order"`
}

// UpdateNotificationCategoryRequest represents the request to update a category
type UpdateNotificationCategoryRequest struct {
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	DefaultOptIn *bool   `json:"default_opt_in,omitempty"`
	Active       *bool   `json:"active,omitempty"`
	Order        *int    `json:"order,omitempty"`
}

// NotificationPreferencesResponse represents notification preferences for a CPF
type NotificationPreferencesResponse struct {
	CPF            string          `json:"cpf"`
	OptIn          bool            `json:"opt_in"`
	CategoryOptIns map[string]bool `json:"category_opt_ins"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// PhoneNotificationPreferencesResponse represents notification preferences for a phone
type PhoneNotificationPreferencesResponse struct {
	PhoneNumber    string          `json:"phone_number"`
	OptIn          bool            `json:"opt_in"`
	CategoryOptIns map[string]bool `json:"category_opt_ins"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// UpdateNotificationPreferencesRequest represents the request to update preferences
type UpdateNotificationPreferencesRequest struct {
	OptIn          *bool           `json:"opt_in,omitempty"`
	CategoryOptIns map[string]bool `json:"category_opt_ins,omitempty"`
	Channel        string          `json:"channel" binding:"required"`
	Reason         *string         `json:"reason,omitempty"`
}

// UpdateCategoryPreferenceRequest represents the request to update a single category
// Note: opt_in field uses bool (not *bool) so false values work correctly in JSON
type UpdateCategoryPreferenceRequest struct {
	OptIn   bool    `json:"opt_in"`
	Channel string  `json:"channel" binding:"required"`
	Reason  *string `json:"reason,omitempty"`
}
