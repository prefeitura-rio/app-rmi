package models

import (
	"time"
)

// Nascimento represents birth information
type Nascimento struct {
	Data        *time.Time `json:"data" bson:"data,omitempty"`
	MunicipioID *string    `json:"municipio_id" bson:"municipio_id,omitempty"`
	Municipio   *string    `json:"municipio" bson:"municipio,omitempty"`
	UF          *string    `json:"uf" bson:"uf,omitempty"`
	PaisID      *string    `json:"pais_id" bson:"pais_id,omitempty"`
	Pais        *string    `json:"pais" bson:"pais,omitempty"`
}

// Mae represents mother's information
type Mae struct {
	Nome *string `json:"nome" bson:"nome,omitempty"`
	CPF  *string `json:"cpf" bson:"cpf,omitempty"`
}

// Obito represents death information
type Obito struct {
	Indicador *bool  `json:"indicador" bson:"indicador,omitempty"`
	Ano       *int32 `json:"ano" bson:"ano,omitempty"`
}

// Documentos represents document information
type Documentos struct {
	CNS []string `json:"cns" bson:"cns,omitempty"`
}

// EnderecoPrincipal represents the main address
type EnderecoPrincipal struct {
	Origem         *string    `json:"origem" bson:"origem,omitempty"`
	Sistema        *string    `json:"sistema" bson:"sistema,omitempty"`
	CEP            *string    `json:"cep" bson:"cep,omitempty"`
	Estado         *string    `json:"estado" bson:"estado,omitempty"`
	Municipio      *string    `json:"municipio" bson:"municipio,omitempty"`
	TipoLogradouro *string    `json:"tipo_logradouro" bson:"tipo_logradouro,omitempty"`
	Logradouro     *string    `json:"logradouro" bson:"logradouro,omitempty"`
	Numero         *string    `json:"numero" bson:"numero,omitempty"`
	Complemento    *string    `json:"complemento" bson:"complemento,omitempty"`
	Bairro         *string    `json:"bairro" bson:"bairro,omitempty"`
	UpdatedAt      *time.Time `json:"updated_at" bson:"updated_at,omitempty"`
}

// Endereco represents address information
type Endereco struct {
	Indicador   *bool              `json:"indicador" bson:"indicador,omitempty"`
	Principal   *EnderecoPrincipal `json:"principal" bson:"principal,omitempty"`
	Alternativo []int32            `json:"alternativo" bson:"alternativo,omitempty"`
}

// EmailPrincipal represents the main email
type EmailPrincipal struct {
	Origem    *string    `json:"origem" bson:"origem,omitempty"`
	Sistema   *string    `json:"sistema" bson:"sistema,omitempty"`
	Valor     *string    `json:"valor" bson:"valor,omitempty"`
	UpdatedAt *time.Time `json:"updated_at" bson:"updated_at,omitempty"`
}

// Email represents email information
type Email struct {
	Indicador   *bool           `json:"indicador" bson:"indicador,omitempty"`
	Principal   *EmailPrincipal `json:"principal" bson:"principal,omitempty"`
	Alternativo []int32         `json:"alternativo" bson:"alternativo,omitempty"`
}

// TelefonePrincipal represents the main phone
type TelefonePrincipal struct {
	Origem    *string    `json:"origem" bson:"origem,omitempty"`
	Sistema   *string    `json:"sistema" bson:"sistema,omitempty"`
	DDI       *string    `json:"ddi" bson:"ddi,omitempty"`
	DDD       *string    `json:"ddd" bson:"ddd,omitempty"`
	Valor     *string    `json:"valor" bson:"valor,omitempty"`
	UpdatedAt *time.Time `json:"updated_at" bson:"updated_at,omitempty"`
}

// Telefone represents phone information
type Telefone struct {
	Indicador   *bool              `json:"indicador" bson:"indicador,omitempty"`
	Principal   *TelefonePrincipal `json:"principal" bson:"principal,omitempty"`
	Alternativo []int32            `json:"alternativo" bson:"alternativo,omitempty"`
}

