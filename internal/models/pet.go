package models

import (
	"time"
)

// Pet represents a single pet record
type Pet struct {
	Indicator            *bool      `bson:"indicador,omitempty" json:"indicador,omitempty"`
	ID                   *int       `bson:"id_animal,omitempty" json:"id_animal,omitempty"`
	Name                 string     `bson:"animal_nome,omitempty" json:"animal_nome,omitempty"`
	MicrochipNumber      string     `bson:"microchip_numero,omitempty" json:"microchip_numero,omitempty"`
	SexAbbreviation      string     `bson:"sexo_sigla,omitempty" json:"sexo_sigla,omitempty"`
	BirthDate            *time.Time `bson:"nascimento_data,omitempty" json:"nascimento_data,omitempty"`
	NeuteredIndicator    *bool      `bson:"indicador_castrado,omitempty" json:"indicador_castrado,omitempty"`
	ActiveIndicator      *bool      `bson:"indicador_ativo,omitempty" json:"indicador_ativo,omitempty"`
	SpeciesName          string     `bson:"especie_nome,omitempty" json:"especie_nome,omitempty"`
	PedigreeIndicator    *bool      `bson:"pedigree_indicador,omitempty" json:"pedigree_indicador,omitempty"`
	PedigreeOriginName   string     `bson:"pedigree_origem_nome,omitempty" json:"pedigree_origem_nome,omitempty"`
	LifeStageName        string     `bson:"fase_vida_nome,omitempty" json:"fase_vida_nome,omitempty"`
	BreedName            string     `bson:"raca_nome,omitempty" json:"raca_nome,omitempty"`
	SizeName             string     `bson:"porte_nome,omitempty" json:"porte_nome,omitempty"`
	QRCodePayload        string     `bson:"qrcode_payload,omitempty" json:"qrcode_payload,omitempty"`
	PhotoURL             string     `bson:"foto_url,omitempty" json:"foto_url,omitempty"`
	RegistrationDate     *time.Time `bson:"registro_data,omitempty" json:"registro_data,omitempty"`
	AntiRabiesDate       *time.Time `bson:"antirrabica_data,omitempty" json:"antirrabica_data,omitempty"`
	AntiRabiesExpiryDate *time.Time `bson:"antirrabica_validade_data,omitempty" json:"antirrabica_validade_data,omitempty"`
	DewormingDate        *time.Time `bson:"vermifugacao_data,omitempty" json:"vermifugacao_data,omitempty"`
	DewormingExpiryDate  *time.Time `bson:"vermifugacao_validade_data,omitempty" json:"vermifugacao_validade_data,omitempty"`
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
	ID               interface{}       `bson:"_id,omitempty" json:"_id"`
	CPF              string            `bson:"cpf" json:"cpf"`
	Pets             []Pet             `bson:"pet,omitempty" json:"pet,omitempty"`
	Statistics       *Statistics       `bson:"estatisticas,omitempty" json:"estatisticas,omitempty"`
	AccreditedClinic *AccreditedClinic `bson:"clinica_credenciada,omitempty" json:"clinica_credenciada,omitempty"`
	CPFPartition     *int              `bson:"cpf_particao,omitempty" json:"cpf_particao,omitempty"`
}

// RawCitizenPets represents the raw data structure from MongoDB (with nested pet structure)
type RawCitizenPets struct {
	ID               interface{}       `bson:"_id,omitempty"`
	CPF              string            `bson:"cpf"`
	PetData          *NestedPetData    `bson:"pet,omitempty"`
	Statistics       *Statistics       `bson:"estatisticas,omitempty"`
	AccreditedClinic *AccreditedClinic `bson:"clinica_credenciada,omitempty"`
	CPFPartition     *int              `bson:"cpf_particao,omitempty"`
}

