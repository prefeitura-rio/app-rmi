package models

import "time"

// SelfDeclaredData represents data that has been self-declared by the citizen
type SelfDeclaredData struct {
	CPF             string    `bson:"cpf" json:"cpf"`
	Endereco        *Endereco `bson:"endereco,omitempty" json:"endereco"`
	Email           *Email    `bson:"email,omitempty" json:"email"`
	Telefone        *Telefone `bson:"telefone,omitempty" json:"telefone"`
	TelefonePending *Telefone `bson:"telefone_pending,omitempty" json:"telefone_pending"`
	Raca            *string   `bson:"raca,omitempty" json:"raca"`
	Version         int32     `bson:"version,omitempty" json:"version,omitempty"`
	UpdatedAt       time.Time `bson:"updated_at" json:"updated_at"`
}
