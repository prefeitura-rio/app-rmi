package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSelfDeclaredDeltaPatch_EmailEmptyString(t *testing.T) {
	set, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"email": json.RawMessage(`""`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	require.Contains(t, set, "email")
	require.NotContains(t, unset, "email")
	email, ok := set["email"].(*models.Email)
	require.True(t, ok)
	require.NotNil(t, email.Principal)
	require.NotNil(t, email.Principal.Valor)
	assert.Equal(t, "", *email.Principal.Valor)
}

func TestBuildSelfDeclaredDeltaPatch_EmailNullClears(t *testing.T) {
	_, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"email": json.RawMessage(`null`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	assert.Equal(t, "", unset["email"])
}

func TestBuildSelfDeclaredDeltaPatch_AbsentFieldNoOp(t *testing.T) {
	set, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"genero": json.RawMessage(`"Homem_cisgenero"`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	assert.NotContains(t, set, "email")
	assert.NotContains(t, unset, "email")
	require.Contains(t, set, "genero")
}

func TestParseSalesforceUpdatedAt(t *testing.T) {
	ts, err := parseSalesforceUpdatedAt("2026-08-27T17:00:00Z")
	require.NoError(t, err)
	require.NotNil(t, ts)

	empty, err := parseSalesforceUpdatedAt("")
	require.NoError(t, err)
	assert.Nil(t, empty)

	_, err = parseSalesforceUpdatedAt("not-a-date")
	require.Error(t, err)
}

func TestIsStaleSalesforceUpdatedAt(t *testing.T) {
	old := time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 8, 27, 17, 0, 0, 0, time.UTC)

	assert.False(t, isStaleSalesforceUpdatedAt(nil, &newer))
	assert.False(t, isStaleSalesforceUpdatedAt(&old, nil))
	assert.True(t, isStaleSalesforceUpdatedAt(&newer, &old))
	assert.False(t, isStaleSalesforceUpdatedAt(&old, &newer))
	assert.True(t, isStaleSalesforceUpdatedAt(&old, &old))
}

func TestBuildSelfDeclaredDeltaPatch_AnonimizacaoFlags(t *testing.T) {
	set, _ := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"nomeExibicao": json.RawMessage(`"ANONIMIZADO"`),
	}, SalesforceWebhookEventAnonimizacao, time.Now(), nil)

	assert.Equal(t, true, set["salesforce_anonymized"])
	assert.NotNil(t, set["salesforce_anonymized_at"])
}

func TestBuildSelfDeclaredDeltaPatch_ConsentimentoMergeFields(t *testing.T) {
	raw, err := json.Marshal([]clients.SalesforceConsentimento{
		{Categoria: "Comunicacao", Status: "OUT", Motivo: "nao quero"},
	})
	require.NoError(t, err)

	fields := map[string]json.RawMessage{"consentimento": raw}
	assert.True(t, fieldPresent(fields, "consentimento"))
}

func TestMapSalesforceCidadaoToSelfDeclared_FromDelta(t *testing.T) {
	raw, err := json.Marshal(map[string]string{
		"email":        "maria@test.com",
		"telefone1":    "5521988888888",
		"genero":       "Mulher_cisgenero",
		"raca":         "Parda",
		"nomeExibicao": "Maria",
	})
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))

	set, _ := buildSelfDeclaredDeltaPatch(nil, fields, SalesforceWebhookEventAtualizacao, time.Now(), nil)
	assert.Contains(t, set, "email")
	assert.Contains(t, set, "telefone")
	assert.Contains(t, set, "genero")
	assert.Contains(t, set, "raca")
	assert.Contains(t, set, "nome_exibicao")

	// legacy helper wraps delta builder
	legacy := mapSalesforceCidadaoToSelfDeclared(&clients.SalesforceCidadao{
		Email:        "maria@test.com",
		Telefone1:    "5521988888888",
		Genero:       "Mulher_cisgenero",
		Raca:         "Parda",
		NomeExibicao: "Maria",
	})
	assert.Contains(t, legacy, "email")
	assert.Contains(t, legacy, "genero")
}

