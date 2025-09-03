package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// Global CF lookup service instance
var CFLookupServiceInstance *CFLookupService

// CFLookupService handles CF lookup business logic
type CFLookupService struct {
	database  *mongo.Database
	mcpClient *MCPClient
	logger    *logging.SafeLogger
}

// NewCFLookupService creates a new CF lookup service instance
func NewCFLookupService(database *mongo.Database, mcpClient *MCPClient, logger *logging.SafeLogger) *CFLookupService {
	return &CFLookupService{
		database:  database,
		mcpClient: mcpClient,
		logger:    logger,
	}
}

// InitCFLookupService initializes the global CF lookup service instance
func InitCFLookupService() {
	logger := zap.L().Named("cf_lookup_service")
	
	// Log configuration for debugging
	logger.Info("initializing CF lookup service",
		zap.String("mcp_server_url", config.AppConfig.MCPServerURL),
		zap.Duration("sync_timeout", config.AppConfig.CFLookupSyncTimeout),
		zap.Duration("cache_ttl", config.AppConfig.CFLookupCacheTTL))
	
	// Initialize MCP client with error handling
	mcpClient := NewMCPClient(config.AppConfig, &logging.SafeLogger{})
	if mcpClient == nil {
		logger.Error("failed to initialize MCP client - CF lookup service disabled")
		return
	}
	
	// Test MCP client connectivity
	if config.AppConfig.MCPServerURL == "" {
		logger.Error("MCP_SERVER_URL not configured - CF lookup service disabled")
		return
	}
	
	CFLookupServiceInstance = NewCFLookupService(config.MongoDB, mcpClient, &logging.SafeLogger{})
	logger.Info("CF lookup service initialized successfully")
}

// ShouldLookupCF determines if a CF lookup should be performed for a citizen
func (s *CFLookupService) ShouldLookupCF(ctx context.Context, cpf string, citizenData *models.Citizen) (bool, string, error) {
	startTime := time.Now()
	ctx, span := utils.TraceBusinessLogic(ctx, "cf_lookup_should_lookup")
	defer span.End()
	defer func() {
		s.logger.Debug("CF lookup eligibility check completed",
			zap.String("cpf", cpf),
			zap.Duration("duration", time.Since(startTime)),
			zap.String("operation", "should_lookup_cf"))
	}()

	// Check if citizen already has CF data from base data
	if citizenData.Saude != nil &&
		citizenData.Saude.ClinicaFamilia != nil &&
		citizenData.Saude.ClinicaFamilia.Indicador != nil &&
		*citizenData.Saude.ClinicaFamilia.Indicador {
		s.logger.Debug("citizen already has CF data from base data", zap.String("cpf", cpf))
		return false, "", nil
	}

	// Extract address from citizen data (prioritize self-declared, then base data)
	address := s.ExtractAddress(citizenData)
	if address == "" {
		s.logger.Debug("no address available for CF lookup", zap.String("cpf", cpf))
		return false, "", nil
	}

	// Check if we already have a recent CF lookup for this address
	addressHash := s.GenerateAddressHash(address)
	existingLookup, err := s.getExistingCFLookup(ctx, cpf, addressHash)
	if err != nil {
		s.logger.Warn("failed to check existing CF lookup", zap.Error(err), zap.String("cpf", cpf))
		// Continue with lookup despite error
	}

	// If we have existing data for the same address, use it (no need to re-lookup)
	if existingLookup != nil {
		s.logger.Debug("using existing CF lookup for same address",
			zap.String("cpf", cpf),
			zap.String("address_hash", addressHash))
		return false, address, nil
	}

	// No rate limiting needed for CF lookups

	s.logger.Debug("CF lookup should be performed",
		zap.String("cpf", cpf),
		zap.String("address", address))

	return true, address, nil
}

