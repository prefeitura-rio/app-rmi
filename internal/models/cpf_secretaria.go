package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CPFSecretariaMapping struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CPF       string             `bson:"cpf"           json:"cpf"`
	CdUA      string             `bson:"cd_ua"         json:"cd_ua"`
	CreatedAt time.Time          `bson:"created_at"    json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"    json:"updated_at"`
	CreatedBy string             `bson:"created_by"    json:"created_by"`
}

type CPFSecretariaResponse struct {
	ID        string    `json:"id"`
	CPF       string    `json:"cpf"`
	CdUA      string    `json:"cd_ua"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedBy string    `json:"created_by"`
}

func (m *CPFSecretariaMapping) ToResponse() CPFSecretariaResponse {
	return CPFSecretariaResponse{
		ID:        m.ID.Hex(),
		CPF:       m.CPF,
		CdUA:      m.CdUA,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		CreatedBy: m.CreatedBy,
	}
}

type AddCPFSecretariaRequest struct {
	CdUA string `json:"cd_ua" binding:"required"`
}

type CPFSecretariaListResponse struct {
	CPF      string                  `json:"cpf"`
	Mappings []CPFSecretariaResponse `json:"mappings"`
}

type CPFSecretariaQueryResponse struct {
	CPF   string   `json:"cpf"`
	CdUAs []string `json:"cd_uas"`
}
