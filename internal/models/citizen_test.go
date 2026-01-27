package models

import (
	"testing"
	"time"
)

func TestIsValidEthnicity(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "Valid - branca",
			value: "branca",
			want:  true,
		},
		{
			name:  "Valid - preta",
			value: "preta",
			want:  true,
		},
		{
			name:  "Valid - parda",
			value: "parda",
			want:  true,
		},
		{
			name:  "Valid - amarela",
			value: "amarela",
			want:  true,
		},
		{
			name:  "Valid - indigena",
			value: "indigena",
			want:  true,
		},
		{
			name:  "Valid - outra",
			value: "outra",
			want:  true,
		},
		{
			name:  "Invalid - wrong case",
			value: "Branca",
			want:  false,
		},
		{
			name:  "Invalid - random value",
			value: "xyz",
			want:  false,
		},
		{
			name:  "Invalid - empty string",
			value: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidEthnicity(tt.value)
			if result != tt.want {
				t.Errorf("IsValidEthnicity(%q) = %v, want %v", tt.value, result, tt.want)
			}
		})
	}
}

func TestIsValidGender(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "Valid - suggested option",
			value: "Homem cisgênero",
			want:  true,
		},
		{
			name:  "Valid - custom text",
			value: "My custom gender identity",
			want:  true,
		},
		{
			name:  "Valid - single character",
			value: "X",
			want:  true,
		},
		{
			name:  "Invalid - empty string",
			value: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidGender(tt.value)
			if result != tt.want {
				t.Errorf("IsValidGender(%q) = %v, want %v", tt.value, result, tt.want)
			}
		})
	}
}

func TestIsValidFamilyIncome(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "Valid - Menos de 1 salário mínimo",
			value: "Menos de 1 salário mínimo",
			want:  true,
		},
		{
			name:  "Valid - 1 a 2 salários mínimos",
			value: "1 a 2 salários mínimos",
			want:  true,
		},
		{
			name:  "Valid - 2 a 3 salários mínimos",
			value: "2 a 3 salários mínimos",
			want:  true,
		},
		{
			name:  "Valid - 3 a 5 salários mínimos",
			value: "3 a 5 salários mínimos",
			want:  true,
		},
		{
			name:  "Valid - Mais de 5 salários mínimos",
			value: "Mais de 5 salários mínimos",
			want:  true,
		},
		{
			name:  "Invalid - wrong format",
			value: "1-2 salários",
			want:  false,
		},
		{
			name:  "Invalid - empty string",
			value: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidFamilyIncome(tt.value)
			if result != tt.want {
				t.Errorf("IsValidFamilyIncome(%q) = %v, want %v", tt.value, result, tt.want)
			}
		})
	}
}

func TestIsValidEducation(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "Valid - Fundamental incompleto",
			value: "Fundamental incompleto",
			want:  true,
		},
		{
			name:  "Valid - Fundamental completo",
			value: "Fundamental completo",
			want:  true,
		},
		{
			name:  "Valid - Médio incompleto",
			value: "Médio incompleto",
			want:  true,
		},
		{
			name:  "Valid - Médio completo",
			value: "Médio completo",
			want:  true,
		},
		{
			name:  "Valid - Superior incompleto",
			value: "Superior incompleto",
			want:  true,
		},
		{
			name:  "Valid - Superior completo",
			value: "Superior completo",
			want:  true,
		},
		{
			name:  "Valid - Pós Graduação",
			value: "Pós Graduação",
			want:  true,
		},
		{
			name:  "Valid - Mestrado",
			value: "Mestrado",
			want:  true,
		},
		{
			name:  "Valid - Doutorado",
			value: "Doutorado",
			want:  true,
		},
		{
			name:  "Invalid - wrong value",
			value: "PhD",
			want:  false,
		},
		{
			name:  "Invalid - empty string",
			value: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidEducation(tt.value)
			if result != tt.want {
				t.Errorf("IsValidEducation(%q) = %v, want %v", tt.value, result, tt.want)
			}
		})
	}
}

