package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	salesforceCidadaoPath      = "/api/private/cidadao"
	salesforceAnonimizacaoPath = "/api/private/anonimizacao"
	salesforceChamadosPath     = "/api/private/chamados"

	salesforceHTTPMaxAttempts = 3
	salesforceHTTPRetryBase   = 100 * time.Millisecond
)

// BearerTokenSource provides JWT access tokens for Salesforce API calls.
// Salesforce is authenticated only with the same Bearer JWT used by RMI (no OAuth client-credentials).
type BearerTokenSource interface {
	BearerToken(ctx context.Context) (string, error)
}

// StaticBearerToken always returns the same JWT (request Authorization / tests).
type StaticBearerToken string

// BearerToken implements BearerTokenSource.
func (s StaticBearerToken) BearerToken(ctx context.Context) (string, error) {
	token := strings.TrimSpace(string(s))
	if token == "" {
		return "", fmt.Errorf("bearer token is empty")
	}
	return token, nil
}

// SalesforceAPIError is returned when Salesforce responds with a non-2xx status.
type SalesforceAPIError struct {
	StatusCode int
	Body       string
	Errors     []SalesforceFieldError
}

func (e *SalesforceAPIError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("salesforce API status %d: %s (%s)", e.StatusCode, e.Errors[0].Code, e.Errors[0].Message)
	}
	return fmt.Sprintf("salesforce API status %d: %s", e.StatusCode, e.Body)
}

// IsNotFound reports whether the error is an HTTP 404.
func (e *SalesforceAPIError) IsNotFound() bool {
	return e != nil && e.StatusCode == http.StatusNotFound
}

// IsConflict reports whether the error is an HTTP 409.
func (e *SalesforceAPIError) IsConflict() bool {
	return e != nil && e.StatusCode == http.StatusConflict
}

// IsUnauthorized reports whether the error is an HTTP 401.
func (e *SalesforceAPIError) IsUnauthorized() bool {
	return e != nil && e.StatusCode == http.StatusUnauthorized
}

// IsRetryable reports whether the caller should retry (429 or 5xx).
func (e *SalesforceAPIError) IsRetryable() bool {
	if e == nil {
		return false
	}
	if e.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return e.StatusCode >= 500 && e.StatusCode <= 599
}

// SalesforceFieldError matches Salesforce error payloads.
type SalesforceFieldError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SalesforceErrorBody is the envelope for 4xx/5xx responses.
type SalesforceErrorBody struct {
	Errors []SalesforceFieldError `json:"errors"`
}

// SalesforceEndereco is the address block used by Salesforce citizen APIs.
type SalesforceEndereco struct {
	Logradouro  string `json:"logradouro,omitempty"`
	Cidade      string `json:"cidade,omitempty"`
	Estado      string `json:"estado,omitempty"`
	CEP         string `json:"cep,omitempty"`
	Pais        string `json:"pais,omitempty"`
	Complemento string `json:"complemento,omitempty"`
	Bairro      string `json:"bairro,omitempty"`
}

// SalesforceConsentimento is a consent preference from Salesforce (GET cidadao / webhook dados).
type SalesforceConsentimento struct {
	Categoria  string `json:"categoria"`
	Acao       string `json:"acao,omitempty"`   // legacy: optin | optout
	Status     string `json:"status,omitempty"` // IN | OUT
	Canal      string `json:"canal,omitempty"`
	Data       string `json:"data,omitempty"`
	Finalidade string `json:"finalidade,omitempty"`
	Motivo     string `json:"motivo,omitempty"` // required when status=OUT / acao=optout
}

