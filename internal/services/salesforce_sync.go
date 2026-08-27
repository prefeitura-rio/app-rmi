package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

const (
	// SalesforceSyncQueue applies inbound Salesforce Person Account data into RMI (origem=salesforce).
	SalesforceSyncQueue = "salesforce_sync"
	// SalesforcePushQueue pushes RMI citizen/self-declared changes to Salesforce.
	SalesforcePushQueue = "salesforce_push"

	// SalesforceContaOrigem is always sent as contaOrigem on Salesforce writes.
	SalesforceContaOrigem = "Portal Pref.Rio"

	salesforceSistema = "salesforce"
	salesforceOrigem  = "salesforce"
)

// SalesforceCidadaoAPI is the subset of the Salesforce client used by sync workers.
type SalesforceCidadaoAPI interface {
	Configured() bool
	CreateOrUpdateCidadao(ctx context.Context, req *clients.SalesforceCidadaoCreateRequest) (*clients.SalesforceCidadaoCreateResponse, error)
	GetCidadao(ctx context.Context, cpf string) (*clients.SalesforceCidadao, error)
	PatchCidadao(ctx context.Context, cpf string, req *clients.SalesforceCidadaoPatchRequest) (*clients.SalesforceCidadao, error)
}

// SalesforceSyncPayload is enqueued after the webhook GETs Person Account data.
type SalesforceSyncPayload struct {
	CPF     string                     `json:"cpf"`
	Cidadao *clients.SalesforceCidadao `json:"cidadao"`
}

// SalesforcePushPayload is enqueued after a successful RMI citizen/self-declared persist.
type SalesforcePushPayload struct {
	CPF         string `json:"cpf"`
	BearerToken string `json:"bearer_token,omitempty"`
}

// SalesforceConfigured reports whether Salesforce CRM sync can run (base URL set).
// Outbound calls still require a Bearer JWT from the authenticated request.
func SalesforceConfigured() bool {
	return SalesforceBaseURLConfigured()
}

// SalesforceBaseURLConfigured reports whether the Salesforce API base URL is set.
func SalesforceBaseURLConfigured() bool {
	return config.AppConfig != nil && strings.TrimSpace(config.AppConfig.SalesforceBaseURL) != ""
}

func newSalesforceClientWithJWT(bearer string) *clients.SalesforceClient {
	if !SalesforceBaseURLConfigured() {
		return nil
	}
	timeout := config.AppConfig.SalesforceTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return clients.NewSalesforceClient(
		config.AppConfig.SalesforceBaseURL,
		timeout,
		clients.StaticBearerToken(bearer),
	)
}

// EnqueueSalesforceSyncJob queues an inbound apply job (worker persists; no push back).
func EnqueueSalesforceSyncJob(ctx context.Context, redis *redisclient.Client, cpf string, cidadao *clients.SalesforceCidadao) error {
	if redis == nil {
		return fmt.Errorf("redis client is nil")
	}
	cpf = strings.TrimSpace(cpf)
	if cpf == "" {
		return fmt.Errorf("cpf is required")
	}
	if cidadao == nil {
		return fmt.Errorf("cidadao payload is nil")
	}

	job := SyncJob{
		ID:         utils.GenerateUUID(),
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Data: SalesforceSyncPayload{
			CPF:     cpf,
			Cidadao: cidadao,
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
		Origin:     SyncOriginSalesforce,
	}
	return enqueueNamedSyncJob(ctx, redis, job)
}

// EnqueueSalesforcePushJob queues an outbound push for the given CPF using the caller's JWT.
func EnqueueSalesforcePushJob(ctx context.Context, redis *redisclient.Client, cpf, bearerToken string) error {
	if redis == nil {
		return fmt.Errorf("redis client is nil")
	}
	cpf = strings.TrimSpace(cpf)
	if cpf == "" {
		return fmt.Errorf("cpf is required")
	}
	bearerToken = strings.TrimSpace(bearerToken)
	if bearerToken == "" {
		return fmt.Errorf("bearer token is required for salesforce push")
	}

	job := SyncJob{
		ID:         utils.GenerateUUID(),
		Type:       SalesforcePushQueue,
		Key:        cpf,
		Collection: SalesforcePushQueue,
		Data: SalesforcePushPayload{
			CPF:         cpf,
			BearerToken: bearerToken,
		},
		Timestamp:   time.Now(),
		RetryCount:  0,
		MaxRetries:  3,
		BearerToken: bearerToken,
	}
	return enqueueNamedSyncJob(ctx, redis, job)
}

func enqueueNamedSyncJob(ctx context.Context, redis *redisclient.Client, job SyncJob) error {
	jobBytes, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal sync job: %w", err)
	}
	queueKey := fmt.Sprintf("sync:queue:%s", job.Type)
	if err := redis.LPush(ctx, queueKey, string(jobBytes)).Err(); err != nil {
		return fmt.Errorf("failed to queue sync job: %w", err)
	}
	return nil
}

