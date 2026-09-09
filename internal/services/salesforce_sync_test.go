package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestShouldEnqueueSalesforcePush(t *testing.T) {
	assert.False(t, shouldEnqueueSalesforcePush(nil))
	assert.False(t, shouldEnqueueSalesforcePush(&SyncJob{Type: "self_declared_email", Origin: SyncOriginSalesforce}))
	assert.False(t, shouldEnqueueSalesforcePush(&SyncJob{Type: "phone_mapping"}))
	assert.True(t, shouldEnqueueSalesforcePush(&SyncJob{Type: "self_declared_email"}))
	assert.True(t, shouldEnqueueSalesforcePush(&SyncJob{Type: "citizen"}))
	assert.True(t, shouldEnqueueSalesforcePush(&SyncJob{Type: "self_declared_genero"}))
}

func TestMapRMIGeneroToSalesforce(t *testing.T) {
	assert.Equal(t, "Homem_cisgenero", mapRMIGeneroToSalesforce("Homem cisgênero"))
	assert.Equal(t, "Mulher_cisgenero", mapRMIGeneroToSalesforce("Mulher cisgênero"))
	assert.Equal(t, "Homem_transgenero", mapRMIGeneroToSalesforce("Homem transgênero"))
	assert.Equal(t, "Mulher_transgenero", mapRMIGeneroToSalesforce("Mulher transgênero"))
	assert.Equal(t, "Nao_binario", mapRMIGeneroToSalesforce("Não binário"))
	assert.Equal(t, "Outro_Prefere_nao_informar", mapRMIGeneroToSalesforce("Prefiro não informar"))
	assert.Equal(t, "Outro_Prefere_nao_informar", mapRMIGeneroToSalesforce("Outro"))
	assert.Equal(t, "Homem_cisgenero", mapRMIGeneroToSalesforce("Homem_cisgenero"))
	assert.Equal(t, "", mapRMIGeneroToSalesforce("valor inventado"))
}

func TestMapSalesforceGeneroToRMI(t *testing.T) {
	assert.Equal(t, "Homem cisgênero", mapSalesforceGeneroToRMI("Homem_cisgenero"))
	assert.Equal(t, "Mulher cisgênero", mapSalesforceGeneroToRMI("Mulher_cisgenero"))
	assert.Equal(t, "Homem transgênero", mapSalesforceGeneroToRMI("Homem_transgenero"))
	assert.Equal(t, "Mulher transgênero", mapSalesforceGeneroToRMI("Mulher_transgenero"))
	assert.Equal(t, "Não binário", mapSalesforceGeneroToRMI("Nao_binario"))
	assert.Equal(t, "Prefiro não informar", mapSalesforceGeneroToRMI("Outro_Prefere_nao_informar"))
	assert.Equal(t, "", mapSalesforceGeneroToRMI("Desconhecido"))
}

func TestMapRMIRacaToSalesforce(t *testing.T) {
	assert.Equal(t, "Parda", mapRMIRacaToSalesforce("parda"))
	assert.Equal(t, "Branca", mapRMIRacaToSalesforce("branca"))
	assert.Equal(t, "Preta", mapRMIRacaToSalesforce("preta"))
	assert.Equal(t, "Amarela", mapRMIRacaToSalesforce("amarela"))
	assert.Equal(t, "Indigena", mapRMIRacaToSalesforce("indigena"))
	assert.Equal(t, "Outra", mapRMIRacaToSalesforce("outra"))
	assert.Equal(t, "", mapRMIRacaToSalesforce(""))
}

func TestMapSalesforceCidadaoToSelfDeclared(t *testing.T) {
	cidadao := &clients.SalesforceCidadao{
		Email:     "maria@test.com",
		Telefone1: "5521988888888",
		Genero:    "Mulher_cisgenero",
		Raca:      "Parda",
		Endereco: &clients.SalesforceEndereco{
			Logradouro: "Rua A",
			Cidade:     "Rio de Janeiro",
			Estado:     "RJ",
			CEP:        "20000000",
		},
		NomeExibicao: "Maria",
	}
	set := mapSalesforceCidadaoToSelfDeclared(cidadao)
	require.Contains(t, set, "email")
	require.Contains(t, set, "telefone")
	require.Contains(t, set, "endereco")
	require.Contains(t, set, "genero")
	require.Contains(t, set, "raca")
	require.Contains(t, set, "nome_exibicao")

	genero, ok := set["genero"].(*string)
	require.True(t, ok)
	assert.Equal(t, "Mulher cisgênero", *genero)

	raca, ok := set["raca"].(*string)
	require.True(t, ok)
	assert.Equal(t, "parda", *raca)
}

func TestMapSalesforceConsentimentoToOptIn(t *testing.T) {
	optIn, cats := mapSalesforceConsentimentoToOptIn([]clients.SalesforceConsentimento{
		{Categoria: "Comunicacao", Status: "IN"},
		{Categoria: "Marketing", Acao: "optout", Motivo: "nao quero"},
	})
	assert.True(t, optIn)
	assert.True(t, cats["Comunicacao"])
	assert.False(t, cats["Marketing"])
}

