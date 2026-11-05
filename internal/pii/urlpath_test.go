package pii

import "testing"

func TestMaskURLPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: "", want: ""},
		{name: "root", path: "/", want: "/"},
		{name: "health kept", path: "/v1/health", want: "/v1/health"},
		{name: "citizen CPF", path: "/v1/citizen/12345678901", want: "/v1/citizen/***"},
		{name: "citizen CPF with sub-route", path: "/v1/citizen/12345678901/phone/validate", want: "/v1/citizen/***/phone/validate"},
		{name: "formatted CPF", path: "/v1/citizen/123.456.789-01/wallet", want: "/v1/citizen/***/wallet"},
		{name: "memory phone", path: "/v1/memory/5521987654321", want: "/v1/memory/***"},
		{name: "memory phone and name", path: "/v1/memory/5521987654321/preferencias", want: "/v1/memory/***/preferencias"},
		{name: "phone with DDI sign", path: "/v1/phone/+5521987654321/opt-in", want: "/v1/phone/***/opt-in"},
		{name: "e-mail segment", path: "/v1/validate/fulano@example.com", want: "/v1/validate/***"},
		{name: "CPF and object id", path: "/v1/citizen/12345678901/vehicles/65a1b2c3d4e5f60718293a4b", want: "/v1/citizen/***/vehicles/65a1b2c3d4e5f60718293a4b"},
		{name: "short numeric id kept", path: "/v1/admin/beta/groups/1234", want: "/v1/admin/beta/groups/1234"},
		{name: "trailing slash kept", path: "/v1/citizen/12345678901/", want: "/v1/citizen/***/"},
		{name: "CPF with a suffix", path: "/v1/citizen/12345678901x/wallet", want: "/v1/citizen/***/wallet"},
		{name: "CPF with a prefix", path: "/v1/citizen/cpf-123.456.789-01", want: "/v1/citizen/***"},
		{name: "phone in a file name", path: "/v1/memory/5521987654321.json", want: "/v1/memory/***"},
		{name: "uuid kept", path: "/v1/jobs/3f2b7c1e-9a4d-4e2b-8c1a-0d9e8f7a6b5c", want: "/v1/jobs/3f2b7c1e-9a4d-4e2b-8c1a-0d9e8f7a6b5c"},
		{name: "date kept", path: "/v1/report/2026-11", want: "/v1/report/2026-11"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskURLPath(tt.path); got != tt.want {
				t.Errorf("MaskURLPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMaskQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "empty", query: "", want: ""},
		{name: "no personal data", query: "page=2&per_page=10", want: "page=2&per_page=10"},
		{name: "CPF value", query: "cpf=12345678901&page=2", want: "cpf=***&page=2"},
		{name: "phone value with escaped plus", query: "phone=%2B5521987654321", want: "phone=***"},
		{name: "escaped e-mail", query: "email=fulano%40example.com", want: "email=***"},
		{name: "list with a CPF", query: "ids=1,12345678901", want: "ids=***"},
		{name: "bare value", query: "12345678901", want: "***"},
		{name: "bare flag kept", query: "debug", want: "debug"},
		{name: "empty value kept", query: "cpf=", want: "cpf="},
		{name: "invalid escape still checked", query: "cpf=12345678901%zz", want: "cpf=***"},
		{name: "CPF embedded in text", query: "q=cpf12345678901", want: "q=***"},
		{name: "escaped e-mail with invalid escape", query: "email=fulano@example.com%zz", want: "email=***"},
		{name: "object id kept", query: "vehicle=65a1b2c3d4e5f60718293a4b", want: "vehicle=65a1b2c3d4e5f60718293a4b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskQuery(tt.query); got != tt.want {
				t.Errorf("MaskQuery(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}
