package models

import "time"

// Nascimento represents birth information
type Nascimento struct {
	Data        *time.Time `json:"data,omitempty" bson:"data,omitempty"`
	MunicipioID *string    `json:"municipio_id,omitempty" bson:"municipio_id,omitempty"`
	Municipio   *string    `json:"municipio,omitempty" bson:"municipio,omitempty"`
	UF          *string    `json:"uf,omitempty" bson:"uf,omitempty"`
	PaisID      *string    `json:"pais_id,omitempty" bson:"pais_id,omitempty"`
	Pais        *string    `json:"pais,omitempty" bson:"pais,omitempty"`
}

// Mae represents mother's information
type Mae struct {
	Nome *string `json:"nome,omitempty" bson:"nome,omitempty"`
	CPF  *string `json:"cpf,omitempty" bson:"cpf,omitempty"`
}

// Obito represents death information
type Obito struct {
	Indicador *bool  `json:"indicador,omitempty" bson:"indicador,omitempty"`
	Ano       *int32 `json:"ano,omitempty" bson:"ano,omitempty"`
}

// Documentos represents document information
type Documentos struct {
	CNS []string `json:"cns,omitempty" bson:"cns,omitempty"`
}

// EnderecoPrincipal represents the main address
type EnderecoPrincipal struct {
	Origem        *string `json:"origem,omitempty" bson:"origem,omitempty"`
	Sistema       *string `json:"sistema,omitempty" bson:"sistema,omitempty"`
	CEP           *string `json:"cep,omitempty" bson:"cep,omitempty"`
	Estado        *string `json:"estado,omitempty" bson:"estado,omitempty"`
	Municipio     *string `json:"municipio,omitempty" bson:"municipio,omitempty"`
	TipoLogradouro *string `json:"tipo_logradouro,omitempty" bson:"tipo_logradouro,omitempty"`
	Logradouro    *string `json:"logradouro,omitempty" bson:"logradouro,omitempty"`
	Numero        *string `json:"numero,omitempty" bson:"numero,omitempty"`
	Complemento   *string `json:"complemento,omitempty" bson:"complemento,omitempty"`
	Bairro        *string `json:"bairro,omitempty" bson:"bairro,omitempty"`
}

// Endereco represents address information
type Endereco struct {
	Indicador  *bool            `json:"indicador,omitempty" bson:"indicador,omitempty"`
	Principal  *EnderecoPrincipal `json:"principal,omitempty" bson:"principal,omitempty"`
	Alternativo []int32         `json:"alternativo,omitempty" bson:"alternativo,omitempty"`
}

// EmailPrincipal represents the main email
type EmailPrincipal struct {
	Origem  *string `json:"origem,omitempty" bson:"origem,omitempty"`
	Sistema *string `json:"sistema,omitempty" bson:"sistema,omitempty"`
	Valor   *string `json:"valor,omitempty" bson:"valor,omitempty"`
}

// Email represents email information
type Email struct {
	Indicador  *bool          `json:"indicador,omitempty" bson:"indicador,omitempty"`
	Principal  *EmailPrincipal `json:"principal,omitempty" bson:"principal,omitempty"`
	Alternativo []int32       `json:"alternativo,omitempty" bson:"alternativo,omitempty"`
}

// TelefonePrincipal represents the main phone
type TelefonePrincipal struct {
	Origem  *string `json:"origem,omitempty" bson:"origem,omitempty"`
	Sistema *string `json:"sistema,omitempty" bson:"sistema,omitempty"`
	DDI     *string `json:"ddi,omitempty" bson:"ddi,omitempty"`
	DDD     *string `json:"ddd,omitempty" bson:"ddd,omitempty"`
	Valor   *string `json:"valor,omitempty" bson:"valor,omitempty"`
}

// Telefone represents phone information
type Telefone struct {
	Indicador  *bool             `json:"indicador,omitempty" bson:"indicador,omitempty"`
	Principal  *TelefonePrincipal `json:"principal,omitempty" bson:"principal,omitempty"`
	Alternativo []int32          `json:"alternativo,omitempty" bson:"alternativo,omitempty"`
}

// ClinicaFamilia represents family clinic information
type ClinicaFamilia struct {
	Indicador *bool   `json:"indicador,omitempty" bson:"indicador,omitempty"`
	IDCNES    *string `json:"id_cnes,omitempty" bson:"id_cnes,omitempty"`
	Nome      *string `json:"nome,omitempty" bson:"nome,omitempty"`
	Telefone  *string `json:"telefone,omitempty" bson:"telefone,omitempty"`
}

// EquipeSaudeFamilia represents family health team information
type EquipeSaudeFamilia struct {
	Indicador *bool                `json:"indicador,omitempty" bson:"indicador,omitempty"`
	IDINE     *string              `json:"id_ine,omitempty" bson:"id_ine,omitempty"`
	Nome      *string              `json:"nome,omitempty" bson:"nome,omitempty"`
	Telefone  *string              `json:"telefone,omitempty" bson:"telefone,omitempty"`
}

// Saude represents health information
type Saude struct {
	ClinicaFamilia      *ClinicaFamilia      `json:"clinica_familia,omitempty" bson:"clinica_familia,omitempty"`
	EquipeSaudeFamilia  *EquipeSaudeFamilia  `json:"equipe_saude_familia,omitempty" bson:"equipe_saude_familia,omitempty"`
}

// Datalake represents datalake information
type Datalake struct {
	LastUpdated *time.Time `json:"last_updated,omitempty" bson:"last_updated,omitempty"`
}

// Citizen represents the complete citizen data
type Citizen struct {
	CPF          string     `json:"cpf" bson:"cpf"`
	Nome         *string    `json:"nome,omitempty" bson:"nome,omitempty"`
	NomeSocial   *string    `json:"nome_social,omitempty" bson:"nome_social,omitempty"`
	Sexo         *string    `json:"sexo,omitempty" bson:"sexo,omitempty"`
	Nascimento   *Nascimento `json:"nascimento,omitempty" bson:"nascimento,omitempty"`
	Mae          *Mae       `json:"mae,omitempty" bson:"mae,omitempty"`
	MenorIdade   *bool      `json:"menor_idade,omitempty" bson:"menor_idade,omitempty"`
	Raca         *string    `json:"raca,omitempty" bson:"raca,omitempty"`
	Obito        *Obito     `json:"obito,omitempty" bson:"obito,omitempty"`
	Documentos   *Documentos `json:"documentos,omitempty" bson:"documentos,omitempty"`
	Endereco     *Endereco  `json:"endereco,omitempty" bson:"endereco,omitempty"`
	Email        *Email     `json:"email,omitempty" bson:"email,omitempty"`
	Telefone     *Telefone  `json:"telefone,omitempty" bson:"telefone,omitempty"`
	Saude        *Saude     `json:"saude,omitempty" bson:"saude,omitempty"`
	Datalake     *Datalake  `json:"datalake,omitempty" bson:"datalake,omitempty"`
	CPFParticao  int64      `json:"cpf_particao" bson:"cpf_particao"`
	RowNumber    *int32     `json:"row_number,omitempty" bson:"row_number,omitempty"`
} 