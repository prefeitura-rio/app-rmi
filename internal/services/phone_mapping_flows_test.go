package services

import (
	"context"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// Synthetic, check-digit-valid CPFs (never real people).
const (
	flowCPFOwner = "11144477735"
	flowCPFOther = "52998224725"
)

// setupPhoneFlowsTest points the phone flows at collections used by no other test. The
// shared test_* collections are also used, and dropped, by the handlers package, whose
// tests run in parallel with these under go test ./....
func setupPhoneFlowsTest(t *testing.T) (*PhoneMappingService, func()) {
	t.Helper()
	setupTestEnvironment()
	if config.MongoDB == nil {
		t.Skip("Skipping phone flows tests: MongoDB not available")
	}
	cfg := config.AppConfig
	orig := [5]interface{}{cfg.CitizenCollection, cfg.PhoneMappingCollection, cfg.OptInHistoryCollection, cfg.BetaGroupCollection, cfg.PhoneQuarantineTTL}
	cfg.CitizenCollection = "flows_test_citizens"
	cfg.PhoneMappingCollection = "flows_test_phone_mappings"
	cfg.OptInHistoryCollection = "flows_test_opt_in_history"
	cfg.BetaGroupCollection = "flows_test_beta_groups"
	cfg.PhoneQuarantineTTL = 6 * 30 * 24 * time.Hour
	drop := func() {
		ctx := context.Background()
		for _, c := range []string{cfg.CitizenCollection, cfg.PhoneMappingCollection, cfg.OptInHistoryCollection, cfg.BetaGroupCollection} {
			_ = config.MongoDB.Collection(c).Drop(ctx)
		}
	}
	drop()
	return NewPhoneMappingService(logging.GetLogger()), func() {
		drop()
		cfg.CitizenCollection = orig[0].(string)
		cfg.PhoneMappingCollection = orig[1].(string)
		cfg.OptInHistoryCollection = orig[2].(string)
		cfg.BetaGroupCollection = orig[3].(string)
		cfg.PhoneQuarantineTTL = orig[4].(time.Duration)
	}
}

func findMapping(t *testing.T, storagePhone string) models.PhoneCPFMapping {
	t.Helper()
	var mapping models.PhoneCPFMapping
	err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).
		FindOne(context.Background(), bson.M{"phone_number": storagePhone}).Decode(&mapping)
	require.NoError(t, err)
	return mapping
}

func countHistory(t *testing.T, filter bson.M) int64 {
	t.Helper()
	n, err := config.MongoDB.Collection(config.AppConfig.OptInHistoryCollection).CountDocuments(context.Background(), filter)
	require.NoError(t, err)
	return n
}

func insertFlowCitizen(t *testing.T, cpf, nome string, nascimento time.Time) {
	t.Helper()
	_, err := config.MongoDB.Collection(config.AppConfig.CitizenCollection).InsertOne(context.Background(), models.Citizen{
		CPF:        cpf,
		Nome:       &nome,
		Nascimento: &models.Nascimento{Data: &nascimento},
	})
	require.NoError(t, err)
}

func TestBindPhoneToCPF_NewPhoneCreatesActiveMappingWithoutOptIn(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()

	resp, err := service.BindPhoneToCPF(context.Background(), "+5521987650001", flowCPFOwner, "whatsapp")
	require.NoError(t, err)
	assert.Equal(t, "bound", resp.Status)
	assert.False(t, resp.OptIn)
	assert.Equal(t, flowCPFOwner, resp.CPF)

	mapping := findMapping(t, "5521987650001")
	assert.Equal(t, flowCPFOwner, mapping.CPF)
	assert.Equal(t, models.MappingStatusActive, mapping.Status)
	assert.Equal(t, "whatsapp", mapping.Channel)
	assert.False(t, mapping.OptIn)
	assert.Zero(t, countHistory(t, bson.M{"phone_number": "5521987650001"}), "bind must not record opt-in history")
}

func TestBindPhoneToCPF_ExistingQuarantinedPhoneIsReleasedAndRebound(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now()
	until := now.Add(48 * time.Hour)
	_, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, models.PhoneCPFMapping{
		PhoneNumber:       "5521987650002",
		CPF:               flowCPFOther,
		Status:            models.MappingStatusQuarantined,
		QuarantineUntil:   &until,
		QuarantineHistory: []models.QuarantineEvent{{QuarantinedAt: now.Add(-time.Hour), QuarantineUntil: until}},
		CreatedAt:         &now,
		UpdatedAt:         &now,
	})
	require.NoError(t, err)

	resp, err := service.BindPhoneToCPF(ctx, "+5521987650002", flowCPFOwner, "web")
	require.NoError(t, err)
	assert.Equal(t, "bound", resp.Status)

	mapping := findMapping(t, "5521987650002")
	assert.Equal(t, flowCPFOwner, mapping.CPF)
	assert.Equal(t, models.MappingStatusActive, mapping.Status)
	assert.Equal(t, "web", mapping.Channel)
	assert.Nil(t, mapping.QuarantineUntil)
	require.Len(t, mapping.QuarantineHistory, 1)
	assert.NotNil(t, mapping.QuarantineHistory[0].ReleasedAt, "the open quarantine event must be closed")
}