// SalesforceCidadaoCreateRequest is the POST .../api/private/cidadao body.
// POST creates or updates a citizen (upsert).
type SalesforceCidadaoCreateRequest struct {
	CPF                    string              `json:"cpf"`
	Nome                   string              `json:"nome,omitempty"`
	NomeSocial             string              `json:"nomeSocial,omitempty"`
	NomeExibicao           string              `json:"nomeExibicao,omitempty"`
	Email                  string              `json:"email,omitempty"`
	Telefone1              string              `json:"telefone1,omitempty"`
	Telefone2              string              `json:"telefone2,omitempty"`
	Telefone3              string              `json:"telefone3,omitempty"`
	TipoTelefone1          string              `json:"tipoTelefone1,omitempty"`
	TipoTelefone2          string              `json:"tipoTelefone2,omitempty"`
	TipoTelefone3          string              `json:"tipoTelefone3,omitempty"`
	TelefoneInternacional  string              `json:"telefoneInternacional,omitempty"`
	Genero                 string              `json:"genero,omitempty"`
	Raca                   string              `json:"raca,omitempty"`
	Escolaridade           string              `json:"escolaridade,omitempty"`
	RendaFamiliar          string              `json:"rendaFamiliar,omitempty"`
	Deficiencia            string              `json:"deficiencia,omitempty"`
	DataNascimento         string              `json:"dataNascimento,omitempty"`
	Complemento            string              `json:"complemento,omitempty"`
	Nacionalidade          string              `json:"nacionalidade,omitempty"`
	Passaporte             string              `json:"passaporte,omitempty"`
	IsTourist              *bool               `json:"isTourist,omitempty"`
	Idioma                 []string            `json:"idioma,omitempty"`
	Endereco               *SalesforceEndereco `json:"endereco,omitempty"`
	Cidade                 string              `json:"cidade,omitempty"`
	CanalOrigem            string              `json:"canalOrigem,omitempty"`
	CanalUltimaModificacao string              `json:"canalUltimaModificacao,omitempty"`
	ContaOrigem            string              `json:"contaOrigem,omitempty"`
}

// SalesforceCidadaoCreateResponse is returned on 2xx from POST (status created|updated).
type SalesforceCidadaoCreateResponse struct {
	Status    string `json:"status"`
	AccountID string `json:"accountId"`
}

// SalesforceCidadao is the citizen DTO returned by GET and PATCH.
type SalesforceCidadao struct {
	AccountID              string                    `json:"accountId"`
	CPF                    string                    `json:"cpf"`
	Nome                   string                    `json:"nome"`
	NomeSocial             string                    `json:"nomeSocial"`
	NomeExibicao           string                    `json:"nomeExibicao"`
	Email                  string                    `json:"email"`
	TelefonePrincipal      string                    `json:"telefonePrincipal"`
	Telefone1              string                    `json:"telefone1"`
	TipoTelefone1          string                    `json:"tipoTelefone1"`
	Telefone2              string                    `json:"telefone2"`
	TipoTelefone2          string                    `json:"tipoTelefone2"`
	Telefone3              string                    `json:"telefone3"`
	TipoTelefone3          string                    `json:"tipoTelefone3"`
	TelefoneInternacional  string                    `json:"telefoneInternacional"`
	Endereco               *SalesforceEndereco       `json:"endereco"`
	Complemento            string                    `json:"complemento"`
	DataNascimento         string                    `json:"dataNascimento"`
	Genero                 string                    `json:"genero"`
	Raca                   string                    `json:"raca"`
	Escolaridade           string                    `json:"escolaridade"`
	RendaFamiliar          string                    `json:"rendaFamiliar"`
	Deficiencia            string                    `json:"deficiencia"`
	Nacionalidade          string                    `json:"nacionalidade"`
	Idioma                 []string                  `json:"idioma"`
	IsTourist              bool                      `json:"isTourist"`
	Passaporte             string                    `json:"passaporte"`
	Consentimento          []SalesforceConsentimento `json:"consentimento"`
	CanalOrigem            string                    `json:"canalOrigem"`
	CanalUltimaModificacao string                    `json:"canalUltimaModificacao"`
}

// SalesforceConsentimentoPatchRequest is the PATCH .../cidadao/{cpf}/consentimento body.
// Motivo is required by Salesforce when Acao is "optout".
type SalesforceConsentimentoPatchRequest struct {
	Categoria string `json:"categoria"`
	Acao      string `json:"acao"`
	Motivo    string `json:"motivo,omitempty"`
	Origem    string `json:"origem,omitempty"`
}