func TestNormalizeSalesforceWebhookEvento(t *testing.T) {
	assert.Equal(t, SalesforceWebhookEventAtualizacao, NormalizeSalesforceWebhookEvento(""))
	assert.Equal(t, SalesforceWebhookEventAtualizacao, NormalizeSalesforceWebhookEvento("atualizacao"))
	assert.Equal(t, SalesforceWebhookEventAnonimizacao, NormalizeSalesforceWebhookEvento("anonimizacao"))
	assert.Equal(t, "", NormalizeSalesforceWebhookEvento("foo"))
}

func TestBuildSalesforceSnapshotPatch(t *testing.T) {
	nome := "Maria Silva"
	email := "maria@test.com"
	genero := "Homem cisgênero"
	raca := "parda"
	ddi, ddd, valor := "55", "21", "988888888"

	citizen := &models.Citizen{
		CPF:  "14202478754",
		Nome: &nome,
	}
	sd := &models.SelfDeclaredData{
		CPF:    "14202478754",
		Genero: &genero,
		Raca:   &raca,
		Email: &models.Email{
			Principal: &models.EmailPrincipal{Valor: &email},
		},
		Telefone: &models.Telefone{
			Principal: &models.TelefonePrincipal{DDI: &ddi, DDD: &ddd, Valor: &valor},
		},
		Idioma: []string{"Portugues_Brasil"},
	}

	patch := buildSalesforceSnapshotPatch(citizen, sd, nil, true, true)
	fields := patch.Fields()
	assert.Equal(t, "maria@test.com", fields["email"])
	assert.Equal(t, "5521988888888", fields["telefone1"])
	assert.Equal(t, "Homem_cisgenero", fields["genero"])
	assert.Equal(t, "Parda", fields["raca"])
	assert.Equal(t, "Maria", fields["primeiroNome"])
	assert.Equal(t, "Maria Silva", fields["nome"])
	assert.Equal(t, []string{"Portugues_Brasil"}, fields["idioma"])
	assert.Equal(t, SalesforceContaOrigem, fields["contaOrigem"])
	assert.NotContains(t, fields, "escolaridade")
	assert.NotContains(t, fields, "telefone2")
	assert.NotContains(t, fields, "endereco")
	assert.NotContains(t, fields, "nomeSocial")
	assert.NotContains(t, fields, "cidade")
	for k, v := range fields {
		assert.NotNil(t, v, k)
	}
}

func TestBuildSalesforceSnapshotPatch_OmitsIdiomaWhenUnset(t *testing.T) {
	sd := &models.SelfDeclaredData{CPF: "14202478754"}
	patch := buildSalesforceSnapshotPatch(nil, sd, nil, false, true)
	fields := patch.Fields()
	assert.NotContains(t, fields, "idioma")
	assert.NotContains(t, fields, "isTourist")
}

func TestBuildSalesforceSnapshotPatch_IncludesConsentimentoFromUserConfig(t *testing.T) {
	uc := &models.UserConfig{
		CPF: "14202478754",
		SalesforceConsentimentos: map[string]models.SalesforceConsentimentoEntry{
			"PREF_X|Portal Pref.Rio": {
				Categoria: "PREF_X",
				Status:    "IN",
				Canal:     "Portal Pref.Rio",
				OptIn:     true,
			},
		},
	}
	patch := buildSalesforceSnapshotPatch(nil, nil, uc, false, false)
	fields := patch.Fields()
	items, ok := fields["consentimento"].([]clients.SalesforceConsentimento)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Equal(t, "PREF_X", items[0].Categoria)
	assert.Equal(t, "IN", items[0].Status)
}

func TestBuildSalesforceSnapshotPatch_MirrorsTelefonePrincipal(t *testing.T) {
	ddi, ddd, valor := "55", "21", "988888888"
	sd := &models.SelfDeclaredData{
		CPF: "02075979600",
		Telefone: &models.Telefone{
			Principal: &models.TelefonePrincipal{DDI: &ddi, DDD: &ddd, Valor: &valor},
		},
	}
	patch := buildSalesforceSnapshotPatch(nil, sd, nil, false, true)
	fields := patch.Fields()
	assert.Equal(t, "5521988888888", fields["telefone1"])
	assert.Equal(t, "5521988888888", fields["telefonePrincipal"])
}

