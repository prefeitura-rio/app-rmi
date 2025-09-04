package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CFLookup represents a CF lookup result for a citizen
type CFLookup struct {
	ID                 primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	CPF                string              `bson:"cpf" json:"cpf"`
	AddressHash        string              `bson:"address_hash" json:"address_hash"`
	AddressUsed        string              `bson:"address_used" json:"address_used"`
	CFData             CFInfo              `bson:"cf_data" json:"cf_data"`
	EquipeSaudeData    *EquipeSaudeInfo    `bson:"equipe_saude_data,omitempty" json:"equipe_saude_data,omitempty"`
	DistanceMeters     int                 `bson:"distance_meters" json:"distance_meters"`
	LookupSource       string              `bson:"lookup_source" json:"lookup_source"` // "mcp"
	CreatedAt          time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt          time.Time           `bson:"updated_at" json:"updated_at"`
	IsActive           bool                `bson:"is_active" json:"is_active"`
}

// CFInfo represents detailed information about a Clínica da Família
type CFInfo struct {
	IDEquipamento        *string       `bson:"id_equipamento,omitempty" json:"id_equipamento,omitempty"`
	NomeOficial          string        `bson:"nome_oficial" json:"nome_oficial"`
	NomePopular          string        `bson:"nome_popular" json:"nome_popular"`
	Logradouro           string        `bson:"logradouro" json:"logradouro"`
	Numero               string        `bson:"numero" json:"numero"`
	Complemento          *string       `bson:"complemento" json:"complemento"`
	Bairro               string        `bson:"bairro" json:"bairro"`
	RegiaoAdministrativa string        `bson:"regiao_administrativa" json:"regiao_administrativa"`
	RegiaoPlaneamento    string        `bson:"regiao_planejamento" json:"regiao_planejamento"`
	Subprefeitura        string        `bson:"subprefeitura" json:"subprefeitura"`
	Contato              CFContactInfo `bson:"contato" json:"contato"`
	HorarioFuncionamento []CFHorario   `bson:"horario_funcionamento" json:"horario_funcionamento"`
	Ativo                bool          `bson:"ativo" json:"ativo"`
	AbertoAoPublico      bool          `bson:"aberto_ao_publico" json:"aberto_ao_publico"`
	UpdatedAt            time.Time     `bson:"updated_at" json:"updated_at"`
}

// CFContactInfo represents contact information for a CF
type CFContactInfo struct {
	Telefones   []string `bson:"telefones" json:"telefones"`
	Email       string   `bson:"email" json:"email"`
	Site        *string  `bson:"site" json:"site"`
	RedesSocial CFSocial `bson:"redes_social" json:"redes_social"`
}

// CFSocial represents social media information for a CF
type CFSocial struct {
	Facebook  *string `bson:"facebook" json:"facebook"`
	Instagram *string `bson:"instagram" json:"instagram"`
	Twitter   *string `bson:"twitter" json:"twitter"`
}

// CFHorario represents operating hours for a CF
type CFHorario struct {
	Dia   string `bson:"dia" json:"dia"`
	Abre  string `bson:"abre" json:"abre"`
	Fecha string `bson:"fecha" json:"fecha"`
}

// EquipeSaudeInfo represents detailed information about a Family Health Team
type EquipeSaudeInfo struct {
	IDEquipe           *string       `bson:"id_equipe" json:"id_equipe"`
	NomeOficial        string        `bson:"nome_oficial" json:"nome_oficial"`
	NomePopular        string        `bson:"nome_popular" json:"nome_popular"`
	RegiaoPlaneamento  string        `bson:"regiao_planejamento" json:"regiao_planejamento"`
	Contato            CFContactInfo `bson:"contato" json:"contato"`
	Ativo              bool          `bson:"ativo" json:"ativo"`
	AbertoAoPublico    bool          `bson:"aberto_ao_publico" json:"aberto_ao_publico"`
	Medicos            []string      `bson:"medicos" json:"medicos"`
	Enfermeiros        []string      `bson:"enfermeiros" json:"enfermeiros"`
	UpdatedAt          time.Time     `bson:"updated_at" json:"updated_at"`
}

// HealthServicesResult represents the combined result from MCP lookup
type HealthServicesResult struct {
	HealthFacility     *CFInfo          `json:"health_facility,omitempty"`
	FamilyHealthTeam   *EquipeSaudeInfo `json:"family_health_team,omitempty"`
}

// CFLookupRequest represents a request to lookup CF for a citizen
type CFLookupRequest struct {
	CPF     string `json:"cpf" binding:"required"`
	Address string `json:"address" binding:"required"`
	Force   bool   `json:"force,omitempty"`
}

