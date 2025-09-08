package models

import (
	"time"
)

// LegalEntity represents a legal entity (Pessoa Jurídica) document
type LegalEntity struct {
	ID                    interface{}           `bson:"_id,omitempty" json:"_id"`
	CNPJ                  string                `bson:"cnpj" json:"cnpj"`
	CompanyName           string                `bson:"razao_social" json:"razao_social"`
	TradeName             *string               `bson:"nome_fantasia" json:"nome_fantasia"`
	ShareCapital          float64               `bson:"capital_social" json:"capital_social"`
	PrimaryCNAE           *string               `bson:"cnae_fiscal" json:"cnae_fiscal"`
	SecondaryCNAEs        *[]string             `bson:"cnae_secundarias" json:"cnae_secundarias"`
	NIRE                  *string               `bson:"nire" json:"nire"`
	LegalNature           LegalNature           `bson:"natureza_juridica" json:"natureza_juridica"`
	CompanySize           CompanySize           `bson:"porte" json:"porte"`
	HeadquartersBranch    HeadquartersBranch    `bson:"matriz_filial" json:"matriz_filial"`
	RegistrationAuthority RegistrationAuthority `bson:"orgao_registro" json:"orgao_registro"`
	ActivityStartDate     time.Time             `bson:"inicio_atividade_data" json:"inicio_atividade_data"`
	RegistrationStatus    RegistrationStatus    `bson:"situacao_cadastral" json:"situacao_cadastral"`
	SpecialStatus         SpecialStatus         `bson:"situacao_especial" json:"situacao_especial"`
	FederativeEntity      FederativeEntity      `bson:"ente_federativo" json:"ente_federativo"`
	Contact               LegalEntityContact    `bson:"contato" json:"contato"`
	Address               LegalEntityAddress    `bson:"endereco" json:"endereco"`
	Accountant            Accountant            `bson:"contador" json:"contador"`
	ResponsiblePerson     ResponsiblePerson     `bson:"responsavel" json:"responsavel"`
	UnitTypes             []string              `bson:"tipos_unidade" json:"tipos_unidade"`
	OperationMethods      []string              `bson:"formas_atuacao" json:"formas_atuacao"`
	PartnersCount         int                   `bson:"socios_quantidade" json:"socios_quantidade"`
	Partners              []Partner             `bson:"socios" json:"socios"`
	Successions           []interface{}         `bson:"sucessoes" json:"sucessoes"`
	Language              *string               `bson:"language" json:"language"`
}

// LegalNature represents the legal nature classification
type LegalNature struct {
	ID          string `bson:"id" json:"id"`
	Description string `bson:"descricao" json:"descricao"`
}

// CompanySize represents the company size classification
type CompanySize struct {
	ID          string `bson:"id" json:"id"`
	Description string `bson:"descricao" json:"descricao"`
}

// HeadquartersBranch represents whether it's a headquarters or branch
type HeadquartersBranch struct {
	ID          string `bson:"id" json:"id"`
	Description string `bson:"descricao" json:"descricao"`
}

// RegistrationAuthority represents the registration authority
type RegistrationAuthority struct {
	ID          *string `bson:"id" json:"id"`
	Description *string `bson:"descricao" json:"descricao"`
}

// RegistrationStatus represents the registration status
type RegistrationStatus struct {
	ID                string    `bson:"id" json:"id"`
	Description       string    `bson:"descricao" json:"descricao"`
	Date              time.Time `bson:"data" json:"data"`
	ReasonID          string    `bson:"motivo_id" json:"motivo_id"`
	ReasonDescription string    `bson:"motivo_descricao" json:"motivo_descricao"`
}

// SpecialStatus represents special status information
type SpecialStatus struct {
	Description *string    `bson:"descricao" json:"descricao"`
	Date        *time.Time `bson:"data" json:"data"`
}

// FederativeEntity represents federative entity information
type FederativeEntity struct {
	ID   *string `bson:"id" json:"id"`
	Type *string `bson:"tipo" json:"tipo"`
}

