package clients

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSalesforcePatch_MarshalPartialOmitsEmpty(t *testing.T) {
	raw, err := json.Marshal(NewSalesforcePatch().Put("contaOrigem", "Portal Pref.Rio"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"contaOrigem":"Portal Pref.Rio"}`, string(raw))
}

func TestSalesforcePatch_MarshalClearSendsNull(t *testing.T) {
	p := NewSalesforceSnapshotPatch()
	p.PutClear("email")
	p.Put("contaOrigem", "Portal Pref.Rio")
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	assert.JSONEq(t, `{"email":null,"contaOrigem":"Portal Pref.Rio"}`, string(raw))
}

func TestSalesforcePatch_MarshalIdiomaArray(t *testing.T) {
	p := NewSalesforceSnapshotPatch()
	p.Put("idioma", []string{"Portugues_Brasil"})
	raw, err := json.Marshal(p)
	require.NoError(t, err)
	assert.JSONEq(t, `{"idioma":["Portugues_Brasil"]}`, string(raw))
}

func TestNormalizePatchPhones(t *testing.T) {
	fields := map[string]any{
		"telefone1": "21988888888",
		"telefone2": "5521977776666",
	}
	normalizePatchPhones(fields)
	assert.Equal(t, "5521988888888", fields["telefone1"])
	assert.Equal(t, "5521977776666", fields["telefone2"])
}