// CFLookupResponse represents the response for CF lookup operations
type CFLookupResponse struct {
	Found          bool       `json:"found"`
	CFData         *CFInfo    `json:"cf_data,omitempty"`
	DistanceMeters *int       `json:"distance_meters,omitempty"`
	LookupSource   string     `json:"lookup_source,omitempty"`
	CreatedAt      *time.Time `json:"created_at,omitempty"`
}

// CFLookupStats represents statistics about CF lookups
type CFLookupStats struct {
	TotalLookups      int64      `json:"total_lookups"`
	SuccessfulLookups int64      `json:"successful_lookups"`
	FailedLookups     int64      `json:"failed_lookups"`
	SuccessRate       float64    `json:"success_rate"`
	AvgDistance       float64    `json:"avg_distance_meters"`
	LastLookup        *time.Time `json:"last_lookup"`
}

// ToResponse converts CFLookup to CFLookupResponse
func (cf *CFLookup) ToResponse() CFLookupResponse {
	if cf == nil {
		return CFLookupResponse{Found: false}
	}

	return CFLookupResponse{
		Found:          true,
		CFData:         &cf.CFData,
		DistanceMeters: &cf.DistanceMeters,
		LookupSource:   cf.LookupSource,
		CreatedAt:      &cf.CreatedAt,
	}
}

// ToClinicaFamilia converts CFLookup to ClinicaFamilia format for citizen/wallet responses
func (cf *CFLookup) ToClinicaFamilia() *ClinicaFamilia {
	if cf == nil {
		return nil
	}

	// Build address string from CF data
	endereco := cf.CFData.Logradouro + ", " + cf.CFData.Numero
	if cf.CFData.Complemento != nil && *cf.CFData.Complemento != "" {
		endereco += ", " + *cf.CFData.Complemento
	}
	endereco += " - " + cf.CFData.Bairro

	// Build horario atendimento string from CF horario data
	var horarioAtendimento string
	if len(cf.CFData.HorarioFuncionamento) > 0 {
		// Create a simple schedule string (could be improved later)
		horarioAtendimento = "Consulte horários específicos"
		for _, h := range cf.CFData.HorarioFuncionamento {
			if h.Dia != "" && h.Abre != "" && h.Fecha != "" {
				if horarioAtendimento == "Consulte horários específicos" {
					horarioAtendimento = h.Dia + ": " + h.Abre + "-" + h.Fecha
				} else {
					horarioAtendimento += "; " + h.Dia + ": " + h.Abre + "-" + h.Fecha
				}
			}
		}
	}

	// Get first telephone if available
	var telefone *string
	if len(cf.CFData.Contato.Telefones) > 0 {
		telefone = &cf.CFData.Contato.Telefones[0]
	}

	// Get email if available
	var email *string
	if cf.CFData.Contato.Email != "" {
		email = &cf.CFData.Contato.Email
	}

	fonte := "mcp"
	indicador := true

	return &ClinicaFamilia{
		Indicador:          &indicador,
		IDCNES:             cf.CFData.IDEquipamento,
		Nome:               &cf.CFData.NomePopular,
		Telefone:           telefone,
		Email:              email,
		Endereco:           &endereco,
		HorarioAtendimento: &horarioAtendimento,
		Fonte:              &fonte,
	}
}

// ToEquipeSaudeFamilia converts CFLookup to EquipeSaudeFamilia format for citizen/wallet responses
func (cf *CFLookup) ToEquipeSaudeFamilia() *EquipeSaudeFamilia {
	if cf == nil || cf.EquipeSaudeData == nil {
		return nil
	}

	equipeSaude := cf.EquipeSaudeData

	// Build list of ProfissionalSaude for doctors
	var medicos []ProfissionalSaude
	for _, medico := range equipeSaude.Medicos {
		medicos = append(medicos, ProfissionalSaude{
			IDProfissionalSUS: nil, // MCP data doesn't have SUS ID
			Nome:              &medico,
		})
	}

	// Build list of ProfissionalSaude for nurses
	var enfermeiros []ProfissionalSaude
	for _, enfermeiro := range equipeSaude.Enfermeiros {
		enfermeiros = append(enfermeiros, ProfissionalSaude{
			IDProfissionalSUS: nil, // MCP data doesn't have SUS ID
			Nome:              &enfermeiro,
		})
	}

	// Get first telephone if available
	var telefone *string
	if len(equipeSaude.Contato.Telefones) > 0 {
		telefone = &equipeSaude.Contato.Telefones[0]
	}

	indicador := true
	nome := equipeSaude.NomeOficial

	return &EquipeSaudeFamilia{
		Indicador:   &indicador,
		IDINE:       equipeSaude.IDEquipe,
		Nome:        &nome,
		Telefone:    telefone,
		Medicos:     medicos,
		Enfermeiros: enfermeiros,
	}
}
