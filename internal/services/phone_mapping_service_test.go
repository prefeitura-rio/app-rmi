package services

import (
	"context"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
)

// setupPhoneMappingTest initializes MongoDB for phone mapping service tests
func setupPhoneMappingTest(t *testing.T) (*PhoneMappingService, func()) {
	// Ensure test environment is initialized
	setupTestEnvironment()

	if config.MongoDB == nil {
		t.Skip("Skipping phone mapping service tests: MongoDB not available")
	}

	ctx := context.Background()

	// Store original collection names to restore later
	originalCitizenCollection := config.AppConfig.CitizenCollection
	originalPhoneMappingCollection := config.AppConfig.PhoneMappingCollection
	originalOptInHistoryCollection := config.AppConfig.OptInHistoryCollection
	originalBetaGroupCollection := config.AppConfig.BetaGroupCollection
	originalPhoneQuarantineTTL := config.AppConfig.PhoneQuarantineTTL

	// Use test collection names
	config.AppConfig.CitizenCollection = "test_citizens"
	config.AppConfig.PhoneMappingCollection = "test_phone_mappings"
	config.AppConfig.OptInHistoryCollection = "test_opt_in_history"
	config.AppConfig.BetaGroupCollection = "test_beta_groups"
	config.AppConfig.PhoneQuarantineTTL = 6 * 30 * 24 * time.Hour

	// Create service
	service := NewPhoneMappingService(logging.Logger)

	// Return cleanup function
	return service, func() {
		// Drop only test collections
		config.MongoDB.Collection(config.AppConfig.CitizenCollection).Drop(ctx)
		config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).Drop(ctx)
		config.MongoDB.Collection(config.AppConfig.OptInHistoryCollection).Drop(ctx)
		config.MongoDB.Collection(config.AppConfig.BetaGroupCollection).Drop(ctx)

		// Restore original collection names
		config.AppConfig.CitizenCollection = originalCitizenCollection
		config.AppConfig.PhoneMappingCollection = originalPhoneMappingCollection
		config.AppConfig.OptInHistoryCollection = originalOptInHistoryCollection
		config.AppConfig.BetaGroupCollection = originalBetaGroupCollection
		config.AppConfig.PhoneQuarantineTTL = originalPhoneQuarantineTTL
	}
}

func TestNewPhoneMappingService(t *testing.T) {
	service := NewPhoneMappingService(logging.Logger)
	if service == nil {
		t.Error("NewPhoneMappingService() returned nil")
		return
	}
	if service.logger == nil {
		t.Error("service.logger is nil")
	}
}

func TestGetPhoneStatus_NotFound(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test with non-existent phone number
	status, err := service.GetPhoneStatus(ctx, "+5521987654321")
	if err != nil {
		t.Errorf("GetPhoneStatus() error = %v, want nil", err)
	}
	if status == nil {
		t.Fatal("GetPhoneStatus() returned nil status")
	}
	if status.Found {
		t.Error("GetPhoneStatus() Found = true, want false for non-existent phone")
	}
	if status.Quarantined {
		t.Error("GetPhoneStatus() Quarantined = true, want false")
	}
	if status.OptedOut {
		t.Error("GetPhoneStatus() OptedOut = true, want false")
	}
}

