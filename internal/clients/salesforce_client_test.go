package clients

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSalesforceClient_Configured(t *testing.T) {
	assert.False(t, (*SalesforceClient)(nil).Configured())
	assert.False(t, NewSalesforceClient("", time.Second, StaticBearerToken("t")).Configured())
	assert.False(t, NewSalesforceClient("https://x", time.Second, nil).Configured())
	assert.True(t, NewSalesforceClient("https://x/", time.Second, StaticBearerToken("t")).Configured())
}

func TestSalesforceClient_CreateOrUpdateCidadao_Success(t *testing.T) {
	var gotAuth string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/private/cidadao", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		gotAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &gotBody))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":"created","accountId":"001be00000ZPNrIAAX"}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("test-jwt"))
	resp, err := client.CreateOrUpdateCidadao(context.Background(), &SalesforceCidadaoCreateRequest{
		CPF:         "14202478754",
		Nome:        "Maria Silva",
		Email:       "maria@test.com",
		Telefone1:   "5521988888888",
		Genero:      "Mulher_cisgenero",
		Raca:        "Parda",
		Idioma:      "Portugues_Brasil",
		ContaOrigem: "Portal Pref.Rio",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-jwt", gotAuth)
	assert.Equal(t, "14202478754", gotBody["cpf"])
	assert.Equal(t, "created", resp.Status)
	assert.Equal(t, "001be00000ZPNrIAAX", resp.AccountID)
}

func TestSalesforceClient_CreateOrUpdateCidadao_StringBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`"001be00000ZPNrIAAX"`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	resp, err := client.CreateOrUpdateCidadao(context.Background(), &SalesforceCidadaoCreateRequest{
		CPF:  "14202478754",
		Nome: "Maria",
	})
	require.NoError(t, err)
	assert.Equal(t, "created", resp.Status)
	assert.Equal(t, "001be00000ZPNrIAAX", resp.AccountID)
}

func TestDecodeSalesforceCreateResponse(t *testing.T) {
	out, err := decodeSalesforceCreateResponse([]byte(`{"status":"created","accountId":"001abc"}`))
	require.NoError(t, err)
	assert.Equal(t, "001abc", out.AccountID)

	out, err = decodeSalesforceCreateResponse([]byte(`"created"`))
	require.NoError(t, err)
	assert.Equal(t, "created", out.Status)
	assert.Empty(t, out.AccountID)

	out, err = decodeSalesforceCreateResponse(nil)
	require.NoError(t, err)
	assert.Equal(t, "created", out.Status)
}

func TestSalesforceClient_GetCidadao_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/private/cidadao/14202478754", r.URL.Path)
		assert.Equal(t, "Bearer keycloak-jwt", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"accountId":"001be00000XqUOzAAN",
			"cpf":"14202478754",
			"nome":"Maria",
			"email":"maria@test.com",
			"telefone1":"5521988888888",
			"genero":"Homem_cisgenero",
			"idioma":["Portugues_Brasil"],
			"isTourist":false,
			"endereco":{"cidade":"Rio de Janeiro","estado":"RJ"}
		}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL+"/", time.Second, StaticBearerToken("keycloak-jwt"))
	got, err := client.GetCidadao(context.Background(), "14202478754")
	require.NoError(t, err)
	assert.Equal(t, "001be00000XqUOzAAN", got.AccountID)
	assert.Equal(t, "maria@test.com", got.Email)
	require.NotNil(t, got.Endereco)
	assert.Equal(t, "Rio de Janeiro", got.Endereco.Cidade)
	assert.Equal(t, []string{"Portugues_Brasil"}, got.Idioma)
}

func TestDecodeSalesforceCidadaoResponse_HomologExportShape(t *testing.T) {
	got, err := decodeSalesforceCidadaoResponse([]byte(`{
		"relacionados":{"ordensServico":[],"ligacoes":[],"protocolos":[]},
		"consentimento":[
			{"canais":[],"descricao":"Informacoes da cidade","codigo":"PREF_Info_Cidade"}
		],
		"dadosPessoais":{
			"email":"andrelopesbr1999@gmail.com",
			"sobrenome":"Nome",
			"primeiroNome":"Meu"
		},
		"dataExportacao":"2026-08-25T12:01:17Z",
		"cpf":"02075979600"
	}`))
	require.NoError(t, err)
	assert.Equal(t, "02075979600", got.CPF)
	assert.Equal(t, "andrelopesbr1999@gmail.com", got.Email)
	assert.Equal(t, "Meu Nome", got.Nome)
	require.Len(t, got.Consentimento, 1)
	assert.Equal(t, "PREF_Info_Cidade", got.Consentimento[0].Categoria)
	assert.Equal(t, "", got.Consentimento[0].Acao)
}