// ClinicaFamilia represents family clinic information
type ClinicaFamilia struct {
	Indicador          *bool   `json:"indicador" bson:"indicador,omitempty"`
	IDCNES             *string `json:"id_cnes" bson:"id_cnes,omitempty"`
	Nome               *string `json:"nome" bson:"nome,omitempty"`
	Telefone           *string `json:"telefone" bson:"telefone,omitempty"`
	Email              *string `json:"email" bson:"email,omitempty"`
	Endereco           *string `json:"endereco" bson:"endereco,omitempty"`
	HorarioAtendimento *string `json:"horario_atendimento" bson:"horario_atendimento,omitempty"`
}

// EquipeSaudeFamilia represents family health team information
type EquipeSaudeFamilia struct {
	Indicador   *bool               `json:"indicador" bson:"indicador,omitempty"`
	IDINE       *string             `json:"id_ine" bson:"id_ine,omitempty"`
	Nome        *string             `json:"nome" bson:"nome,omitempty"`
	Telefone    *string             `json:"telefone" bson:"telefone,omitempty"`
	Medicos     []ProfissionalSaude `json:"medicos" bson:"medicos,omitempty"`
	Enfermeiros []ProfissionalSaude `json:"enfermeiros" bson:"enfermeiros,omitempty"`
}

// ProfissionalSaude represents health professional information
type ProfissionalSaude struct {
	IDProfissionalSUS *string `json:"id_profissional_sus" bson:"id_profissional_sus,omitempty"`
	Nome              *string `json:"nome" bson:"nome,omitempty"`
}

// Saude represents health information
type Saude struct {
	ClinicaFamilia     *ClinicaFamilia     `json:"clinica_familia" bson:"clinica_familia,omitempty"`
	EquipeSaudeFamilia *EquipeSaudeFamilia `json:"equipe_saude_familia" bson:"equipe_saude_familia,omitempty"`
}

// CadUnico represents CadÚnico information
type CadUnico struct {
	Indicador               *bool      `json:"indicador" bson:"indicador,omitempty"`
	DataCadastro            *time.Time `json:"data_cadastro" bson:"data_cadastro,omitempty"`
	DataUltimaAtualizacao   *time.Time `json:"data_ultima_atualizacao" bson:"data_ultima_atualizacao,omitempty"`
	DataLimiteCadastroAtual *time.Time `json:"data_limite_cadastro_atual" bson:"data_limite_cadastro_atual,omitempty"`
	StatusCadastral         *string    `json:"status_cadastral" bson:"status_cadastral,omitempty"`
}

// CRAS represents CRAS information
type CRAS struct {
	Nome     *string `json:"nome" bson:"nome,omitempty"`
	Endereco *string `json:"endereco" bson:"endereco,omitempty"`
	Telefone *string `json:"telefone" bson:"telefone,omitempty"`
}

// AssistenciaSocial represents social assistance information
type AssistenciaSocial struct {
	CadUnico *CadUnico `json:"cadunico" bson:"cadunico,omitempty"`
	CRAS     *CRAS     `json:"cras" bson:"cras,omitempty"`
}

// Aluno represents student information
type Aluno struct {
	Indicador  *bool    `json:"indicador" bson:"indicador,omitempty"`
	Conceito   *string  `json:"conceito" bson:"conceito,omitempty"`
	Frequencia *float64 `json:"frequencia" bson:"frequencia,omitempty"`
}

// Escola represents school information
type Escola struct {
	Nome                 *string `json:"nome" bson:"nome,omitempty"`
	HorarioFuncionamento *string `json:"horario_funcionamento" bson:"horario_funcionamento,omitempty"`
	Telefone             *string `json:"telefone" bson:"telefone,omitempty"`
	Email                *string `json:"email" bson:"email,omitempty"`
	Whatsapp             *string `json:"whatsapp" bson:"whatsapp,omitempty"`
	Endereco             *string `json:"endereco" bson:"endereco,omitempty"`
}

