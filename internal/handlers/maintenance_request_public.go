package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
)

const maxChamadosPerProtocolo = 50

var protocoloSGRCPattern = regexp.MustCompile(`^RIO-[0-9]{8}-[0-9]$`)

var (
	maintenanceRequestVerifierMu sync.RWMutex
	maintenanceRequestVerifier   *middleware.JWTVerifier
)

// SetMaintenanceRequestJWTVerifier sets the verifier that unlocks the owner view of the public
// lookup by protocol. nil keeps every caller on the public view.
func SetMaintenanceRequestJWTVerifier(v *middleware.JWTVerifier) {
	maintenanceRequestVerifierMu.Lock()
	defer maintenanceRequestVerifierMu.Unlock()
	maintenanceRequestVerifier = v
}

func getMaintenanceRequestJWTVerifier() *middleware.JWTVerifier {
	maintenanceRequestVerifierMu.RLock()
	defer maintenanceRequestVerifierMu.RUnlock()
	return maintenanceRequestVerifier
}

// NormalizeProtocoloSGRC upper-cases and trims a SGRC protocol and reports whether it has the
// RIO-12345678-9 format.
func NormalizeProtocoloSGRC(raw string) (string, bool) {
	protocolo := strings.ToUpper(strings.TrimSpace(raw))
	return protocolo, protocoloSGRCPattern.MatchString(protocolo)
}

// GetMaintenanceRequestByProtocolo godoc
// @Summary Consultar chamado do 1746 (SGRC) por protocolo
// @Description Consulta pública dos chamados históricos do 1746/SGRC de um protocolo (um protocolo pode ter mais de um chamado; cada chamado expõe protocolos[]). Sem autenticação, ou com o JWT de outra pessoa, a resposta traz categoria, tipo, status, datas, bairro, andamentos públicos e loaded_at, e nunca CPF, descrição, logradouro nem número. Com um JWT do Keycloak de assinatura válida cujo preferred_username é o CPF do chamado, o chamado vem com titular = true, logradouro e descrição. Token inválido, expirado ou não verificável é tratado como anônimo. Ordenação obrigatória por data_inicio DESC.
// @Tags maintenance-request
// @Produce json
// @Param protocolo path string true "Protocolo do SGRC (formato RIO-12345678-9)"
// @Param Authorization header string false "Bearer <JWT> opcional: libera logradouro e descrição para o titular"
// @Success 200 {object} models.PublicMaintenanceRequestResponse "Chamados do protocolo"
// @Failure 400 {object} ErrorResponse "Protocolo em formato inválido"
// @Failure 404 {object} ErrorResponse "Nenhum chamado com esse protocolo"
// @Failure 500 {object} ErrorResponse "Erro interno do servidor"
// @Router /maintenance-request/{protocolo} [get]
func GetMaintenanceRequestByProtocolo(c *gin.Context) {
	ctx, span := otel.Tracer("").Start(c.Request.Context(), "GetMaintenanceRequestByProtocolo")
	defer span.End()
	logger := observability.Logger()

	protocolo, ok := NormalizeProtocoloSGRC(c.Param("protocolo"))
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid protocolo format (expected RIO-12345678-9)"})
		return
	}
	source := config.AppConfig.ActiveMaintenanceRequestSource()
	span.SetAttributes(
		attribute.String("operation", "get_maintenance_request_by_protocolo"),
		attribute.String("maintenance_request.source", source),
	)

	docs, err := findMaintenanceRequestsByProtocolo(ctx, protocolo)
	if err != nil {
		observability.DatabaseOperations.WithLabelValues("find", "error").Inc()
		logger.Error("failed to get maintenance requests by protocolo", zap.Error(err))
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "internal server error"})
		return
	}
	if len(docs) == 0 {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "maintenance request not found"})
		return
	}

	authHeader := c.GetHeader("Authorization")
	ownerCPF, verifyReason := verifiedCallerCPF(ctx, authHeader)
	span.SetAttributes(
		attribute.Bool("caller.verified", ownerCPF != ""),
		attribute.Bool("caller.authorization_present", middleware.BearerToken(authHeader) != ""),
		attribute.String("caller.verify_reason", verifyReason),
	)
	if ownerCPF == "" && middleware.BearerToken(authHeader) != "" {
		// Bearer was sent but the owner view stayed locked — visible at info so staging
		// can tell JWT_ISSUER_URL / JWKS / gateway issues apart from a true anonymous call.
		logger.Info("maintenance request owner view not unlocked",
			zap.String("verify_reason", verifyReason),
			zap.String("maintenance_request_source", source),
		)
	}

	response := models.PublicMaintenanceRequestResponse{
		Protocolo: protocolo,
		Chamados:  make([]models.PublicMaintenanceRequest, 0, len(docs)),
	}
	for i := range docs {
		response.Chamados = append(response.Chamados, docs[i].ToPublicMaintenanceRequest(ownerCPF))
	}

	// Homologation requires private,no-store so a titular response is never reused by another
	// browser/CDN user. Gateways may append no-cache/must-revalidate; keep private explicit.
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, response)
}

