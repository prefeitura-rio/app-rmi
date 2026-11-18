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
		ProtocoloLinks:            []MaintenanceRequestProtocoloLink{{CodigoProtocolo: "RIO-12345678-9", CPF: testOwnerCPF}},
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
		Numero:                    "100",
		Complemento:               "casa",
		CEP:                       "20000000",
		Andamentos: []MaintenanceRequestAndamento{
			{IDAndamento: "1", Data: "2025-01-10T10:05:00", Tipo: "Abertura", Descricao: "Chamado registrado", NomeUnidadeOrganizacional: "RIOLUZ", FlPublico: true},
			{IDAndamento: "2", Data: "2025-01-11T08:00:00", Tipo: "Interno", Descricao: "nota interna", FlPublico: false},
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
			if got.Bairro != "Centro" || got.LoadedAt != "2025-11-03T03:00:00Z" || len(got.Protocolos) != 1 || got.Protocolos[0] != "RIO-12345678-9" {
				t.Fatalf("public view misses public fields: %+v", got)
			}
			if len(got.Andamentos) != 1 || got.Andamentos[0].Descricao != "Chamado registrado" {
				t.Fatalf("andamentos = %+v, want only the fl_publico entry", got.Andamentos)
			}

			body, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			for _, leaked := range []string{testOwnerCPF, `"cpf"`, "logradouro", "Rua Exemplo", "texto livre", "nota interna", "endereco", `"numero"`, "complemento", "20000000", "id_andamento", "fl_publico"} {
				if strings.Contains(string(body), leaked) {
					t.Fatalf("public JSON contains %q: %s", leaked, body)
				}
			}
		})
	}
}

func TestToPublicMaintenanceRequest_OwnerGetsStreetAndDescription(t *testing.T) {
	doc := sampleProtocolDocument()
	doc.CPF = "" // ownership only via protocolos[].cpf
	got := doc.ToPublicMaintenanceRequest(testOwnerCPF)

	if !got.Titular || got.Logradouro != "Rua Exemplo, 100" || got.Descricao != "texto livre de quem abriu o chamado" {
		t.Fatalf("owner view = %+v, want titular with logradouro and descricao", got)
	}
	body, _ := json.Marshal(got)
	if strings.Contains(string(body), testOwnerCPF) || strings.Contains(string(body), `"numero"`) || strings.Contains(string(body), "complemento") {
		t.Fatalf("owner JSON leaks CPF/numero/complemento: %s", body)
	}
}

func TestToPublicMaintenanceRequest_DocumentWithoutCPFNeverOwned(t *testing.T) {
	doc := sampleProtocolDocument()
	doc.CPF = ""
	doc.ProtocoloLinks = []MaintenanceRequestProtocoloLink{{CodigoProtocolo: "RIO-12345678-9"}}
	if got := doc.ToPublicMaintenanceRequest(testOwnerCPF); got.Titular || got.Descricao != "" {
		t.Fatalf("document without CPF matched caller: %+v", got)
	}
}

func TestToPublicMaintenanceRequest_EmptyAndamentosIsEmptySlice(t *testing.T) {
	doc := sampleProtocolDocument()
	doc.Andamentos = nil
	got := doc.ToPublicMaintenanceRequest("")
	if got.Andamentos == nil || len(got.Andamentos) != 0 {
		t.Fatalf("andamentos = %#v, want empty non-nil slice", got.Andamentos)
	}
}

