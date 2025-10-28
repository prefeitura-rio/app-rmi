package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// BetaGroupService handles beta group and whitelist operations
type BetaGroupService struct {
	logger *logging.SafeLogger
}

// NewBetaGroupService creates a new beta group service
func NewBetaGroupService(logger *logging.SafeLogger) *BetaGroupService {
	return &BetaGroupService{
		logger: logger,
	}
}

// CreateGroup creates a new beta group
func (s *BetaGroupService) CreateGroup(ctx context.Context, name string) (*models.BetaGroupResponse, error) {
	group := &models.BetaGroup{
		Name: name,
	}

	if err := group.ValidateName(); err != nil {
		return nil, err
	}

	// Check if group name already exists (case-insensitive)
	normalizedName := group.GetNormalizedName()
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)

	var existingGroup models.BetaGroup
	err := collection.FindOne(ctx, bson.M{"name": bson.M{"$regex": primitive.Regex{Pattern: "^" + normalizedName + "$", Options: "i"}}}).Decode(&existingGroup)
	if err == nil {
		return nil, models.ErrGroupNameExists
	} else if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("failed to check existing group: %w", err)
	}

	// Set timestamps
	group.BeforeCreate()

	// Insert the group
	result, err := collection.InsertOne(ctx, group)
	if err != nil {
		return nil, fmt.Errorf("failed to create beta group: %w", err)
	}

	group.ID = result.InsertedID.(primitive.ObjectID)

	return &models.BetaGroupResponse{
		ID:        group.ID.Hex(),
		Name:      group.Name,
		CreatedAt: group.CreatedAt,
		UpdatedAt: group.UpdatedAt,
	}, nil
}

// GetGroup retrieves a beta group by ID
func (s *BetaGroupService) GetGroup(ctx context.Context, groupID string) (*models.BetaGroupResponse, error) {
	objectID, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		return nil, models.ErrInvalidGroupID
	}

	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)

	var group models.BetaGroup
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&group)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, models.ErrGroupNotFound
		}
		return nil, fmt.Errorf("failed to get beta group: %w", err)
	}

	return &models.BetaGroupResponse{
		ID:        group.ID.Hex(),
		Name:      group.Name,
		CreatedAt: group.CreatedAt,
		UpdatedAt: group.UpdatedAt,
	}, nil
}

// ListGroups retrieves paginated list of beta groups
func (s *BetaGroupService) ListGroups(ctx context.Context, page, perPage int) (*models.BetaGroupListResponse, error) {
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)

	// Calculate skip
	skip := (page - 1) * perPage

	// Get total count
	totalGroups, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to count beta groups: %w", err)
	}

	// Find groups with pagination
	findOptions := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPage)).
		SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list beta groups: %w", err)
	}
	defer cursor.Close(ctx)

	var groups []models.BetaGroupResponse
	for cursor.Next(ctx) {
		var group models.BetaGroup
		if err := cursor.Decode(&group); err != nil {
			continue
		}
		groups = append(groups, models.BetaGroupResponse{
			ID:        group.ID.Hex(),
			Name:      group.Name,
			CreatedAt: group.CreatedAt,
			UpdatedAt: group.UpdatedAt,
		})
	}

	return &models.BetaGroupListResponse{
		Groups:      groups,
		TotalGroups: totalGroups,
		Pagination: models.PaginationInfo{
			Page:    page,
			PerPage: perPage,
			Total:   int(totalGroups),
		},
	}, nil
}