func TestIsValidDisability(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{
			name:  "Valid - Não sou pessoa com deficiência",
			value: "Não sou pessoa com deficiência",
			want:  true,
		},
		{
			name:  "Valid - Física",
			value: "Física",
			want:  true,
		},
		{
			name:  "Valid - Auditiva",
			value: "Auditiva",
			want:  true,
		},
		{
			name:  "Valid - Visual",
			value: "Visual",
			want:  true,
		},
		{
			name:  "Valid - Transtorno do Espectro Autista",
			value: "Transtorno do Espectro Autista",
			want:  true,
		},
		{
			name:  "Valid - Intelectual",
			value: "Intelectual",
			want:  true,
		},
		{
			name:  "Valid - Mental (psicossocial)",
			value: "Mental (psicossocial)",
			want:  true,
		},
		{
			name:  "Valid - Reabilitado do INSS",
			value: "Reabilitado do INSS",
			want:  true,
		},
		{
			name:  "Invalid - wrong value",
			value: "Motora",
			want:  false,
		},
		{
			name:  "Invalid - empty string",
			value: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDisability(tt.value)
			if result != tt.want {
				t.Errorf("IsValidDisability(%q) = %v, want %v", tt.value, result, tt.want)
			}
		})
	}
}

func TestToCitizenResponse(t *testing.T) {
	// Test data
	cpf := "12345678909"
	nome := "João Silva"
	nomeSocial := "João"
	nomeExibicao := "João S."
	sexo := "M"
	menorIdade := false
	raca := "parda"
	genero := "Homem cisgênero"
	rendaFamiliar := "1 a 2 salários mínimos"
	escolaridade := "Superior completo"
	deficiencia := "Não sou pessoa com deficiência"

	nascimentoData := time.Now().AddDate(-30, 0, 0) // 30 years ago
	nascimento := &Nascimento{
		Data: &nascimentoData,
	}

	citizen := &Citizen{
		ID:            "test-id",
		CPF:           cpf,
		Nome:          &nome,
		NomeSocial:    &nomeSocial,
		NomeExibicao:  &nomeExibicao,
		Sexo:          &sexo,
		Nascimento:    nascimento,
		MenorIdade:    &menorIdade,
		Raca:          &raca,
		Genero:        &genero,
		RendaFamiliar: &rendaFamiliar,
		Escolaridade:  &escolaridade,
		Deficiencia:   &deficiencia,
	}

	response := citizen.ToCitizenResponse()

	// Verify all fields are correctly mapped
	if response.ID != citizen.ID {
		t.Errorf("ToCitizenResponse() ID = %v, want %v", response.ID, citizen.ID)
	}
	if response.CPF != citizen.CPF {
		t.Errorf("ToCitizenResponse() CPF = %v, want %v", response.CPF, citizen.CPF)
	}
	if *response.Nome != *citizen.Nome {
		t.Errorf("ToCitizenResponse() Nome = %v, want %v", *response.Nome, *citizen.Nome)
	}
	if *response.NomeSocial != *citizen.NomeSocial {
		t.Errorf("ToCitizenResponse() NomeSocial = %v, want %v", *response.NomeSocial, *citizen.NomeSocial)
	}
	if *response.NomeExibicao != *citizen.NomeExibicao {
		t.Errorf("ToCitizenResponse() NomeExibicao = %v, want %v", *response.NomeExibicao, *citizen.NomeExibicao)
	}
	if *response.Sexo != *citizen.Sexo {
		t.Errorf("ToCitizenResponse() Sexo = %v, want %v", *response.Sexo, *citizen.Sexo)
	}
	if *response.MenorIdade != *citizen.MenorIdade {
		t.Errorf("ToCitizenResponse() MenorIdade = %v, want %v", *response.MenorIdade, *citizen.MenorIdade)
	}
	if *response.Raca != *citizen.Raca {
		t.Errorf("ToCitizenResponse() Raca = %v, want %v", *response.Raca, *citizen.Raca)
	}
	if *response.Genero != *citizen.Genero {
		t.Errorf("ToCitizenResponse() Genero = %v, want %v", *response.Genero, *citizen.Genero)
	}
	if *response.RendaFamiliar != *citizen.RendaFamiliar {
		t.Errorf("ToCitizenResponse() RendaFamiliar = %v, want %v", *response.RendaFamiliar, *citizen.RendaFamiliar)
	}
	if *response.Escolaridade != *citizen.Escolaridade {
		t.Errorf("ToCitizenResponse() Escolaridade = %v, want %v", *response.Escolaridade, *citizen.Escolaridade)
	}
	if *response.Deficiencia != *citizen.Deficiencia {
		t.Errorf("ToCitizenResponse() Deficiencia = %v, want %v", *response.Deficiencia, *citizen.Deficiencia)
	}

	// Verify wallet fields are excluded (not checking them in CitizenResponse)
	// This is the key difference - CitizenResponse should not have Documentos, Saude, etc.
}