// Educacao represents education information
type Educacao struct {
	Aluno  *Aluno  `json:"aluno" bson:"aluno,omitempty"`
	Escola *Escola `json:"escola" bson:"escola,omitempty"`
}

// Datalake represents datalake information
type Datalake struct {
	LastUpdated *time.Time `json:"last_updated" bson:"last_updated,omitempty"`
}

// Citizen represents citizen information
type Citizen struct {
	ID         string      `json:"_id" bson:"_id,omitempty"`
	CPF        string      `json:"cpf" bson:"cpf"`
	Nome       *string     `json:"nome" bson:"nome,omitempty"`
	NomeSocial *string     `json:"nome_social" bson:"nome_social,omitempty"`
	Sexo       *string     `json:"sexo" bson:"sexo,omitempty"`
	Nascimento *Nascimento `json:"nascimento" bson:"nascimento,omitempty"`
	Mae        *Mae        `json:"mae" bson:"mae,omitempty"`
	MenorIdade *bool       `json:"menor_idade" bson:"menor_idade,omitempty"`
	Raca       *string     `json:"raca" bson:"raca,omitempty"`
	Obito      *Obito      `json:"obito" bson:"obito,omitempty"`
	Endereco   *Endereco   `json:"endereco" bson:"endereco,omitempty"`
	Email      *Email      `json:"email" bson:"email,omitempty"`
	Telefone   *Telefone   `json:"telefone" bson:"telefone,omitempty"`
	// Wallet and internal fields
	Documentos        *Documentos        `json:"documentos,omitempty" bson:"documentos,omitempty"`
	Saude             *Saude             `json:"saude,omitempty" bson:"saude,omitempty"`
	AssistenciaSocial *AssistenciaSocial `json:"assistencia_social,omitempty" bson:"assistencia_social,omitempty"`
	Educacao          *Educacao          `json:"educacao,omitempty" bson:"educacao,omitempty"`
	// Internal fields excluded from all API responses
	Datalake    *Datalake `json:"-" bson:"datalake,omitempty"`
	CPFParticao int64     `json:"-" bson:"cpf_particao"`
	RowNumber   *int32    `json:"-" bson:"row_number,omitempty"`
}

// CitizenResponse represents citizen data for the regular citizen endpoint (excluding wallet fields)
type CitizenResponse struct {
	ID         string      `json:"_id" bson:"_id,omitempty"`
	CPF        string      `json:"cpf" bson:"cpf"`
	Nome       *string     `json:"nome" bson:"nome,omitempty"`
	NomeSocial *string     `json:"nome_social" bson:"nome_social,omitempty"`
	Sexo       *string     `json:"sexo" bson:"sexo,omitempty"`
	Nascimento *Nascimento `json:"nascimento" bson:"nascimento,omitempty"`
	Mae        *Mae        `json:"mae" bson:"mae,omitempty"`
	MenorIdade *bool       `json:"menor_idade" bson:"menor_idade,omitempty"`
	Raca       *string     `json:"raca" bson:"raca,omitempty"`
	Obito      *Obito      `json:"obito" bson:"obito,omitempty"`
	Endereco   *Endereco   `json:"endereco" bson:"endereco,omitempty"`
	Email      *Email      `json:"email" bson:"email,omitempty"`
	Telefone   *Telefone   `json:"telefone" bson:"telefone,omitempty"`
}

// ToCitizenResponse converts a Citizen to CitizenResponse (excluding wallet fields)
func (c *Citizen) ToCitizenResponse() *CitizenResponse {
	return &CitizenResponse{
		ID:         c.ID,
		CPF:        c.CPF,
		Nome:       c.Nome,
		NomeSocial: c.NomeSocial,
		Sexo:       c.Sexo,
		Nascimento: c.Nascimento,
		Mae:        c.Mae,
		MenorIdade: c.MenorIdade,
		Raca:       c.Raca,
		Obito:      c.Obito,
		Endereco:   c.Endereco,
		Email:      c.Email,
		Telefone:   c.Telefone,
	}
}

