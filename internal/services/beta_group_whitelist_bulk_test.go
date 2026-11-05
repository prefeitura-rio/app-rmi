package services

import (
	"context"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// setupBetaBulkTest uses the collections already configured for the package instead of
// renaming them: config.AppConfig is read concurrently by the index maintenance goroutine
// started in TestMain, so writing to it here would be a data race. Everything the test
// creates (groups and phones) is removed at the end.
func setupBetaBulkTest(t *testing.T, phones ...string) *BetaGroupService {
	t.Helper()
	if config.MongoDB == nil || config.Redis == nil {
		t.Skip("MongoDB/Redis not initialized")
	}
	service := NewBetaGroupService(logging.GetLogger())
	groups := config.MongoDB.Collection(config.AppConfig.BetaGroupCollection)
	mappings := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection)
	clean := func() {
		ctx := context.Background()
		_, _ = groups.DeleteMany(ctx, bson.M{"name": bson.M{"$regex": "^Bulk Teste "}})
		_, _ = mappings.DeleteMany(ctx, bson.M{"phone_number": bson.M{"$in": phones}})
		for _, p := range phones {
			_ = config.Redis.Del(ctx, "beta_status:"+p).Err()
		}
	}
	clean()
	t.Cleanup(clean)
	return service
}

func betaGroupOf(t *testing.T, storagePhone string) string {
	t.Helper()
	var mapping models.PhoneCPFMapping
	err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).
		FindOne(context.Background(), bson.M{"phone_number": storagePhone}).Decode(&mapping)
	require.NoError(t, err)
	return mapping.BetaGroupID
}

func TestGetBetaStatus_CachesAndInvalidatesOnWhitelistChanges(t *testing.T) {
	service := setupBetaBulkTest(t, "5521987660001")
	ctx := context.Background()

	status, err := service.GetBetaStatus(ctx, "+5521987660001")
	require.NoError(t, err)
	assert.False(t, status.BetaWhitelisted, "unknown phone is not whitelisted")
	cached, err := config.Redis.Get(ctx, "beta_status:5521987660001").Result()
	require.NoError(t, err)
	assert.Contains(t, cached, `"beta_whitelisted":false`, "negative result is cached")

	group, err := service.CreateGroup(ctx, "Bulk Teste Status")
	require.NoError(t, err)
	_, err = service.AddToWhitelist(ctx, "+5521987660001", group.ID)
	require.NoError(t, err)

	// Adding to the whitelist invalidates the cached negative answer.
	status, err = service.GetBetaStatus(ctx, "+5521987660001")
	require.NoError(t, err)
	assert.True(t, status.BetaWhitelisted)
	assert.Equal(t, group.ID, status.GroupID)
	assert.Equal(t, "Bulk Teste Status", status.GroupName)

	// Served from cache: same answer even if Mongo changes behind the service's back.
	_, err = config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).UpdateOne(ctx,
		bson.M{"phone_number": "5521987660001"}, bson.M{"$unset": bson.M{"beta_group_id": ""}})
	require.NoError(t, err)
	status, err = service.GetBetaStatus(ctx, "+5521987660001")
	require.NoError(t, err)
	assert.True(t, status.BetaWhitelisted, "answer comes from the cache")

	// A corrupted cache entry falls back to the database.
	require.NoError(t, config.Redis.Set(ctx, "beta_status:5521987660001", "{not json", 0).Err())
	status, err = service.GetBetaStatus(ctx, "+5521987660001")
	require.NoError(t, err)
	assert.False(t, status.BetaWhitelisted)
}

