package models

import "time"

// SelfDeclaredData represents data that has been self-declared by the citizen
type SelfDeclaredData struct {
	CPF                    string     `bson:"cpf" json:"cpf"`
	Endereco               *Endereco  `bson:"endereco,omitempty" json:"endereco"`
	Email                  *Email     `bson:"email,omitempty" json:"email"`
	Telefone               *Telefone  `bson:"telefone,omitempty" json:"telefone"`
	TelefonePending        *Telefone  `bson:"telefone_pending,omitempty" json:"telefone_pending"`
	Raca                   *string    `bson:"raca,omitempty" json:"raca"`
	NomeExibicao           *string    `bson:"nome_exibicao,omitempty" json:"nome_exibicao"`
	Genero                 *string    `bson:"genero,omitempty" json:"genero"`
	RendaFamiliar          *string    `bson:"renda_familiar,omitempty" json:"renda_familiar"`
	Escolaridade           *string    `bson:"escolaridade,omitempty" json:"escolaridade"`
	Deficiencia            *string    `bson:"deficiencia,omitempty" json:"deficiencia"`
	Nacionalidade          *string    `bson:"nacionalidade,omitempty" json:"nacionalidade,omitempty"`
	Idioma                 []string   `bson:"idioma,omitempty" json:"idioma,omitempty"`
	Passaporte             *string    `bson:"passaporte,omitempty" json:"passaporte,omitempty"`
	IsTourist              *bool      `bson:"is_tourist,omitempty" json:"is_tourist,omitempty"`
	Complemento            *string    `bson:"complemento,omitempty" json:"complemento,omitempty"`
	TipoTelefone1          *string    `bson:"tipo_telefone1,omitempty" json:"tipo_telefone1,omitempty"`
	TipoTelefone2          *string    `bson:"tipo_telefone2,omitempty" json:"tipo_telefone2,omitempty"`
	TipoTelefone3          *string    `bson:"tipo_telefone3,omitempty" json:"tipo_telefone3,omitempty"`
	TelefoneInternacional  *string    `bson:"telefone_internacional,omitempty" json:"telefone_internacional,omitempty"`
	CanalOrigem            *string    `bson:"canal_origem,omitempty" json:"canal_origem,omitempty"`
	CanalUltimaModificacao *string    `bson:"canal_ultima_modificacao,omitempty" json:"canal_ultima_modificacao,omitempty"`
	SalesforceAccountID    *string    `bson:"salesforce_account_id,omitempty" json:"salesforce_account_id,omitempty"`
	SalesforceAnonymized   bool       `bson:"salesforce_anonymized,omitempty" json:"salesforce_anonymized,omitempty"`
	SalesforceAnonymizedAt *time.Time `bson:"salesforce_anonymized_at,omitempty" json:"salesforce_anonymized_at,omitempty"`
	SalesforceSyncedAt     *time.Time `bson:"salesforce_synced_at,omitempty" json:"salesforce_synced_at,omitempty"`
	SalesforceUpdatedAt    *time.Time `bson:"salesforce_updated_at,omitempty" json:"salesforce_updated_at,omitempty"`
	Version                int32      `bson:"version,omitempty" json:"version,omitempty"`
	UpdatedAt              time.Time  `bson:"updated_at" json:"updated_at"`
}
