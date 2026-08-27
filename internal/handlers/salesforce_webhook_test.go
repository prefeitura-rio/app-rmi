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
	return `{"cpf":"` + cpf + `","dados":{"email":"maria@test.com","genero":"Mulher_cisgenero","accountId":"001be00000XqUOzAAN"}}`
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
	assert.Equal(t, testWebhookCPF, body.CPF)

	n, err := redisClient.LLen(context.Background(), "sync:queue:"+services.SalesforceSyncQueue).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	rawJobs, err := redisClient.LRange(context.Background(), "sync:queue:"+services.SalesforceSyncQueue, 0, 0).Result()
	require.NoError(t, err)
	require.Len(t, rawJobs, 1)
	assert.Contains(t, rawJobs[0], "maria@test.com")
	assert.Contains(t, rawJobs[0], `"origin":"salesforce"`)
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
