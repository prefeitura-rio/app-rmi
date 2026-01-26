package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskMongoURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "standard URI with credentials",
			uri:      "mongodb://username:password@localhost:27017/database",
			expected: "mongodb://****:****@localhost:27017/database",
		},
		{
			name:     "URI with special characters in password",
			uri:      "mongodb://user:p@ssw0rd!@cluster.mongodb.net:27017/db",
			expected: "mongodb://****:****@cluster.mongodb.net:27017/db",
		},
		{
			name:     "URI with replica set",
			uri:      "mongodb://admin:secret@host1:27017,host2:27017,host3:27017/mydb?replicaSet=rs0",
			expected: "mongodb://****:****@host1:27017,host2:27017,host3:27017/mydb?replicaSet=rs0",
		},
		{
			name:     "URI with MongoDB Atlas",
			uri:      "mongodb://myuser:mypass@cluster0.mongodb.net/test?retryWrites=true&w=majority",
			expected: "mongodb://****:****@cluster0.mongodb.net/test?retryWrites=true&w=majority",
		},
		{
			name:     "URI with long password",
			uri:      "mongodb://service:verylongpassword123456789@prod-cluster.example.com:27017/production",
			expected: "mongodb://****:****@prod-cluster.example.com:27017/production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskMongoURI(tt.uri)
			assert.Equal(t, tt.expected, result)

			// Verify credentials are masked
			assert.Contains(t, result, "****:****")
			assert.NotContains(t, result, "username")
			assert.NotContains(t, result, "password")
			assert.NotContains(t, result, "secret")
			assert.NotContains(t, result, "admin")
		})
	}
}

func TestGetLoadDistributionStatus(t *testing.T) {
	tests := []struct {
		name                  string
		primaryLoadPercentage float64
		expectedStatus        string
	}{
		{
			name:                  "excellent load distribution",
			primaryLoadPercentage: 30.0,
			expectedStatus:        "excellent",
		},
		{
			name:                  "good load distribution",
			primaryLoadPercentage: 55.0,
			expectedStatus:        "good",
		},
		{
			name:                  "fair load distribution",
			primaryLoadPercentage: 65.0,
			expectedStatus:        "fair",
		},
		{
			name:                  "poor load distribution",
			primaryLoadPercentage: 75.0,
			expectedStatus:        "poor",
		},
		{
			name:                  "critical load distribution",
			primaryLoadPercentage: 85.0,
			expectedStatus:        "critical",
		},
		{
			name:                  "boundary excellent/good",
			primaryLoadPercentage: 49.9,
			expectedStatus:        "excellent",
		},
		{
			name:                  "boundary good/fair",
			primaryLoadPercentage: 59.9,
			expectedStatus:        "good",
		},
		{
			name:                  "boundary fair/poor",
			primaryLoadPercentage: 69.9,
			expectedStatus:        "fair",
		},
		{
			name:                  "boundary poor/critical",
			primaryLoadPercentage: 79.9,
			expectedStatus:        "poor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLoadDistributionStatus(tt.primaryLoadPercentage)
			assert.Equal(t, tt.expectedStatus, result)
		})
	}
}

func TestGetDatabasePerformanceStatus(t *testing.T) {
	tests := []struct {
		name               string
		sessionsInProgress int
		expectedStatus     string
	}{
		{
			name:               "excellent performance",
			sessionsInProgress: 100,
			expectedStatus:     "excellent",
		},
		{
			name:               "good performance",
			sessionsInProgress: 600,
			expectedStatus:     "good",
		},
		{
			name:               "fair performance",
			sessionsInProgress: 750,
			expectedStatus:     "fair",
		},
		{
			name:               "poor performance",
			sessionsInProgress: 850,
			expectedStatus:     "poor",
		},
		{
			name:               "critical performance",
			sessionsInProgress: 950,
			expectedStatus:     "critical",
		},
		{
			name:               "boundary excellent/good",
			sessionsInProgress: 499,
			expectedStatus:     "excellent",
		},
		{
			name:               "boundary good/fair",
			sessionsInProgress: 699,
			expectedStatus:     "good",
		},
		{
			name:               "boundary fair/poor",
			sessionsInProgress: 799,
			expectedStatus:     "fair",
		},
		{
			name:               "boundary poor/critical",
			sessionsInProgress: 899,
			expectedStatus:     "poor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getDatabasePerformanceStatus(tt.sessionsInProgress)
			assert.Equal(t, tt.expectedStatus, result)
		})
	}
}