func TestBindPhoneToCPF_RejectsInvalidInput(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.BindPhoneToCPF(ctx, "abc", flowCPFOwner, "whatsapp")
	assert.ErrorContains(t, err, "invalid phone number")

	_, err = service.BindPhoneToCPF(ctx, "+5521987650003", "11111111111", "whatsapp")
	assert.ErrorContains(t, err, "invalid CPF")

	n, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Zero(t, n, "invalid input must not create a mapping")
}

func TestGetBoundCPF(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.BindPhoneToCPF(ctx, "+5521987650004", flowCPFOwner, "whatsapp")
	require.NoError(t, err)
	_, err = service.OptOut(ctx, "+5521987650005", "Não quero receber", "whatsapp")
	require.NoError(t, err)

	cpf, err := service.GetBoundCPF(ctx, "+5521987650004")
	require.NoError(t, err)
	assert.Equal(t, flowCPFOwner, cpf, "bound CPF is returned unmasked for authorization")

	cpf, err = service.GetBoundCPF(ctx, "+5521987650005")
	require.NoError(t, err)
	assert.Empty(t, cpf, "an opt-out of an unknown phone has no bound CPF")

	cpf, err = service.GetBoundCPF(ctx, "+5521987650099")
	require.NoError(t, err)
	assert.Empty(t, cpf, "a phone without mapping has no bound CPF")

	_, err = service.GetBoundCPF(ctx, "not-a-phone")
	assert.ErrorContains(t, err, "invalid phone number")
}

func TestOptIn_RebindsPhoneOfAnotherCPFAndReleasesQuarantine(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now()
	until := now.Add(24 * time.Hour)
	_, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).InsertOne(ctx, models.PhoneCPFMapping{
		PhoneNumber:       "5521987650006",
		CPF:               flowCPFOther,
		Status:            models.MappingStatusQuarantined,
		QuarantineUntil:   &until,
		QuarantineHistory: []models.QuarantineEvent{{QuarantinedAt: now, QuarantineUntil: until}},
		CreatedAt:         &now,
		UpdatedAt:         &now,
	})
	require.NoError(t, err)

	resp, err := service.OptIn(ctx, "+5521987650006", flowCPFOwner, "whatsapp")
	require.NoError(t, err)
	assert.Equal(t, "opted_in", resp.Status)

	mapping := findMapping(t, "5521987650006")
	assert.Equal(t, flowCPFOwner, mapping.CPF)
	assert.Equal(t, models.MappingStatusActive, mapping.Status)
	assert.Nil(t, mapping.QuarantineUntil)
	require.Len(t, mapping.QuarantineHistory, 1)
	assert.NotNil(t, mapping.QuarantineHistory[0].ReleasedAt)
	assert.Equal(t, int64(1), countHistory(t, bson.M{"phone_number": "5521987650006", "cpf": flowCPFOwner, "action": "opt_in"}))
}

func TestOptIn_RejectsInvalidInput(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.OptIn(ctx, "12", flowCPFOwner, "whatsapp")
	assert.ErrorContains(t, err, "invalid phone number")
	_, err = service.OptIn(ctx, "+5521987650007", "00000000000", "whatsapp")
	assert.ErrorContains(t, err, "invalid CPF")
	assert.Zero(t, countHistory(t, bson.M{}), "invalid opt-in must not record history")
}

func TestOptOut_BoundPhoneBlocksMappingAndRecordsHistory(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	_, err := service.OptIn(ctx, "+5521987650008", flowCPFOwner, "whatsapp")
	require.NoError(t, err)

	resp, err := service.OptOut(ctx, "+5521987650008", "Mensagem era engano", "whatsapp")
	require.NoError(t, err)
	assert.Equal(t, "opted_out", resp.Status)

	mapping := findMapping(t, "5521987650008")
	assert.Equal(t, models.MappingStatusBlocked, mapping.Status)
	assert.Equal(t, flowCPFOwner, mapping.CPF, "opt-out keeps the bound CPF for the audit trail")
	assert.Equal(t, int64(1), countHistory(t, bson.M{"phone_number": "5521987650008", "action": "opt_out", "reason": "Mensagem era engano"}))

	status, err := service.GetPhoneStatus(ctx, "+5521987650008")
	require.NoError(t, err)
	assert.True(t, status.OptedOut)

	_, err = service.OptOut(ctx, "x", "motivo", "whatsapp")
	assert.ErrorContains(t, err, "invalid phone number")
}

