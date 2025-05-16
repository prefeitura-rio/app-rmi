package observability

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Logger is the global logger instance
	Logger *zap.Logger
)

// InitLogger initializes the global logger
func InitLogger() error {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Set log level from environment
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel != "" {
		var level zapcore.Level
		if err := level.UnmarshalText([]byte(logLevel)); err == nil {
			config.Level = zap.NewAtomicLevelAt(level)
		}
	}

	// Create logger
	var err error
	Logger, err = config.Build(
		zap.AddCallerSkip(1),
		zap.Fields(
			zap.String("service", "app-rmi"),
			zap.String("version", "v1"),
		),
	)
	if err != nil {
		return err
	}

	return nil
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