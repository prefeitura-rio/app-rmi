package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// setupBetaGroupTest initializes MongoDB and Redis for beta group service tests
func setupBetaGroupTest(t *testing.T) (*BetaGroupService, func()) {
	// Use shared MongoDB connection from common_test.go
	if config.MongoDB == nil {
		t.Fatal("MongoDB not initialized - ensure TestMain has run")
	}

	// Initialize logging
	logging.InitLogger()

	ctx := context.Background()

	// Save original collection names
	originalBetaGroupCollection := config.AppConfig.BetaGroupCollection
	originalPhoneMappingCollection := config.AppConfig.PhoneMappingCollection

	// Set test collection names
	config.AppConfig.BetaGroupCollection = "test_beta_groups"
	config.AppConfig.PhoneMappingCollection = "test_phone_mappings"

	// Create service with nil client (uses config.MongoDB)
	service := NewBetaGroupService(logging.Logger)

	// Return cleanup function
	return service, func() {
		// Clean up Redis keys
		keys, _ := config.Redis.Keys(ctx, "beta:*").Result()
		if len(keys) > 0 {
			config.Redis.Del(ctx, keys...)
		}

		// Drop only test collections, not entire database
		config.MongoDB.Collection(config.AppConfig.BetaGroupCollection).Drop(ctx)
		config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).Drop(ctx)

		// Restore original collection names
		config.AppConfig.BetaGroupCollection = originalBetaGroupCollection
		config.AppConfig.PhoneMappingCollection = originalPhoneMappingCollection
	}
}

func TestNewBetaGroupService(t *testing.T) {
	// Use shared MongoDB connection from common_test.go
	if config.MongoDB == nil {
		t.Fatal("MongoDB not initialized - ensure TestMain has run")
	}

	logging.InitLogger()
	service := NewBetaGroupService(logging.Logger)
	if service == nil {
		t.Error("NewBetaGroupService() returned nil")
	}
	if service.logger == nil {
		t.Error("service.logger is nil")
	}
}

func TestCreateGroup_Success(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	response, err := service.CreateGroup(ctx, "Test Group 1")
	if err != nil {
		t.Errorf("CreateGroup() error = %v, want nil", err)
	}
	if response == nil {
		t.Fatal("CreateGroup() returned nil response")
	}
	if response.ID == "" {
		t.Error("CreateGroup() ID is empty")
	}
	if response.Name != "Test Group 1" {
		t.Errorf("CreateGroup() Name = %s, want 'Test Group 1'", response.Name)
	}
	if response.CreatedAt.IsZero() {
		t.Error("CreateGroup() CreatedAt is zero")
	}
	if response.UpdatedAt.IsZero() {
		t.Error("CreateGroup() UpdatedAt is zero")
	}
}

func TestCreateGroup_DuplicateName(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create first group
	_, err := service.CreateGroup(ctx, "Duplicate Group")
	if err != nil {
		t.Fatalf("First CreateGroup() error = %v, want nil", err)
	}

	// Try to create group with same name
	_, err = service.CreateGroup(ctx, "Duplicate Group")
	if err != models.ErrGroupNameExists {
		t.Errorf("CreateGroup() error = %v, want ErrGroupNameExists", err)
	}
}

func TestCreateGroup_CaseInsensitiveDuplicate(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create first group
	_, err := service.CreateGroup(ctx, "Test Group")
	if err != nil {
		t.Fatalf("First CreateGroup() error = %v, want nil", err)
	}

	// Try to create group with different case
	_, err = service.CreateGroup(ctx, "TEST GROUP")
	if err != models.ErrGroupNameExists {
		t.Errorf("CreateGroup() error = %v, want ErrGroupNameExists for case-insensitive duplicate", err)
	}
}