func TestBuildSelfDeclaredDeltaPatch_OnlyPresentFieldsInBson(t *testing.T) {
	set, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"escolaridade": json.RawMessage(`""`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	esc, ok := set["escolaridade"].(*string)
	require.True(t, ok)
	assert.Equal(t, "", *esc)
	assert.Empty(t, unset)
}

func TestBuildSelfDeclaredDeltaPatch_TelefoneEmptyString(t *testing.T) {
	set, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"telefone1": json.RawMessage(`""`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	require.Contains(t, set, "telefone")
	require.NotContains(t, unset, "telefone")
	tel, ok := set["telefone"].(*models.Telefone)
	require.True(t, ok)
	require.NotNil(t, tel.Principal)
	require.NotNil(t, tel.Principal.Valor)
	assert.Equal(t, "", *tel.Principal.Valor)
}

func TestBuildSelfDeclaredDeltaPatch_TelefoneNullClears(t *testing.T) {
	_, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"telefonePrincipal": json.RawMessage(`null`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	assert.Equal(t, "", unset["telefone"])
}

func TestBuildSelfDeclaredDeltaPatch_EnderecoNullClears(t *testing.T) {
	_, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"endereco": json.RawMessage(`null`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	assert.Equal(t, "", unset["endereco"])
}

func TestBuildSelfDeclaredDeltaPatch_EnderecoPartialMerge(t *testing.T) {
	logradouro := "Rua A"
	cidade := "Niterói"
	estado := "RJ"
	cep := "24000000"
	existing := &models.SelfDeclaredData{
		Endereco: &models.Endereco{
			Principal: &models.EnderecoPrincipal{
				Logradouro: &logradouro,
				Municipio:  &cidade,
				Estado:     &estado,
				CEP:        &cep,
			},
		},
	}

	set, unset := buildSelfDeclaredDeltaPatch(existing, map[string]json.RawMessage{
		"endereco": json.RawMessage(`{"cidade":"Rio de Janeiro"}`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	require.Contains(t, set, "endereco")
	require.Empty(t, unset)
	addr, ok := set["endereco"].(*models.Endereco)
	require.True(t, ok)
	require.NotNil(t, addr.Principal)
	assert.Equal(t, "Rua A", *addr.Principal.Logradouro)
	assert.Equal(t, "Rio de Janeiro", *addr.Principal.Municipio)
	assert.Equal(t, "RJ", *addr.Principal.Estado)
	assert.Equal(t, "24000000", *addr.Principal.CEP)
}

func TestBuildSelfDeclaredDeltaPatch_EnderecoSubfieldEmptyString(t *testing.T) {
	logradouro := "Rua A"
	cidade := "Niterói"
	estado := "RJ"
	cep := "24000000"
	existing := &models.SelfDeclaredData{
		Endereco: &models.Endereco{
			Principal: &models.EnderecoPrincipal{
				Logradouro: &logradouro,
				Municipio:  &cidade,
				Estado:     &estado,
				CEP:        &cep,
			},
		},
	}

	set, _ := buildSelfDeclaredDeltaPatch(existing, map[string]json.RawMessage{
		"endereco": json.RawMessage(`{"logradouro":""}`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	addr := set["endereco"].(*models.Endereco)
	assert.Equal(t, "", *addr.Principal.Logradouro)
	assert.Equal(t, "Niterói", *addr.Principal.Municipio)
}

func TestBuildSelfDeclaredDeltaPatch_EnderecoSubfieldNullClearsPart(t *testing.T) {
	logradouro := "Rua A"
	cidade := "Niterói"
	estado := "RJ"
	cep := "24000000"
	existing := &models.SelfDeclaredData{
		Endereco: &models.Endereco{
			Principal: &models.EnderecoPrincipal{
				Logradouro: &logradouro,
				Municipio:  &cidade,
				Estado:     &estado,
				CEP:        &cep,
			},
		},
	}

	set, _ := buildSelfDeclaredDeltaPatch(existing, map[string]json.RawMessage{
		"endereco": json.RawMessage(`{"cep":null}`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	addr := set["endereco"].(*models.Endereco)
	assert.Equal(t, "Rua A", *addr.Principal.Logradouro)
	assert.Equal(t, "", *addr.Principal.CEP)
}

func TestBuildSelfDeclaredDeltaPatch_AccountIdEmptyString(t *testing.T) {
	set, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"accountId": json.RawMessage(`""`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	require.Contains(t, set, "salesforce_account_id")
	assert.Equal(t, "", set["salesforce_account_id"])
	assert.NotContains(t, unset, "salesforce_account_id")
}

func TestBuildSelfDeclaredDeltaPatch_NomeExibicao(t *testing.T) {
	set, _ := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"nomeExibicao": json.RawMessage(`"Maria Social"`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	nome, ok := set["nome_exibicao"].(*string)
	require.True(t, ok)
	assert.Equal(t, "Maria Social", *nome)
}

func TestBuildSelfDeclaredDeltaPatch_DoesNotStampWatermark(t *testing.T) {
	incoming := time.Date(2026, 8, 27, 17, 0, 0, 0, time.UTC)
	set, _ := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"email": json.RawMessage(`"sf@test.com"`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), &incoming)

	assert.NotContains(t, set, "salesforce_updated_at")
	assert.Contains(t, set, "email")
}

func TestSalesforceDeltaHasWork_IgnoresCanonicalCitizenFields(t *testing.T) {
	fields := map[string]json.RawMessage{
		"nome":           json.RawMessage(`"João Silva"`),
		"nomeSocial":     json.RawMessage(`"João"`),
		"dataNascimento": json.RawMessage(`"1990-05-15"`),
	}
	set, unset := buildSelfDeclaredDeltaPatch(nil, fields, SalesforceWebhookEventAtualizacao, time.Now(), nil)
	assert.False(t, salesforceDeltaHasWork(fields, set, unset))
}

func TestSalesforceConsentimentoKey(t *testing.T) {
	assert.Equal(t, "PREF_X", salesforceConsentimentoKey("PREF_X", ""))
	assert.Equal(t, "PREF_X|Portal Pref.Rio", salesforceConsentimentoKey("PREF_X", "Portal Pref.Rio"))
}

func TestBuildSelfDeclaredDeltaPatch_IdiomaAndNacionalidade(t *testing.T) {
	set, unset := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"idioma":        json.RawMessage(`["Portugues_Brasil"]`),
		"nacionalidade": json.RawMessage(`"Brasil"`),
		"isTourist":     json.RawMessage(`false`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	assert.Equal(t, []string{"Portugues_Brasil"}, set["idioma"])
	nac, ok := set["nacionalidade"].(*string)
	require.True(t, ok)
	assert.Equal(t, "Brasil", *nac)
	tourist, ok := set["is_tourist"].(*bool)
	require.True(t, ok)
	assert.False(t, *tourist)
	assert.Empty(t, unset)
}

func TestBuildSelfDeclaredDeltaPatch_Telefone2Alternate(t *testing.T) {
	valor := "988887777"
	ddd := "21"
	ddi := "55"
	existing := &models.SelfDeclaredData{
		Telefone: &models.Telefone{
			Principal: &models.TelefonePrincipal{DDI: &ddi, DDD: &ddd, Valor: &valor},
		},
	}
	set, _ := buildSelfDeclaredDeltaPatch(existing, map[string]json.RawMessage{
		"telefone2": json.RawMessage(`"5521977776666"`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	tel, ok := set["telefone"].(*models.Telefone)
	require.True(t, ok)
	require.Len(t, tel.Alternativo, 1)
	require.NotNil(t, tel.Alternativo[0].Valor)
	assert.Equal(t, "977776666", *tel.Alternativo[0].Valor)
	require.NotNil(t, tel.Principal)
	assert.Equal(t, "988887777", *tel.Principal.Valor)
}

func TestBuildSelfDeclaredDeltaPatch_EnderecoComplementoBairro(t *testing.T) {
	set, _ := buildSelfDeclaredDeltaPatch(nil, map[string]json.RawMessage{
		"endereco": json.RawMessage(`{"complemento":"Apto 1","bairro":"Centro"}`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	addr := set["endereco"].(*models.Endereco)
	require.NotNil(t, addr.Principal)
	assert.Equal(t, "Apto 1", *addr.Principal.Complemento)
	assert.Equal(t, "Centro", *addr.Principal.Bairro)
}

func TestBuildSelfDeclaredDeltaPatch_TopLevelCidade(t *testing.T) {
	existing := &models.SelfDeclaredData{
		Endereco: &models.Endereco{
			Principal: &models.EnderecoPrincipal{
				Logradouro: stringPtr("Rua A"),
				Estado:     stringPtr("RJ"),
			},
		},
	}
	set, _ := buildSelfDeclaredDeltaPatch(existing, map[string]json.RawMessage{
		"cidade": json.RawMessage(`"Niterói"`),
	}, SalesforceWebhookEventAtualizacao, time.Now(), nil)

	addr := set["endereco"].(*models.Endereco)
	require.NotNil(t, addr.Principal)
	assert.Equal(t, "Niterói", *addr.Principal.Municipio)
	assert.Equal(t, "Rua A", *addr.Principal.Logradouro)
}

func TestJSONFields_InvalidJSONReturnsEmpty(t *testing.T) {
	assert.Empty(t, jsonFields([]byte(`not-json`)))
	assert.Empty(t, jsonFields([]byte(`null`)))
	assert.Equal(t, json.RawMessage(`"a"`), jsonFields([]byte(`{"email":"a"}`))["email"])
}

func TestIsUnsafeMongoMapKey(t *testing.T) {
	assert.False(t, isUnsafeMongoMapKey("PREF_Lembrete_Pagamento"))
	assert.False(t, isUnsafeMongoMapKey("PREF_Lembrete_Pagamento|WhatsApp"))
	assert.True(t, isUnsafeMongoMapKey("foo.bar"))
	assert.True(t, isUnsafeMongoMapKey("$set"))
}

func stringPtr(s string) *string { return &s }
