package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/clients"
	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestBuildMobilidadeInviteEmail(t *testing.T) {
	msg := BuildMobilidadeInviteEmail(MobilidadeInviteEmailPayload{
		NotifyEmail: "joao@example.com",
		OwnerName:   "Ana Souza",
		DisplayName: "Bike do trabalho",
	}, "https://pref.rio/")

	assert.Equal(t, "joao@example.com", msg.To)
	assert.Contains(t, msg.Subject, "Ana Souza")
	assert.Contains(t, msg.Body, "Bike do trabalho")
	assert.Contains(t, msg.Body, "https://pref.rio/carteira?mobilidade=true")
}

func TestBuildMobilidadeInviteEmail_Defaults(t *testing.T) {
	msg := BuildMobilidadeInviteEmail(MobilidadeInviteEmailPayload{
		NotifyEmail: "x@example.com",
	}, "")

	assert.Contains(t, msg.Subject, "Alguém")
	assert.Contains(t, msg.Body, "um veículo")
	assert.Contains(t, msg.Body, "https://pref.rio/carteira?mobilidade=true")
}

func TestProcessMobilidadeInviteEmail_Success(t *testing.T) {
	sender := &RecordingEmailSender{}
	err := ProcessMobilidadeInviteEmail(context.Background(), sender, MobilidadeInviteEmailPayload{
		ConductorID: "cond1",
		NotifyEmail: "joao@example.com",
		OwnerName:   "Ana",
		DisplayName: "Bike",
	}, "https://pref.rio")
	require.NoError(t, err)
	require.Equal(t, 1, sender.SentCount())
	assert.Equal(t, "joao@example.com", sender.Messages[0].To)
}

func TestProcessMobilidadeInviteEmail_RequiresEmailAndConductorID(t *testing.T) {
	sender := &RecordingEmailSender{}
	err := ProcessMobilidadeInviteEmail(context.Background(), sender, MobilidadeInviteEmailPayload{
		ConductorID: "cond1",
	}, "https://pref.rio")
	require.Error(t, err)
	assert.Equal(t, 0, sender.SentCount())

	err = ProcessMobilidadeInviteEmail(context.Background(), sender, MobilidadeInviteEmailPayload{
		NotifyEmail: "a@example.com",
	}, "https://pref.rio")
	require.Error(t, err)
}

func TestProcessMobilidadeInviteEmail_SenderError(t *testing.T) {
	sender := &RecordingEmailSender{Err: errors.New("smtp down")}
	err := ProcessMobilidadeInviteEmail(context.Background(), sender, MobilidadeInviteEmailPayload{
		ConductorID: "cond1",
		NotifyEmail: "a@example.com",
	}, "https://pref.rio")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "smtp down")
}

func TestSyncWorker_HandleMobilidadeInviteEmailJob(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	job := &SyncJob{
		ID:         "job-1",
		Type:       MobilidadeInviteEmailQueue,
		Key:        "cond-1",
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID:  "cond-1",
			NotifyEmail:  "joao@example.com",
			VehicleID:    "veh-1",
			OwnerName:    "Ana Souza",
			DisplayName:  "Bike do trabalho",
			ConductorCPF: "45049725810",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}

	err := worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.NoError(t, err)
	require.Equal(t, 1, sender.SentCount())
	assert.Contains(t, sender.Messages[0].Body, "Ana Souza")
}

func TestSyncWorker_ProcessMobilidadeInviteEmailFromQueue(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{}
	worker.SetEmailSender(sender)

	job := SyncJob{
		ID:         "job-queue-1",
		Type:       MobilidadeInviteEmailQueue,
		Key:        "cond-2",
		Collection: MobilidadeInviteEmailQueue,
		Data: map[string]interface{}{
			"conductor_id":  "cond-2",
			"notify_email":  "maria@example.com",
			"vehicle_id":    "veh-2",
			"owner_name":    "Pedro",
			"display_name":  "Patinete",
			"conductor_cpf": "11144477735",
		},
		Timestamp:  time.Now().UTC(),
		MaxRetries: 3,
	}
	raw, err := json.Marshal(job)
	require.NoError(t, err)

	ctx := context.Background()
	queueKey := "sync:queue:" + MobilidadeInviteEmailQueue
	require.NoError(t, worker.redis.LPush(ctx, queueKey, raw).Err())

	got, err := worker.getJobNonBlocking(MobilidadeInviteEmailQueue)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, MobilidadeInviteEmailQueue, got.Type)

	worker.processJob(got)
	require.Equal(t, 1, sender.SentCount())
	assert.Equal(t, "maria@example.com", sender.Messages[0].To)
	assert.Contains(t, sender.Messages[0].Body, "Patinete")
}

