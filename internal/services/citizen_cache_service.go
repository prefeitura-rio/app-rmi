package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.uber.org/zap"
)

// CitizenCacheService provides cached access to citizen data
type CitizenCacheService struct {
	dataManager *DataManager
	logger      *logging.SafeLogger
}

// NewCitizenCacheService creates a new citizen cache service
func NewCitizenCacheService() *CitizenCacheService {
	dataManager := NewDataManager(
		config.Redis,
		config.MongoDB,
		logging.Logger,
	)

	return &CitizenCacheService{
		dataManager: dataManager,
		logger:      logging.Logger,
	}
}

// GetCitizen retrieves citizen data from cache or MongoDB
func (s *CitizenCacheService) GetCitizen(ctx context.Context, cpf string) (*models.Citizen, error) {
	var citizen models.Citizen

	err := s.dataManager.Read(ctx, cpf, config.AppConfig.CitizenCollection, "citizen", &citizen)
	if err != nil {
		return nil, fmt.Errorf("failed to get citizen: %w", err)
	}

	return &citizen, nil
}

// UpdateCitizen updates citizen data in cache and queues for MongoDB sync
func (s *CitizenCacheService) UpdateCitizen(ctx context.Context, cpf string, citizen *models.Citizen) error {
	// Check if there's already a pending write
	existingKey := fmt.Sprintf("citizen:write:%s", cpf)
	existingData, err := s.dataManager.redis.Get(ctx, existingKey).Result()

	if err == nil {
		// There's a pending write - log the overwrite
		s.logger.Info("citizen update overwrite detected",
			zap.String("cpf", cpf),
			zap.String("existing_data", existingData))

		// TODO: Add audit logging here
	}

	// Create data operation
	op := &CitizenDataOperation{
		CPF:  cpf,
		Data: citizen,
	}

	// Write to cache and queue for sync
	return s.dataManager.Write(ctx, op)
}

// DeleteCitizen removes citizen data from all cache layers and MongoDB
func (s *CitizenCacheService) DeleteCitizen(ctx context.Context, cpf string) error {
	return s.dataManager.Delete(ctx, cpf, config.AppConfig.CitizenCollection, "citizen")
}

// GetCitizenFromCacheOnly retrieves citizen data only from cache (no MongoDB fallback)
func (s *CitizenCacheService) GetCitizenFromCacheOnly(ctx context.Context, cpf string) (*models.Citizen, error) {
	// Check Redis write buffer first (most recent data)
	writeKey := fmt.Sprintf("citizen:write:%s", cpf)
	if data, err := s.dataManager.redis.Get(ctx, writeKey).Result(); err == nil {
		var citizen models.Citizen
		if err := json.Unmarshal([]byte(data), &citizen); err == nil {
			return &citizen, nil
		}
	}

	// Check Redis read cache
	cacheKey := fmt.Sprintf("citizen:cache:%s", cpf)
	if data, err := s.dataManager.redis.Get(ctx, cacheKey).Result(); err == nil {
		var citizen models.Citizen
		if err := json.Unmarshal([]byte(data), &citizen); err == nil {
			return &citizen, nil
		}
	}

	return nil, fmt.Errorf("citizen not found in cache")
}

// IsCitizenInCache checks if citizen data exists in any cache layer
func (s *CitizenCacheService) IsCitizenInCache(ctx context.Context, cpf string) bool {
	// Check write buffer
	writeKey := fmt.Sprintf("citizen:write:%s", cpf)
	if _, err := s.dataManager.redis.Get(ctx, writeKey).Result(); err == nil {
		return true
	}

	// Check read cache
	cacheKey := fmt.Sprintf("citizen:cache:%s", cpf)
	if _, err := s.dataManager.redis.Get(ctx, cacheKey).Result(); err == nil {
		return true
	}

	return false
}
