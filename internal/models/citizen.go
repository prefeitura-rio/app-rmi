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
	Indicador           *bool   `json:"indicador,omitempty" bson:"indicador,omitempty"`
	IDCNES              *string `json:"id_cnes,omitempty" bson:"id_cnes,omitempty"`
	Nome                *string `json:"nome,omitempty" bson:"nome,omitempty"`
	Telefone            *string `json:"telefone,omitempty" bson:"telefone,omitempty"`
	Email               *string `json:"email,omitempty" bson:"email,omitempty"`
	Endereco            *string `json:"endereco,omitempty" bson:"endereco,omitempty"`
	HorarioAtendimento  *string `json:"horario_atendimento,omitempty" bson:"horario_atendimento,omitempty"`
}

// EquipeSaudeFamilia represents family health team information
type EquipeSaudeFamilia struct {
	Indicador  *bool     `json:"indicador,omitempty" bson:"indicador,omitempty"`
	IDINE      *string   `json:"id_ine,omitempty" bson:"id_ine,omitempty"`
	Nome       *string   `json:"nome,omitempty" bson:"nome,omitempty"`
	Telefone   *string   `json:"telefone,omitempty" bson:"telefone,omitempty"`
	Medicos    []string  `json:"medicos,omitempty" bson:"medicos,omitempty"`
	Enfermeiros []string `json:"enfermeiros,omitempty" bson:"enfermeiros,omitempty"`
}

// Saude represents health information
type Saude struct {
	ClinicaFamilia      *ClinicaFamilia      `json:"clinica_familia,omitempty" bson:"clinica_familia,omitempty"`
	EquipeSaudeFamilia  *EquipeSaudeFamilia  `json:"equipe_saude_familia,omitempty" bson:"equipe_saude_familia,omitempty"`
}

// CadUnico represents CadÚnico information
type CadUnico struct {
	Indicador                    *bool      `json:"indicador,omitempty" bson:"indicador,omitempty"`
	DataCadastro                 *time.Time `json:"data_cadastro,omitempty" bson:"data_cadastro,omitempty"`
	DataUltimaAtualizacao        *time.Time `json:"data_ultima_atualizacao,omitempty" bson:"data_ultima_atualizacao,omitempty"`
	DataLimiteCadastroAtual      *time.Time `json:"data_limite_cadastro_atual,omitempty" bson:"data_limite_cadastro_atual,omitempty"`
	StatusCadastral              *string    `json:"status_cadastral,omitempty" bson:"status_cadastral,omitempty"`
}

// CRAS represents CRAS information
type CRAS struct {
	Nome      *string `json:"nome,omitempty" bson:"nome,omitempty"`
	Endereco  *string `json:"endereco,omitempty" bson:"endereco,omitempty"`
	Telefone  *string `json:"telefone,omitempty" bson:"telefone,omitempty"`
}

// AssistenciaSocial represents social assistance information
type AssistenciaSocial struct {
	CadUnico *CadUnico `json:"cadunico,omitempty" bson:"cadunico,omitempty"`
	CRAS     *CRAS     `json:"cras,omitempty" bson:"cras,omitempty"`
}

// Aluno represents student information
type Aluno struct {
	Indicador  *bool    `json:"indicador,omitempty" bson:"indicador,omitempty"`
	Conceito   *string  `json:"conceito,omitempty" bson:"conceito,omitempty"`
	Frequencia *float64 `json:"frequencia,omitempty" bson:"frequencia,omitempty"`
}

// Escola represents school information
type Escola struct {
	Nome                  *string `json:"nome,omitempty" bson:"nome,omitempty"`
	HorarioFuncionamento  *string `json:"horario_funcionamento,omitempty" bson:"horario_funcionamento,omitempty"`
	Telefone              *string `json:"telefone,omitempty" bson:"telefone,omitempty"`
	Email                 *string `json:"email,omitempty" bson:"email,omitempty"`
	Whatsapp              *string `json:"whatsapp,omitempty" bson:"whatsapp,omitempty"`
	Endereco              *string `json:"endereco,omitempty" bson:"endereco,omitempty"`
}

// Educacao represents education information
type Educacao struct {
	Aluno  *Aluno  `json:"aluno,omitempty" bson:"aluno,omitempty"`
	Escola *Escola `json:"escola,omitempty" bson:"escola,omitempty"`
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
	Endereco     *Endereco  `json:"endereco,omitempty" bson:"endereco,omitempty"`
	Email        *Email     `json:"email,omitempty" bson:"email,omitempty"`
	Telefone     *Telefone  `json:"telefone,omitempty" bson:"telefone,omitempty"`
	// Internal fields excluded from API response
	Documentos         *Documentos         `json:"-" bson:"documentos,omitempty"`
	Saude              *Saude              `json:"-" bson:"saude,omitempty"`
	AssistenciaSocial  *AssistenciaSocial  `json:"-" bson:"assistencia_social,omitempty"`
	Educacao           *Educacao           `json:"-" bson:"educacao,omitempty"`
	Datalake           *Datalake           `json:"-" bson:"datalake,omitempty"`
	CPFParticao        int64               `json:"-" bson:"cpf_particao"`
	RowNumber          *int32              `json:"-" bson:"row_number,omitempty"`
}