func TestBuildSalesforceSnapshotPatch_OmitsAbsentSelfDeclaredFields(t *testing.T) {
	genero := "Homem cisgênero"
	sd := &models.SelfDeclaredData{
		CPF:    "14202478754",
		Genero: &genero,
	}
	patch := buildSalesforceSnapshotPatch(nil, sd, nil, false, true)
	fields := patch.Fields()
	assert.Equal(t, "Homem_cisgenero", fields["genero"])
	assert.NotContains(t, fields, "escolaridade")
	assert.NotContains(t, fields, "email")
	assert.NotContains(t, fields, "telefone1")
	assert.NotContains(t, fields, "raca")
	assert.NotContains(t, fields, "endereco")

	sd.Genero = nil
	patch = buildSalesforceSnapshotPatch(nil, sd, nil, false, true)
	fields = patch.Fields()
	assert.NotContains(t, fields, "genero")
}

func TestBuildSalesforceSnapshotPatch_ClearsExplicitEmpty(t *testing.T) {
	empty := ""
	ddi, ddd, valor := "55", "21", "988888888"
	sd := &models.SelfDeclaredData{
		CPF:    "14202478754",
		Genero: &empty,
		Email:  &models.Email{Principal: &models.EmailPrincipal{Valor: nil}},
		Telefone: &models.Telefone{
			Principal:   &models.TelefonePrincipal{DDI: &ddi, DDD: &ddd, Valor: &valor},
			Alternativo: []models.TelefoneAlternativo{{Valor: &empty}},
		},
		Endereco: &models.Endereco{Principal: &models.EnderecoPrincipal{}},
	}
	patch := buildSalesforceSnapshotPatch(nil, sd, nil, false, true)
	raw, err := json.Marshal(patch)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"genero":null`)
	assert.Contains(t, string(raw), `"email":null`)
	assert.Contains(t, string(raw), `"telefone1":"5521988888888"`)
	assert.Contains(t, string(raw), `"telefone2":null`)
	assert.Contains(t, string(raw), `"endereco":null`)
	assert.NotContains(t, string(raw), `"telefone3"`)
}

func TestBuildSalesforceSnapshotPatch_UsesCitizenWhenSelfDeclaredLacksField(t *testing.T) {
	nome := "Maria Silva"
	raca := "branca"
	ddi, ddd, valor := "55", "21", "988888888"
	citizen := &models.Citizen{
		CPF:  "14202478754",
		Nome: &nome,
		Raca: &raca,
		Telefone: &models.Telefone{
			Principal: &models.TelefonePrincipal{DDI: &ddi, DDD: &ddd, Valor: &valor},
		},
	}
	sd := &models.SelfDeclaredData{CPF: "14202478754"}
	patch := buildSalesforceSnapshotPatch(citizen, sd, nil, true, true)
	fields := patch.Fields()
	assert.Equal(t, "Branca", fields["raca"])
	assert.Equal(t, "5521988888888", fields["telefone1"])
	assert.Equal(t, "Maria Silva", fields["nome"])
	assert.NotContains(t, fields, "genero")
	assert.NotContains(t, fields, "email")
}

func TestBuildSalesforceSnapshotPatch_OmitsWhenNoMongoDoc(t *testing.T) {
	patch := buildSalesforceSnapshotPatch(nil, nil, nil, false, false)
	fields := patch.Fields()
	assert.NotContains(t, fields, "genero")
	assert.NotContains(t, fields, "email")
	assert.Equal(t, SalesforceContaOrigem, fields["contaOrigem"])
}

func TestHandleSalesforcePushJob_PatchThenCreateOn404(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.CitizenCollection == "" {
		config.AppConfig.CitizenCollection = "citizens"
	}
	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	var patchCalled, createCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			patchCalled = true
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":[{"code":"NAO_ENCONTRADO","message":"not found"}]}`))
		case http.MethodPost:
			createCalled = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"created","accountId":"001abc"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	worker.SetSalesforceClient(clients.NewSalesforceClient(srv.URL, time.Second, clients.StaticBearerToken("jwt")))

	nome := "Joao"
	_, err := worker.mongo.Collection(config.AppConfig.CitizenCollection).InsertOne(context.Background(), models.Citizen{
		CPF:  "14202478754",
		Nome: &nome,
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:          "job-1",
		Type:        SalesforcePushQueue,
		Key:         "14202478754",
		Collection:  SalesforcePushQueue,
		BearerToken: "jwt",
		Data:        SalesforcePushPayload{CPF: "14202478754", BearerToken: "jwt"},
	}
	err = worker.handleSalesforcePushJob(context.Background(), job)
	require.NoError(t, err)
	assert.True(t, patchCalled)
	assert.True(t, createCalled)
}

