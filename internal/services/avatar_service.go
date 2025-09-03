package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
)

// AvatarService handles avatar operations with caching
type AvatarService struct {
	mongoClient *mongo.Client
	database    *mongo.Database
	logger      *zap.Logger
}

// NewAvatarService creates a new AvatarService instance
func NewAvatarService(mongoClient *mongo.Client, database *mongo.Database, logger *zap.Logger) *AvatarService {
	return &AvatarService{
		mongoClient: mongoClient,
		database:    database,
		logger:      logger,
	}
}

// ListAvatars retrieves paginated list of active avatars with caching
func (s *AvatarService) ListAvatars(ctx context.Context, page, perPage int) (*models.AvatarsListResponse, error) {
	ctx, span := utils.TraceBusinessLogic(ctx, "list_avatars")
	defer span.End()

	// Validate pagination parameters
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20 // Default page size
	}

	// Try cache first
	cacheKey := fmt.Sprintf("avatars:list:page:%d:per_page:%d", page, perPage)
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("list_avatars").Inc()
		s.logger.Debug("avatars list cache hit", zap.Int("page", page), zap.Int("per_page", perPage))
		
		var response models.AvatarsListResponse
		if err := json.Unmarshal([]byte(cached), &response); err == nil {
			return &response, nil
		}
	}

	s.logger.Debug("avatars list cache miss", zap.Int("page", page), zap.Int("per_page", perPage))

	// Query database
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.AvatarsCollection, "list_avatars")
	defer dbSpan.End()

	collection := s.database.Collection(config.AppConfig.AvatarsCollection)

	// Filter for active avatars only
	filter := bson.M{"is_active": true}

	// Count total active avatars
	total, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		s.logger.Error("failed to count avatars", zap.Error(err))
		return nil, fmt.Errorf("failed to count avatars: %w", err)
	}

	// Calculate pagination
	skip := (page - 1) * perPage
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))

	// Query avatars with pagination
	findOptions := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPage)).
		SetSort(bson.D{{Key: "created_at", Value: -1}}) // Latest first

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		s.logger.Error("failed to query avatars", zap.Error(err))
		return nil, fmt.Errorf("failed to query avatars: %w", err)
	}
	defer cursor.Close(ctx)

	var avatars []models.Avatar
	if err := cursor.All(ctx, &avatars); err != nil {
		s.logger.Error("failed to decode avatars", zap.Error(err))
		return nil, fmt.Errorf("failed to decode avatars: %w", err)
	}

	// Convert to response format
	avatarResponses := make([]models.AvatarResponse, len(avatars))
	for i, avatar := range avatars {
		avatarResponses[i] = avatar.ToResponse()
	}

	response := &models.AvatarsListResponse{
		Data:       avatarResponses,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}

	// Cache the result
	responseJSON, err := json.Marshal(response)
	if err == nil {
		err = config.Redis.Set(ctx, cacheKey, responseJSON, config.AppConfig.AvatarCacheTTL).Err()
		if err != nil {
			s.logger.Warn("failed to cache avatars list", zap.Error(err))
		}
	}

	s.logger.Debug("avatars list retrieved successfully",
		zap.Int("total", int(total)),
		zap.Int("page", page),
		zap.Int("per_page", perPage))

	return response, nil
}

// GetAvatarByID retrieves a specific avatar by ID with caching
func (s *AvatarService) GetAvatarByID(ctx context.Context, avatarID string) (*models.Avatar, error) {
	ctx, span := utils.TraceCacheGet(ctx, fmt.Sprintf("avatar:%s", avatarID))
	defer span.End()

	// Validate ObjectID
	objectID, err := primitive.ObjectIDFromHex(avatarID)
	if err != nil {
		return nil, fmt.Errorf("invalid avatar ID: %w", err)
	}

	// Try cache first
	cacheKey := fmt.Sprintf("avatar:id:%s", avatarID)
	cached, err := config.Redis.Get(ctx, cacheKey).Result()
	if err == nil {
		observability.CacheHits.WithLabelValues("get_avatar").Inc()
		s.logger.Debug("avatar cache hit", zap.String("id", avatarID))
		
		var avatar models.Avatar
		if err := json.Unmarshal([]byte(cached), &avatar); err == nil {
			return &avatar, nil
		}
	}

	s.logger.Debug("avatar cache miss", zap.String("id", avatarID))

	// Query database
	ctx, dbSpan := utils.TraceDatabaseFind(ctx, config.AppConfig.AvatarsCollection, "avatar_by_id")
	defer dbSpan.End()

	collection := s.database.Collection(config.AppConfig.AvatarsCollection)
	var avatar models.Avatar

	err = collection.FindOne(ctx, bson.M{"_id": objectID, "is_active": true}).Decode(&avatar)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		s.logger.Error("failed to query avatar", zap.Error(err), zap.String("id", avatarID))
		return nil, fmt.Errorf("failed to query avatar: %w", err)
	}

	// Cache the result
	avatarJSON, err := json.Marshal(avatar)
	if err == nil {
		err = config.Redis.Set(ctx, cacheKey, avatarJSON, config.AppConfig.AvatarCacheTTL).Err()
		if err != nil {
			s.logger.Warn("failed to cache avatar", zap.Error(err), zap.String("id", avatarID))
		}
	}

	s.logger.Debug("avatar found and cached", zap.String("id", avatarID), zap.String("name", avatar.Name))

	return &avatar, nil
}

