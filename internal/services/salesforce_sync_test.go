package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
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

func TestBuildSalesforcePatch(t *testing.T) {
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
	}

	patch := buildSalesforcePatch(citizen, sd)
	assert.Equal(t, "maria@test.com", patch.Email)
	assert.Equal(t, "5521988888888", patch.Telefone1)
	assert.Equal(t, "Homem_cisgenero", patch.Genero)
	assert.Equal(t, "Parda", patch.Raca)
	assert.Equal(t, "Maria", patch.PrimeiroNome)
	assert.Equal(t, "Portugues_Brasil", patch.Idioma)
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

func TestHandleSalesforceSyncJob_AppliesSelfDeclared(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	if config.AppConfig.SelfDeclaredCollection == "" {
		config.AppConfig.SelfDeclaredCollection = "self_declared"
	}

	cpf := "14202478754"
	job := &SyncJob{
		ID:         "job-sf-sync",
		Type:       SalesforceSyncQueue,
		Key:        cpf,
		Collection: SalesforceSyncQueue,
		Origin:     SyncOriginSalesforce,
		Data: SalesforceSyncPayload{
			CPF: cpf,
			Cidadao: &clients.SalesforceCidadao{
				AccountID: "001x",
				CPF:       cpf,
				Email:     "sf@test.com",
				Genero:    "Homem_cisgenero",
				Raca:      "Parda",
			},
		},
	}

	err := worker.handleSalesforceSyncJob(context.Background(), job)
	require.NoError(t, err)

	var sd models.SelfDeclaredData
	err = worker.mongo.Collection(config.AppConfig.SelfDeclaredCollection).FindOne(context.Background(), bson.M{"cpf": cpf}).Decode(&sd)
	require.NoError(t, err)
	require.NotNil(t, sd.Email)
	require.NotNil(t, sd.Email.Principal)
	require.NotNil(t, sd.Email.Principal.Valor)
	assert.Equal(t, "sf@test.com", *sd.Email.Principal.Valor)
	require.NotNil(t, sd.Genero)
	assert.Equal(t, "Homem cisgênero", *sd.Genero)
	require.NotNil(t, sd.Raca)
	assert.Equal(t, "parda", *sd.Raca)
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
	assert.Equal(t, "user-jwt", job.BearerToken)
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
