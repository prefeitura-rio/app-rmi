package models

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MaintenanceRequestProtocoloLink is one entry of protocolos[] from
// rj-segovi.adm_central_atendimento_1746.chamado_protocolos_andamentos
// (codigo_protocolo + cpf do solicitante).
type MaintenanceRequestProtocoloLink struct {
	CodigoProtocolo string `bson:"codigo_protocolo"`
	CPF             string `bson:"cpf"`
}

// MaintenanceRequestDocument represents a 1746 chamado document (legacy or SGRC grain).
type MaintenanceRequestDocument struct {
	ID                        string
	CPF                       string // legacy top-level; SGRC uses protocolos[].cpf
	CPFParticao               int64
	Categoria                 string
	DataAlvoFinalizacao       string // maps from data_prevista_atendimento (SGRC) or data_alvo_finalizacao (legacy)
	DataFim                   string
	DataInicio                string
	Descricao                 string // maps from descricao_chamado (SGRC) or descricao (legacy)
	IDChamado                 string
	Indicador                 bool
	NomeUnidadeOrganizacional string // maps from unidade_organizacional_responsavel_atual (SGRC)
	OrigemOcorrencia          string
	Status                    string
	Subtipo                   string
	Tipo                      string
	Endereco                  string
	Bairro                    string
	Logradouro                string
	Numero                    string
	Complemento               string
	Referencia                string
	CEP                       string
	TotalChamados             int
	TotalFechados             int
	ProtocoloLinks            []MaintenanceRequestProtocoloLink
	Andamentos                []MaintenanceRequestAndamento
	LoadedAt                  BSONTimeString
}

// Protocolos returns the list of protocol codes for the API (string[]).
func (doc *MaintenanceRequestDocument) Protocolos() []string {
	return protocoloCodes(doc.ProtocoloLinks)
}

// OwnerCPF returns a CPF associated with the chamado (legacy top-level or first link).
func (doc *MaintenanceRequestDocument) OwnerCPF() string {
	return ownerCPFFrom(doc.CPF, doc.ProtocoloLinks)
}

