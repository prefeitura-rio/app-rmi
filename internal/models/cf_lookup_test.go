package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestCFLookup_ToResponse(t *testing.T) {
	now := time.Now()
	idEquip := "equip-123"
	complemento := "Sala 101"

	cf := &CFLookup{
		ID:             primitive.NewObjectID(),
		CPF:            "12345678901",
		AddressHash:    "hash123",
		AddressUsed:    "Rua Teste, 123",
		DistanceMeters: 500,
		LookupSource:   "mcp",
		CreatedAt:      now,
		UpdatedAt:      now,
		IsActive:       true,
		CFData: CFInfo{
			IDEquipamento:        &idEquip,
			NomeOficial:          "CF Teste",
			NomePopular:          "Clinica Teste",
			Logradouro:           "Rua das Clinicas",
			Numero:               "100",
			Complemento:          &complemento,
			Bairro:               "Centro",
			RegiaoAdministrativa: "I RA",
			Ativo:                true,
			AbertoAoPublico:      true,
		},
	}

	response := cf.ToResponse()

	assert.True(t, response.Found)
	require.NotNil(t, response.CFData)
	assert.Equal(t, cf.CFData.NomeOficial, response.CFData.NomeOficial)
	require.NotNil(t, response.DistanceMeters)
	assert.Equal(t, 500, *response.DistanceMeters)
	assert.Equal(t, "mcp", response.LookupSource)
	require.NotNil(t, response.CreatedAt)
	assert.Equal(t, now.Unix(), response.CreatedAt.Unix())
}

func TestCFLookup_ToResponse_Nil(t *testing.T) {
	var cf *CFLookup = nil

	response := cf.ToResponse()

	assert.False(t, response.Found)
	assert.Nil(t, response.CFData)
	assert.Nil(t, response.DistanceMeters)
	assert.Empty(t, response.LookupSource)
	assert.Nil(t, response.CreatedAt)
}

func TestCFLookup_ToClinicaFamilia(t *testing.T) {
	now := time.Now()
	complemento := "2º andar"
	telefone1 := "21-1234-5678"
	telefone2 := "21-9876-5432"

	cf := &CFLookup{
		CFData: CFInfo{
			NomeOficial: "CF Centro",
			NomePopular: "Clinica do Centro",
			Logradouro:  "Rua Principal",
			Numero:      "200",
			Complemento: &complemento,
			Bairro:      "Centro",
			Contato: CFContactInfo{
				Telefones: []string{telefone1, telefone2},
				Email:     "clinica@exemplo.com",
			},
			HorarioFuncionamento: []CFHorario{
				{Dia: "Segunda", Abre: "08:00", Fecha: "17:00"},
				{Dia: "Terça", Abre: "08:00", Fecha: "17:00"},
			},
			UpdatedAt: now,
		},
	}

	clinica := cf.ToClinicaFamilia()

	require.NotNil(t, clinica)
	require.NotNil(t, clinica.Nome)
	assert.Equal(t, "Clinica do Centro", *clinica.Nome)
	require.NotNil(t, clinica.Endereco)
	assert.Contains(t, *clinica.Endereco, "Rua Principal, 200")
	assert.Contains(t, *clinica.Endereco, "2º andar")
	assert.Contains(t, *clinica.Endereco, "Centro")
	require.NotNil(t, clinica.Telefone)
	assert.Equal(t, telefone1, *clinica.Telefone)
	require.NotNil(t, clinica.HorarioAtendimento)
	assert.NotEmpty(t, *clinica.HorarioAtendimento)
}

func TestCFLookup_ToClinicaFamilia_Nil(t *testing.T) {
	var cf *CFLookup = nil

	clinica := cf.ToClinicaFamilia()

	assert.Nil(t, clinica)
}

func TestCFLookup_ToClinicaFamilia_NoComplemento(t *testing.T) {
	cf := &CFLookup{
		CFData: CFInfo{
			NomeOficial: "CF Sem Complemento",
			NomePopular: "Clinica Sem Compl",
			Logradouro:  "Av Principal",
			Numero:      "500",
			Complemento: nil,
			Bairro:      "Zona Sul",
		},
	}

	clinica := cf.ToClinicaFamilia()

	require.NotNil(t, clinica)
	require.NotNil(t, clinica.Endereco)
	assert.Contains(t, *clinica.Endereco, "Av Principal, 500")
	assert.Contains(t, *clinica.Endereco, "Zona Sul")
	assert.NotContains(t, *clinica.Endereco, "nil")
}