func TestHandleSalesforcePushJob_PatchConflictIsSuccess(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.CitizenCollection == "" {
		config.AppConfig.CitizenCollection = "citizens"
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"errors":[{"code":"CONFLICT","message":"já no status"}]}`))
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"cpf":"14202478754","accountId":"001already"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	worker.SetSalesforceClient(clients.NewSalesforceClient(srv.URL, time.Second, clients.StaticBearerToken("jwt")))
	nome := "Joao"
	_, err := worker.mongo.Collection(config.AppConfig.CitizenCollection).InsertOne(context.Background(), models.Citizen{
		CPF:  "14202478754",
		Nome: &nome,
	})
	require.NoError(t, err)

	err = worker.handleSalesforcePushJob(context.Background(), &SyncJob{
		Type:        SalesforcePushQueue,
		Key:         "14202478754",
		BearerToken: "jwt",
		Data:        SalesforcePushPayload{CPF: "14202478754", BearerToken: "jwt"},
	})
	require.NoError(t, err)
}

func TestEnqueueSalesforceConsentimentoMirror(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	require.NoError(t, EnqueueSalesforceConsentimentoMirror(context.Background(), nil, "11144477735", &clients.SalesforceConsentimentoPatchRequest{
		Categoria: "PREF_Lembrete_Pagamento",
		Acao:      "optin",
	}))

	require.NoError(t, EnqueueSalesforceConsentimentoMirror(context.Background(), worker.redis, "11144477735", &clients.SalesforceConsentimentoPatchRequest{
		Categoria: "PREF_Lembrete_Pagamento",
		Acao:      "optin",
	}))
	n, err := worker.redis.LLen(context.Background(), "sync:queue:"+SalesforceSyncQueue).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

func TestHandleSalesforceSyncJob_AppliesSelfDeclared(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}
	if config.AppConfig.UserConfigCollection == "" {
		config.AppConfig.UserConfigCollection = "user_config"
	}

	cpf := "14202478754"
	dados, err := json.Marshal(map[string]interface{}{
		"accountId": "001x",
		"email":     "sf@test.com",
		"genero":    "Homem_cisgenero",
		"raca":      "Parda",
		"consentimento": []clients.SalesforceConsentimento{
			{Categoria: "Comunicacao", Status: "IN"},
		},
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-sync",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  dados,
		},
	}

	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	require.NotNil(t, sd.Email)
	require.NotNil(t, sd.Email.Principal)
	require.NotNil(t, sd.Email.Principal.Valor)
	assert.Equal(t, "sf@test.com", *sd.Email.Principal.Valor)
	require.NotNil(t, sd.Genero)
	assert.Equal(t, "Homem cisgênero", *sd.Genero)
	require.NotNil(t, sd.Raca)
	assert.Equal(t, "parda", *sd.Raca)
}

func TestHandleSalesforceSyncJob_DoesNotWriteCanonicalCitizen(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.CitizenCollection == "" {
		config.AppConfig.CitizenCollection = "citizens"
	}
	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "14202478754"
	job := &SyncJob{
		ID:         "job-sf-citizen",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"nome":"João Silva","nomeSocial":"João","dataNascimento":"1990-05-15","nacionalidade":"Brasil"}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var citizen models.Citizen
	err := worker.mongo.Collection(config.AppConfig.CitizenCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&citizen)
	require.Error(t, err)
	assert.True(t, errors.Is(err, mongo.ErrNoDocuments))

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	require.NotNil(t, sd.Nacionalidade)
	assert.Equal(t, "Brasil", *sd.Nacionalidade)
}

func TestHandleSalesforceSyncJob_DeltaNullClearsEmail(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}
	if config.AppConfig.UserConfigCollection == "" {
		config.AppConfig.UserConfigCollection = "user_config"
	}

	cpf := "14202478754"
	oldEmail := "old@test.com"
	_, err := worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).InsertOne(context.Background(), models.SelfDeclaredData{
		CPF: cpf,
		Email: &models.Email{
			Principal: &models.EmailPrincipal{Valor: &oldEmail},
		},
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-null-email",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAnonimizacao,
			Dados:  json.RawMessage(`{"nomeExibicao":"ANONIMIZADO","email":null}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	assert.Nil(t, sd.Email)
	require.NotNil(t, sd.NomeExibicao)
	assert.Equal(t, "ANONIMIZADO", *sd.NomeExibicao)
	assert.True(t, sd.SalesforceAnonymized)
}