func shouldEnqueueSalesforcePush(job *SyncJob) bool {
	if job == nil {
		return false
	}
	if job.Origin == SyncOriginSalesforce {
		return false
	}
	switch job.Type {
	case "citizen",
		"self_declared_address",
		"self_declared_email",
		"self_declared_phone",
		"self_declared_raca",
		"self_declared_nome_exibicao",
		"self_declared_genero",
		"self_declared_renda_familiar",
		"self_declared_escolaridade",
		"self_declared_deficiencia":
		return true
	default:
		return false
	}
}

func (w *SyncWorker) maybeEnqueueSalesforcePush(job *SyncJob) {
	if w == nil || !SalesforceBaseURLConfigured() {
		return
	}
	if !shouldEnqueueSalesforcePush(job) {
		return
	}
	bearer := strings.TrimSpace(job.BearerToken)
	if bearer == "" {
		w.logger.Debug("skipping salesforce push enqueue: missing bearer token on sync job",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("cpf", job.Key))
		return
	}
	ctx := context.Background()
	if err := EnqueueSalesforcePushJob(ctx, w.redis, job.Key, bearer); err != nil {
		w.logger.Warn("failed to enqueue salesforce push after sync",
			zap.String("job_id", job.ID),
			zap.String("type", job.Type),
			zap.String("cpf", job.Key),
			zap.Error(err))
		return
	}
	w.logger.Debug("enqueued salesforce push after sync",
		zap.String("job_id", job.ID),
		zap.String("type", job.Type),
		zap.String("cpf", job.Key))
}

func (w *SyncWorker) handleSalesforceSyncJob(ctx context.Context, job *SyncJob) error {
	payload, err := parseSalesforceSyncPayload(job.Data)
	if err != nil {
		return err
	}
	cpf := payload.CPF
	if cpf == "" {
		cpf = job.Key
	}
	if cpf == "" || payload.Cidadao == nil {
		return fmt.Errorf("invalid salesforce_sync payload")
	}

	w.logger.Info("applying salesforce inbound sync",
		zap.String("job_id", job.ID),
		zap.String("cpf", cpf),
		zap.String("account_id", payload.Cidadao.AccountID))

	setFields := mapSalesforceCidadaoToSelfDeclared(payload.Cidadao)
	if len(setFields) == 0 {
		w.logger.Info("salesforce inbound sync had no mappable fields",
			zap.String("cpf", cpf))
		return nil
	}
	setFields["cpf"] = cpf
	setFields["updated_at"] = time.Now()

	coll := w.mongo.Collection(config.AppConfig.SelfDeclaredCollection)
	_, err = coll.UpdateOne(ctx,
		bson.M{"cpf": cpf},
		bson.M{"$set": setFields},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("failed to apply salesforce sync to self_declared: %w", err)
	}

	_ = w.redis.Del(ctx, fmt.Sprintf("citizen_wallet:%s", cpf)).Err()
	for _, t := range []string{
		"self_declared_address", "self_declared_email", "self_declared_phone",
		"self_declared_raca", "self_declared_nome_exibicao", "self_declared_genero",
		"self_declared_renda_familiar", "self_declared_escolaridade", "self_declared_deficiencia",
	} {
		_ = w.redis.Del(ctx, fmt.Sprintf("%s:cache:%s", t, cpf)).Err()
		_ = w.redis.Del(ctx, fmt.Sprintf("%s:write:%s", t, cpf)).Err()
	}

	return nil
}

