package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/services"
)

// GetMobilidadeVehicleBrands godoc
// @Summary Listar marcas de veículos
// @Description Catálogo de marcas para o formulário de mobilidade (seed Mongo).
// @Tags mobilidade
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Lista de marcas em data"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /mobilidade/vehicle-brands [get]
func GetMobilidadeVehicleBrands(c *gin.Context) {
	if services.RioMobCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.RioMobCatalogServiceInstance.ListBrands(c.Request.Context())
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetMobilidadeVehicleModels godoc
// @Summary Listar modelos de veículos por marca
// @Description Modelos do catálogo filtrados por brand_id (obrigatório), incluindo vehicle_type.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param brand_id query string true "ID da marca"
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Lista de modelos em data"
// @Failure 400 {object} ErrorResponse "brand_id ausente"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /mobilidade/vehicle-models [get]
func GetMobilidadeVehicleModels(c *gin.Context) {
	brandID := c.Query("brand_id")
	if brandID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "brand_id is required"})
		return
	}
	if services.RioMobCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.RioMobCatalogServiceInstance.ListModelsByBrand(c.Request.Context(), brandID)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

// GetMobilidadeVehicleColors godoc
// @Summary Listar cores de veículos
// @Description Lista fixa de cores permitidas no formulário de mobilidade.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{} "Lista de cores em data"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /mobilidade/vehicle-colors [get]
func GetMobilidadeVehicleColors(c *gin.Context) {
	if services.RioMobCatalogServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Catalog service unavailable"})
		return
	}
	result, err := services.RioMobCatalogServiceInstance.ListColors(c.Request.Context())
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
