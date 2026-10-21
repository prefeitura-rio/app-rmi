package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

const addressOutdatedTestCPF = "10000000019"

func setupAddressUpdateRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/v1/citizen/:cpf/address", UpdateSelfDeclaredAddress)
	return r
}

func cleanupAddressOutdatedTest(ctx context.Context, cpf string) {
	_, _ = config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).DeleteOne(ctx, bson.M{"cpf": cpf})
	_, _ = config.MongoDB.Collection(config.AppConfig.CFLookupCollection).DeleteMany(ctx, bson.M{"cpf": cpf})
	_ = config.Redis.Del(ctx,
		"citizen:"+cpf,
		"citizen:cache:"+cpf,
		"citizen:write:"+cpf,
		"self_declared_address:write:"+cpf,
		"self_declared_address:cache:"+cpf,
		"self_declared:write:"+cpf,
		"self_declared:cache:"+cpf,
		services.CFLookupPendingKey(cpf),
	).Err()
}

func sampleAddressFields() (bairro, cep, estado, logradouro, municipio, numero string, complemento, tipo *string) {
	comp := "Apt 101"
	tipoLog := "Rua"
	return "Centro", "20040020", "RJ", "Rua da Assembleia", "Rio de Janeiro", "10", &comp, &tipoLog
}

func seedSelfDeclaredAddress(t *testing.T, ctx context.Context, cpf string, principalUpdatedAt *time.Time, documentUpdatedAt time.Time) {
	t.Helper()
	bairro, cep, estado, logradouro, municipio, numero, complemento, tipo := sampleAddressFields()
	origem := "self-declared"
	sistema := "rmi"
	doc := models.SelfDeclaredData{
		CPF: cpf,
		Endereco: &models.Endereco{
			Indicador: utils.BoolPtr(true),
			Principal: &models.EnderecoPrincipal{
				Bairro:         &bairro,
				CEP:            &cep,
				Complemento:    complemento,
				Estado:         &estado,
				Logradouro:     &logradouro,
				Municipio:      &municipio,
				Numero:         &numero,
				TipoLogradouro: tipo,
				Origem:         &origem,
				Sistema:        &sistema,
				UpdatedAt:      principalUpdatedAt,
			},
		},
		UpdatedAt: documentUpdatedAt,
	}
	_, err := config.MongoDB.Collection(config.AppConfig.SelfDeclaredCollection).InsertOne(ctx, doc)
	require.NoError(t, err)
}

func addressPayloadJSON(t *testing.T) []byte {
	t.Helper()
	bairro, cep, estado, logradouro, municipio, numero, complemento, tipo := sampleAddressFields()
	body, err := json.Marshal(map[string]interface{}{
		"bairro":          bairro,
		"cep":             cep,
		"complemento":     *complemento,
		"estado":          estado,
		"logradouro":      logradouro,
		"municipio":       municipio,
		"numero":          numero,
		"tipo_logradouro": *tipo,
	})
	require.NoError(t, err)
	return body
}