func TestMaintenanceRequestProtocolDocument_DecodesBigQueryShape(t *testing.T) {
	loadedAt := time.Date(2025, 11, 3, 3, 0, 0, 0, time.UTC)
	raw, err := bson.Marshal(bson.M{
		"id_chamado":                              int64(1234567),
		"descricao_chamado":                       "texto livre de quem abriu o chamado",
		"data_inicio":                             time.Date(2025, 1, 10, 10, 0, 0, 0, time.UTC),
		"data_prevista_atendimento":               time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		"unidade_organizacional_responsavel_atual": "RIOLUZ",
		"bairro":                                  "Centro",
		"logradouro":                              "Rua Exemplo",
		"numero":                                  "10",
		"complemento":                             "ap 1",
		"cep":                                     "20000000",
		"loaded_at":                               loadedAt,
		"protocolos": bson.A{
			bson.M{"codigo_protocolo": "RIO-12345678-9", "cpf": testOwnerCPF},
			bson.M{"codigo_protocolo": "RIO-87654321-0", "cpf": testOwnerCPF},
		},
		"andamentos": bson.A{
			bson.M{"id_andamento": int64(1), "data": "2025-01-10T10:05:00", "descricao": "int32", "fl_publico": true, "nome_unidade_organizacional": "RIOLUZ"},
			bson.M{"id_andamento": int32(2), "data": loadedAt, "descricao": "datetime", "fl_publico": true},
			bson.M{"descricao": "privado", "fl_publico": false},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var doc MaintenanceRequestProtocolDocument
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.IDChamado != "1234567" {
		t.Fatalf("id_chamado = %q", doc.IDChamado)
	}
	if doc.Descricao != "texto livre de quem abriu o chamado" {
		t.Fatalf("descricao_chamado mapped = %q", doc.Descricao)
	}
	if doc.NomeUnidadeOrganizacional != "RIOLUZ" {
		t.Fatalf("unidade mapped = %q", doc.NomeUnidadeOrganizacional)
	}
	if len(doc.ProtocoloLinks) != 2 || doc.ProtocoloLinks[0].CodigoProtocolo != "RIO-12345678-9" {
		t.Fatalf("protocolos = %+v", doc.ProtocoloLinks)
	}
	if doc.DataAlvoFinalizacao == "" {
		t.Fatal("data_prevista_atendimento should map to DataAlvoFinalizacao")
	}

	got := doc.ToPublicMaintenanceRequest("")
	if len(got.Protocolos) != 2 || len(got.Andamentos) != 2 {
		t.Fatalf("public projection = %+v", got)
	}
	if got.Andamentos[1].Data != "2025-11-03T03:00:00Z" {
		t.Fatalf("andamento datetime = %q", got.Andamentos[1].Data)
	}
	owner := doc.ToPublicMaintenanceRequest(testOwnerCPF)
	if !owner.Titular || owner.Logradouro != "Rua Exemplo" {
		t.Fatalf("owner via protocolos.cpf = %+v", owner)
	}
}

func TestMaintenanceRequestProtocolDocument_DecodesLegacyDocument(t *testing.T) {
	raw, err := bson.Marshal(bson.M{
		"cpf": testOwnerCPF, "id": "1", "protocolo": "RIO-12345678-9",
		"endereco": "Rua Exemplo, 100 - Centro",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc MaintenanceRequestProtocolDocument
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("legacy document without the new fields must decode: %v", err)
	}
	if doc.IDChamado != "1" {
		t.Fatalf("legacy id mapped to id_chamado = %q", doc.IDChamado)
	}
	if len(doc.Protocolos()) != 1 || doc.Protocolos()[0] != "RIO-12345678-9" {
		t.Fatalf("legacy protocolo adapted = %v", doc.Protocolos())
	}
	got := doc.ToPublicMaintenanceRequest("")
	if got.Andamentos == nil || len(got.Andamentos) != 0 || got.Bairro != "" || got.LoadedAt != "" {
		t.Fatalf("legacy projection = %+v", got)
	}
}

func TestConvertToMaintenanceRequest_IncludesProtocolos(t *testing.T) {
	raw, err := bson.Marshal(bson.M{"_id": "x", "cpf": testOwnerCPF, "id": "1", "protocolo": "RIO-12345678-9"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc MaintenanceRequestDocument
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := doc.ConvertToMaintenanceRequest()
	if len(got.Protocolos) != 1 || got.Protocolos[0] != "RIO-12345678-9" {
		t.Fatalf("protocolos = %v, want [RIO-12345678-9]", got.Protocolos)
	}

	legacy := MaintenanceRequestDocument{ID: "y", CPF: testOwnerCPF, ProtocoloLinks: nil}
	body, _ := json.Marshal(legacy.ConvertToMaintenanceRequest())
	if strings.Contains(string(body), `"protocolo":`) {
		t.Fatalf("response must not use scalar protocolo: %s", body)
	}
	if !strings.Contains(string(body), `"protocolos":[]`) {
		t.Fatalf("empty protocolos must serialize as []: %s", body)
	}
}

func TestConvertToMaintenanceRequest_SGRCDocument(t *testing.T) {
	raw, err := bson.Marshal(bson.M{
		"_id": "z", "id_chamado": int64(42),
		"descricao_chamado": "desc",
		"protocolos": bson.A{
			bson.M{"codigo_protocolo": "RIO-11111111-1", "cpf": testOwnerCPF},
		},
		"bairro": "Copacabana", "logradouro": "Av Atlantica", "numero": "100",
		"data_prevista_atendimento": "2025-01-15T10:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc MaintenanceRequestDocument
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := doc.ConvertToMaintenanceRequest()
	if got.IDChamado != "42" || got.Bairro != "Copacabana" || got.Numero != "100" || got.CPF != testOwnerCPF {
		t.Fatalf("sgrc conversion = %+v", got)
	}
	if len(got.Protocolos) != 1 || got.Protocolos[0] != "RIO-11111111-1" {
		t.Fatalf("protocolos = %v", got.Protocolos)
	}
	if got.Descricao == nil || *got.Descricao != "desc" {
		t.Fatalf("descricao_chamado = %v", got.Descricao)
	}
}
