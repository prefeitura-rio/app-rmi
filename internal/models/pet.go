package models

import (
	"time"
)

// Pet represents a single pet record
type Pet struct {
	Indicator            *bool             `bson:"indicador,omitempty" json:"indicador,omitempty"`
	ID                   *int              `bson:"id_animal,omitempty" json:"id_animal,omitempty"`
	Name                 string            `bson:"animal_nome,omitempty" json:"animal_nome,omitempty"`
	MicrochipNumber      string            `bson:"microchip_numero,omitempty" json:"microchip_numero,omitempty"`
	SexAbbreviation      string            `bson:"sexo_sigla,omitempty" json:"sexo_sigla,omitempty"`
	BirthDate            *time.Time        `bson:"nascimento_data,omitempty" json:"nascimento_data,omitempty"`
	NeuteredIndicator    *bool             `bson:"indicador_castrado,omitempty" json:"indicador_castrado,omitempty"`
	ActiveIndicator      *bool             `bson:"indicador_ativo,omitempty" json:"indicador_ativo,omitempty"`
	SpeciesName          string            `bson:"especie_nome,omitempty" json:"especie_nome,omitempty"`
	PedigreeIndicator    *bool             `bson:"pedigree_indicador,omitempty" json:"pedigree_indicador,omitempty"`
	PedigreeOriginName   string            `bson:"pedigree_origem_nome,omitempty" json:"pedigree_origem_nome,omitempty"`
	LifeStageName        string            `bson:"fase_vida_nome,omitempty" json:"fase_vida_nome,omitempty"`
	BreedName            string            `bson:"raca_nome,omitempty" json:"raca_nome,omitempty"`
	SizeName             string            `bson:"porte_nome,omitempty" json:"porte_nome,omitempty"`
	QRCodePayload        string            `bson:"qrcode_payload,omitempty" json:"qrcode_payload,omitempty"`
	PhotoURL             string            `bson:"foto_url,omitempty" json:"foto_url,omitempty"`
	RegistrationDate     *time.Time        `bson:"registro_data,omitempty" json:"registro_data,omitempty"`
	AntiRabiesDate       *time.Time        `bson:"antirrabica_data,omitempty" json:"antirrabica_data,omitempty"`
	AntiRabiesExpiryDate *time.Time        `bson:"antirrabica_validade_data,omitempty" json:"antirrabica_validade_data,omitempty"`
	DewormingDate        *time.Time        `bson:"vermifugacao_data,omitempty" json:"vermifugacao_data,omitempty"`
	DewormingExpiryDate  *time.Time        `bson:"vermifugacao_validade_data,omitempty" json:"vermifugacao_validade_data,omitempty"`
	AccreditedClinic     *AccreditedClinic `bson:"clinica_credenciada,omitempty" json:"clinica_credenciada,omitempty"`
	Source               string            `bson:"source,omitempty" json:"source,omitempty"` // "curated" or "self_registered"
}

// Statistics represents pet statistics for a citizen
type Statistics struct {
	DogCount   int `bson:"quantidade_cachorro" json:"quantidade_cachorro"`
	CatCount   int `bson:"quantidade_gato" json:"quantidade_gato"`
	OtherCount int `bson:"quantidade_outro" json:"quantidade_outro"`
}

// ClinicAddress represents the address of an accredited clinic
type ClinicAddress struct {
	Street       string `bson:"logradouro" json:"logradouro"`
	Number       string `bson:"numero" json:"numero"`
	Complement   string `bson:"complemento" json:"complemento"`
	Neighborhood string `bson:"bairro" json:"bairro"`
	City         string `bson:"cidade" json:"cidade"`
}

// AccreditedClinic represents an accredited veterinary clinic
type AccreditedClinic struct {
	Name    string        `bson:"nome" json:"nome"`
	Phone   string        `bson:"telefone" json:"telefone"`
	Mobile  string        `bson:"celular" json:"celular"`
	Email   string        `bson:"email" json:"email"`
	Address ClinicAddress `bson:"endereco" json:"endereco"`
}

// CitizenPets represents the complete pets document for a citizen
type CitizenPets struct {
	ID           interface{} `bson:"_id,omitempty" json:"_id"`
	CPF          string      `bson:"cpf" json:"cpf"`
	Pets         []Pet       `bson:"pet,omitempty" json:"pet,omitempty"`
	Statistics   *Statistics `bson:"estatisticas,omitempty" json:"estatisticas,omitempty"`
	CPFPartition *int64      `bson:"cpf_particao,omitempty" json:"cpf_particao,omitempty"`
}

// RawCitizenPets represents the raw data structure from MongoDB (with nested pet structure)
type RawCitizenPets struct {
	ID           interface{}    `bson:"_id,omitempty"`
	CPF          string         `bson:"cpf"`
	PetData      *NestedPetData `bson:"pet,omitempty"`
	Statistics   *Statistics    `bson:"estatisticas,omitempty"`
	CPFPartition *int64         `bson:"cpf_particao,omitempty"`
}