func TestSalesforceClient_WithBearerToken_ForwardsJWT(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer inbound-jwt", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"001","cpf":"14202478754"}`))
	}))
	defer srv.Close()

	base := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("service-jwt"))
	got, err := base.WithBearerToken("inbound-jwt").GetCidadao(context.Background(), "14202478754")
	require.NoError(t, err)
	assert.Equal(t, "001", got.AccountID)
}

func TestSalesforceClient_PatchCidadao_Success(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "Bearer jwt", r.Header.Get("Authorization"))
		assert.Equal(t, "/api/private/cidadao/14202478754", r.URL.Path)
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &gotBody))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"accountId":"001be00000XqUOzAAN",
			"cpf":"14202478754",
			"email":"a@b.com",
			"canalOrigem":"Portal Pref.Rio",
			"canalUltimaModificacao":"Portal Pref.Rio"
		}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	resp, err := client.PatchCidadao(context.Background(), "14202478754", &SalesforceCidadaoPatchRequest{
		Email:        "a@b.com",
		PrimeiroNome: "Joao",
		Cidade:       "Rio de Janeiro",
		Genero:       "Homem_cisgenero",
		ContaOrigem:  "Portal Pref.Rio",
	})
	require.NoError(t, err)
	assert.Equal(t, "Joao", gotBody["primeiroNome"])
	assert.Equal(t, "Portal Pref.Rio", gotBody["contaOrigem"])
	assert.Equal(t, "001be00000XqUOzAAN", resp.AccountID)
	assert.Equal(t, "a@b.com", resp.Email)
	assert.Equal(t, "Portal Pref.Rio", resp.CanalUltimaModificacao)
}

