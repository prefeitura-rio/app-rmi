package services

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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

// GetPetsByCPF retrieves pets associated with a CPF with pagination (merges curated and self-registered)
func (s *PetService) GetPetsByCPF(ctx context.Context, cpf string, page, perPage int) (*models.PaginatedPets, error) {
	// Fetch curated pets from original collection
	curatedCollection := s.database.Collection(config.AppConfig.PetCollection)
	curatedFilter := bson.M{"cpf": cpf}

	curatedCursor, err := curatedCollection.Find(ctx, curatedFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to find curated pets: %w", err)
	}
	defer curatedCursor.Close(ctx)

	var rawCuratedPets []models.RawCitizenPets
	if err = curatedCursor.All(ctx, &rawCuratedPets); err != nil {
		return nil, fmt.Errorf("failed to decode curated pets: %w", err)
	}

	// Convert raw curated pets to individual Pet structs and mark as curated
	var allPets []models.Pet
	for _, rawPet := range rawCuratedPets {
		parsedPet, err := rawPet.ToCitizenPets()
		if err != nil {
			s.logger.Error("failed to parse curated pet data",
				zap.String("cpf", rawPet.CPF),
				zap.Error(err))
			continue
		}
		// Mark all curated pets with source
		for i := range parsedPet.Pets {
			parsedPet.Pets[i].Source = "curated"
		}
		allPets = append(allPets, parsedPet.Pets...)
	}

	// Fetch self-registered pets
	selfRegisteredCollection := s.database.Collection(config.AppConfig.PetsSelfRegisteredCollection)
	selfRegisteredFilter := bson.M{"cpf": cpf}

	selfRegisteredCursor, err := selfRegisteredCollection.Find(ctx, selfRegisteredFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to find self-registered pets: %w", err)
	}
	defer selfRegisteredCursor.Close(ctx)

	var selfRegisteredPets []models.SelfRegisteredPet
	if err = selfRegisteredCursor.All(ctx, &selfRegisteredPets); err != nil {
		return nil, fmt.Errorf("failed to decode self-registered pets: %w", err)
	}

	// Convert self-registered pets and add to the list
	for _, selfPet := range selfRegisteredPets {
		allPets = append(allPets, *selfPet.ToPet())
	}

	// Apply pagination to the merged pet list
	skip := (page - 1) * perPage
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
	totalPets := len(allPets)
	totalPetsPages := int(math.Ceil(float64(totalPets) / float64(perPage)))

	response := &models.PaginatedPets{
		Data: paginatedPets,
	}
	response.Pagination.Page = page
	response.Pagination.PerPage = perPage
	response.Pagination.Total = totalPets
	response.Pagination.TotalPages = totalPetsPages

	s.logger.Debug("retrieved merged pets for CPF",
		zap.String("cpf", cpf),
		zap.Int("page", page),
		zap.Int("per_page", perPage),
		zap.Int("total", totalPets),
		zap.Int("curated", len(allPets)-len(selfRegisteredPets)),
		zap.Int("self_registered", len(selfRegisteredPets)),
		zap.Int("returned", len(paginatedPets)))

	return response, nil
}

// GetPetByID retrieves a specific pet by its ID for a given CPF (searches both collections)
func (s *PetService) GetPetByID(ctx context.Context, cpf string, petID int) (*models.Pet, error) {
	// First, try to find in curated collection
	curatedCollection := s.database.Collection(config.AppConfig.PetCollection)
	curatedFilter := bson.M{"cpf": cpf}

	pipeline := []bson.M{
		{"$match": curatedFilter},
		{"$unwind": "$pet.pet"},
		{"$match": bson.M{"pet.pet.id_animal": petID}},
		{"$replaceRoot": bson.M{"newRoot": "$pet.pet"}},
	}

	cursor, err := curatedCollection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to find curated pet: %w", err)
	}
	defer cursor.Close(ctx)

	var pets []models.Pet
	if err = cursor.All(ctx, &pets); err != nil {
		return nil, fmt.Errorf("failed to decode curated pet: %w", err)
	}

	if len(pets) > 0 {
		pets[0].Source = "curated"
		s.logger.Debug("retrieved curated pet",
			zap.String("cpf", cpf),
			zap.Int("pet_id", petID),
			zap.String("pet_name", pets[0].Name))
		return &pets[0], nil
	}

	// Not found in curated collection, try self-registered
	selfRegisteredCollection := s.database.Collection(config.AppConfig.PetsSelfRegisteredCollection)
	selfRegisteredFilter := bson.M{
		"cpf": cpf,
		"_id": petID,
	}

	var selfPet models.SelfRegisteredPet
	err = selfRegisteredCollection.FindOne(ctx, selfRegisteredFilter).Decode(&selfPet)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("pet not found")
		}
		return nil, fmt.Errorf("failed to find self-registered pet: %w", err)
	}

	pet := selfPet.ToPet()
	s.logger.Debug("retrieved self-registered pet",
		zap.String("cpf", cpf),
		zap.Int("pet_id", petID),
		zap.String("pet_name", pet.Name))

	return pet, nil
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

// RegisterPet registers a new self-registered pet for a CPF
func (s *PetService) RegisterPet(ctx context.Context, cpf string, req *models.PetRegistrationRequest) (*models.Pet, error) {
	collection := s.database.Collection(config.AppConfig.PetsSelfRegisteredCollection)

	// Generate unique ID using timestamp + random ObjectID bytes
	objectID := primitive.NewObjectID()
	petID := int(time.Now().Unix() + int64(objectID.Timestamp().Unix()))

	// Create self-registered pet document
	now := time.Now()
	selfPet := models.SelfRegisteredPet{
		ID:                 petID,
		CPF:                cpf,
		Name:               req.Name,
		MicrochipNumber:    req.MicrochipNumber,
		SexAbbreviation:    req.SexAbbreviation,
		BirthDate:          req.BirthDate,
		NeuteredIndicator:  req.NeuteredIndicator,
		SpeciesName:        req.SpeciesName,
		PedigreeIndicator:  req.PedigreeIndicator,
		PedigreeOriginName: req.PedigreeOriginName,
		BreedName:          req.BreedName,
		SizeName:           req.SizeName,
		PhotoURL:           req.PhotoURL,
		Source:             "self_registered",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// Insert the pet document
	_, err := collection.InsertOne(ctx, selfPet)
	if err != nil {
		return nil, fmt.Errorf("failed to register pet: %w", err)
	}

	s.logger.Info("registered new pet",
		zap.String("cpf", cpf),
		zap.Int("pet_id", petID),
		zap.String("pet_name", req.Name))

	return selfPet.ToPet(), nil
}
