package services

import (
	"strings"
	"testing"

	"github.com/prefeitura-rio/app-rmi/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testSyncJobBearerKey = []byte("0123456789abcdef0123456789abcdef")

func TestSealBearerToken_RoundTrip(t *testing.T) {
	aad := bearerAAD(SalesforcePushQueue, "14202478754")
	plain := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjMifQ.sig"
	sealed, err := sealBearerToken(testSyncJobBearerKey, plain, aad)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(sealed, bearerSealPrefix))
	assert.NotContains(t, sealed, plain)

	got, err := openBearerToken(testSyncJobBearerKey, sealed, aad)
	require.NoError(t, err)
	assert.Equal(t, plain, got)
}

func TestSealBearerToken_UniqueCiphertext(t *testing.T) {
	aad := bearerAAD(SalesforcePushQueue, "14202478754")
	a, err := sealBearerToken(testSyncJobBearerKey, "user-jwt", aad)
	require.NoError(t, err)
	b, err := sealBearerToken(testSyncJobBearerKey, "user-jwt", aad)
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
}

func TestOpenBearerToken_WrongAADFails(t *testing.T) {
	sealed, err := sealBearerToken(testSyncJobBearerKey, "user-jwt", bearerAAD(SalesforcePushQueue, "111"))
	require.NoError(t, err)
	_, err = openBearerToken(testSyncJobBearerKey, sealed, bearerAAD(SalesforcePushQueue, "222"))
	require.Error(t, err)
}

func TestBearerFromQueue_LegacyPlaintext(t *testing.T) {
	got, err := bearerFromQueue("user-jwt", SalesforcePushQueue, "14202478754")
	require.NoError(t, err)
	assert.Equal(t, "user-jwt", got)
}

func TestPrepareBearerForQueue_RequiresKeyForSalesforcePush(t *testing.T) {
	prev := config.AppConfig
	config.AppConfig = &config.Config{}
	defer func() { config.AppConfig = prev }()

	_, err := prepareBearerForQueue("user-jwt", SalesforcePushQueue, "14202478754")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SYNC_JOB_BEARER_ENCRYPTION_KEY")

	plain, err := prepareBearerForQueue("user-jwt", "self_declared_email", "14202478754")
	require.NoError(t, err)
	assert.Equal(t, "user-jwt", plain)
}

func TestPrepareBearerForQueue_SealsWhenKeyPresent(t *testing.T) {
	prev := config.AppConfig
	config.AppConfig = &config.Config{SyncJobBearerEncryptionKey: testSyncJobBearerKey}
	defer func() { config.AppConfig = prev }()

	sealed, err := prepareBearerForQueue("user-jwt", SalesforcePushQueue, "14202478754")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(sealed, bearerSealPrefix))
	assert.NotContains(t, sealed, "user-jwt")

	got, err := bearerFromQueue(sealed, SalesforcePushQueue, "14202478754")
	require.NoError(t, err)
	assert.Equal(t, "user-jwt", got)
}
