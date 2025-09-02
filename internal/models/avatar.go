package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Avatar represents a profile picture option in the system
type Avatar struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"`
	URL       string             `bson:"url" json:"url"`
	IsActive  bool               `bson:"is_active" json:"is_active"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// AvatarRequest represents the request payload for creating/updating avatars
type AvatarRequest struct {
	Name string `json:"name" binding:"required" validate:"min=1,max=100"`
	URL  string `json:"url" binding:"required" validate:"url"`
}

// AvatarResponse represents the response format for avatar endpoints
type AvatarResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// AvatarsListResponse represents paginated response for listing avatars
type AvatarsListResponse struct {
	Data       []AvatarResponse `json:"data"`
	Total      int64            `json:"total"`
	Page       int              `json:"page"`
	PerPage    int              `json:"per_page"`
	TotalPages int              `json:"total_pages"`
}

// UserAvatarResponse represents user's avatar information
type UserAvatarResponse struct {
	AvatarID *string         `json:"avatar_id"`
	Avatar   *AvatarResponse `json:"avatar,omitempty"`
}

// UserAvatarRequest represents request to update user's avatar
type UserAvatarRequest struct {
	AvatarID *string `json:"avatar_id"`
}

// ToResponse converts Avatar model to AvatarResponse
func (a *Avatar) ToResponse() AvatarResponse {
	return AvatarResponse{
		ID:        a.ID.Hex(),
		Name:      a.Name,
		URL:       a.URL,
		IsActive:  a.IsActive,
		CreatedAt: a.CreatedAt,
	}
}