// PerformCFLookup performs a CF lookup and stores the result
func (s *CFLookupService) PerformCFLookup(ctx context.Context, cpf, address string) error {
	startTime := time.Now()
	ctx, span := utils.TraceBusinessLogic(ctx, "cf_lookup_perform")
	defer span.End()

	// Track operation outcome and duration
	defer func() {
		duration := time.Since(startTime)
		s.logger.Info("CF lookup operation completed",
			zap.String("cpf", cpf),
			zap.Duration("total_duration", duration),
			zap.String("operation", "cf_lookup_complete"))
	}()

	s.logger.Info("performing CF lookup",
		zap.String("cpf", cpf),
		zap.String("address", address),
		zap.String("operation", "cf_lookup_start"),
		zap.String("address_hash", s.GenerateAddressHash(address)))

	// Add timeout for the entire operation
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Call MCP server to find CF with enhanced error handling
	cfInfo, err := s.mcpClient.FindNearestCF(ctx, address)
	if err != nil {
		// Categorize the error for better handling
		errorType := s.categorizeError(err)
		s.logger.Error("MCP CF lookup failed",
			zap.Error(err),
			zap.String("cpf", cpf),
			zap.String("address", address),
			zap.String("error_type", errorType))

		// Return different error messages based on error type
		switch errorType {
		case "timeout":
			return fmt.Errorf("CF lookup timed out for address: %s", address)
		case "network":
			return fmt.Errorf("network error during CF lookup: %w", err)
		case "authorization":
			return fmt.Errorf("authorization failed for CF lookup: %w", err)
		case "validation":
			return fmt.Errorf("address validation failed: %w", err)
		default:
			return fmt.Errorf("CF lookup failed: %w", err)
		}
	}

	if cfInfo == nil {
		s.logger.Info("no CF found for address",
			zap.String("cpf", cpf),
			zap.String("address", address))
		return nil
	}

	// Store CF lookup result
	addressHash := s.GenerateAddressHash(address)
	cfLookup := &models.CFLookup{
		ID:           primitive.NewObjectID(),
		CPF:          cpf,
		AddressHash:  addressHash,
		AddressUsed:  address,
		CFData:       *cfInfo,
		LookupSource: "mcp",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		IsActive:     true,
	}

	// Extract distance if available (from MCP response)
	// Note: Distance is not in the CFInfo struct, would need to be added to MCP response parsing
	cfLookup.DistanceMeters = 0 // TODO: Extract from MCP response

	err = s.storeCFLookup(ctx, cfLookup)
	if err != nil {
		s.logger.Error("failed to store CF lookup result",
			zap.Error(err),
			zap.String("cpf", cpf))
		return fmt.Errorf("failed to store CF lookup: %w", err)
	}

	// Cache the result
	err = s.cacheCFData(ctx, cpf, cfLookup)
	if err != nil {
		s.logger.Warn("failed to cache CF lookup result",
			zap.Error(err),
			zap.String("cpf", cpf))
		// Don't fail the operation for cache errors
	}

	s.logger.Info("CF lookup completed successfully",
		zap.String("cpf", cpf),
		zap.String("cf_name_popular", cfInfo.NomePopular),
		zap.String("cf_name_oficial", cfInfo.NomeOficial),
		zap.String("cf_bairro", cfInfo.Bairro),
		zap.String("cf_logradouro", cfInfo.Logradouro),
		zap.String("operation", "cf_lookup_success"),
		zap.String("address_hash", addressHash),
		zap.Int("distance_meters", cfLookup.DistanceMeters),
		zap.Bool("cf_ativo", cfInfo.Ativo),
		zap.Bool("cf_aberto_publico", cfInfo.AbertoAoPublico))

	return nil
}

// GetCFDataForCitizen retrieves CF data for a citizen (from cache or database)
func (s *CFLookupService) GetCFDataForCitizen(ctx context.Context, cpf string) (*models.CFLookup, error) {
	ctx, span := utils.TraceCacheGet(ctx, fmt.Sprintf("cf_lookup:%s", cpf))
	defer span.End()

	// Try cache first
	cfData, err := s.getCachedCFData(ctx, cpf)
	if err == nil && cfData != nil {
		s.logger.Debug("CF data cache hit", zap.String("cpf", cpf))
		return cfData, nil
	}

	s.logger.Debug("CF data cache miss", zap.String("cpf", cpf))

	// Fallback to database
	cfData, err = s.getActiveCFLookup(ctx, cpf)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get CF data from database: %w", err)
	}

	// Cache for future requests
	if cfData != nil {
		err = s.cacheCFData(ctx, cpf, cfData)
		if err != nil {
			s.logger.Warn("failed to cache CF data", zap.Error(err), zap.String("cpf", cpf))
		}
	}

	return cfData, nil
}