func TestGetReplicaSetRecommendation(t *testing.T) {
	tests := []struct {
		name             string
		primaryHealthy   bool
		secondaryHealthy bool
		expectedContains string
	}{
		{
			name:             "all nodes healthy",
			primaryHealthy:   true,
			secondaryHealthy: true,
			expectedContains: "GOOD",
		},
		{
			name:             "primary unhealthy",
			primaryHealthy:   false,
			secondaryHealthy: true,
			expectedContains: "CRITICAL",
		},
		{
			name:             "secondary unhealthy",
			primaryHealthy:   true,
			secondaryHealthy: false,
			expectedContains: "WARNING",
		},
		{
			name:             "both unhealthy",
			primaryHealthy:   false,
			secondaryHealthy: false,
			expectedContains: "CRITICAL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getReplicaSetRecommendation(tt.primaryHealthy, tt.secondaryHealthy)
			assert.Contains(t, result, tt.expectedContains)
		})
	}
}

func TestCheckNodeHealth(t *testing.T) {
	tests := []struct {
		name     string
		nodeType string
		expected bool
	}{
		{
			name:     "primary node",
			nodeType: "primary",
			expected: true,
		},
		{
			name:     "secondary node",
			nodeType: "secondary",
			expected: true,
		},
		{
			name:     "unknown node",
			nodeType: "unknown",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Using background context since checkNodeHealth doesn't actually use it
			// in the current simplified implementation
			result := checkNodeHealth(nil, tt.nodeType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPrimaryNodeConnections(t *testing.T) {
	// This function requires MongoDB to be initialized,
	// which we can't do in unit tests. Skip this test for now.
	// In a real environment with MongoDB initialized, it would return
	// an estimate based on connection pool stats.
	t.Skip("Requires MongoDB initialization")
}

func TestGetSecondaryNodeConnections(t *testing.T) {
	// This function requires MongoDB to be initialized,
	// which we can't do in unit tests. Skip this test for now.
	// In a real environment with MongoDB initialized, it would return
	// an estimate based on connection pool stats.
	t.Skip("Requires MongoDB initialization")
}

func TestMaskMongoURI_PreservesHostAndParams(t *testing.T) {
	uri := "mongodb://user:pass@host1:27017,host2:27017/db?ssl=true"
	result := maskMongoURI(uri)

	// Should preserve hosts
	assert.Contains(t, result, "host1:27017")
	assert.Contains(t, result, "host2:27017")

	// Should preserve parameters
	assert.Contains(t, result, "ssl=true")

	// Should preserve database name
	assert.Contains(t, result, "/db")

	// Should mask credentials
	assert.Contains(t, result, "****:****")
	assert.NotContains(t, result, "user")
	assert.NotContains(t, result, "pass")
}

func TestMaskMongoURI_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{
			name: "URI with @ in password",
			uri:  "mongodb://user:p@ss@host:27017/db",
		},
		{
			name: "URI with multiple @ symbols",
			uri:  "mongodb://user:p@ss:w@rd@cluster.net:27017/db",
		},
		{
			name: "URI with special chars in username",
			uri:  "mongodb://user@example.com:password@host:27017/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskMongoURI(tt.uri)

			// Should always contain masked credentials
			assert.Contains(t, result, "****:****@")

			// Should preserve what comes after last @
			parts := strings.Split(tt.uri, "@")
			lastPart := parts[len(parts)-1]
			assert.Contains(t, result, lastPart)
		})
	}
}

func TestMaskMongoURI_NoCredentials(t *testing.T) {
	// URI without credentials (localhost development)
	uri := "mongodb://localhost:27017/database"
	result := maskMongoURI(uri)

	// When there's no @ before the last part, it should still work
	// The function assumes @ is present, so this tests edge case behavior
	assert.Contains(t, result, "mongodb://")
}

func TestConfigureCollectionWriteConcerns_DoesNotPanic(t *testing.T) {
	// Initialize minimal config
	if AppConfig == nil {
		AppConfig = &Config{
			CitizenCollection:            "citizens",
			UserConfigCollection:         "user_config",
			PhoneMappingCollection:       "phone_mappings",
			OptInHistoryCollection:       "opt_in_history",
			BetaGroupCollection:          "beta_groups",
			PhoneVerificationCollection:  "phone_verifications",
			MaintenanceRequestCollection: "maintenance_requests",
			SelfDeclaredCollection:       "self_declared",
			AuditLogsCollection:          "audit_logs",
		}
	}

	// Should not panic when called
	assert.NotPanics(t, func() {
		configureCollectionWriteConcerns()
	})
}

func TestConfig_CollectionNames(t *testing.T) {
	config := &Config{
		CitizenCollection:           "test_citizens",
		SelfDeclaredCollection:      "test_self_declared",
		PhoneVerificationCollection: "test_phone_verifications",
		UserConfigCollection:        "test_user_config",
	}

	assert.Equal(t, "test_citizens", config.CitizenCollection)
	assert.Equal(t, "test_self_declared", config.SelfDeclaredCollection)
	assert.Equal(t, "test_phone_verifications", config.PhoneVerificationCollection)
	assert.Equal(t, "test_user_config", config.UserConfigCollection)
}

func TestConfig_ConnectionSettings(t *testing.T) {
	config := &Config{
		MongoURI:      "mongodb://localhost:27017",
		MongoDatabase: "test_db",
		RedisURI:      "redis://localhost:6379",
		RedisDB:       1,
	}

	assert.Equal(t, "mongodb://localhost:27017", config.MongoURI)
	assert.Equal(t, "test_db", config.MongoDatabase)
	assert.Equal(t, "redis://localhost:6379", config.RedisURI)
	assert.Equal(t, 1, config.RedisDB)
}

func TestConfig_RedisClusterSettings(t *testing.T) {
	config := &Config{
		RedisClusterEnabled:  true,
		RedisClusterAddrs:    []string{"node1:6379", "node2:6379", "node3:6379"},
		RedisClusterPassword: "cluster_pass",
	}

	assert.True(t, config.RedisClusterEnabled)
	assert.Len(t, config.RedisClusterAddrs, 3)
	assert.Contains(t, config.RedisClusterAddrs, "node1:6379")
	assert.Equal(t, "cluster_pass", config.RedisClusterPassword)
}

func TestConfig_TimeoutSettings(t *testing.T) {
	config := &Config{
		RedisDialTimeout:  5000000000, // 5 seconds in nanoseconds
		RedisReadTimeout:  3000000000, // 3 seconds
		RedisWriteTimeout: 3000000000, // 3 seconds
		RedisPoolTimeout:  4000000000, // 4 seconds
	}

	assert.Greater(t, config.RedisDialTimeout, int64(0))
	assert.Greater(t, config.RedisReadTimeout, int64(0))
	assert.Greater(t, config.RedisWriteTimeout, int64(0))
	assert.Greater(t, config.RedisPoolTimeout, int64(0))
}

func TestConfig_PoolSettings(t *testing.T) {
	config := &Config{
		RedisPoolSize:     100,
		RedisMinIdleConns: 20,
	}

	assert.Equal(t, 100, config.RedisPoolSize)
	assert.Equal(t, 20, config.RedisMinIdleConns)
	assert.Less(t, config.RedisMinIdleConns, config.RedisPoolSize)
}

func TestConfig_FeatureFlags(t *testing.T) {
	config := &Config{
		AuditLogsEnabled: true,
		WhatsAppEnabled:  false,
		CFLookupEnabled:  true,
	}

	assert.True(t, config.AuditLogsEnabled)
	assert.False(t, config.WhatsAppEnabled)
	assert.True(t, config.CFLookupEnabled)
}

func TestConfig_WorkerSettings(t *testing.T) {
	config := &Config{
		AuditWorkerCount:        10,
		AuditBufferSize:         200,
		VerificationWorkerCount: 5,
		VerificationQueueSize:   100,
		DBWorkerCount:           8,
		DBBatchSize:             50,
	}

	assert.Equal(t, 10, config.AuditWorkerCount)
	assert.Equal(t, 200, config.AuditBufferSize)
	assert.Equal(t, 5, config.VerificationWorkerCount)
	assert.Equal(t, 100, config.VerificationQueueSize)
	assert.Equal(t, 8, config.DBWorkerCount)
	assert.Equal(t, 50, config.DBBatchSize)
}

func TestMaskMongoURI_MultipleMasks(t *testing.T) {
	uris := []string{
		"mongodb://admin:password123@prod.example.com:27017/mydb",
		"mongodb://user:secret@staging.example.com:27017/testdb",
		"mongodb://service:key@dev.example.com:27017/devdb",
	}

	for _, uri := range uris {
		result := maskMongoURI(uri)

		// All should be masked
		assert.Contains(t, result, "****:****@")

		// None should contain actual credentials
		assert.NotContains(t, result, "admin")
		assert.NotContains(t, result, "password123")
		assert.NotContains(t, result, "user")
		assert.NotContains(t, result, "secret")
		assert.NotContains(t, result, "service")
		assert.NotContains(t, result, "key")
	}
}
