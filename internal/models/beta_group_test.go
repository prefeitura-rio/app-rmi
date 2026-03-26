package models

import (
	"strings"
	"testing"
	"time"
)

func TestBetaGroup_GetNormalizedName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "test_group", "test_group"},
		{"uppercase", "TEST_GROUP", "test_group"},
		{"mixed case", "Test_Group", "test_group"},
		{"with spaces", "  Test Group  ", "test group"},
		{"with leading spaces", "   test", "test"},
		{"with trailing spaces", "test   ", "test"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bg := &BetaGroup{Name: tt.input}
			result := bg.GetNormalizedName()
			if result != tt.expected {
				t.Errorf("GetNormalizedName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBetaGroup_ValidateName(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		wantErr   bool
		errType   error
	}{
		{"valid name", "valid_group", false, nil},
		{"valid with spaces", "  valid group  ", false, nil},
		{"empty name", "", true, ErrInvalidGroupName},
		{"only spaces", "   ", true, ErrInvalidGroupName},
		{"name too long", strings.Repeat("a", 101), true, ErrGroupNameTooLong},
		{"name exactly 100 chars", strings.Repeat("a", 100), false, nil},
		{"name 99 chars", strings.Repeat("a", 99), false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bg := &BetaGroup{Name: tt.groupName}
			err := bg.ValidateName()

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateName() error = nil, want error")
					return
				}
				if tt.errType != nil && err != tt.errType {
					t.Errorf("ValidateName() error = %v, want %v", err, tt.errType)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateName() error = %v, want nil", err)
				}
			}
		})
	}
}

func TestBetaGroup_BeforeCreate(t *testing.T) {
	bg := &BetaGroup{
		Name: "test_group",
	}

	// Initially, timestamps should be zero
	if !bg.CreatedAt.IsZero() {
		t.Errorf("CreatedAt should be zero before BeforeCreate()")
	}
	if !bg.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt should be zero before BeforeCreate()")
	}

	// Call BeforeCreate
	before := time.Now()
	bg.BeforeCreate()
	after := time.Now()

	// Check that timestamps were set
	if bg.CreatedAt.IsZero() {
		t.Errorf("CreatedAt should not be zero after BeforeCreate()")
	}
	if bg.UpdatedAt.IsZero() {
		t.Errorf("UpdatedAt should not be zero after BeforeCreate()")
	}

	// Check that both timestamps are equal
	if !bg.CreatedAt.Equal(bg.UpdatedAt) {
		t.Errorf("CreatedAt and UpdatedAt should be equal after BeforeCreate()")
	}

	// Check that timestamps are within the expected range
	if bg.CreatedAt.Before(before) || bg.CreatedAt.After(after) {
		t.Errorf("CreatedAt = %v, should be between %v and %v", bg.CreatedAt, before, after)
	}
}

func TestBetaGroup_BeforeUpdate(t *testing.T) {
	bg := &BetaGroup{
		Name:      "test_group",
		CreatedAt: time.Now().Add(-1 * time.Hour), // Created 1 hour ago
		UpdatedAt: time.Now().Add(-1 * time.Hour), // Updated 1 hour ago
	}

	originalCreatedAt := bg.CreatedAt
	originalUpdatedAt := bg.UpdatedAt

	// Wait a bit to ensure time difference
	time.Sleep(1 * time.Millisecond)

	// Call BeforeUpdate
	before := time.Now()
	bg.BeforeUpdate()
	after := time.Now()

	// Check that CreatedAt didn't change
	if !bg.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("CreatedAt should not change after BeforeUpdate()")
	}

	// Check that UpdatedAt changed
	if bg.UpdatedAt.Equal(originalUpdatedAt) {
		t.Errorf("UpdatedAt should change after BeforeUpdate()")
	}

	// Check that UpdatedAt is within the expected range
	if bg.UpdatedAt.Before(before) || bg.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt = %v, should be between %v and %v", bg.UpdatedAt, before, after)
	}

	// Check that UpdatedAt is after CreatedAt
	if !bg.UpdatedAt.After(bg.CreatedAt) {
		t.Errorf("UpdatedAt should be after CreatedAt")
	}
}

func TestBetaGroup_BeforeCreate_MultipleCalls(t *testing.T) {
	bg := &BetaGroup{Name: "test_group"}

	bg.BeforeCreate()
	firstCreatedAt := bg.CreatedAt
	firstUpdatedAt := bg.UpdatedAt

	time.Sleep(1 * time.Millisecond)

	bg.BeforeCreate()
	secondCreatedAt := bg.CreatedAt
	secondUpdatedAt := bg.UpdatedAt

	// Second call should update both timestamps
	if secondCreatedAt.Equal(firstCreatedAt) {
		t.Errorf("CreatedAt should change on second BeforeCreate() call")
	}

	if secondUpdatedAt.Equal(firstUpdatedAt) {
		t.Errorf("UpdatedAt should change on second BeforeCreate() call")
	}
}

func TestBetaGroupRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request BetaGroupRequest
		valid   bool
	}{
		{
			name: "valid request",
			request: BetaGroupRequest{
				Name: "valid_group",
			},
			valid: true,
		},
		{
			name: "empty name",
			request: BetaGroupRequest{
				Name: "",
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
		})
	}
}

func TestBetaWhitelistRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request BetaWhitelistRequest
		valid   bool
	}{
		{
			name: "valid group ID",
			request: BetaWhitelistRequest{
				GroupID: "group123",
			},
			valid: true,
		},
		{
			name:    "empty request",
			request: BetaWhitelistRequest{},
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify fields are set correctly
			if tt.request.GroupID == "" && tt.valid {
				t.Errorf("Expected invalid request with empty GroupID")
			}
		})
	}
}

func TestBetaGroupListResponse(t *testing.T) {
	response := BetaGroupListResponse{
		Groups:      []BetaGroupResponse{},
		TotalGroups: 0,
	}

	if len(response.Groups) != 0 {
		t.Errorf("BetaGroupListResponse Groups length = %v, want 0", len(response.Groups))
	}

	if response.TotalGroups != 0 {
		t.Errorf("BetaGroupListResponse TotalGroups = %v, want 0", response.TotalGroups)
	}
}

func TestBetaGroupListResponse_WithData(t *testing.T) {
	groups := []BetaGroupResponse{
		{
			ID:        "id1",
			Name:      "group1",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			ID:        "id2",
			Name:      "group2",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	response := BetaGroupListResponse{
		Groups:      groups,
		TotalGroups: 2,
	}

	if len(response.Groups) != 2 {
		t.Errorf("BetaGroupListResponse Groups length = %v, want 2", len(response.Groups))
	}

	if response.TotalGroups != 2 {
		t.Errorf("BetaGroupListResponse TotalGroups = %v, want 2", response.TotalGroups)
	}

	if response.Groups[0].Name != "group1" {
		t.Errorf("BetaGroupListResponse Groups[0].Name = %v, want group1", response.Groups[0].Name)
	}
}