func TestBulkWhitelistOperations(t *testing.T) {
	phones := []string{"5521987660011", "5521987660012", "5521987660013"}
	service := setupBetaBulkTest(t, phones...)
	ctx := context.Background()

	from, err := service.CreateGroup(ctx, "Bulk Teste Origem")
	require.NoError(t, err)
	to, err := service.CreateGroup(ctx, "Bulk Teste Destino")
	require.NoError(t, err)

	// A phone already in a group is skipped by the bulk add.
	_, err = service.AddToWhitelist(ctx, "+5521987660013", to.ID)
	require.NoError(t, err)

	added, err := service.BulkAddToWhitelist(ctx, []string{"+5521987660011", "+5521987660012", "+5521987660013"}, from.ID)
	require.NoError(t, err)
	require.Len(t, added, 2)
	for _, a := range added {
		assert.Equal(t, from.ID, a.GroupID)
		assert.Equal(t, "Bulk Teste Origem", a.GroupName)
	}
	assert.Equal(t, from.ID, betaGroupOf(t, "5521987660011"))
	assert.Equal(t, from.ID, betaGroupOf(t, "5521987660012"))
	assert.Equal(t, to.ID, betaGroupOf(t, "5521987660013"), "already whitelisted phone keeps its group")

	// Cache a status so the move must invalidate it.
	status, err := service.GetBetaStatus(ctx, "+5521987660011")
	require.NoError(t, err)
	assert.Equal(t, from.ID, status.GroupID)

	require.NoError(t, service.BulkMoveWhitelist(ctx, []string{"+5521987660011", "+5521987660012"}, from.ID, to.ID))
	assert.Equal(t, to.ID, betaGroupOf(t, "5521987660011"))
	assert.Equal(t, to.ID, betaGroupOf(t, "5521987660012"))
	status, err = service.GetBetaStatus(ctx, "+5521987660011")
	require.NoError(t, err)
	assert.Equal(t, to.ID, status.GroupID, "cached status was invalidated by the move")

	list, err := service.ListWhitelistedPhones(ctx, 1, 10, to.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), list.TotalCount)

	require.NoError(t, service.BulkRemoveFromWhitelist(ctx, []string{"+5521987660011", "+5521987660012", "+5521987660013"}))
	for _, p := range phones {
		assert.Empty(t, betaGroupOf(t, p))
	}
	status, err = service.GetBetaStatus(ctx, "+5521987660013")
	require.NoError(t, err)
	assert.False(t, status.BetaWhitelisted)
}

func TestBulkWhitelistOperations_InvalidGroups(t *testing.T) {
	service := setupBetaBulkTest(t, "5521987660021")
	ctx := context.Background()

	group, err := service.CreateGroup(ctx, "Bulk Teste Valido")
	require.NoError(t, err)
	missing := primitive.NewObjectID().Hex()

	_, err = service.BulkAddToWhitelist(ctx, []string{"+5521987660021"}, "not-an-id")
	assert.ErrorIs(t, err, models.ErrInvalidGroupID)
	_, err = service.BulkAddToWhitelist(ctx, []string{"+5521987660021"}, missing)
	assert.ErrorIs(t, err, models.ErrGroupNotFound)

	assert.ErrorIs(t, service.BulkMoveWhitelist(ctx, []string{"+5521987660021"}, "bad", group.ID), models.ErrInvalidGroupID)
	assert.ErrorIs(t, service.BulkMoveWhitelist(ctx, []string{"+5521987660021"}, group.ID, "bad"), models.ErrInvalidGroupID)
	assert.ErrorIs(t, service.BulkMoveWhitelist(ctx, []string{"+5521987660021"}, missing, group.ID), models.ErrGroupNotFound)
	assert.ErrorIs(t, service.BulkMoveWhitelist(ctx, []string{"+5521987660021"}, group.ID, missing), models.ErrGroupNotFound)

	n, err := config.MongoDB.Collection(config.AppConfig.PhoneMappingCollection).CountDocuments(ctx, bson.M{"phone_number": "5521987660021"})
	require.NoError(t, err)
	assert.Zero(t, n, "rejected bulk operations must not create mappings")
}

func TestBulkMoveWhitelist_IndividualFallbackOnlyMovesMembersOfTheSourceGroup(t *testing.T) {
	service := setupBetaBulkTest(t, "5521987660031", "5521987660032")
	ctx := context.Background()

	from, err := service.CreateGroup(ctx, "Bulk Teste Fallback Origem")
	require.NoError(t, err)
	to, err := service.CreateGroup(ctx, "Bulk Teste Fallback Destino")
	require.NoError(t, err)
	other, err := service.CreateGroup(ctx, "Bulk Teste Fallback Outro")
	require.NoError(t, err)
	_, err = service.AddToWhitelist(ctx, "+5521987660031", from.ID)
	require.NoError(t, err)
	_, err = service.AddToWhitelist(ctx, "+5521987660032", other.ID)
	require.NoError(t, err)

	require.NoError(t, service.bulkMoveWhitelistIndividual(ctx, []string{"+5521987660031", "+5521987660032"}, from.ID, to.ID))
	assert.Equal(t, to.ID, betaGroupOf(t, "5521987660031"))
	assert.Equal(t, other.ID, betaGroupOf(t, "5521987660032"), "phone of another group is not moved")

	service.invalidateBetaStatusCache(ctx, to.ID)
}
