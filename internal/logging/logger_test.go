package logging

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInitLogger(t *testing.T) {
	err := InitLogger()
	require.NoError(t, err)
	assert.NotNil(t, Logger)
	assert.NotNil(t, Logger.logger)
}

func TestInitLogger_WithLogLevel(t *testing.T) {
	// Set log level environment variable
	os.Setenv("LOG_LEVEL", "debug")
	defer os.Unsetenv("LOG_LEVEL")

	err := InitLogger()
	require.NoError(t, err)
	assert.NotNil(t, Logger)
}

func TestInitLogger_WithInvalidLogLevel(t *testing.T) {
	// Set invalid log level - should still succeed with default
	os.Setenv("LOG_LEVEL", "invalid")
	defer os.Unsetenv("LOG_LEVEL")

	err := InitLogger()
	require.NoError(t, err)
	assert.NotNil(t, Logger)
}

func TestSafeLogger_Info(t *testing.T) {
	logger := &SafeLogger{logger: zap.NewNop()}

	// Should not panic
	logger.Info("test message")
	logger.Info("test with fields", zap.String("key", "value"))
}

func TestSafeLogger_Warn(t *testing.T) {
	logger := &SafeLogger{logger: zap.NewNop()}

	// Should not panic
	logger.Warn("test warning")
	logger.Warn("test warning with fields", zap.Int("count", 42))
}

func TestSafeLogger_Debug(t *testing.T) {
	logger := &SafeLogger{logger: zap.NewNop()}

	// Should not panic
	logger.Debug("test debug")
	logger.Debug("test debug with fields", zap.Bool("flag", true))
}

func TestSafeLogger_Error(t *testing.T) {
	logger := &SafeLogger{logger: zap.NewNop()}

	// Should not panic
	logger.Error("test error")
	logger.Error("test error with fields", zap.String("error", "something went wrong"))
}

func TestSafeLogger_NilLogger(t *testing.T) {
	logger := &SafeLogger{logger: nil}

	// All methods should be safe to call with nil logger
	logger.Info("test")
	logger.Warn("test")
	logger.Debug("test")
	logger.Error("test")
}

func TestSafeLogger_NilSafeLogger(t *testing.T) {
	var logger *SafeLogger = nil

	// Should not panic even with nil SafeLogger
	logger.Info("test")
	logger.Warn("test")
	logger.Debug("test")
	logger.Error("test")
}

func TestSafeLogger_With(t *testing.T) {
	logger := &SafeLogger{logger: zap.NewNop()}

	newLogger := logger.With(zap.String("key", "value"), zap.Int("count", 42))

	require.NotNil(t, newLogger)
	assert.NotNil(t, newLogger.logger)

	// Should be able to use the new logger
	newLogger.Info("test message")
}

func TestSafeLogger_With_NilLogger(t *testing.T) {
	logger := &SafeLogger{logger: nil}

	newLogger := logger.With(zap.String("key", "value"))

	assert.Equal(t, logger, newLogger)
}

func TestSafeLogger_With_NilSafeLogger(t *testing.T) {
	var logger *SafeLogger = nil

	newLogger := logger.With(zap.String("key", "value"))

	assert.Nil(t, newLogger)
}

func TestSafeLogger_Unwrap(t *testing.T) {
	zapLogger := zap.NewNop()
	logger := &SafeLogger{logger: zapLogger}

	unwrapped := logger.Unwrap()

	assert.NotNil(t, unwrapped)
	assert.Equal(t, zapLogger, unwrapped)
}

func TestSafeLogger_Unwrap_NilLogger(t *testing.T) {
	logger := &SafeLogger{logger: nil}

	unwrapped := logger.Unwrap()

	assert.NotNil(t, unwrapped)
	// Should return a new nop logger
}

func TestSafeLogger_Unwrap_NilSafeLogger(t *testing.T) {
	var logger *SafeLogger = nil

	unwrapped := logger.Unwrap()

	assert.NotNil(t, unwrapped)
	// Should return a new nop logger
}

func TestGlobalLogger(t *testing.T) {
	// Global logger should be initialized with nop logger
	assert.NotNil(t, Logger)
	assert.NotNil(t, Logger.logger)

	// Should be safe to use immediately
	Logger.Info("test message")
}

func TestSafeLogger_MultipleFields(t *testing.T) {
	logger := &SafeLogger{logger: zap.NewNop()}

	logger.Info("test",
		zap.String("field1", "value1"),
		zap.Int("field2", 42),
		zap.Bool("field3", true),
		zap.Float64("field4", 3.14),
	)

	logger.With(
		zap.String("context1", "value1"),
		zap.String("context2", "value2"),
	).Info("test with context")
}

func TestSafeLogger_ChainedWith(t *testing.T) {
	logger := &SafeLogger{logger: zap.NewNop()}

	logger1 := logger.With(zap.String("key1", "value1"))
	logger2 := logger1.With(zap.String("key2", "value2"))
	logger3 := logger2.With(zap.String("key3", "value3"))

	require.NotNil(t, logger1)
	require.NotNil(t, logger2)
	require.NotNil(t, logger3)

	logger3.Info("test with chained context")
}

func TestInitLogger_MultipleInit(t *testing.T) {
	// Should be safe to call InitLogger multiple times
	err1 := InitLogger()
	require.NoError(t, err1)

	err2 := InitLogger()
	require.NoError(t, err2)

	assert.NotNil(t, Logger)
}