func TestCreateGroup_InvalidName(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	tests := []struct {
		name      string
		groupName string
		wantErr   error
	}{
		{
			name:      "Empty name",
			groupName: "",
			wantErr:   models.ErrInvalidGroupName,
		},
		{
			name:      "Whitespace only",
			groupName: "   ",
			wantErr:   models.ErrInvalidGroupName,
		},
		{
			name:      "Name too long",
			groupName: "This is a very long beta group name that exceeds the maximum allowed length of 100 characters for the name field",
			wantErr:   models.ErrGroupNameTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.CreateGroup(ctx, tt.groupName)
			if err != tt.wantErr {
				t.Errorf("CreateGroup() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetGroup_Success(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group
	created, err := service.CreateGroup(ctx, "Get Test Group")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	// Get the group
	retrieved, err := service.GetGroup(ctx, created.ID)
	if err != nil {
		t.Errorf("GetGroup() error = %v, want nil", err)
	}
	if retrieved.ID != created.ID {
		t.Errorf("GetGroup() ID = %s, want %s", retrieved.ID, created.ID)
	}
	if retrieved.Name != "Get Test Group" {
		t.Errorf("GetGroup() Name = %s, want 'Get Test Group'", retrieved.Name)
	}
}

func TestGetGroup_NotFound(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Try to get non-existent group
	nonExistentID := primitive.NewObjectID().Hex()
	_, err := service.GetGroup(ctx, nonExistentID)
	if err != models.ErrGroupNotFound {
		t.Errorf("GetGroup() error = %v, want ErrGroupNotFound", err)
	}
}

func TestGetGroup_InvalidID(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	_, err := service.GetGroup(ctx, "invalid-id")
	if err != models.ErrInvalidGroupID {
		t.Errorf("GetGroup() error = %v, want ErrInvalidGroupID", err)
	}
}

func TestListGroups(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple groups
	for i := 1; i <= 5; i++ {
		_, err := service.CreateGroup(ctx, fmt.Sprintf("List Group %d", i))
		if err != nil {
			t.Fatalf("CreateGroup() error = %v", err)
		}
	}

	// List first page
	response, err := service.ListGroups(ctx, 1, 3)
	if err != nil {
		t.Errorf("ListGroups() error = %v, want nil", err)
	}
	if len(response.Groups) != 3 {
		t.Errorf("ListGroups() returned %d groups, want 3", len(response.Groups))
	}
	if response.TotalGroups != 5 {
		t.Errorf("ListGroups() TotalGroups = %d, want 5", response.TotalGroups)
	}
	if response.Pagination.Page != 1 {
		t.Errorf("ListGroups() Page = %d, want 1", response.Pagination.Page)
	}
	if response.Pagination.PerPage != 3 {
		t.Errorf("ListGroups() PerPage = %d, want 3", response.Pagination.PerPage)
	}

	// List second page
	response, err = service.ListGroups(ctx, 2, 3)
	if err != nil {
		t.Errorf("ListGroups() error = %v, want nil", err)
	}
	if len(response.Groups) != 2 {
		t.Errorf("ListGroups() page 2 returned %d groups, want 2", len(response.Groups))
	}
}

func TestUpdateGroup_Success(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group
	created, err := service.CreateGroup(ctx, "Original Name")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	// Update the group
	updated, err := service.UpdateGroup(ctx, created.ID, "Updated Name")
	if err != nil {
		t.Errorf("UpdateGroup() error = %v, want nil", err)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("UpdateGroup() Name = %s, want 'Updated Name'", updated.Name)
	}
	if updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Error("UpdateGroup() UpdatedAt was not updated")
	}
}

func TestUpdateGroup_DuplicateName(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create two groups
	_, _ = service.CreateGroup(ctx, "Group 1")
	group2, _ := service.CreateGroup(ctx, "Group 2")

	// Try to update group2 to have the same name as group1
	_, err := service.UpdateGroup(ctx, group2.ID, "Group 1")
	if err != models.ErrGroupNameExists {
		t.Errorf("UpdateGroup() error = %v, want ErrGroupNameExists", err)
	}
}

func TestUpdateGroup_NotFound(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	nonExistentID := primitive.NewObjectID().Hex()
	_, err := service.UpdateGroup(ctx, nonExistentID, "New Name")
	if err != models.ErrGroupNotFound {
		t.Errorf("UpdateGroup() error = %v, want ErrGroupNotFound", err)
	}
}

func TestDeleteGroup_Success(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group
	created, err := service.CreateGroup(ctx, "To Delete")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	// Delete the group
	err = service.DeleteGroup(ctx, created.ID)
	if err != nil {
		t.Errorf("DeleteGroup() error = %v, want nil", err)
	}

	// Verify it's deleted
	_, err = service.GetGroup(ctx, created.ID)
	if err != models.ErrGroupNotFound {
		t.Errorf("After delete, GetGroup() error = %v, want ErrGroupNotFound", err)
	}
}

func TestDeleteGroup_WithMembers(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group
	created, err := service.CreateGroup(ctx, "Group With Members")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	// Add a phone to the group's whitelist
	phoneMappingCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	_, err = phoneMappingCollection.InsertOne(ctx, bson.M{
		"phone_number":  "5521987654321",
		"beta_group_id": created.ID,
	})
	if err != nil {
		t.Fatalf("Failed to insert phone mapping: %v", err)
	}

	// Delete should succeed and clean up phone associations automatically
	err = service.DeleteGroup(ctx, created.ID)
	if err != nil {
		t.Errorf("DeleteGroup() error = %v, want nil (should auto-cleanup members)", err)
	}

	// Verify group was deleted
	_, err = service.GetGroup(ctx, created.ID)
	if err != models.ErrGroupNotFound {
		t.Errorf("GetGroup() after delete error = %v, want ErrGroupNotFound", err)
	}

	// Verify phone association was removed
	var phoneMapping bson.M
	err = phoneMappingCollection.FindOne(ctx, bson.M{"phone_number": "5521987654321"}).Decode(&phoneMapping)
	if err != nil {
		t.Errorf("Failed to find phone mapping: %v", err)
	}
	// beta_group_id should not exist after cleanup
	if _, exists := phoneMapping["beta_group_id"]; exists {
		t.Error("Phone mapping should not have beta_group_id after group deletion")
	}
}

func TestDeleteGroup_NotFound(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	nonExistentID := primitive.NewObjectID().Hex()
	err := service.DeleteGroup(ctx, nonExistentID)
	if err != models.ErrGroupNotFound {
		t.Errorf("DeleteGroup() error = %v, want ErrGroupNotFound", err)
	}
}

func TestAddToWhitelist_Success(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group
	group, err := service.CreateGroup(ctx, "Whitelist Test Group")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	// Add phone to whitelist
	_, err = service.AddToWhitelist(ctx, "+5521987654321", group.ID)
	if err != nil {
		t.Errorf("AddToWhitelist() error = %v, want nil", err)
	}

	// Verify phone was added
	phoneMappingCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	var mapping bson.M
	err = phoneMappingCollection.FindOne(ctx, bson.M{"phone_number": "5521987654321"}).Decode(&mapping)
	if err != nil {
		t.Errorf("Failed to find phone mapping: %v", err)
	}
	if mapping["beta_group_id"] != group.ID {
		t.Errorf("beta_group_id = %v, want %v", mapping["beta_group_id"], group.ID)
	}
}

func TestAddToWhitelist_AlreadyWhitelisted(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group
	group, err := service.CreateGroup(ctx, "Whitelist Test 2")
	if err != nil {
		t.Fatalf("CreateGroup() error = %v", err)
	}

	// Add phone to whitelist
	_, err = service.AddToWhitelist(ctx, "+5521987654322", group.ID)
	if err != nil {
		t.Fatalf("First AddToWhitelist() error = %v", err)
	}

	// Try to add again
	_, err = service.AddToWhitelist(ctx, "+5521987654322", group.ID)
	if err != models.ErrPhoneAlreadyWhitelisted {
		t.Errorf("AddToWhitelist() error = %v, want ErrPhoneAlreadyWhitelisted", err)
	}
}

func TestRemoveFromWhitelist_Success(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group and add a phone
	group, _ := service.CreateGroup(ctx, "Remove Test")
	service.AddToWhitelist(ctx, "+5521987654323", group.ID)

	// Remove phone from whitelist
	err := service.RemoveFromWhitelist(ctx, "+5521987654323")
	if err != nil {
		t.Errorf("RemoveFromWhitelist() error = %v, want nil", err)
	}

	// Verify phone was removed
	phoneMappingCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	var mapping bson.M
	err = phoneMappingCollection.FindOne(ctx, bson.M{"phone_number": "5521987654323"}).Decode(&mapping)
	if err == nil {
		if mapping["beta_group_id"] == group.ID {
			t.Error("Phone still has beta_group_id after removal")
		}
	}
}

func TestRemoveFromWhitelist_NotWhitelisted(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group
	_, _ = service.CreateGroup(ctx, "Remove Test 2")

	// Try to remove phone that's not whitelisted
	err := service.RemoveFromWhitelist(ctx, "+5521987654324")
	if err != models.ErrPhoneNotWhitelisted {
		t.Errorf("RemoveFromWhitelist() error = %v, want ErrPhoneNotWhitelisted", err)
	}
}

func TestListWhitelistedPhones(t *testing.T) {
	service, cleanup := setupBetaGroupTest(t)
	defer cleanup()

	ctx := context.Background()

	// Create a group and add multiple phones
	group, _ := service.CreateGroup(ctx, "Whitelist List Test")
	service.AddToWhitelist(ctx, "+5521987651111", group.ID)
	service.AddToWhitelist(ctx, "+5521987652222", group.ID)
	service.AddToWhitelist(ctx, "+5521987653333", group.ID)

	// List whitelisted phones
	phones, err := service.ListWhitelistedPhones(ctx, 1, 10, group.ID)
	if err != nil {
		t.Errorf("ListWhitelistedPhones() error = %v, want nil", err)
	}
	if len(phones.Whitelisted) != 3 {
		t.Errorf("ListWhitelistedPhones() returned %d phones, want 3", len(phones.Whitelisted))
	}
	if phones.TotalCount != 3 {
		t.Errorf("ListWhitelistedPhones() TotalCount = %d, want 3", phones.TotalCount)
	}
}