// SalesforceExportacao is the GET .../cidadao/{cpf}/exportar response (LGPD portability snapshot).
type SalesforceExportacao struct {
	CPF            string                          `json:"cpf"`
	DataExportacao string                          `json:"dataExportacao"`
	DadosPessoais  map[string]interface{}          `json:"dadosPessoais"`
	Relacionados   map[string]interface{}          `json:"relacionados"`
	Consentimento  []SalesforceExportConsentimento `json:"consentimento,omitempty"`
}

// SalesforceExportConsentimento is a consent category in the LGPD export snapshot.
// Homolog groups preferences by codigo with nested canais (not the flat GET /cidadao shape).
type SalesforceExportConsentimento struct {
	Codigo    string                               `json:"codigo"`
	Descricao string                               `json:"descricao,omitempty"`
	Canais    []SalesforceExportConsentimentoCanal `json:"canais,omitempty"`
}

// SalesforceExportConsentimentoCanal is a channel preference inside an export consentimento group.
type SalesforceExportConsentimentoCanal struct {
	Canal  string `json:"canal,omitempty"`
	Status string `json:"status,omitempty"`
	Tipo   string `json:"tipo,omitempty"`
	Motivo string `json:"motivo,omitempty"`
}

// SalesforceAnonimizacaoStatus is returned by POST anonimizar (202/409) and GET anonimizacao.
type SalesforceAnonimizacaoStatus struct {
	NumeroSolicitacao string `json:"numeroSolicitacao"`
	CPF               string `json:"cpf,omitempty"`
	Status            string `json:"status"`
	DataSolicitacao   string `json:"dataSolicitacao,omitempty"`
	DataConclusao     string `json:"dataConclusao,omitempty"`
}

// SalesforceChamadosList is the GET .../chamados response.
type SalesforceChamadosList struct {
	OuvidoriaWindow *SalesforceOuvidoriaWindow `json:"ouvidoriaWindow,omitempty"`
	Protocolos      []SalesforceProtocolo      `json:"protocolos"`
}

// SalesforceProtocolo is a Case-like protocol with nested work orders.
type SalesforceProtocolo struct {
	Protocolo             string                   `json:"protocolo"`
	Categoria             string                   `json:"categoria,omitempty"`
	Subcategoria          string                   `json:"subcategoria,omitempty"`
	DataAbertura          string                   `json:"dataAbertura,omitempty"`
	DataUltimaAtualizacao string                   `json:"dataUltimaAtualizacao,omitempty"`
	DataFechamento        *string                  `json:"dataFechamento,omitempty"`
	Status                string                   `json:"status,omitempty"`
	Origem                string                   `json:"origem,omitempty"`
	OrdensDeServico       []SalesforceOrdemServico `json:"ordens_de_servico,omitempty"`
}

// SalesforceOrgao is an organization block on a work order.
type SalesforceOrgao struct {
	ID    string `json:"id,omitempty"`
	Nome  string `json:"nome,omitempty"`
	Sigla string `json:"sigla,omitempty"`
}

// SalesforceCanOpenAction describes whether an ouvidoria action is available.
type SalesforceCanOpenAction struct {
	Available  bool   `json:"available"`
	CategoryID string `json:"categoryId,omitempty"`
}

// SalesforceCanOpen groups elogio/sugestao/reclamacao availability.
type SalesforceCanOpen struct {
	Elogio     *SalesforceCanOpenAction `json:"elogio,omitempty"`
	Sugestao   *SalesforceCanOpenAction `json:"sugestao,omitempty"`
	Reclamacao *SalesforceCanOpenAction `json:"reclamacao,omitempty"`
}

// SalesforceOuvidoriaWindow is the citizen-facing ouvidoria window metadata.
type SalesforceOuvidoriaWindow struct {
	Days  int    `json:"days,omitempty"`
	Label string `json:"label,omitempty"`
}