func TestHandleSalesforceSyncJob_DeltaEmptyStringKeepsEmailField(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "14202478754"
	oldEmail := "old@test.com"
	_, err := worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).InsertOne(context.Background(), models.SelfDeclaredData{
		CPF: cpf,
		Email: &models.Email{
			Principal: &models.EmailPrincipal{Valor: &oldEmail},
		},
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-empty-email",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"email":""}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	require.NotNil(t, sd.Email)
	require.NotNil(t, sd.Email.Principal)
	require.NotNil(t, sd.Email.Principal.Valor)
	assert.Equal(t, "", *sd.Email.Principal.Valor)
}

func TestHandleSalesforceSyncJob_SkipsStaleUpdatedAt(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "14202478754"
	newer := time.Date(2026, 8, 27, 18, 0, 0, 0, time.UTC)
	genero := "Homem cisgênero"
	_, err := worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).InsertOne(context.Background(), models.SelfDeclaredData{
		CPF:                 cpf,
		SalesforceUpdatedAt: &newer,
		Genero:              &genero,
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-stale",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:       cpf,
			Evento:    SalesforceWebhookEventAtualizacao,
			UpdatedAt: "2026-08-27T17:00:00Z",
			Dados:     json.RawMessage(`{"genero":"Mulher_cisgenero"}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	require.NotNil(t, sd.Genero)
	assert.Equal(t, "Homem cisgênero", *sd.Genero)
}

func TestHandleSalesforceSyncJob_AppliesConsentimentoToUserConfig(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}
	if config.AppConfig.UserConfigCollection == "" {
		config.AppConfig.UserConfigCollection = "user_config"
	}

	cpf := "14202478754"
	dados := json.RawMessage(`{"consentimento":[{"categoria":"Comunicacao","status":"OUT","motivo":"nao desejo"}]}`)

	job := &SyncJob{
		ID:         "job-sf-consent",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  dados,
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var uc models.UserConfig
	require.NoError(t, worker.mongo.Collection(config.AppConfig.UserConfigCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&uc))
	require.NotNil(t, uc.SalesforceConsentimentos)
	entry := uc.SalesforceConsentimentos["Comunicacao"]
	assert.Equal(t, "Comunicacao", entry.Categoria)
	assert.Equal(t, "OUT", entry.Status)
	assert.False(t, entry.OptIn)
	assert.Empty(t, uc.CategoryOptIns)
	assert.False(t, uc.OptIn)
}

func TestMaybeEnqueueSalesforcePush_SkipsWhenOriginSalesforce(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	config.AppConfig.SalesforceBaseURL = "https://sf.example.com"
	defer func() { config.AppConfig.SalesforceBaseURL = "" }()

	worker.maybeEnqueueSalesforcePush(&SyncJob{
		Type:        "self_declared_email",
		Key:         "14202478754",
		Origin:      SyncOriginSalesforce,
		BearerToken: "jwt",
	})

	n, err := worker.redis.LLen(context.Background(), "sync:queue:"+SalesforcePushQueue).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestMaybeEnqueueSalesforcePush_Enqueues(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	config.AppConfig.SalesforceBaseURL = "https://sf.example.com"
	defer func() { config.AppConfig.SalesforceBaseURL = "" }()

	worker.maybeEnqueueSalesforcePush(&SyncJob{
		ID:          "j1",
		Type:        "self_declared_email",
		Key:         "14202478754",
		BearerToken: "user-jwt",
	})

	n, err := worker.redis.LLen(context.Background(), "sync:queue:"+SalesforcePushQueue).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	raw, err := worker.redis.RPop(context.Background(), "sync:queue:"+SalesforcePushQueue).Result()
	require.NoError(t, err)
	var job SyncJob
	require.NoError(t, json.Unmarshal([]byte(raw), &job))
	assert.Equal(t, SalesforcePushQueue, job.Type)
	assert.Equal(t, "14202478754", job.Key)
	assert.True(t, strings.HasPrefix(job.BearerToken, bearerSealPrefix))
	assert.NotContains(t, raw, "user-jwt")
	opened, err := bearerFromQueue(job.BearerToken, job.Type, job.Key)
	require.NoError(t, err)
	assert.Equal(t, "user-jwt", opened)
	payloadBytes, err := json.Marshal(job.Data)
	require.NoError(t, err)
	var payload SalesforcePushPayload
	require.NoError(t, json.Unmarshal(payloadBytes, &payload))
	assert.Equal(t, "14202478754", payload.CPF)
	assert.Empty(t, payload.BearerToken)
	assert.NotContains(t, string(payloadBytes), "user-jwt")
}

func TestHandleSalesforcePushJob_OpensSealedBearer(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cpf":"14202478754","accountId":"001abc"}`))
	}))
	defer srv.Close()
	worker.SetSalesforceClient(clients.NewSalesforceClient(srv.URL, time.Second, clients.StaticBearerToken("ignored")))

	sealed, _, err := prepareBearerForQueue("real-user-jwt", SalesforcePushQueue, "14202478754")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(sealed, bearerSealPrefix))

	err = worker.handleSalesforcePushJob(context.Background(), &SyncJob{
		Type:        SalesforcePushQueue,
		Key:         "14202478754",
		BearerToken: sealed,
		Data:        SalesforcePushPayload{CPF: "14202478754"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer real-user-jwt", gotAuth)
}

func TestHandleSalesforcePushJob_RequiresBearer(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()
	worker.SetSalesforceClient(nil)

	err := worker.handleSalesforcePushJob(context.Background(), &SyncJob{
		Type: SalesforcePushQueue,
		Key:  "14202478754",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bearer token")
}

func testJWTWithExp(exp int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp)))
	return header + "." + payload + ".sig"
}

func TestSalesforceBearerExpired(t *testing.T) {
	assert.False(t, salesforceBearerExpired("jwt"))
	assert.False(t, salesforceBearerExpired("a.b"))
	assert.False(t, salesforceBearerExpired(testJWTWithExp(time.Now().Add(time.Hour).Unix())))
	assert.True(t, salesforceBearerExpired(testJWTWithExp(1)))
}

func TestHandleSalesforcePushJob_ExpiredBearerIsNonRetryable(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()
	worker.SetSalesforceClient(clients.NewSalesforceClient("https://sf.example.com", time.Second, clients.StaticBearerToken("jwt")))

	err := worker.handleSalesforcePushJob(context.Background(), &SyncJob{
		Type:        SalesforcePushQueue,
		Key:         "14202478754",
		BearerToken: testJWTWithExp(1),
		Data:        SalesforcePushPayload{CPF: "14202478754"},
	})
	require.Error(t, err)
	assert.True(t, isNonRetryableSyncError(err))
	assert.Contains(t, err.Error(), "expired")
}

func TestHandleSalesforcePushJob_UnauthorizedIsNonRetryable(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"code":"UNAUTHORIZED","message":"invalid token"}]}`))
	}))
	defer srv.Close()
	worker.SetSalesforceClient(clients.NewSalesforceClient(srv.URL, time.Second, clients.StaticBearerToken("jwt")))

	err := worker.handleSalesforcePushJob(context.Background(), &SyncJob{
		Type:        SalesforcePushQueue,
		Key:         "14202478754",
		BearerToken: "jwt",
		Data:        SalesforcePushPayload{CPF: "14202478754"},
	})
	require.Error(t, err)
	assert.True(t, isNonRetryableSyncError(err))
}

func TestHandleSyncFailure_NonRetryableGoesToDLQ(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	job := &SyncJob{
		ID:          "job-nr",
		Type:        SalesforcePushQueue,
		Key:         "14202478754",
		BearerToken: "user-jwt",
		Data:        SalesforcePushPayload{CPF: "14202478754", BearerToken: "legacy-jwt"},
		RetryCount:  0,
		MaxRetries:  5,
	}
	worker.handleSyncFailure(job, newNonRetryableSyncError(fmt.Errorf("salesforce patch failed: unauthorized")))

	n, err := worker.redis.LLen(context.Background(), "sync:queue:"+SalesforcePushQueue).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	raw, err := worker.redis.LRange(context.Background(), "sync:dlq:"+SalesforcePushQueue, 0, 0).Result()
	require.NoError(t, err)
	require.Len(t, raw, 1)
	assert.NotContains(t, raw[0], "user-jwt")
	assert.NotContains(t, raw[0], "legacy-jwt")
	var dlq DLQJob
	require.NoError(t, json.Unmarshal([]byte(raw[0]), &dlq))
	assert.Empty(t, dlq.OriginalJob.BearerToken)
}

func TestMaybeEnqueueSalesforcePush_SkipsWithoutBearer(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	config.AppConfig.SalesforceBaseURL = "https://sf.example.com"
	defer func() { config.AppConfig.SalesforceBaseURL = "" }()

	worker.maybeEnqueueSalesforcePush(&SyncJob{
		Type: "self_declared_email",
		Key:  "14202478754",
	})

	n, err := worker.redis.LLen(context.Background(), "sync:queue:"+SalesforcePushQueue).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestHandleSalesforceSyncJob_ConsentimentoMergePreservesExistingCategories(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.UserConfigCollection == "" {
		config.AppConfig.UserConfigCollection = "user_config"
	}

	cpf := "11144477735"
	_, err := worker.mongo.Collection(config.AppConfig.UserConfigCollection).InsertOne(context.Background(), models.UserConfig{
		CPF:   cpf,
		OptIn: true,
		CategoryOptIns: map[string]bool{
			"Comunicacao": true,
			"Marketing":   true,
		},
		SalesforceConsentimentos: map[string]models.SalesforceConsentimentoEntry{
			"Comunicacao": {Categoria: "Comunicacao", Status: "IN", OptIn: true},
			"Marketing":   {Categoria: "Marketing", Status: "IN", OptIn: true},
		},
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-consent-merge",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"consentimento":[{"categoria":"Comunicacao","status":"OUT","motivo":"nao desejo"}]}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var uc models.UserConfig
	require.NoError(t, worker.mongo.Collection(config.AppConfig.UserConfigCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&uc))
	assert.True(t, uc.OptIn)
	require.NotNil(t, uc.CategoryOptIns)
	assert.True(t, uc.CategoryOptIns["Comunicacao"])
	assert.True(t, uc.CategoryOptIns["Marketing"])
	require.NotNil(t, uc.SalesforceConsentimentos)
	assert.False(t, uc.SalesforceConsentimentos["Comunicacao"].OptIn)
	assert.Equal(t, "OUT", uc.SalesforceConsentimentos["Comunicacao"].Status)
	assert.True(t, uc.SalesforceConsentimentos["Marketing"].OptIn)
}

func TestHandleSalesforceSyncJob_ConsentimentoSameCategoriaDifferentCanal(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.UserConfigCollection == "" {
		config.AppConfig.UserConfigCollection = "user_config"
	}

	cpf := "11144477735"
	_, err := worker.mongo.Collection(config.AppConfig.UserConfigCollection).InsertOne(context.Background(), models.UserConfig{
		CPF:   cpf,
		OptIn: true,
		CategoryOptIns: map[string]bool{
			"PREF_X|Portal Pref.Rio": true,
		},
		SalesforceConsentimentos: map[string]models.SalesforceConsentimentoEntry{
			"PREF_X|Portal Pref.Rio": {
				Categoria: "PREF_X",
				Canal:     "Portal Pref.Rio",
				Status:    "IN",
				OptIn:     true,
			},
		},
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-consent-canal",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"consentimento":[{"categoria":"PREF_X","status":"IN","canal":"WhatsApp","finalidade":"lembrete","data":"2026-08-27T10:00:00Z"}]}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var uc models.UserConfig
	require.NoError(t, worker.mongo.Collection(config.AppConfig.UserConfigCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&uc))
	assert.True(t, uc.OptIn)
	require.NotNil(t, uc.CategoryOptIns)
	assert.True(t, uc.CategoryOptIns["PREF_X|Portal Pref.Rio"])
	_, hasRMIWhatsApp := uc.CategoryOptIns["PREF_X|WhatsApp"]
	assert.False(t, hasRMIWhatsApp)
	require.NotNil(t, uc.SalesforceConsentimentos)
	entry := uc.SalesforceConsentimentos["PREF_X|WhatsApp"]
	assert.Equal(t, "PREF_X", entry.Categoria)
	assert.Equal(t, "WhatsApp", entry.Canal)
	assert.Equal(t, "lembrete", entry.Finalidade)
	assert.Equal(t, "2026-08-27T10:00:00Z", entry.Data)
	assert.True(t, entry.OptIn)
	portal := uc.SalesforceConsentimentos["PREF_X|Portal Pref.Rio"]
	assert.True(t, portal.OptIn)
}

func TestHandleSalesforceSyncJob_ConsentimentoNullClearsAll(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.UserConfigCollection == "" {
		config.AppConfig.UserConfigCollection = "user_config"
	}

	cpf := "11144477735"
	_, err := worker.mongo.Collection(config.AppConfig.UserConfigCollection).InsertOne(context.Background(), models.UserConfig{
		CPF:   cpf,
		OptIn: true,
		CategoryOptIns: map[string]bool{
			"Comunicacao": true,
			"Marketing":   true,
		},
		SalesforceConsentimentos: map[string]models.SalesforceConsentimentoEntry{
			"Comunicacao": {Categoria: "Comunicacao", OptIn: true},
		},
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-consent-null",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"consentimento":null}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var uc models.UserConfig
	require.NoError(t, worker.mongo.Collection(config.AppConfig.UserConfigCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&uc))
	assert.True(t, uc.OptIn)
	require.NotNil(t, uc.CategoryOptIns)
	assert.True(t, uc.CategoryOptIns["Comunicacao"])
	assert.True(t, uc.CategoryOptIns["Marketing"])
	assert.Empty(t, uc.SalesforceConsentimentos)
}

func TestHandleSalesforceSyncJob_InvalidConsentimentoJSON(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}
	if config.AppConfig.UserConfigCollection == "" {
		config.AppConfig.UserConfigCollection = "user_config"
	}

	cpf := "14202478754"
	job := &SyncJob{
		ID:         "job-sf-consent-bad",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"consentimento":"not-an-array"}`),
		},
	}
	err := worker.handleSalesforceSyncJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "consentimento")
}