// CreateAvatar creates a new avatar (admin only)
func (s *AvatarService) CreateAvatar(ctx context.Context, request *models.AvatarRequest) (*models.Avatar, error) {
	ctx, span := utils.TraceBusinessLogic(ctx, "create_avatar")
	defer span.End()

	// Create new avatar
	avatar := &models.Avatar{
		ID:        primitive.NewObjectID(),
		Name:      request.Name,
		URL:       request.URL,
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Insert into database
	ctx, dbSpan := utils.TraceDatabaseUpdate(ctx, config.AppConfig.AvatarsCollection, "create_avatar", false)
	defer dbSpan.End()

	collection := s.database.Collection(config.AppConfig.AvatarsCollection)
	_, err := collection.InsertOne(ctx, avatar)
	if err != nil {
		s.logger.Error("failed to create avatar", zap.Error(err), zap.String("name", request.Name))
		return nil, fmt.Errorf("failed to create avatar: %w", err)
	}

	// Invalidate list cache
	s.invalidateListCache(ctx)

	s.logger.Info("avatar created successfully",
		zap.String("id", avatar.ID.Hex()),
		zap.String("name", avatar.Name),
		zap.String("url", avatar.URL))

	return avatar, nil
}

// DeleteAvatar soft-deletes an avatar (admin only)
func (s *AvatarService) DeleteAvatar(ctx context.Context, avatarID string) error {
	ctx, span := utils.TraceBusinessLogic(ctx, "delete_avatar")
	defer span.End()

	// Validate ObjectID
	objectID, err := primitive.ObjectIDFromHex(avatarID)
	if err != nil {
		return fmt.Errorf("invalid avatar ID: %w", err)
	}

	// Soft delete by setting is_active to false
	ctx, dbSpan := utils.TraceDatabaseUpdate(ctx, config.AppConfig.AvatarsCollection, "delete_avatar", false)
	defer dbSpan.End()

	collection := s.database.Collection(config.AppConfig.AvatarsCollection)
	update := bson.M{
		"$set": bson.M{
			"is_active":  false,
			"updated_at": time.Now(),
		},
	}

	result, err := collection.UpdateOne(ctx, bson.M{"_id": objectID}, update)
	if err != nil {
		s.logger.Error("failed to delete avatar", zap.Error(err), zap.String("id", avatarID))
		return fmt.Errorf("failed to delete avatar: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("avatar not found")
	}

	// Invalidate caches
	s.invalidateAvatarCache(ctx, avatarID)
	s.invalidateListCache(ctx)

	// Queue background job to reset user configs using this avatar
	err = s.queueAvatarCleanupJob(ctx, avatarID)
	if err != nil {
		s.logger.Warn("failed to queue avatar cleanup job", zap.Error(err), zap.String("avatar_id", avatarID))
	}

	s.logger.Info("avatar deleted successfully", zap.String("id", avatarID))
	return nil
}

// ValidateAvatarExists checks if an avatar exists and is active
func (s *AvatarService) ValidateAvatarExists(ctx context.Context, avatarID string) (bool, error) {
	if avatarID == "" {
		return true, nil // Allow null avatar
	}

	avatar, err := s.GetAvatarByID(ctx, avatarID)
	if err != nil {
		return false, err
	}

	return avatar != nil, nil
}

// invalidateAvatarCache removes avatar from cache
func (s *AvatarService) invalidateAvatarCache(ctx context.Context, avatarID string) {
	cacheKey := fmt.Sprintf("avatar:id:%s", avatarID)
	err := config.Redis.Del(ctx, cacheKey).Err()
	if err != nil {
		s.logger.Warn("failed to invalidate avatar cache", zap.Error(err), zap.String("avatar_id", avatarID))
	}
}

// invalidateListCache removes all list cache entries
func (s *AvatarService) invalidateListCache(ctx context.Context) {
	// Use a pattern to delete all list cache entries
	pattern := "avatars:list:*"
	keys, err := config.Redis.Keys(ctx, pattern).Result()
	if err != nil {
		s.logger.Warn("failed to get list cache keys", zap.Error(err))
		return
	}

	if len(keys) > 0 {
		err = config.Redis.Del(ctx, keys...).Err()
		if err != nil {
			s.logger.Warn("failed to invalidate list cache", zap.Error(err))
		}
	}
}

// queueAvatarCleanupJob adds a cleanup job for orphaned avatar references
func (s *AvatarService) queueAvatarCleanupJob(ctx context.Context, avatarID string) error {
	// Create cleanup job data
	jobData := map[string]interface{}{
		"type":      "avatar_cleanup",
		"avatar_id": avatarID,
		"timestamp": time.Now().Unix(),
	}

	jobJSON, err := json.Marshal(jobData)
	if err != nil {
		return fmt.Errorf("failed to marshal cleanup job: %w", err)
	}

	// Queue job in user config queue for sync worker
	queueName := "user_config:write"
	err = config.Redis.LPush(ctx, queueName, jobJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to queue cleanup job: %w", err)
	}

	s.logger.Info("avatar cleanup job queued", zap.String("avatar_id", avatarID))
	return nil
}

// Global instance
var AvatarServiceInstance *AvatarService

// InitAvatarService initializes the global avatar service instance
func InitAvatarService() {
	logger := zap.L().Named("avatar_service")
	AvatarServiceInstance = NewAvatarService(config.MongoDB.Client(), config.MongoDB, logger)
	logger.Info("avatar service initialized")
}