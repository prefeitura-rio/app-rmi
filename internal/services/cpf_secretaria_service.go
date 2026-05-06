package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

type CPFSecretariaService struct {
	database *mongo.Database
}

func NewCPFSecretariaService(database *mongo.Database) *CPFSecretariaService {
	return &CPFSecretariaService{database: database}
}

var CPFSecretariaServiceInstance *CPFSecretariaService

func InitCPFSecretariaService() {
	CPFSecretariaServiceInstance = NewCPFSecretariaService(config.MongoDB)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coll := config.MongoDB.Collection(config.AppConfig.CPFSecretariaCollection)
	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "cpf", Value: 1}}},
		{
			Keys:    bson.D{{Key: "cpf", Value: 1}, {Key: "cd_ua", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}
	if _, err := coll.Indexes().CreateMany(ctx, indexes); err != nil {
		zap.L().Warn("cpf_secretaria: failed to create indexes", zap.Error(err))
	}
}

func normalizeCPF(cpf string) string {
	cpf = strings.ReplaceAll(cpf, ".", "")
	cpf = strings.ReplaceAll(cpf, "-", "")
	return strings.TrimSpace(cpf)
}

func (s *CPFSecretariaService) ListByCPF(ctx context.Context, cpf string) ([]models.CPFSecretariaMapping, error) {
	coll := s.database.Collection(config.AppConfig.CPFSecretariaCollection)
	cursor, err := coll.Find(ctx, bson.M{"cpf": normalizeCPF(cpf)})
	if err != nil {
		return nil, fmt.Errorf("cpf_secretaria: list: %w", err)
	}
	defer cursor.Close(ctx)

	var mappings []models.CPFSecretariaMapping
	if err := cursor.All(ctx, &mappings); err != nil {
		return nil, fmt.Errorf("cpf_secretaria: decode: %w", err)
	}
	return mappings, nil
}

func (s *CPFSecretariaService) AddMapping(ctx context.Context, cpf, cdUA, createdBy string) (*models.CPFSecretariaMapping, error) {
	coll := s.database.Collection(config.AppConfig.CPFSecretariaCollection)
	now := time.Now()
	mapping := models.CPFSecretariaMapping{
		ID:        primitive.NewObjectID(),
		CPF:       normalizeCPF(cpf),
		CdUA:      cdUA,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: createdBy,
	}

	_, err := coll.InsertOne(ctx, mapping)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("cpf_secretaria: mapping already exists for CPF %s and cd_ua %s", cpf, cdUA)
		}
		return nil, fmt.Errorf("cpf_secretaria: insert: %w", err)
	}
	return &mapping, nil
}

func (s *CPFSecretariaService) RemoveMapping(ctx context.Context, cpf, cdUA string) error {
	coll := s.database.Collection(config.AppConfig.CPFSecretariaCollection)
	result, err := coll.DeleteOne(ctx, bson.M{"cpf": normalizeCPF(cpf), "cd_ua": cdUA})
	if err != nil {
		return fmt.Errorf("cpf_secretaria: delete: %w", err)
	}
	if result.DeletedCount == 0 {
		return fmt.Errorf("cpf_secretaria: mapping not found for CPF %s and cd_ua %s", cpf, cdUA)
	}
	return nil
}

func (s *CPFSecretariaService) GetCdUAsByCPF(ctx context.Context, cpf string) ([]string, error) {
	mappings, err := s.ListByCPF(ctx, cpf)
	if err != nil {
		return nil, err
	}
	cdUAs := make([]string, 0, len(mappings))
	for _, m := range mappings {
		cdUAs = append(cdUAs, m.CdUA)
	}
	return cdUAs, nil
}
