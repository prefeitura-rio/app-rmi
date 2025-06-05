package observability

import (
	"github.com/prefeitura-rio/app-rmi/internal/logging"
)

// Logger returns the global safe logger instance
func Logger() *logging.SafeLogger {
	return logging.Logger
}

// MaskCPF masks a CPF number for logging
func MaskCPF(cpf string) string {
	if len(cpf) != 11 {
		return "***.***.***-**"
	}
	return cpf[:3] + ".***" + "." + cpf[6:9] + "-**"
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