func (w *SyncWorker) handleSalesforcePushJob(ctx context.Context, job *SyncJob) error {
	cpf := job.Key
	bearer := strings.TrimSpace(job.BearerToken)
	if payload, err := parseSalesforcePushPayload(job.Data); err == nil {
		if payload.CPF != "" {
			cpf = payload.CPF
		}
		if payload.BearerToken != "" {
			bearer = payload.BearerToken
		}
	}
	if cpf == "" {
		return fmt.Errorf("cpf is required for salesforce push")
	}
	if bearer == "" {
		return fmt.Errorf("bearer token is required for salesforce push")
	}

	sf := w.salesforce
	if sf == nil {
		sf = newSalesforceClientWithJWT(bearer)
	} else if c, ok := sf.(*clients.SalesforceClient); ok {
		sf = c.WithBearerToken(bearer)
	}
	if sf == nil || !sf.Configured() {
		w.logger.Debug("salesforce push skipped: client not configured",
			zap.String("job_id", job.ID))
		return nil
	}

	w.logger.Info("pushing citizen data to salesforce",
		zap.String("job_id", job.ID),
		zap.String("cpf", cpf))

	citizen, selfDeclared, err := w.loadCitizenBundle(ctx, cpf)
	if err != nil {
		return err
	}

	patch := buildSalesforcePatch(citizen, selfDeclared)
	patch.ContaOrigem = SalesforceContaOrigem

	_, err = sf.PatchCidadao(ctx, cpf, patch)
	if err == nil {
		return nil
	}

	var apiErr *clients.SalesforceAPIError
	if errors.As(err, &apiErr) && apiErr.IsNotFound() {
		createReq := buildSalesforceCreate(citizen, selfDeclared, SalesforceContaOrigem)
		_, createErr := sf.CreateOrUpdateCidadao(ctx, createReq)
		if createErr != nil {
			return fmt.Errorf("salesforce create-or-update after 404 failed: %w", createErr)
		}
		return nil
	}
	return fmt.Errorf("salesforce patch failed: %w", err)
}

func (w *SyncWorker) loadCitizenBundle(ctx context.Context, cpf string) (*models.Citizen, *models.SelfDeclaredData, error) {
	var citizen models.Citizen
	err := w.mongo.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil, fmt.Errorf("failed to load citizen: %w", err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		citizen.CPF = cpf
	}

	var selfDeclared models.SelfDeclaredData
	err = w.mongo.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&selfDeclared)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil, fmt.Errorf("failed to load self_declared: %w", err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		selfDeclared.CPF = cpf
	}
	return &citizen, &selfDeclared, nil
}

func parseSalesforceSyncPayload(data interface{}) (*SalesforceSyncPayload, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("invalid salesforce_sync data: %w", err)
	}
	var payload SalesforceSyncPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid salesforce_sync data: %w", err)
	}
	return &payload, nil
}

func parseSalesforcePushPayload(data interface{}) (*SalesforcePushPayload, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("invalid salesforce_push data: %w", err)
	}
	var payload SalesforcePushPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid salesforce_push data: %w", err)
	}
	return &payload, nil
}

