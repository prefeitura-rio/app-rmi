package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
)

// AdminListMobilidadeVehicleBrands godoc
// @Summary Listar marcas (admin)
// @Description Lista completa de marcas do catálogo, incluindo soft-deleted (admin).
// @Tags mobilidade
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.VehicleBrandsResponse "Lista tipada de marcas em data"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /admin/mobilidade/vehicle-brands [get]
func AdminListMobilidadeVehicleBrands(c *gin.Context) {
	if services.MobilidadeCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.MobilidadeCatalogServiceInstance.ListBrandsAdmin(c.Request.Context())
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.VehicleBrandsResponse{Data: result})
}

// CreateMobilidadeVehicleBrand godoc
// @Summary Criar marca (admin)
// @Description Cadastra uma marca no catálogo. ID gerado no backend (brand_&lt;slug&gt;); is_other=false.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param body body models.VehicleBrandCreateRequest true "Nome da marca"
// @Security BearerAuth
// @Success 201 {object} models.VehicleBrand "Marca criada"
// @Failure 400 {object} ErrorResponse "Validação inválida"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 409 {object} ErrorResponse "Nome ou ID já existente"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /admin/mobilidade/vehicle-brands [post]
func CreateMobilidadeVehicleBrand(c *gin.Context) {
	var req models.VehicleBrandCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if services.MobilidadeCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.MobilidadeCatalogServiceInstance.CreateBrand(c.Request.Context(), &req)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

// UpdateMobilidadeVehicleBrand godoc
// @Summary Atualizar marca (admin)
// @Description Atualiza o nome de uma marca ativa. Sentinel brand_outro não pode ser alterada. 409 se referenciada por veículo.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param brand_id path string true "ID da marca"
// @Param body body models.VehicleBrandUpdateRequest true "Campos a atualizar"
// @Security BearerAuth
// @Success 200 {object} models.VehicleBrand "Marca atualizada"
// @Failure 400 {object} ErrorResponse "Validação inválida ou sentinel"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Marca não encontrada"
// @Failure 409 {object} ErrorResponse "Conflito (nome duplicado ou veículo referenciando)"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /admin/mobilidade/vehicle-brands/{brand_id} [patch]
func UpdateMobilidadeVehicleBrand(c *gin.Context) {
	var req models.VehicleBrandUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if services.MobilidadeCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.MobilidadeCatalogServiceInstance.UpdateBrand(c.Request.Context(), c.Param("brand_id"), &req)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteMobilidadeVehicleBrand godoc
// @Summary Excluir marca (admin, soft-delete)
// @Description Soft-delete de marca. 409 se houver modelos ativos ou veículos referenciando. Sentinel brand_outro não pode ser excluída.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param brand_id path string true "ID da marca"
// @Security BearerAuth
// @Success 204 "Marca excluída"
// @Failure 400 {object} ErrorResponse "Sentinel ou validação"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Marca não encontrada"
// @Failure 409 {object} ErrorResponse "Modelos ativos ou veículos referenciando"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /admin/mobilidade/vehicle-brands/{brand_id} [delete]
func DeleteMobilidadeVehicleBrand(c *gin.Context) {
	if services.MobilidadeCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	if err := services.MobilidadeCatalogServiceInstance.DeleteBrand(c.Request.Context(), c.Param("brand_id")); err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// AdminListMobilidadeVehicleModels godoc
// @Summary Listar modelos (admin)
// @Description Lista completa de modelos, incluindo soft-deleted. Filtro opcional por brand_id.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param brand_id query string false "Filtrar por ID da marca"
// @Security BearerAuth
// @Success 200 {object} models.VehicleModelsResponse "Lista tipada de modelos em data"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /admin/mobilidade/vehicle-models [get]
func AdminListMobilidadeVehicleModels(c *gin.Context) {
	if services.MobilidadeCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.MobilidadeCatalogServiceInstance.ListModelsAdmin(c.Request.Context(), c.Query("brand_id"))
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, models.VehicleModelsResponse{Data: result})
}

// CreateMobilidadeVehicleModel godoc
// @Summary Criar modelo (admin)
// @Description Cadastra um modelo vinculado a brand_id com vehicle_type. ID gerado no backend; is_other=false.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param body body models.VehicleModelCreateRequest true "Dados do modelo"
// @Security BearerAuth
// @Success 201 {object} models.VehicleModel "Modelo criado"
// @Failure 400 {object} ErrorResponse "Validação inválida"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Marca não encontrada"
// @Failure 409 {object} ErrorResponse "Nome ou ID já existente"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /admin/mobilidade/vehicle-models [post]
func CreateMobilidadeVehicleModel(c *gin.Context) {
	var req models.VehicleModelCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if services.MobilidadeCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.MobilidadeCatalogServiceInstance.CreateModel(c.Request.Context(), &req)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

// UpdateMobilidadeVehicleModel godoc
// @Summary Atualizar modelo (admin)
// @Description Atualiza nome, vehicle_type e/ou brand_id de um modelo ativo. Sentinel model_outro não pode ser alterado. 409 se referenciado por veículo.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param model_id path string true "ID do modelo"
// @Param body body models.VehicleModelUpdateRequest true "Campos a atualizar"
// @Security BearerAuth
// @Success 200 {object} models.VehicleModel "Modelo atualizado"
// @Failure 400 {object} ErrorResponse "Validação inválida ou sentinel"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Modelo ou marca não encontrado"
// @Failure 409 {object} ErrorResponse "Conflito (nome duplicado ou veículo referenciando)"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /admin/mobilidade/vehicle-models/{model_id} [patch]
func UpdateMobilidadeVehicleModel(c *gin.Context) {
	var req models.VehicleModelUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if services.MobilidadeCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.MobilidadeCatalogServiceInstance.UpdateModel(c.Request.Context(), c.Param("model_id"), &req)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteMobilidadeVehicleModel godoc
// @Summary Excluir modelo (admin, soft-delete)
// @Description Soft-delete de modelo. 409 se veículos referenciarem. Sentinel model_outro não pode ser excluído.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param model_id path string true "ID do modelo"
// @Security BearerAuth
// @Success 204 "Modelo excluído"
// @Failure 400 {object} ErrorResponse "Sentinel ou validação"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Modelo não encontrado"
// @Failure 409 {object} ErrorResponse "Veículos referenciando"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /admin/mobilidade/vehicle-models/{model_id} [delete]
func DeleteMobilidadeVehicleModel(c *gin.Context) {
	if services.MobilidadeCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	if err := services.MobilidadeCatalogServiceInstance.DeleteModel(c.Request.Context(), c.Param("model_id")); err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