// verifiedCallerCPF returns the preferred_username of a bearer token whose signature, issuer
// and expiry were verified against the Keycloak JWKS, or "" when there is no token, no
// verifier configured, or the token does not verify. Never decode the token without verifying:
// on a public gateway action nothing upstream checked it.
//
// reason is a stable label for logs/spans (no token material): no_bearer, verifier_disabled,
// verify_failed, ok.
func verifiedCallerCPF(ctx context.Context, authHeader string) (cpf string, reason string) {
	token := middleware.BearerToken(authHeader)
	if token == "" {
		return "", "no_bearer"
	}
	verifier := getMaintenanceRequestJWTVerifier()
	if verifier == nil {
		observability.Logger().Warn("maintenance request bearer ignored: JWT verifier disabled (JWT_ISSUER_URL empty or unset)")
		return "", "verifier_disabled"
	}
	cpf, err := verifier.VerifiedPreferredUsername(ctx, token)
	if err != nil {
		observability.Logger().Warn("maintenance request bearer not verified",
			zap.Error(err),
			zap.String("verify_reason", "verify_failed"),
		)
		return "", "verify_failed"
	}
	return cpf, "ok"
}

func findMaintenanceRequestsByProtocolo(ctx context.Context, protocolo string) ([]models.MaintenanceRequestProtocolDocument, error) {
	source := config.AppConfig.ActiveMaintenanceRequestSource()
	collectionName := config.AppConfig.ActiveMaintenanceRequestCollection()
	cacheKey := fmt.Sprintf("maintenance_request:%s:protocolo:%s", source, protocolo)
	if cached, err := config.Redis.Get(ctx, cacheKey).Result(); err == nil {
		var docs []models.MaintenanceRequestProtocolDocument
		if err := json.Unmarshal([]byte(cached), &docs); err == nil {
			observability.CacheHits.WithLabelValues("get_maintenance_request_by_protocolo").Inc()
			return docs, nil
		}
	}

	filter := bson.M{"protocolo": protocolo}
	if config.AppConfig.MaintenanceRequestSGRCEnabled {
		// SGRC: protocolos is RECORD REPEATED {codigo_protocolo, cpf}
		filter = bson.M{"protocolos.codigo_protocolo": protocolo}
	}
	opts := options.Find().
		SetLimit(maxChamadosPerProtocolo).
		SetSort(bson.D{{Key: "data_inicio", Value: -1}})
	cursor, err := config.MongoDB.Collection(collectionName).
		Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var docs []models.MaintenanceRequestProtocolDocument
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	observability.DatabaseOperations.WithLabelValues("find", "success").Inc()

	if len(docs) > 0 {
		if data, err := json.Marshal(docs); err == nil {
			config.Redis.Set(ctx, cacheKey, data, config.AppConfig.RedisTTL)
		}
	}
	return docs, nil
}
