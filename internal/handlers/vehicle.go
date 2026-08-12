package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"go.uber.org/zap"
)

// GetVehicles godoc
// @Summary Listar veículos da carteira (mobilidade)
// @Description Retorna veículos em que o CPF é proprietário ou condutor aceito, com role em cada item. Quando role=conductor, inclui conductor_id (id do vínculo aceito).
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param page query int false "Número da página (padrão: 1)" minimum(1)
// @Param per_page query int false "Itens por página (padrão: 10, máximo: 100)" minimum(1) maximum(100)
// @Security BearerAuth
// @Success 200 {object} models.PaginatedVehicles "Lista paginada de veículos"
// @Failure 400 {object} ErrorResponse "CPF ou paginação inválidos"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicles [get]
func GetVehicles(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	page, perPage, err := services.ValidatePaginationParams(c.Query("page"), c.Query("per_page"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if services.VehicleServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Vehicle service unavailable"})
		return
	}

	result, err := services.VehicleServiceInstance.ListVehicles(c.Request.Context(), cpf, page, perPage)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetVehicle godoc
// @Summary Obter detalhe do veículo
// @Description Retorna o detalhe do veículo para proprietário ou condutor aceito (inclui invoice_photo_url, metadados de arquivos e registration_number). Quando role=conductor, inclui conductor_id (id do vínculo aceito). owner_name/owner_phone/owner_email são enriquecidos ao vivo via RMI a partir de owner_cpf.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param vehicle_id path string true "ID do veículo"
// @Security BearerAuth
// @Success 200 {object} models.VehicleDetail "Detalhe do veículo"
// @Failure 400 {object} ErrorResponse "CPF inválido"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Veículo não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicles/{vehicle_id} [get]
func GetVehicle(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	if services.VehicleServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Vehicle service unavailable"})
		return
	}

	result, err := services.VehicleServiceInstance.GetVehicle(c.Request.Context(), cpf, c.Param("vehicle_id"))
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// CreateVehicle godoc
// @Summary Cadastrar veículo
// @Description Cadastra um veículo para o CPF autenticado. Fluxo catálogo (brand_id+model_id) ou Outro (brand_other/model_other+vehicle_type). URLs de foto devem ser HTTPS GCS. Se has_invoice=true, invoice_photo_url é obrigatória. Contato do dono (nome/telefone/e-mail) é enriquecido ao vivo via RMI a partir do owner_cpf — não enviar owner_* no body. registration_number é gerado pelo backend (formato RJ-E-XXXXXX).
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param body body models.VehicleCreateRequest true "Dados do veículo"
// @Security BearerAuth
// @Success 201 {object} models.VehicleDetail "Veículo criado"
// @Failure 400 {object} ErrorResponse "Validação inválida"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicles [post]
func CreateVehicle(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var req models.VehicleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if services.VehicleServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Vehicle service unavailable"})
		return
	}

	result, err := services.VehicleServiceInstance.CreateVehicle(c.Request.Context(), cpf, &req)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

// UpdateVehicle godoc
// @Summary Atualizar veículo
// @Description Atualiza campos do veículo (somente proprietário). Em mudança de marca/modelo do catálogo, vehicle_type é rederivado do modelo. Para trocar para fluxo Outro (texto livre), envie brand_id/model_id como null (ou "") junto com brand_other/model_other e vehicle_type; campo omitido não altera o valor atual.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param vehicle_id path string true "ID do veículo"
// @Param body body models.VehicleUpdateRequest true "Campos a atualizar"
// @Security BearerAuth
// @Success 200 {object} models.VehicleDetail "Veículo atualizado"
// @Failure 400 {object} ErrorResponse "Validação inválida"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado (não é o proprietário)"
// @Failure 404 {object} ErrorResponse "Veículo não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicles/{vehicle_id} [patch]
func UpdateVehicle(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var req models.VehicleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request: " + err.Error()})
		return
	}

	if services.VehicleServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Vehicle service unavailable"})
		return
	}

	result, err := services.VehicleServiceInstance.UpdateVehicle(c.Request.Context(), cpf, c.Param("vehicle_id"), &req)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteVehicle godoc
// @Summary Excluir veículo (soft delete)
// @Description Soft-delete do veículo e revoga vínculos pending/accepted (somente proprietário).
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param vehicle_id path string true "ID do veículo"
// @Security BearerAuth
// @Success 204 "Veículo excluído"
// @Failure 400 {object} ErrorResponse "CPF inválido"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Veículo não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicles/{vehicle_id} [delete]
func DeleteVehicle(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	if services.VehicleServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Vehicle service unavailable"})
		return
	}

	if err := services.VehicleServiceInstance.DeleteVehicle(c.Request.Context(), cpf, c.Param("vehicle_id")); err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GetVehicleInvitations godoc
// @Summary Listar convites pendentes de veículo
// @Description Lista convites pending em que o CPF é o convidado.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Security BearerAuth
// @Success 200 {object} models.VehicleInvitationsResponse "Convites pendentes"
// @Failure 400 {object} ErrorResponse "CPF inválido"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicle-invitations [get]
func GetVehicleInvitations(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	if services.VehicleConductorServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Conductor service unavailable"})
		return
	}

	result, err := services.VehicleConductorServiceInstance.ListInvitations(c.Request.Context(), cpf)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// RespondVehicleInvitation godoc
// @Summary Aceitar ou recusar convite de veículo
// @Description Atualiza status do convite para accepted ou rejected (somente o convidado).
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param conductor_id path string true "ID do vínculo/convite"
// @Param body body models.RespondInvitationRequest true "Novo status"
// @Security BearerAuth
// @Success 200 {object} models.VehicleConductor "Convite respondido"
// @Failure 400 {object} ErrorResponse "Validação inválida"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Convite não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicle-invitations/{conductor_id} [patch]
func RespondVehicleInvitation(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var req models.RespondInvitationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request: " + err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if services.VehicleConductorServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Conductor service unavailable"})
		return
	}

	result, err := services.VehicleConductorServiceInstance.RespondInvitation(c.Request.Context(), cpf, c.Param("conductor_id"), &req)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetVehicleConductors godoc