func TestConvertToMaintenanceRequest(t *testing.T) {
	// Test data
	doc := &MaintenanceRequestDocument{
		ID:                        "test-id",
		CPF:                       "12345678909",
		CPFParticao:               123,
		Categoria:                 "Limpeza",
		DataAlvoFinalizacao:       "2024-01-15T10:00:00Z",
		DataFim:                   "2024-01-16T10:00:00Z",
		DataInicio:                "2024-01-10T10:00:00Z",
		Descricao:                 "Test description",
		IDChamado:                 "chamado-123",
		Indicador:                 true,
		NomeUnidadeOrganizacional: "Test Unit",
		OrigemOcorrencia:          "1746",
		Status:                    "Fechado",
		Subtipo:                   "Calçada",
		Tipo:                      "Manutenção",
		Endereco:                  "Rua Test, 123",
		TotalChamados:             10,
		TotalFechados:             5,
	}

	result := doc.ConvertToMaintenanceRequest()

	// Verify basic fields
	if result.ID != doc.ID {
		t.Errorf("ConvertToMaintenanceRequest() ID = %v, want %v", result.ID, doc.ID)
	}
	if result.CPF != doc.CPF {
		t.Errorf("ConvertToMaintenanceRequest() CPF = %v, want %v", result.CPF, doc.CPF)
	}
	if result.IDChamado != doc.IDChamado {
		t.Errorf("ConvertToMaintenanceRequest() IDChamado = %v, want %v", result.IDChamado, doc.IDChamado)
	}
	if result.Categoria != doc.Categoria {
		t.Errorf("ConvertToMaintenanceRequest() Categoria = %v, want %v", result.Categoria, doc.Categoria)
	}

	// Verify time parsing
	if result.DataInicio == nil {
		t.Error("ConvertToMaintenanceRequest() DataInicio should not be nil")
	}
	if result.DataAlvoFinalizacao == nil {
		t.Error("ConvertToMaintenanceRequest() DataAlvoFinalizacao should not be nil")
	}
	if result.DataFim == nil {
		t.Error("ConvertToMaintenanceRequest() DataFim should not be nil")
	}

	// Verify string pointer fields
	if result.Descricao == nil || *result.Descricao != doc.Descricao {
		t.Errorf("ConvertToMaintenanceRequest() Descricao = %v, want %v", result.Descricao, doc.Descricao)
	}
	if result.Endereco == nil || *result.Endereco != doc.Endereco {
		t.Errorf("ConvertToMaintenanceRequest() Endereco = %v, want %v", result.Endereco, doc.Endereco)
	}

	// Verify integer fields
	if result.TotalChamados != doc.TotalChamados {
		t.Errorf("ConvertToMaintenanceRequest() TotalChamados = %v, want %v", result.TotalChamados, doc.TotalChamados)
	}
	if result.TotalFechados != doc.TotalFechados {
		t.Errorf("ConvertToMaintenanceRequest() TotalFechados = %v, want %v", result.TotalFechados, doc.TotalFechados)
	}
}

