package services

import (
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPetService(t *testing.T) {
	_ = logging.InitLogger()

	service := NewPetService(nil, logging.Logger)

	require.NotNil(t, service)
	assert.NotNil(t, service.logger)
}

func TestNewPetService_WithNilLogger(t *testing.T) {
	service := NewPetService(nil, nil)

	require.NotNil(t, service)
	assert.Nil(t, service.logger)
}

func TestPetService_Structure(t *testing.T) {
	_ = logging.InitLogger()

	service := &PetService{
		database: nil,
		logger:   logging.Logger,
	}

	assert.Nil(t, service.database)
	assert.NotNil(t, service.logger)
}
