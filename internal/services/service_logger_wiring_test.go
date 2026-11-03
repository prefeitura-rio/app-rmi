package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// syntheticWiringCPF is a synthetic CPF with valid check digits (never a real citizen).
const syntheticWiringCPF = "12345678909"

// withGlobalLogger swaps the process-wide logger the way logging.InitLogger does in cmd/api and cmd/sync.
func withGlobalLogger(t *testing.T, logger *logging.SafeLogger) {
	t.Helper()
	previous := logging.Logger
	logging.Logger = logger
	t.Cleanup(func() { logging.Logger = previous })
}

// assertValueNotLogged fails when value shows up in any message or field of the captured entries.
func assertValueNotLogged(t *testing.T, logs *observer.ObservedLogs, value string) {
	t.Helper()
	for _, entry := range logs.All() {
		assert.NotContains(t, entry.Message, value, "value in log message %q", entry.Message)
		for key, field := range entry.ContextMap() {
			assert.NotContains(t, fmt.Sprint(field), value, "value in field %q of log %q", key, entry.Message)
		}
	}
}

func TestInitPetService_WiresGlobalLogger(t *testing.T) {
	previous := PetServiceInstance
	t.Cleanup(func() { PetServiceInstance = previous })

	logger, logs := observedLogger(zapcore.DebugLevel)
	withGlobalLogger(t, logger)

	InitPetService()
	require.NotNil(t, PetServiceInstance)

	assert.Equal(t, "pet_service", requireLogEntry(t, logs, "pet service initialized successfully").LoggerName)

	PetServiceInstance.logger.Error("pet service logger probe")
	entry := requireLogEntry(t, logs, "pet service logger probe")
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	assert.Equal(t, "pet_service", entry.LoggerName)
}

func TestInitLegalEntityService_WiresGlobalLogger(t *testing.T) {
	previous := LegalEntityServiceInstance
	t.Cleanup(func() { LegalEntityServiceInstance = previous })

	logger, logs := observedLogger(zapcore.DebugLevel)
	withGlobalLogger(t, logger)

	InitLegalEntityService()
	require.NotNil(t, LegalEntityServiceInstance)

	assert.Equal(t, "legal_entity_service", requireLogEntry(t, logs, "legal entity service initialized successfully").LoggerName)

	LegalEntityServiceInstance.logger.Error("legal entity service logger probe")
	entry := requireLogEntry(t, logs, "legal entity service logger probe")
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	assert.Equal(t, "legal_entity_service", entry.LoggerName)
}

func TestInitDepartmentService_WiresGlobalLogger(t *testing.T) {
	previous := DepartmentServiceInstance
	t.Cleanup(func() { DepartmentServiceInstance = previous })

	logger, logs := observedLogger(zapcore.DebugLevel)
	withGlobalLogger(t, logger)

	InitDepartmentService()
	require.NotNil(t, DepartmentServiceInstance)

	if config.Redis != nil {
		assert.Equal(t, "department_service", requireLogEntry(t, logs, "department service initialized successfully with DataManager caching").LoggerName)
		require.NotNil(t, DepartmentServiceInstance.dataManager)
		DepartmentServiceInstance.dataManager.logger.Warn("department data manager logger probe")
		assert.Equal(t, "department_service", requireLogEntry(t, logs, "department data manager logger probe").LoggerName)
	} else {
		entry := requireLogEntry(t, logs, "Redis not available, department service will operate without caching")
		assert.Equal(t, zapcore.WarnLevel, entry.Level)
		assert.Equal(t, "department_service", entry.LoggerName)
	}

	DepartmentServiceInstance.logger.Error("department service logger probe")
	entry := requireLogEntry(t, logs, "department service logger probe")
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	assert.Equal(t, "department_service", entry.LoggerName)
}

