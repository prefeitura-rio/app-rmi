package observability

import (
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
