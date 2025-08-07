package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
)

// RegistrationValidator defines the interface for registration validation
type RegistrationValidator interface {
	ValidateRegistration(ctx context.Context, name, cpf, birthDate string) (bool, string, string, error)
}

// BaseDataValidator implements RegistrationValidator using base data collection
type BaseDataValidator struct {
	logger *logging.SafeLogger
}

// NewBaseDataValidator creates a new BaseDataValidator
func NewBaseDataValidator(logger *logging.SafeLogger) *BaseDataValidator {
	return &BaseDataValidator{
		logger: logger,
	}
}

// ValidateRegistration validates registration data against base data collection
// This is a placeholder implementation that can be enhanced later
func (v *BaseDataValidator) ValidateRegistration(ctx context.Context, name, cpf, birthDate string) (bool, string, string, error) {
	v.logger.Info("validating registration", 
		zap.String("cpf", cpf),
		zap.String("name", name),
		zap.String("birthDate", birthDate))

	// TODO: Implement actual validation logic
	// For now, we'll do basic validation:
	// 1. Check if CPF exists in base data
	// 2. Check if name matches (with fuzzy matching)
	// 3. Check if birth date matches (with tolerance)

	// Step 1: Check if CPF exists in base data
	var citizen models.Citizen
	err := config.MongoDB.Collection(config.AppConfig.CitizenCollection).FindOne(
		ctx,
		bson.M{"cpf": cpf},
	).Decode(&citizen)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			v.logger.Info("CPF not found in base data", zap.String("cpf", cpf))
			return false, "", "", nil
		}
		v.logger.Error("error querying base data", zap.Error(err))
		return false, "", "", fmt.Errorf("failed to query base data: %w", err)
	}

	// Step 2: Basic name matching (can be enhanced with fuzzy matching later)
	baseName := ""
	if citizen.Nome != nil {
		baseName = *citizen.Nome
	}

	nameMatch := v.simpleNameMatch(name, baseName)
	if !nameMatch {
		v.logger.Info("name does not match", 
			zap.String("provided_name", name),
			zap.String("base_name", baseName))
		return false, cpf, baseName, nil
	}

	// Step 3: Basic birth date matching (can be enhanced with tolerance later)
	baseBirthDate := ""
	if citizen.Nascimento != nil && citizen.Nascimento.Data != nil {
		baseBirthDate = citizen.Nascimento.Data.Format("2006-01-02")
	}
	
	birthDateMatch := v.simpleBirthDateMatch(birthDate, baseBirthDate)
	if !birthDateMatch {
		v.logger.Info("birth date does not match",
			zap.String("provided_birth_date", birthDate),
			zap.String("base_birth_date", baseBirthDate))
		return false, cpf, baseName, nil
	}

	v.logger.Info("registration validation successful",
		zap.String("cpf", cpf),
		zap.String("name", baseName))

	return true, cpf, baseName, nil
}

// simpleNameMatch performs basic name matching
// This can be enhanced with fuzzy matching algorithms later
func (v *BaseDataValidator) simpleNameMatch(providedName, baseName string) bool {
	if baseName == "" {
		return false
	}

	// Normalize names for comparison
	provided := strings.ToLower(strings.TrimSpace(providedName))
	base := strings.ToLower(strings.TrimSpace(baseName))

	// Exact match
	if provided == base {
		return true
	}

	// Check if provided name is contained in base name or vice versa
	if strings.Contains(base, provided) || strings.Contains(provided, base) {
		return true
	}

	// TODO: Implement more sophisticated name matching
	// - Fuzzy string matching
	// - Phonetic matching
	// - Abbreviation handling
	// - Middle name handling

	return false
}

// simpleBirthDateMatch performs basic birth date matching
// This can be enhanced with tolerance and different formats later
func (v *BaseDataValidator) simpleBirthDateMatch(providedDate, baseDate string) bool {
	if baseDate == "" {
		return false
	}

	// Normalize dates for comparison
	provided := strings.TrimSpace(providedDate)
	base := strings.TrimSpace(baseDate)

	// Exact match
	if provided == base {
		return true
	}

	// TODO: Implement more sophisticated date matching
	// - Different date formats
	// - Tolerance for day/month/year variations
	// - Handle missing leading zeros

	return false
}

// MockValidator is a mock implementation for testing
type MockValidator struct {
	ShouldValidate bool
	MockCPF        string
	MockName       string
}

// NewMockValidator creates a new MockValidator
func NewMockValidator(shouldValidate bool, mockCPF, mockName string) *MockValidator {
	return &MockValidator{
		ShouldValidate: shouldValidate,
		MockCPF:        mockCPF,
		MockName:       mockName,
	}
}

// ValidateRegistration implements RegistrationValidator for MockValidator
func (m *MockValidator) ValidateRegistration(ctx context.Context, name, cpf, birthDate string) (bool, string, string, error) {
	if m.ShouldValidate {
		return true, m.MockCPF, m.MockName, nil
	}
	return false, "", "", nil
} 