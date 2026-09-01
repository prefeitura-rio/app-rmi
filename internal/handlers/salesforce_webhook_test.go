package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/middleware"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/prefeitura-rio/app-rmi/internal/utils"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWebhookCPF = "52998224725"

// minimalJWT is a structurally valid JWT for AuthMiddleware claim extraction.
// payload: {"sub":"svc","preferred_username":"52998224725"}
const minimalJWT = "eyJhbGciOiJub25lIn0.eyJzdWIiOiJzdmMiLCJwcmVmZXJyZWRfdXNlcm5hbWUiOiI1Mjk5ODIyNDcyNSJ9.sig"

// salesforceWebhookJWT has azp=salesforce-rmi (allowed webhook client).
const salesforceWebhookJWT = "eyJhbGciOiJub25lIn0.eyJzdWIiOiJzZiIsImF6cCI6InNhbGVzZm9yY2Utcm1pIn0.sig"

// otherClientJWT has azp=superapp (not allowed for webhook).
const otherClientJWT = "eyJhbGciOiJub25lIn0.eyJzdWIiOiJ1c2VyIiwiYXpwIjoic3VwZXJhcHAiLCJwcmVmZXJyZWRfdXNlcm5hbWUiOiI1Mjk5ODIyNDcyNSJ9.sig"

func webhookPayload(cpf string) string {
	return `{"cpf":"` + cpf + `","evento":"atualizacao","dados":{"cpf":"` + cpf + `","email":"maria@test.com","genero":"Mulher_cisgenero","accountId":"001be00000XqUOzAAN"}}`
}

func salesforceWebhookRouter() *gin.Engine {
	r := gin.New()
	r.POST("/v1/webhooks/salesforce/cidadao",
		middleware.AuthMiddleware(),
		middleware.RequireSalesforceWebhookClient(),
		HandleSalesforceCidadaoWebhook,
	)
	return r
}

func TestHandleSalesforceCidadaoWebhook_NotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{SalesforceWebhookClients: []string{"salesforce-rmi"}}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(webhookPayload(testWebhookCPF)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_UnauthorizedWithoutJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(webhookPayload(testWebhookCPF)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_ForbiddenWrongAZP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(webhookPayload(testWebhookCPF)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+otherClientJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_WebhookClientsNotConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{SalesforceBaseURL: "https://sf.example.com"}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(webhookPayload(testWebhookCPF)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_InvalidCPF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(webhookPayload("123")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_MissingDados(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(`{"cpf":"`+testWebhookCPF+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_Success_UsesPayloadNoGET(t *testing.T) {
	gin.SetMode(gin.TestMode)

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	singleClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	redisClient := redisclient.NewClient(singleClient)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis unavailable: %v", err)
	}
	prevRedis := config.Redis
	config.SetRedis(redisClient)
	defer func() {
		config.SetRedis(prevRedis)
		_ = redisClient.Del(context.Background(), "sync:queue:"+services.SalesforceSyncQueue).Err()
	}()

	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(webhookPayload(testWebhookCPF)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)

	var body SalesforceWebhookResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "accepted", body.Status)
	assert.Equal(t, "salesforce sync job enqueued", body.Message)
	assert.Equal(t, utils.MaskCPFForWebhookResponse(testWebhookCPF), body.CPF)
	assert.Equal(t, services.SalesforceWebhookEventAtualizacao, body.Evento)

	n, err := redisClient.LLen(context.Background(), "sync:queue:"+services.SalesforceSyncQueue).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	rawJobs, err := redisClient.LRange(context.Background(), "sync:queue:"+services.SalesforceSyncQueue, 0, 0).Result()
	require.NoError(t, err)
	require.Len(t, rawJobs, 1)
	assert.Contains(t, rawJobs[0], "maria@test.com")
	assert.Contains(t, rawJobs[0], `"evento":"atualizacao"`)
	assert.Contains(t, rawJobs[0], `"origin":"salesforce"`)
}

func TestHandleSalesforceCidadaoWebhook_InvalidEvento(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao",
		bytes.NewBufferString(`{"cpf":"`+testWebhookCPF+`","evento":"invalido","dados":{"cpf":"`+testWebhookCPF+`"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_AnonimizacaoEvento(t *testing.T) {
	gin.SetMode(gin.TestMode)

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	singleClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	redisClient := redisclient.NewClient(singleClient)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis unavailable: %v", err)
	}
	prevRedis := config.Redis
	config.SetRedis(redisClient)
	defer func() {
		config.SetRedis(prevRedis)
		_ = redisClient.Del(context.Background(), "sync:queue:"+services.SalesforceSyncQueue).Err()
	}()

	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	payload := `{"cpf":"` + testWebhookCPF + `","evento":"anonimizacao","dados":{"cpf":"` + testWebhookCPF + `","nome":"ANONIMIZADO","email":""}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
	var body SalesforceWebhookResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, services.SalesforceWebhookEventAnonimizacao, body.Evento)

	rawJobs, err := redisClient.LRange(context.Background(), "sync:queue:"+services.SalesforceSyncQueue, 0, 0).Result()
	require.NoError(t, err)
	require.Len(t, rawJobs, 1)
	assert.Contains(t, rawJobs[0], `"evento":"anonimizacao"`)
}

func TestHandleSalesforceCidadaoWebhook_DadosNull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao",
		bytes.NewBufferString(`{"cpf":"`+testWebhookCPF+`","dados":null}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "dados must be a JSON object")
}

func TestHandleSalesforceCidadaoWebhook_DadosNotObject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao",
		bytes.NewBufferString(`{"cpf":"`+testWebhookCPF+`","dados":["x"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "dados must be a JSON object")
}

func TestHandleSalesforceCidadaoWebhook_DadosStringNotObject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao",
		bytes.NewBufferString(`{"cpf":"`+testWebhookCPF+`","dados":"not-an-object"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "dados must be a JSON object")
}

func TestHandleSalesforceCidadaoWebhook_MalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao",
		bytes.NewBufferString(`{"cpf":"`+testWebhookCPF+`","dados":`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_EmptyDadosObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	singleClient := redis.NewClient(&redis.Options{Addr: redisAddr})
	redisClient := redisclient.NewClient(singleClient)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis unavailable: %v", err)
	}
	prevRedis := config.Redis
	config.SetRedis(redisClient)
	defer func() {
		config.SetRedis(prevRedis)
		_ = redisClient.Del(context.Background(), "sync:queue:"+services.SalesforceSyncQueue).Err()
	}()

	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao",
		bytes.NewBufferString(`{"cpf":"`+testWebhookCPF+`","dados":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
}

func TestHandleSalesforceCidadaoWebhook_RedisFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	prevRedis := config.Redis
	config.SetRedis(nil)
	defer config.SetRedis(prevRedis)

	prev := config.AppConfig
	config.AppConfig = &config.Config{
		SalesforceBaseURL:        "https://sf.example.com",
		SalesforceWebhookClients: []string{"salesforce-rmi"},
	}
	defer func() { config.AppConfig = prev }()

	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(webhookPayload(testWebhookCPF)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "failed to enqueue sync job")
}

func TestExtractBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer abc.def.ghi")
	assert.Equal(t, "abc.def.ghi", extractBearerToken(c))

	c.Request.Header.Set("Authorization", "Basic x")
	assert.Equal(t, "", extractBearerToken(c))
}
