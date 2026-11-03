package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

const (
	testOwnerCPF = "11144477735"
	testOtherCPF = "52998224725"
)

func sampleProtocolDocument() MaintenanceRequestProtocolDocument {
	return MaintenanceRequestProtocolDocument{
		CPF:                       testOwnerCPF,
		IDChamado:                 "1234567",
		Protocolo:                 "RIO-12345678-9",
		Categoria:                 "Serviço",
		Tipo:                      "Iluminação Pública",
		Subtipo:                   "Lâmpada apagada",
		Status:                    "Fechado com solução",
		OrigemOcorrencia:          "Portal",
		NomeUnidadeOrganizacional: "RIOLUZ",
		DataInicio:                "2025-01-10T10:00:00",
		DataFim:                   "2025-01-12T09:00:00",
		Descricao:                 "texto livre de quem abriu o chamado",
		Bairro:                    "Centro",
		Logradouro:                "Rua Exemplo, 100",
		Andamentos: []MaintenanceRequestAndamento{
			{Data: "2025-01-10T10:05:00", Tipo: "Abertura", Descricao: "Chamado registrado", FlPublico: true},
			{Data: "2025-01-11T08:00:00", Tipo: "Interno", Descricao: "nota interna", FlPublico: false},
		},
		LoadedAt: "2025-11-03T03:00:00Z",
	}
}

func TestToPublicMaintenanceRequest_AnonymousAndOtherCPFGetPublicView(t *testing.T) {
	doc := sampleProtocolDocument()
	for name, caller := range map[string]string{"anonymous": "", "other cpf": testOtherCPF} {
		t.Run(name, func(t *testing.T) {
			got := doc.ToPublicMaintenanceRequest(caller)

			if got.Titular || got.Logradouro != "" || got.Descricao != "" {
				t.Fatalf("public view exposes owner data: titular=%v logradouro=%q descricao=%q", got.Titular, got.Logradouro, got.Descricao)
			}
			if got.Bairro != "Centro" || got.LoadedAt != "2025-11-03T03:00:00Z" || got.Protocolo != "RIO-12345678-9" {
				t.Fatalf("public view misses public fields: %+v", got)
			}
			if len(got.Andamentos) != 1 || got.Andamentos[0].Descricao != "Chamado registrado" {
				t.Fatalf("andamentos = %+v, want only the fl_publico = 1 entry", got.Andamentos)
			}

			body, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for _, leaked := range []string{testOwnerCPF, "cpf", "logradouro", "Rua Exemplo", "texto livre", "nota interna", "endereco"} {
				if strings.Contains(string(body), leaked) {
					t.Fatalf("public JSON contains %q: %s", leaked, body)
				}
			}
		})
	}
}

func TestToPublicMaintenanceRequest_OwnerGetsStreetAndDescription(t *testing.T) {
	doc := sampleProtocolDocument()
	got := doc.ToPublicMaintenanceRequest(testOwnerCPF)

	if !got.Titular || got.Logradouro != "Rua Exemplo, 100" || got.Descricao != "texto livre de quem abriu o chamado" {
		t.Fatalf("owner view = %+v, want titular with logradouro and descricao", got)
	}
	if len(got.Andamentos) != 1 {
		t.Fatalf("owner view andamentos = %+v, want only public entries", got.Andamentos)
	}
	body, _ := json.Marshal(got)
	if strings.Contains(string(body), testOwnerCPF) {
		t.Fatalf("owner JSON exposes the CPF: %s", body)
	}
}

func TestToPublicMaintenanceRequest_DocumentWithoutCPFNeverOwned(t *testing.T) {
	doc := sampleProtocolDocument()
	doc.CPF = ""
	if got := doc.ToPublicMaintenanceRequest(""); got.Titular || got.Descricao != "" {
		t.Fatalf("document without CPF matched an empty caller: %+v", got)
	}
}

func TestMaintenanceRequestProtocolDocument_DecodesLoaderShapes(t *testing.T) {
	loadedAt := time.Date(2025, 11, 3, 3, 0, 0, 0, time.UTC)
	raw, err := bson.Marshal(bson.M{
		"cpf":       testOwnerCPF,
		"id":        "1234567",
		"protocolo": "RIO-12345678-9",
		"bairro":    "Centro",
		"loaded_at": loadedAt,
		"andamentos": bson.A{
			bson.M{"data": "2025-01-10T10:05:00", "descricao": "int32", "fl_publico": int32(1)},
			bson.M{"data": loadedAt, "descricao": "int64", "fl_publico": int64(1)},
			bson.M{"descricao": "double", "fl_publico": 1.0},
			bson.M{"descricao": "string", "fl_publico": "1"},
			bson.M{"descricao": "bool", "fl_publico": true},
			bson.M{"descricao": "zero", "fl_publico": int32(0)},
			bson.M{"descricao": "string zero", "fl_publico": "0"},
			bson.M{"descricao": "two", "fl_publico": int32(2)},
			bson.M{"descricao": "null", "fl_publico": nil},
			bson.M{"descricao": "missing"},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc MaintenanceRequestProtocolDocument
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.LoadedAt != "2025-11-03T03:00:00Z" {
		t.Fatalf("loaded_at = %q, want RFC3339 UTC", doc.LoadedAt)
	}

	got := doc.ToPublicMaintenanceRequest("")
	var public []string
	for _, a := range got.Andamentos {
		public = append(public, a.Descricao)
	}
	if strings.Join(public, ",") != "int32,int64,double,string,bool" {
		t.Fatalf("public andamentos = %v, want only entries with fl_publico = 1", public)
	}
	if got.Andamentos[1].Data != "2025-11-03T03:00:00Z" {
		t.Fatalf("andamento datetime = %q, want RFC3339 UTC", got.Andamentos[1].Data)
	}
}

func TestMaintenanceRequestProtocolDocument_DecodesLegacyDocument(t *testing.T) {
	raw, err := bson.Marshal(bson.M{"cpf": testOwnerCPF, "id": "1", "endereco": "Rua Exemplo, 100 - Centro"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc MaintenanceRequestProtocolDocument
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("legacy document without the new fields must decode: %v", err)
	}
	got := doc.ToPublicMaintenanceRequest("")
	if got.Andamentos == nil || len(got.Andamentos) != 0 || got.Bairro != "" || got.LoadedAt != "" {
		t.Fatalf("legacy projection = %+v", got)
	}
}

func TestConvertToMaintenanceRequest_IncludesProtocolo(t *testing.T) {
	raw, err := bson.Marshal(bson.M{"_id": "x", "cpf": testOwnerCPF, "id": "1", "protocolo": "RIO-12345678-9"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc MaintenanceRequestDocument
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := doc.ConvertToMaintenanceRequest(); got.Protocolo != "RIO-12345678-9" {
		t.Fatalf("protocolo = %q, want RIO-12345678-9", got.Protocolo)
	}

	legacy := MaintenanceRequestDocument{ID: "y", CPF: testOwnerCPF}
	body, _ := json.Marshal(legacy.ConvertToMaintenanceRequest())
	if strings.Contains(string(body), "protocolo") {
		t.Fatalf("legacy chamado must omit protocolo: %s", body)
	}
}
