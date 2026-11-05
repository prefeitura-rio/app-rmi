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
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// flowsCPF is a synthetic, check-digit-valid CPF reserved for these tests.
const flowsCPF = "86288366757"

// citizenFlowsRedisDB isolates the write buffer and the sync queues of these tests from
// Redis DB 0, where the services package (running in parallel under go test ./...)
// deletes user_config:* and sync:queue:* keys between its tests.
const citizenFlowsRedisDB = 12

// resetCitizenFlowState points config.Redis at an empty database of its own and removes
// the Mongo documents of flowsCPF, so the tests start from "citizen never seen" and leave
// nothing behind.
func resetCitizenFlowState(t *testing.T) {
	t.Helper()
	if config.MongoDB == nil || config.Redis == nil {
		t.Skip("MongoDB/Redis not initialized")
	}
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rc := redisclient.NewClient(redis.NewClient(&redis.Options{Addr: addr, Password: os.Getenv("REDIS_PASSWORD"), DB: citizenFlowsRedisDB}))
	require.NoError(t, rc.Ping(context.Background()).Err())
	prevRedis := config.Redis
	config.SetRedis(rc)

	clean := func() {
		ctx := context.Background()
		_ = rc.FlushDB(ctx).Err()
		for _, coll := range []string{config.AppConfig.UserConfigCollection, config.AppConfig.CitizenCollection, config.AppConfig.SelfDeclaredCollection} {
			_, _ = config.MongoDB.Collection(coll).DeleteMany(ctx, bson.M{"cpf": flowsCPF})
		}
	}
	clean()
	t.Cleanup(func() {
		clean()
		config.SetRedis(prevRedis)
	})
}

func citizenFlowsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/v1/citizen/:cpf/optin", GetOptIn)
	r.PUT("/v1/citizen/:cpf/optin", UpdateOptIn)
	r.GET("/v1/citizen/:cpf/firstlogin", GetFirstLogin)
	r.PUT("/v1/citizen/:cpf/firstlogin", UpdateFirstLogin)
	r.GET("/v1/citizen/:cpf/wallet", GetCitizenWallet)
	return r
}

func serveJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeOptIn(t *testing.T, w *httptest.ResponseRecorder) bool {
	t.Helper()
	var resp models.UserConfigOptInResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.OptIn)
	return *resp.OptIn
}