func TestCFLookup_ToClinicaFamilia_EmptyComplemento(t *testing.T) {
	emptyComplemento := ""
	cf := &CFLookup{
		CFData: CFInfo{
			NomeOficial: "CF Empty Complemento",
			NomePopular: "Clinica Empty",
			Logradouro:  "Rua Vazia",
			Numero:      "123",
			Complemento: &emptyComplemento,
			Bairro:      "Bairro",
		},
	}

	clinica := cf.ToClinicaFamilia()

	require.NotNil(t, clinica)
	require.NotNil(t, clinica.Endereco)
	// Empty complemento should not be included
	assert.Contains(t, *clinica.Endereco, "Rua Vazia, 123")
}

func TestCFLookup_ToClinicaFamilia_NoTelefones(t *testing.T) {
	cf := &CFLookup{
		CFData: CFInfo{
			NomeOficial: "CF Sem Tel",
			Logradouro:  "Rua Sem Tel",
			Numero:      "1",
			Bairro:      "Centro",
			Contato: CFContactInfo{
				Telefones: []string{},
			},
		},
	}

	clinica := cf.ToClinicaFamilia()

	require.NotNil(t, clinica)
	assert.Nil(t, clinica.Telefone)
}

func TestCFLookup_ToClinicaFamilia_NoHorario(t *testing.T) {
	cf := &CFLookup{
		CFData: CFInfo{
			NomeOficial:          "CF Sem Horario",
			Logradouro:           "Rua",
			Numero:               "1",
			Bairro:               "Bairro",
			HorarioFuncionamento: []CFHorario{},
		},
	}

	clinica := cf.ToClinicaFamilia()

	require.NotNil(t, clinica)
	// Should have empty or default horario
	assert.NotNil(t, clinica.HorarioAtendimento)
}

func TestCFInfo_Structure(t *testing.T) {
	info := &CFInfo{}

	assert.IsType(t, "", info.NomeOficial)
	assert.IsType(t, "", info.NomePopular)
	assert.IsType(t, "", info.Logradouro)
	assert.IsType(t, true, info.Ativo)
	assert.IsType(t, CFContactInfo{}, info.Contato)
	assert.IsType(t, []CFHorario{}, info.HorarioFuncionamento)
}

func TestCFContactInfo_Structure(t *testing.T) {
	contact := &CFContactInfo{}

	assert.IsType(t, []string{}, contact.Telefones)
	assert.IsType(t, "", contact.Email)
	assert.IsType(t, CFSocial{}, contact.RedesSocial)
}

func TestCFSocial_Structure(t *testing.T) {
	social := &CFSocial{}

	assert.Nil(t, social.Facebook)
	assert.Nil(t, social.Instagram)
	assert.Nil(t, social.Twitter)
}

func TestCFHorario_Structure(t *testing.T) {
	horario := &CFHorario{
		Dia:   "Segunda",
		Abre:  "08:00",
		Fecha: "17:00",
	}

	assert.Equal(t, "Segunda", horario.Dia)
	assert.Equal(t, "08:00", horario.Abre)
	assert.Equal(t, "17:00", horario.Fecha)
}

func TestEquipeSaudeInfo_Structure(t *testing.T) {
	equipe := &EquipeSaudeInfo{}

	assert.IsType(t, "", equipe.NomeOficial)
	assert.IsType(t, "", equipe.NomePopular)
	assert.IsType(t, true, equipe.Ativo)
	assert.IsType(t, []string{}, equipe.Medicos)
	assert.IsType(t, []string{}, equipe.Enfermeiros)
}

func TestHealthServicesResult_Structure(t *testing.T) {
	result := &HealthServicesResult{
		HealthFacility:   &CFInfo{NomeOficial: "CF Test"},
		FamilyHealthTeam: &EquipeSaudeInfo{NomeOficial: "Equipe Test"},
	}

	require.NotNil(t, result.HealthFacility)
	require.NotNil(t, result.FamilyHealthTeam)
	assert.Equal(t, "CF Test", result.HealthFacility.NomeOficial)
	assert.Equal(t, "Equipe Test", result.FamilyHealthTeam.NomeOficial)
}