// NestedPetData represents the nested pet object structure
type NestedPetData struct {
	CPF              string            `bson:"cpf"`
	Pets             []Pet             `bson:"pet"`
	Statistics       *Statistics       `bson:"estatisticas,omitempty"`
	AccreditedClinic *AccreditedClinic `bson:"clinica_credenciada,omitempty"`
	CPFPartition     *int              `bson:"cpf_particao,omitempty"`
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

// PetClinicResponse represents the clinic data for a citizen's pets
type PetClinicResponse struct {
	CPF    string            `json:"cpf"`
	Clinic *AccreditedClinic `json:"clinic"`
}

// PetStatsResponse represents the statistics for a citizen's pets
type PetStatsResponse struct {
	CPF        string      `json:"cpf"`
	Statistics *Statistics `json:"statistics"`
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
		ID:               raw.ID,
		CPF:              raw.CPF,
		Statistics:       raw.Statistics,
		AccreditedClinic: raw.AccreditedClinic,
		CPFPartition:     raw.CPFPartition,
		Pets:             []Pet{},
	}

	// Extract pets from the nested PetData structure
	if raw.PetData != nil {
		result.Pets = raw.PetData.Pets

		// Use nested statistics and clinic if they exist and root ones are nil
		if result.Statistics == nil && raw.PetData.Statistics != nil {
			result.Statistics = raw.PetData.Statistics
		}
		if result.AccreditedClinic == nil && raw.PetData.AccreditedClinic != nil {
			result.AccreditedClinic = raw.PetData.AccreditedClinic
		}
		if result.CPFPartition == nil && raw.PetData.CPFPartition != nil {
			result.CPFPartition = raw.PetData.CPFPartition
		}
	}

	return result, nil
}

// parseSinglePet converts a single pet's key-value data to Pet struct
func parseSinglePet(kvPairs []interface{}) (*Pet, error) {
	petMap := make(map[string]interface{})

	for _, item := range kvPairs {
		if kvMap, ok := item.(map[string]interface{}); ok {
			if key, keyOk := kvMap["Key"].(string); keyOk {
				petMap[key] = kvMap["Value"]
			}
		}
	}

	pet := &Pet{}

	// Parse each field
	if val, ok := petMap["indicador"].(bool); ok {
		pet.Indicator = &val
	}

	if val, ok := petMap["id_animal"]; ok {
		if intVal, ok := val.(int); ok {
			pet.ID = &intVal
		} else if floatVal, ok := val.(float64); ok {
			intVal := int(floatVal)
			pet.ID = &intVal
		}
	}

	if val, ok := petMap["animal_nome"].(string); ok && val != "" {
		pet.Name = val
	}

	if val, ok := petMap["microchip_numero"].(string); ok && val != "" {
		pet.MicrochipNumber = val
	}

	if val, ok := petMap["sexo_sigla"].(string); ok && val != "" {
		pet.SexAbbreviation = val
	}

	if val, ok := petMap["nascimento_data"]; ok && val != nil {
		if dateStr, ok := val.(string); ok {
			if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
				pet.BirthDate = &parsedDate
			}
		}
	}

	if val, ok := petMap["indicador_castrado"].(bool); ok {
		pet.NeuteredIndicator = &val
	}

	if val, ok := petMap["indicador_ativo"].(bool); ok {
		pet.ActiveIndicator = &val
	}

	if val, ok := petMap["especie_nome"].(string); ok && val != "" {
		pet.SpeciesName = val
	}

	if val, ok := petMap["pedigree_indicador"].(bool); ok {
		pet.PedigreeIndicator = &val
	}

	if val, ok := petMap["pedigree_origem_nome"].(string); ok && val != "" {
		pet.PedigreeOriginName = val
	}

	if val, ok := petMap["fase_vida_nome"].(string); ok && val != "" {
		pet.LifeStageName = val
	}

	if val, ok := petMap["raca_nome"].(string); ok && val != "" {
		pet.BreedName = val
	}

	if val, ok := petMap["porte_nome"].(string); ok && val != "" {
		pet.SizeName = val
	}

	if val, ok := petMap["qrcode_payload"].(string); ok && val != "" {
		pet.QRCodePayload = val
	}

	if val, ok := petMap["foto_url"].(string); ok && val != "" {
		pet.PhotoURL = val
	}

	// Parse dates
	if val, ok := petMap["registro_data"]; ok && val != nil {
		if dateStr, ok := val.(string); ok {
			if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
				pet.RegistrationDate = &parsedDate
			}
		}
	}

	if val, ok := petMap["antirrabica_data"]; ok && val != nil {
		if dateStr, ok := val.(string); ok {
			if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
				pet.AntiRabiesDate = &parsedDate
			}
		}
	}

	if val, ok := petMap["antirrabica_validade_data"]; ok && val != nil {
		if dateStr, ok := val.(string); ok {
			if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
				pet.AntiRabiesExpiryDate = &parsedDate
			}
		}
	}

	if val, ok := petMap["vermifugacao_data"]; ok && val != nil {
		if dateStr, ok := val.(string); ok {
			if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
				pet.DewormingDate = &parsedDate
			}
		}
	}

	if val, ok := petMap["vermifugacao_validade_data"]; ok && val != nil {
		if dateStr, ok := val.(string); ok {
			if parsedDate, err := time.Parse(time.RFC3339, dateStr); err == nil {
				pet.DewormingExpiryDate = &parsedDate
			}
		}
	}

	return pet, nil
}
