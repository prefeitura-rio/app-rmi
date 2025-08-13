package services

import (
	"testing"
	"time"
)

func TestDataOperationInterface(t *testing.T) {
	// Test that CitizenDataOperation implements DataOperation interface
	var _ DataOperation = (*CitizenDataOperation)(nil)

	// Test that PhoneMappingDataOperation implements DataOperation interface
	var _ DataOperation = (*PhoneMappingDataOperation)(nil)

	// Test that UserConfigDataOperation implements DataOperation interface
	var _ DataOperation = (*UserConfigDataOperation)(nil)

	// Test that OptInHistoryDataOperation implements DataOperation interface
	var _ DataOperation = (*OptInHistoryDataOperation)(nil)

	// Test that BetaGroupDataOperation implements DataOperation interface
	var _ DataOperation = (*BetaGroupDataOperation)(nil)

	// Test that PhoneVerificationDataOperation implements DataOperation interface
	var _ DataOperation = (*PhoneVerificationDataOperation)(nil)

	// Test that MaintenanceRequestDataOperation implements DataOperation interface
	var _ DataOperation = (*MaintenanceRequestDataOperation)(nil)
}

func TestCitizenDataOperation(t *testing.T) {
	op := &CitizenDataOperation{
		CPF:  "12345678901",
		Data: nil, // We don't need actual data for interface testing
	}

	if op.GetKey() != "12345678901" {
		t.Errorf("Expected key '12345678901', got '%s'", op.GetKey())
	}

	if op.GetCollection() != "citizens" {
		t.Errorf("Expected collection 'citizens', got '%s'", op.GetCollection())
	}

	if op.GetType() != "citizen" {
		t.Errorf("Expected type 'citizen', got '%s'", op.GetType())
	}

	if op.GetTTL() != 24*time.Hour {
		t.Errorf("Expected TTL 24h, got %v", op.GetTTL())
	}
}

func TestPhoneMappingDataOperation(t *testing.T) {
	op := &PhoneMappingDataOperation{
		PhoneNumber: "5511999999999",
		Data:        nil,
	}

	if op.GetKey() != "5511999999999" {
		t.Errorf("Expected key '5511999999999', got '%s'", op.GetKey())
	}

	if op.GetCollection() != "phone_mappings" {
		t.Errorf("Expected collection 'phone_mappings', got '%s'", op.GetCollection())
	}

	if op.GetType() != "phone_mapping" {
		t.Errorf("Expected type 'phone_mapping', got '%s'", op.GetType())
	}

	if op.GetTTL() != 24*time.Hour {
		t.Errorf("Expected TTL 24h, got %v", op.GetTTL())
	}
}
