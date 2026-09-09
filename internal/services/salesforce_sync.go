package services

import (
	"context"
	"encoding/base64"
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
	"go.uber.org/zap"
)

const (
	// SalesforceSyncQueue applies inbound Salesforce Person Account data into RMI (origem=salesforce).
	SalesforceSyncQueue = "salesforce_sync"
	// SalesforcePushQueue pushes RMI citizen/self-declared changes to Salesforce.
	SalesforcePushQueue = "salesforce_push"

	// SalesforceContaOrigem is always sent as contaOrigem on Salesforce writes.
	SalesforceContaOrigem = "Portal Pref.Rio"

	// SalesforceWebhookEventAtualizacao is the default inbound webhook event (partial field patch).
	SalesforceWebhookEventAtualizacao = "atualizacao"
	// SalesforceWebhookEventAnonimizacao marks a masked snapshot after LGPD anonymization in SF.
	SalesforceWebhookEventAnonimizacao = "anonimizacao"

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

// SalesforceSyncPayload is enqueued after the inbound Salesforce webhook (delta in dados).
type SalesforceSyncPayload struct {
	CPF       string          `json:"cpf"`
	Evento    string          `json:"evento"`
	UpdatedAt string          `json:"updatedAt,omitempty"`
	Dados     json.RawMessage `json:"dados"`
}

// SalesforcePushPayload is enqueued after a successful RMI citizen/self-declared persist.
// BearerToken is accepted when reading older queue items; new jobs store the JWT only on SyncJob.
type SalesforcePushPayload struct {
	CPF         string `json:"cpf"`
	BearerToken string `json:"bearer_token,omitempty"`
}

// nonRetryableSyncError marks a sync failure that must go to DLQ without further retries.
type nonRetryableSyncError struct {
	err error
}

func (e *nonRetryableSyncError) Error() string {
	if e == nil || e.err == nil {
		return "non-retryable sync error"
	}
	return e.err.Error()
}

func (e *nonRetryableSyncError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func newNonRetryableSyncError(err error) error {
	if err == nil {
		return nil
	}
	return &nonRetryableSyncError{err: err}
}

func isNonRetryableSyncError(err error) bool {
	var nr *nonRetryableSyncError
	return errors.As(err, &nr)
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
func EnqueueSalesforceSyncJob(ctx context.Context, redis *redisclient.Client, cpf, evento, updatedAt string, rawDados json.RawMessage) error {
	if redis == nil {
		return fmt.Errorf("redis client is nil")
	}
	cpf = strings.TrimSpace(cpf)
	if cpf == "" {
		return fmt.Errorf("cpf is required")
	}
	if len(bytesTrimSpaceJSON(rawDados)) == 0 {
		return fmt.Errorf("dados is required")
	}
	evento = NormalizeSalesforceWebhookEvento(evento)
	if evento == "" {
		return fmt.Errorf("invalid salesforce webhook evento")
	}

	job := SyncJob{
		ID:         utils.GenerateUUID(),
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Data: SalesforceSyncPayload{
			CPF:       cpf,
			Evento:    evento,
			UpdatedAt: strings.TrimSpace(updatedAt),
			Dados:     rawDados,
		},
		Timestamp:  time.Now(),
		RetryCount: 0,
		MaxRetries: 3,
		Origin:     SyncOriginSalesforce,
	}
	return enqueueNamedSyncJob(ctx, redis, job)
}

func bytesTrimSpaceJSON(raw json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.TrimSpace(string(raw)))
}

// NormalizeSalesforceWebhookEvento returns a supported evento or empty string when invalid.
// Empty input defaults to atualizacao.
func NormalizeSalesforceWebhookEvento(evento string) string {
	evento = strings.ToLower(strings.TrimSpace(evento))
	if evento == "" {
		return SalesforceWebhookEventAtualizacao
	}
	switch evento {
	case SalesforceWebhookEventAtualizacao, SalesforceWebhookEventAnonimizacao:
		return evento
	default:
		return ""
	}
}

// EnqueueSalesforcePushJob queues an outbound push for the given CPF using the caller's JWT.
// The JWT is AES-256-GCM sealed on SyncJob.BearerToken (work queue only), not duplicated in Data,
// and stripped when the job is moved to the DLQ. Per-job lifetime is the JWT exp claim.
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

	sealed, err := prepareBearerForQueue(bearerToken, SalesforcePushQueue, cpf)
	if err != nil {
		return err
	}

	job := SyncJob{
		ID:         utils.GenerateUUID(),
		Type:       SalesforcePushQueue,
		Key:        cpf,
		Collection: SalesforcePushQueue,
		Data: SalesforcePushPayload{
			CPF: cpf,
		},
		Timestamp:   time.Now(),
		RetryCount:  0,
		MaxRetries:  5,
		BearerToken: sealed,
	}
	return enqueueNamedSyncJob(ctx, redis, job)
}

// EnqueueSalesforceConsentimentoMirror applies a successful RMI→SF consentimento PATCH
// into Mongo via the inbound salesforce_sync path (origin=salesforce, no push-back).
func EnqueueSalesforceConsentimentoMirror(ctx context.Context, redis *redisclient.Client, cpf string, req *clients.SalesforceConsentimentoPatchRequest) error {
	if redis == nil || req == nil {
		return nil
	}
	acao := strings.ToLower(strings.TrimSpace(req.Acao))
	status := "OUT"
	if acao == "optin" {
		status = "IN"
	}
	payload, err := json.Marshal(map[string]any{
		"consentimento": []clients.SalesforceConsentimento{{
			Categoria: strings.TrimSpace(req.Categoria),
			Acao:      acao,
			Status:    status,
			Motivo:    strings.TrimSpace(req.Motivo),
		}},
	})
	if err != nil {
		return err
	}
	return EnqueueSalesforceSyncJob(ctx, redis, cpf, SalesforceWebhookEventAtualizacao, "", payload)
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
	if bearer != "" {
		opened, err := bearerFromQueue(bearer, job.Type, job.Key)
		if err != nil {
			w.logger.Warn("skipping salesforce push enqueue: failed to open bearer token",
				zap.String("job_id", job.ID),
				zap.String("type", job.Type),
				zap.String("cpf", job.Key),
				zap.Error(err))
			return
		}
		bearer = opened
	}
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
	if cpf == "" || len(bytesTrimSpaceJSON(payload.Dados)) == 0 {
		return fmt.Errorf("invalid salesforce_sync payload")
	}
	evento := NormalizeSalesforceWebhookEvento(payload.Evento)
	if evento == "" {
		return fmt.Errorf("invalid salesforce_sync evento")
	}

	w.logger.Info("applying salesforce inbound sync",
		zap.String("job_id", job.ID),
		zap.String("cpf", cpf),
		zap.String("evento", evento))

	return w.applySalesforceDelta(ctx, cpf, evento, payload.UpdatedAt, payload.Dados, time.Now())
}

func (w *SyncWorker) invalidateSalesforceMirrorCaches(ctx context.Context, cpf string) {
	_ = w.redis.Del(ctx, fmt.Sprintf("citizen_wallet:%s", cpf)).Err()
	_ = w.redis.Del(ctx, fmt.Sprintf("user_config:%s", cpf)).Err()
	_ = w.redis.Del(ctx, fmt.Sprintf("citizen:cache:%s", cpf)).Err()
	_ = w.redis.Del(ctx, fmt.Sprintf("citizen:write:%s", cpf)).Err()
	for _, t := range []string{
		"self_declared_address", "self_declared_email", "self_declared_phone",
		"self_declared_raca", "self_declared_nome_exibicao", "self_declared_genero",
		"self_declared_renda_familiar", "self_declared_escolaridade", "self_declared_deficiencia",
	} {
		_ = w.redis.Del(ctx, fmt.Sprintf("%s:cache:%s", t, cpf)).Err()
		_ = w.redis.Del(ctx, fmt.Sprintf("%s:write:%s", t, cpf)).Err()
	}
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
	opened, err := bearerFromQueue(bearer, job.Type, job.Key)
	if err != nil {
		return newNonRetryableSyncError(fmt.Errorf("salesforce push failed: %w", err))
	}
	bearer = opened
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
	if salesforceBearerExpired(bearer) {
		return newNonRetryableSyncError(fmt.Errorf("salesforce push failed: bearer token expired"))
	}

	w.logger.Info("pushing citizen data to salesforce",
		zap.String("job_id", job.ID),
		zap.String("cpf", cpf))

	citizen, selfDeclared, citizenFound, selfDeclaredFound, err := w.loadCitizenBundle(ctx, cpf)
	if err != nil {
		return err
	}
	userConfig, _, err := w.loadUserConfig(ctx, cpf)
	if err != nil {
		return err
	}

	patch := buildSalesforceSnapshotPatch(citizen, selfDeclared, userConfig, citizenFound, selfDeclaredFound)

	_, err = sf.PatchCidadao(ctx, cpf, patch)
	if err == nil {
		return nil
	}

	var apiErr *clients.SalesforceAPIError
	if errors.As(err, &apiErr) && apiErr.IsConflict() {
		return nil
	}
	if errors.As(err, &apiErr) && apiErr.IsUnauthorized() {
		return newNonRetryableSyncError(fmt.Errorf("salesforce patch failed: %w", err))
	}
	if errors.As(err, &apiErr) && apiErr.IsNotFound() {
		createReq := buildSalesforceCreate(citizen, selfDeclared, userConfig, citizenFound, selfDeclaredFound, SalesforceContaOrigem)
		_, createErr := sf.CreateOrUpdateCidadao(ctx, createReq)
		if createErr != nil {
			wrapped := fmt.Errorf("salesforce create-or-update after 404 failed: %w", createErr)
			var createAPIErr *clients.SalesforceAPIError
			if errors.As(createErr, &createAPIErr) && createAPIErr.IsUnauthorized() {
				return newNonRetryableSyncError(wrapped)
			}
			return wrapped
		}
		return nil
	}
	return fmt.Errorf("salesforce patch failed: %w", err)
}

func (w *SyncWorker) loadCitizenBundle(ctx context.Context, cpf string) (*models.Citizen, *models.SelfDeclaredData, bool, bool, error) {
	var citizen models.Citizen
	citizenFound := true
	err := w.mongo.Collection(config.AppConfig.CitizenCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil, false, false, fmt.Errorf("failed to load citizen: %w", err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		citizenFound = false
		citizen.CPF = cpf
	}

	var selfDeclared models.SelfDeclaredData
	selfDeclaredFound := true
	err = w.mongo.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&selfDeclared)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil, false, false, fmt.Errorf("failed to load self_declared: %w", err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		selfDeclaredFound = false
		selfDeclared.CPF = cpf
	}
	return &citizen, &selfDeclared, citizenFound, selfDeclaredFound, nil
}

func (w *SyncWorker) loadUserConfig(ctx context.Context, cpf string) (*models.UserConfig, bool, error) {
	if config.AppConfig == nil || strings.TrimSpace(config.AppConfig.UserConfigCollection) == "" {
		return nil, false, nil
	}
	var uc models.UserConfig
	err := w.mongo.Collection(config.AppConfig.UserConfigCollection).FindOne(ctx, bson.M{"cpf": cpf}).Decode(&uc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to load user_config: %w", err)
	}
	return &uc, true, nil
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

func buildSalesforceTelefoneField(phone string, now time.Time) *models.Telefone {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil
	}
	components, err := utils.ParsePhoneNumber(phone)
	if err != nil {
		return nil
	}
	ddi, ddd, valor := components.DDI, components.DDD, components.Valor
	origem := salesforceOrigem
	sistema := salesforceSistema
	return &models.Telefone{
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
	v = strings.TrimSpace(strings.ToLower(stripPTAccents(v)))
	if v == "" {
		return ""
	}
	switch v {
	case "branca":
		return "Branca"
	case "preta":
		return "Preta"
	case "parda":
		return "Parda"
	case "amarela":
		return "Amarela"
	case "indigena":
		return "Indigena"
	case "outra":
		return "Outra"
	default:
		// Already a Salesforce picklist value (e.g. from a prior GET).
		if strings.TrimSpace(v) != strings.ToLower(v) {
			return strings.TrimSpace(v)
		}
		runes := []rune(v)
		if len(runes) == 0 {
			return ""
		}
		runes[0] = unicode.ToUpper(runes[0])
		return string(runes)
	}
}

func mapSalesforceRacaToRMI(v string) string {
	v = strings.TrimSpace(strings.ToLower(stripPTAccents(v)))
	if v == "" {
		return ""
	}
	return v
}

// salesforceBearerExpired reports whether a three-part JWT has a past exp claim.
// Opaque tokens (including test placeholders like "jwt") are not treated as expired.
func salesforceBearerExpired(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := decodeJWTPayloadSegment(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	if claims.Exp <= 0 {
		return false
	}
	return time.Now().Unix() >= claims.Exp
}

func decodeJWTPayloadSegment(segment string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(segment); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(segment)
}

func firstName(full string) string {
	parts := strings.Fields(strings.TrimSpace(full))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
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