// UpdateGroup updates a beta group
func (s *BetaGroupService) UpdateGroup(ctx context.Context, groupID, name string) (*models.BetaGroupResponse, error) {
	objectID, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		return nil, models.ErrInvalidGroupID
	}

	// Validate the new name
	group := &models.BetaGroup{Name: name}
	if err := group.ValidateName(); err != nil {
		return nil, err
	}

	// Check if group name already exists (case-insensitive, excluding current group)
	normalizedName := group.GetNormalizedName()
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)

	var existingGroup models.BetaGroup
	err = collection.FindOne(ctx, bson.M{
		"_id":  bson.M{"$ne": objectID},
		"name": bson.M{"$regex": primitive.Regex{Pattern: "^" + normalizedName + "$", Options: "i"}},
	}).Decode(&existingGroup)
	if err == nil {
		return nil, models.ErrGroupNameExists
	} else if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("failed to check existing group: %w", err)
	}

	// Update the group
	group.BeforeUpdate()
	update := bson.M{
		"$set": bson.M{
			"name":       name,
			"updated_at": group.UpdatedAt,
		},
	}

	result := collection.FindOneAndUpdate(ctx, bson.M{"_id": objectID}, update, options.FindOneAndUpdate().SetReturnDocument(options.After))
	if err := result.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, models.ErrGroupNotFound
		}
		return nil, fmt.Errorf("failed to update beta group: %w", err)
	}

	var updatedGroup models.BetaGroup
	if err := result.Decode(&updatedGroup); err != nil {
		return nil, fmt.Errorf("failed to decode updated group: %w", err)
	}

	return &models.BetaGroupResponse{
		ID:        updatedGroup.ID.Hex(),
		Name:      updatedGroup.Name,
		CreatedAt: updatedGroup.CreatedAt,
		UpdatedAt: updatedGroup.UpdatedAt,
	}, nil
}

// DeleteGroup deletes a beta group and removes all phone associations
func (s *BetaGroupService) DeleteGroup(ctx context.Context, groupID string) error {
	objectID, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		return models.ErrInvalidGroupID
	}

	// Check if group exists
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
	var group models.BetaGroup
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&group)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return models.ErrGroupNotFound
		}
		return fmt.Errorf("failed to get beta group: %w", err)
	}

	// Remove all phone associations from this group
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	_, err = phoneCollection.UpdateMany(ctx,
		bson.M{"beta_group_id": groupID},
		bson.M{"$unset": bson.M{"beta_group_id": ""}},
	)
	if err != nil {
		return fmt.Errorf("failed to remove phone associations: %w", err)
	}

	// Delete the group
	_, err = collection.DeleteOne(ctx, bson.M{"_id": objectID})
	if err != nil {
		return fmt.Errorf("failed to delete beta group: %w", err)
	}

	// Invalidate cache for all phones that were in this group
	s.invalidateBetaStatusCache(ctx, groupID)

	return nil
}

// AddToWhitelist adds a phone number to a beta group
func (s *BetaGroupService) AddToWhitelist(ctx context.Context, phoneNumber, groupID string) (*models.BetaWhitelistResponse, error) {
	// Validate group ID
	objectID, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		return nil, models.ErrInvalidGroupID
	}

	// Check if group exists
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
	var group models.BetaGroup
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&group)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, models.ErrGroupNotFound
		}
		return nil, fmt.Errorf("failed to get beta group: %w", err)
	}

	// Format phone number for storage
	storagePhone := strings.TrimPrefix(phoneNumber, "+")

	// Check if phone is already whitelisted
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	var existingMapping models.PhoneCPFMapping
	err = phoneCollection.FindOne(ctx, bson.M{"phone_number": storagePhone}).Decode(&existingMapping)
	if err == nil && existingMapping.BetaGroupID != "" {
		return nil, models.ErrPhoneAlreadyWhitelisted
	}

	// Add or update phone mapping with beta group
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"beta_group_id": groupID,
			"updated_at":    now,
		},
		"$setOnInsert": bson.M{
			"phone_number": storagePhone,
			"status":       "active",
			"created_at":   now,
		},
	}

	_, err = phoneCollection.UpdateOne(ctx,
		bson.M{"phone_number": storagePhone},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add phone to whitelist: %w", err)
	}

	// Invalidate cache for this phone
	s.invalidateBetaStatusCacheForPhone(ctx, storagePhone)

	return &models.BetaWhitelistResponse{
		PhoneNumber: phoneNumber,
		GroupID:     groupID,
		GroupName:   group.Name,
		AddedAt:     now,
	}, nil
}