// CitizenWallet represents citizen wallet information
type CitizenWallet struct {
	CPF               string             `json:"cpf" bson:"cpf"`
	Documentos        *Documentos        `json:"documentos" bson:"documentos,omitempty"`
	Saude             *Saude             `json:"saude" bson:"saude,omitempty"`
	AssistenciaSocial *AssistenciaSocial `json:"assistencia_social" bson:"assistencia_social,omitempty"`
	Educacao          *Educacao          `json:"educacao" bson:"educacao,omitempty"`
}

// MaintenanceRequestDocument represents the new document structure for 1746 calls
type MaintenanceRequestDocument struct {
	ID                             string      `json:"_id" bson:"_id"`
	CPF                            string      `json:"cpf" bson:"cpf"`
	CPFParticao                    int64       `json:"cpf_particao" bson:"cpf_particao"`
	OrigemOcorrencia               string      `json:"origem_ocorrencia" bson:"origem_ocorrencia"`
	IDChamado                      string      `json:"id_chamado" bson:"id_chamado"`
	IDOrigemOcorrencia             string      `json:"id_origem_ocorrencia" bson:"id_origem_ocorrencia"`
	DataInicio                     string      `json:"data_inicio" bson:"data_inicio"`
	DataFim                        string      `json:"data_fim" bson:"data_fim"`
	IDBairro                       string      `json:"id_bairro" bson:"id_bairro"`
	IDTerritorialidade             string      `json:"id_territorialidade" bson:"id_territorialidade"`
	IDLogradouro                   string      `json:"id_logradouro" bson:"id_logradouro"`
	NumeroLogradouro               int         `json:"numero_logradouro" bson:"numero_logradouro"`
	IDUnidadeOrganizacional        string      `json:"id_unidade_organizacional" bson:"id_unidade_organizacional"`
	NomeUnidadeOrganizacional      string      `json:"nome_unidade_organizacional" bson:"nome_unidade_organizacional"`
	IDUnidadeOrganizacionalMae     string      `json:"id_unidade_organizacional_mae" bson:"id_unidade_organizacional_mae"`
	UnidadeOrganizacionalOuvidoria string      `json:"unidade_organizacional_ouvidoria" bson:"unidade_organizacional_ouvidoria"`
	Categoria                      string      `json:"categoria" bson:"categoria"`
	IDTipo                         string      `json:"id_tipo" bson:"id_tipo"`
	Tipo                           string      `json:"tipo" bson:"tipo"`
	IDSubtipo                      string      `json:"id_subtipo" bson:"id_subtipo"`
	Subtipo                        string      `json:"subtipo" bson:"subtipo"`
	Status                         string      `json:"status" bson:"status"`
	Longitude                      *float64    `json:"longitude" bson:"longitude"`
	Latitude                       *float64    `json:"latitude" bson:"latitude"`
	DataAlvoFinalizacao            string      `json:"data_alvo_finalizacao" bson:"data_alvo_finalizacao"`
	DataAlvoDiagnostico            string      `json:"data_alvo_diagnostico" bson:"data_alvo_diagnostico"`
	DataRealDiagnostico            string      `json:"data_real_diagnostico" bson:"data_real_diagnostico"`
	TempoPrazo                     interface{} `json:"tempo_prazo" bson:"tempo_prazo"`
	PrazoUnidade                   string      `json:"prazo_unidade" bson:"prazo_unidade"`
	PrazoTipo                      string      `json:"prazo_tipo" bson:"prazo_tipo"`
	DentroPrazo                    string      `json:"dentro_prazo" bson:"dentro_prazo"`
	Situacao                       string      `json:"situacao" bson:"situacao"`
	TipoSituacao                   string      `json:"tipo_situacao" bson:"tipo_situacao"`
	JustificativaStatus            interface{} `json:"justificativa_status" bson:"justificativa_status"`
	Reclamacoes                    int         `json:"reclamacoes" bson:"reclamacoes"`
	Descricao                      string      `json:"descricao" bson:"descricao"`
	DataParticao                   string      `json:"data_particao" bson:"data_particao"`
}

