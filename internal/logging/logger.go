package logging

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// SafeLogger wraps a zap.Logger and always provides a valid logger
// If the underlying logger is nil, it uses a no-op logger
// All logging methods are safe to call even if not initialized

type SafeLogger struct {
	logger *zap.Logger
}

// global instance
var Logger = &SafeLogger{logger: zap.NewNop()}

// InitLogger initializes the global logger safely
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
	zlogger, err := config.Build(
		zap.AddCallerSkip(1),
		zap.Fields(
			zap.String("service", "app-rmi"),
			zap.String("version", "v1"),
		),
	)
	if err != nil {
		return err
	}

	Logger = &SafeLogger{logger: zlogger}
	return nil
}

// Info logs an info message
func (l *SafeLogger) Info(msg string, fields ...zap.Field) {
	if l != nil && l.logger != nil {
		l.logger.Info(msg, fields...)
	}
}

// Warn logs a warning message
func (l *SafeLogger) Warn(msg string, fields ...zap.Field) {
	if l != nil && l.logger != nil {
		l.logger.Warn(msg, fields...)
	}
}

// Error logs an error message
func (l *SafeLogger) Error(msg string, fields ...zap.Field) {
	if l != nil && l.logger != nil {
		l.logger.Error(msg, fields...)
	}
}

// Fatal logs a fatal message and exits
func (l *SafeLogger) Fatal(msg string, fields ...zap.Field) {
	if l != nil && l.logger != nil {
		l.logger.Fatal(msg, fields...)
	}
}

// With returns a new SafeLogger with additional fields
func (l *SafeLogger) With(fields ...zap.Field) *SafeLogger {
	if l != nil && l.logger != nil {
		return &SafeLogger{logger: l.logger.With(fields...)}
	}
	return l
}
