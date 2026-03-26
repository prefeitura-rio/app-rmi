package utils

import (
	"strings"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/models"
)

func TestNewValidationResult(t *testing.T) {
	result := NewValidationResult()

	if result == nil {
		t.Fatal("NewValidationResult() returned nil")
	}
	if !result.IsValid {
		t.Error("NewValidationResult() IsValid should be true")
	}
	if result.Errors == nil {
		t.Error("NewValidationResult() Errors should not be nil")
	}
	if len(result.Errors) != 0 {
		t.Errorf("NewValidationResult() should have 0 errors, got %d", len(result.Errors))
	}
}

func TestValidationResult_AddError(t *testing.T) {
	result := NewValidationResult()

	result.AddError("test_field", "test message")

	if result.IsValid {
		t.Error("AddError() should set IsValid to false")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("AddError() should have 1 error, got %d", len(result.Errors))
	}
	if result.Errors[0].Field != "test_field" {
		t.Errorf("AddError() Field = %q, want %q", result.Errors[0].Field, "test_field")
	}
	if result.Errors[0].Message != "test message" {
		t.Errorf("AddError() Message = %q, want %q", result.Errors[0].Message, "test message")
	}

	// Add another error
	result.AddError("field2", "message2")
	if len(result.Errors) != 2 {
		t.Errorf("AddError() should have 2 errors, got %d", len(result.Errors))
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name          string
		input         models.SelfDeclaredAddressInput
		wantValid     bool
		wantErrorKeys []string
	}{
		{
			name: "Valid address with CEP dash",
			input: models.SelfDeclaredAddressInput{
				CEP:        "20040-020",
				Estado:     "RJ",
				Municipio:  "Rio de Janeiro",
				Logradouro: "Avenida Rio Branco",
				Numero:     "156",
				Bairro:     "Centro",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid address without CEP dash",
			input: models.SelfDeclaredAddressInput{
				CEP:        "20040020",
				Estado:     "RJ",
				Municipio:  "Rio de Janeiro",
				Logradouro: "Avenida Rio Branco",
				Numero:     "156",
				Bairro:     "Centro",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid address with complemento",
			input: models.SelfDeclaredAddressInput{
				CEP:         "20040-020",
				Estado:      "RJ",
				Municipio:   "Rio de Janeiro",
				Logradouro:  "Avenida Rio Branco",
				Numero:      "156",
				Bairro:      "Centro",
				Complemento: strPtr("Apto 101"),
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Missing required fields",
			input: models.SelfDeclaredAddressInput{
				CEP:    "",
				Estado: "",
			},
			wantValid:     false,
			wantErrorKeys: []string{"cep", "estado", "municipio", "logradouro", "numero", "bairro"},
		},
		{
			name: "Invalid CEP format",
			input: models.SelfDeclaredAddressInput{
				CEP:        "123",
				Estado:     "RJ",
				Municipio:  "Rio de Janeiro",
				Logradouro: "Avenida Rio Branco",
				Numero:     "156",
				Bairro:     "Centro",
			},
			wantValid:     false,
			wantErrorKeys: []string{"cep"},
		},
		{
			name: "Invalid estado length",
			input: models.SelfDeclaredAddressInput{
				CEP:        "20040-020",
				Estado:     "RIO",
				Municipio:  "Rio de Janeiro",
				Logradouro: "Avenida Rio Branco",
				Numero:     "156",
				Bairro:     "Centro",
			},
			wantValid:     false,
			wantErrorKeys: []string{"estado"},
		},
		{
			name: "Logradouro too long",
			input: models.SelfDeclaredAddressInput{
				CEP:        "20040-020",
				Estado:     "RJ",
				Municipio:  "Rio de Janeiro",
				Logradouro: strings.Repeat("A", 201),
				Numero:     "156",
				Bairro:     "Centro",
			},
			wantValid:     false,
			wantErrorKeys: []string{"logradouro"},
		},
		{
			name: "Numero too long",
			input: models.SelfDeclaredAddressInput{
				CEP:        "20040-020",
				Estado:     "RJ",
				Municipio:  "Rio de Janeiro",
				Logradouro: "Avenida Rio Branco",
				Numero:     strings.Repeat("1", 21),
				Bairro:     "Centro",
			},
			wantValid:     false,
			wantErrorKeys: []string{"numero"},
		},
		{
			name: "Complemento too long",
			input: models.SelfDeclaredAddressInput{
				CEP:         "20040-020",
				Estado:      "RJ",
				Municipio:   "Rio de Janeiro",
				Logradouro:  "Avenida Rio Branco",
				Numero:      "156",
				Bairro:      "Centro",
				Complemento: strPtr(strings.Repeat("A", 101)),
			},
			wantValid:     false,
			wantErrorKeys: []string{"complemento"},
		},
		{
			name: "Bairro too long",
			input: models.SelfDeclaredAddressInput{
				CEP:        "20040-020",
				Estado:     "RJ",
				Municipio:  "Rio de Janeiro",
				Logradouro: "Avenida Rio Branco",
				Numero:     "156",
				Bairro:     strings.Repeat("A", 101),
			},
			wantValid:     false,
			wantErrorKeys: []string{"bairro"},
		},
		{
			name: "Municipio too long",
			input: models.SelfDeclaredAddressInput{
				CEP:        "20040-020",
				Estado:     "RJ",
				Municipio:  strings.Repeat("A", 101),
				Logradouro: "Avenida Rio Branco",
				Numero:     "156",
				Bairro:     "Centro",
			},
			wantValid:     false,
			wantErrorKeys: []string{"municipio"},
		},
		{
			name: "Multiple errors",
			input: models.SelfDeclaredAddressInput{
				CEP:        "123",
				Estado:     "RIO",
				Municipio:  strings.Repeat("A", 101),
				Logradouro: "",
				Numero:     "",
				Bairro:     "",
			},
			wantValid:     false,
			wantErrorKeys: []string{"cep", "estado", "logradouro", "numero", "bairro", "municipio"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateAddress(tt.input)

			if result.IsValid != tt.wantValid {
				t.Errorf("ValidateAddress() IsValid = %v, want %v", result.IsValid, tt.wantValid)
			}

			if len(result.Errors) != len(tt.wantErrorKeys) {
				t.Errorf("ValidateAddress() error count = %d, want %d. Errors: %v", len(result.Errors), len(tt.wantErrorKeys), result.Errors)
			}

			// Verify expected error fields are present
			errorFields := make(map[string]bool)
			for _, err := range result.Errors {
				errorFields[err.Field] = true
			}

			for _, expectedField := range tt.wantErrorKeys {
				if !errorFields[expectedField] {
					t.Errorf("ValidateAddress() missing expected error for field %q", expectedField)
				}
			}
		})
	}
}

func TestValidatePhone(t *testing.T) {
	tests := []struct {
		name          string
		input         models.SelfDeclaredPhoneInput
		wantValid     bool
		wantErrorKeys []string
	}{
		{
			name: "Valid Brazilian mobile",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "55",
				DDD:   "21",
				Valor: "987654321",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid US phone",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "1",
				DDD:   "212",
				Valor: "5551234",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid phone without DDD",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "44",
				DDD:   "",
				Valor: "2071838750",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Missing required fields",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "",
				Valor: "",
			},
			wantValid:     false,
			wantErrorKeys: []string{"ddi", "valor"},
		},
		{
			name: "Invalid DDI format - too long",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "5555",
				DDD:   "21",
				Valor: "987654321",
			},
			wantValid:     false,
			wantErrorKeys: []string{"ddi"},
		},
		{
			name: "Invalid DDI format - letters",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "5A",
				DDD:   "21",
				Valor: "987654321",
			},
			wantValid:     false,
			wantErrorKeys: []string{"ddi"},
		},
		{
			name: "Invalid Brazilian DDD - not 2 digits",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "55",
				DDD:   "211",
				Valor: "987654321",
			},
			wantValid:     false,
			wantErrorKeys: []string{"ddd"},
		},
		{
			name: "Invalid international DDD - too long",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "1",
				DDD:   "12345",
				Valor: "5551234",
			},
			wantValid:     false,
			wantErrorKeys: []string{"ddd"},
		},
		{
			name: "Invalid phone number - too short",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "55",
				DDD:   "21",
				Valor: "123456",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Invalid phone number - too long",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "55",
				DDD:   "21",
				Valor: "1234567890123456",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Invalid phone number - contains letters",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "55",
				DDD:   "21",
				Valor: "98765ABC1",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Multiple errors",
			input: models.SelfDeclaredPhoneInput{
				DDI:   "",
				DDD:   "211",
				Valor: "123",
			},
			wantValid:     false,
			wantErrorKeys: []string{"ddi", "valor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidatePhone(tt.input)

			if result.IsValid != tt.wantValid {
				t.Errorf("ValidatePhone() IsValid = %v, want %v", result.IsValid, tt.wantValid)
			}

			if len(result.Errors) != len(tt.wantErrorKeys) {
				t.Errorf("ValidatePhone() error count = %d, want %d. Errors: %v", len(result.Errors), len(tt.wantErrorKeys), result.Errors)
			}

			errorFields := make(map[string]bool)
			for _, err := range result.Errors {
				errorFields[err.Field] = true
			}

			for _, expectedField := range tt.wantErrorKeys {
				if !errorFields[expectedField] {
					t.Errorf("ValidatePhone() missing expected error for field %q", expectedField)
				}
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name          string
		input         models.SelfDeclaredEmailInput
		wantValid     bool
		wantErrorKeys []string
	}{
		{
			name: "Valid email",
			input: models.SelfDeclaredEmailInput{
				Valor: "user@example.com",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid email with subdomain",
			input: models.SelfDeclaredEmailInput{
				Valor: "user@mail.example.com",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid email with numbers",
			input: models.SelfDeclaredEmailInput{
				Valor: "user123@example456.com",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid email with special chars",
			input: models.SelfDeclaredEmailInput{
				Valor: "user.name+tag@example.com",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Empty email",
			input: models.SelfDeclaredEmailInput{
				Valor: "",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Email without @",
			input: models.SelfDeclaredEmailInput{
				Valor: "userexample.com",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Email without domain",
			input: models.SelfDeclaredEmailInput{
				Valor: "user@",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Email without local part",
			input: models.SelfDeclaredEmailInput{
				Valor: "@example.com",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Email without TLD",
			input: models.SelfDeclaredEmailInput{
				Valor: "user@example",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Email too long",
			input: models.SelfDeclaredEmailInput{
				Valor: strings.Repeat("a", 250) + "@example.com",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Email domain too long",
			input: models.SelfDeclaredEmailInput{
				Valor: "user@" + strings.Repeat("a", 250) + ".com",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor", "valor"}, // Two errors: length AND domain length
		},
		{
			name: "Email domain starts with dot",
			input: models.SelfDeclaredEmailInput{
				Valor: "user@.example.com",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Email domain ends with dot",
			input: models.SelfDeclaredEmailInput{
				Valor: "user@example.com.",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor", "valor"}, // Two errors: invalid format AND domain dot check
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateEmail(tt.input)

			if result.IsValid != tt.wantValid {
				t.Errorf("ValidateEmail() IsValid = %v, want %v. Errors: %v", result.IsValid, tt.wantValid, result.Errors)
			}

			if len(result.Errors) != len(tt.wantErrorKeys) {
				t.Errorf("ValidateEmail() error count = %d, want %d. Errors: %v", len(result.Errors), len(tt.wantErrorKeys), result.Errors)
			}

			errorFields := make(map[string]bool)
			for _, err := range result.Errors {
				errorFields[err.Field] = true
			}

			for _, expectedField := range tt.wantErrorKeys {
				if !errorFields[expectedField] {
					t.Errorf("ValidateEmail() missing expected error for field %q", expectedField)
				}
			}
		})
	}
}

func TestValidateEthnicity(t *testing.T) {
	tests := []struct {
		name          string
		input         models.SelfDeclaredRacaInput
		wantValid     bool
		wantErrorKeys []string
	}{
		{
			name: "Valid - branca",
			input: models.SelfDeclaredRacaInput{
				Valor: "branca",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid - preta",
			input: models.SelfDeclaredRacaInput{
				Valor: "preta",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid - parda",
			input: models.SelfDeclaredRacaInput{
				Valor: "parda",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid - amarela",
			input: models.SelfDeclaredRacaInput{
				Valor: "amarela",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid - indigena",
			input: models.SelfDeclaredRacaInput{
				Valor: "indigena",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Valid - outra",
			input: models.SelfDeclaredRacaInput{
				Valor: "outra",
			},
			wantValid:     true,
			wantErrorKeys: []string{},
		},
		{
			name: "Empty ethnicity",
			input: models.SelfDeclaredRacaInput{
				Valor: "",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Invalid ethnicity",
			input: models.SelfDeclaredRacaInput{
				Valor: "invalid",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
		{
			name: "Wrong case",
			input: models.SelfDeclaredRacaInput{
				Valor: "Branca",
			},
			wantValid:     false,
			wantErrorKeys: []string{"valor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateEthnicity(tt.input)

			if result.IsValid != tt.wantValid {
				t.Errorf("ValidateEthnicity() IsValid = %v, want %v. Errors: %v", result.IsValid, tt.wantValid, result.Errors)
			}

			if len(result.Errors) != len(tt.wantErrorKeys) {
				t.Errorf("ValidateEthnicity() error count = %d, want %d. Errors: %v", len(result.Errors), len(tt.wantErrorKeys), result.Errors)
			}

			errorFields := make(map[string]bool)
			for _, err := range result.Errors {
				errorFields[err.Field] = true
			}

			for _, expectedField := range tt.wantErrorKeys {
				if !errorFields[expectedField] {
					t.Errorf("ValidateEthnicity() missing expected error for field %q", expectedField)
				}
			}
		})
	}
}

func TestValidateSelfDeclaredData(t *testing.T) {
	t.Run("No existing data - should pass", func(t *testing.T) {
		result := ValidateSelfDeclaredData(nil, models.SelfDeclaredAddressInput{
			CEP: "20040-020",
		})

		if !result.IsValid {
			t.Errorf("ValidateSelfDeclaredData() with nil existing data should be valid")
		}
	})

	t.Run("Duplicate address - should fail", func(t *testing.T) {
		existingData := &models.SelfDeclaredData{
			Endereco: &models.Endereco{
				Principal: &models.EnderecoPrincipal{
					CEP:        strPtr("20040-020"),
					Logradouro: strPtr("Avenida Rio Branco"),
					Numero:     strPtr("156"),
				},
			},
		}

		newAddress := models.SelfDeclaredAddressInput{
			CEP:        "20040-020",
			Logradouro: "Avenida Rio Branco",
			Numero:     "156",
		}

		result := ValidateSelfDeclaredData(existingData, newAddress)

		if result.IsValid {
			t.Error("ValidateSelfDeclaredData() should reject duplicate address")
		}
		if len(result.Errors) == 0 {
			t.Error("ValidateSelfDeclaredData() should have errors for duplicate address")
		}
	})

	t.Run("Different address - should pass", func(t *testing.T) {
		existingData := &models.SelfDeclaredData{
			Endereco: &models.Endereco{
				Principal: &models.EnderecoPrincipal{
					CEP:        strPtr("20040-020"),
					Logradouro: strPtr("Avenida Rio Branco"),
					Numero:     strPtr("156"),
				},
			},
		}

		newAddress := models.SelfDeclaredAddressInput{
			CEP:        "20040-021",
			Logradouro: "Avenida Atlântica",
			Numero:     "200",
		}

		result := ValidateSelfDeclaredData(existingData, newAddress)

		if !result.IsValid {
			t.Errorf("ValidateSelfDeclaredData() should accept different address. Errors: %v", result.Errors)
		}
	})

	t.Run("Duplicate phone - should fail", func(t *testing.T) {
		existingData := &models.SelfDeclaredData{
			Telefone: &models.Telefone{
				Principal: &models.TelefonePrincipal{
					DDI:   strPtr("55"),
					DDD:   strPtr("21"),
					Valor: strPtr("987654321"),
				},
			},
		}

		newPhone := models.SelfDeclaredPhoneInput{
			DDI:   "55",
			DDD:   "21",
			Valor: "987654321",
		}

		result := ValidateSelfDeclaredData(existingData, newPhone)

		if result.IsValid {
			t.Error("ValidateSelfDeclaredData() should reject duplicate phone")
		}
	})

	t.Run("Duplicate email - should fail", func(t *testing.T) {
		existingData := &models.SelfDeclaredData{
			Email: &models.Email{
				Principal: &models.EmailPrincipal{
					Valor: strPtr("user@example.com"),
				},
			},
		}

		newEmail := models.SelfDeclaredEmailInput{
			Valor: "user@example.com",
		}

		result := ValidateSelfDeclaredData(existingData, newEmail)

		if result.IsValid {
			t.Error("ValidateSelfDeclaredData() should reject duplicate email")
		}
	})
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "No whitespace",
			input: "test",
			want:  "test",
		},
		{
			name:  "Leading whitespace",
			input: "  test",
			want:  "test",
		},
		{
			name:  "Trailing whitespace",
			input: "test  ",
			want:  "test",
		},
		{
			name:  "Both leading and trailing",
			input: "  test  ",
			want:  "test",
		},
		{
			name:  "Empty string",
			input: "",
			want:  "",
		},
		{
			name:  "Only whitespace",
			input: "   ",
			want:  "",
		},
		{
			name:  "Internal whitespace preserved",
			input: "  hello world  ",
			want:  "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input)
			if result != tt.want {
				t.Errorf("SanitizeString(%q) = %q, want %q", tt.input, result, tt.want)
			}
		})
	}
}

func TestSanitizeAddressInput(t *testing.T) {
	complemento := "  Apto 101  "
	tipoLogradouro := "  Avenida  "

	input := models.SelfDeclaredAddressInput{
		CEP:            "  20040-020  ",
		Estado:         "  RJ  ",
		Municipio:      "  Rio de Janeiro  ",
		TipoLogradouro: &tipoLogradouro,
		Logradouro:     "  Rio Branco  ",
		Numero:         "  156  ",
		Complemento:    &complemento,
		Bairro:         "  Centro  ",
	}

	result := SanitizeAddressInput(input)

	if result.CEP != "20040-020" {
		t.Errorf("SanitizeAddressInput() CEP = %q, want %q", result.CEP, "20040-020")
	}
	if result.Estado != "RJ" {
		t.Errorf("SanitizeAddressInput() Estado = %q, want %q", result.Estado, "RJ")
	}
	if result.Municipio != "Rio de Janeiro" {
		t.Errorf("SanitizeAddressInput() Municipio = %q, want %q", result.Municipio, "Rio de Janeiro")
	}
	if result.Logradouro != "Rio Branco" {
		t.Errorf("SanitizeAddressInput() Logradouro = %q, want %q", result.Logradouro, "Rio Branco")
	}
	if result.Numero != "156" {
		t.Errorf("SanitizeAddressInput() Numero = %q, want %q", result.Numero, "156")
	}
	if result.Bairro != "Centro" {
		t.Errorf("SanitizeAddressInput() Bairro = %q, want %q", result.Bairro, "Centro")
	}
	if result.Complemento == nil || *result.Complemento != "Apto 101" {
		t.Errorf("SanitizeAddressInput() Complemento = %v, want %q", result.Complemento, "Apto 101")
	}
	if result.TipoLogradouro == nil || *result.TipoLogradouro != "Avenida" {
		t.Errorf("SanitizeAddressInput() TipoLogradouro = %v, want %q", result.TipoLogradouro, "Avenida")
	}
}

func TestSanitizePhoneInput(t *testing.T) {
	input := models.SelfDeclaredPhoneInput{
		DDI:   "  55  ",
		DDD:   "  21  ",
		Valor: "  987654321  ",
	}

	result := SanitizePhoneInput(input)

	if result.DDI != "55" {
		t.Errorf("SanitizePhoneInput() DDI = %q, want %q", result.DDI, "55")
	}
	if result.DDD != "21" {
		t.Errorf("SanitizePhoneInput() DDD = %q, want %q", result.DDD, "21")
	}
	if result.Valor != "987654321" {
		t.Errorf("SanitizePhoneInput() Valor = %q, want %q", result.Valor, "987654321")
	}
}

func TestSanitizeEmailInput(t *testing.T) {
	tests := []struct {
		name  string
		input models.SelfDeclaredEmailInput
		want  string
	}{
		{
			name: "Lowercase and trim",
			input: models.SelfDeclaredEmailInput{
				Valor: "  User@Example.COM  ",
			},
			want: "user@example.com",
		},
		{
			name: "Already lowercase",
			input: models.SelfDeclaredEmailInput{
				Valor: "user@example.com",
			},
			want: "user@example.com",
		},
		{
			name: "Only whitespace to trim",
			input: models.SelfDeclaredEmailInput{
				Valor: "  user@example.com  ",
			},
			want: "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeEmailInput(tt.input)
			if result.Valor != tt.want {
				t.Errorf("SanitizeEmailInput() Valor = %q, want %q", result.Valor, tt.want)
			}
		})
	}
}

func TestSanitizeEthnicityInput(t *testing.T) {
	input := models.SelfDeclaredRacaInput{
		Valor: "  parda  ",
	}

	result := SanitizeEthnicityInput(input)

	if result.Valor != "parda" {
		t.Errorf("SanitizeEthnicityInput() Valor = %q, want %q", result.Valor, "parda")
	}
}

func TestSanitizeStringPtr(t *testing.T) {
	t.Run("Nil pointer", func(t *testing.T) {
		result := sanitizeStringPtr(nil)
		if result != nil {
			t.Error("sanitizeStringPtr(nil) should return nil")
		}
	})

	t.Run("Non-nil pointer", func(t *testing.T) {
		input := "  test  "
		result := sanitizeStringPtr(&input)
		if result == nil {
			t.Fatal("sanitizeStringPtr() returned nil for non-nil input")
		}
		if *result != "test" {
			t.Errorf("sanitizeStringPtr() = %q, want %q", *result, "test")
		}
	})

	t.Run("Empty string pointer", func(t *testing.T) {
		input := ""
		result := sanitizeStringPtr(&input)
		if result == nil {
			t.Fatal("sanitizeStringPtr() returned nil for empty string")
		}
		if *result != "" {
			t.Errorf("sanitizeStringPtr() = %q, want %q", *result, "")
		}
	})
}

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}
