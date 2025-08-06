package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/prefeitura-rio/app-rmi/internal/models"
)

// ValidationError represents a validation error with field and message
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationResult represents the result of validation
type ValidationResult struct {
	IsValid bool              `json:"is_valid"`
	Errors  []ValidationError `json:"errors,omitempty"`
}

// NewValidationResult creates a new validation result
func NewValidationResult() *ValidationResult {
	return &ValidationResult{
		IsValid: true,
		Errors:  []ValidationError{},
	}
}

// AddError adds a validation error to the result
func (vr *ValidationResult) AddError(field, message string) {
	vr.IsValid = false
	vr.Errors = append(vr.Errors, ValidationError{
		Field:   field,
		Message: message,
	})
}

// ValidateAddress validates address input data
func ValidateAddress(input models.SelfDeclaredAddressInput) *ValidationResult {
	result := NewValidationResult()

	// Required fields validation
	if strings.TrimSpace(input.CEP) == "" {
		result.AddError("cep", "CEP is required")
	}
	if strings.TrimSpace(input.Estado) == "" {
		result.AddError("estado", "Estado is required")
	}
	if strings.TrimSpace(input.Municipio) == "" {
		result.AddError("municipio", "Municipio is required")
	}
	if strings.TrimSpace(input.Logradouro) == "" {
		result.AddError("logradouro", "Logradouro is required")
	}
	if strings.TrimSpace(input.Numero) == "" {
		result.AddError("numero", "Numero is required")
	}
	if strings.TrimSpace(input.Bairro) == "" {
		result.AddError("bairro", "Bairro is required")
	}

	// CEP format validation (Brazilian postal code: 00000-000)
	cepRegex := regexp.MustCompile(`^\d{5}-?\d{3}$`)
	if input.CEP != "" && !cepRegex.MatchString(input.CEP) {
		result.AddError("cep", "CEP must be in format 00000-000 or 00000000")
	}

	// State validation (must be 2 characters)
	if input.Estado != "" && len(input.Estado) != 2 {
		result.AddError("estado", "Estado must be exactly 2 characters")
	}

	// Length validations
	if input.Logradouro != "" && len(input.Logradouro) > 200 {
		result.AddError("logradouro", "Logradouro must not exceed 200 characters")
	}
	if input.Numero != "" && len(input.Numero) > 20 {
		result.AddError("numero", "Numero must not exceed 20 characters")
	}
	if input.Complemento != nil && len(*input.Complemento) > 100 {
		result.AddError("complemento", "Complemento must not exceed 100 characters")
	}
	if input.Bairro != "" && len(input.Bairro) > 100 {
		result.AddError("bairro", "Bairro must not exceed 100 characters")
	}
	if input.Municipio != "" && len(input.Municipio) > 100 {
		result.AddError("municipio", "Municipio must not exceed 100 characters")
	}

	return result
}

// ValidatePhone validates phone input data
func ValidatePhone(input models.SelfDeclaredPhoneInput) *ValidationResult {
	result := NewValidationResult()

	// Required fields validation
	if strings.TrimSpace(input.DDI) == "" {
		result.AddError("ddi", "DDI is required")
	}
	if strings.TrimSpace(input.Valor) == "" {
		result.AddError("valor", "Valor is required")
	}

	// DDI validation (country code: 1-3 digits)
	ddiRegex := regexp.MustCompile(`^\d{1,3}$`)
	if input.DDI != "" && !ddiRegex.MatchString(input.DDI) {
		result.AddError("ddi", "DDI must be 1-3 digits")
	}

	// DDD validation (area code: 2 digits for Brazil, optional for international)
	if input.DDD != "" {
		if input.DDI == "55" { // Brazil
			dddRegex := regexp.MustCompile(`^\d{2}$`)
			if !dddRegex.MatchString(input.DDD) {
				result.AddError("ddd", "DDD must be exactly 2 digits for Brazil")
			}
		} else {
			// For international numbers, DDD can be 1-4 digits
			dddRegex := regexp.MustCompile(`^\d{1,4}$`)
			if !dddRegex.MatchString(input.DDD) {
				result.AddError("ddd", "DDD must be 1-4 digits for international numbers")
			}
		}
	}

	// Phone number validation (7-15 digits)
	phoneRegex := regexp.MustCompile(`^\d{7,15}$`)
	if input.Valor != "" && !phoneRegex.MatchString(input.Valor) {
		result.AddError("valor", "Phone number must be 7-15 digits")
	}

	return result
}

