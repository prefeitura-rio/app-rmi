package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDepartment_ToResponse(t *testing.T) {
	now := time.Now()
	msg := "Test message"

	dept := &Department{
		ID:            "dept-123",
		CdUA:          "1000",
		SiglaUA:       "PCRJ",
		NomeUA:        "Prefeitura do Rio de Janeiro",
		CdUAPai:       "0",
		Nivel:         1,
		OrdemUABasica: "001",
		OrdemAbsoluta: "001",
		OrdemRelativa: "001",
		Msg:           &msg,
		UpdatedAt:     &now,
	}

	response := dept.ToResponse()

	assert.Equal(t, dept.ID, response.ID)
	assert.Equal(t, dept.CdUA, response.CdUA)
	assert.Equal(t, dept.SiglaUA, response.SiglaUA)
	assert.Equal(t, dept.NomeUA, response.NomeUA)
	assert.Equal(t, dept.CdUAPai, response.CdUAPai)
	assert.Equal(t, dept.Nivel, response.Nivel)
	assert.Equal(t, dept.OrdemUABasica, response.OrdemUABasica)
	assert.Equal(t, dept.OrdemAbsoluta, response.OrdemAbsoluta)
	assert.Equal(t, dept.OrdemRelativa, response.OrdemRelativa)
	require.NotNil(t, response.Msg)
	assert.Equal(t, *dept.Msg, *response.Msg)
	require.NotNil(t, response.UpdatedAt)
	assert.Equal(t, *dept.UpdatedAt, *response.UpdatedAt)
}

func TestDepartment_ToResponse_NilFields(t *testing.T) {
	dept := &Department{
		ID:            "dept-456",
		CdUA:          "2000",
		SiglaUA:       "SMF",
		NomeUA:        "Secretaria Municipal de Fazenda",
		CdUAPai:       "1000",
		Nivel:         2,
		OrdemUABasica: "002",
		OrdemAbsoluta: "002",
		OrdemRelativa: "001.001",
		Msg:           nil,
		UpdatedAt:     nil,
	}

	response := dept.ToResponse()

	assert.Equal(t, dept.ID, response.ID)
	assert.Equal(t, dept.CdUA, response.CdUA)
	assert.Equal(t, dept.SiglaUA, response.SiglaUA)
	assert.Equal(t, dept.NomeUA, response.NomeUA)
	assert.Nil(t, response.Msg)
	assert.Nil(t, response.UpdatedAt)
}

func TestDepartment_ToResponse_EmptyStrings(t *testing.T) {
	dept := &Department{
		ID:            "",
		CdUA:          "",
		SiglaUA:       "",
		NomeUA:        "",
		CdUAPai:       "",
		Nivel:         0,
		OrdemUABasica: "",
		OrdemAbsoluta: "",
		OrdemRelativa: "",
	}

	response := dept.ToResponse()

	assert.Empty(t, response.ID)
	assert.Empty(t, response.CdUA)
	assert.Empty(t, response.SiglaUA)
	assert.Empty(t, response.NomeUA)
	assert.Empty(t, response.CdUAPai)
	assert.Equal(t, 0, response.Nivel)
}

func TestDepartment_Structure(t *testing.T) {
	dept := &Department{}

	assert.IsType(t, "", dept.ID)
	assert.IsType(t, "", dept.CdUA)
	assert.IsType(t, "", dept.SiglaUA)
	assert.IsType(t, "", dept.NomeUA)
	assert.IsType(t, 0, dept.Nivel)
}

func TestDepartmentResponse_Structure(t *testing.T) {
	response := &DepartmentResponse{}

	assert.IsType(t, "", response.ID)
	assert.IsType(t, "", response.CdUA)
	assert.IsType(t, "", response.SiglaUA)
	assert.IsType(t, "", response.NomeUA)
	assert.IsType(t, 0, response.Nivel)
}