func TestHandleSalesforceSyncJob_RetriesAfterPartialConsentimentoFailure(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}
	if config.AppConfig.UserConfigCollection == "" {
		config.AppConfig.UserConfigCollection = "user_config"
	}

	cpf := "14202478754"
	updatedAt := "2026-08-27T17:00:00Z"
	first := &SyncJob{
		ID:         "job-sf-partial-1",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:       cpf,
			Evento:    SalesforceWebhookEventAtualizacao,
			UpdatedAt: updatedAt,
			Dados:     json.RawMessage(`{"email":"sf@test.com","consentimento":"not-an-array"}`),
		},
	}
	err := worker.handleSalesforceSyncJob(context.Background(), first)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "consentimento")

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	require.NotNil(t, sd.Email)
	require.NotNil(t, sd.Email.Principal)
	require.NotNil(t, sd.Email.Principal.Valor)
	assert.Equal(t, "sf@test.com", *sd.Email.Principal.Valor)
	assert.Nil(t, sd.SalesforceUpdatedAt)

	retry := &SyncJob{
		ID:         "job-sf-partial-2",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:       cpf,
			Evento:    SalesforceWebhookEventAtualizacao,
			UpdatedAt: updatedAt,
			Dados:     json.RawMessage(`{"email":"sf@test.com","consentimento":[{"categoria":"Comunicacao","status":"OUT","motivo":"nao desejo"}]}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), retry))

	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	require.NotNil(t, sd.SalesforceUpdatedAt)
	assert.Equal(t, updatedAt, sd.SalesforceUpdatedAt.UTC().Format(time.RFC3339))

	var uc models.UserConfig
	require.NoError(t, worker.mongo.Collection(config.AppConfig.UserConfigCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&uc))
	require.NotNil(t, uc.SalesforceConsentimentos)
	assert.False(t, uc.SalesforceConsentimentos["Comunicacao"].OptIn)
	assert.Empty(t, uc.CategoryOptIns)
}

func TestHandleSalesforceSyncJob_InvalidUpdatedAt(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "14202478754"
	job := &SyncJob{
		ID:         "job-sf-bad-updated-at",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:       cpf,
			Evento:    SalesforceWebhookEventAtualizacao,
			UpdatedAt: "not-a-date",
			Dados:     json.RawMessage(`{"genero":"Homem_cisgenero"}`),
		},
	}
	err := worker.handleSalesforceSyncJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "updatedAt")
}

func TestHandleSalesforceSyncJob_EmptyDadosObjectIsNoOp(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "14202478754"
	job := &SyncJob{
		ID:         "job-sf-empty-dados",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	count, err := worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		CountDocuments(context.Background(), bson.M{"cpf": cpf})
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestHandleSalesforceSyncJob_InvalidPayloadWhitespaceDados(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	job := &SyncJob{
		ID:         "job-sf-whitespace-dados",
		Type:       SalesforceSyncQueue,
		Key:        "14202478754",
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    "14202478754",
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`   `),
		},
	}
	err := worker.handleSalesforceSyncJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid salesforce_sync")
}

func TestHandleSalesforceSyncJob_InvalidPayloadInvalidEvento(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	job := &SyncJob{
		ID:         "job-sf-bad-evento",
		Type:       SalesforceSyncQueue,
		Key:        "14202478754",
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    "14202478754",
			Evento: "invalido",
			Dados:  json.RawMessage(`{"email":"a@test.com"}`),
		},
	}
	err := worker.handleSalesforceSyncJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evento")
}

func TestHandleSalesforceSyncJob_InvalidPayloadMissingCPF(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	job := &SyncJob{
		ID:         "job-sf-no-cpf",
		Type:       SalesforceSyncQueue,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"email":"a@test.com"}`),
		},
	}
	err := worker.handleSalesforceSyncJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid salesforce_sync payload")
}