// ValidateEmail validates email input data
func ValidateEmail(input models.SelfDeclaredEmailInput) *ValidationResult {
	result := NewValidationResult()

	// Required field validation
	if strings.TrimSpace(input.Valor) == "" {
		result.AddError("valor", "Email is required")
		return result
	}

	// Email format validation
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(input.Valor) {
		result.AddError("valor", "Invalid email format")
	}

	// Length validation
	if len(input.Valor) > 254 {
		result.AddError("valor", "Email must not exceed 254 characters")
	}

	// Domain validation (basic)
	parts := strings.Split(input.Valor, "@")
	if len(parts) == 2 {
		domain := parts[1]
		if len(domain) > 253 {
			result.AddError("valor", "Email domain is too long")
		}
		if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
			result.AddError("valor", "Email domain cannot start or end with a dot")
		}
	}

	return result
}

// ValidateEthnicity validates ethnicity input data
func ValidateEthnicity(input models.SelfDeclaredRacaInput) *ValidationResult {
	result := NewValidationResult()

	// Required field validation
	if strings.TrimSpace(input.Valor) == "" {
		result.AddError("valor", "Ethnicity is required")
		return result
	}

	// Check if ethnicity is in valid options
	if !models.IsValidEthnicity(input.Valor) {
		validOptions := models.ValidEthnicityOptions()
		result.AddError("valor", fmt.Sprintf("Invalid ethnicity. Valid options are: %s", strings.Join(validOptions, ", ")))
	}

	return result
}

// ValidateSelfDeclaredData validates all self-declared data for consistency
func ValidateSelfDeclaredData(existingData *models.SelfDeclaredData, newData interface{}) *ValidationResult {
	result := NewValidationResult()

	// Check for conflicts with existing data
	if existingData != nil {
		switch data := newData.(type) {
		case models.SelfDeclaredAddressInput:
			if existingData.Endereco != nil && existingData.Endereco.Principal != nil {
				// Check if the new address is significantly different
				existing := existingData.Endereco.Principal
				if existing.CEP != nil && *existing.CEP == data.CEP &&
					existing.Numero != nil && *existing.Numero == data.Numero &&
					existing.Logradouro != nil && *existing.Logradouro == data.Logradouro {
					result.AddError("address", "New address is too similar to existing address")
				}
			}
		case models.SelfDeclaredPhoneInput:
			if existingData.Telefone != nil && existingData.Telefone.Principal != nil {
				existing := existingData.Telefone.Principal
				if existing.DDI != nil && *existing.DDI == data.DDI &&
					existing.DDD != nil && *existing.DDD == data.DDD &&
					existing.Valor != nil && *existing.Valor == data.Valor {
					result.AddError("phone", "Phone number already exists")
				}
			}
		case models.SelfDeclaredEmailInput:
			if existingData.Email != nil && existingData.Email.Principal != nil {
				existing := existingData.Email.Principal
				if existing.Valor != nil && *existing.Valor == data.Valor {
					result.AddError("email", "Email already exists")
				}
			}
		}
	}

	return result
}

// SanitizeString removes leading/trailing whitespace and normalizes string
func SanitizeString(s string) string {
	return strings.TrimSpace(s)
}

// SanitizeAddressInput sanitizes address input data
func SanitizeAddressInput(input models.SelfDeclaredAddressInput) models.SelfDeclaredAddressInput {
	return models.SelfDeclaredAddressInput{
		CEP:            SanitizeString(input.CEP),
		Estado:         SanitizeString(input.Estado),
		Municipio:      SanitizeString(input.Municipio),
		TipoLogradouro: sanitizeStringPtr(input.TipoLogradouro),
		Logradouro:     SanitizeString(input.Logradouro),
		Numero:         SanitizeString(input.Numero),
		Complemento:    sanitizeStringPtr(input.Complemento),
		Bairro:         SanitizeString(input.Bairro),
	}
}

// SanitizePhoneInput sanitizes phone input data
func SanitizePhoneInput(input models.SelfDeclaredPhoneInput) models.SelfDeclaredPhoneInput {
	return models.SelfDeclaredPhoneInput{
		DDI:   SanitizeString(input.DDI),
		DDD:   SanitizeString(input.DDD),
		Valor: SanitizeString(input.Valor),
	}
}

// SanitizeEmailInput sanitizes email input data
func SanitizeEmailInput(input models.SelfDeclaredEmailInput) models.SelfDeclaredEmailInput {
	return models.SelfDeclaredEmailInput{
		Valor: strings.ToLower(SanitizeString(input.Valor)),
	}
}

// SanitizeEthnicityInput sanitizes ethnicity input data
func SanitizeEthnicityInput(input models.SelfDeclaredRacaInput) models.SelfDeclaredRacaInput {
	return models.SelfDeclaredRacaInput{
		Valor: SanitizeString(input.Valor),
	}
}

// sanitizeStringPtr sanitizes a string pointer
func sanitizeStringPtr(s *string) *string {
	if s == nil {
		return nil
	}
	sanitized := SanitizeString(*s)
	return &sanitized
} 