func TestDepartmentListResponse_Structure(t *testing.T) {
	listResp := &DepartmentListResponse{
		Departments: []DepartmentResponse{},
		Pagination:  PaginationInfo{},
		TotalCount:  0,
	}

	assert.NotNil(t, listResp.Departments)
	assert.Len(t, listResp.Departments, 0)
	assert.Equal(t, int64(0), listResp.TotalCount)
}

func TestDepartmentListResponse_WithData(t *testing.T) {
	dept1 := &Department{
		ID:      "dept-1",
		CdUA:    "1000",
		SiglaUA: "PCRJ",
		NomeUA:  "Prefeitura",
		Nivel:   1,
	}

	dept2 := &Department{
		ID:      "dept-2",
		CdUA:    "2000",
		SiglaUA: "SMF",
		NomeUA:  "Secretaria de Fazenda",
		Nivel:   2,
	}

	listResp := &DepartmentListResponse{
		Departments: []DepartmentResponse{
			dept1.ToResponse(),
			dept2.ToResponse(),
		},
		Pagination: PaginationInfo{
			Page:       1,
			PerPage:    10,
			Total:      2,
			TotalPages: 1,
		},
		TotalCount: 2,
	}

	assert.Len(t, listResp.Departments, 2)
	assert.Equal(t, int64(2), listResp.TotalCount)
	assert.Equal(t, 2, listResp.Pagination.Total)
	assert.Equal(t, "dept-1", listResp.Departments[0].ID)
	assert.Equal(t, "dept-2", listResp.Departments[1].ID)
}

func TestDepartment_ToResponse_PreservesAllFields(t *testing.T) {
	now := time.Now()
	msg := "Department message"

	original := &Department{
		ID:            "test-id",
		CdUA:          "9999",
		SiglaUA:       "TEST",
		NomeUA:        "Test Department",
		CdUAPai:       "8888",
		Nivel:         3,
		OrdemUABasica: "999",
		OrdemAbsoluta: "999.888",
		OrdemRelativa: "001.002.003",
		Msg:           &msg,
		UpdatedAt:     &now,
	}

	response := original.ToResponse()

	// Verify all fields are copied
	assert.Equal(t, original.ID, response.ID)
	assert.Equal(t, original.CdUA, response.CdUA)
	assert.Equal(t, original.SiglaUA, response.SiglaUA)
	assert.Equal(t, original.NomeUA, response.NomeUA)
	assert.Equal(t, original.CdUAPai, response.CdUAPai)
	assert.Equal(t, original.Nivel, response.Nivel)
	assert.Equal(t, original.OrdemUABasica, response.OrdemUABasica)
	assert.Equal(t, original.OrdemAbsoluta, response.OrdemAbsoluta)
	assert.Equal(t, original.OrdemRelativa, response.OrdemRelativa)

	// Verify pointer fields
	require.NotNil(t, response.Msg)
	assert.Equal(t, *original.Msg, *response.Msg)
	require.NotNil(t, response.UpdatedAt)
	assert.Equal(t, original.UpdatedAt.Unix(), response.UpdatedAt.Unix())
}

func TestDepartment_MultipleToResponseCalls(t *testing.T) {
	dept := &Department{
		ID:      "dept-multi",
		CdUA:    "1234",
		SiglaUA: "MULTI",
		NomeUA:  "Multiple Calls Test",
		Nivel:   1,
	}

	// Call ToResponse multiple times
	resp1 := dept.ToResponse()
	resp2 := dept.ToResponse()
	resp3 := dept.ToResponse()

	// All should have same values
	assert.Equal(t, resp1.ID, resp2.ID)
	assert.Equal(t, resp2.ID, resp3.ID)
	assert.Equal(t, resp1.CdUA, resp2.CdUA)
	assert.Equal(t, resp2.CdUA, resp3.CdUA)
}

func TestDepartment_DifferentNiveis(t *testing.T) {
	niveis := []int{1, 2, 3, 4, 5}

	for _, nivel := range niveis {
		dept := &Department{
			ID:    "dept-nivel",
			CdUA:  "1000",
			Nivel: nivel,
		}

		response := dept.ToResponse()
		assert.Equal(t, nivel, response.Nivel)
	}
}