// MaintenanceRequestChamados represents maintenance request calls
type MaintenanceRequestChamados struct {
	Chamado      MaintenanceRequestChamado      `json:"chamado" bson:"chamado"`
	Data         MaintenanceRequestData         `json:"data" bson:"data"`
	Estatisticas MaintenanceRequestEstatisticas `json:"estatisticas" bson:"estatisticas"`
	Localidade   MaintenanceRequestLocalidade   `json:"localidade" bson:"localidade"`
	Prazo        MaintenanceRequestPrazo        `json:"prazo" bson:"prazo"`
	Status       MaintenanceRequestStatus       `json:"status" bson:"status"`
}

// MaintenanceRequestChamado represents the chamado object
type MaintenanceRequestChamado struct {
	Categoria                      string  `json:"categoria" bson:"categoria"`
	Descricao                      *string `json:"descricao" bson:"descricao"`
	IDChamado                      string  `json:"id_chamado" bson:"id_chamado"`
	IDOrigemOcorrencia             string  `json:"id_origem_ocorrencia" bson:"id_origem_ocorrencia"`
	IDSubtipo                      string  `json:"id_subtipo" bson:"id_subtipo"`
	IDTipo                         string  `json:"id_tipo" bson:"id_tipo"`
	IDUnidadeOrganizacional        string  `json:"id_unidade_organizacional" bson:"id_unidade_organizacional"`
	IDUnidadeOrganizacionalMae     string  `json:"id_unidade_organizacional_mae" bson:"id_unidade_organizacional_mae"`
	Indicador                      bool    `json:"indicador" bson:"indicador"`
	NomeUnidadeOrganizacional      string  `json:"nome_unidade_organizacional" bson:"nome_unidade_organizacional"`
	OrigemOcorrencia               string  `json:"origem_ocorrencia" bson:"origem_ocorrencia"`
	Reclamacoes                    int     `json:"reclamacoes" bson:"reclamacoes"`
	Subtipo                        string  `json:"subtipo" bson:"subtipo"`
	Tipo                           string  `json:"tipo" bson:"tipo"`
	UnidadeOrganizacionalOuvidoria string  `json:"unidade_organizacional_ouvidoria" bson:"unidade_organizacional_ouvidoria"`
}

// MaintenanceRequestData represents the data object
type MaintenanceRequestData struct {
	DataAlvoDiagnostico *string `json:"data_alvo_diagnostico" bson:"data_alvo_diagnostico"`
	DataAlvoFinalizacao *string `json:"data_alvo_finalizacao" bson:"data_alvo_finalizacao"`
	DataFim             *string `json:"data_fim" bson:"data_fim"`
	DataInicio          string  `json:"data_inicio" bson:"data_inicio"`
	DataRealDiagnostico *string `json:"data_real_diagnostico" bson:"data_real_diagnostico"`
}

// MaintenanceRequestEstatisticas represents the estatisticas object
type MaintenanceRequestEstatisticas struct {
	TotalChamados int `json:"total_chamados" bson:"total_chamados"`
	TotalFechados int `json:"total_fechados" bson:"total_fechados"`
}

// MaintenanceRequestLocalidade represents the localidade object
type MaintenanceRequestLocalidade struct {
	IDBairro           *string  `json:"id_bairro" bson:"id_bairro"`
	IDLogradouro       *string  `json:"id_logradouro" bson:"id_logradouro"`
	IDTerritorialidade *string  `json:"id_territorialidade" bson:"id_territorialidade"`
	Latitude           *float64 `json:"latitude" bson:"latitude"`
	Longitude          *float64 `json:"longitude" bson:"longitude"`
	NumeroLogradouro   *int     `json:"numero_logradouro" bson:"numero_logradouro"`
}

