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
)

func TestSyncSalesforceOnLogin_Match(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"001","cpf":"52998224725","email":"a@test.com","nome":"Ana"}`))
	}))
	defer srv.Close()

	sf := clients.NewSalesforceClient(srv.URL, time.Second, clients.StaticBearerToken("jwt"))
	result, err := SyncSalesforceOnLogin(context.Background(), sf, &models.JWTClaims{
		PreferredUsername: "52998224725",
		Name:              "Ana",
		Email:             "a@test.com",
	})
	require.NoError(t, err)
	assert.Equal(t, "matched", result.Action)
	assert.Equal(t, "001", result.Cidadao.AccountID)
}

func TestSyncSalesforceOnLogin_SilentEmailUpdate(t *testing.T) {
	var patchBody map[string]interface{}
	gets := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets++
			w.WriteHeader(http.StatusOK)
			if gets == 1 {
				_, _ = w.Write([]byte(`{"accountId":"001","cpf":"52998224725","email":"old@test.com","nome":"Ana"}`))
			} else {
				_, _ = w.Write([]byte(`{"accountId":"001","cpf":"52998224725","email":"new@test.com","nome":"Ana","canalUltimaModificacao":"Portal Pref.Rio"}`))
			}
		case http.MethodPatch:
			require.NoError(t, json.NewDecoder(r.Body).Decode(&patchBody))
			// Homolog-style body: not a full citizen DTO.
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"camposAtualizados":["email"]}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	prev := config.AppConfig
	config.AppConfig = &config.Config{}
	defer func() { config.AppConfig = prev }()

	sf := clients.NewSalesforceClient(srv.URL, time.Second, clients.StaticBearerToken("jwt"))
	result, err := SyncSalesforceOnLogin(context.Background(), sf, &models.JWTClaims{
		PreferredUsername: "52998224725",
		Email:             "new@test.com",
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", result.Action)
	assert.Equal(t, "new@test.com", patchBody["email"])
	assert.Equal(t, SalesforceContaOrigem, patchBody["contaOrigem"])
	assert.Equal(t, "001", result.Cidadao.AccountID)
	assert.Equal(t, "new@test.com", result.Cidadao.Email)
	assert.Equal(t, "Ana", result.Cidadao.Nome)
	assert.Equal(t, 2, gets)
}

func TestSyncSalesforceOnLogin_CreateOn404(t *testing.T) {
	var createBody map[string]interface{}
	gets := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets++
			if gets == 1 {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"errors":[{"code":"NOT_FOUND","message":"Cidadão não encontrado para o CPF informado."}]}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accountId":"001new","cpf":"52998224725","nome":"Maria Silva","email":"maria@test.com","canalOrigem":"Portal Pref.Rio","canalUltimaModificacao":"Portal Pref.Rio"}`))
		case http.MethodPost:
			require.NoError(t, json.NewDecoder(r.Body).Decode(&createBody))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"created","accountId":"001new"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	sf := clients.NewSalesforceClient(srv.URL, time.Second, clients.StaticBearerToken("jwt"))
	result, err := SyncSalesforceOnLogin(context.Background(), sf, &models.JWTClaims{
		PreferredUsername: "52998224725",
		Name:              "Maria Silva",
		Email:             "maria@test.com",
	})
	require.NoError(t, err)
	assert.Equal(t, "created", result.Action)
	assert.Equal(t, "52998224725", createBody["cpf"])
	assert.Equal(t, "Maria Silva", createBody["nome"])
	_, hasTelefone := createBody["telefone1"]
	assert.False(t, hasTelefone, "telefone1 must not come from JWT phone_number")
	assert.Equal(t, SalesforceContaOrigem, createBody["contaOrigem"])
	assert.Equal(t, "001new", result.Cidadao.AccountID)
	assert.Equal(t, "Maria Silva", result.Cidadao.Nome)
	assert.Equal(t, "maria@test.com", result.Cidadao.Email)
	assert.Equal(t, 2, gets)
}
