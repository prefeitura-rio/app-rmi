package services

import (
	"context"
	"fmt"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

// BairroData represents a bairro document
type BairroData struct {
	ID   string `bson:"_id" json:"_id"`
	Nome string `bson:"nome" json:"nome"`
}

// LogradouroData represents a logradouro document
type LogradouroData struct {
	ID           string `bson:"_id" json:"_id"`
	NomeCompleto string `bson:"nome_completo" json:"nome_completo"`
}

// AddressData represents a complete address
type AddressData struct {
	Logradouro string `json:"logradouro"`
	Bairro     string `json:"bairro"`
}

// AddressService handles address building operations
type AddressService struct {
	mongoClient *mongo.Client
	database    *mongo.Database
	logger      *zap.Logger
}

// NewAddressService creates a new AddressService instance
func NewAddressService(mongoClient *mongo.Client, database *mongo.Database, logger *zap.Logger) *AddressService {
	return &AddressService{
		mongoClient: mongoClient,
		database:    database,
		logger:      logger,
	}
}

// GetBairroByID retrieves bairro name by ID with caching
func (s *AddressService) GetBairroByID(ctx context.Context, bairroID string) (string, error) {
	if bairroID == "" {
		return "", nil
	}

	ctx, span := utils.TraceCacheGet(ctx, fmt.Sprintf("bairro:%s", bairroID))
	defer span.End()

	// Try cache first
	cacheKey := fmt.Sprintf("address:bairro:%s", bairroID)
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("get_bairro").Inc()
		s.logger.Debug("bairro cache hit", zap.String("id", bairroID))
		return cached, nil
	}

	s.logger.Debug("bairro cache miss", zap.String("id", bairroID))

	// Query database
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.BairroCollection, "bairro_by_id")
	defer dbSpan.End()

	collection := s.database.Collection(config.AppConfig.BairroCollection)
	var bairroData BairroData

	err = collection.FindOne(ctx, bson.M{"id_bairro": bairroID}).Decode(&bairroData)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			s.logger.Debug("bairro not found", zap.String("id", bairroID))
			return "", nil
		}
		s.logger.Error("failed to query bairro", zap.Error(err), zap.String("id", bairroID))
		return "", fmt.Errorf("failed to query bairro: %w", err)
	}

	// Cache the result
	err = config.Redis.Set(ctx, cacheKey, bairroData.Nome, config.AppConfig.AddressCacheTTL).Err()
	if err != nil {
		s.logger.Warn("failed to cache bairro", zap.Error(err), zap.String("id", bairroID))
	}

	s.logger.Debug("bairro found and cached",
		zap.String("id", bairroID),
		zap.String("nome", bairroData.Nome))

	return bairroData.Nome, nil
}

// GetLogradouroByID retrieves logradouro name by ID with caching
func (s *AddressService) GetLogradouroByID(ctx context.Context, logradouroID string) (string, error) {
	if logradouroID == "" {
		return "", nil
	}

	ctx, span := utils.TraceCacheGet(ctx, fmt.Sprintf("logradouro:%s", logradouroID))
	defer span.End()

	// Try cache first
	cacheKey := fmt.Sprintf("address:logradouro:%s", logradouroID)
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("get_logradouro").Inc()
		s.logger.Debug("logradouro cache hit", zap.String("id", logradouroID))
		return cached, nil
	}

	s.logger.Debug("logradouro cache miss", zap.String("id", logradouroID))

	// Query database
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.LogradouroCollection, "logradouro_by_id")
	defer dbSpan.End()

	collection := s.database.Collection(config.AppConfig.LogradouroCollection)
	var logradouroData LogradouroData

	err = collection.FindOne(ctx, bson.M{"id_logradouro": logradouroID}).Decode(&logradouroData)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			s.logger.Debug("logradouro not found", zap.String("id", logradouroID))
			return "", nil
		}
		s.logger.Error("failed to query logradouro", zap.Error(err), zap.String("id", logradouroID))
		return "", fmt.Errorf("failed to query logradouro: %w", err)
	}

	// Cache the result
	err = config.Redis.Set(ctx, cacheKey, logradouroData.NomeCompleto, config.AppConfig.AddressCacheTTL).Err()
	if err != nil {
		s.logger.Warn("failed to cache logradouro", zap.Error(err), zap.String("id", logradouroID))
	}

	s.logger.Debug("logradouro found and cached",
		zap.String("id", logradouroID),
		zap.String("nome_completo", logradouroData.NomeCompleto))

	return logradouroData.NomeCompleto, nil
}