// MaintenanceRequestPrazo represents the prazo object
type MaintenanceRequestPrazo struct {
	DentroPrazo  string      `json:"dentro_prazo" bson:"dentro_prazo"`
	PrazoTipo    string      `json:"prazo_tipo" bson:"prazo_tipo"`
	PrazoUnidade string      `json:"prazo_unidade" bson:"prazo_unidade"`
	TempoPrazo   interface{} `json:"tempo_prazo" bson:"tempo_prazo"` // Can be null
}

// MaintenanceRequestStatus represents maintenance request status
type MaintenanceRequestStatus struct {
	JustificativaStatus interface{} `json:"justificativa_status" bson:"justificativa_status"` // Can be null
	Situacao            string      `json:"situacao" bson:"situacao"`
	Status              string      `json:"status" bson:"status"`
	TipoSituacao        string      `json:"tipo_situacao" bson:"tipo_situacao"`
}

// MaintenanceRequest represents a maintenance request (for backward compatibility)
type MaintenanceRequest struct {
	ID                             string      `json:"id" bson:"_id"`
	CPF                            string      `json:"cpf" bson:"cpf"`
	OrigemOcorrencia               string      `json:"origem_ocorrencia" bson:"origem_ocorrencia"`
	IDChamado                      string      `json:"id_chamado" bson:"id_chamado"`
	IDOrigemOcorrencia             string      `json:"id_origem_ocorrencia" bson:"id_origem_ocorrencia"`
	DataInicio                     *time.Time  `json:"data_inicio" bson:"data_inicio"`
	DataFim                        *time.Time  `json:"data_fim" bson:"data_fim"`
	IDBairro                       *string     `json:"id_bairro" bson:"id_bairro"`
	IDTerritorialidade             *string     `json:"id_territorialidade" bson:"id_territorialidade"`
	IDLogradouro                   *string     `json:"id_logradouro" bson:"id_logradouro"`
	NumeroLogradouro               *int        `json:"numero_logradouro" bson:"numero_logradouro"`
	IDUnidadeOrganizacional        string      `json:"id_unidade_organizacional" bson:"id_unidade_organizacional"`
	NomeUnidadeOrganizacional      string      `json:"nome_unidade_organizacional" bson:"nome_unidade_organizacional"`
	IDUnidadeOrganizacionalMae     string      `json:"id_unidade_organizacional_mae" bson:"id_unidade_organizacional_mae"`
	UnidadeOrganizacionalOuvidoria string      `json:"unidade_organizacional_ouvidoria" bson:"unidade_organizacional_ouvidoria"`
	Categoria                      string      `json:"categoria" bson:"categoria"`
	IDTipo                         string      `json:"id_tipo" bson:"id_tipo"`
	Tipo                           string      `json:"tipo" bson:"tipo"`
	IDSubtipo                      string      `json:"id_subtipo" bson:"id_subtipo"`
	Subtipo                        string      `json:"subtipo" bson:"subtipo"`
	Status                         string      `json:"status" bson:"status"`
	Longitude                      *float64    `json:"longitude" bson:"longitude"`
	Latitude                       *float64    `json:"latitude" bson:"latitude"`
	DataAlvoFinalizacao            *time.Time  `json:"data_alvo_finalizacao" bson:"data_alvo_finalizacao"`
	DataAlvoDiagnostico            *time.Time  `json:"data_alvo_diagnostico" bson:"data_alvo_diagnostico"`
	DataRealDiagnostico            *time.Time  `json:"data_real_diagnostico" bson:"data_real_diagnostico"`
	TempoPrazo                     interface{} `json:"tempo_prazo" bson:"tempo_prazo"`
	PrazoUnidade                   string      `json:"prazo_unidade" bson:"prazo_unidade"`
	PrazoTipo                      string      `json:"prazo_tipo" bson:"prazo_tipo"`
	DentroPrazo                    string      `json:"dentro_prazo" bson:"dentro_prazo"`
	Situacao                       string      `json:"situacao" bson:"situacao"`
	TipoSituacao                   string      `json:"tipo_situacao" bson:"tipo_situacao"`
	JustificativaStatus            interface{} `json:"justificativa_status" bson:"justificativa_status"`
	Reclamacoes                    int         `json:"reclamacoes" bson:"reclamacoes"`
	Descricao                      *string     `json:"descricao" bson:"descricao"`
	Indicador                      bool        `json:"indicador" bson:"indicador"`
	TotalChamados                  int         `json:"total_chamados" bson:"total_chamados"`
	TotalFechados                  int         `json:"total_fechados" bson:"total_fechados"`
}