func TestGetPhoneStatus_Active(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test citizen
	testCPF := "03561350712"
	testName := "João Silva"
	citizen := models.Citizen{
		CPF:  testCPF,
		Nome: &testName,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.CitizenCollection).InsertOne(ctx, citizen)

	// Insert active phone mapping
	now := time.Now()
	mapping := models.PhoneCPFMapping{
		PhoneNumber: "5521987654321", // Storage format: DDI+DDD+Valor
		CPF:         testCPF,
		Status:      models.MappingStatusActive,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Test
	status, err := service.GetPhoneStatus(ctx, "+5521987654321")
	if err != nil {
		t.Errorf("GetPhoneStatus() error = %v, want nil", err)
	}
	if !status.Found {
		t.Error("GetPhoneStatus() Found = false, want true")
	}
	if status.Quarantined {
		t.Error("GetPhoneStatus() Quarantined = true, want false")
	}
	if status.OptedOut {
		t.Error("GetPhoneStatus() OptedOut = true, want false")
	}
	if status.CPF == "" {
		t.Error("GetPhoneStatus() CPF is empty, want masked CPF")
	}
}

func TestGetPhoneStatus_Quarantined(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert quarantined phone mapping
	now := time.Now()
	quarantineUntil := now.Add(24 * time.Hour)
	mapping := models.PhoneCPFMapping{
		PhoneNumber:     "5521987654322",
		Status:          models.MappingStatusQuarantined,
		QuarantineUntil: &quarantineUntil,
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Test
	status, err := service.GetPhoneStatus(ctx, "+5521987654322")
	if err != nil {
		t.Errorf("GetPhoneStatus() error = %v, want nil", err)
	}
	if !status.Found {
		t.Error("GetPhoneStatus() Found = false, want true")
	}
	if !status.Quarantined {
		t.Error("GetPhoneStatus() Quarantined = false, want true")
	}
}

func TestGetPhoneStatus_OptedOut(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert opted-out phone mapping
	now := time.Now()
	mapping := models.PhoneCPFMapping{
		PhoneNumber: "5521987654323",
		Status:      models.MappingStatusBlocked,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Test
	status, err := service.GetPhoneStatus(ctx, "+5521987654323")
	if err != nil {
		t.Errorf("GetPhoneStatus() error = %v, want nil", err)
	}
	if !status.Found {
		t.Error("GetPhoneStatus() Found = false, want true")
	}
	if !status.OptedOut {
		t.Error("GetPhoneStatus() OptedOut = false, want true")
	}
}

func TestQuarantinePhone_NewPhone(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test quarantining new phone number
	response, err := service.QuarantinePhone(ctx, "+5521999887766")
	if err != nil {
		t.Errorf("QuarantinePhone() error = %v, want nil", err)
	}
	if response.Status != "quarantined" {
		t.Errorf("QuarantinePhone() Status = %s, want quarantined", response.Status)
	}
	if response.QuarantineUntil.Before(time.Now()) {
		t.Error("QuarantinePhone() QuarantineUntil is in the past")
	}

	// Verify mapping was created
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": "5521999887766"},
	).Decode(&mapping)
	if err != nil {
		t.Errorf("Failed to find quarantine record: %v", err)
	}
	if mapping.Status != models.MappingStatusQuarantined {
		t.Errorf("Mapping Status = %s, want %s", mapping.Status, models.MappingStatusQuarantined)
	}
}

func TestQuarantinePhone_ExtendExisting(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert existing quarantine
	now := time.Now()
	oldQuarantineUntil := now.Add(24 * time.Hour)
	mapping := models.PhoneCPFMapping{
		PhoneNumber:     "5521999887767",
		Status:          models.MappingStatusQuarantined,
		QuarantineUntil: &oldQuarantineUntil,
		QuarantineHistory: []models.QuarantineEvent{
			{
				QuarantinedAt:   now.Add(-48 * time.Hour),
				QuarantineUntil: oldQuarantineUntil,
			},
		},
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Extend quarantine
	response, err := service.QuarantinePhone(ctx, "+5521999887767")
	if err != nil {
		t.Errorf("QuarantinePhone() error = %v, want nil", err)
	}
	if response.Status != "quarantined" {
		t.Errorf("QuarantinePhone() Status = %s, want quarantined", response.Status)
	}

	// Verify quarantine was extended
	var updatedMapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": "5521999887767"},
	).Decode(&updatedMapping)
	if err != nil {
		t.Errorf("Failed to find updated quarantine record: %v", err)
	}
	if !updatedMapping.QuarantineUntil.After(oldQuarantineUntil) {
		t.Error("Quarantine was not extended")
	}
	if len(updatedMapping.QuarantineHistory) != 2 {
		t.Errorf("QuarantineHistory length = %d, want 2", len(updatedMapping.QuarantineHistory))
	}
}

func TestReleaseQuarantine_WithCPF(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert quarantined phone with CPF
	now := time.Now()
	quarantineUntil := now.Add(24 * time.Hour)
	mapping := models.PhoneCPFMapping{
		PhoneNumber:     "5521988776655",
		CPF:             "03561350712",
		Status:          models.MappingStatusQuarantined,
		QuarantineUntil: &quarantineUntil,
		QuarantineHistory: []models.QuarantineEvent{
			{
				QuarantinedAt:   now.Add(-48 * time.Hour),
				QuarantineUntil: quarantineUntil,
			},
		},
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Release quarantine
	response, err := service.ReleaseQuarantine(ctx, "+5521988776655")
	if err != nil {
		t.Errorf("ReleaseQuarantine() error = %v, want nil", err)
	}
	if response.Status != "released" {
		t.Errorf("ReleaseQuarantine() Status = %s, want released", response.Status)
	}

	// Verify mapping was updated
	var updatedMapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": "5521988776655"},
	).Decode(&updatedMapping)
	if err != nil {
		t.Errorf("Failed to find updated mapping: %v", err)
	}
	if updatedMapping.Status != models.MappingStatusActive {
		t.Errorf("Status = %s, want %s", updatedMapping.Status, models.MappingStatusActive)
	}
	if updatedMapping.QuarantineUntil != nil {
		t.Error("QuarantineUntil should be nil after release")
	}
}

func TestReleaseQuarantine_WithoutCPF(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert quarantined phone without CPF
	now := time.Now()
	quarantineUntil := now.Add(24 * time.Hour)
	mapping := models.PhoneCPFMapping{
		PhoneNumber:     "5521988776656",
		Status:          models.MappingStatusQuarantined,
		QuarantineUntil: &quarantineUntil,
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Release quarantine
	response, err := service.ReleaseQuarantine(ctx, "+5521988776656")
	if err != nil {
		t.Errorf("ReleaseQuarantine() error = %v, want nil", err)
	}
	if response.Status != "released" {
		t.Errorf("ReleaseQuarantine() Status = %s, want released", response.Status)
	}

	// Verify mapping was deleted
	count, _ := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(
		ctx,
		bson.M{"phone_number": "5521988776656"},
	)
	if count != 0 {
		t.Error("Mapping should be deleted when released without CPF")
	}
}

func TestOptIn_NewPhone(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test opt-in for new phone
	response, err := service.OptIn(ctx, "+5521987651234", "03561350712", "whatsapp")
	if err != nil {
		t.Errorf("OptIn() error = %v, want nil", err)
	}
	if response.Status != "opted_in" {
		t.Errorf("OptIn() Status = %s, want opted_in", response.Status)
	}

	// Verify mapping was created
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": "5521987651234"},
	).Decode(&mapping)
	if err != nil {
		t.Errorf("Failed to find mapping: %v", err)
	}
	if mapping.CPF != "03561350712" {
		t.Errorf("Mapping CPF = %s, want 12345678901", mapping.CPF)
	}
	if mapping.Status != models.MappingStatusActive {
		t.Errorf("Mapping Status = %s, want %s", mapping.Status, models.MappingStatusActive)
	}

	// Verify opt-in history was recorded
	count, _ := config.MongoDB.Collection(config.AppConfig.OptInHistoryCollection).CountDocuments(
		ctx,
		bson.M{"phone_number": "5521987651234", "action": "opt_in"},
	)
	if count != 1 {
		t.Errorf("OptInHistory count = %d, want 1", count)
	}
}

func TestOptIn_AlreadyOptedIn(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert existing active mapping
	now := time.Now()
	mapping := models.PhoneCPFMapping{
		PhoneNumber: "5521987651235",
		CPF:         "03561350712",
		Status:      models.MappingStatusActive,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Try to opt-in again with same CPF
	response, err := service.OptIn(ctx, "+5521987651235", "03561350712", "whatsapp")
	if err != nil {
		t.Errorf("OptIn() error = %v, want nil", err)
	}
	if response.Status != "already_opted_in" {
		t.Errorf("OptIn() Status = %s, want already_opted_in", response.Status)
	}
}

func TestOptOut_ExistingPhone(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert active mapping
	now := time.Now()
	mapping := models.PhoneCPFMapping{
		PhoneNumber: "5521987651236",
		CPF:         "03561350712",
		Status:      models.MappingStatusActive,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Opt out
	response, err := service.OptOut(ctx, "+5521987651236", "Mensagem era engano", "whatsapp")
	if err != nil {
		t.Errorf("OptOut() error = %v, want nil", err)
	}
	if response.Status != "opted_out" {
		t.Errorf("OptOut() Status = %s, want opted_out", response.Status)
	}

	// Verify mapping was blocked
	var updatedMapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": "5521987651236"},
	).Decode(&updatedMapping)
	if err != nil {
		t.Errorf("Failed to find mapping: %v", err)
	}
	if updatedMapping.Status != models.MappingStatusBlocked {
		t.Errorf("Status = %s, want %s", updatedMapping.Status, models.MappingStatusBlocked)
	}

	// Verify opt-out history was recorded
	count, _ := config.MongoDB.Collection(config.AppConfig.OptInHistoryCollection).CountDocuments(
		ctx,
		bson.M{"phone_number": "5521987651236", "action": "opt_out"},
	)
	if count != 1 {
		t.Errorf("OptOutHistory count = %d, want 1", count)
	}
}

func TestOptOut_NewPhone(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Opt out phone that doesn't exist
	response, err := service.OptOut(ctx, "+5521987651237", "Mensagem era engano", "whatsapp")
	if err != nil {
		t.Errorf("OptOut() error = %v, want nil", err)
	}
	if response.Status != "opted_out" {
		t.Errorf("OptOut() Status = %s, want opted_out", response.Status)
	}

	// Verify mapping was created with blocked status
	var mapping models.PhoneCPFMapping
	err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).FindOne(
		ctx,
		bson.M{"phone_number": "5521987651237"},
	).Decode(&mapping)
	if err != nil {
		t.Errorf("Failed to find mapping: %v", err)
	}
	if mapping.Status != models.MappingStatusBlocked {
		t.Errorf("Status = %s, want %s", mapping.Status, models.MappingStatusBlocked)
	}
	if mapping.CPF != "" {
		t.Errorf("CPF should be empty for opt-out without binding, got %s", mapping.CPF)
	}
}

func TestFindCPFByPhone_Found(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert citizen
	testCPF := "98765432100"
	testName := "Maria Santos"
	citizen := models.Citizen{
		CPF:  testCPF,
		Nome: &testName,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.CitizenCollection).InsertOne(ctx, citizen)

	// Insert phone mapping
	now := time.Now()
	mapping := models.PhoneCPFMapping{
		PhoneNumber: "5521987654400",
		CPF:         testCPF,
		Status:      models.MappingStatusActive,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Find CPF by phone
	response, err := service.FindCPFByPhone(ctx, "+5521987654400")
	if err != nil {
		t.Errorf("FindCPFByPhone() error = %v, want nil", err)
	}
	if !response.Found {
		t.Error("FindCPFByPhone() Found = false, want true")
	}
	if response.CPF == "" {
		t.Error("FindCPFByPhone() CPF is empty")
	}
	if response.Name == "" {
		t.Error("FindCPFByPhone() Name is empty")
	}
}

func TestFindCPFByPhone_Quarantined(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert quarantined phone mapping
	now := time.Now()
	quarantineUntil := now.Add(24 * time.Hour)
	mapping := models.PhoneCPFMapping{
		PhoneNumber:     "5521987654401",
		CPF:             "03561350712",
		Status:          models.MappingStatusQuarantined,
		QuarantineUntil: &quarantineUntil,
		CreatedAt:       &now,
		UpdatedAt:       &now,
	}
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, mapping)

	// Find CPF by phone - should not find quarantined phones
	response, err := service.FindCPFByPhone(ctx, "+5521987654401")
	if err != nil {
		t.Errorf("FindCPFByPhone() error = %v, want nil", err)
	}
	if response.Found {
		t.Error("FindCPFByPhone() Found = true, want false for quarantined phone")
	}
}

func TestGetQuarantineStats(t *testing.T) {
	service, cleanup := setupPhoneMappingTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test data
	now := time.Now()

	// Active quarantine with CPF
	futureQuarantine := now.Add(24 * time.Hour)
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, models.PhoneCPFMapping{
		PhoneNumber:     "5521999000001",
		CPF:             "03561350712",
		Status:          models.MappingStatusQuarantined,
		QuarantineUntil: &futureQuarantine,
	})

	// Expired quarantine without CPF
	pastQuarantine := now.Add(-24 * time.Hour)
	_, _ = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, models.PhoneCPFMapping{
		PhoneNumber:     "5521999000002",
		Status:          models.MappingStatusQuarantined,
		QuarantineUntil: &pastQuarantine,
	})

	// Get stats
	stats, err := service.GetQuarantineStats(ctx)
	if err != nil {
		t.Errorf("GetQuarantineStats() error = %v, want nil", err)
	}
	if stats.TotalQuarantined != 2 {
		t.Errorf("TotalQuarantined = %d, want 2", stats.TotalQuarantined)
	}
	if stats.ActiveQuarantines != 1 {
		t.Errorf("ActiveQuarantines = %d, want 1", stats.ActiveQuarantines)
	}
	if stats.ExpiredQuarantines != 1 {
		t.Errorf("ExpiredQuarantines = %d, want 1", stats.ExpiredQuarantines)
	}
	if stats.QuarantinesWithCPF != 1 {
		t.Errorf("QuarantinesWithCPF = %d, want 1", stats.QuarantinesWithCPF)
	}
	if stats.QuarantinesWithoutCPF != 1 {
		t.Errorf("QuarantinesWithoutCPF = %d, want 1", stats.QuarantinesWithoutCPF)
	}
}