// UnmarshalBSON decodes legacy and SGRC (BigQuery → Airbyte → Mongo) shapes.
func (doc *MaintenanceRequestDocument) UnmarshalBSON(data []byte) error {
	var raw struct {
		ObjectID                              any                           `bson:"_id"`
		CPF                                   string                        `bson:"cpf"`
		CPFParticao                           int64                         `bson:"cpf_particao"`
		Categoria                             string                        `bson:"categoria"`
		DataAlvoFinalizacao                   BSONTimeString                `bson:"data_alvo_finalizacao"`
		DataPrevistaAtendimento               BSONTimeString                `bson:"data_prevista_atendimento"`
		DataFim                               BSONTimeString                `bson:"data_fim"`
		DataInicio                            BSONTimeString                `bson:"data_inicio"`
		Descricao                             string                        `bson:"descricao"`
		DescricaoChamado                      string                        `bson:"descricao_chamado"`
		LegacyID                              BSONStringish                 `bson:"id"`
		IDChamado                             BSONStringish                 `bson:"id_chamado"`
		Indicador                             bool                          `bson:"indicador"`
		NomeUnidadeOrganizacional             string                        `bson:"nome_unidade_organizacional"`
		UnidadeOrganizacionalResponsavelAtual string                        `bson:"unidade_organizacional_responsavel_atual"`
		OrigemOcorrencia                      string                        `bson:"origem_ocorrencia"`
		Status                                string                        `bson:"status"`
		Subtipo                               string                        `bson:"subtipo"`
		Tipo                                  string                        `bson:"tipo"`
		Endereco                              string                        `bson:"endereco"`
		Bairro                                string                        `bson:"bairro"`
		Logradouro                            string                        `bson:"logradouro"`
		Numero                                string                        `bson:"numero"`
		Complemento                           string                        `bson:"complemento"`
		Referencia                            string                        `bson:"referencia"`
		CEP                                   string                        `bson:"cep"`
		TotalChamados                         int                           `bson:"total_chamados"`
		TotalFechados                         int                           `bson:"total_fechados"`
		Protocolo                             string                        `bson:"protocolo"`
		Protocolos                            any                           `bson:"protocolos"`
		Andamentos                            []MaintenanceRequestAndamento `bson:"andamentos"`
		LoadedAt                              BSONTimeString                `bson:"loaded_at"`
	}
	if err := bson.Unmarshal(data, &raw); err != nil {
		return err
	}
	links, err := decodeProtocoloLinks(raw.Protocolos, raw.Protocolo, raw.CPF)
	if err != nil {
		return err
	}
	*doc = MaintenanceRequestDocument{
		ID:                        bsonIDString(raw.ObjectID),
		CPF:                       raw.CPF,
		CPFParticao:               raw.CPFParticao,
		Categoria:                 raw.Categoria,
		DataAlvoFinalizacao:       firstNonEmpty(string(raw.DataPrevistaAtendimento), string(raw.DataAlvoFinalizacao)),
		DataFim:                   string(raw.DataFim),
		DataInicio:                string(raw.DataInicio),
		Descricao:                 firstNonEmpty(raw.DescricaoChamado, raw.Descricao),
		IDChamado:                 firstNonEmpty(raw.IDChamado.String(), raw.LegacyID.String()),
		Indicador:                 raw.Indicador,
		NomeUnidadeOrganizacional: firstNonEmpty(raw.UnidadeOrganizacionalResponsavelAtual, raw.NomeUnidadeOrganizacional),
		OrigemOcorrencia:          raw.OrigemOcorrencia,
		Status:                    raw.Status,
		Subtipo:                   raw.Subtipo,
		Tipo:                      raw.Tipo,
		Endereco:                  raw.Endereco,
		Bairro:                    raw.Bairro,
		Logradouro:                raw.Logradouro,
		Numero:                    raw.Numero,
		Complemento:               raw.Complemento,
		Referencia:                raw.Referencia,
		CEP:                       raw.CEP,
		TotalChamados:             raw.TotalChamados,
		TotalFechados:             raw.TotalFechados,
		ProtocoloLinks:            links,
		Andamentos:                raw.Andamentos,
		LoadedAt:                  raw.LoadedAt,
	}
	return nil
}

// MaintenanceRequestProtocolDocument is a chamado read by the public lookup by protocol.
type MaintenanceRequestProtocolDocument struct {
	CPF                       string
	IDChamado                 string
	ProtocoloLinks            []MaintenanceRequestProtocoloLink
	Categoria                 string
	Tipo                      string
	Subtipo                   string
	Status                    string
	OrigemOcorrencia          string
	NomeUnidadeOrganizacional string
	DataInicio                string
	DataFim                   string
	DataAlvoFinalizacao       string
	Descricao                 string
	Bairro                    string
	Logradouro                string
	Numero                    string
	Complemento               string
	Referencia                string
	CEP                       string
	Andamentos                []MaintenanceRequestAndamento
	LoadedAt                  BSONTimeString
}

// Protocolos returns protocol codes for the API.
func (doc *MaintenanceRequestProtocolDocument) Protocolos() []string {
	return protocoloCodes(doc.ProtocoloLinks)
}

// MatchesOwner reports whether ownerCPF is the solicitante of this chamado
// (legacy top-level cpf or any protocolos[].cpf).
func (doc *MaintenanceRequestProtocolDocument) MatchesOwner(ownerCPF string) bool {
	if ownerCPF == "" {
		return false
	}
	if doc.CPF != "" && doc.CPF == ownerCPF {
		return true
	}
	for _, p := range doc.ProtocoloLinks {
		if p.CPF != "" && p.CPF == ownerCPF {
			return true
		}
	}
	return false
}