// @Summary Listar condutores do veículo
// @Description Lista vínculos pending e accepted (somente proprietário). Pending devolve snapshot do convite; accepted enriquece nome/e-mail/celular ao vivo via RMI.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param vehicle_id path string true "ID do veículo"
// @Security BearerAuth
// @Success 200 {object} models.ConductorsListResponse "Lista de condutores"
// @Failure 400 {object} ErrorResponse "CPF inválido"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Veículo não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicles/{vehicle_id}/conductors [get]
func GetVehicleConductors(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	if services.VehicleConductorServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Conductor service unavailable"})
		return
	}

	result, err := services.VehicleConductorServiceInstance.ListConductors(c.Request.Context(), cpf, c.Param("vehicle_id"))
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// InviteVehicleConductor godoc
// @Summary Convidar condutor
// @Description Cria vínculo pending e enfileira e-mail de convite via notify_email do body (somente proprietário). name/phone são hints opcionais enquanto pending. Duplicata pending/accepted retorna 409.
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param vehicle_id path string true "ID do veículo"
// @Param body body models.InviteConductorRequest true "CPF + e-mail do convidado (name/phone opcionais)"
// @Security BearerAuth
// @Success 201 {object} models.VehicleConductor "Convite criado"
// @Failure 400 {object} ErrorResponse "Validação inválida"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Veículo não encontrado"
// @Failure 409 {object} ErrorResponse "Já existe vínculo pending/accepted"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicles/{vehicle_id}/conductors [post]
func InviteVehicleConductor(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	var req models.InviteConductorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request: " + err.Error()})
		return
	}

	if services.VehicleConductorServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Conductor service unavailable"})
		return
	}

	result, err := services.VehicleConductorServiceInstance.InviteConductor(c.Request.Context(), cpf, c.Param("vehicle_id"), &req)
	if err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

// RemoveVehicleConductor godoc
// @Summary Remover / revogar condutor
// @Description Proprietário revoga pending/accepted; condutor aceito pode sair (self-leave).
// @Tags mobilidade
// @Accept json
// @Produce json
// @Param cpf path string true "CPF do cidadão (11 dígitos)" minLength(11) maxLength(11)
// @Param vehicle_id path string true "ID do veículo"
// @Param conductor_id path string true "ID do vínculo"
// @Security BearerAuth
// @Success 204 "Vínculo revogado"
// @Failure 400 {object} ErrorResponse "Validação inválida"
// @Failure 401 {object} ErrorResponse "Não autenticado"
// @Failure 403 {object} ErrorResponse "Acesso negado"
// @Failure 404 {object} ErrorResponse "Não encontrado"
// @Failure 500 {object} ErrorResponse "Erro interno"
// @Router /citizen/{cpf}/vehicles/{vehicle_id}/conductors/{conductor_id} [delete]
func RemoveVehicleConductor(c *gin.Context) {
	cpf := c.Param("cpf")
	if !utils.ValidateCPF(cpf) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid CPF format"})
		return
	}

	if services.VehicleConductorServiceInstance == nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Conductor service unavailable"})
		return
	}

	if err := services.VehicleConductorServiceInstance.RemoveConductor(c.Request.Context(), cpf, c.Param("vehicle_id"), c.Param("conductor_id")); err != nil {
		mapMobilidadeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func mapMobilidadeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrVehicleNotFound), errors.Is(err, services.ErrConductorNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, services.ErrCatalogBrandNotFound), errors.Is(err, services.ErrCatalogModelNotFound):
		c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	case errors.Is(err, services.ErrMobilidadeForbidden):
		c.JSON(http.StatusForbidden, ErrorResponse{Error: err.Error()})
	case errors.Is(err, services.ErrMobilidadeConflict):
		c.JSON(http.StatusConflict, ErrorResponse{Error: err.Error()})
	case errors.Is(err, services.ErrMobilidadeInvalidInput):
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	case errors.Is(err, services.ErrMobilidadeNotImplemented):
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	default:
		logging.GetLogger().Error("mobilidade handler unexpected error",
			zap.Error(err),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
		)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal error"})
	}
}