// InvalidateCFDataForAddress invalidates CF data when address changes
func (s *CFLookupService) InvalidateCFDataForAddress(ctx context.Context, cpf, newAddressHash string) error {
	ctx, span := utils.TraceBusinessLogic(ctx, "cf_lookup_invalidate")
	defer span.End()

	// Get current CF lookup to check if address changed
	collection := s.database.Collection(config.AppConfig.CFLookupCollection)
	var currentLookup models.CFLookup
	err := collection.FindOne(ctx, bson.M{"cpf": cpf, "is_active": true}).Decode(&currentLookup)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// No existing CF data, nothing to invalidate
			return nil
		}
		return fmt.Errorf("failed to get current CF data: %w", err)
	}

	// If address hasn't changed, don't invalidate
	if currentLookup.AddressHash == newAddressHash {
		s.logger.Debug("address hash unchanged, keeping CF data",
			zap.String("cpf", cpf),
			zap.String("address_hash", newAddressHash))
		return nil
	}

	// Address changed, delete the CF lookup document
	result, err := collection.DeleteOne(ctx, bson.M{"cpf": cpf})
	if err != nil {
		return fmt.Errorf("failed to delete CF data: %w", err)
	}

	// Invalidate cache
	err = s.invalidateCFCache(ctx, cpf)
	if err != nil {
		s.logger.Warn("failed to invalidate CF cache", zap.Error(err), zap.String("cpf", cpf))
	}

	s.logger.Debug("invalidated CF data for address change",
		zap.String("cpf", cpf),
		zap.String("old_address_hash", currentLookup.AddressHash),
		zap.String("new_address_hash", newAddressHash),
		zap.Int64("deleted_count", result.DeletedCount))

	return nil
}

// GenerateAddressHash creates a hash of the address for tracking changes
func (s *CFLookupService) GenerateAddressHash(address string) string {
	// Normalize address (lowercase, trim spaces)
	normalized := strings.TrimSpace(strings.ToLower(address))

	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// ExtractAddress extracts the best available address from citizen data
func (s *CFLookupService) ExtractAddress(citizenData *models.Citizen) string {
	// Priority 1: Self-declared address (check if address has origem = self-declared)
	if citizenData.Endereco != nil &&
		citizenData.Endereco.Principal != nil &&
		citizenData.Endereco.Principal.Origem != nil &&
		*citizenData.Endereco.Principal.Origem == "self-declared" {
		return s.buildFullAddress(
			citizenData.Endereco.Principal.Logradouro,
			citizenData.Endereco.Principal.Numero,
			citizenData.Endereco.Principal.Complemento,
			citizenData.Endereco.Principal.Bairro,
			citizenData.Endereco.Principal.Municipio,
			citizenData.Endereco.Principal.Estado,
		)
	}

	// Priority 2: Base data address (any address)
	if citizenData.Endereco != nil &&
		citizenData.Endereco.Principal != nil {
		return s.buildFullAddress(
			citizenData.Endereco.Principal.Logradouro,
			citizenData.Endereco.Principal.Numero,
			citizenData.Endereco.Principal.Complemento,
			citizenData.Endereco.Principal.Bairro,
			citizenData.Endereco.Principal.Municipio,
			citizenData.Endereco.Principal.Estado,
		)
	}

	return ""
}

// buildFullAddress builds a complete address string for MCP lookup
func (s *CFLookupService) buildFullAddress(logradouro, numero, complemento, bairro, cidade, estado *string) string {
	if logradouro == nil || *logradouro == "" {
		return ""
	}

	parts := []string{*logradouro}

	if numero != nil && *numero != "" {
		parts = append(parts, *numero)
	}

	if complemento != nil && *complemento != "" {
		parts = append(parts, *complemento)
	}

	if bairro != nil && *bairro != "" {
		parts = append(parts, *bairro)
	}

	if cidade != nil && *cidade != "" {
		parts = append(parts, *cidade)
	} else {
		parts = append(parts, "Rio de Janeiro") // Default city
	}

	if estado != nil && *estado != "" {
		parts = append(parts, *estado)
	} else {
		parts = append(parts, "RJ") // Default state
	}

	return strings.Join(parts, ", ")
}

// categorizeError categorizes errors for better handling and monitoring
func (s *CFLookupService) categorizeError(err error) string {
	errStr := strings.ToLower(err.Error())

	// Timeout errors
	if strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline") ||
		strings.Contains(errStr, "context canceled") {
		return "timeout"
	}

	// Network errors
	if strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "dial") ||
		strings.Contains(errStr, "no such host") {
		return "network"
	}

	// Authorization errors
	if strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "403") {
		return "authorization"
	}

	// Validation errors
	if strings.Contains(errStr, "invalid") ||
		strings.Contains(errStr, "validation") ||
		strings.Contains(errStr, "400") ||
		strings.Contains(errStr, "bad request") {
		return "validation"
	}

	// Server errors
	if strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") {
		return "server"
	}

	return "unknown"
}