// CitizenWallet represents the citizen's wallet data
type CitizenWallet struct {
	CPF                string              `json:"cpf" bson:"cpf"`
	Documentos         *Documentos         `json:"documentos,omitempty" bson:"documentos,omitempty"`
	Saude              *Saude              `json:"saude,omitempty" bson:"saude,omitempty"`
	AssistenciaSocial  *AssistenciaSocial  `json:"assistencia_social,omitempty" bson:"assistencia_social,omitempty"`
	Educacao           *Educacao           `json:"educacao,omitempty" bson:"educacao,omitempty"`
}

// MaintenanceRequest represents a maintenance request
type MaintenanceRequest struct {
	ID                           string     `json:"id" bson:"_id"`
	CPF                          string     `json:"cpf" bson:"cpf"`
	OrigemOcorrencia             string     `json:"origem_ocorrencia" bson:"origem_ocorrencia"`
	IDChamado                    string     `json:"id_chamado" bson:"id_chamado"`
	IDOrigemOcorrencia           string     `json:"id_origem_ocorrencia" bson:"id_origem_ocorrencia"`
	DataInicio                   *time.Time `json:"data_inicio" bson:"data_inicio"`
	DataFim                      *time.Time `json:"data_fim" bson:"data_fim"`
	IDBairro                     string     `json:"id_bairro" bson:"id_bairro"`
	IDTerritorialidade           string     `json:"id_territorialidade" bson:"id_territorialidade"`
	IDLogradouro                 string     `json:"id_logradouro" bson:"id_logradouro"`
	NumeroLogradouro             *int       `json:"numero_logradouro" bson:"numero_logradouro"`
	IDUnidadeOrganizacional      string     `json:"id_unidade_organizacional" bson:"id_unidade_organizacional"`
	NomeUnidadeOrganizacional    string     `json:"nome_unidade_organizacional" bson:"nome_unidade_organizacional"`
	IDUnidadeOrganizacionalMae   string     `json:"id_unidade_organizacional_mae" bson:"id_unidade_organizacional_mae"`
	UnidadeOrganizacionalOuvidoria string   `json:"unidade_organizacional_ouvidoria" bson:"unidade_organizacional_ouvidoria"`
	Categoria                    string     `json:"categoria" bson:"categoria"`
	IDTipo                       string     `json:"id_tipo" bson:"id_tipo"`
	Tipo                         string     `json:"tipo" bson:"tipo"`
	IDSubtipo                    string     `json:"id_subtipo" bson:"id_subtipo"`
	Subtipo                      string     `json:"subtipo" bson:"subtipo"`
	Status                       string     `json:"status" bson:"status"`
	Longitude                    *float64   `json:"longitude" bson:"longitude"`
	Latitude                     *float64   `json:"latitude" bson:"latitude"`
	DataAlvoFinalizacao          *time.Time `json:"data_alvo_finalizacao" bson:"data_alvo_finalizacao"`
	DataAlvoDiagnostico          *time.Time `json:"data_alvo_diagnostico" bson:"data_alvo_diagnostico"`
	DataRealDiagnostico          *time.Time `json:"data_real_diagnostico" bson:"data_real_diagnostico"`
	TempoPrazo                   *string    `json:"tempo_prazo" bson:"tempo_prazo"`
	PrazoUnidade                 string     `json:"prazo_unidade" bson:"prazo_unidade"`
	PrazoTipo                    string     `json:"prazo_tipo" bson:"prazo_tipo"`
	DentroPrazo                  string     `json:"dentro_prazo" bson:"dentro_prazo"`
	Situacao                     string     `json:"situacao" bson:"situacao"`
	TipoSituacao                 string     `json:"tipo_situacao" bson:"tipo_situacao"`
	JustificativaStatus          *string    `json:"justificativa_status" bson:"justificativa_status"`
	Reclamacoes                  *int       `json:"reclamacoes" bson:"reclamacoes"`
	Descricao                    string     `json:"descricao" bson:"descricao"`
	// Internal fields excluded from API response
	CPFParticao                  int64      `json:"-" bson:"cpf_particao"`
	DataParticao                 *time.Time `json:"-" bson:"data_particao"`
}

// PaginatedMaintenanceRequests represents a paginated response of maintenance requests
type PaginatedMaintenanceRequests struct {
	Data       []MaintenanceRequest `json:"data"`
	Pagination struct {
		Page       int `json:"page"`
		PerPage    int `json:"per_page"`
		Total      int `json:"total"`
		TotalPages int `json:"total_pages"`
	} `json:"pagination"`
}

// ValidEthnicityOptions returns a list of valid ethnicity options
func ValidEthnicityOptions() []string {
	return []string{
		"branca",
		"preta",
		"parda",
		"amarela",
		"indigena",
		"outra",
	}
}

// IsValidEthnicity checks if a given ethnicity value is valid
func IsValidEthnicity(value string) bool {
	for _, valid := range ValidEthnicityOptions() {
		if valid == value {
			return true
		}
	}
	return false
} 