func TestCFLookupRequest_Structure(t *testing.T) {
	req := &CFLookupRequest{
		CPF:     "12345678901",
		Address: "Rua Teste, 123",
		Force:   true,
	}

	assert.Equal(t, "12345678901", req.CPF)
	assert.Equal(t, "Rua Teste, 123", req.Address)
	assert.True(t, req.Force)
}

func TestCFLookupResponse_NotFound(t *testing.T) {
	resp := &CFLookupResponse{
		Found: false,
	}

	assert.False(t, resp.Found)
	assert.Nil(t, resp.CFData)
	assert.Nil(t, resp.DistanceMeters)
	assert.Empty(t, resp.LookupSource)
}

func TestCFLookupStats_Structure(t *testing.T) {
	now := time.Now()
	stats := &CFLookupStats{
		TotalLookups:      100,
		SuccessfulLookups: 80,
		FailedLookups:     20,
		SuccessRate:       0.8,
		AvgDistance:       450.5,
		LastLookup:        &now,
	}

	assert.Equal(t, int64(100), stats.TotalLookups)
	assert.Equal(t, int64(80), stats.SuccessfulLookups)
	assert.Equal(t, int64(20), stats.FailedLookups)
	assert.Equal(t, 0.8, stats.SuccessRate)
	assert.Equal(t, 450.5, stats.AvgDistance)
	require.NotNil(t, stats.LastLookup)
}

func TestCFLookup_ToResponse_WithAllFields(t *testing.T) {
	now := time.Now()
	idEquip := "equip-456"
	complemento := "Prédio A"
	facebook := "fb.com/cf"
	instagram := "ig.com/cf"

	cf := &CFLookup{
		ID:             primitive.NewObjectID(),
		CPF:            "98765432100",
		AddressHash:    "hash456",
		AddressUsed:    "Av Test, 999",
		DistanceMeters: 1500,
		LookupSource:   "mcp",
		CreatedAt:      now,
		UpdatedAt:      now,
		IsActive:       true,
		CFData: CFInfo{
			IDEquipamento: &idEquip,
			NomeOficial:   "CF Completa",
			NomePopular:   "Clinica Completa",
			Logradouro:    "Avenida Central",
			Numero:        "999",
			Complemento:   &complemento,
			Bairro:        "Zona Norte",
			Contato: CFContactInfo{
				Telefones: []string{"21-1111-1111", "21-2222-2222"},
				Email:     "contato@cf.com",
				RedesSocial: CFSocial{
					Facebook:  &facebook,
					Instagram: &instagram,
				},
			},
			HorarioFuncionamento: []CFHorario{
				{Dia: "Segunda", Abre: "07:00", Fecha: "19:00"},
				{Dia: "Terça", Abre: "07:00", Fecha: "19:00"},
				{Dia: "Quarta", Abre: "07:00", Fecha: "19:00"},
			},
			Ativo:           true,
			AbertoAoPublico: true,
			UpdatedAt:       now,
		},
	}

	response := cf.ToResponse()

	assert.True(t, response.Found)
	require.NotNil(t, response.CFData)
	assert.Equal(t, "CF Completa", response.CFData.NomeOficial)
	assert.Equal(t, "Clinica Completa", response.CFData.NomePopular)
	require.NotNil(t, response.DistanceMeters)
	assert.Equal(t, 1500, *response.DistanceMeters)
	assert.Equal(t, "mcp", response.LookupSource)
}