// RemoveFromWhitelist removes a phone number from beta whitelist
func (s *BetaGroupService) RemoveFromWhitelist(ctx context.Context, phoneNumber string) error {
	storagePhone := strings.TrimPrefix(phoneNumber, "+")

	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)

	// Check if phone is whitelisted
	var mapping models.PhoneCPFMapping
	err := phoneCollection.FindOne(ctx, bson.M{"phone_number": storagePhone}).Decode(&mapping)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return models.ErrPhoneNotWhitelisted
		}
		return fmt.Errorf("failed to get phone mapping: %w", err)
	}

	if mapping.BetaGroupID == "" {
		return models.ErrPhoneNotWhitelisted
	}

	// Remove from whitelist
	_, err = phoneCollection.UpdateOne(ctx,
		bson.M{"phone_number": storagePhone},
		bson.M{
			"$unset": bson.M{"beta_group_id": ""},
			"$set":   bson.M{"updated_at": time.Now()},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to remove phone from whitelist: %w", err)
	}

	// Invalidate cache for this phone
	s.invalidateBetaStatusCacheForPhone(ctx, storagePhone)

	return nil
}

// GetBetaStatus gets the beta status for a phone number (with caching)
func (s *BetaGroupService) GetBetaStatus(ctx context.Context, phoneNumber string) (*models.BetaStatusResponse, error) {
	storagePhone := strings.TrimPrefix(phoneNumber, "+")

	// Try to get from cache first
	cacheKey := fmt.Sprintf("beta_status:%s", storagePhone)
	cached := config.Redis.Get(ctx, cacheKey)
	if err := cached.Err(); err == nil {
		cachedValue, err := cached.Result()
		if err == nil && cachedValue != "" {
			// Deserialize full response from cache
			var response models.BetaStatusResponse
			if err := json.Unmarshal([]byte(cachedValue), &response); err == nil {
				return &response, nil
			}
			// If deserialization fails, fall through to database query
		}
	}

	// Get from database
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	var mapping models.PhoneCPFMapping
	err := phoneCollection.FindOne(ctx, bson.M{"phone_number": storagePhone}).Decode(&mapping)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Cache negative result
			response := &models.BetaStatusResponse{
				PhoneNumber:     phoneNumber,
				BetaWhitelisted: false,
			}
			if cacheJSON, err := json.Marshal(response); err == nil {
				config.Redis.Set(ctx, cacheKey, string(cacheJSON), config.AppConfig.BetaStatusCacheTTL)
			}
			return response, nil
		}
		return nil, fmt.Errorf("failed to get phone mapping: %w", err)
	}

	response := &models.BetaStatusResponse{
		PhoneNumber:     phoneNumber,
		BetaWhitelisted: mapping.BetaGroupID != "",
		GroupID:         mapping.BetaGroupID,
	}

	// Get group name if whitelisted
	if mapping.BetaGroupID != "" {
		group, err := s.GetGroup(ctx, mapping.BetaGroupID)
		if err == nil {
			response.GroupName = group.Name
		}
	}

	// Cache the complete response as JSON
	if cacheJSON, err := json.Marshal(response); err == nil {
		config.Redis.Set(ctx, cacheKey, string(cacheJSON), config.AppConfig.BetaStatusCacheTTL)
	}

	return response, nil
}