// ConvertToMaintenanceRequest converts a MaintenanceRequestDocument to a MaintenanceRequest for backward compatibility
func (doc *MaintenanceRequestDocument) ConvertToMaintenanceRequest() *MaintenanceRequest {
	// Helper function to parse time from string
	parseTime := func(timeStr *string) *time.Time {
		if timeStr == nil || *timeStr == "" {
			return nil
		}
		// Try multiple time formats
		formats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, *timeStr); err == nil {
				return &t
			}
		}
		return nil
	}

	// Helper function to safely handle string to pointer
	stringPtr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}

	// Helper function to safely handle int to pointer
	intPtr := func(i int) *int {
		if i == 0 {
			return nil
		}
		return &i
	}

	// Use flat structure mapping (new format)
	return &MaintenanceRequest{
		ID:                             doc.ID,
		CPF:                            doc.CPF,
		OrigemOcorrencia:               doc.OrigemOcorrencia,
		IDChamado:                      doc.IDChamado,
		IDOrigemOcorrencia:             doc.IDOrigemOcorrencia,
		DataInicio:                     parseTime(&doc.DataInicio),
		DataFim:                        parseTime(&doc.DataFim),
		IDBairro:                       stringPtr(doc.IDBairro),
		IDTerritorialidade:             stringPtr(doc.IDTerritorialidade),
		IDLogradouro:                   stringPtr(doc.IDLogradouro),
		NumeroLogradouro:               intPtr(doc.NumeroLogradouro),
		IDUnidadeOrganizacional:        doc.IDUnidadeOrganizacional,
		NomeUnidadeOrganizacional:      doc.NomeUnidadeOrganizacional,
		IDUnidadeOrganizacionalMae:     doc.IDUnidadeOrganizacionalMae,
		UnidadeOrganizacionalOuvidoria: doc.UnidadeOrganizacionalOuvidoria,
		Categoria:                      doc.Categoria,
		IDTipo:                         doc.IDTipo,
		Tipo:                           doc.Tipo,
		IDSubtipo:                      doc.IDSubtipo,
		Subtipo:                        doc.Subtipo,
		Status:                         doc.Status,
		Longitude:                      doc.Longitude,
		Latitude:                       doc.Latitude,
		DataAlvoFinalizacao:            parseTime(&doc.DataAlvoFinalizacao),
		DataAlvoDiagnostico:            parseTime(&doc.DataAlvoDiagnostico),
		DataRealDiagnostico:            parseTime(&doc.DataRealDiagnostico),
		TempoPrazo:                     doc.TempoPrazo,
		PrazoUnidade:                   doc.PrazoUnidade,
		PrazoTipo:                      doc.PrazoTipo,
		DentroPrazo:                    doc.DentroPrazo,
		Situacao:                       doc.Situacao,
		TipoSituacao:                   doc.TipoSituacao,
		JustificativaStatus:            doc.JustificativaStatus,
		Reclamacoes:                    doc.Reclamacoes,
		Descricao:                      stringPtr(doc.Descricao),
		Indicador:                      false, // Default value since not in flat structure
		TotalChamados:                  0,     // Default value since not in flat structure
		TotalFechados:                  0,     // Default value since not in flat structure
	}
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