// SalesforceEnderecoChamado is address detail on a work order.
type SalesforceEnderecoChamado struct {
	Logradouro      string  `json:"logradouro,omitempty"`
	Numero          string  `json:"numero,omitempty"`
	Complemento     string  `json:"complemento,omitempty"`
	Bairro          string  `json:"bairro,omitempty"`
	CEP             string  `json:"cep,omitempty"`
	CoordenadaX     float64 `json:"coordenadaX,omitempty"`
	CoordenadaY     float64 `json:"coordenadaY,omitempty"`
	PontoReferencia string  `json:"pontoReferencia,omitempty"`
	TipoEndereco    string  `json:"tipoEndereco,omitempty"`
}

// SalesforceAndamento is a citizen-visible work-order update.
type SalesforceAndamento struct {
	DataInsercao string `json:"dataInsercao,omitempty"`
	Status       string `json:"status,omitempty"`
	Evento       string `json:"evento,omitempty"`
	TipoEvento   string `json:"tipoEvento,omitempty"`
	Descricao    string `json:"descricao,omitempty"`
}

// SalesforceOrdemServico is a WorkOrder nested under a protocol.
type SalesforceOrdemServico struct {
	CodigoOs            string                     `json:"codigoOs,omitempty"`
	IDCatalogoServico   string                     `json:"idCatalogoServico,omitempty"`
	Categoria           string                     `json:"categoria,omitempty"`
	Servico             string                     `json:"servico,omitempty"`
	Tema                string                     `json:"tema,omitempty"`
	Subtema             string                     `json:"subtema,omitempty"`
	OrgaoResponsavel    *SalesforceOrgao           `json:"orgaoResponsavel,omitempty"`
	OrgaoResponsavelPai *SalesforceOrgao           `json:"orgaoResponsavelPai,omitempty"`
	Bairro              string                     `json:"bairro,omitempty"`
	DataAbertura        string                     `json:"dataAbertura,omitempty"`
	DataTermino         string                     `json:"dataTermino,omitempty"`
	PrevisaoSLA         string                     `json:"previsaoSLA,omitempty"`
	Prioridade          string                     `json:"prioridade,omitempty"`
	Descricao           string                     `json:"descricao,omitempty"`
	Endereco            *SalesforceEnderecoChamado `json:"endereco,omitempty"`
	Status              string                     `json:"status,omitempty"`
	Andamentos          []SalesforceAndamento      `json:"andamentos,omitempty"`
	ID                  string                     `json:"id,omitempty"`
	IsClosed            bool                       `json:"isClosed"`
	IsOverdue           bool                       `json:"isOverdue"`
	DataFechamento      *string                    `json:"dataFechamento,omitempty"`
	CanOpen             *SalesforceCanOpen         `json:"canOpen,omitempty"`
	OuvidoriaWindow     *SalesforceOuvidoriaWindow `json:"ouvidoriaWindow,omitempty"`
	OuvidoriaURL        string                     `json:"ouvidoriaUrl,omitempty"`
}

// SalesforceClient talks to the Salesforce private citizen API using Bearer JWT.
type SalesforceClient struct {
	baseURL     string
	httpClient  *http.Client
	tokenSource BearerTokenSource
}

// NewSalesforceClient creates a Salesforce client authenticated via BearerTokenSource.
// timeout defaults to 30s when <= 0.
func NewSalesforceClient(baseURL string, timeout time.Duration, tokens BearerTokenSource) *SalesforceClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &SalesforceClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		tokenSource: tokens,
	}
}

// WithBearerToken returns a shallow copy that uses a static JWT (e.g. inbound webhook Authorization).
func (c *SalesforceClient) WithBearerToken(token string) *SalesforceClient {
	if c == nil {
		return nil
	}
	clone := *c
	clone.tokenSource = StaticBearerToken(token)
	return &clone
}

// Configured reports whether base URL and a token source are both set.
func (c *SalesforceClient) Configured() bool {
	return c != nil && c.baseURL != "" && c.tokenSource != nil
}

// CreateOrUpdateCidadao POSTs to .../api/private/cidadao (creates or updates).
func (c *SalesforceClient) CreateOrUpdateCidadao(ctx context.Context, req *SalesforceCidadaoCreateRequest) (*SalesforceCidadaoCreateResponse, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("salesforce create request is nil")
	}
	if strings.TrimSpace(req.CPF) == "" {
		return nil, fmt.Errorf("cpf is required")
	}
	if phone := NormalizeSalesforcePhone(req.Telefone1); phone != "" {
		req.Telefone1 = phone
	}

	respBody, err := c.doJSONBytes(ctx, http.MethodPost, salesforceCidadaoPath, req)
	if err != nil {
		return nil, err
	}
	return decodeSalesforceCreateResponse(respBody)
}