func TestRejectRegistration(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := service.RejectRegistration(ctx, "+5521987650009", flowCPFOwner)
	require.NoError(t, err)
	assert.Equal(t, "not_found", resp.Status, "rejecting an unknown phone is not an error")
	assert.Zero(t, countHistory(t, bson.M{"phone_number": "5521987650009"}))

	_, err = service.BindPhoneToCPF(ctx, "+5521987650009", flowCPFOwner, "whatsapp")
	require.NoError(t, err)

	resp, err = service.RejectRegistration(ctx, "+5521987650009", flowCPFOwner)
	require.NoError(t, err)
	assert.Equal(t, "rejected", resp.Status)
	assert.Equal(t, models.MappingStatusBlocked, findMapping(t, "5521987650009").Status)
	assert.Equal(t, int64(1), countHistory(t, bson.M{"phone_number": "5521987650009", "action": "rejected", "cpf": flowCPFOwner}))

	_, err = service.RejectRegistration(ctx, "+5521987650009", "12345678900")
	assert.ErrorContains(t, err, "invalid CPF")
	_, err = service.RejectRegistration(ctx, "bad", flowCPFOwner)
	assert.ErrorContains(t, err, "invalid phone number")
}

func TestValidateRegistration(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	birth := time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC)
	insertFlowCitizen(t, flowCPFOwner, "Maria Teste da Silva", birth)

	t.Run("matching name (case and spaces ignored) and birth date", func(t *testing.T) {
		resp, err := service.ValidateRegistration(ctx, "+5521987650010", "  maria teste DA silva ", flowCPFOwner, "1990-05-17")
		require.NoError(t, err)
		assert.True(t, resp.Valid)
		assert.Equal(t, flowCPFOwner, resp.MatchedCPF)
		assert.Equal(t, "Maria Teste da Silva", resp.MatchedName)

		mapping := findMapping(t, "5521987650010")
		assert.True(t, mapping.ValidationAttempt.Valid)
		assert.Equal(t, "whatsapp", mapping.ValidationAttempt.Channel)
	})

	t.Run("wrong birth date", func(t *testing.T) {
		resp, err := service.ValidateRegistration(ctx, "+5521987650011", "Maria Teste da Silva", flowCPFOwner, "1990-05-18")
		require.NoError(t, err)
		assert.False(t, resp.Valid)
		assert.Empty(t, resp.MatchedCPF, "a failed validation must not reveal the CPF")
		assert.Empty(t, resp.MatchedName)
		assert.False(t, findMapping(t, "5521987650011").ValidationAttempt.Valid)
	})

	t.Run("wrong name", func(t *testing.T) {
		resp, err := service.ValidateRegistration(ctx, "+5521987650012", "Outra Pessoa", flowCPFOwner, "1990-05-17")
		require.NoError(t, err)
		assert.False(t, resp.Valid)
		assert.Empty(t, resp.MatchedName)
	})

	t.Run("unknown CPF", func(t *testing.T) {
		resp, err := service.ValidateRegistration(ctx, "+5521987650013", "Maria Teste da Silva", flowCPFOther, "1990-05-17")
		require.NoError(t, err)
		assert.False(t, resp.Valid)
	})

	t.Run("invalid CPF", func(t *testing.T) {
		resp, err := service.ValidateRegistration(ctx, "+5521987650014", "Maria Teste da Silva", "11111111111", "1990-05-17")
		require.NoError(t, err)
		assert.False(t, resp.Valid)
	})

	t.Run("invalid phone", func(t *testing.T) {
		_, err := service.ValidateRegistration(ctx, "phone", "Maria Teste da Silva", flowCPFOwner, "1990-05-17")
		assert.ErrorContains(t, err, "invalid phone number")
	})
}