// getExistingCFLookup checks for existing CF lookup data
func (s *CFLookupService) getExistingCFLookup(ctx context.Context, cpf, addressHash string) (*models.CFLookup, error) {
	collection := s.database.Collection(config.AppConfig.CFLookupCollection)

	filter := bson.M{
		"cpf":          cpf,
		"address_hash": addressHash,
		"is_active":    true,
	}

	var cfLookup models.CFLookup
	err := collection.FindOne(ctx, filter).Decode(&cfLookup)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return &cfLookup, nil
}

// getActiveCFLookup gets the CF lookup for a citizen (single document per CPF)
func (s *CFLookupService) getActiveCFLookup(ctx context.Context, cpf string) (*models.CFLookup, error) {
	collection := s.database.Collection(config.AppConfig.CFLookupCollection)

	filter := bson.M{
		"cpf":       cpf,
		"is_active": true,
	}

	var cfLookup models.CFLookup
	err := collection.FindOne(ctx, filter).Decode(&cfLookup)
	if err != nil {
		return nil, err
	}

	return &cfLookup, nil
}

// storeCFLookup stores CF lookup result in database (single document per CPF)
func (s *CFLookupService) storeCFLookup(ctx context.Context, cfLookup *models.CFLookup) error {
	ctx, span := utils.TraceDatabaseUpdate(ctx, config.AppConfig.CFLookupCollection, "store_cf_lookup", false)
	defer span.End()

	collection := s.database.Collection(config.AppConfig.CFLookupCollection)

	// Use upsert to replace/create a single document per CPF
	filter := bson.M{"cpf": cfLookup.CPF}

	update := bson.M{
		"$set": bson.M{
			"cpf":             cfLookup.CPF,
			"address_hash":    cfLookup.AddressHash,
			"address_used":    cfLookup.AddressUsed,
			"cf_data":         cfLookup.CFData,
			"distance_meters": cfLookup.DistanceMeters,
			"lookup_source":   cfLookup.LookupSource,
			"updated_at":      time.Now(),
			"is_active":       true,
		},
		"$setOnInsert": bson.M{
			"_id":        cfLookup.ID,
			"created_at": cfLookup.CreatedAt,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to upsert CF lookup: %w", err)
	}

	s.logger.Debug("CF lookup stored successfully",
		zap.String("cpf", cfLookup.CPF),
		zap.String("address_hash", cfLookup.AddressHash))

	return nil
}

// getCachedCFData retrieves CF data from Redis cache
func (s *CFLookupService) getCachedCFData(ctx context.Context, cpf string) (*models.CFLookup, error) {
	cacheKey := fmt.Sprintf("cf_lookup:cpf:%s", cpf)

	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	if err != nil {
		return nil, err
	}

	var cfLookup models.CFLookup
	err = json.Unmarshal([]byte(cached), &cfLookup)
	if err != nil {
		return nil, err
	}

	return &cfLookup, nil
}

// cacheCFData stores CF data in Redis cache
func (s *CFLookupService) cacheCFData(ctx context.Context, cpf string, cfData *models.CFLookup) error {
	cacheKey := fmt.Sprintf("cf_lookup:cpf:%s", cpf)

	dataBytes, err := json.Marshal(cfData)
	if err != nil {
		return err
	}

	return config.Redis.Set(ctx, cacheKey, dataBytes, config.AppConfig.CFLookupCacheTTL).Err()
}

// invalidateCFCache removes CF data from Redis cache
func (s *CFLookupService) invalidateCFCache(ctx context.Context, cpf string) error {
	cacheKey := fmt.Sprintf("cf_lookup:cpf:%s", cpf)
	return config.Redis.Del(ctx, cacheKey).Err()
}

// TrySynchronousCFLookup attempts to get CF data immediately for immediate wallet response
func (s *CFLookupService) TrySynchronousCFLookup(ctx context.Context, cpf, address string) (*models.CFLookup, error) {
	ctx, span := utils.TraceBusinessLogic(ctx, "cf_lookup_synchronous")
	defer span.End()

	// Check if CF lookup service is properly initialized
	if s == nil || s.mcpClient == nil {
		if s != nil && s.logger != nil {
			s.logger.Error("CF lookup service not properly initialized", zap.String("cpf", cpf))
		}
		return nil, fmt.Errorf("CF lookup service not available")
	}

	// Check if we already have cached CF data
	cachedData, err := s.GetCFDataForCitizen(ctx, cpf)
	if err == nil && cachedData != nil && cachedData.IsActive {
		s.logger.Debug("found cached CF data for synchronous lookup", zap.String("cpf", cpf))
		return cachedData, nil
	}

	// Try synchronous MCP lookup with configurable timeout
	// Default 8 seconds balances user experience with MCP server response times
	syncCtx, cancel := context.WithTimeout(ctx, config.AppConfig.CFLookupSyncTimeout)
	defer cancel()

	s.logger.Debug("attempting synchronous CF lookup", zap.String("cpf", cpf))

	// No rate limiting needed for CF lookups

	// Perform MCP lookup
	cfData, err := s.mcpClient.FindNearestCF(syncCtx, address)
	if err != nil {
		s.logger.Debug("synchronous CF lookup failed", zap.Error(err), zap.String("cpf", cpf))
		// Fall back to async lookup - queue a job manually
		s.queueCFLookupJob(ctx, cpf, address)
		return nil, err
	}

	// Check if CF was found
	if cfData == nil {
		s.logger.Debug("no CF found for address in synchronous lookup",
			zap.String("cpf", cpf),
			zap.String("address", address))
		return nil, nil
	}

	// Success! Store the result immediately
	cfLookup := &models.CFLookup{
		ID:           primitive.NewObjectID(),
		CPF:          cpf,
		AddressHash:  s.GenerateAddressHash(address),
		AddressUsed:  address,
		CFData:       *cfData,
		LookupSource: "mcp",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		IsActive:     true,
	}

	// Store in database
	collection := s.database.Collection(config.AppConfig.CFLookupCollection)
	_, err = collection.InsertOne(ctx, cfLookup)
	if err != nil {
		s.logger.Error("failed to store synchronous CF lookup result", zap.Error(err))
		return cfLookup, nil // Return the data even if storage failed
	}

	// Cache the result
	err = s.cacheCFData(ctx, cpf, cfLookup)
	if err != nil {
		s.logger.Warn("failed to cache synchronous CF lookup result", zap.Error(err))
	}

	s.logger.Info("synchronous CF lookup successful",
		zap.String("cpf", cpf),
		zap.String("cf_name", cfData.NomePopular))

	return cfLookup, nil
}

// queueCFLookupJob queues a CF lookup job for background processing
func (s *CFLookupService) queueCFLookupJob(ctx context.Context, cpf, address string) {
	job := SyncJob{
		ID:         primitive.NewObjectID().Hex(),
		Type:       "cf_lookup",
		Collection: "cf_lookup",
		Data: map[string]interface{}{
			"cpf":     cpf,
			"address": address,
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	jobBytes, err := json.Marshal(job)
	if err != nil {
		s.logger.Error("failed to marshal CF lookup job", zap.Error(err))
		return
	}

	// Queue job using Redis
	queueKey := "sync:queue:cf_lookup"
	err = config.Redis.LPush(ctx, queueKey, string(jobBytes)).Err()
	if err != nil {
		s.logger.Error("failed to queue CF lookup job", zap.Error(err))
		return
	}

	s.logger.Debug("CF lookup job queued successfully", zap.String("job_id", job.ID))
}
