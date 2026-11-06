package services

import (
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Every Salesforce picklist API value must survive SF -> RMI label -> SF, so an inbound
// webhook followed by an outbound push never changes or drops the citizen's answer.
func TestSalesforcePicklists_RoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		toRMI    func(string) string
		toSF     func(string) string
		sfValues []string
	}{
		{
			name:  "genero",
			toRMI: mapSalesforceGeneroToRMI,
			toSF:  mapRMIGeneroToSalesforce,
			sfValues: []string{"Homem_cisgenero", "Mulher_cisgenero", "Homem_transgenero", "Mulher_transgenero",
				"Nao_binario", "Outro_Prefere_nao_informar"},
		},
		{
			name:  "escolaridade",
			toRMI: mapSalesforceEscolaridadeToRMI,
			toSF:  mapRMIEscolaridadeToSalesforce,
			sfValues: []string{"Fundamental incompleto", "Fundamental completo", "Medio incompleto", "Medio completo",
				"Superior incompleto", "Superior completo", "Pos Graduacao", "Mestrado", "Doutorado"},
		},
		{
			name:  "renda familiar",
			toRMI: mapSalesforceRendaFamiliarToRMI,
			toSF:  mapRMIRendaFamiliarToSalesforce,
			sfValues: []string{"Menos de 1 salario minimo", "1 a 2 salarios minimos", "2 a 3 salarios minimos",
				"3 a 5 salarios minimos", "Mais de 5 salarios minimos"},
		},
		{
			name:  "deficiencia",
			toRMI: mapSalesforceDeficienciaToRMI,
			toSF:  mapRMIDeficienciaToSalesforce,
			sfValues: []string{"Nao sou pessoa com deficiencia", "Fisica", "Auditiva", "Visual",
				"Transtorno do Espectro Autista", "Intelectual", "Mental (psicossocial)", "Reabilitado do INSS"},
		},
		{
			name:     "raca",
			toRMI:    mapSalesforceRacaToRMI,
			toSF:     mapRMIRacaToSalesforce,
			sfValues: []string{"Branca", "Preta", "Parda", "Amarela", "Indigena", "Outra"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, sf := range tc.sfValues {
				label := tc.toRMI(sf)
				assert.NotEmpty(t, label, "SF value %q has no RMI label", sf)
				assert.Equal(t, sf, tc.toSF(label), "round trip of %q via %q", sf, label)
				assert.Equal(t, sf, tc.toSF(sf), "SF value %q is accepted as is", sf)
				assert.Equal(t, sf, tc.toSF("  "+sf+"  "), "surrounding spaces are ignored")
			}
			assert.Empty(t, tc.toSF(""), "empty value is not sent")
			assert.Empty(t, tc.toSF("   "))
		})
	}
}

func TestSalesforcePicklists_UnknownValues(t *testing.T) {
	assert.Empty(t, mapRMIGeneroToSalesforce("Valor livre"))
	assert.Empty(t, mapSalesforceGeneroToRMI("Valor_desconhecido"))
	assert.Equal(t, "Prefiro não informar", mapSalesforceGeneroToRMI("Prefiro_nao_informar"), "legacy SF value")
	assert.Equal(t, "Outro_Prefere_nao_informar", mapRMIGeneroToSalesforce("Outro"))

	assert.Empty(t, mapRMIEscolaridadeToSalesforce("Ensino tecnico"))
	assert.Empty(t, mapSalesforceEscolaridadeToRMI("Ensino tecnico"))
	assert.Empty(t, mapRMIRendaFamiliarToSalesforce("Muito"))
	assert.Empty(t, mapSalesforceRendaFamiliarToRMI("Muito"))
	assert.Empty(t, mapRMIDeficienciaToSalesforce("Outra"))
	assert.Empty(t, mapSalesforceDeficienciaToRMI("Outra"))

	// RMI labels that already arrive in their final form are kept on the inbound side.
	assert.Equal(t, "Médio completo", mapSalesforceEscolaridadeToRMI("Médio completo"))
	assert.Equal(t, "Menos de 1 salário mínimo", mapSalesforceRendaFamiliarToRMI("Menos de 1 salário mínimo"))
	assert.Equal(t, "Física", mapSalesforceDeficienciaToRMI("Física"))

	// Raca: accents and case are normalized both ways; other values are capitalized.
	assert.Equal(t, "Indigena", mapRMIRacaToSalesforce("Indígena"))
	assert.Equal(t, "indigena", mapSalesforceRacaToRMI("Indígena"))
	assert.Empty(t, mapSalesforceRacaToRMI("  "))
	assert.Equal(t, "Quilombola", mapRMIRacaToSalesforce("quilombola"))
}

func TestSalesforceBearerExpired_ExpClaim(t *testing.T) {
	seg := func(payload string) string { return base64.RawURLEncoding.EncodeToString([]byte(payload)) }
	past := fmt.Sprintf(`{"exp":%d}`, time.Now().Add(-time.Minute).Unix())
	future := fmt.Sprintf(`{"exp":%d}`, time.Now().Add(time.Hour).Unix())

	assert.True(t, salesforceBearerExpired("h."+seg(past)+".s"))
	assert.False(t, salesforceBearerExpired("h."+seg(future)+".s"))
	assert.False(t, salesforceBearerExpired("h."+seg(`{"sub":"x"}`)+".s"), "no exp claim")
	assert.False(t, salesforceBearerExpired("h."+seg(`not json`)+".s"))
	assert.False(t, salesforceBearerExpired("h.%%%.s"), "undecodable payload")
	assert.False(t, salesforceBearerExpired("opaque-token"))
	// Padded base64url payloads are also accepted.
	assert.True(t, salesforceBearerExpired("h."+base64.URLEncoding.EncodeToString([]byte(past))+".s"))
}

func TestNonRetryableSyncError(t *testing.T) {
	cause := errors.New("salesforce 401")
	err := newNonRetryableSyncError(cause)
	assert.True(t, isNonRetryableSyncError(err))
	assert.True(t, isNonRetryableSyncError(fmt.Errorf("wrapped: %w", err)))
	assert.ErrorIs(t, err, cause, "the cause stays reachable")
	assert.Contains(t, err.Error(), "salesforce 401")
	assert.Nil(t, newNonRetryableSyncError(nil))
	assert.False(t, isNonRetryableSyncError(cause))
}

func TestFirstNameAndDeref(t *testing.T) {
	assert.Equal(t, "Maria", firstName("  Maria da Silva "))
	assert.Empty(t, firstName("   "))
	v := "x"
	assert.Equal(t, "x", deref(&v))
	assert.Empty(t, deref(nil))
}
