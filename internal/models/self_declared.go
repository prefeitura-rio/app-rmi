package models

import "time"

// SelfDeclaredData represents data that has been self-declared by the citizen
type SelfDeclaredData struct {
	CPF            string     `bson:"cpf" json:"cpf"`
	Endereco       *Endereco  `bson:"endereco,omitempty" json:"endereco,omitempty"`
	Email          *Email     `bson:"email,omitempty" json:"email,omitempty"`
	Telefone       *Telefone  `bson:"telefone,omitempty" json:"telefone,omitempty"`
	TelefonePending *Telefone `bson:"telefone_pending,omitempty" json:"telefone_pending,omitempty"`
	UpdatedAt      time.Time  `bson:"updated_at" json:"updated_at"`
} 