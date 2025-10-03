package services

import (
	"context"
	"fmt"
	"math"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

// PetService handles pet business logic
type PetService struct {
	database *mongo.Database
	logger   *logging.SafeLogger
}

// NewPetService creates a new pet service instance
func NewPetService(database *mongo.Database, logger *logging.SafeLogger) *PetService {
	return &PetService{
		database: database,
		logger:   logger,
	}
}

// Global pet service instance
var PetServiceInstance *PetService

// InitPetService initializes the global pet service instance
func InitPetService() {
	logger := zap.L().Named("pet_service")

	PetServiceInstance = NewPetService(config.MongoDB, &logging.SafeLogger{})

	logger.Info("pet service initialized successfully")
	logger.Info("indexes will be managed by global database maintenance system")
}

// GetPetsByCPF retrieves pets associated with a CPF with pagination
func (s *PetService) GetPetsByCPF(ctx context.Context, cpf string, page, perPage int) (*models.PaginatedPets, error) {
	collection := s.database.Collection(config.AppConfig.PetCollection)

	// Build filter query
	filter := bson.M{
		"cpf": cpf,
	}

	// Calculate pagination (will be applied after parsing)
	skip := (page - 1) * perPage

	// Set up find options with sorting (no pagination yet since we need to flatten first)
	findOptions := options.Find().
		SetSort(bson.D{{Key: "cpf", Value: 1}}) // Sort by CPF

	// Execute query
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to find pets: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode results into raw format first
	var rawPets []models.RawCitizenPets
	if err = cursor.All(ctx, &rawPets); err != nil {
		return nil, fmt.Errorf("failed to decode pets: %w", err)
	}

	// Convert raw pets to individual Pet structs (flatten the structure)
	var allPets []models.Pet
	for _, rawPet := range rawPets {
		parsedPet, err := rawPet.ToCitizenPets()
		if err != nil {
			// Log error but continue with other pets
			s.logger.Error("failed to parse pet data",
				zap.String("cpf", rawPet.CPF),
				zap.Error(err))
			continue
		}
		// Add all pets from this citizen to the flat list
		allPets = append(allPets, parsedPet.Pets...)
	}

	// Apply pagination to the flattened pet list
	start := skip
	end := skip + perPage
	if start >= len(allPets) {
		start = len(allPets)
	}
	if end > len(allPets) {
		end = len(allPets)
	}

	paginatedPets := allPets[start:end]

	// Build paginated response
	response := &models.PaginatedPets{
		Data: paginatedPets,
	}
	// Calculate correct totals based on actual pets count
	totalPets := len(allPets)
	totalPetsPages := int(math.Ceil(float64(totalPets) / float64(perPage)))

	response.Pagination.Page = page
	response.Pagination.PerPage = perPage
	response.Pagination.Total = totalPets
	response.Pagination.TotalPages = totalPetsPages

	s.logger.Debug("retrieved pets for CPF",
		zap.String("cpf", cpf),
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int("total", totalPets),
		zap.Int("returned", len(paginatedPets)))

	return response, nil
}

// GetPetByID retrieves a specific pet by its ID for a given CPF
func (s *PetService) GetPetByID(ctx context.Context, cpf string, petID int) (*models.Pet, error) {
	collection := s.database.Collection(config.AppConfig.PetCollection)

	// Build filter query to find the citizen's pets document
	filter := bson.M{
		"cpf": cpf,
	}

	// Use aggregation to extract the specific pet from the nested structure
	pipeline := []bson.M{
		{"$match": filter},
		{"$unwind": "$pet.pet"},
		{"$match": bson.M{"pet.pet.id_animal": petID}},
		{"$replaceRoot": bson.M{"newRoot": "$pet.pet"}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to find pet: %w", err)
	}
	defer cursor.Close(ctx)

	var pets []models.Pet
	if err = cursor.All(ctx, &pets); err != nil {
		return nil, fmt.Errorf("failed to decode pet: %w", err)
	}

	if len(pets) == 0 {
		return nil, fmt.Errorf("pet not found")
	}

	s.logger.Debug("retrieved specific pet",
		zap.String("cpf", cpf),
		zap.Int("pet_id", petID),
		zap.String("pet_name", pets[0].Name))

	return &pets[0], nil
}

// GetPetStats retrieves the pet statistics for a CPF
func (s *PetService) GetPetStats(ctx context.Context, cpf string) (*models.PetStatsResponse, error) {
	collection := s.database.Collection(config.AppConfig.PetCollection)

	// Build filter query
	filter := bson.M{
		"cpf": cpf,
	}

	// Execute query for first document
	var rawPet models.RawCitizenPets
	err := collection.FindOne(ctx, filter).Decode(&rawPet)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &models.PetStatsResponse{
				CPF:        cpf,
				Statistics: nil,
			}, nil
		}
		return nil, fmt.Errorf("failed to find pet statistics: %w", err)
	}

	// Extract statistics (prioritize nested over root)
	var stats *models.Statistics
	if rawPet.PetData != nil && rawPet.PetData.Statistics != nil {
		stats = rawPet.PetData.Statistics
	} else {
		stats = rawPet.Statistics
	}

	response := &models.PetStatsResponse{
		CPF:        cpf,
		Statistics: stats,
	}

	s.logger.Debug("retrieved pet statistics",
		zap.String("cpf", cpf),
		zap.Bool("has_stats", stats != nil))

	return response, nil
}