func mapSalesforceCidadaoToSelfDeclared(c *clients.SalesforceCidadao) bson.M {
	set := bson.M{}
	now := time.Now()

	if email := strings.TrimSpace(c.Email); email != "" {
		emailVal := email
		origem := salesforceOrigem
		sistema := salesforceSistema
		set["email"] = &models.Email{
			Indicador: utils.BoolPtr(true),
			Principal: &models.EmailPrincipal{
				Valor:     &emailVal,
				Origem:    &origem,
				Sistema:   &sistema,
				UpdatedAt: &now,
			},
		}
	}

	phone := firstNonEmpty(c.Telefone1, c.TelefonePrincipal)
	if phone != "" {
		if components, err := utils.ParsePhoneNumber(phone); err == nil {
			ddi, ddd, valor := components.DDI, components.DDD, components.Valor
			origem := salesforceOrigem
			sistema := salesforceSistema
			set["telefone"] = &models.Telefone{
				Indicador: utils.BoolPtr(true),
				Principal: &models.TelefonePrincipal{
					DDI:       &ddi,
					DDD:       &ddd,
					Valor:     &valor,
					Origem:    &origem,
					Sistema:   &sistema,
					UpdatedAt: &now,
				},
			}
		}
	}

	if c.Endereco != nil {
		logradouro := strings.TrimSpace(c.Endereco.Logradouro)
		cidade := strings.TrimSpace(c.Endereco.Cidade)
		estado := strings.TrimSpace(c.Endereco.Estado)
		cep := strings.TrimSpace(c.Endereco.CEP)
		if logradouro != "" || cidade != "" || estado != "" || cep != "" {
			origem := salesforceOrigem
			sistema := salesforceSistema
			set["endereco"] = &models.Endereco{
				Indicador: utils.BoolPtr(true),
				Principal: &models.EnderecoPrincipal{
					Logradouro: &logradouro,
					Municipio:  &cidade,
					Estado:     &estado,
					CEP:        &cep,
					Origem:     &origem,
					Sistema:    &sistema,
					UpdatedAt:  &now,
				},
			}
		}
	}

	if v := mapSalesforceRacaToRMI(c.Raca); v != "" {
		raca := v
		set["raca"] = &raca
	}
	if v := mapSalesforceGeneroToRMI(c.Genero); v != "" {
		genero := v
		set["genero"] = &genero
	}
	if v := strings.TrimSpace(c.NomeExibicao); v != "" {
		nome := v
		set["nome_exibicao"] = &nome
	} else if v := strings.TrimSpace(c.NomeSocial); v != "" {
		nome := v
		set["nome_exibicao"] = &nome
	}
	if v := strings.TrimSpace(c.Escolaridade); v != "" {
		esc := v
		set["escolaridade"] = &esc
	}
	if v := strings.TrimSpace(c.RendaFamiliar); v != "" {
		renda := v
		set["renda_familiar"] = &renda
	}
	if v := strings.TrimSpace(c.Deficiencia); v != "" {
		def := v
		set["deficiencia"] = &def
	}

	return set
}

func buildSalesforcePatch(citizen *models.Citizen, sd *models.SelfDeclaredData) *clients.SalesforceCidadaoPatchRequest {
	req := &clients.SalesforceCidadaoPatchRequest{}

	if sd != nil && sd.Email != nil && sd.Email.Principal != nil && sd.Email.Principal.Valor != nil {
		req.Email = strings.TrimSpace(*sd.Email.Principal.Valor)
	} else if citizen != nil && citizen.Email != nil && citizen.Email.Principal != nil && citizen.Email.Principal.Valor != nil {
		req.Email = strings.TrimSpace(*citizen.Email.Principal.Valor)
	}

	if phone := extractPhoneDigits(sd, citizen); phone != "" {
		req.Telefone1 = clients.NormalizeSalesforcePhone(phone)
	}

	if sd != nil && sd.Endereco != nil && sd.Endereco.Principal != nil {
		p := sd.Endereco.Principal
		req.Endereco = &clients.SalesforceEndereco{
			Logradouro: deref(p.Logradouro),
			Cidade:     deref(p.Municipio),
			Estado:     deref(p.Estado),
			CEP:        deref(p.CEP),
			Pais:       "Brasil",
		}
		req.Cidade = deref(p.Municipio)
	} else if citizen != nil && citizen.Endereco != nil && citizen.Endereco.Principal != nil {
		p := citizen.Endereco.Principal
		req.Endereco = &clients.SalesforceEndereco{
			Logradouro: deref(p.Logradouro),
			Cidade:     deref(p.Municipio),
			Estado:     deref(p.Estado),
			CEP:        deref(p.CEP),
			Pais:       "Brasil",
		}
		req.Cidade = deref(p.Municipio)
	}

	if sd != nil && sd.Raca != nil {
		req.Raca = mapRMIRacaToSalesforce(*sd.Raca)
	}
	if sd != nil && sd.Genero != nil {
		req.Genero = mapRMIGeneroToSalesforce(*sd.Genero)
	}
	if sd != nil && sd.Escolaridade != nil {
		req.Escolaridade = strings.TrimSpace(*sd.Escolaridade)
	}
	if sd != nil && sd.RendaFamiliar != nil {
		req.RendaFamiliar = strings.TrimSpace(*sd.RendaFamiliar)
	}
	if sd != nil && sd.Deficiencia != nil {
		req.Deficiencia = strings.TrimSpace(*sd.Deficiencia)
	}
	if sd != nil && sd.NomeExibicao != nil {
		req.NomeExibicao = strings.TrimSpace(*sd.NomeExibicao)
	}
	if citizen != nil && citizen.Nome != nil {
		req.PrimeiroNome = firstName(*citizen.Nome)
	}
	req.Idioma = "Portugues_Brasil"
	return req
}

