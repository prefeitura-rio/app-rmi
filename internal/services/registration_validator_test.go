package services

import (
	"context"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

// setupValidatorTest initializes MongoDB for registration validator tests
func setupValidatorTest(t *testing.T) (*BaseDataValidator, func()) {
	// Use shared MongoDB from TestMain
	if config.MongoDB == nil {
		t.Skip("Skipping registration validator tests: MongoDB not initialized")
	}

	// Initialize logging
	_ = logging.InitLogger()

	ctx := context.Background()

	// Save original collection name
	origCitizenCollection := config.AppConfig.CitizenCollection

	// Set test collection
	config.AppConfig.CitizenCollection = "test_validator_citizens"

	// Create validator
	validator := NewBaseDataValidator(logging.Logger)

	// Return cleanup function
	return validator, func() {
		// Clean up test collection only
		config.MongoDB.Collection(config.AppConfig.CitizenCollection).Drop(ctx)
		// Restore original collection name
		config.AppConfig.CitizenCollection = origCitizenCollection
	}
}

func TestNewBaseDataValidator(t *testing.T) {
	validator := NewBaseDataValidator(logging.Logger)
	if validator == nil {
		t.Error("NewBaseDataValidator() returned nil")
		return
	}
	if validator.logger == nil {
		t.Error("validator.logger is nil")
	}
}

func TestValidateRegistration_CPFNotFound(t *testing.T) {
	validator, cleanup := setupValidatorTest(t)
	defer cleanup()

	ctx := context.Background()

	// Test with non-existent CPF
	valid, cpf, name, err := validator.ValidateRegistration(ctx, "John Doe", "12345678901", "1990-01-01")
	if err != nil {
		t.Errorf("ValidateRegistration() error = %v, want nil", err)
	}
	if valid {
		t.Error("ValidateRegistration() valid = true, want false for non-existent CPF")
	}
	if cpf != "" {
		t.Errorf("ValidateRegistration() cpf = %s, want empty for non-existent CPF", cpf)
	}
	if name != "" {
		t.Errorf("ValidateRegistration() name = %s, want empty for non-existent CPF", name)
	}
}

func TestValidateRegistration_ExactMatch(t *testing.T) {
	validator, cleanup := setupValidatorTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test citizen
	testCPF := "12345678901"
	testName := "João da Silva"
	testBirthDate := time.Date(1990, 1, 15, 0, 0, 0, 0, time.UTC)

	citizen := models.Citizen{
		CPF:  testCPF,
		Nome: &testName,
		Nascimento: &models.Nascimento{
			Data: &testBirthDate,
		},
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert test citizen: %v", err)
	}

	// Test exact match
	valid, cpf, name, err := validator.ValidateRegistration(ctx, "João da Silva", testCPF, "1990-01-15")
	if err != nil {
		t.Errorf("ValidateRegistration() error = %v, want nil", err)
	}
	if !valid {
		t.Error("ValidateRegistration() valid = false, want true for exact match")
	}
	if cpf != testCPF {
		t.Errorf("ValidateRegistration() cpf = %s, want %s", cpf, testCPF)
	}
	if name != testName {
		t.Errorf("ValidateRegistration() name = %s, want %s", name, testName)
	}
}

func TestValidateRegistration_CaseInsensitiveNameMatch(t *testing.T) {
	validator, cleanup := setupValidatorTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test citizen
	testCPF := "98765432100"
	testName := "Maria Santos"
	testBirthDate := time.Date(1985, 5, 20, 0, 0, 0, 0, time.UTC)

	citizen := models.Citizen{
		CPF:  testCPF,
		Nome: &testName,
		Nascimento: &models.Nascimento{
			Data: &testBirthDate,
		},
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert test citizen: %v", err)
	}

	// Test case-insensitive match
	valid, cpf, name, err := validator.ValidateRegistration(ctx, "MARIA SANTOS", testCPF, "1985-05-20")
	if err != nil {
		t.Errorf("ValidateRegistration() error = %v, want nil", err)
	}
	if !valid {
		t.Error("ValidateRegistration() valid = false, want true for case-insensitive match")
	}
	if cpf != testCPF {
		t.Errorf("ValidateRegistration() cpf = %s, want %s", cpf, testCPF)
	}
	if name != testName {
		t.Errorf("ValidateRegistration() name = %s, want %s", name, testName)
	}
}

func TestValidateRegistration_PartialNameMatch(t *testing.T) {
	validator, cleanup := setupValidatorTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test citizen with full name
	testCPF := "11122233344"
	testName := "José Carlos da Silva Santos"
	testBirthDate := time.Date(1975, 12, 10, 0, 0, 0, 0, time.UTC)

	citizen := models.Citizen{
		CPF:  testCPF,
		Nome: &testName,
		Nascimento: &models.Nascimento{
			Data: &testBirthDate,
		},
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert test citizen: %v", err)
	}

	// Test partial name match (first and last name)
	valid, cpf, name, err := validator.ValidateRegistration(ctx, "José Carlos", testCPF, "1975-12-10")
	if err != nil {
		t.Errorf("ValidateRegistration() error = %v, want nil", err)
	}
	if !valid {
		t.Error("ValidateRegistration() valid = false, want true for partial name match")
	}
	if cpf != testCPF {
		t.Errorf("ValidateRegistration() cpf = %s, want %s", cpf, testCPF)
	}
	if name != testName {
		t.Errorf("ValidateRegistration() name = %s, want %s", name, testName)
	}
}

func TestValidateRegistration_NameMismatch(t *testing.T) {
	validator, cleanup := setupValidatorTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test citizen
	testCPF := "55566677788"
	testName := "Pedro Oliveira"
	testBirthDate := time.Date(1992, 3, 25, 0, 0, 0, 0, time.UTC)

	citizen := models.Citizen{
		CPF:  testCPF,
		Nome: &testName,
		Nascimento: &models.Nascimento{
			Data: &testBirthDate,
		},
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert test citizen: %v", err)
	}

	// Test name mismatch
	valid, cpf, name, err := validator.ValidateRegistration(ctx, "Carlos Souza", testCPF, "1992-03-25")
	if err != nil {
		t.Errorf("ValidateRegistration() error = %v, want nil", err)
	}
	if valid {
		t.Error("ValidateRegistration() valid = true, want false for name mismatch")
	}
	if cpf != testCPF {
		t.Errorf("ValidateRegistration() cpf = %s, want %s", cpf, testCPF)
	}
	if name != testName {
		t.Errorf("ValidateRegistration() name = %s, want %s (base name)", name, testName)
	}
}

func TestValidateRegistration_BirthDateMismatch(t *testing.T) {
	validator, cleanup := setupValidatorTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test citizen
	testCPF := "99988877766"
	testName := "Ana Costa"
	testBirthDate := time.Date(1988, 7, 14, 0, 0, 0, 0, time.UTC)

	citizen := models.Citizen{
		CPF:  testCPF,
		Nome: &testName,
		Nascimento: &models.Nascimento{
			Data: &testBirthDate,
		},
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert test citizen: %v", err)
	}

	// Test birth date mismatch
	valid, cpf, name, err := validator.ValidateRegistration(ctx, "Ana Costa", testCPF, "1990-07-14")
	if err != nil {
		t.Errorf("ValidateRegistration() error = %v, want nil", err)
	}
	if valid {
		t.Error("ValidateRegistration() valid = true, want false for birth date mismatch")
	}
	if cpf != testCPF {
		t.Errorf("ValidateRegistration() cpf = %s, want %s", cpf, testCPF)
	}
	if name != testName {
		t.Errorf("ValidateRegistration() name = %s, want %s (base name)", name, testName)
	}
}

func TestValidateRegistration_MissingBaseName(t *testing.T) {
	validator, cleanup := setupValidatorTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test citizen without name
	testCPF := "33344455566"
	testBirthDate := time.Date(1995, 11, 5, 0, 0, 0, 0, time.UTC)

	citizen := models.Citizen{
		CPF:  testCPF,
		Nome: nil, // No name in base data
		Nascimento: &models.Nascimento{
			Data: &testBirthDate,
		},
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert test citizen: %v", err)
	}

	// Test with missing base name
	valid, _, _, err := validator.ValidateRegistration(ctx, "Some Name", testCPF, "1995-11-05")
	if err != nil {
		t.Errorf("ValidateRegistration() error = %v, want nil", err)
	}
	if valid {
		t.Error("ValidateRegistration() valid = true, want false for missing base name")
	}
}

func TestValidateRegistration_MissingBaseBirthDate(t *testing.T) {
	validator, cleanup := setupValidatorTest(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test citizen without birth date
	testCPF := "77788899900"
	testName := "Lucas Ferreira"

	citizen := models.Citizen{
		CPF:        testCPF,
		Nome:       &testName,
		Nascimento: nil, // No birth date in base data
	}

	collection := config.MongoDB.Collection(config.AppConfig.CitizenCollection)
	_, err := collection.InsertOne(ctx, citizen)
	if err != nil {
		t.Fatalf("Failed to insert test citizen: %v", err)
	}

	// Test with missing base birth date
	valid, _, _, err := validator.ValidateRegistration(ctx, "Lucas Ferreira", testCPF, "2000-01-01")
	if err != nil {
		t.Errorf("ValidateRegistration() error = %v, want nil", err)
	}
	if valid {
		t.Error("ValidateRegistration() valid = true, want false for missing base birth date")
	}
}

func TestSimpleNameMatch(t *testing.T) {
	validator := NewBaseDataValidator(logging.Logger)

	tests := []struct {
		name         string
		providedName string
		baseName     string
		want         bool
	}{
		{
			name:         "Exact match",
			providedName: "João Silva",
			baseName:     "João Silva",
			want:         true,
		},
		{
			name:         "Case insensitive match",
			providedName: "joão silva",
			baseName:     "JOÃO SILVA",
			want:         true,
		},
		{
			name:         "Partial match - provided in base",
			providedName: "João",
			baseName:     "João da Silva",
			want:         true,
		},
		{
			name:         "Partial match - base in provided",
			providedName: "João da Silva Santos",
			baseName:     "João da Silva",
			want:         true,
		},
		{
			name:         "No match",
			providedName: "Carlos Souza",
			baseName:     "Pedro Santos",
			want:         false,
		},
		{
			name:         "Empty base name",
			providedName: "João Silva",
			baseName:     "",
			want:         false,
		},
		{
			name:         "Whitespace handling",
			providedName: "  João Silva  ",
			baseName:     "João Silva",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.simpleNameMatch(tt.providedName, tt.baseName)
			if got != tt.want {
				t.Errorf("simpleNameMatch(%q, %q) = %v, want %v",
					tt.providedName, tt.baseName, got, tt.want)
			}
		})
	}
}

func TestSimpleBirthDateMatch(t *testing.T) {
	validator := NewBaseDataValidator(logging.Logger)

	tests := []struct {
		name         string
		providedDate string
		baseDate     string
		want         bool
	}{
		{
			name:         "Exact match",
			providedDate: "1990-01-15",
			baseDate:     "1990-01-15",
			want:         true,
		},
		{
			name:         "No match - different date",
			providedDate: "1990-01-15",
			baseDate:     "1991-01-15",
			want:         false,
		},
		{
			name:         "Empty base date",
			providedDate: "1990-01-15",
			baseDate:     "",
			want:         false,
		},
		{
			name:         "Whitespace handling",
			providedDate: "  1990-01-15  ",
			baseDate:     "1990-01-15",
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validator.simpleBirthDateMatch(tt.providedDate, tt.baseDate)
			if got != tt.want {
				t.Errorf("simpleBirthDateMatch(%q, %q) = %v, want %v",
					tt.providedDate, tt.baseDate, got, tt.want)
			}
		})
	}
}