// ListWhitelistedPhones gets paginated list of whitelisted phones
func (s *BetaGroupService) ListWhitelistedPhones(ctx context.Context, page, perPage int, groupID string) (*models.BetaWhitelistListResponse, error) {
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)

	// Build filter
	filter := bson.M{"beta_group_id": bson.M{"$exists": true, "$ne": ""}}
	if groupID != "" {
		filter["beta_group_id"] = groupID
	}

	// Calculate skip
	skip := (page - 1) * perPage

	// Get total count
	totalCount, err := phoneCollection.CountDocuments(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count whitelisted phones: %w", err)
	}

	// Find phones with pagination
	// Use compound sort: updated_at desc, then _id desc for consistent ordering
	findOptions := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(perPage)).
		SetSort(bson.D{
			{Key: "updated_at", Value: -1},
			{Key: "_id", Value: -1},
		})

	cursor, err := phoneCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list whitelisted phones: %w", err)
	}
	defer cursor.Close(ctx)

	var whitelisted []models.BetaWhitelistResponse
	for cursor.Next(ctx) {
		// Use flexible bson.M to handle potentially malformed entries
		var rawDoc bson.M
		if err := cursor.Decode(&rawDoc); err != nil {
			s.logger.Warn("failed to decode raw document in whitelist",
				zap.Error(err),
				zap.String("collection", config.AppConfig.PhoneMappingCollection))
			continue
		}

		// Extract phone number (required field)
		phoneNumber, ok := rawDoc["phone_number"].(string)
		if !ok {
			s.logger.Warn("phone mapping has non-string phone_number field",
				zap.String("collection", config.AppConfig.PhoneMappingCollection),
				zap.Any("phone_number_value", rawDoc["phone_number"]),
				zap.String("phone_number_type", fmt.Sprintf("%T", rawDoc["phone_number"])),
				zap.Any("beta_group_id", rawDoc["beta_group_id"]))
			continue
		}
		if phoneNumber == "" {
			s.logger.Warn("phone mapping has empty phone_number",
				zap.String("collection", config.AppConfig.PhoneMappingCollection),
				zap.Any("beta_group_id", rawDoc["beta_group_id"]))
			continue
		}

		// Extract beta_group_id (handle both string and ObjectID types)
		var betaGroupID string
		switch v := rawDoc["beta_group_id"].(type) {
		case string:
			betaGroupID = v
		case primitive.ObjectID:
			betaGroupID = v.Hex()
		default:
			s.logger.Warn("phone mapping has invalid beta_group_id type",
				zap.String("phone_number", phoneNumber),
				zap.String("collection", config.AppConfig.PhoneMappingCollection),
				zap.Any("beta_group_id", rawDoc["beta_group_id"]))
			continue
		}

		if betaGroupID == "" {
			s.logger.Warn("phone mapping has empty beta_group_id",
				zap.String("phone_number", phoneNumber),
				zap.String("collection", config.AppConfig.PhoneMappingCollection))
			continue
		}

		// Get group name
		groupName := ""
		group, err := s.GetGroup(ctx, betaGroupID)
		if err == nil {
			groupName = group.Name
		}

		// Extract updated_at timestamp with fallback to created_at
		var addedAt time.Time
		if updatedAt, ok := rawDoc["updated_at"].(primitive.DateTime); ok {
			addedAt = updatedAt.Time()
		} else if createdAt, ok := rawDoc["created_at"].(primitive.DateTime); ok {
			addedAt = createdAt.Time()
		} else {
			// If both timestamps are missing, use zero time
			addedAt = time.Time{}
		}

		whitelisted = append(whitelisted, models.BetaWhitelistResponse{
			PhoneNumber: phoneNumber,
			GroupID:     betaGroupID,
			GroupName:   groupName,
			AddedAt:     addedAt,
		})
	}

	return &models.BetaWhitelistListResponse{
		Whitelisted: whitelisted,
		TotalCount:  totalCount,
		Pagination: models.PaginationInfo{
			Page:    page,
			PerPage: perPage,
			Total:   int(totalCount),
		},
	}, nil
}

