package observability

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogger(t *testing.T) {
	logger := Logger()
	require.NotNil(t, logger)

	// Should be safe to use
	logger.Info("test message")
}

func TestMaskCPF(t *testing.T) {
	tests := []struct {
		name     string
		cpf      string
		expected string
	}{
		{
			name:     "valid 11-digit CPF without punctuation",
			cpf:      "12345678901",
			expected: "123.***.789-**",
		},
		{
			name:     "another valid CPF without punctuation",
			cpf:      "03561350712",
			expected: "035.***.507-**",
		},
		{
			name:     "formatted CPF with dots and dash",
			cpf:      "123.456.789-01",
			expected: "123.***.789-**",
		},
		{
			name:     "formatted CPF with dots only",
			cpf:      "035.613.507-12",
			expected: "035.***.507-**",
		},
		{
			name:     "invalid CPF too short",
			cpf:      "123456789",
			expected: "***.***.***-**",
		},
		{
			name:     "invalid CPF too long",
			cpf:      "123456789012",
			expected: "***.***.***-**",
		},
		{
			name:     "invalid empty CPF",
			cpf:      "",
			expected: "***.***.***-**",
		},
		{
			name:     "invalid non-numeric",
			cpf:      "abcdefghijk",
			expected: "***.***.***-**",
		},
		{
			name:     "invalid formatted incomplete",
			cpf:      "123.456.789",
			expected: "***.***.***-**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskCPF(tt.cpf)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMaskCPF_PreservesFormat(t *testing.T) {
	cpf := "12345678901"
	masked := MaskCPF(cpf)

	assert.Equal(t, "123", masked[:3])
	assert.Contains(t, masked, ".")
	assert.Contains(t, masked, "-")
	assert.Contains(t, masked, "***")
	assert.Contains(t, masked, "**")
}

func TestCPFFieldAndAttr(t *testing.T) {
	field := CPFField("12345678901")
	assert.Equal(t, "cpf", field.Key)
	assert.Equal(t, "123.***.789-**", field.String)

	attr := CPFAttr("123.456.789-01")
	assert.Equal(t, "cpf", string(attr.Key))
	assert.Equal(t, "123.***.789-**", attr.Value.AsString())
}

func TestMaskSensitiveData(t *testing.T) {
	data := map[string]interface{}{
		"cpf":      "12345678901",
		"nome_mae": "Maria Silva",
		"nome_pai": "João Silva",
		"telefone": "21987654321",
		"nome":     "José Silva",
		"idade":    30,
		"email":    "jose@example.com",
	}

	masked := MaskSensitiveData(data)

	// Sensitive fields should be masked
	assert.Equal(t, "********", masked["cpf"])
	assert.Equal(t, "********", masked["nome_mae"])
	assert.Equal(t, "********", masked["nome_pai"])
	assert.Equal(t, "********", masked["telefone"])

	// Non-sensitive fields should be preserved
	assert.Equal(t, "José Silva", masked["nome"])
	assert.Equal(t, 30, masked["idade"])
	assert.Equal(t, "jose@example.com", masked["email"])
}

func TestMaskSensitiveData_EmptyMap(t *testing.T) {
	data := map[string]interface{}{}

	masked := MaskSensitiveData(data)

	assert.NotNil(t, masked)
	assert.Len(t, masked, 0)
}

func TestMaskSensitiveData_OnlySensitiveFields(t *testing.T) {
	data := map[string]interface{}{
		"cpf":      "12345678901",
		"nome_mae": "Maria",
	}

	masked := MaskSensitiveData(data)

	assert.Len(t, masked, 2)
	assert.Equal(t, "********", masked["cpf"])
	assert.Equal(t, "********", masked["nome_mae"])
}

func TestMaskSensitiveData_OnlyNonSensitiveFields(t *testing.T) {
	data := map[string]interface{}{
		"nome":  "José",
		"idade": 25,
		"email": "jose@example.com",
	}

	masked := MaskSensitiveData(data)

	assert.Len(t, masked, 3)
	assert.Equal(t, "José", masked["nome"])
	assert.Equal(t, 25, masked["idade"])
	assert.Equal(t, "jose@example.com", masked["email"])
}

func TestMaskSensitiveData_MixedTypes(t *testing.T) {
	data := map[string]interface{}{
		"cpf":      "12345678901",
		"idade":    30,
		"ativo":    true,
		"saldo":    1500.50,
		"telefone": "21987654321",
	}

	masked := MaskSensitiveData(data)

	// Sensitive string fields should be masked
	assert.Equal(t, "********", masked["cpf"])
	assert.Equal(t, "********", masked["telefone"])

	// Non-sensitive fields should preserve types
	assert.Equal(t, 30, masked["idade"])
	assert.Equal(t, true, masked["ativo"])
	assert.Equal(t, 1500.50, masked["saldo"])
}

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "orange"}

	assert.True(t, contains(slice, "apple"))
	assert.True(t, contains(slice, "banana"))
	assert.True(t, contains(slice, "orange"))
	assert.False(t, contains(slice, "grape"))
	assert.False(t, contains(slice, ""))
}

func TestContains_EmptySlice(t *testing.T) {
	slice := []string{}

	assert.False(t, contains(slice, "anything"))
	assert.False(t, contains(slice, ""))
}

func TestContains_EmptyString(t *testing.T) {
	slice := []string{"a", "", "b"}

	assert.True(t, contains(slice, ""))
	assert.True(t, contains(slice, "a"))
	assert.True(t, contains(slice, "b"))
}
