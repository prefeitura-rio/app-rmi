package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/prefeitura-rio/app-rmi/internal/services"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// e2eRedisDB is a Redis database used only by this test, so the sync worker started
// here sees nothing but the job enqueued by the webhook.
const e2eRedisDB = 9

// TestSalesforceAnonimizacao_WebhookToSyncWorkerToMongo exercises the RTBF flow end to
// end with the real router, Redis queue, sync worker and MongoDB: the Salesforce service
// client posts evento=anonimizacao, the worker consumes the reliable queue and wipes the
// Salesforce mirror of the CPF, leaving the canonical citizen data and the RMI opt-in alone.
func TestSalesforceAnonimizacao_WebhookToSyncWorkerToMongo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if config.MongoDB == nil {
		t.Skip("MongoDB not initialized")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rc := redisclient.NewClient(redis.NewClient(&redis.Options{Addr: redisAddr, Password: os.Getenv("REDIS_PASSWORD"), DB: e2eRedisDB}))
	ctx := context.Background()
	if err := rc.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis unavailable: %v", err)
	}
	require.NoError(t, rc.FlushDB(ctx).Err())

	prevRedis := config.Redis
	prevConfig := config.AppConfig
	cfg := *config.AppConfig
	cfg.SalesforceBaseURL = "https://sf.example.com"
	cfg.SalesforceWebhookClients = []string{"salesforce-rmi"}
	config.AppConfig = &cfg
	config.SetRedis(rc)

	const cpf = "39053344705" // synthetic, check-digit valid
	selfDeclared := config.MongoDB.Collection(cfg.SelfDeclaredCollection)
	userConfig := config.MongoDB.Collection(cfg.UserConfigCollection)
	citizens := config.MongoDB.Collection(cfg.CitizenCollection)
	cleanupDocs := func() {
		_, _ = selfDeclared.DeleteMany(context.Background(), bson.M{"cpf": cpf})
		_, _ = userConfig.DeleteMany(context.Background(), bson.M{"cpf": cpf})
		_, _ = citizens.DeleteMany(context.Background(), bson.M{"cpf": cpf})
	}
	cleanupDocs()
	t.Cleanup(func() {
		cleanupDocs()
		_ = rc.FlushDB(context.Background()).Err()
		config.SetRedis(prevRedis)
		config.AppConfig = prevConfig
	})

	nome := "Cidada Teste"
	email := "cidada.teste@example.com"
	phone := "5521988880000"
	accountID := "001XX000004TEST"
	_, err := citizens.InsertOne(ctx, models.Citizen{CPF: cpf, Nome: &nome})
	require.NoError(t, err)
	_, err = selfDeclared.InsertOne(ctx, models.SelfDeclaredData{
		CPF:                 cpf,
		NomeExibicao:        &nome,
		Email:               &models.Email{Principal: &models.EmailPrincipal{Valor: &email}},
		Telefone:            &models.Telefone{Principal: &models.TelefonePrincipal{Valor: &phone}},
		SalesforceAccountID: &accountID,
	})
	require.NoError(t, err)
	_, err = userConfig.InsertOne(ctx, models.UserConfig{
		CPF:   cpf,
		OptIn: true,
		SalesforceConsentimentos: map[string]models.SalesforceConsentimentoEntry{
			"PREF_Lembrete_Pagamento": {Categoria: "PREF_Lembrete_Pagamento", Status: "IN", OptIn: true},
		},
	})
	require.NoError(t, err)

	// 1. Salesforce posts the anonimizacao event (no dados in the new contract).
	payload := `{"cpf":"` + cpf + `","evento":"anonimizacao","updatedAt":"2026-08-27T17:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/salesforce/cidadao", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+salesforceWebhookJWT)
	w := httptest.NewRecorder()
	salesforceWebhookRouter().ServeHTTP(w, req)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	queueKey := "sync:queue:{" + services.SalesforceSyncQueue + "}"
	queued, err := rc.LLen(ctx, queueKey).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), queued, "webhook must enqueue exactly one salesforce_sync job")

	// 2. The sync worker consumes the job.
	metrics := services.NewMetrics()
	worker := services.NewSyncWorker(rc, config.MongoDB, 1, logging.GetLogger(), metrics, services.NewDegradedMode(rc, config.MongoDB, metrics))
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Start()
	}()

	var sd models.SelfDeclaredData
	deadline := time.Now().Add(15 * time.Second)
	for {
		sd = models.SelfDeclaredData{}
		require.NoError(t, selfDeclared.FindOne(ctx, bson.M{"cpf": cpf}).Decode(&sd))
		if sd.SalesforceAnonymized {
			break
		}
		if time.Now().After(deadline) {
			worker.Stop()
			<-done
			t.Fatalf("sync worker did not apply the anonimizacao within 15s")
		}
		time.Sleep(50 * time.Millisecond)
	}
	worker.Stop()
	<-done

	// 3. The Salesforce mirror is wiped and stamped with the event time.
	assert.Nil(t, sd.Email)
	assert.Nil(t, sd.Telefone)
	assert.Nil(t, sd.NomeExibicao)
	assert.Nil(t, sd.SalesforceAccountID)
	require.NotNil(t, sd.SalesforceAnonymizedAt)
	require.NotNil(t, sd.SalesforceUpdatedAt)
	assert.Equal(t, "2026-08-27T17:00:00Z", sd.SalesforceUpdatedAt.UTC().Format(time.RFC3339))

	var uc models.UserConfig
	require.NoError(t, userConfig.FindOne(ctx, bson.M{"cpf": cpf}).Decode(&uc))
	assert.Empty(t, uc.SalesforceConsentimentos, "Salesforce consentimentos are cleared")
	assert.True(t, uc.OptIn, "RMI opt-in is not touched by the anonimizacao")

	var citizen models.Citizen
	require.NoError(t, citizens.FindOne(ctx, bson.M{"cpf": cpf}).Decode(&citizen))
	require.NotNil(t, citizen.Nome)
	assert.Equal(t, nome, *citizen.Nome, "canonical citizen data is not touched")

	// 4. The reliable queue is drained: nothing left queued, in flight or in the DLQ.
	for _, key := range []string{
		queueKey,
		"sync:processing:{" + services.SalesforceSyncQueue + "}",
		"sync:dlq:{" + services.SalesforceSyncQueue + "}",
	} {
		n, err := rc.LLen(ctx, key).Result()
		require.NoError(t, err)
		assert.Zero(t, n, "%s must be empty", key)
	}
}
