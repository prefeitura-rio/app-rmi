package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePhoneNumber(t *testing.T) {
	tests := []struct {
		name        string
		phoneString string
		wantDDI     string
		wantDDD     string
		wantValor   string
		wantErr     bool
	}{
		// Brazilian numbers
		{
			name:        "Brazilian mobile with country code",
			phoneString: "+5521987654321",
			wantDDI:     "55",
			wantDDD:     "21",
			wantValor:   "987654321",
			wantErr:     false,
		},
		{
			name:        "Brazilian mobile without plus",
			phoneString: "5521987654321",
			wantDDI:     "55",
			wantDDD:     "21",
			wantValor:   "987654321",
			wantErr:     false,
		},
		{
			name:        "Brazilian mobile without country code",
			phoneString: "21987654321",
			wantDDI:     "55",
			wantDDD:     "21",
			wantValor:   "987654321",
			wantErr:     false,
		},
		{
			name:        "Brazilian landline 8 digits",
			phoneString: "+552133334444",
			wantDDI:     "55",
			wantDDD:     "21",
			wantValor:   "33334444",
			wantErr:     false,
		},
		{
			name:        "Brazilian landline 9 digits",
			phoneString: "+5521933334444",
			wantDDI:     "55",
			wantDDD:     "21",
			wantValor:   "933334444",
			wantErr:     false,
		},
		{
			name:        "São Paulo mobile",
			phoneString: "+5511999887766",
			wantDDI:     "55",
			wantDDD:     "11",
			wantValor:   "999887766",
			wantErr:     false,
		},

		// International numbers
		{
			name:        "US number",
			phoneString: "+12125551234",
			wantDDI:     "1",
			wantDDD:     "212",
			wantValor:   "5551234",
			wantErr:     false,
		},
		{
			name:        "UK number",
			phoneString: "+442071838750",
			wantDDI:     "44",
			wantDDD:     "2071",
			wantValor:   "838750",
			wantErr:     false,
		},

		// Invalid numbers
		{
			name:        "Too short",
			phoneString: "+5521123",
			wantErr:     true,
		},
		{
			name:        "Too long",
			phoneString: "+552112345678901234",
			wantErr:     true,
		},
		{
			name:        "Invalid format with letters",
			phoneString: "+5521abc87654321",
			wantErr:     true,
		},
		{
			name:        "Empty string",
			phoneString: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePhoneNumber(tt.phoneString)

			if tt.wantErr {
				assert.Error(t, err, "ParsePhoneNumber(%q) should return error", tt.phoneString)
				return
			}

			require.NoError(t, err, "ParsePhoneNumber(%q) unexpected error", tt.phoneString)
			require.NotNil(t, result, "ParsePhoneNumber(%q) should not return nil result", tt.phoneString)

			assert.Equal(t, tt.wantDDI, result.DDI, "DDI mismatch")
			assert.Equal(t, tt.wantDDD, result.DDD, "DDD mismatch")
			assert.Equal(t, tt.wantValor, result.Valor, "Valor mismatch")
			assert.NotEmpty(t, result.Full, "Full phone number should not be empty")
		})
	}
}

func TestParsePhoneNumber_BrazilianSpecialCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantDDI   string
		wantDDD   string
		wantValor string
	}{
		{"Rio mobile with spaces", "+55 21 98765 4321", "55", "21", "987654321"},
		{"São Paulo landline", "1133334444", "55", "11", "33334444"},
		{"Brasília mobile", "+556199887766", "55", "61", "99887766"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePhoneNumber(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDDI, result.DDI)
			assert.Equal(t, tt.wantDDD, result.DDD)
			assert.Equal(t, tt.wantValor, result.Valor)
		})
	}
}

func TestParsePhoneNumber_InternationalNumbers(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantDDI string
		wantErr bool
	}{
		{"Germany", "+4930123456", "49", false},
		{"France", "+33123456789", "33", false},
		{"Argentina", "+5491123456789", "54", false},
		{"Portugal", "+351212345678", "351", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePhoneNumber(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantDDI, result.DDI)
				assert.NotEmpty(t, result.DDD)
				assert.NotEmpty(t, result.Valor)
			}
		})
	}
}