func TestGetQuarantinedPhones_ActiveAndExpiredWithPagination(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now()
	coll := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	for i, offset := range []time.Duration{time.Hour, 2 * time.Hour, 3 * time.Hour, -time.Hour} {
		until := now.Add(offset)
		doc := models.PhoneCPFMapping{
			PhoneNumber:     "552198765010" + string(rune('0'+i)),
			Status:          models.MappingStatusQuarantined,
			QuarantineUntil: &until,
			CreatedAt:       &now,
		}
		if i == 0 {
			doc.CPF = flowCPFOwner
		}
		_, err := coll.InsertOne(ctx, doc)
		require.NoError(t, err)
	}
	// A phone that was never quarantined is not listed.
	_, err := coll.InsertOne(ctx, models.PhoneCPFMapping{PhoneNumber: "5521987650199", Status: models.MappingStatusActive})
	require.NoError(t, err)

	active, err := service.GetQuarantinedPhones(ctx, 1, 2, false)
	require.NoError(t, err)
	assert.Equal(t, 3, active.Pagination.Total)
	assert.Equal(t, 2, active.Pagination.TotalPages)
	require.Len(t, active.Data, 2)
	assert.True(t, active.Data[0].QuarantineUntil.Before(active.Data[1].QuarantineUntil), "sorted by quarantine end")
	assert.False(t, active.Data[0].Expired)
	assert.NotEqual(t, flowCPFOwner, active.Data[0].CPF, "CPF must be masked")
	assert.NotEmpty(t, active.Data[0].CPF)

	page2, err := service.GetQuarantinedPhones(ctx, 2, 2, false)
	require.NoError(t, err)
	require.Len(t, page2.Data, 1)
	assert.Empty(t, page2.Data[0].CPF)

	expired, err := service.GetQuarantinedPhones(ctx, 0, 1000, true)
	require.NoError(t, err)
	assert.Equal(t, 1, expired.Pagination.Page, "page below 1 is normalized")
	assert.Equal(t, 20, expired.Pagination.PerPage, "per_page above 100 falls back to the default")
	require.Len(t, expired.Data, 1)
	assert.True(t, expired.Data[0].Expired)
}

func TestGetQuarantineStats_CountsHistoryAndCPF(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)
	coll := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	_, err := coll.InsertMany(ctx, []interface{}{
		models.PhoneCPFMapping{PhoneNumber: "5521987650201", CPF: flowCPFOwner, QuarantineUntil: &future,
			QuarantineHistory: []models.QuarantineEvent{{QuarantinedAt: past, QuarantineUntil: future}, {QuarantinedAt: now, QuarantineUntil: future}}},
		models.PhoneCPFMapping{PhoneNumber: "5521987650202", QuarantineUntil: &past,
			QuarantineHistory: []models.QuarantineEvent{{QuarantinedAt: past, QuarantineUntil: past}}},
	})
	require.NoError(t, err)

	stats, err := service.GetQuarantineStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, stats.TotalQuarantined)
	assert.Equal(t, 1, stats.ActiveQuarantines)
	assert.Equal(t, 1, stats.ExpiredQuarantines)
	assert.Equal(t, 1, stats.QuarantinesWithCPF)
	assert.Equal(t, 1, stats.QuarantinesWithoutCPF)
	assert.Equal(t, 3, stats.QuarantineHistoryTotal)
}

func TestBulkPhoneMappingOperations(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	require.NoError(t, service.BulkInsertPhoneMappings(ctx, nil))
	require.NoError(t, service.BulkUpdatePhoneStatuses(ctx, nil))

	now := time.Now()
	require.NoError(t, service.BulkInsertPhoneMappings(ctx, []models.PhoneCPFMapping{
		{PhoneNumber: "5521987650301", CPF: flowCPFOwner, Status: models.MappingStatusActive, CreatedAt: &now},
		{PhoneNumber: "5521987650302", CPF: flowCPFOther, Status: models.MappingStatusActive, CreatedAt: &now},
	}))

	require.NoError(t, service.BulkUpdatePhoneStatuses(ctx, []PhoneStatusUpdate{
		{PhoneNumber: "5521987650301", Fields: map[string]interface{}{"status": models.MappingStatusBlocked}},
		{PhoneNumber: "5521987650302", Fields: map[string]interface{}{"opt_in": true}},
	}))

	assert.Equal(t, models.MappingStatusBlocked, findMapping(t, "5521987650301").Status)
	second := findMapping(t, "5521987650302")
	assert.True(t, second.OptIn)
	assert.Equal(t, models.MappingStatusActive, second.Status)
}

func TestFindCPFByPhone_EdgeCases(t *testing.T) {
	service, cleanup := setupPhoneFlowsTest(t)
	defer cleanup()
	ctx := context.Background()

	resp, err := service.FindCPFByPhone(ctx, "+5521987650401")
	require.NoError(t, err)
	assert.False(t, resp.Found, "unknown phone")

	_, err = service.OptOut(ctx, "+5521987650402", "motivo", "whatsapp")
	require.NoError(t, err)
	resp, err = service.FindCPFByPhone(ctx, "+5521987650402")
	require.NoError(t, err)
	assert.False(t, resp.Found, "mapping without CPF")

	_, err = service.BindPhoneToCPF(ctx, "+5521987650403", flowCPFOther, "whatsapp")
	require.NoError(t, err)
	resp, err = service.FindCPFByPhone(ctx, "+5521987650403")
	require.NoError(t, err)
	assert.False(t, resp.Found, "mapping whose citizen does not exist")

	_, err = service.FindCPFByPhone(ctx, "zzz")
	assert.ErrorContains(t, err, "invalid phone number")
}