// LegalEntityContact represents contact information for legal entity
type LegalEntityContact struct {
	Phones []LegalEntityPhone `bson:"telefone" json:"telefone"`
	Email  *string            `bson:"email" json:"email"`
}

// LegalEntityPhone represents phone number for legal entity
type LegalEntityPhone struct {
	AreaCode string `bson:"ddd" json:"ddd"`
	Number   string `bson:"telefone" json:"telefone"`
}

// LegalEntityAddress represents address information for legal entity
type LegalEntityAddress struct {
	ZipCode         string  `bson:"cep" json:"cep"`
	CountryID       *string `bson:"id_pais" json:"id_pais"`
	State           string  `bson:"uf" json:"uf"`
	CityID          string  `bson:"id_municipio" json:"id_municipio"`
	CityName        string  `bson:"municipio_nome" json:"municipio_nome"`
	ForeignCityName *string `bson:"municipio_exterior_nome" json:"municipio_exterior_nome"`
	Neighborhood    string  `bson:"bairro" json:"bairro"`
	StreetType      string  `bson:"tipo_logradouro" json:"tipo_logradouro"`
	Street          string  `bson:"logradouro" json:"logradouro"`
	Number          string  `bson:"numero" json:"numero"`
	Complement      string  `bson:"complemento" json:"complemento"`
}

// Accountant represents accountant information
type Accountant struct {
	Individual AccountantIndividual `bson:"pf" json:"pf"`
	Corporate  AccountantCorporate  `bson:"pj" json:"pj"`
}

// AccountantIndividual represents individual accountant information
type AccountantIndividual struct {
	CRCType           string `bson:"tipo_crc" json:"tipo_crc"`
	CRCClassification string `bson:"classificacao_crc" json:"classificacao_crc"`
	CRCSequential     string `bson:"sequencial_crc" json:"sequencial_crc"`
	ID                string `bson:"id" json:"id"`
}

// AccountantCorporate represents corporate accountant information
type AccountantCorporate struct {
	ID                *string `bson:"id" json:"id"`
	CRCType           *string `bson:"tipo_crc" json:"tipo_crc"`
	CRCClassification *string `bson:"classificacao_crc" json:"classificacao_crc"`
	CRCSequential     *string `bson:"sequencial_crc" json:"sequencial_crc"`
}

// ResponsiblePerson represents the responsible person for the legal entity
type ResponsiblePerson struct {
	CPF                      string    `bson:"cpf" json:"cpf"`
	QualificationID          string    `bson:"qualificacao_id" json:"qualificacao_id"`
	QualificationDescription string    `bson:"qualificacao_descricao" json:"qualificacao_descricao"`
	InclusionDate            time.Time `bson:"inclusao_data" json:"inclusao_data"`
}

// Partner represents a partner/shareholder of the legal entity
type Partner struct {
	CountryCode                      *string   `bson:"codigo_pais" json:"codigo_pais"`
	PartnerCPF                       *string   `bson:"cpf_socio" json:"cpf_socio"`
	PartnerCNPJ                      *string   `bson:"cnpj_socio" json:"cnpj_socio"`
	LegalRepresentativeCPF           *string   `bson:"cpf_representante_legal" json:"cpf_representante_legal"`
	SpecialStatusDate                time.Time `bson:"data_situacao_especial" json:"data_situacao_especial"`
	ForeignPartnerName               *string   `bson:"nome_socio_estrangeiro" json:"nome_socio_estrangeiro"`
	LegalRepresentativeQualification *string   `bson:"qualificacao_representante_legal" json:"qualificacao_representante_legal"`
	PartnerQualification             string    `bson:"qualificacao_socio" json:"qualificacao_socio"`
	Type                             string    `bson:"tipo" json:"tipo"`
}

// PaginatedLegalEntities represents a paginated response of legal entities
type PaginatedLegalEntities struct {
	Data       []LegalEntity `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
}
