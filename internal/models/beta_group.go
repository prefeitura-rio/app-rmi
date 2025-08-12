package models

import (
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BetaGroup represents a closed beta group for analytics purposes
type BetaGroup struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name      string             `bson:"name" json:"name"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// BetaGroupRequest represents the request body for creating/updating a beta group
type BetaGroupRequest struct {
	Name string `json:"name" binding:"required"`
}

// BetaGroupResponse represents the response for beta group operations
type BetaGroupResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BetaGroupListResponse represents the paginated response for listing beta groups
type BetaGroupListResponse struct {
	Groups      []BetaGroupResponse `json:"groups"`
	Pagination  PaginationInfo      `json:"pagination"`
	TotalGroups int64               `json:"total_groups"`
}

// BetaWhitelistRequest represents the request body for adding a phone to beta whitelist
type BetaWhitelistRequest struct {
	GroupID string `json:"group_id" binding:"required"`
}

// BetaWhitelistBulkRequest represents the request body for bulk operations
type BetaWhitelistBulkRequest struct {
	PhoneNumbers []string `json:"phone_numbers" binding:"required"`
	GroupID      string   `json:"group_id" binding:"required"`
}

// BetaWhitelistBulkRemoveRequest represents the request body for bulk remove operations
type BetaWhitelistBulkRemoveRequest struct {
	PhoneNumbers []string `json:"phone_numbers" binding:"required"`
}

// BetaWhitelistMoveRequest represents the request body for moving phones between groups
type BetaWhitelistMoveRequest struct {
	PhoneNumbers []string `json:"phone_numbers" binding:"required"`
	FromGroupID  string   `json:"from_group_id" binding:"required"`
	ToGroupID    string   `json:"to_group_id" binding:"required"`
}

// BetaWhitelistResponse represents a whitelisted phone entry
type BetaWhitelistResponse struct {
	PhoneNumber string    `json:"phone_number"`
	GroupID     string    `json:"group_id"`
	GroupName   string    `json:"group_name"`
	AddedAt     time.Time `json:"added_at"`
}

// BetaWhitelistListResponse represents the paginated response for listing whitelisted phones
type BetaWhitelistListResponse struct {
	Whitelisted []BetaWhitelistResponse `json:"whitelisted"`
	Pagination  PaginationInfo          `json:"pagination"`
	TotalCount  int64                   `json:"total_count"`
}

// BetaStatusResponse represents the response for beta status check
type BetaStatusResponse struct {
	PhoneNumber     string `json:"phone_number"`
	BetaWhitelisted bool   `json:"beta_whitelisted"`
	GroupID         string `json:"group_id,omitempty"`
	GroupName       string `json:"group_name,omitempty"`
}

// GetNormalizedName returns the normalized (lowercase) name for uniqueness checks
func (bg *BetaGroup) GetNormalizedName() string {
	return strings.ToLower(strings.TrimSpace(bg.Name))
}

// ValidateName checks if the group name is valid
func (bg *BetaGroup) ValidateName() error {
	name := strings.TrimSpace(bg.Name)
	if name == "" {
		return ErrInvalidGroupName
	}
	if len(name) > 100 {
		return ErrGroupNameTooLong
	}
	return nil
}

// BeforeCreate sets the creation and update timestamps
func (bg *BetaGroup) BeforeCreate() {
	now := time.Now()
	bg.CreatedAt = now
	bg.UpdatedAt = now
}

// BeforeUpdate sets the update timestamp
func (bg *BetaGroup) BeforeUpdate() {
	bg.UpdatedAt = time.Now()
}