// GetCidadao GETs .../api/private/cidadao/{cpf}.
func (c *SalesforceClient) GetCidadao(ctx context.Context, cpf string) (*SalesforceCidadao, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	cpf = strings.TrimSpace(cpf)
	if cpf == "" {
		return nil, fmt.Errorf("cpf is required")
	}

	path := salesforceCidadaoPath + "/" + url.PathEscape(cpf)
	respBody, err := c.doJSONBytes(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return decodeSalesforceCidadaoResponse(respBody)
}

// PatchCidadao PATCHes .../api/private/cidadao/{cpf} and returns the full citizen DTO.
// Homolog may acknowledge with {camposAtualizados,...} instead of the Person Account body;
// in that case we re-GET so callers always receive a complete DTO when possible.
func (c *SalesforceClient) PatchCidadao(ctx context.Context, cpf string, req *SalesforceCidadaoPatchRequest) (*SalesforceCidadao, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	cpf = strings.TrimSpace(cpf)
	if cpf == "" {
		return nil, fmt.Errorf("cpf is required")
	}
	if req == nil {
		return nil, fmt.Errorf("salesforce patch request is nil")
	}
	normalizePatchPhones(req.fields)

	path := salesforceCidadaoPath + "/" + url.PathEscape(cpf)
	respBody, err := c.doJSONBytes(ctx, http.MethodPatch, path, req)
	if err != nil {
		var apiErr *SalesforceAPIError
		if errors.As(err, &apiErr) && apiErr.IsConflict() {
			// SF returns 409 when the Person Account is already in the requested state.
			return c.GetCidadao(ctx, cpf)
		}
		return nil, err
	}
	if isSalesforcePatchAck(respBody) {
		return c.GetCidadao(ctx, cpf)
	}
	return decodeSalesforceCidadaoResponse(respBody)
}

// PatchCidadaoConsentimento PATCHes .../cidadao/{cpf}/consentimento (200 empty body).
func (c *SalesforceClient) PatchCidadaoConsentimento(ctx context.Context, cpf string, req *SalesforceConsentimentoPatchRequest) error {
	if err := c.requireConfigured(); err != nil {
		return err
	}
	cpf = strings.TrimSpace(cpf)
	if cpf == "" {
		return fmt.Errorf("cpf is required")
	}
	if req == nil {
		return fmt.Errorf("salesforce consentimento request is nil")
	}
	if strings.TrimSpace(req.Categoria) == "" {
		return fmt.Errorf("categoria is required")
	}
	if strings.TrimSpace(req.Acao) == "" {
		return fmt.Errorf("acao is required")
	}
	if strings.EqualFold(strings.TrimSpace(req.Acao), "optout") && strings.TrimSpace(req.Motivo) == "" {
		return fmt.Errorf("motivo is required when acao is optout")
	}

	path := salesforceCidadaoPath + "/" + url.PathEscape(cpf) + "/consentimento"
	err := c.doJSON(ctx, http.MethodPatch, path, req, nil)
	if err == nil {
		return nil
	}
	var apiErr *SalesforceAPIError
	if errors.As(err, &apiErr) && apiErr.IsConflict() {
		// SF returns 409 when consent is already IN/OUT as requested; treat as success.
		return nil
	}
	return err
}

// ExportarCidadao GETs .../cidadao/{cpf}/exportar (synchronous LGPD snapshot).
func (c *SalesforceClient) ExportarCidadao(ctx context.Context, cpf string) (*SalesforceExportacao, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	cpf = strings.TrimSpace(cpf)
	if cpf == "" {
		return nil, fmt.Errorf("cpf is required")
	}

	path := salesforceCidadaoPath + "/" + url.PathEscape(cpf) + "/exportar"
	var out SalesforceExportacao
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AnonimizarCidadao POSTs .../cidadao/{cpf}/anonimizar.
// On 202 returns the queued status. On 409 returns the existing open request plus a conflict error
// (response body is still populated when Salesforce returns the status payload).
func (c *SalesforceClient) AnonimizarCidadao(ctx context.Context, cpf string) (*SalesforceAnonimizacaoStatus, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	cpf = strings.TrimSpace(cpf)
	if cpf == "" {
		return nil, fmt.Errorf("cpf is required")
	}

	path := salesforceCidadaoPath + "/" + url.PathEscape(cpf) + "/anonimizar"
	var out SalesforceAnonimizacaoStatus
	err := c.doJSON(ctx, http.MethodPost, path, nil, &out)
	if err == nil {
		return &out, nil
	}
	var apiErr *SalesforceAPIError
	if errors.As(err, &apiErr) && apiErr.IsConflict() && len(apiErr.Body) > 0 {
		_ = json.Unmarshal([]byte(apiErr.Body), &out)
		if out.NumeroSolicitacao != "" || out.Status != "" {
			return &out, apiErr
		}
	}
	return nil, err
}

// GetAnonimizacao GETs .../anonimizacao/{numeroSolicitacao} for polling.
func (c *SalesforceClient) GetAnonimizacao(ctx context.Context, numeroSolicitacao string) (*SalesforceAnonimizacaoStatus, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	numeroSolicitacao = strings.TrimSpace(numeroSolicitacao)
	if numeroSolicitacao == "" {
		return nil, fmt.Errorf("numeroSolicitacao is required")
	}

	path := salesforceAnonimizacaoPath + "/" + url.PathEscape(numeroSolicitacao)
	var out SalesforceAnonimizacaoStatus
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListChamados GETs .../chamados for the Community User bound to the JWT (no CPF in path).
func (c *SalesforceClient) ListChamados(ctx context.Context) (*SalesforceChamadosList, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	var out SalesforceChamadosList
	if err := c.doJSON(ctx, http.MethodGet, salesforceChamadosPath, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetChamado GETs .../chamados/{protocolo} detail for the authenticated Community User.
func (c *SalesforceClient) GetChamado(ctx context.Context, protocolo string) (*SalesforceProtocolo, error) {
	if err := c.requireConfigured(); err != nil {
		return nil, err
	}
	protocolo = strings.TrimSpace(protocolo)
	if protocolo == "" {
		return nil, fmt.Errorf("protocolo is required")
	}

	path := salesforceChamadosPath + "/" + url.PathEscape(protocolo)
	var out SalesforceProtocolo
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NormalizeSalesforcePhone ensures Brazilian numbers without DDI get a leading 55.
// Empty input is returned unchanged.
func NormalizeSalesforcePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if digits == "" {
		return phone
	}
	if strings.HasPrefix(digits, "55") && len(digits) >= 12 {
		return digits
	}
	// Already has another country code (starts with + handled above via digit strip)
	// or local BR mobile/landline without DDI → prepend 55.
	if !strings.HasPrefix(digits, "55") {
		return "55" + digits
	}
	return digits
}

func (c *SalesforceClient) requireConfigured() error {
	if c == nil || c.baseURL == "" {
		return fmt.Errorf("salesforce base URL not configured")
	}
	if c.tokenSource == nil {
		return fmt.Errorf("salesforce bearer token source not configured")
	}
	return nil
}

func (c *SalesforceClient) doJSON(ctx context.Context, method, path string, body any, out any) error {
	respBody, err := c.doJSONBytes(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed to decode salesforce response: %w", err)
	}
	return nil
}

// doJSONBytes performs the request and returns the raw success body (or a SalesforceAPIError).
// Transient failures (network, 429, 5xx) are retried up to salesforceHTTPMaxAttempts.
func (c *SalesforceClient) doJSONBytes(ctx context.Context, method, path string, body any) ([]byte, error) {
	var bodyBytes []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal salesforce request: %w", err)
		}
		bodyBytes = encoded
	}

	var lastErr error
	for attempt := 1; attempt <= salesforceHTTPMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		respBody, err := c.doJSONBytesOnce(ctx, method, path, bodyBytes)
		if err == nil {
			return respBody, nil
		}
		lastErr = err
		if !isSalesforceHTTPRetryable(err) || attempt == salesforceHTTPMaxAttempts {
			return nil, err
		}
		delay := time.Duration(attempt) * salesforceHTTPRetryBase
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func (c *SalesforceClient) doJSONBytesOnce(ctx context.Context, method, path string, bodyBytes []byte) ([]byte, error) {
	token, err := c.tokenSource.BearerToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get bearer token: %w", err)
	}

	var reader io.Reader
	if bodyBytes != nil {
		reader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create salesforce request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	if bodyBytes != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	} else if method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut {
		// Mule rejects POST without Content-Length (HTTP 411).
		httpReq.ContentLength = 0
		httpReq.Body = http.NoBody
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute salesforce request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("failed to read salesforce response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &SalesforceAPIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
		var env SalesforceErrorBody
		if json.Unmarshal(respBody, &env) == nil {
			apiErr.Errors = env.Errors
		}
		return nil, apiErr
	}
	return respBody, nil
}

func isSalesforceHTTPRetryable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *SalesforceAPIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRetryable()
	}
	msg := err.Error()
	return strings.Contains(msg, "failed to execute salesforce request") ||
		strings.Contains(msg, "failed to read salesforce response")
}

// isSalesforcePatchAck reports whether the PATCH body is an update acknowledgement
// (camposAtualizados) rather than a full Person Account DTO.
func isSalesforcePatchAck(body []byte) bool {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return true
	}
	if body[0] != '{' {
		return false
	}
	var ack struct {
		CamposAtualizados []string `json:"camposAtualizados"`
		AccountID         string   `json:"accountId"`
		Nome              string   `json:"nome"`
		Email             string   `json:"email"`
	}
	if err := json.Unmarshal(body, &ack); err != nil {
		return false
	}
	if len(ack.CamposAtualizados) > 0 {
		return true
	}
	// Empty/partial object without identity fields — not a usable DTO.
	return strings.TrimSpace(ack.AccountID) == "" &&
		strings.TrimSpace(ack.Nome) == "" &&
		strings.TrimSpace(ack.Email) == ""
}

// decodeSalesforceCidadaoResponse accepts the documented flat Person Account DTO.
// Legacy homolog/export-shaped payloads with nested dadosPessoais are still normalized
// for compatibility.
func decodeSalesforceCidadaoResponse(body []byte) (*SalesforceCidadao, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return &SalesforceCidadao{}, nil
	}

	var out SalesforceCidadao
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("failed to decode salesforce cidadao response: %w", err)
	}

	var envelope struct {
		DadosPessoais json.RawMessage `json:"dadosPessoais"`
		Consentimento []struct {
			Categoria  string `json:"categoria"`
			Acao       string `json:"acao"`
			Codigo     string `json:"codigo"`
			Status     string `json:"status"`
			Canal      string `json:"canal"`
			Data       string `json:"data"`
			Finalidade string `json:"finalidade"`
			Motivo     string `json:"motivo"`
		} `json:"consentimento"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return &out, nil
	}

	if len(envelope.DadosPessoais) > 0 && string(envelope.DadosPessoais) != "null" {
		var dp map[string]interface{}
		if err := json.Unmarshal(envelope.DadosPessoais, &dp); err == nil {
			applySalesforceDadosPessoais(&out, dp)
		}
	}

	if len(envelope.Consentimento) > 0 {
		mapped := make([]SalesforceConsentimento, 0, len(envelope.Consentimento))
		for _, item := range envelope.Consentimento {
			categoria := strings.TrimSpace(item.Categoria)
			if categoria == "" {
				categoria = strings.TrimSpace(item.Codigo)
			}
			acao := strings.TrimSpace(item.Acao)
			if categoria == "" && acao == "" {
				continue
			}
			mapped = append(mapped, SalesforceConsentimento{
				Categoria:  categoria,
				Acao:       acao,
				Status:     strings.TrimSpace(item.Status),
				Canal:      strings.TrimSpace(item.Canal),
				Data:       strings.TrimSpace(item.Data),
				Finalidade: strings.TrimSpace(item.Finalidade),
				Motivo:     strings.TrimSpace(item.Motivo),
			})
		}
		if len(mapped) > 0 {
			out.Consentimento = mapped
		}
	}

	return &out, nil
}

func applySalesforceDadosPessoais(out *SalesforceCidadao, dp map[string]interface{}) {
	if out == nil || len(dp) == 0 {
		return
	}

	setIfEmpty := func(dst *string, keys ...string) {
		if dst == nil || strings.TrimSpace(*dst) != "" {
			return
		}
		for _, key := range keys {
			if v := salesforceAnyString(dp[key]); v != "" {
				*dst = v
				return
			}
		}
	}

	setIfEmpty(&out.AccountID, "accountId")
	setIfEmpty(&out.CPF, "cpf")
	setIfEmpty(&out.Email, "email")
	setIfEmpty(&out.NomeSocial, "nomeSocial")
	setIfEmpty(&out.NomeExibicao, "nomeExibicao")
	setIfEmpty(&out.TelefonePrincipal, "telefonePrincipal")
	setIfEmpty(&out.Telefone1, "telefone1", "telefone")
	setIfEmpty(&out.Telefone2, "telefone2")
	setIfEmpty(&out.Telefone3, "telefone3")
	setIfEmpty(&out.Genero, "genero")
	setIfEmpty(&out.Raca, "raca")
	setIfEmpty(&out.Escolaridade, "escolaridade")
	setIfEmpty(&out.RendaFamiliar, "rendaFamiliar")
	setIfEmpty(&out.Deficiencia, "deficiencia")
	setIfEmpty(&out.Nacionalidade, "nacionalidade")
	setIfEmpty(&out.DataNascimento, "dataNascimento")
	setIfEmpty(&out.Complemento, "complemento")
	setIfEmpty(&out.CanalOrigem, "canalOrigem", "contaOrigem")
	setIfEmpty(&out.CanalUltimaModificacao, "canalUltimaModificacao")

	if strings.TrimSpace(out.Nome) == "" {
		if nome := salesforceAnyString(dp["nome"]); nome != "" {
			out.Nome = nome
		} else {
			out.Nome = strings.TrimSpace(salesforceAnyString(dp["primeiroNome"]) + " " + salesforceAnyString(dp["sobrenome"]))
		}
	}

	if out.Endereco == nil {
		if rawAddr, ok := dp["endereco"]; ok && rawAddr != nil {
			if b, err := json.Marshal(rawAddr); err == nil {
				var addr SalesforceEndereco
				if json.Unmarshal(b, &addr) == nil && (addr.Cidade != "" || addr.CEP != "" || addr.Logradouro != "") {
					out.Endereco = &addr
				}
			}
		}
	}
}

func salesforceAnyString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%.0f", t))
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	default:
		return ""
	}
}

// decodeSalesforceCreateResponse accepts the documented JSON object or a bare JSON string
// (some homolog gateways return only a status/accountId string on 201).
func decodeSalesforceCreateResponse(body []byte) (*SalesforceCidadaoCreateResponse, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return &SalesforceCidadaoCreateResponse{Status: "created"}, nil
	}

	switch body[0] {
	case '{':
		var out SalesforceCidadaoCreateResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("failed to decode salesforce create response: %w", err)
		}
		if out.Status == "" {
			out.Status = "created"
		}
		return &out, nil
	case '"':
		var asString string
		if err := json.Unmarshal(body, &asString); err != nil {
			return nil, fmt.Errorf("failed to decode salesforce create response: %w", err)
		}
		asString = strings.TrimSpace(asString)
		out := &SalesforceCidadaoCreateResponse{Status: "created"}
		if strings.HasPrefix(asString, "001") {
			out.AccountID = asString
		} else if asString != "" {
			out.Status = asString
		}
		return out, nil
	default:
		return nil, fmt.Errorf("failed to decode salesforce create response: %s", truncateForErr(string(body), 200))
	}
}

func truncateForErr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