// UnmarshalBSON decodes legacy and SGRC shapes for the public lookup.
func (doc *MaintenanceRequestProtocolDocument) UnmarshalBSON(data []byte) error {
	var raw struct {
		CPF                                   string                        `bson:"cpf"`
		ID                                    BSONStringish                 `bson:"id"`
		IDChamado                             BSONStringish                 `bson:"id_chamado"`
		Protocolo                             string                        `bson:"protocolo"`
		Protocolos                            any                           `bson:"protocolos"`
		Categoria                             string                        `bson:"categoria"`
		Tipo                                  string                        `bson:"tipo"`
		Subtipo                               string                        `bson:"subtipo"`
		Status                                string                        `bson:"status"`
		OrigemOcorrencia                      string                        `bson:"origem_ocorrencia"`
		NomeUnidadeOrganizacional             string                        `bson:"nome_unidade_organizacional"`
		UnidadeOrganizacionalResponsavelAtual string                        `bson:"unidade_organizacional_responsavel_atual"`
		DataInicio                            BSONTimeString                `bson:"data_inicio"`
		DataFim                               BSONTimeString                `bson:"data_fim"`
		DataAlvoFinalizacao                   BSONTimeString                `bson:"data_alvo_finalizacao"`
		DataPrevistaAtendimento               BSONTimeString                `bson:"data_prevista_atendimento"`
		Descricao                             string                        `bson:"descricao"`
		DescricaoChamado                      string                        `bson:"descricao_chamado"`
		Bairro                                string                        `bson:"bairro"`
		Logradouro                            string                        `bson:"logradouro"`
		Numero                                string                        `bson:"numero"`
		Complemento                           string                        `bson:"complemento"`
		Referencia                            string                        `bson:"referencia"`
		CEP                                   string                        `bson:"cep"`
		Andamentos                            []MaintenanceRequestAndamento `bson:"andamentos"`
		LoadedAt                              BSONTimeString                `bson:"loaded_at"`
	}
	if err := bson.Unmarshal(data, &raw); err != nil {
		return err
	}
	links, err := decodeProtocoloLinks(raw.Protocolos, raw.Protocolo, raw.CPF)
	if err != nil {
		return err
	}
	*doc = MaintenanceRequestProtocolDocument{
		CPF:                       raw.CPF,
		IDChamado:                 firstNonEmpty(raw.IDChamado.String(), raw.ID.String()),
		ProtocoloLinks:            links,
		Categoria:                 raw.Categoria,
		Tipo:                      raw.Tipo,
		Subtipo:                   raw.Subtipo,
		Status:                    raw.Status,
		OrigemOcorrencia:          raw.OrigemOcorrencia,
		NomeUnidadeOrganizacional: firstNonEmpty(raw.UnidadeOrganizacionalResponsavelAtual, raw.NomeUnidadeOrganizacional),
		DataInicio:                string(raw.DataInicio),
		DataFim:                   string(raw.DataFim),
		DataAlvoFinalizacao:       firstNonEmpty(string(raw.DataPrevistaAtendimento), string(raw.DataAlvoFinalizacao)),
		Descricao:                 firstNonEmpty(raw.DescricaoChamado, raw.Descricao),
		Bairro:                    raw.Bairro,
		Logradouro:                raw.Logradouro,
		Numero:                    raw.Numero,
		Complemento:               raw.Complemento,
		Referencia:                raw.Referencia,
		CEP:                       raw.CEP,
		Andamentos:                raw.Andamentos,
		LoadedAt:                  raw.LoadedAt,
	}
	return nil
}