func TestHandleSalesforceSyncJob_EnderecoPartialMerge(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "52998224725"
	logradouro := "Rua Antiga"
	cidade := "Niterói"
	estado := "RJ"
	cep := "24000000"
	_, err := worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).InsertOne(context.Background(), models.SelfDeclaredData{
		CPF: cpf,
		Endereco: &models.Endereco{
			Principal: &models.EnderecoPrincipal{
				Logradouro: &logradouro,
				Municipio:  &cidade,
				Estado:     &estado,
				CEP:        &cep,
			},
		},
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-endereco-partial",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"endereco":{"cidade":"Rio de Janeiro"}}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	require.NotNil(t, sd.Endereco)
	require.NotNil(t, sd.Endereco.Principal)
	assert.Equal(t, "Rua Antiga", *sd.Endereco.Principal.Logradouro)
	assert.Equal(t, "Rio de Janeiro", *sd.Endereco.Principal.Municipio)
	assert.Equal(t, "RJ", *sd.Endereco.Principal.Estado)
	assert.Equal(t, "24000000", *sd.Endereco.Principal.CEP)
}

func TestHandleSalesforceSyncJob_TelefoneNullClears(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "52998224725"
	valor := "988888888"
	ddd := "21"
	ddi := "55"
	_, err := worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).InsertOne(context.Background(), models.SelfDeclaredData{
		CPF: cpf,
		Telefone: &models.Telefone{
			Principal: &models.TelefonePrincipal{DDI: &ddi, DDD: &ddd, Valor: &valor},
		},
	})
	require.NoError(t, err)

	job := &SyncJob{
		ID:         "job-sf-tel-null",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"telefone1":null}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	assert.Nil(t, sd.Telefone)
}

func TestHandleSalesforceSyncJob_UpsertWhenNoExistingDocument(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "11144477735"
	job := &SyncJob{
		ID:         "job-sf-upsert",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF:    cpf,
			Evento: SalesforceWebhookEventAtualizacao,
			Dados:  json.RawMessage(`{"genero":"Mulher_cisgenero"}`),
		},
	}
	require.NoError(t, worker.handleSalesforceSyncJob(context.Background(), job))

	var sd models.SelfDeclaredData
	require.NoError(t, worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).
		FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd))
	require.NotNil(t, sd.Genero)
	assert.Equal(t, "Mulher cisgênero", *sd.Genero)
}