// BulkAddToWhitelist adds multiple phone numbers to a beta group
func (s *BetaGroupService) BulkAddToWhitelist(ctx context.Context, phoneNumbers []string, groupID string) ([]models.BetaWhitelistResponse, error) {
	// Validate group ID
	objectID, err := primitive.ObjectIDFromHex(groupID)
	if err != nil {
		return nil, models.ErrInvalidGroupID
	}

	// Check if group exists
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
	var group models.BetaGroup
	err = collection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&group)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, models.ErrGroupNotFound
		}
		return nil, fmt.Errorf("failed to get beta group: %w", err)
	}

	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	now := time.Now()
	var results []models.BetaWhitelistResponse

	for _, phoneNumber := range phoneNumbers {
		storagePhone := strings.TrimPrefix(phoneNumber, "+")

		// Check if already whitelisted
		var existingMapping models.PhoneCPFMapping
		err := phoneCollection.FindOne(ctx, bson.M{"phone_number": storagePhone}).Decode(&existingMapping)
		if err == nil && existingMapping.BetaGroupID != "" {
			continue // Skip if already whitelisted
		}

		// Add to whitelist
		update := bson.M{
			"$set": bson.M{
				"beta_group_id": groupID,
				"updated_at":    now,
			},
			"$setOnInsert": bson.M{
				"phone_number": storagePhone,
				"status":       "active",
				"created_at":   now,
			},
		}

		_, err = phoneCollection.UpdateOne(ctx,
			bson.M{"phone_number": storagePhone},
			update,
			options.Update().SetUpsert(true),
		)
		if err != nil {
			continue // Skip on error
		}

		// Invalidate cache for this phone
		s.invalidateBetaStatusCacheForPhone(ctx, storagePhone)

		results = append(results, models.BetaWhitelistResponse{
			PhoneNumber: phoneNumber,
			GroupID:     groupID,
			GroupName:   group.Name,
			AddedAt:     now,
		})
	}

	return results, nil
}

// BulkRemoveFromWhitelist removes multiple phone numbers from beta whitelist
func (s *BetaGroupService) BulkRemoveFromWhitelist(ctx context.Context, phoneNumbers []string) error {
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	now := time.Now()

	for _, phoneNumber := range phoneNumbers {
		storagePhone := strings.TrimPrefix(phoneNumber, "+")

		// Remove from whitelist
		_, err := phoneCollection.UpdateOne(ctx,
			bson.M{"phone_number": storagePhone},
			bson.M{
				"$unset": bson.M{"beta_group_id": ""},
				"$set":   bson.M{"updated_at": now},
			},
		)
		if err != nil {
			continue // Skip on error
		}

		// Invalidate cache for this phone
		s.invalidateBetaStatusCacheForPhone(ctx, storagePhone)
	}

	return nil
}

// BulkMoveWhitelist moves multiple phone numbers from one group to another using batch operations
func (s *BetaGroupService) BulkMoveWhitelist(ctx context.Context, phoneNumbers []string, fromGroupID, toGroupID string) error {
	// Validate group IDs
	fromObjectID, err := primitive.ObjectIDFromHex(fromGroupID)
	if err != nil {
		return models.ErrInvalidGroupID
	}
	toObjectID, err := primitive.ObjectIDFromHex(toGroupID)
	if err != nil {
		return models.ErrInvalidGroupID
	}

	// Check if groups exist
	collection := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
	var fromGroup, toGroup models.BetaGroup

	err = collection.FindOne(ctx, bson.M{"_id": fromObjectID}).Decode(&fromGroup)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return models.ErrGroupNotFound
		}
		return fmt.Errorf("failed to get from group: %w", err)
	}

	err = collection.FindOne(ctx, bson.M{"_id": toObjectID}).Decode(&toGroup)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return models.ErrGroupNotFound
		}
		return fmt.Errorf("failed to get to group: %w", err)
	}

	// Use batch operations for better performance
	if err := s.bulkMoveWhitelistBatch(ctx, phoneNumbers, fromGroupID, toGroupID); err != nil {
		s.logger.Warn("batch move operation failed, falling back to individual operations", zap.Error(err))
		// Fallback to individual operations
		return s.bulkMoveWhitelistIndividual(ctx, phoneNumbers, fromGroupID, toGroupID)
	}

	return nil
}

