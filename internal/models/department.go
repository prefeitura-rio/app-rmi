package models

import (
	"time"
)

// Department represents an administrative unit (Unidade Administrativa - UA)
type Department struct {
	ID            string     `bson:"_id" json:"id"`
	CdUA          string     `bson:"cd_ua" json:"cd_ua"`
	SiglaUA       string     `bson:"sigla_ua" json:"sigla_ua"`
	NomeUA        string     `bson:"nome_ua" json:"nome_ua"`
	CdUAPai       string     `bson:"cd_ua_pai" json:"cd_ua_pai"`
	Nivel         int        `bson:"nivel" json:"nivel"`
	OrdemUABasica string     `bson:"ordem_ua_basica" json:"ordem_ua_basica"`
	OrdemAbsoluta string     `bson:"ordem_absoluta" json:"ordem_absoluta"`
	OrdemRelativa string     `bson:"ordem_relativa" json:"ordem_relativa"`
	Msg           *string    `bson:"msg,omitempty" json:"msg,omitempty"`
	UpdatedAt     *time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// DepartmentResponse represents a single department in API responses
type DepartmentResponse struct {
	ID            string     `json:"id"`
	CdUA          string     `json:"cd_ua"`
	SiglaUA       string     `json:"sigla_ua"`
	NomeUA        string     `json:"nome_ua"`
	CdUAPai       string     `json:"cd_ua_pai"`
	Nivel         int        `json:"nivel"`
	OrdemUABasica string     `json:"ordem_ua_basica"`
	OrdemAbsoluta string     `json:"ordem_absoluta"`
	OrdemRelativa string     `json:"ordem_relativa"`
	Msg           *string    `json:"msg,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

// DepartmentListResponse represents a paginated list of departments
type DepartmentListResponse struct {
	Departments []DepartmentResponse `json:"departments"`
	Pagination  PaginationInfo       `json:"pagination"`
	TotalCount  int64                `json:"total_count"`
}

// ToResponse converts a Department to DepartmentResponse
func (d *Department) ToResponse() DepartmentResponse {
	return DepartmentResponse{
		ID:            d.ID,
		CdUA:          d.CdUA,
		SiglaUA:       d.SiglaUA,
		NomeUA:        d.NomeUA,
		CdUAPai:       d.CdUAPai,
		Nivel:         d.Nivel,
		OrdemUABasica: d.OrdemUABasica,
		OrdemAbsoluta: d.OrdemAbsoluta,
		OrdemRelativa: d.OrdemRelativa,
		Msg:           d.Msg,
		UpdatedAt:     d.UpdatedAt,
	}
}