func TestValidatePhoneFormat(t *testing.T) {
	tests := []struct {
		name        string
		phoneString string
		wantErr     bool
	}{
		{
			name:        "Valid Brazilian mobile",
			phoneString: "+5521987654321",
			wantErr:     false,
		},
		{
			name:        "Valid without plus",
			phoneString: "5521987654321",
			wantErr:     false,
		},
		{
			name:        "Valid US number",
			phoneString: "+12125551234",
			wantErr:     false,
		},
		{
			name:        "Too short",
			phoneString: "+123456789",
			wantErr:     true,
		},
		{
			name:        "Too long",
			phoneString: "+12345678901234567",
			wantErr:     true,
		},
		{
			name:        "Contains letters",
			phoneString: "+5521abc654321",
			wantErr:     true,
		},
		{
			name:        "Empty string",
			phoneString: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhoneFormat(tt.phoneString)
			if tt.wantErr {
				assert.Error(t, err, "ValidatePhoneFormat(%q) should return error", tt.phoneString)
			} else {
				assert.NoError(t, err, "ValidatePhoneFormat(%q) should not return error", tt.phoneString)
			}
		})
	}
}

func TestValidatePhoneFormat_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Minimum valid length", "+1234567890", false},
		{"Maximum valid length", "+123456789012345", false},
		{"One digit too short", "+123456789", true},
		{"One digit too long", "+1234567890123456", true},
		{"Special characters", "+55-21-98765-4321", true},
		{"Parentheses", "+55(21)987654321", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhoneFormat(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFormatPhoneForStorage(t *testing.T) {
	tests := []struct {
		name  string
		ddi   string
		ddd   string
		valor string
		want  string
	}{
		{
			name:  "Brazilian mobile",
			ddi:   "55",
			ddd:   "21",
			valor: "987654321",
			want:  "5521987654321",
		},
		{
			name:  "US number",
			ddi:   "1",
			ddd:   "212",
			valor: "5551234",
			want:  "12125551234",
		},
		{
			name:  "Empty components",
			ddi:   "",
			ddd:   "",
			valor: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatPhoneForStorage(tt.ddi, tt.ddd, tt.valor)
			assert.Equal(t, tt.want, result, "FormatPhoneForStorage result mismatch")
		})
	}
}

func TestFormatPhoneForStorage_Concatenation(t *testing.T) {
	result := FormatPhoneForStorage("55", "21", "987654321")
	assert.Equal(t, "5521987654321", result)
	assert.Len(t, result, 13)
	assert.Equal(t, "55", result[:2])
	assert.Equal(t, "21", result[2:4])
	assert.Equal(t, "987654321", result[4:])
}

func TestExtractPhoneFromComponents(t *testing.T) {
	tests := []struct {
		name  string
		ddi   string
		ddd   string
		valor string
		want  string
	}{
		{
			name:  "Brazilian mobile",
			ddi:   "55",
			ddd:   "21",
			valor: "987654321",
			want:  "+5521987654321",
		},
		{
			name:  "US number",
			ddi:   "1",
			ddd:   "212",
			valor: "5551234",
			want:  "+12125551234",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractPhoneFromComponents(tt.ddi, tt.ddd, tt.valor)
			assert.Equal(t, tt.want, result, "ExtractPhoneFromComponents result mismatch")
			assert.True(t, result[0] == '+', "Result should start with +")
		})
	}
}

func TestExtractPhoneFromComponents_Format(t *testing.T) {
	result := ExtractPhoneFromComponents("55", "21", "987654321")
	assert.Equal(t, "+5521987654321", result)
	assert.True(t, result[0] == '+', "Should start with +")
	assert.Contains(t, result, "55")
	assert.Contains(t, result, "21")
	assert.Contains(t, result, "987654321")
}

func TestPhoneComponents_Structure(t *testing.T) {
	components := &PhoneComponents{
		DDI:   "55",
		DDD:   "21",
		Valor: "987654321",
		Full:  "+5521987654321",
	}

	assert.Equal(t, "55", components.DDI)
	assert.Equal(t, "21", components.DDD)
	assert.Equal(t, "987654321", components.Valor)
	assert.Equal(t, "+5521987654321", components.Full)
}