func buildSalesforceCreate(citizen *models.Citizen, sd *models.SelfDeclaredData, contaOrigem string) *clients.SalesforceCidadaoCreateRequest {
	req := &clients.SalesforceCidadaoCreateRequest{
		ContaOrigem: contaOrigem,
		Idioma:      "Portugues_Brasil",
	}
	if citizen != nil {
		req.CPF = citizen.CPF
		if citizen.Nome != nil {
			req.Nome = strings.TrimSpace(*citizen.Nome)
		}
	}
	if sd != nil {
		req.CPF = firstNonEmpty(req.CPF, sd.CPF)
	}
	patch := buildSalesforcePatch(citizen, sd)
	req.Email = patch.Email
	req.Telefone1 = patch.Telefone1
	req.Genero = patch.Genero
	req.Raca = patch.Raca
	return req
}

func extractPhoneDigits(sd *models.SelfDeclaredData, citizen *models.Citizen) string {
	if sd != nil && sd.Telefone != nil && sd.Telefone.Principal != nil {
		p := sd.Telefone.Principal
		if p.DDI != nil && p.DDD != nil && p.Valor != nil {
			return utils.FormatPhoneForStorage(*p.DDI, *p.DDD, *p.Valor)
		}
	}
	if citizen != nil && citizen.Telefone != nil && citizen.Telefone.Principal != nil {
		p := citizen.Telefone.Principal
		if p.DDI != nil && p.DDD != nil && p.Valor != nil {
			return utils.FormatPhoneForStorage(*p.DDI, *p.DDD, *p.Valor)
		}
	}
	return ""
}

func mapRMIGeneroToSalesforce(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// Already a Salesforce picklist API value.
	switch v {
	case "Homem_cisgenero",
		"Mulher_cisgenero",
		"Homem_transgenero",
		"Mulher_transgenero",
		"Nao_binario",
		"Outro_Prefere_nao_informar":
		return v
	}

	normalized := strings.ToLower(stripPTAccents(v))
	switch normalized {
	case "homem cisgenero":
		return "Homem_cisgenero"
	case "mulher cisgenero":
		return "Mulher_cisgenero"
	case "homem transgenero":
		return "Homem_transgenero"
	case "mulher transgenero":
		return "Mulher_transgenero"
	case "nao binario":
		return "Nao_binario"
	case "prefiro nao informar", "outro", "outro prefere nao informar":
		return "Outro_Prefere_nao_informar"
	default:
		return ""
	}
}

func mapSalesforceGeneroToRMI(v string) string {
	switch strings.TrimSpace(v) {
	case "Homem_cisgenero":
		return "Homem cisgênero"
	case "Mulher_cisgenero":
		return "Mulher cisgênero"
	case "Homem_transgenero":
		return "Homem transgênero"
	case "Mulher_transgenero":
		return "Mulher transgênero"
	case "Nao_binario":
		return "Não binário"
	case "Outro_Prefere_nao_informar", "Prefiro_nao_informar":
		return "Prefiro não informar"
	default:
		return ""
	}
}

func mapRMIRacaToSalesforce(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return ""
	}
	runes := []rune(v)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func mapSalesforceRacaToRMI(v string) string {
	v = strings.TrimSpace(strings.ToLower(stripPTAccents(v)))
	if v == "" {
		return ""
	}
	return v
}

func firstName(full string) string {
	parts := strings.Fields(strings.TrimSpace(full))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func stripPTAccents(s string) string {
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "ô", "o", "õ", "o",
		"ú", "u", "ü", "u",
		"ç", "c",
		"Á", "A", "À", "A", "Ã", "A", "Â", "A",
		"É", "E", "Ê", "E",
		"Í", "I",
		"Ó", "O", "Ô", "O", "Õ", "O",
		"Ú", "U", "Ü", "U",
		"Ç", "C",
	)
	return replacer.Replace(s)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