func TestInitPetService_GetPetsByCPFLogsMaskedCPF(t *testing.T) {
	if config.MongoDB == nil || config.AppConfig == nil {
		t.Skip("Skipping: MongoDB not initialized")
	}

	previousInstance := PetServiceInstance
	previousCurated := config.AppConfig.PetCollection
	previousSelf := config.AppConfig.PetsSelfRegisteredCollection
	config.AppConfig.PetCollection = "test_logger_wiring_pets"
	config.AppConfig.PetsSelfRegisteredCollection = "test_logger_wiring_pets_self"
	t.Cleanup(func() {
		ctx := context.Background()
		_ = config.MongoDB.Collection(config.AppConfig.PetCollection).Drop(ctx)
		_ = config.MongoDB.Collection(config.AppConfig.PetsSelfRegisteredCollection).Drop(ctx)
		config.AppConfig.PetCollection = previousCurated
		config.AppConfig.PetsSelfRegisteredCollection = previousSelf
		PetServiceInstance = previousInstance
	})

	logger, logs := observedLogger(zapcore.DebugLevel)
	withGlobalLogger(t, logger)

	InitPetService()
	_, err := PetServiceInstance.GetPetsByCPF(context.Background(), syntheticWiringCPF, 1, 10)
	require.NoError(t, err)

	entry := requireLogEntry(t, logs, "retrieved merged pets for CPF")
	assert.Equal(t, "pet_service", entry.LoggerName)
	assert.Equal(t, observability.MaskCPF(syntheticWiringCPF), entry.ContextMap()["cpf"])
	assertValueNotLogged(t, logs, syntheticWiringCPF)
}

func TestInitLegalEntityService_GetLegalEntitiesByCPFLogsMaskedCPF(t *testing.T) {
	if config.MongoDB == nil || config.AppConfig == nil {
		t.Skip("Skipping: MongoDB not initialized")
	}

	previousInstance := LegalEntityServiceInstance
	previousCollection := config.AppConfig.LegalEntityCollection
	config.AppConfig.LegalEntityCollection = "test_logger_wiring_legal_entities"
	t.Cleanup(func() {
		_ = config.MongoDB.Collection(config.AppConfig.LegalEntityCollection).Drop(context.Background())
		config.AppConfig.LegalEntityCollection = previousCollection
		LegalEntityServiceInstance = previousInstance
	})

	logger, logs := observedLogger(zapcore.DebugLevel)
	withGlobalLogger(t, logger)

	InitLegalEntityService()
	_, err := LegalEntityServiceInstance.GetLegalEntitiesByCPF(context.Background(), syntheticWiringCPF, 1, 10, nil)
	require.NoError(t, err)

	entry := requireLogEntry(t, logs, "retrieved legal entities for CPF")
	assert.Equal(t, "legal_entity_service", entry.LoggerName)
	assert.Equal(t, observability.MaskCPF(syntheticWiringCPF), entry.ContextMap()["cpf"])
	assertValueNotLogged(t, logs, syntheticWiringCPF)
}

func TestInitDepartmentService_LogsReadFailure(t *testing.T) {
	if config.MongoDB == nil || config.Redis == nil || config.AppConfig == nil {
		t.Skip("Skipping: MongoDB or Redis not initialized")
	}

	previousInstance := DepartmentServiceInstance
	previousCollection := config.AppConfig.DepartmentCollection
	config.AppConfig.DepartmentCollection = "test_logger_wiring_departments"
	t.Cleanup(func() {
		config.AppConfig.DepartmentCollection = previousCollection
		DepartmentServiceInstance = previousInstance
	})

	logger, logs := observedLogger(zapcore.InfoLevel)
	withGlobalLogger(t, logger)

	InitDepartmentService()
	require.NotNil(t, DepartmentServiceInstance.dataManager)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DepartmentServiceInstance.GetDepartmentByID(ctx, "logger-wiring-1000")
	require.Error(t, err)

	entry := requireLogEntry(t, logs, "failed to get department by ID")
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	assert.Equal(t, "department_service", entry.LoggerName)
	assert.Equal(t, "logger-wiring-1000", entry.ContextMap()["cd_ua"])
}
