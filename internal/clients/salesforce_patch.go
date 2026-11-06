package clients

import (
	"encoding/json"
	"strings"
)

// SalesforceCidadaoPatchRequest is the PATCH .../api/private/cidadao/{cpf} body.
// Omitted keys are left unchanged on Salesforce. JSON null clears a field.
type SalesforceCidadaoPatchRequest struct {
	fields map[string]any
}

// NewSalesforcePatch builds a partial PATCH body (omits unset keys).
func NewSalesforcePatch() *SalesforceCidadaoPatchRequest {
	return &SalesforceCidadaoPatchRequest{fields: make(map[string]any)}
}

// NewSalesforceSnapshotPatch is an alias of NewSalesforcePatch (partial body).
func NewSalesforceSnapshotPatch() *SalesforceCidadaoPatchRequest {
	return NewSalesforcePatch()
}

func (p *SalesforceCidadaoPatchRequest) PutIfNonempty(key, value string) *SalesforceCidadaoPatchRequest {
	if strings.TrimSpace(value) != "" {
		p.Put(key, value)
	}
	return p
}

func (p *SalesforceCidadaoPatchRequest) Put(key string, value any) *SalesforceCidadaoPatchRequest {
	if p == nil {
		return p
	}
	if p.fields == nil {
		p.fields = make(map[string]any)
	}
	p.fields[key] = value
	return p
}

func (p *SalesforceCidadaoPatchRequest) PutClear(key string) *SalesforceCidadaoPatchRequest {
	p.Put(key, nil)
	return p
}

func (p *SalesforceCidadaoPatchRequest) PutEndereco(addr *SalesforceEndereco) {
	if addr == nil {
		p.PutClear("endereco")
		return
	}
	p.Put("endereco", addr)
}

// MarshalJSON implements json.Marshaler.
func (p *SalesforceCidadaoPatchRequest) MarshalJSON() ([]byte, error) {
	if p == nil || len(p.fields) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(p.fields)
}

// Fields returns a copy of the PATCH body map (for tests).
func (p *SalesforceCidadaoPatchRequest) Fields() map[string]any {
	if p == nil || len(p.fields) == 0 {
		return nil
	}
	out := make(map[string]any, len(p.fields))
	for k, v := range p.fields {
		out[k] = v
	}
	return out
}

// normalizePatchPhones normalizes telefone fields present in the PATCH body.
func normalizePatchPhones(fields map[string]any) {
	if fields == nil {
		return
	}
	for _, key := range []string{"telefone1", "telefone2", "telefone3", "telefonePrincipal"} {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		if raw == nil {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		if phone := NormalizeSalesforcePhone(s); phone != "" {
			fields[key] = phone
		}
	}
}
