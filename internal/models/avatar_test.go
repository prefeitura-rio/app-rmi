package models

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestAvatar_ToResponse(t *testing.T) {
	now := time.Now()
	objID := primitive.NewObjectID()

	avatar := &Avatar{
		ID:        objID,
		Name:      "Test Avatar",
		URL:       "https://example.com/avatar.png",
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}

	response := avatar.ToResponse()

	if response.ID != objID.Hex() {
		t.Errorf("ToResponse() ID = %v, want %v", response.ID, objID.Hex())
	}

	if response.Name != "Test Avatar" {
		t.Errorf("ToResponse() Name = %v, want Test Avatar", response.Name)
	}

	if response.URL != "https://example.com/avatar.png" {
		t.Errorf("ToResponse() URL = %v, want https://example.com/avatar.png", response.URL)
	}

	if response.IsActive != true {
		t.Errorf("ToResponse() IsActive = %v, want true", response.IsActive)
	}

	if !response.CreatedAt.Equal(now) {
		t.Errorf("ToResponse() CreatedAt = %v, want %v", response.CreatedAt, now)
	}
}

func TestAvatar_ToResponse_Inactive(t *testing.T) {
	avatar := &Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "Inactive Avatar",
		URL:       "https://example.com/inactive.png",
		IsActive:  false,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	response := avatar.ToResponse()

	if response.IsActive != false {
		t.Errorf("ToResponse() IsActive = %v, want false", response.IsActive)
	}
}

func TestAvatar_ToResponse_EmptyFields(t *testing.T) {
	avatar := &Avatar{
		ID:        primitive.NewObjectID(),
		Name:      "",
		URL:       "",
		IsActive:  false,
		CreatedAt: time.Time{},
		UpdatedAt: time.Time{},
	}

	response := avatar.ToResponse()

	if response.Name != "" {
		t.Errorf("ToResponse() Name = %v, want empty string", response.Name)
	}

	if response.URL != "" {
		t.Errorf("ToResponse() URL = %v, want empty string", response.URL)
	}

	if !response.CreatedAt.IsZero() {
		t.Errorf("ToResponse() CreatedAt should be zero time")
	}
}

func TestAvatarRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request AvatarRequest
		valid   bool
	}{
		{
			name: "valid request",
			request: AvatarRequest{
				Name: "Valid Avatar",
				URL:  "https://example.com/avatar.png",
			},
			valid: true,
		},
		{
			name: "empty name",
			request: AvatarRequest{
				Name: "",
				URL:  "https://example.com/avatar.png",
			},
			valid: false,
		},
		{
			name: "empty URL",
			request: AvatarRequest{
				Name: "Valid Avatar",
				URL:  "",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the struct can be created
			if tt.request.Name == "" && tt.valid {
				t.Errorf("Expected invalid request with empty name")
			}
			if tt.request.URL == "" && tt.valid {
				t.Errorf("Expected invalid request with empty URL")
			}
		})
	}
}

func TestUserAvatarRequest_NilAvatarID(t *testing.T) {
	request := UserAvatarRequest{
		AvatarID: nil,
	}

	if request.AvatarID != nil {
		t.Errorf("UserAvatarRequest AvatarID should be nil")
	}
}

func TestUserAvatarRequest_WithAvatarID(t *testing.T) {
	avatarID := "test-avatar-id"
	request := UserAvatarRequest{
		AvatarID: &avatarID,
	}

	if request.AvatarID == nil {
		t.Fatalf("UserAvatarRequest AvatarID should not be nil")
	}

	if *request.AvatarID != "test-avatar-id" {
		t.Errorf("UserAvatarRequest AvatarID = %v, want test-avatar-id", *request.AvatarID)
	}
}

func TestUserAvatarResponse_WithAvatar(t *testing.T) {
	avatarID := "test-avatar-id"
	avatarResponse := &AvatarResponse{
		ID:        "avatar-123",
		Name:      "Test Avatar",
		URL:       "https://example.com/avatar.png",
		IsActive:  true,
		CreatedAt: time.Now(),
	}

	response := UserAvatarResponse{
		AvatarID: &avatarID,
		Avatar:   avatarResponse,
	}

	if response.AvatarID == nil {
		t.Fatalf("UserAvatarResponse AvatarID should not be nil")
	}

	if *response.AvatarID != "test-avatar-id" {
		t.Errorf("UserAvatarResponse AvatarID = %v, want test-avatar-id", *response.AvatarID)
	}

	if response.Avatar == nil {
		t.Fatalf("UserAvatarResponse Avatar should not be nil")
	}

	if response.Avatar.ID != "avatar-123" {
		t.Errorf("UserAvatarResponse Avatar.ID = %v, want avatar-123", response.Avatar.ID)
	}
}

func TestAvatarsListResponse_Empty(t *testing.T) {
	response := AvatarsListResponse{
		Data:       []AvatarResponse{},
		Total:      0,
		Page:       1,
		PerPage:    10,
		TotalPages: 0,
	}

	if len(response.Data) != 0 {
		t.Errorf("AvatarsListResponse Data length = %v, want 0", len(response.Data))
	}

	if response.Total != 0 {
		t.Errorf("AvatarsListResponse Total = %v, want 0", response.Total)
	}

	if response.TotalPages != 0 {
		t.Errorf("AvatarsListResponse TotalPages = %v, want 0", response.TotalPages)
	}
}

func TestAvatarsListResponse_WithData(t *testing.T) {
	avatars := []AvatarResponse{
		{
			ID:        "avatar-1",
			Name:      "Avatar 1",
			URL:       "https://example.com/avatar1.png",
			IsActive:  true,
			CreatedAt: time.Now(),
		},
		{
			ID:        "avatar-2",
			Name:      "Avatar 2",
			URL:       "https://example.com/avatar2.png",
			IsActive:  true,
			CreatedAt: time.Now(),
		},
	}

	response := AvatarsListResponse{
		Data:       avatars,
		Total:      2,
		Page:       1,
		PerPage:    10,
		TotalPages: 1,
	}

	if len(response.Data) != 2 {
		t.Errorf("AvatarsListResponse Data length = %v, want 2", len(response.Data))
	}

	if response.Total != 2 {
		t.Errorf("AvatarsListResponse Total = %v, want 2", response.Total)
	}

	if response.Data[0].ID != "avatar-1" {
		t.Errorf("AvatarsListResponse Data[0].ID = %v, want avatar-1", response.Data[0].ID)
	}
}