func bsonIDString(v any) string {
	switch id := v.(type) {
	case nil:
		return ""
	case string:
		return id
	case primitive.ObjectID:
		return id.Hex()
	default:
		return fmt.Sprintf("%v", id)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func protocoloCodes(links []MaintenanceRequestProtocoloLink) []string {
	out := make([]string, 0, len(links))
	for _, p := range links {
		if p.CodigoProtocolo != "" {
			out = append(out, p.CodigoProtocolo)
		}
	}
	return out
}

func ownerCPFFrom(legacyCPF string, links []MaintenanceRequestProtocoloLink) string {
	if legacyCPF != "" {
		return legacyCPF
	}
	for _, p := range links {
		if p.CPF != "" {
			return p.CPF
		}
	}
	return ""
}

// decodeProtocoloLinks accepts SGRC RECORD REPEATED [{codigo_protocolo,cpf}],
// a legacy string array, or a legacy scalar protocolo (+ optional top-level cpf).
func decodeProtocoloLinks(raw any, legacyProtocolo, legacyCPF string) ([]MaintenanceRequestProtocoloLink, error) {
	if raw == nil {
		if legacyProtocolo != "" {
			return []MaintenanceRequestProtocoloLink{{CodigoProtocolo: legacyProtocolo, CPF: legacyCPF}}, nil
		}
		return []MaintenanceRequestProtocoloLink{}, nil
	}

	switch v := raw.(type) {
	case bson.A:
		return decodeProtocoloLinkSlice([]any(v), legacyCPF)
	case []any:
		return decodeProtocoloLinkSlice(v, legacyCPF)
	case []string:
		out := make([]MaintenanceRequestProtocoloLink, 0, len(v))
		for _, code := range v {
			if code == "" {
				continue
			}
			out = append(out, MaintenanceRequestProtocoloLink{CodigoProtocolo: code, CPF: legacyCPF})
		}
		return out, nil
	case string:
		if v != "" {
			return []MaintenanceRequestProtocoloLink{{CodigoProtocolo: v, CPF: legacyCPF}}, nil
		}
	}

	if legacyProtocolo != "" {
		return []MaintenanceRequestProtocoloLink{{CodigoProtocolo: legacyProtocolo, CPF: legacyCPF}}, nil
	}
	return []MaintenanceRequestProtocoloLink{}, nil
}

func decodeProtocoloLinkSlice(items []any, legacyCPF string) ([]MaintenanceRequestProtocoloLink, error) {
	out := make([]MaintenanceRequestProtocoloLink, 0, len(items))
	for _, item := range items {
		switch entry := item.(type) {
		case string:
			if entry != "" {
				out = append(out, MaintenanceRequestProtocoloLink{CodigoProtocolo: entry, CPF: legacyCPF})
			}
		case bson.M:
			out = append(out, MaintenanceRequestProtocoloLink{
				CodigoProtocolo: anyString(entry["codigo_protocolo"]),
				CPF:             firstNonEmpty(anyString(entry["cpf"]), legacyCPF),
			})
		case map[string]any:
			out = append(out, MaintenanceRequestProtocoloLink{
				CodigoProtocolo: anyString(entry["codigo_protocolo"]),
				CPF:             firstNonEmpty(anyString(entry["cpf"]), legacyCPF),
			})
		default:
			// Re-encode typed structs from the driver.
			b, err := bson.Marshal(entry)
			if err != nil {
				continue
			}
			var link MaintenanceRequestProtocoloLink
			if err := bson.Unmarshal(b, &link); err != nil {
				continue
			}
			if link.CPF == "" {
				link.CPF = legacyCPF
			}
			if link.CodigoProtocolo != "" || link.CPF != "" {
				out = append(out, link)
			}
		}
	}
	return out, nil
}

func anyString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	case BSONStringish:
		return s.String()
	default:
		return fmt.Sprintf("%v", s)
	}
}

// MaintenanceRequestAndamento is one progress entry as loaded from SGRC.
type MaintenanceRequestAndamento struct {
	IDAndamento               BSONStringish  `bson:"id_andamento"`
	Data                      BSONTimeString `bson:"data"`
	Tipo                      string         `bson:"tipo"` // optional; absent in the BigQuery contract
	Descricao                 string         `bson:"descricao"`
	NomeUnidadeOrganizacional string         `bson:"nome_unidade_organizacional"`
	FlPublico                 BSONFlag       `bson:"fl_publico"`
}

