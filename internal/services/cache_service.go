package services

import (
	"context"
	"fmt"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
)

// CacheService provides a unified interface for all cache operations
type CacheService struct {
	citizenService *CitizenCacheService
	logger         *logging.SafeLogger
}

// NewCacheService creates a new unified cache service
func NewCacheService() *CacheService {
	return &CacheService{
		citizenService: NewCitizenCacheService(),
		logger:         logging.Logger,
	}
}

// UpdateSelfDeclaredAddress updates self-declared address via cache system
func (s *CacheService) UpdateSelfDeclaredAddress(ctx context.Context, cpf string, endereco *models.Endereco) error {
	// Create a citizen update operation
	op := &SelfDeclaredAddressDataOperation{
		CPF:       cpf,
		Endereco:  endereco,
		UpdatedAt: time.Now(),
	}

	// Use the data manager to write to cache and queue for sync
	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateSelfDeclaredEmail updates self-declared email via cache system
func (s *CacheService) UpdateSelfDeclaredEmail(ctx context.Context, cpf string, email *models.Email) error {
	op := &SelfDeclaredEmailDataOperation{
		CPF:       cpf,
		Email:     email,
		UpdatedAt: time.Now(),
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateSelfDeclaredPhone updates self-declared phone via cache system
func (s *CacheService) UpdateSelfDeclaredPhone(ctx context.Context, cpf string, telefone *models.Telefone) error {
	op := &SelfDeclaredPhoneDataOperation{
		CPF:       cpf,
		Telefone:  telefone,
		UpdatedAt: time.Now(),
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateSelfDeclaredRaca updates self-declared ethnicity via cache system
func (s *CacheService) UpdateSelfDeclaredRaca(ctx context.Context, cpf string, raca string) error {
	op := &SelfDeclaredRacaDataOperation{
		CPF:       cpf,
		Raca:      raca,
		UpdatedAt: time.Now(),
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateSelfDeclaredNomeExibicao updates self-declared exhibition name via cache system
func (s *CacheService) UpdateSelfDeclaredNomeExibicao(ctx context.Context, cpf string, nomeExibicao string) error {
	op := &SelfDeclaredNomeExibicaoDataOperation{
		CPF:          cpf,
		NomeExibicao: nomeExibicao,
		UpdatedAt:    time.Now(),
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateSelfDeclaredGenero updates self-declared gender via cache system
func (s *CacheService) UpdateSelfDeclaredGenero(ctx context.Context, cpf string, genero string) error {
	op := &SelfDeclaredGeneroDataOperation{
		CPF:       cpf,
		Genero:    genero,
		UpdatedAt: time.Now(),
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateSelfDeclaredRendaFamiliar updates self-declared family income via cache system
func (s *CacheService) UpdateSelfDeclaredRendaFamiliar(ctx context.Context, cpf string, rendaFamiliar string) error {
	op := &SelfDeclaredRendaFamiliarDataOperation{
		CPF:           cpf,
		RendaFamiliar: rendaFamiliar,
		UpdatedAt:     time.Now(),
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateSelfDeclaredEscolaridade updates self-declared education level via cache system
func (s *CacheService) UpdateSelfDeclaredEscolaridade(ctx context.Context, cpf string, escolaridade string) error {
	op := &SelfDeclaredEscolaridadeDataOperation{
		CPF:          cpf,
		Escolaridade: escolaridade,
		UpdatedAt:    time.Now(),
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateSelfDeclaredDeficiencia updates self-declared disability via cache system
func (s *CacheService) UpdateSelfDeclaredDeficiencia(ctx context.Context, cpf string, deficiencia string) error {
	op := &SelfDeclaredDeficienciaDataOperation{
		CPF:         cpf,
		Deficiencia: deficiencia,
		UpdatedAt:   time.Now(),
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateUserConfig updates user configuration via cache system
func (s *CacheService) UpdateUserConfig(ctx context.Context, userID string, userConfig *models.UserConfig) error {
	op := &UserConfigDataOperation{
		UserID: userID,
		Data:   userConfig,
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateOptInHistory updates opt-in history via cache system
func (s *CacheService) UpdateOptInHistory(ctx context.Context, id string, optInHistory *models.OptInHistory) error {
	op := &OptInHistoryDataOperation{
		ID:   id,
		Data: optInHistory,
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdatePhoneMapping updates phone mapping via cache system
func (s *CacheService) UpdatePhoneMapping(ctx context.Context, phone string, phoneMapping *models.PhoneCPFMapping) error {
	op := &PhoneMappingDataOperation{
		PhoneNumber: phone,
		Data:        phoneMapping,
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateBetaGroup updates beta group via cache system
func (s *CacheService) UpdateBetaGroup(ctx context.Context, id string, betaGroup *models.BetaGroup) error {
	op := &BetaGroupDataOperation{
		ID:   id,
		Data: betaGroup,
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdatePhoneVerification updates phone verification via cache system
func (s *CacheService) UpdatePhoneVerification(ctx context.Context, phone string, verification *models.PhoneVerification) error {
	op := &PhoneVerificationDataOperation{
		PhoneNumber: phone,
		Data:        verification,
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// UpdateMaintenanceRequest updates maintenance request via cache system
func (s *CacheService) UpdateMaintenanceRequest(ctx context.Context, id string, request *models.MaintenanceRequest) error {
	op := &MaintenanceRequestDataOperation{
		ID:   id,
		Data: request,
	}

	dataManager := NewDataManager(config.Redis, config.MongoDB, s.logger)
	return dataManager.Write(ctx, op)
}

// GetCitizen retrieves citizen data using the cache system
func (s *CacheService) GetCitizen(ctx context.Context, cpf string) (*models.Citizen, error) {
	return s.citizenService.GetCitizen(ctx, cpf)
}

// GetCitizenFromCacheOnly retrieves citizen data only from cache
func (s *CacheService) GetCitizenFromCacheOnly(ctx context.Context, cpf string) (*models.Citizen, error) {
	return s.citizenService.GetCitizenFromCacheOnly(ctx, cpf)
}

// IsCitizenInCache checks if citizen data exists in cache
func (s *CacheService) IsCitizenInCache(ctx context.Context, cpf string) bool {
	return s.citizenService.IsCitizenInCache(ctx, cpf)
}

// DeleteCitizen removes citizen data from all cache layers
func (s *CacheService) DeleteCitizen(ctx context.Context, cpf string) error {
	return s.citizenService.DeleteCitizen(ctx, cpf)
}

// GetQueueDepth returns the current depth of a sync queue
func (s *CacheService) GetQueueDepth(ctx context.Context, queueType string) (int64, error) {
	queueKey := fmt.Sprintf("sync:queue:%s", queueType)
	return config.Redis.LLen(ctx, queueKey).Result()
}

// GetDLQDepth returns the current depth of a dead letter queue
func (s *CacheService) GetDLQDepth(ctx context.Context, queueType string) (int64, error) {
	dlqKey := fmt.Sprintf("sync:dlq:%s", queueType)
	return config.Redis.LLen(ctx, dlqKey).Result()
}

// GetCacheStats returns cache statistics for monitoring
func (s *CacheService) GetCacheStats(ctx context.Context) map[string]interface{} {
	stats := make(map[string]interface{})

	// Check queue depths for all types
	queueTypes := []string{"citizen", "phone_mapping", "user_config", "opt_in_history", "beta_group", "phone_verification", "maintenance_request"}

	for _, queueType := range queueTypes {
		if depth, err := s.GetQueueDepth(ctx, queueType); err == nil {
			stats[fmt.Sprintf("queue_depth_%s", queueType)] = depth
		}

		if dlqDepth, err := s.GetDLQDepth(ctx, queueType); err == nil {
			stats[fmt.Sprintf("dlq_depth_%s", queueType)] = dlqDepth
		}
	}

	// Check write buffer sizes
	for _, queueType := range queueTypes {
		pattern := fmt.Sprintf("%s:write:*", queueType)
		if keys, err := config.Redis.Keys(ctx, pattern).Result(); err == nil {
			stats[fmt.Sprintf("write_buffer_%s", queueType)] = len(keys)
		}
	}

	// Check read cache sizes
	for _, queueType := range queueTypes {
		pattern := fmt.Sprintf("%s:cache:*", queueType)
		if keys, err := config.Redis.Keys(ctx, pattern).Result(); err == nil {
			stats[fmt.Sprintf("read_cache_%s", queueType)] = len(keys)
		}
	}

	return stats
}