func TestSalesforceClient_PatchCidadao_AckReGets(t *testing.T) {
	gets := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"cpf":"14202478754","camposAtualizados":["email"],"dataAtualizacao":"2026-08-26T12:00:00Z"}`))
		case http.MethodGet:
			gets++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"accountId":"001abc","cpf":"14202478754","nome":"Ana","email":"a@b.com"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	resp, err := client.PatchCidadao(context.Background(), "14202478754", &SalesforceCidadaoPatchRequest{
		Email:       "a@b.com",
		ContaOrigem: "Portal Pref.Rio",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, gets)
	assert.Equal(t, "001abc", resp.AccountID)
	assert.Equal(t, "Ana", resp.Nome)
	assert.Equal(t, "a@b.com", resp.Email)
}

func TestDecodeSalesforceCidadaoResponse_FlatDTO(t *testing.T) {
	got, err := decodeSalesforceCidadaoResponse([]byte(`{
		"accountId":"001be00000d6EBVAA2",
		"cpf":"02075979600",
		"nome":"André Cardoso",
		"email":"andrelopesbr1999@gmail.com",
		"telefone1":"5521988888888",
		"idioma":["Portugues_Brasil"],
		"consentimento":null,
		"canalOrigem":"Portal Pref.Rio",
		"canalUltimaModificacao":"Portal Pref.Rio"
	}`))
	require.NoError(t, err)
	assert.Equal(t, "001be00000d6EBVAA2", got.AccountID)
	assert.Equal(t, "André Cardoso", got.Nome)
	assert.Equal(t, "andrelopesbr1999@gmail.com", got.Email)
	assert.Equal(t, []string{"Portugues_Brasil"}, got.Idioma)
	assert.Nil(t, got.Consentimento)
}

func TestNormalizeSalesforcePhone(t *testing.T) {
	assert.Equal(t, "", NormalizeSalesforcePhone(""))
	assert.Equal(t, "5521988888888", NormalizeSalesforcePhone("21988888888"))
	assert.Equal(t, "5521988888888", NormalizeSalesforcePhone("5521988888888"))
	assert.Equal(t, "5521988888888", NormalizeSalesforcePhone("+55 21 98888-8888"))
}

func TestSalesforceClient_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"DADOS_INVALIDOS","message":"O campo Telefone 1 é obrigatório."}]}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	_, err := client.GetCidadao(context.Background(), "14202478754")
	require.Error(t, err)
	var apiErr *SalesforceAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Len(t, apiErr.Errors, 1)
	assert.Equal(t, "DADOS_INVALIDOS", apiErr.Errors[0].Code)
	assert.False(t, apiErr.IsNotFound())
}

func TestSalesforceClient_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"code":"NAO_ENCONTRADO","message":"not found"}]}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	_, err := client.PatchCidadao(context.Background(), "14202478754", &SalesforceCidadaoPatchRequest{Email: "a@b.com"})
	require.Error(t, err)
	var apiErr *SalesforceAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.True(t, apiErr.IsNotFound())
}

func TestSalesforceClient_RequiresConfig(t *testing.T) {
	_, err := NewSalesforceClient("", time.Second, StaticBearerToken("jwt")).GetCidadao(context.Background(), "14202478754")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base URL")

	_, err = NewSalesforceClient("https://sf.example.com", time.Second, nil).GetCidadao(context.Background(), "14202478754")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token source")

	_, err = NewSalesforceClient("https://sf.example.com", time.Second, StaticBearerToken("jwt")).CreateOrUpdateCidadao(context.Background(), nil)
	require.Error(t, err)
}

func TestSalesforceClient_PatchCidadaoConsentimento_Success(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/private/cidadao/14202478754/consentimento", r.URL.Path)
		assert.Equal(t, "Bearer user-jwt", r.Header.Get("Authorization"))
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &gotBody))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("user-jwt"))
	err := client.PatchCidadaoConsentimento(context.Background(), "14202478754", &SalesforceConsentimentoPatchRequest{
		Categoria: "PREF_Lembrete_Pagamento",
		Acao:      "optout",
		Motivo:    "Too_Many_Messages",
		Origem:    "Portal Pref.Rio",
	})
	require.NoError(t, err)
	assert.Equal(t, "optout", gotBody["acao"])
	assert.Equal(t, "Too_Many_Messages", gotBody["motivo"])
}

func TestSalesforceClient_PatchCidadaoConsentimento_RequiresMotivoOnOptout(t *testing.T) {
	client := NewSalesforceClient("https://sf.example.com", time.Second, StaticBearerToken("jwt"))
	err := client.PatchCidadaoConsentimento(context.Background(), "14202478754", &SalesforceConsentimentoPatchRequest{
		Categoria: "PREF_Lembrete_Pagamento",
		Acao:      "optout",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "motivo")
}

func TestSalesforceClient_ExportarCidadao_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/private/cidadao/52998224725/exportar", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"cpf":"52998224725",
			"dataExportacao":"2026-07-01T19:31:18Z",
			"dadosPessoais":{"primeiroNome":"TESTE","email":"cidadao@exemplo.com"},
			"relacionados":{"protocolos":[],"ligacoes":[],"ordensServico":[]}
		}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	got, err := client.ExportarCidadao(context.Background(), "52998224725")
	require.NoError(t, err)
	assert.Equal(t, "52998224725", got.CPF)
	assert.Equal(t, "TESTE", got.DadosPessoais["primeiroNome"])
}

func TestSalesforceClient_AnonimizarCidadao_Accepted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/private/cidadao/52998224725/anonimizar", r.URL.Path)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{
			"numeroSolicitacao":"PRIVRTBF-00000002",
			"cpf":"52998224725",
			"status":"enfileirado",
			"dataSolicitacao":"2026-07-01T19:38:17Z"
		}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	got, err := client.AnonimizarCidadao(context.Background(), "52998224725")
	require.NoError(t, err)
	assert.Equal(t, "PRIVRTBF-00000002", got.NumeroSolicitacao)
	assert.Equal(t, "enfileirado", got.Status)
}

func TestSalesforceClient_AnonimizarCidadao_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{
			"numeroSolicitacao":"PRIVRTBF-00000002",
			"cpf":"52998224725",
			"status":"enfileirado",
			"dataSolicitacao":"2026-07-01T19:38:17Z"
		}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	got, err := client.AnonimizarCidadao(context.Background(), "52998224725")
	require.Error(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "PRIVRTBF-00000002", got.NumeroSolicitacao)
	var apiErr *SalesforceAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.True(t, apiErr.IsConflict())
}

func TestSalesforceClient_GetAnonimizacao_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/private/anonimizacao/PRIVRTBF-00000002", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"numeroSolicitacao":"PRIVRTBF-00000002",
			"status":"concluido",
			"dataSolicitacao":"2026-07-01T19:38:17Z",
			"dataConclusao":"2026-07-01T19:40:05Z"
		}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	got, err := client.GetAnonimizacao(context.Background(), "PRIVRTBF-00000002")
	require.NoError(t, err)
	assert.Equal(t, "concluido", got.Status)
}

func TestSalesforceClient_ListChamados_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/private/chamados", r.URL.Path)
		assert.Equal(t, "Bearer user-jwt", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"protocolos":[{"protocolo":"RIO-2026-00011171","status":"Em andamento","ordens_de_servico":[]}]}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("user-jwt"))
	got, err := client.ListChamados(context.Background())
	require.NoError(t, err)
	require.Len(t, got.Protocolos, 1)
	assert.Equal(t, "RIO-2026-00011171", got.Protocolos[0].Protocolo)
}

func TestSalesforceClient_GetChamado_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/private/chamados/RIO-2026-00011171", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"protocolo":"RIO-2026-00011171","status":"Em andamento","ordens_de_servico":[{"codigoOs":"00000109","isClosed":false,"isOverdue":false}]}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	got, err := client.GetChamado(context.Background(), "RIO-2026-00011171")
	require.NoError(t, err)
	assert.Equal(t, "RIO-2026-00011171", got.Protocolo)
	require.Len(t, got.OrdensDeServico, 1)
	assert.Equal(t, "00000109", got.OrdensDeServico[0].CodigoOs)
}

func TestSalesforceClient_PrivacyAndChamados_ValidationErrors(t *testing.T) {
	client := NewSalesforceClient("https://sf.example.com", time.Second, StaticBearerToken("jwt"))

	err := client.PatchCidadaoConsentimento(context.Background(), "", &SalesforceConsentimentoPatchRequest{Categoria: "x", Acao: "optin"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cpf")

	err = client.PatchCidadaoConsentimento(context.Background(), "14202478754", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")

	err = client.PatchCidadaoConsentimento(context.Background(), "14202478754", &SalesforceConsentimentoPatchRequest{Acao: "optin"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "categoria")

	err = client.PatchCidadaoConsentimento(context.Background(), "14202478754", &SalesforceConsentimentoPatchRequest{Categoria: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "acao")

	_, err = client.ExportarCidadao(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cpf")

	_, err = client.AnonimizarCidadao(context.Background(), "  ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cpf")

	_, err = client.GetAnonimizacao(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "numeroSolicitacao")

	_, err = client.GetChamado(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protocolo")
}

func TestSalesforceClient_PrivacyAndChamados_UpstreamErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"bad_request", http.StatusBadRequest, `{"errors":[{"code":"DADOS_INVALIDOS","message":"campo inválido"}]}`},
		{"unauthorized", http.StatusUnauthorized, `{"errors":[{"code":"NAO_AUTORIZADO","message":"token inválido"}]}`},
		{"not_found", http.StatusNotFound, `{"errors":[{"code":"NAO_ENCONTRADO","message":"não encontrado"}]}`},
		{"server", http.StatusInternalServerError, `{"errors":[{"code":"ERRO_INTERNO","message":"falha"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))

			err := client.PatchCidadaoConsentimento(context.Background(), "14202478754", &SalesforceConsentimentoPatchRequest{
				Categoria: "PREF_Lembrete_Pagamento",
				Acao:      "optin",
				Origem:    "Portal Pref.Rio",
			})
			require.Error(t, err)
			var apiErr *SalesforceAPIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.status, apiErr.StatusCode)
			require.NotEmpty(t, apiErr.Errors)

			_, err = client.ExportarCidadao(context.Background(), "14202478754")
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.status, apiErr.StatusCode)

			_, err = client.GetAnonimizacao(context.Background(), "PRIV-1")
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.status, apiErr.StatusCode)

			_, err = client.ListChamados(context.Background())
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.status, apiErr.StatusCode)

			_, err = client.GetChamado(context.Background(), "RIO-1")
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.status, apiErr.StatusCode)

			if tc.status != http.StatusConflict {
				_, err = client.AnonimizarCidadao(context.Background(), "14202478754")
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, tc.status, apiErr.StatusCode)
			}
		})
	}
}

func TestSalesforceClient_AnonimizarCidadao_Upstream400WithoutStatusBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"DADOS_INVALIDOS","message":"cpf inválido"}]}`))
	}))
	defer srv.Close()

	client := NewSalesforceClient(srv.URL, time.Second, StaticBearerToken("jwt"))
	got, err := client.AnonimizarCidadao(context.Background(), "14202478754")
	require.Error(t, err)
	assert.Nil(t, got)
	var apiErr *SalesforceAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	assert.False(t, apiErr.IsConflict())
}

func TestSalesforceAPIError_IsConflict(t *testing.T) {
	assert.False(t, (*SalesforceAPIError)(nil).IsConflict())
	assert.False(t, (&SalesforceAPIError{StatusCode: 404}).IsConflict())
	assert.True(t, (&SalesforceAPIError{StatusCode: 409}).IsConflict())
}