// PublicMaintenanceRequestAndamento is a progress entry exposed by the public lookup.
// Internal fields (id_andamento, fl_publico, nome_unidade_organizacional) are never serialized.
type PublicMaintenanceRequestAndamento struct {
	Data      string `json:"data,omitempty"`
	Tipo      string `json:"tipo,omitempty"`
	Descricao string `json:"descricao,omitempty"`
}

// PublicMaintenanceRequest is one chamado of the public lookup by protocol.
// Logradouro and Descricao only for a verified owner; never CPF, numero, complemento, cep, referencia.
type PublicMaintenanceRequest struct {
	IDChamado                 string                              `json:"id_chamado"`
	Protocolos                []string                            `json:"protocolos"`
	Categoria                 string                              `json:"categoria,omitempty"`
	Tipo                      string                              `json:"tipo,omitempty"`
	Subtipo                   string                              `json:"subtipo,omitempty"`
	Status                    string                              `json:"status,omitempty"`
	OrigemOcorrencia          string                              `json:"origem_ocorrencia,omitempty"`
	NomeUnidadeOrganizacional string                              `json:"nome_unidade_organizacional,omitempty"`
	DataInicio                *time.Time                          `json:"data_inicio,omitempty"`
	DataFim                   *time.Time                          `json:"data_fim,omitempty"`
	DataAlvoFinalizacao       *time.Time                          `json:"data_alvo_finalizacao,omitempty"`
	Bairro                    string                              `json:"bairro,omitempty"`
	Andamentos                []PublicMaintenanceRequestAndamento `json:"andamentos"`
	LoadedAt                  string                              `json:"loaded_at,omitempty"`
	Titular                   bool                                `json:"titular"`
	Logradouro                string                              `json:"logradouro,omitempty"`
	Descricao                 string                              `json:"descricao,omitempty"`
}

// PublicMaintenanceRequestResponse is the response of GET /v1/maintenance-request/{protocolo}.
type PublicMaintenanceRequestResponse struct {
	Protocolo string                     `json:"protocolo"`
	Chamados  []PublicMaintenanceRequest `json:"chamados"`
}

// ToPublicMaintenanceRequest projects a chamado for the public lookup.
func (doc *MaintenanceRequestProtocolDocument) ToPublicMaintenanceRequest(ownerCPF string) PublicMaintenanceRequest {
	andamentos := make([]PublicMaintenanceRequestAndamento, 0, len(doc.Andamentos))
	for _, a := range doc.Andamentos {
		if !a.FlPublico.IsSet() {
			continue
		}
		andamentos = append(andamentos, PublicMaintenanceRequestAndamento{
			Data:      string(a.Data),
			Tipo:      a.Tipo,
			Descricao: a.Descricao,
		})
	}

	out := PublicMaintenanceRequest{
		IDChamado:                 doc.IDChamado,
		Protocolos:                doc.Protocolos(),
		Categoria:                 doc.Categoria,
		Tipo:                      doc.Tipo,
		Subtipo:                   doc.Subtipo,
		Status:                    doc.Status,
		OrigemOcorrencia:          doc.OrigemOcorrencia,
		NomeUnidadeOrganizacional: doc.NomeUnidadeOrganizacional,
		DataInicio:                parseMaintenanceRequestTime(doc.DataInicio),
		DataFim:                   parseMaintenanceRequestTime(doc.DataFim),
		DataAlvoFinalizacao:       parseMaintenanceRequestTime(doc.DataAlvoFinalizacao),
		Bairro:                    doc.Bairro,
		Andamentos:                andamentos,
		LoadedAt:                  string(doc.LoadedAt),
	}

	if doc.MatchesOwner(ownerCPF) {
		out.Titular = true
		out.Logradouro = doc.Logradouro
		out.Descricao = doc.Descricao
	}
	return out
}

func parseMaintenanceRequestTime(timeStr string) *time.Time {
	if timeStr == "" {
		return nil
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return &t
		}
	}
	return nil
}
