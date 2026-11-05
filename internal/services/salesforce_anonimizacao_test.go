package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreSalesforceAnonimizacaoOwner(t *testing.T) {
	worker, _, cleanup := setupSyncWorkerTest(t)
	defer cleanup()

	ctx := context.Background()
	require.Error(t, StoreSalesforceAnonimizacaoOwner(ctx, nil, "PRIV-1", "14202478754"))
	require.Error(t, StoreSalesforceAnonimizacaoOwner(ctx, worker.redis, "", "14202478754"))

	require.NoError(t, StoreSalesforceAnonimizacaoOwner(ctx, worker.redis, "PRIV-1", "142.024.787-54"))
	t.Cleanup(func() {
		_ = worker.redis.Del(ctx, salesforceAnonimizacaoOwnerKey("PRIV-1")).Err()
	})

	got, ok := LookupSalesforceAnonimizacaoOwner(ctx, worker.redis, "PRIV-1")
	require.True(t, ok)
	assert.Equal(t, "14202478754", got)

	_, ok = LookupSalesforceAnonimizacaoOwner(ctx, nil, "PRIV-1")
	assert.False(t, ok)
	_, ok = LookupSalesforceAnonimizacaoOwner(ctx, worker.redis, "missing")
	assert.False(t, ok)
}