func TestSyncWorker_MobilidadeInviteEmailFailure(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	sender := &RecordingEmailSender{Err: errors.New("provider unavailable")}
	worker.SetEmailSender(sender)

	job := &SyncJob{
		ID:         "job-fail-1",
		Type:       MobilidadeInviteEmailQueue,
		Key:        "cond-3",
		Collection: MobilidadeInviteEmailQueue,
		Data: MobilidadeInviteEmailPayload{
			ConductorID: "cond-3",
			NotifyEmail: "fail@example.com",
			OwnerName:   "Ana",
			DisplayName: "Bike",
		},
		Timestamp:  time.Now().UTC(),
		RetryCount: 0,
		MaxRetries: 3,
	}

	err := worker.handleMobilidadeInviteEmailJob(context.Background(), job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider unavailable")
	assert.Equal(t, 0, sender.SentCount())
}

func TestInviteConductor_EnqueuesSyncJobForEmail(t *testing.T) {
	vehicleSvc, conductorSvc, _, cleanup := setupMobilidadeVehicleServiceTest(t)
	defer cleanup()
	seedMobilidadeCatalog(t)

	if config.Redis == nil {
		t.Skip("Redis not available")
	}
	_ = logging.InitLogger()

	ctx := context.Background()
	queueKey := "sync:queue:" + MobilidadeInviteEmailQueue
	_ = config.Redis.Del(ctx, queueKey)

	created, err := vehicleSvc.CreateVehicle(ctx, mobilidadeOwnerCPF, catalogCreateRequest())
	require.NoError(t, err)

	link, err := conductorSvc.InviteConductor(ctx, mobilidadeOwnerCPF, created.ID.Hex(), &models.InviteConductorRequest{
		CPF: mobilidadeConductorCPF, Email: "joao@example.com", Name: "João Condutor",
	})
	require.NoError(t, err)

	raw, err := config.Redis.RPop(ctx, queueKey).Result()
	require.NoError(t, err)

	var job SyncJob
	require.NoError(t, json.Unmarshal([]byte(raw), &job))
	assert.Equal(t, MobilidadeInviteEmailQueue, job.Type)
	assert.Equal(t, link.ID.Hex(), job.Key)
	assert.Equal(t, 3, job.MaxRetries)
	assert.Equal(t, models.InviteEmailStatusQueued, link.EmailStatus)

	// Persist status should be queued on the document too.
	var stored models.VehicleConductor
	require.NoError(t, config.MongoDB.Collection(config.AppConfig.MobilidadeConductorCollection).FindOne(ctx, bson.M{"_id": link.ID}).Decode(&stored))
	assert.Equal(t, models.InviteEmailStatusQueued, stored.EmailStatus)

	payload, err := parseMobilidadeInviteEmailPayload(job.Data)
	require.NoError(t, err)
	assert.Equal(t, "joao@example.com", payload.NotifyEmail)
	assert.Equal(t, link.ID.Hex(), payload.ConductorID)
	assert.Equal(t, created.ID.Hex(), payload.VehicleID)
	assert.Equal(t, "Ana Souza", payload.OwnerName)
}

func TestResolveDefaultEmailSender_LoggingWhenUnconfigured(t *testing.T) {
	prevSender := DefaultEmailSender
	prevCfg := config.AppConfig
	defer func() {
		DefaultEmailSender = prevSender
		config.AppConfig = prevCfg
	}()

	DefaultEmailSender = nil
	config.AppConfig = &config.Config{}
	sender := ResolveDefaultEmailSender(nil)
	_, ok := sender.(*LoggingEmailSender)
	assert.True(t, ok)
}

func TestResolveDefaultEmailSender_DataRelayWhenConfigured(t *testing.T) {
	prevSender := DefaultEmailSender
	prevCfg := config.AppConfig
	defer func() {
		DefaultEmailSender = prevSender
		config.AppConfig = prevCfg
	}()

	DefaultEmailSender = nil
	config.AppConfig = &config.Config{
		DataRelayBaseURL: "https://relay.example.com",
		DataRelayAPIKey:  "secret",
		DataRelayTimeout: time.Second,
	}
	sender := ResolveDefaultEmailSender(nil)
	_, ok := sender.(*DataRelayEmailSender)
	assert.True(t, ok)
}

func TestResolveDefaultEmailSender_OverrideWins(t *testing.T) {
	prevSender := DefaultEmailSender
	prevCfg := config.AppConfig
	defer func() {
		DefaultEmailSender = prevSender
		config.AppConfig = prevCfg
	}()

	rec := &RecordingEmailSender{}
	DefaultEmailSender = rec
	config.AppConfig = &config.Config{
		DataRelayBaseURL: "https://relay.example.com",
		DataRelayAPIKey:  "secret",
	}
	assert.Equal(t, rec, ResolveDefaultEmailSender(nil))
}

func TestDataRelayEmailSender_Send(t *testing.T) {
	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/data/mailman", r.URL.Path)
		assert.Equal(t, "k", r.Header.Get("X-Api-Key"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := clients.NewDataRelayClient(srv.URL, "k", time.Second)
	sender := NewDataRelayEmailSender(client, nil)
	err := sender.Send(context.Background(), EmailMessage{
		To:      "joao@example.com",
		Subject: "Convite",
		Body:    "texto",
	})
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"joao@example.com"}, got["to_addresses"])
	assert.Equal(t, "Convite", got["subject"])
	assert.Equal(t, false, got["is_html_body"])
}