func TestCFLookup_ToEquipeSaudeFamilia(t *testing.T) {
	now := time.Now()
	idEquipe := "equipe-123"
	telefone1 := "21-9876-5432"

	cf := &CFLookup{
		CFData: CFInfo{
			NomeOficial: "CF Centro",
		},
		EquipeSaudeData: &EquipeSaudeInfo{
			IDEquipe:    &idEquipe,
			NomeOficial: "Equipe Saúde da Família Centro",
			NomePopular: "ESF Centro",
			Contato: CFContactInfo{
				Telefones: []string{telefone1, "21-1234-5678"},
			},
			Medicos:     []string{"Dr. João Silva", "Dra. Maria Santos"},
			Enfermeiros: []string{"Enf. Pedro Costa", "Enf. Ana Lima"},
			Ativo:       true,
			UpdatedAt:   now,
		},
	}

	equipe := cf.ToEquipeSaudeFamilia()

	require.NotNil(t, equipe)
	require.NotNil(t, equipe.Indicador)
	assert.True(t, *equipe.Indicador)
	assert.Equal(t, &idEquipe, equipe.IDINE)
	require.NotNil(t, equipe.Nome)
	assert.Equal(t, "Equipe Saúde da Família Centro", *equipe.Nome)
	require.NotNil(t, equipe.Telefone)
	assert.Equal(t, telefone1, *equipe.Telefone)
	assert.Len(t, equipe.Medicos, 2)
	assert.Len(t, equipe.Enfermeiros, 2)
	assert.Equal(t, "Dr. João Silva", *equipe.Medicos[0].Nome)
	assert.Equal(t, "Dra. Maria Santos", *equipe.Medicos[1].Nome)
	assert.Equal(t, "Enf. Pedro Costa", *equipe.Enfermeiros[0].Nome)
	assert.Equal(t, "Enf. Ana Lima", *equipe.Enfermeiros[1].Nome)
}

func TestCFLookup_ToEquipeSaudeFamilia_NilCFLookup(t *testing.T) {
	var cf *CFLookup = nil

	equipe := cf.ToEquipeSaudeFamilia()

	assert.Nil(t, equipe)
}

func TestCFLookup_ToEquipeSaudeFamilia_NilEquipeSaudeData(t *testing.T) {
	cf := &CFLookup{
		CFData: CFInfo{
			NomeOficial: "CF Test",
		},
		EquipeSaudeData: nil,
	}

	equipe := cf.ToEquipeSaudeFamilia()

	assert.Nil(t, equipe)
}

func TestCFLookup_ToEquipeSaudeFamilia_NoTelefones(t *testing.T) {
	idEquipe := "equipe-456"

	cf := &CFLookup{
		EquipeSaudeData: &EquipeSaudeInfo{
			IDEquipe:    &idEquipe,
			NomeOficial: "Equipe Sem Telefone",
			Contato: CFContactInfo{
				Telefones: []string{},
			},
			Medicos:     []string{"Dr. Test"},
			Enfermeiros: []string{"Enf. Test"},
		},
	}

	equipe := cf.ToEquipeSaudeFamilia()

	require.NotNil(t, equipe)
	assert.Nil(t, equipe.Telefone)
}

func TestCFLookup_ToEquipeSaudeFamilia_NoMedicosEnfermeiros(t *testing.T) {
	idEquipe := "equipe-789"

	cf := &CFLookup{
		EquipeSaudeData: &EquipeSaudeInfo{
			IDEquipe:    &idEquipe,
			NomeOficial: "Equipe Sem Profissionais",
			Medicos:     []string{},
			Enfermeiros: []string{},
		},
	}

	equipe := cf.ToEquipeSaudeFamilia()

	require.NotNil(t, equipe)
	assert.Len(t, equipe.Medicos, 0)
	assert.Len(t, equipe.Enfermeiros, 0)
}

func TestCFLookup_ToEquipeSaudeFamilia_ProfissionalSaudeStructure(t *testing.T) {
	idEquipe := "equipe-abc"

	cf := &CFLookup{
		EquipeSaudeData: &EquipeSaudeInfo{
			IDEquipe:    &idEquipe,
			NomeOficial: "Equipe Test",
			Medicos:     []string{"Dr. Test"},
			Enfermeiros: []string{"Enf. Test"},
		},
	}

	equipe := cf.ToEquipeSaudeFamilia()

	require.NotNil(t, equipe)
	require.Len(t, equipe.Medicos, 1)
	require.Len(t, equipe.Enfermeiros, 1)

	// Check that ProfissionalSaude has nil SUS ID (MCP doesn't provide it)
	assert.Nil(t, equipe.Medicos[0].IDProfissionalSUS)
	assert.Nil(t, equipe.Enfermeiros[0].IDProfissionalSUS)

	// Check names are set correctly
	require.NotNil(t, equipe.Medicos[0].Nome)
	require.NotNil(t, equipe.Enfermeiros[0].Nome)
	assert.Equal(t, "Dr. Test", *equipe.Medicos[0].Nome)
	assert.Equal(t, "Enf. Test", *equipe.Enfermeiros[0].Nome)
}
