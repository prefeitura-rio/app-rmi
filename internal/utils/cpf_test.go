package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCPF(t *testing.T) {
	tests := []struct {
		name  string
		cpf   string
		valid bool
	}{
		// Valid CPFs
		{
			name:  "Valid CPF without formatting",
			cpf:   "12345678909",
			valid: true,
		},
		{
			name:  "Valid CPF with formatting",
			cpf:   "123.456.789-09",
			valid: true,
		},
		{
			name:  "Valid CPF - real example 1",
			cpf:   "11144477735",
			valid: true,
		},
		{
			name:  "Valid CPF - real example 2",
			cpf:   "52998224725",
			valid: true,
		},

		// Invalid CPFs
		{
			name:  "Invalid CPF - wrong check digit",
			cpf:   "12345678900",
			valid: false,
		},
		{
			name:  "Invalid CPF - all zeros",
			cpf:   "00000000000",
			valid: false,
		},
		{
			name:  "Invalid CPF - all ones",
			cpf:   "11111111111",
			valid: false,
		},
		{
			name:  "Invalid CPF - all twos",
			cpf:   "22222222222",
			valid: false,
		},
		{
			name:  "Invalid CPF - sequential digits",
			cpf:   "12345678910",
			valid: false,
		},
		{
			name:  "Invalid CPF - too short",
			cpf:   "123456789",
			valid: false,
		},
		{
			name:  "Invalid CPF - too long",
			cpf:   "123456789012",
			valid: false,
		},
		{
			name:  "Invalid CPF - empty string",
			cpf:   "",
			valid: false,
		},
		{
			name:  "Invalid CPF - only letters",
			cpf:   "abcdefghijk",
			valid: false,
		},
		{
			name:  "Invalid CPF - mixed alphanumeric",
			cpf:   "123abc78909",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateCPF(tt.cpf)
			assert.Equal(t, tt.valid, result, "ValidateCPF(%q) should be %v", tt.cpf, tt.valid)
		})
	}
}

func TestValidateCPF_AdditionalValidCPFs(t *testing.T) {
	validCPFs := []string{
		"03561350712",
		"45049725810",
		"00000000191",
	}

	for _, cpf := range validCPFs {
		t.Run("Valid CPF: "+cpf, func(t *testing.T) {
			assert.True(t, ValidateCPF(cpf), "CPF %s should be valid", cpf)
		})
	}
}

func TestValidateCPF_AllSameDigits(t *testing.T) {
	invalidCPFs := []string{
		"33333333333",
		"44444444444",
		"55555555555",
		"66666666666",
		"77777777777",
		"88888888888",
		"99999999999",
	}

	for _, cpf := range invalidCPFs {
		t.Run("All same digits: "+cpf, func(t *testing.T) {
			assert.False(t, ValidateCPF(cpf), "CPF %s should be invalid (all same digits)", cpf)
		})
	}
}

func TestValidateCPF_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name string
		cpf  string
	}{
		{"CPF with @ symbol", "123@456#789$0"},
		{"CPF with parentheses", "(123)456.789-09"},
		{"CPF with brackets", "[123]456.789-09"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should handle special chars gracefully (strip them out)
			ValidateCPF(tt.cpf) // Just ensure it doesn't panic
		})
	}
}

func TestValidateCNPJ(t *testing.T) {
	tests := []struct {
		name  string
		cnpj  string
		valid bool
	}{
		// Valid CNPJs
		{
			name:  "Valid CNPJ without formatting",
			cnpj:  "11222333000181",
			valid: true,
		},
		{
			name:  "Valid CNPJ with formatting",
			cnpj:  "11.222.333/0001-81",
			valid: true,
		},
		{
			name:  "Valid CNPJ - real example",
			cnpj:  "60746948000112",
			valid: true,
		},

		// Invalid CNPJs
		{
			name:  "Invalid CNPJ - wrong check digit",
			cnpj:  "11222333000180",
			valid: false,
		},
		{
			name:  "Invalid CNPJ - all zeros",
			cnpj:  "00000000000000",
			valid: false,
		},
		{
			name:  "Invalid CNPJ - all ones",
			cnpj:  "11111111111111",
			valid: false,
		},
		{
			name:  "Invalid CNPJ - too short",
			cnpj:  "1122233300018",
			valid: false,
		},
		{
			name:  "Invalid CNPJ - too long",
			cnpj:  "112223330001811",
			valid: false,
		},
		{
			name:  "Invalid CNPJ - empty string",
			cnpj:  "",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateCNPJ(tt.cnpj)
			assert.Equal(t, tt.valid, result, "ValidateCNPJ(%q) should be %v", tt.cnpj, tt.valid)
		})
	}
}

func TestValidateCNPJ_AdditionalValidCNPJs(t *testing.T) {
	validCNPJs := []string{
		"00000000000191",
		"11444777000161",
	}

	for _, cnpj := range validCNPJs {
		t.Run("Valid CNPJ: "+cnpj, func(t *testing.T) {
			assert.True(t, ValidateCNPJ(cnpj), "CNPJ %s should be valid", cnpj)
		})
	}
}

func TestValidateCNPJ_AllSameDigits(t *testing.T) {
	invalidCNPJs := []string{
		"22222222222222",
		"33333333333333",
		"44444444444444",
		"55555555555555",
		"66666666666666",
		"77777777777777",
		"88888888888888",
		"99999999999999",
	}

	for _, cnpj := range invalidCNPJs {
		t.Run("All same digits: "+cnpj, func(t *testing.T) {
			assert.False(t, ValidateCNPJ(cnpj), "CNPJ %s should be invalid (all same digits)", cnpj)
		})
	}
}

func TestValidateCNPJ_InvalidCheckDigits(t *testing.T) {
	invalidCNPJs := []string{
		"11222333000182", // Last digit wrong
		"00000000000190", // Last digit wrong
		"11444777000160", // Last digit wrong
		"11222333000171", // Both check digits wrong
	}

	for _, cnpj := range invalidCNPJs {
		t.Run("Invalid check digits: "+cnpj, func(t *testing.T) {
			assert.False(t, ValidateCNPJ(cnpj), "CNPJ %s should be invalid (wrong check digits)", cnpj)
		})
	}
}
