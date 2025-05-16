package models

// Origens represents the origins of the citizen data
type Origens struct {
	Origem     *string `json:"origem,omitempty" bson:"origem,omitempty"`
	DataOrigem *string `json:"data_origem,omitempty" bson:"data_origem,omitempty"`
}

// Endereco represents an address
type Endereco struct {
	Logradouro    *string `json:"logradouro,omitempty" bson:"logradouro,omitempty"`
	Numero        *string `json:"numero,omitempty" bson:"numero,omitempty"`
	Complemento   *string `json:"complemento,omitempty" bson:"complemento,omitempty"`
	Bairro        *string `json:"bairro,omitempty" bson:"bairro,omitempty"`
	Cidade        *string `json:"cidade,omitempty" bson:"cidade,omitempty"`
	UF            *string `json:"uf,omitempty" bson:"uf,omitempty"`
	CEP           *string `json:"cep,omitempty" bson:"cep,omitempty"`
	CodigoIBGE    *string `json:"codigo_ibge,omitempty" bson:"codigo_ibge,omitempty"`
	TipoEndereco  *string `json:"tipo_endereco,omitempty" bson:"tipo_endereco,omitempty"`
	Latitude      *string `json:"latitude,omitempty" bson:"latitude,omitempty"`
	Longitude     *string `json:"longitude,omitempty" bson:"longitude,omitempty"`
}

// Telefone represents a phone number
type Telefone struct {
	DDD         *string `json:"ddd,omitempty" bson:"ddd,omitempty"`
	Numero      *string `json:"numero,omitempty" bson:"numero,omitempty"`
	Tipo        *string `json:"tipo,omitempty" bson:"tipo,omitempty"`
	Observacoes *string `json:"observacoes,omitempty" bson:"observacoes,omitempty"`
}

// Email represents an email address
type Email struct {
	Email       *string `json:"email,omitempty" bson:"email,omitempty"`
	Tipo        *string `json:"tipo,omitempty" bson:"tipo,omitempty"`
	Observacoes *string `json:"observacoes,omitempty" bson:"observacoes,omitempty"`
}

// Contato represents contact information
type Contato struct {
	Telefones []Telefone `json:"telefones,omitempty" bson:"telefones,omitempty"`
	Emails    []Email    `json:"emails,omitempty" bson:"emails,omitempty"`
}

// Fazenda represents tax-related information
type Fazenda struct {
	InscricaoEstadual *string `json:"inscricao_estadual,omitempty" bson:"inscricao_estadual,omitempty"`
	NomeFazenda       *string `json:"nome_fazenda,omitempty" bson:"nome_fazenda,omitempty"`
}

// Profissional represents a health professional
type Profissional struct {
	Profissao    *string `json:"profissao,omitempty" bson:"profissao,omitempty"`
	Empresa      *string `json:"empresa,omitempty" bson:"empresa,omitempty"`
	Cargo        *string `json:"cargo,omitempty" bson:"cargo,omitempty"`
	DataAdmissao *string `json:"data_admissao,omitempty" bson:"data_admissao,omitempty"`
}

// ClinicaFamilia represents a family clinic
type ClinicaFamilia struct {
	Nome     *string `json:"nome,omitempty" bson:"nome,omitempty"`
	Codigo   *string `json:"codigo,omitempty" bson:"codigo,omitempty"`
	AP       *string `json:"ap,omitempty" bson:"ap,omitempty"`
	CAP      *string `json:"cap,omitempty" bson:"cap,omitempty"`
	Endereco *string `json:"endereco,omitempty" bson:"endereco,omitempty"`
}

// EquipeSaudeFamilia represents a family health team
type EquipeSaudeFamilia struct {
	Nome          *string `json:"nome,omitempty" bson:"nome,omitempty"`
	Codigo        *string `json:"codigo,omitempty" bson:"codigo,omitempty"`
	Microarea     *string `json:"microarea,omitempty" bson:"microarea,omitempty"`
	AreaAtuacao   *string `json:"area_atuacao,omitempty" bson:"area_atuacao,omitempty"`
	AgenteVinculo *string `json:"agente_vinculo,omitempty" bson:"agente_vinculo,omitempty"`
}

// Saude represents health-related information
type Saude struct {
	ClinicaFamilia      *ClinicaFamilia      `json:"clinica_familia,omitempty" bson:"clinica_familia,omitempty"`
	EquipeSaudeFamilia  *EquipeSaudeFamilia  `json:"equipe_saude_familia,omitempty" bson:"equipe_saude_familia,omitempty"`
	NumeroCartaoSUS     *string              `json:"numero_cartao_sus,omitempty" bson:"numero_cartao_sus,omitempty"`
}

// Citizen represents the complete citizen data
type Citizen struct {
	CPF         string     `json:"cpf" bson:"cpf"`
	Nome        *string    `json:"nome,omitempty" bson:"nome,omitempty"`
	Origens     []Origens  `json:"origens,omitempty" bson:"origens,omitempty"`
	Endereco    *Endereco  `json:"endereco,omitempty" bson:"endereco,omitempty"`
	Contato     *Contato   `json:"contato,omitempty" bson:"contato,omitempty"`
	Fazenda     *Fazenda   `json:"fazenda,omitempty" bson:"fazenda,omitempty"`
	Profissional *Profissional `json:"profissional,omitempty" bson:"profissional,omitempty"`
	Saude       *Saude     `json:"saude,omitempty" bson:"saude,omitempty"`
}

// SelfDeclaredData represents the data that can be self-declared
type SelfDeclaredData struct {
	CPF      string    `json:"cpf" bson:"cpf"`
	Endereco *Endereco `json:"endereco,omitempty" bson:"endereco,omitempty"`
	Contato  *Contato  `json:"contato,omitempty" bson:"contato,omitempty"`
} 