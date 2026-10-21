package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
)

// AdminListCPFSecretaria godoc
// @Summary Listar vínculos CPF-Secretaria
// @Description Retorna todos os vínculos de secretaria associados a um CPF na base de apoio.
// @Tags admin,cpf-secretaria
// @Produce json
// @Param cpf path string true "CPF do usuário (11 dígitos)"
// @Security BearerAuth
// @Success 200 {object} models.CPFSecretariaListResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/cpf-secretaria/{cpf} [get]
func AdminListCPFSecretaria(c *gin.Context) {
	cpf := c.Param("cpf")
	if err := validateCPFParam(cpf); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if services.CPFSecretariaServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "service unavailable"})
		return
	}

	mappings, err := services.CPFSecretariaServiceInstance.ListByCPF(c.Request.Context(), cpf)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	responses := make([]models.CPFSecretariaResponse, 0, len(mappings))
	for _, m := range mappings {
		responses = append(responses, m.ToResponse())
	}

	c.JSON(http.StatusOK, models.CPFSecretariaListResponse{
		CPF:      cpf,
		Mappings: responses,
	})
}

// AdminAddCPFSecretaria godoc
// @Summary Adicionar vínculo CPF-Secretaria
// @Description Vincula um CPF a uma secretaria (cd_ua SICI) na base de apoio. Um CPF pode ter múltiplos vínculos.
// @Tags admin,cpf-secretaria
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do usuário (11 dígitos)"
// @Param data body models.AddCPFSecretariaRequest true "cd_ua da secretaria"
// @Security BearerAuth
// @Success 201 {object} models.CPFSecretariaResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/cpf-secretaria/{cpf} [post]
func AdminAddCPFSecretaria(c *gin.Context) {
	cpf := c.Param("cpf")
	if err := validateCPFParam(cpf); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	var req models.AddCPFSecretariaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.CdUA) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cd_ua is required"})
		return
	}

	if services.CPFSecretariaServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "service unavailable"})
		return
	}

	createdBy, _ := middleware.ExtractCPFFromToken(c)

	mapping, err := services.CPFSecretariaServiceInstance.AddMapping(c.Request.Context(), cpf, req.CdUA, createdBy)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusCreated, mapping.ToResponse())
}

// AdminRemoveCPFSecretaria godoc
// @Summary Remover vínculo CPF-Secretaria
// @Description Remove o vínculo entre um CPF e uma secretaria específica.
// @Tags admin,cpf-secretaria
// @Produce json
// @Param cpf path string true "CPF do usuário (11 dígitos)"
// @Param cd_ua path string true "Código da UA (SICI)"
// @Security BearerAuth
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/cpf-secretaria/{cpf}/{cd_ua} [delete]
func AdminRemoveCPFSecretaria(c *gin.Context) {
	cpf := c.Param("cpf")
	cdUA := c.Param("cd_ua")

	if err := validateCPFParam(cpf); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if strings.TrimSpace(cdUA) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "cd_ua is required"})
		return
	}

	if services.CPFSecretariaServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "service unavailable"})
		return
	}

	if err := services.CPFSecretariaServiceInstance.RemoveMapping(c.Request.Context(), cpf, cdUA); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// GetCPFSecretarias godoc
// @Summary Consultar secretarias de um CPF
// @Description Retorna a lista de cd_uas vinculados ao CPF na base de apoio. Usado por outras APIs (app-go-api) via service account.
// @Tags cpf-secretaria
// @Produce json
// @Param cpf path string true "CPF do usuário (11 dígitos)"
// @Security BearerAuth
// @Success 200 {object} models.CPFSecretariaQueryResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /cpf-secretaria/{cpf} [get]
func GetCPFSecretarias(c *gin.Context) {
	cpf := c.Param("cpf")
	if err := validateCPFParam(cpf); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if services.CPFSecretariaServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "service unavailable"})
		return
	}

	cdUAs, err := services.CPFSecretariaServiceInstance.GetCdUAsByCPF(c.Request.Context(), cpf)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	if cdUAs == nil {
		cdUAs = []string{}
	}

	c.JSON(http.StatusOK, models.CPFSecretariaQueryResponse{
		CPF:   cpf,
		CdUAs: cdUAs,
	})
}

func validateCPFParam(cpf string) error {
	cpf = strings.ReplaceAll(cpf, ".", "")
	cpf = strings.ReplaceAll(cpf, "-", "")
	if len(cpf) != 11 {
		return fmt.Errorf("invalid CPF: must have 11 digits")
	}
	return nil
}
