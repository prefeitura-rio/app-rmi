package services

import (
	"context"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const svcTestCPF = "98765432100"
const svcTestCdUA = "SVC-UA-001"

func setupCPFSecretariaServiceTest(t *testing.T) (*CPFSecretariaService, func()) {
	t.Helper()
	setupTestEnvironment()

	if config.MongoDB == nil {
		t.Skip("Skipping CPFSecretaria service tests: MongoDB not initialized")
	}

	_ = logging.InitLogger()

	if config.AppConfig == nil {
		config.AppConfig = &config.Config{}
	}

	InitCPFSecretariaService()
	require.NotNil(t, CPFSecretariaServiceInstance)

	svc := CPFSecretariaServiceInstance

	cleanup := func() {
		ctx := context.Background()
		coll := config.MongoDB.Collection(config.AppConfig.CPFSecretariaCollection)
		_, _ = coll.DeleteMany(ctx, map[string]interface{}{"cpf": svcTestCPF})
	}
	cleanup()

	return svc, cleanup
}

func TestCPFSecretariaService_ListByCPF_Empty(t *testing.T) {
	svc, cleanup := setupCPFSecretariaServiceTest(t)
	defer cleanup()

	mappings, err := svc.ListByCPF(context.Background(), svcTestCPF)
	require.NoError(t, err)
	assert.Empty(t, mappings)
}

func TestCPFSecretariaService_AddMapping_Success(t *testing.T) {
	svc, cleanup := setupCPFSecretariaServiceTest(t)
	defer cleanup()

	mapping, err := svc.AddMapping(context.Background(), svcTestCPF, svcTestCdUA, "test-admin")
	require.NoError(t, err)
	assert.Equal(t, svcTestCPF, mapping.CPF)
	assert.Equal(t, svcTestCdUA, mapping.CdUA)
	assert.Equal(t, "test-admin", mapping.CreatedBy)
	assert.NotEmpty(t, mapping.ID)
}

func TestCPFSecretariaService_AddMapping_Duplicate(t *testing.T) {
	svc, cleanup := setupCPFSecretariaServiceTest(t)
	defer cleanup()

	_, err := svc.AddMapping(context.Background(), svcTestCPF, svcTestCdUA, "admin")
	require.NoError(t, err)

	_, err = svc.AddMapping(context.Background(), svcTestCPF, svcTestCdUA, "admin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCPFSecretariaService_RemoveMapping_Success(t *testing.T) {
	svc, cleanup := setupCPFSecretariaServiceTest(t)
	defer cleanup()

	_, err := svc.AddMapping(context.Background(), svcTestCPF, svcTestCdUA, "admin")
	require.NoError(t, err)

	err = svc.RemoveMapping(context.Background(), svcTestCPF, svcTestCdUA)
	require.NoError(t, err)

	mappings, err := svc.ListByCPF(context.Background(), svcTestCPF)
	require.NoError(t, err)
	assert.Empty(t, mappings)
}

func TestCPFSecretariaService_RemoveMapping_NotFound(t *testing.T) {
	svc, cleanup := setupCPFSecretariaServiceTest(t)
	defer cleanup()

	err := svc.RemoveMapping(context.Background(), svcTestCPF, "NONEXISTENT")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestCPFSecretariaService_GetCdUAsByCPF(t *testing.T) {
	svc, cleanup := setupCPFSecretariaServiceTest(t)
	defer cleanup()

	_, err := svc.AddMapping(context.Background(), svcTestCPF, svcTestCdUA, "admin")
	require.NoError(t, err)
	_, err = svc.AddMapping(context.Background(), svcTestCPF, "SVC-UA-002", "admin")
	require.NoError(t, err)

	cdUAs, err := svc.GetCdUAsByCPF(context.Background(), svcTestCPF)
	require.NoError(t, err)
	assert.Len(t, cdUAs, 2)
	assert.Contains(t, cdUAs, svcTestCdUA)
	assert.Contains(t, cdUAs, "SVC-UA-002")
}

func TestCPFSecretariaService_ToResponse(t *testing.T) {
	svc, cleanup := setupCPFSecretariaServiceTest(t)
	defer cleanup()

	mapping, err := svc.AddMapping(context.Background(), svcTestCPF, svcTestCdUA, "admin")
	require.NoError(t, err)

	resp := mapping.ToResponse()
	assert.Equal(t, svcTestCPF, resp.CPF)
	assert.Equal(t, svcTestCdUA, resp.CdUA)
	assert.NotEmpty(t, resp.ID)
}

func TestNormalizeCPFVariants(t *testing.T) {
	svc, cleanup := setupCPFSecretariaServiceTest(t)
	defer cleanup()

	formattedCPF := "987.654.321-00"
	_, err := svc.AddMapping(context.Background(), formattedCPF, svcTestCdUA, "admin")
	require.NoError(t, err)

	mappings, err := svc.ListByCPF(context.Background(), svcTestCPF)
	require.NoError(t, err)
	assert.Len(t, mappings, 1)
	assert.Equal(t, svcTestCPF, mappings[0].CPF)
}
