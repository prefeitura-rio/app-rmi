package services

import (
	"context"
	"fmt"
	"strconv"
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

// DepartmentService handles department business logic
type DepartmentService struct {
	database *mongo.Database
	logger   *logging.SafeLogger
}

// NewDepartmentService creates a new department service instance
func NewDepartmentService(database *mongo.Database, logger *logging.SafeLogger) *DepartmentService {
	return &DepartmentService{
		database: database,
		logger:   logger,
	}
}

// Global department service instance
var DepartmentServiceInstance *DepartmentService

// InitDepartmentService initializes the global department service instance
func InitDepartmentService() {
	logger := zap.L().Named("department_service")

	DepartmentServiceInstance = NewDepartmentService(config.MongoDB, &logging.SafeLogger{})

	logger.Info("department service initialized successfully")
	logger.Info("indexes will be managed by global database maintenance system")
}

// DepartmentFilters represents filters for listing departments
type DepartmentFilters struct {
	ParentID   string
	MinLevel   *int
	MaxLevel   *int
	ExactLevel *int
	SiglaUA    string
	Search     string
	Page       int
	PerPage    int
}

// GetDepartmentByID retrieves a department by its cd_ua
func (s *DepartmentService) GetDepartmentByID(ctx context.Context, cdUA string) (*models.Department, error) {
	collection := s.database.Collection(config.AppConfig.DepartmentCollection)

	filter := bson.M{"cd_ua": cdUA}

	var rawDoc bson.M
	err := collection.FindOne(ctx, filter).Decode(&rawDoc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("department not found with cd_ua: %s", cdUA)
		}
		s.logger.Error("failed to get department by ID",
			zap.String("cd_ua", cdUA),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get department: %w", err)
	}

	// Convert to Department model with type handling
	department := s.convertRawToDepartment(rawDoc)
	return &department, nil
}

// ListDepartments retrieves a paginated list of departments with optional filters
func (s *DepartmentService) ListDepartments(ctx context.Context, filters DepartmentFilters) (*models.DepartmentListResponse, error) {
	collection := s.database.Collection(config.AppConfig.DepartmentCollection)

	// Build filter query
	filter := bson.M{}

	// Parent ID filter
	if filters.ParentID != "" {
		filter["cd_ua_pai"] = filters.ParentID
	}

	// Level filters - convert to string for MongoDB comparison since nivel is stored as string
	if filters.ExactLevel != nil {
		filter["nivel"] = strconv.Itoa(*filters.ExactLevel)
	} else {
		// Min/Max level filters (only if exact level not specified)
		if filters.MinLevel != nil || filters.MaxLevel != nil {
			nivelFilter := bson.M{}
			if filters.MinLevel != nil {
				nivelFilter["$gte"] = strconv.Itoa(*filters.MinLevel)
			}
			if filters.MaxLevel != nil {
				nivelFilter["$lte"] = strconv.Itoa(*filters.MaxLevel)
			}
			filter["nivel"] = nivelFilter
		}
	}

	// Sigla UA filter
	if filters.SiglaUA != "" {
		filter["sigla_ua"] = filters.SiglaUA
	}

	// Text search filter - case insensitive partial matching on nome_ua and sigla_ua
	if filters.Search != "" {
		filter["$or"] = []bson.M{
			{"nome_ua": bson.M{"$regex": filters.Search, "$options": "i"}},
			{"sigla_ua": bson.M{"$regex": filters.Search, "$options": "i"}},
		}
	}

	// Calculate pagination
	skip := (filters.Page - 1) * filters.PerPage

	// Get total count
	totalCount, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		s.logger.Error("failed to count departments", zap.Error(err))
		return nil, fmt.Errorf("failed to count departments: %w", err)
	}

	// Set up find options
	findOptions := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(filters.PerPage)).
		SetSort(bson.D{
			{Key: "nivel", Value: 1},
			{Key: "ordem_absoluta", Value: 1},
		})

	// Execute query
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		s.logger.Error("failed to list departments", zap.Error(err))
		return nil, fmt.Errorf("failed to list departments: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode results with flexible bson.M to handle type variations
	var departments []models.Department
	for cursor.Next(ctx) {
		var rawDoc bson.M
		if err := cursor.Decode(&rawDoc); err != nil {
			s.logger.Warn("failed to decode department document",
				zap.Error(err))
			continue
		}

		// Convert to Department model with type handling
		dept := s.convertRawToDepartment(rawDoc)
		departments = append(departments, dept)
	}

	if err := cursor.Err(); err != nil {
		s.logger.Error("cursor error while iterating departments", zap.Error(err))
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	// Convert to response format
	departmentResponses := make([]models.DepartmentResponse, len(departments))
	for i, dept := range departments {
		departmentResponses[i] = dept.ToResponse()
	}

	// Calculate total pages
	totalPages := int(totalCount) / filters.PerPage
	if int(totalCount)%filters.PerPage != 0 {
		totalPages++
	}

	return &models.DepartmentListResponse{
		Departments: departmentResponses,
		Pagination: models.PaginationInfo{
			Page:       filters.Page,
			PerPage:    filters.PerPage,
			Total:      int(totalCount),
			TotalPages: totalPages,
		},
		TotalCount: totalCount,
	}, nil
}

// convertRawToDepartment converts a raw BSON document to a Department model with type handling
func (s *DepartmentService) convertRawToDepartment(rawDoc bson.M) models.Department {
	dept := models.Department{}

	// Handle _id field
	if id, ok := rawDoc["_id"]; ok {
		switch v := id.(type) {
		case string:
			dept.ID = v
		case primitive.ObjectID:
			dept.ID = v.Hex()
		}
	}

	// Handle string fields
	if cdUA, ok := rawDoc["cd_ua"].(string); ok {
		dept.CdUA = cdUA
	}
	if siglaUA, ok := rawDoc["sigla_ua"].(string); ok {
		dept.SiglaUA = siglaUA
	}
	if nomeUA, ok := rawDoc["nome_ua"].(string); ok {
		dept.NomeUA = nomeUA
	}
	if cdUAPai, ok := rawDoc["cd_ua_pai"].(string); ok {
		dept.CdUAPai = cdUAPai
	}
	if ordemUABasica, ok := rawDoc["ordem_ua_basica"].(string); ok {
		dept.OrdemUABasica = ordemUABasica
	}
	if ordemAbsoluta, ok := rawDoc["ordem_absoluta"].(string); ok {
		dept.OrdemAbsoluta = ordemAbsoluta
	}
	if ordemRelativa, ok := rawDoc["ordem_relativa"].(string); ok {
		dept.OrdemRelativa = ordemRelativa
	}

	// Handle nivel field (can be int or string)
	if nivel, ok := rawDoc["nivel"]; ok {
		switch v := nivel.(type) {
		case int:
			dept.Nivel = v
		case int32:
			dept.Nivel = int(v)
		case int64:
			dept.Nivel = int(v)
		case string:
			// Try to parse string to int
			if parsed, err := strconv.Atoi(v); err == nil {
				dept.Nivel = parsed
			}
		}
	}

	// Handle optional msg field
	if msg, ok := rawDoc["msg"].(string); ok {
		dept.Msg = &msg
	}

	// Handle optional updated_at field
	if updatedAt, ok := rawDoc["updated_at"]; ok {
		switch v := updatedAt.(type) {
		case primitive.DateTime:
			t := v.Time()
			dept.UpdatedAt = &t
		case time.Time:
			dept.UpdatedAt = &v
		}
	}

	return dept
}