// BuildAddress builds a human-readable address from IDs with caching
func (s *AddressService) BuildAddress(ctx context.Context, bairroID, logradouroID string, numeroLogradouro interface{}) (*string, error) {
	ctx, span := utils.TraceBusinessLogic(ctx, "build_address")
	defer span.End()

	// If we don't have any address information, return nil
	if bairroID == "" && logradouroID == "" {
		return nil, nil
	}

	// Create a composite cache key for the full address
	cacheKey := fmt.Sprintf("address:full:%s:%s:%v", bairroID, logradouroID, numeroLogradouro)

	// Try cache first for the complete address
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("build_address").Inc()
		s.logger.Debug("full address cache hit",
			zap.String("bairro_id", bairroID),
			zap.String("logradouro_id", logradouroID))
		return &cached, nil
	}

	// Build address from components
	var logradouroNome, bairroNome string

	// Get logradouro name
	if logradouroID != "" {
		logradouroNome, err = s.GetLogradouroByID(ctx, logradouroID)
		if err != nil {
			s.logger.Error("failed to get logradouro", zap.Error(err), zap.String("id", logradouroID))
			return nil, fmt.Errorf("failed to get logradouro: %w", err)
		}
	}

	// Get bairro name
	if bairroID != "" {
		bairroNome, err = s.GetBairroByID(ctx, bairroID)
		if err != nil {
			s.logger.Error("failed to get bairro", zap.Error(err), zap.String("id", bairroID))
			return nil, fmt.Errorf("failed to get bairro: %w", err)
		}
	}

	// If we couldn't get any address components, return nil
	if logradouroNome == "" && bairroNome == "" {
		return nil, nil
	}

	// Build the address string in format: "{logradouro}, {numero} - {bairro}"
	var addressParts []string

	// Add logradouro and number if available
	if logradouroNome != "" {
		if numeroLogradouro != nil {
			// Convert numero to string, handling different types
			var numeroStr string
			switch v := numeroLogradouro.(type) {
			case int:
				numeroStr = strconv.Itoa(v)
			case int32:
				numeroStr = strconv.FormatInt(int64(v), 10)
			case int64:
				numeroStr = strconv.FormatInt(v, 10)
			case float64:
				numeroStr = strconv.FormatFloat(v, 'f', 0, 64)
			case string:
				numeroStr = v
			default:
				numeroStr = fmt.Sprintf("%v", v)
			}

			if numeroStr != "" && numeroStr != "0" {
				addressParts = append(addressParts, fmt.Sprintf("%s, %s", logradouroNome, numeroStr))
			} else {
				addressParts = append(addressParts, logradouroNome)
			}
		} else {
			addressParts = append(addressParts, logradouroNome)
		}
	}

	// Add bairro if available
	if bairroNome != "" {
		if len(addressParts) > 0 {
			addressParts = append(addressParts, fmt.Sprintf(" - %s", bairroNome))
		} else {
			addressParts = append(addressParts, bairroNome)
		}
	}

	if len(addressParts) == 0 {
		return nil, nil
	}

	// Join the parts
	var address string
	for _, part := range addressParts {
		address += part
	}

	// Cache the complete address
	err = config.Redis.Set(ctx, cacheKey, address, config.AppConfig.AddressCacheTTL).Err()
	if err != nil {
		s.logger.Warn("failed to cache complete address", zap.Error(err))
	}

	s.logger.Debug("address built successfully",
		zap.String("address", address),
		zap.String("bairro_id", bairroID),
		zap.String("logradouro_id", logradouroID))

	return &address, nil
}

// Global instance
var AddressServiceInstance *AddressService

// InitAddressService initializes the global address service instance
func InitAddressService() {
	logger := zap.L().Named("address_service")
	AddressServiceInstance = NewAddressService(config.MongoDB.Client(), config.MongoDB, logger)
	logger.Info("address service initialized")
}