func putAddress(t *testing.T, r *gin.Engine, cpf string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "/v1/citizen/"+cpf+"/address", bytes.NewBuffer(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func readAddressFromWriteBuffer(t *testing.T, ctx context.Context, cpf string) *models.Endereco {
	t.Helper()
	raw, err := config.Redis.Get(ctx, "self_declared_address:write:"+cpf).Result()
	require.NoError(t, err)

	var payload struct {
		Endereco *models.Endereco `json:"endereco"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &payload))
	require.NotNil(t, payload.Endereco)
	require.NotNil(t, payload.Endereco.Principal)
	return payload.Endereco
}

func TestUpdateSelfDeclaredAddress_SameAddressRecent_Returns409(t *testing.T) {
	ctx := context.Background()
	cpf := addressOutdatedTestCPF
	cleanupAddressOutdatedTest(ctx, cpf)
	t.Cleanup(func() { cleanupAddressOutdatedTest(ctx, cpf) })

	originalThreshold := config.AppConfig.SelfDeclaredOutdatedThreshold
	config.AppConfig.SelfDeclaredOutdatedThreshold = 180 * 24 * time.Hour
	t.Cleanup(func() { config.AppConfig.SelfDeclaredOutdatedThreshold = originalThreshold })

	recent := time.Now().Add(-24 * time.Hour)
	seedSelfDeclaredAddress(t, ctx, cpf, &recent, recent)

	w := putAddress(t, setupAddressUpdateRouter(), cpf, addressPayloadJSON(t))
	assert.Equal(t, http.StatusConflict, w.Code)

	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	assert.Equal(t, "No change: address matches current data", errResp.Error)

	exists := config.Redis.Exists(ctx, "self_declared_address:write:"+cpf).Val()
	assert.Equal(t, int64(0), exists, "should not write when returning 409")
}

func TestUpdateSelfDeclaredAddress_SameAddressOutdated_RenewsDateWithoutCF(t *testing.T) {
	ctx := context.Background()
	cpf := addressOutdatedTestCPF
	cleanupAddressOutdatedTest(ctx, cpf)
	t.Cleanup(func() { cleanupAddressOutdatedTest(ctx, cpf) })

	originalThreshold := config.AppConfig.SelfDeclaredOutdatedThreshold
	config.AppConfig.SelfDeclaredOutdatedThreshold = 180 * 24 * time.Hour
	t.Cleanup(func() { config.AppConfig.SelfDeclaredOutdatedThreshold = originalThreshold })

	outdated := time.Now().Add(-181 * 24 * time.Hour)
	seedSelfDeclaredAddress(t, ctx, cpf, &outdated, outdated)

	prevCF := services.CFLookupServiceInstance
	services.CFLookupServiceInstance = services.NewCFLookupService(config.MongoDB, nil, &logging.SafeLogger{})
	t.Cleanup(func() { services.CFLookupServiceInstance = prevCF })

	queueKey := "sync:queue:cf_lookup"
	require.NoError(t, config.Redis.Del(ctx, queueKey).Err())
	require.NoError(t, config.Redis.Del(ctx, services.CFLookupPendingKey(cpf)).Err())

	before := time.Now()
	w := putAddress(t, setupAddressUpdateRouter(), cpf, addressPayloadJSON(t))
	assert.Equal(t, http.StatusOK, w.Code)

	var okResp SuccessResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &okResp))
	assert.Equal(t, "Self-declared address updated successfully", okResp.Message)

	endereco := readAddressFromWriteBuffer(t, ctx, cpf)
	require.NotNil(t, endereco.Principal.UpdatedAt)
	assert.True(t, endereco.Principal.UpdatedAt.After(before) || endereco.Principal.UpdatedAt.Equal(before))
	assert.True(t, endereco.Principal.UpdatedAt.After(outdated))

	bairro, _, _, logradouro, _, numero, _, _ := sampleAddressFields()
	assert.Equal(t, bairro, *endereco.Principal.Bairro)
	assert.Equal(t, logradouro, *endereco.Principal.Logradouro)
	assert.Equal(t, numero, *endereco.Principal.Numero)

	queueLen, err := config.Redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), queueLen, "date-only renewal must not enqueue CF lookup")
	assert.Equal(t, int64(0), config.Redis.Exists(ctx, services.CFLookupPendingKey(cpf)).Val())
}

func TestUpdateSelfDeclaredAddress_SameAddressMissingUpdatedAt_RenewsDate(t *testing.T) {
	ctx := context.Background()
	cpf := addressOutdatedTestCPF
	cleanupAddressOutdatedTest(ctx, cpf)
	t.Cleanup(func() { cleanupAddressOutdatedTest(ctx, cpf) })

	originalThreshold := config.AppConfig.SelfDeclaredOutdatedThreshold
	config.AppConfig.SelfDeclaredOutdatedThreshold = 180 * 24 * time.Hour
	t.Cleanup(func() { config.AppConfig.SelfDeclaredOutdatedThreshold = originalThreshold })

	seedSelfDeclaredAddress(t, ctx, cpf, nil, time.Now())

	w := putAddress(t, setupAddressUpdateRouter(), cpf, addressPayloadJSON(t))
	assert.Equal(t, http.StatusOK, w.Code)

	endereco := readAddressFromWriteBuffer(t, ctx, cpf)
	require.NotNil(t, endereco.Principal.UpdatedAt)
	assert.WithinDuration(t, time.Now(), *endereco.Principal.UpdatedAt, 5*time.Second)
}

func TestUpdateSelfDeclaredAddress_UsesPrincipalUpdatedAtNotDocument(t *testing.T) {
	ctx := context.Background()
	cpf := addressOutdatedTestCPF
	cleanupAddressOutdatedTest(ctx, cpf)
	t.Cleanup(func() { cleanupAddressOutdatedTest(ctx, cpf) })

	originalThreshold := config.AppConfig.SelfDeclaredOutdatedThreshold
	config.AppConfig.SelfDeclaredOutdatedThreshold = 180 * 24 * time.Hour
	t.Cleanup(func() { config.AppConfig.SelfDeclaredOutdatedThreshold = originalThreshold })

	// Address itself is outdated, but another self-declared field recently bumped document updated_at.
	outdatedAddress := time.Now().Add(-200 * 24 * time.Hour)
	recentDocument := time.Now().Add(-1 * time.Hour)
	seedSelfDeclaredAddress(t, ctx, cpf, &outdatedAddress, recentDocument)

	w := putAddress(t, setupAddressUpdateRouter(), cpf, addressPayloadJSON(t))
	assert.Equal(t, http.StatusOK, w.Code, "must use endereco.principal.updated_at, not document updated_at")
}

func TestUpdateSelfDeclaredAddress_DifferentAddress_UpdatesAndQueuesCF(t *testing.T) {
	ctx := context.Background()
	cpf := addressOutdatedTestCPF
	cleanupAddressOutdatedTest(ctx, cpf)
	t.Cleanup(func() { cleanupAddressOutdatedTest(ctx, cpf) })

	originalThreshold := config.AppConfig.SelfDeclaredOutdatedThreshold
	config.AppConfig.SelfDeclaredOutdatedThreshold = 180 * 24 * time.Hour
	t.Cleanup(func() { config.AppConfig.SelfDeclaredOutdatedThreshold = originalThreshold })

	recent := time.Now().Add(-24 * time.Hour)
	seedSelfDeclaredAddress(t, ctx, cpf, &recent, recent)

	prevCF := services.CFLookupServiceInstance
	services.CFLookupServiceInstance = services.NewCFLookupService(config.MongoDB, nil, logging.GetLogger())
	t.Cleanup(func() { services.CFLookupServiceInstance = prevCF })

	queueKey := "sync:queue:cf_lookup"
	require.NoError(t, config.Redis.Del(ctx, queueKey).Err())
	require.NoError(t, config.Redis.Del(ctx, services.CFLookupPendingKey(cpf)).Err())

	body, err := json.Marshal(map[string]interface{}{
		"bairro":          "Copacabana",
		"cep":             "22041080",
		"complemento":     "Bloco B",
		"estado":          "RJ",
		"logradouro":      "Avenida Atlantica",
		"municipio":       "Rio de Janeiro",
		"numero":          "500",
		"tipo_logradouro": "Avenida",
	})
	require.NoError(t, err)

	w := putAddress(t, setupAddressUpdateRouter(), cpf, body)
	assert.Equal(t, http.StatusOK, w.Code)

	endereco := readAddressFromWriteBuffer(t, ctx, cpf)
	assert.Equal(t, "Copacabana", *endereco.Principal.Bairro)
	assert.Equal(t, "500", *endereco.Principal.Numero)

	queueLen, err := config.Redis.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), queueLen, "content change should enqueue CF lookup")

	jobJSON, err := config.Redis.RPop(ctx, queueKey).Result()
	require.NoError(t, err)
	var job services.SyncJob
	require.NoError(t, json.Unmarshal([]byte(jobJSON), &job))
	assert.Equal(t, "cf_lookup", job.Type)
	assert.Equal(t, cpf, job.Key)
}
