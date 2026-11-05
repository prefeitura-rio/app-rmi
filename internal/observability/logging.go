package observability

import (
	"strings"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// Logger returns the global safe logger instance
func Logger() *logging.SafeLogger {
	return logging.GetLogger()
}

// MaskCPF masks a CPF for logs and traces (LGPD).
// Accepts digits-only or formatted input (e.g. "123.456.789-01").
// Valid 11-digit CPF → "123.***.789-**"; invalid → "***.***.***-**".
func MaskCPF(cpf string) string {
	digits := make([]byte, 0, 11)
	for i := 0; i < len(cpf); i++ {
		if cpf[i] >= '0' && cpf[i] <= '9' {
			digits = append(digits, cpf[i])
		}
	}
	if len(digits) != 11 {
		return "***.***.***-**"
	}
	s := string(digits)
	return s[:3] + ".***." + s[6:9] + "-**"
}

// CPFField returns a zap field with a masked CPF (use instead of zap.String("cpf", ...)).
func CPFField(cpf string) zap.Field {
	return zap.String("cpf", MaskCPF(cpf))
}

// CPFAttr returns an OpenTelemetry attribute with a masked CPF
// (use instead of attribute.String("cpf", ...)).
func CPFAttr(cpf string) attribute.KeyValue {
	return attribute.String("cpf", MaskCPF(cpf))
}

// MaskPhone masks a phone number for logs and traces (LGPD), keeping only the
// DDI, the DDD and the last 4 digits. Non-digit characters are ignored.
//   - Brazilian with DDI (12-13 digits starting with 55): "5521987654321" → "+55 21 *****4321"
//   - Brazilian without DDI (10-11 digits, no leading "+"): "21987654321" → "21 *****4321"
//   - other lengths with at least 8 digits (unknown DDI/DDD): only the last 4 digits are kept
//   - fewer than 8 digits: "****"
//   - empty input: ""
func MaskPhone(phone string) string {
	if phone == "" {
		return ""
	}
	digits := make([]byte, 0, len(phone))
	for i := 0; i < len(phone); i++ {
		if phone[i] >= '0' && phone[i] <= '9' {
			digits = append(digits, phone[i])
		}
	}
	n := len(digits)
	if n < 8 {
		return "****"
	}
	s := string(digits)
	last4 := s[n-4:]
	hasDDI := strings.HasPrefix(strings.TrimSpace(phone), "+")
	switch {
	case (n == 12 || n == 13) && s[:2] == "55":
		return "+55 " + s[2:4] + " " + strings.Repeat("*", n-8) + last4
	case !hasDDI && (n == 10 || n == 11):
		return s[:2] + " " + strings.Repeat("*", n-6) + last4
	default:
		return strings.Repeat("*", n-4) + last4
	}
}

// PhoneField returns a zap field with a masked phone number
// (use instead of zap.String("phone_number", ...) or zap.String("phone", ...)).
func PhoneField(phone string) zap.Field {
	return zap.String("phone_number", MaskPhone(phone))
}

// PhonesField returns a zap field with a list of masked phone numbers
// (use instead of zap.Strings("phones", ...)).
func PhonesField(phones []string) zap.Field {
	masked := make([]string, len(phones))
	for i, p := range phones {
		masked[i] = MaskPhone(p)
	}
	return zap.Strings("phones", masked)
}

// PhoneAttr returns an OpenTelemetry attribute with a masked phone number
// (use instead of attribute.String("phone_number", ...)).
func PhoneAttr(phone string) attribute.KeyValue {
	return attribute.String("phone_number", MaskPhone(phone))
}

// MaskName masks a person's name for privacy, keeping the first name and the
// initial of every other name (e.g. "João Silva Santos" → "João S**** Santos").
// utils.MaskName delegates to this function.
func MaskName(fullName string) string {
	if fullName == "" {
		return ""
	}

	parts := strings.Fields(strings.TrimSpace(fullName))
	if len(parts) == 0 {
		return ""
	}

	if len(parts) == 1 {
		// Single name - mask all but first character
		name := parts[0]
		if len(name) <= 1 {
			return name
		}
		return name[:1] + strings.Repeat("*", len(name)-1)
	}

	if len(parts) == 2 {
		// Two names - mask last name
		firstName := parts[0]
		lastName := parts[1]
		if len(lastName) <= 1 {
			return firstName + " " + lastName
		}
		return firstName + " " + lastName[:1] + strings.Repeat("*", len(lastName)-1)
	}

	// Three or more names - mask middle names
	firstName := parts[0]
	lastName := parts[len(parts)-1]

	middleMask := ""
	for i := 1; i < len(parts)-1; i++ {
		if len(parts[i]) > 0 {
			middleMask += parts[i][:1] + strings.Repeat("*", len(parts[i])-1) + " "
		}
	}
	middleMask = strings.TrimSpace(middleMask)

	return firstName + " " + middleMask + " " + lastName
}

// NameField returns a zap field with a masked person name
// (use instead of zap.String("name", ...) or zap.String("nome", ...)).
func NameField(name string) zap.Field {
	return zap.String("name", MaskName(name))
}

// MaskSensitiveData masks sensitive data in a map
func MaskSensitiveData(data map[string]interface{}) map[string]interface{} {
	sensitiveFields := []string{"nome_mae", "nome_pai", "cpf", "telefone"}
	masked := make(map[string]interface{})

	for k, v := range data {
		if contains(sensitiveFields, k) {
			masked[k] = "********"
		} else {
			masked[k] = v
		}
	}

	return masked
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