// NestedPetData represents the nested pet object structure
type NestedPetData struct {
	CPF          string      `bson:"cpf"`
	Pets         []Pet       `bson:"pet"`
	Statistics   *Statistics `bson:"estatisticas,omitempty"`
	CPFPartition *int64      `bson:"cpf_particao,omitempty"`
}

// PaginatedPets represents a paginated response of pets data
type PaginatedPets struct {
	Data       []Pet `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
}

// PetStatsResponse represents the statistics for a citizen's pets
type PetStatsResponse struct {
	CPF                     string      `json:"cpf"`
	Statistics              *Statistics `json:"statistics"`                 // Curated pets statistics from governo
	SelfRegisteredPetsCount int         `json:"self_registered_pets_count"` // Count of non-curated self-registered pets
}

// ParseKeyValuePairs converts key-value pair structure to a map
func ParseKeyValuePairs(kvPairs interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	if kvSlice, ok := kvPairs.([]interface{}); ok {
		for _, item := range kvSlice {
			if kvMap, ok := item.(map[string]interface{}); ok {
				if key, keyOk := kvMap["Key"].(string); keyOk {
					result[key] = kvMap["Value"]
				}
			}
		}
	}

	return result
}

// ToCitizenPets converts raw nested pet data to CitizenPets struct
func (raw *RawCitizenPets) ToCitizenPets() (*CitizenPets, error) {
	result := &CitizenPets{
		ID:           raw.ID,
		CPF:          raw.CPF,
		Statistics:   raw.Statistics,
		CPFPartition: raw.CPFPartition,
		Pets:         []Pet{},
	}

	// Extract pets from the nested PetData structure
	if raw.PetData != nil {
		result.Pets = raw.PetData.Pets

		// Use nested statistics if it exists and root one is nil
		if result.Statistics == nil && raw.PetData.Statistics != nil {
			result.Statistics = raw.PetData.Statistics
		}
		if result.CPFPartition == nil && raw.PetData.CPFPartition != nil {
			result.CPFPartition = raw.PetData.CPFPartition
		}
	}

	return result, nil
}

// PetRegistrationRequest represents the request to register a new pet
type PetRegistrationRequest struct {
	Name               string     `json:"animal_nome" binding:"required"`
	MicrochipNumber    string     `json:"microchip_numero,omitempty"`
	SexAbbreviation    string     `json:"sexo_sigla" binding:"required,oneof=M F"`
	BirthDate          *time.Time `json:"nascimento_data" binding:"required"`
	NeuteredIndicator  bool       `json:"indicador_castrado" binding:"required"`
	SpeciesName        string     `json:"especie_nome" binding:"required"`
	PedigreeIndicator  *bool      `json:"pedigree_indicador,omitempty"`
	PedigreeOriginName string     `json:"pedigree_origem_nome,omitempty"`
	BreedName          string     `json:"raca_nome" binding:"required"`
	SizeName           string     `json:"porte_nome" binding:"required"`
	PhotoURL           string     `json:"foto_url,omitempty"`
}

// SelfRegisteredPet represents a pet document in the self-registered collection
type SelfRegisteredPet struct {
	ID                 int        `bson:"_id" json:"id_animal"`
	CPF                string     `bson:"cpf" json:"cpf"`
	Name               string     `bson:"animal_nome" json:"animal_nome"`
	MicrochipNumber    string     `bson:"microchip_numero,omitempty" json:"microchip_numero,omitempty"`
	SexAbbreviation    string     `bson:"sexo_sigla" json:"sexo_sigla"`
	BirthDate          *time.Time `bson:"nascimento_data" json:"nascimento_data"`
	NeuteredIndicator  bool       `bson:"indicador_castrado" json:"indicador_castrado"`
	SpeciesName        string     `bson:"especie_nome" json:"especie_nome"`
	PedigreeIndicator  *bool      `bson:"pedigree_indicador,omitempty" json:"pedigree_indicador,omitempty"`
	PedigreeOriginName string     `bson:"pedigree_origem_nome,omitempty" json:"pedigree_origem_nome,omitempty"`
	BreedName          string     `bson:"raca_nome" json:"raca_nome"`
	SizeName           string     `bson:"porte_nome" json:"porte_nome"`
	PhotoURL           string     `bson:"foto_url,omitempty" json:"foto_url,omitempty"`
	Source             string     `bson:"source" json:"source"`
	CreatedAt          time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt          time.Time  `bson:"updated_at" json:"updated_at"`
}

// ToPet converts a SelfRegisteredPet to a Pet model
func (s *SelfRegisteredPet) ToPet() *Pet {
	neuteredPtr := &s.NeuteredIndicator
	idPtr := &s.ID
	return &Pet{
		ID:                 idPtr,
		Name:               s.Name,
		MicrochipNumber:    s.MicrochipNumber,
		SexAbbreviation:    s.SexAbbreviation,
		BirthDate:          s.BirthDate,
		NeuteredIndicator:  neuteredPtr,
		SpeciesName:        s.SpeciesName,
		PedigreeIndicator:  s.PedigreeIndicator,
		PedigreeOriginName: s.PedigreeOriginName,
		BreedName:          s.BreedName,
		SizeName:           s.SizeName,
		PhotoURL:           s.PhotoURL,
		Source:             s.Source,
	}
}