func TestConvertToMaintenanceRequest_EmptyDates(t *testing.T) {
	doc := &MaintenanceRequestDocument{
		ID:                        "test-id",
		CPF:                       "12345678909",
		IDChamado:                 "chamado-123",
		Categoria:                 "Test",
		DataInicio:                "", // Empty date
		DataAlvoFinalizacao:       "",
		DataFim:                   "",
		Descricao:                 "",
		NomeUnidadeOrganizacional: "Test",
		OrigemOcorrencia:          "1746",
		Status:                    "Aberto",
		Subtipo:                   "Test",
		Tipo:                      "Test",
	}

	result := doc.ConvertToMaintenanceRequest()

	// Verify empty dates are handled correctly
	if result.DataInicio != nil {
		t.Errorf("ConvertToMaintenanceRequest() DataInicio should be nil for empty string, got %v", result.DataInicio)
	}
	if result.DataAlvoFinalizacao != nil {
		t.Errorf("ConvertToMaintenanceRequest() DataAlvoFinalizacao should be nil for empty string, got %v", result.DataAlvoFinalizacao)
	}
	if result.DataFim != nil {
		t.Errorf("ConvertToMaintenanceRequest() DataFim should be nil for empty string, got %v", result.DataFim)
	}

	// Verify empty description is handled correctly
	if result.Descricao != nil {
		t.Errorf("ConvertToMaintenanceRequest() Descricao should be nil for empty string, got %v", result.Descricao)
	}
}

func TestValidEthnicityOptions(t *testing.T) {
	options := ValidEthnicityOptions()

	expectedCount := 6
	if len(options) != expectedCount {
		t.Errorf("ValidEthnicityOptions() returned %d options, want %d", len(options), expectedCount)
	}

	// Verify all expected options are present
	expected := map[string]bool{
		"branca":   true,
		"preta":    true,
		"parda":    true,
		"amarela":  true,
		"indigena": true,
		"outra":    true,
	}

	for _, opt := range options {
		if !expected[opt] {
			t.Errorf("ValidEthnicityOptions() contains unexpected option: %q", opt)
		}
	}
}

func TestValidGenderOptions(t *testing.T) {
	options := ValidGenderOptions()

	expectedCount := 7
	if len(options) != expectedCount {
		t.Errorf("ValidGenderOptions() returned %d options, want %d", len(options), expectedCount)
	}

	// Just verify we have the expected options (order doesn't matter)
	found := make(map[string]bool)
	for _, opt := range options {
		found[opt] = true
	}

	if !found["Homem cisgênero"] {
		t.Error("ValidGenderOptions() missing 'Homem cisgênero'")
	}
	if !found["Mulher cisgênero"] {
		t.Error("ValidGenderOptions() missing 'Mulher cisgênero'")
	}
	if !found["Outro"] {
		t.Error("ValidGenderOptions() missing 'Outro'")
	}
}

func TestValidFamilyIncomeOptions(t *testing.T) {
	options := ValidFamilyIncomeOptions()

	expectedCount := 5
	if len(options) != expectedCount {
		t.Errorf("ValidFamilyIncomeOptions() returned %d options, want %d", len(options), expectedCount)
	}
}

func TestValidEducationOptions(t *testing.T) {
	options := ValidEducationOptions()

	expectedCount := 9
	if len(options) != expectedCount {
		t.Errorf("ValidEducationOptions() returned %d options, want %d", len(options), expectedCount)
	}
}

func TestValidDisabilityOptions(t *testing.T) {
	options := ValidDisabilityOptions()

	expectedCount := 8
	if len(options) != expectedCount {
		t.Errorf("ValidDisabilityOptions() returned %d options, want %d", len(options), expectedCount)
	}
}