func TestMockValidator(t *testing.T) {
	ctx := context.Background()

	t.Run("Should validate", func(t *testing.T) {
		mockCPF := "12345678901"
		mockName := "Test User"
		validator := NewMockValidator(true, mockCPF, mockName)

		valid, cpf, name, err := validator.ValidateRegistration(ctx, "Any Name", "AnyCPF", "AnyDate")
		if err != nil {
			t.Errorf("ValidateRegistration() error = %v, want nil", err)
		}
		if !valid {
			t.Error("ValidateRegistration() valid = false, want true")
		}
		if cpf != mockCPF {
			t.Errorf("ValidateRegistration() cpf = %s, want %s", cpf, mockCPF)
		}
		if name != mockName {
			t.Errorf("ValidateRegistration() name = %s, want %s", name, mockName)
		}
	})

	t.Run("Should not validate", func(t *testing.T) {
		validator := NewMockValidator(false, "", "")

		valid, cpf, name, err := validator.ValidateRegistration(ctx, "Any Name", "AnyCPF", "AnyDate")
		if err != nil {
			t.Errorf("ValidateRegistration() error = %v, want nil", err)
		}
		if valid {
			t.Error("ValidateRegistration() valid = true, want false")
		}
		if cpf != "" {
			t.Errorf("ValidateRegistration() cpf = %s, want empty", cpf)
		}
		if name != "" {
			t.Errorf("ValidateRegistration() name = %s, want empty", name)
		}
	})
}
