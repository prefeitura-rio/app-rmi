package handlers

import "github.com/prefeitura-rio/app-rmi/internal/clients"

// SalesforceWebhookRequestSwagger documents the inbound webhook body (OpenAPI/Swagger).
// dados is a delta: only changed fields; absent=do not change, null=clear, ""=empty value.
type SalesforceWebhookRequestSwagger struct {
	CPF       string                 `json:"cpf" example:"52998224725"`
	Evento    string                 `json:"evento,omitempty" enums:"atualizacao,anonimizacao" example:"atualizacao"`
	UpdatedAt string                 `json:"updatedAt,omitempty" example:"2026-08-31T17:33:49Z"`
	Dados     SalesforceWebhookDados `json:"dados"`
}

// SalesforceWebhookDados is the inbound delta (same wire shape as GET Person Account).
// Persist overlay: self_declared + salesforce_consentimentos. Does not write citizens
// or RMI opt_in / category_opt_ins. nome, nomeSocial and dataNascimento are accepted
// on the wire and ignored.
type SalesforceWebhookDados struct {
	AccountID              string                            `json:"accountId,omitempty"`
	Nome                   string                            `json:"nome,omitempty"`                  // Ignorado: não altera a coleção citizens
	NomeSocial             string                            `json:"nomeSocial,omitempty"`            // Ignorado: não altera a coleção citizens
	NomeExibicao           string                            `json:"nomeExibicao,omitempty"`
	Email                  string                            `json:"email,omitempty"`
	TelefonePrincipal      string                            `json:"telefonePrincipal,omitempty"`
	Telefone1              string                            `json:"telefone1,omitempty"`
	TipoTelefone1          string                            `json:"tipoTelefone1,omitempty"`
	Telefone2              string                            `json:"telefone2,omitempty"`
	TipoTelefone2          string                            `json:"tipoTelefone2,omitempty"`
	Telefone3              string                            `json:"telefone3,omitempty"`
	TipoTelefone3          string                            `json:"tipoTelefone3,omitempty"`
	TelefoneInternacional  string                            `json:"telefoneInternacional,omitempty"`
	Genero                 string                            `json:"genero,omitempty"`
	Raca                   string                            `json:"raca,omitempty"`
	Escolaridade           string                            `json:"escolaridade,omitempty"`
	RendaFamiliar          string                            `json:"rendaFamiliar,omitempty"`
	Deficiencia            string                            `json:"deficiencia,omitempty"`
	Nacionalidade          string                            `json:"nacionalidade,omitempty"`
	Passaporte             string                            `json:"passaporte,omitempty"`
	DataNascimento         string                            `json:"dataNascimento,omitempty"` // Ignorado: não altera a coleção citizens
	Complemento            string                            `json:"complemento,omitempty"`
	Cidade                 string                            `json:"cidade,omitempty"`
	Idioma                 []string                          `json:"idioma,omitempty"`
	IsTourist              *bool                             `json:"isTourist,omitempty"`
	Endereco               *clients.SalesforceEndereco       `json:"endereco,omitempty"`
	CanalOrigem            string                            `json:"canalOrigem,omitempty"`
	CanalUltimaModificacao string                            `json:"canalUltimaModificacao,omitempty"`
	Consentimento          []clients.SalesforceConsentimento `json:"consentimento,omitempty"` // Só salesforce_consentimentos; null limpa o mapa SF (não mexe no opt-in RMI)
}

// AuthValidateResponse documents GET /auth/validate (login sync with Salesforce only; no Mongo persist).
type AuthValidateResponse struct {
	Action  string                    `json:"action" example:"matched" enums:"matched,updated,created"`
	Cidadao *clients.SalesforceCidadao `json:"cidadao"`
}