// bulkMoveWhitelistBatch performs the move operation using MongoDB bulk operations
func (s *BetaGroupService) bulkMoveWhitelistBatch(ctx context.Context, phoneNumbers []string, fromGroupID, toGroupID string) error {
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	now := time.Now()

	// Prepare bulk operations
	bulkOps := make([]mongo.WriteModel, len(phoneNumbers))

	for i, phoneNumber := range phoneNumbers {
		storagePhone := strings.TrimPrefix(phoneNumber, "+")

		bulkOps[i] = mongo.NewUpdateOneModel().
			SetFilter(bson.M{
				"phone_number":  storagePhone,
				"beta_group_id": fromGroupID,
			}).
			SetUpdate(bson.M{
				"$set": bson.M{
					"beta_group_id": toGroupID,
					"updated_at":    now,
				},
			})
	}

	// Execute bulk operation
	result, err := phoneCollection.BulkWrite(ctx, bulkOps)
	if err != nil {
		return fmt.Errorf("bulk write failed: %w", err)
	}

	// Verify all operations were successful
	if result.MatchedCount != int64(len(phoneNumbers)) {
		s.logger.Warn("not all phones were found for move operation",
			zap.Int64("matched", result.MatchedCount),
			zap.Int("requested", len(phoneNumbers)))
	}

	// Invalidate cache for all affected phones using pipeline
	s.invalidateBetaStatusCacheBatch(ctx, phoneNumbers)

	return nil
}

// bulkMoveWhitelistIndividual performs the move operation using individual operations (fallback)
func (s *BetaGroupService) bulkMoveWhitelistIndividual(ctx context.Context, phoneNumbers []string, fromGroupID, toGroupID string) error {
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	now := time.Now()

	for _, phoneNumber := range phoneNumbers {
		storagePhone := strings.TrimPrefix(phoneNumber, "+")

		// Move to new group
		_, err := phoneCollection.UpdateOne(ctx,
			bson.M{"phone_number": storagePhone, "beta_group_id": fromGroupID},
			bson.M{
				"$set": bson.M{
					"beta_group_id": toGroupID,
					"updated_at":    now,
				},
			},
		)
		if err != nil {
			continue // Skip on error
		}

		// Invalidate cache for this phone
		s.invalidateBetaStatusCacheForPhone(ctx, storagePhone)
	}

	return nil
}

// invalidateBetaStatusCacheBatch invalidates cache for multiple phone numbers using Redis pipeline
func (s *BetaGroupService) invalidateBetaStatusCacheBatch(ctx context.Context, phoneNumbers []string) {
	// Use Redis pipeline for batch cache invalidation
	pipe := config.Redis.Pipeline()

	for _, phoneNumber := range phoneNumbers {
		storagePhone := strings.TrimPrefix(phoneNumber, "+")
		cacheKey := fmt.Sprintf("beta_status:%s", storagePhone)
		pipe.Del(ctx, cacheKey)
	}

	// Execute pipeline
	if _, err := pipe.Exec(ctx); err != nil {
		s.logger.Warn("failed to execute cache invalidation pipeline", zap.Error(err))
	}
}

// invalidateBetaStatusCache invalidates cache for all phones in a group
func (s *BetaGroupService) invalidateBetaStatusCache(ctx context.Context, groupID string) {
	phoneCollection := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)

	cursor, err := phoneCollection.Find(ctx, bson.M{"beta_group_id": groupID})
	if err != nil {
		return
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var mapping models.PhoneCPFMapping
		if err := cursor.Decode(&mapping); err != nil {
			continue
		}
		s.invalidateBetaStatusCacheForPhone(ctx, mapping.PhoneNumber)
	}
}

// invalidateBetaStatusCacheForPhone invalidates cache for a specific phone
func (s *BetaGroupService) invalidateBetaStatusCacheForPhone(ctx context.Context, phoneNumber string) {
	cacheKey := fmt.Sprintf("beta_status:%s", phoneNumber)
	config.Redis.Del(ctx, cacheKey)
}