func TestOptInFlow_DefaultThenOptOutThenOptInAgain(t *testing.T) {
	resetCitizenFlowState(t)
	r := citizenFlowsRouter()
	path := "/v1/citizen/" + flowsCPF + "/optin"

	// A citizen without configuration is opted in by default.
	w := serveJSON(t, r, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, decodeOptIn(t, w))

	// Opt out.
	w = serveJSON(t, r, http.MethodPut, path, `{"opt_in": false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, decodeOptIn(t, w))

	w = serveJSON(t, r, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, decodeOptIn(t, w), "the opt-out is read back before the sync worker persists it")

	// The change is queued for MongoDB persistence by the sync worker.
	queued, err := config.Redis.LRange(context.Background(), "sync:queue:user_config", 0, -1).Result()
	require.NoError(t, err)
	found := false
	for _, raw := range queued {
		if bytes.Contains([]byte(raw), []byte(flowsCPF)) {
			found = true
			break
		}
	}
	assert.True(t, found, "opt-in change must be enqueued for the sync worker")

	// Opt in again.
	w = serveJSON(t, r, http.MethodPut, path, `{"opt_in": true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = serveJSON(t, r, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, decodeOptIn(t, w))
}

func TestOptIn_ReadsPersistedConfigWithCategories(t *testing.T) {
	resetCitizenFlowState(t)
	_, err := config.MongoDB.Collection(config.AppConfig.UserConfigCollection).InsertOne(context.Background(), models.UserConfig{
		CPF:            flowsCPF,
		OptIn:          false,
		CategoryOptIns: map[string]bool{"saude": true, "eventos": false},
	})
	require.NoError(t, err)

	w := serveJSON(t, citizenFlowsRouter(), http.MethodGet, "/v1/citizen/"+flowsCPF+"/optin", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.UserConfigOptInResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.OptIn)
	assert.False(t, *resp.OptIn)
	assert.Equal(t, map[string]bool{"saude": true, "eventos": false}, resp.CategoryOptIns)
}

func TestOptIn_RejectsInvalidInput(t *testing.T) {
	resetCitizenFlowState(t)
	r := citizenFlowsRouter()

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"GET with invalid CPF", http.MethodGet, "/v1/citizen/12345678900/optin", ""},
		{"PUT with invalid CPF", http.MethodPut, "/v1/citizen/12345678900/optin", `{"opt_in": true}`},
		{"PUT without opt_in", http.MethodPut, "/v1/citizen/" + flowsCPF + "/optin", `{"OptIn": true}`},
		{"PUT with opt_in of the wrong type", http.MethodPut, "/v1/citizen/" + flowsCPF + "/optin", `{"opt_in": "sim"}`},
		{"PUT with malformed JSON", http.MethodPut, "/v1/citizen/" + flowsCPF + "/optin", `{"opt_in":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serveJSON(t, r, tc.method, tc.path, tc.body)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}

	keys, err := config.Redis.Keys(context.Background(), "user_config:*"+flowsCPF+"*").Result()
	require.NoError(t, err)
	assert.Empty(t, keys, "rejected requests must not write anything")
}

func TestFirstLoginFlow(t *testing.T) {
	resetCitizenFlowState(t)
	r := citizenFlowsRouter()
	path := "/v1/citizen/" + flowsCPF + "/firstlogin"

	decode := func(w *httptest.ResponseRecorder) bool {
		var resp models.UserConfigResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.FirstLogin
	}

	w := serveJSON(t, r, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.True(t, decode(w), "a citizen without configuration is on the first login")

	w = serveJSON(t, r, http.MethodPut, path, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, decode(w))

	w = serveJSON(t, r, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, decode(w), "first login is cleared after the update")

	assert.Equal(t, http.StatusBadRequest, serveJSON(t, r, http.MethodGet, "/v1/citizen/abc/firstlogin", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveJSON(t, r, http.MethodPut, "/v1/citizen/abc/firstlogin", "").Code)
}

func TestGetCitizenWallet_Flows(t *testing.T) {
	resetCitizenFlowState(t)
	r := citizenFlowsRouter()
	path := "/v1/citizen/" + flowsCPF + "/wallet"

	t.Run("invalid CPF", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, serveJSON(t, r, http.MethodGet, "/v1/citizen/11111111111/wallet", "").Code)
	})

	t.Run("unknown citizen", func(t *testing.T) {
		w := serveJSON(t, r, http.MethodGet, path, "")
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})

	t.Run("wallet with clinica da familia from the data lake", func(t *testing.T) {
		indicador := true
		nomeCF := "CF Teste"
		_, err := config.MongoDB.Collection(config.AppConfig.CitizenCollection).InsertOne(context.Background(), models.Citizen{
			CPF: flowsCPF,
			Saude: &models.Saude{
				ClinicaFamilia: &models.ClinicaFamilia{Indicador: &indicador, Nome: &nomeCF},
			},
		})
		require.NoError(t, err)

		w := serveJSON(t, r, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var wallet models.CitizenWallet
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &wallet))
		assert.Equal(t, flowsCPF, wallet.CPF)
		require.NotNil(t, wallet.Saude)
		require.NotNil(t, wallet.Saude.ClinicaFamilia)
		require.NotNil(t, wallet.Saude.ClinicaFamilia.Nome)
		assert.Equal(t, nomeCF, *wallet.Saude.ClinicaFamilia.Nome)
		require.NotNil(t, wallet.Saude.ClinicaFamilia.Fonte)
		assert.Equal(t, "bigquery", *wallet.Saude.ClinicaFamilia.Fonte)
	})
}
