package config

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/prefeitura-rio/app-rmi/internal/redisclient"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

const indexCreationLockKey = "rmi:index_creation_lock"

// indexTestRedisDB isolates the index lock used here from the one in Redis DB 0, which
// the TestMain of other packages, running in parallel under go test ./..., holds.
const indexTestRedisDB = 10

// The connection settings are captured when the test binary starts: the
// LoadConfig tests of this package set and unset MONGODB_URI and REDIS_ADDR,
// so reading them inside a test would depend on the test order.
var (
	testMongoURI  = os.Getenv("MONGODB_URI")
	testRedisAddr = os.Getenv("REDIS_ADDR")
	testRedisPass = os.Getenv("REDIS_PASSWORD")
)

// indexTestCollections returns a Config whose collections are isolated for the
// index tests, so they never touch the collections used by other packages.
func indexTestCollections() *Config {
	return &Config{
		MongoDatabase:                  "rmi_test",
		CitizenCollection:              "idx_test_citizens",
		SelfDeclaredCollection:         "idx_test_self_declared",
		PhoneVerificationCollection:    "idx_test_phone_verifications",
		UserConfigCollection:           "idx_test_user_config",
		MaintenanceRequestCollection:   "idx_test_maintenance_requests",
		PhoneMappingCollection:         "idx_test_phone_mapping",
		OptInHistoryCollection:         "idx_test_opt_in_history",
		BetaGroupCollection:            "idx_test_beta_groups",
		AuditLogsCollection:            "idx_test_audit_logs",
		LegalEntityCollection:          "idx_test_legal_entities",
		PetCollection:                  "idx_test_pets",
		PetsSelfRegisteredCollection:   "idx_test_pets_self_registered",
		ChatMemoryCollection:           "idx_test_chat_memory",
		DepartmentCollection:           "idx_test_departments",
		NotificationCategoryCollection: "idx_test_notification_categories",
		CNAECollection:                 "idx_test_cnae",
		CFLookupCollection:             "idx_test_cf_lookups",
		MobilidadeVehicleCollection:    "idx_test_mobilidade_vehicles",
		MobilidadeConductorCollection:  "idx_test_mobilidade_conductors",
		MobilidadeBrandCollection:      "idx_test_mobilidade_brands",
		MobilidadeModelCollection:      "idx_test_mobilidade_models",
		IndexMaintenanceInterval:       time.Hour,
	}
}

func indexTestCollectionNames(cfg *Config) []string {
	return []string{
		cfg.CitizenCollection, cfg.SelfDeclaredCollection, cfg.PhoneVerificationCollection,
		cfg.UserConfigCollection, cfg.MaintenanceRequestCollection, cfg.PhoneMappingCollection,
		cfg.OptInHistoryCollection, cfg.BetaGroupCollection, cfg.AuditLogsCollection,
		cfg.LegalEntityCollection, cfg.PetCollection, cfg.PetsSelfRegisteredCollection,
		cfg.ChatMemoryCollection, cfg.DepartmentCollection, cfg.NotificationCategoryCollection,
		cfg.CNAECollection, cfg.CFLookupCollection, cfg.MobilidadeVehicleCollection,
		cfg.MobilidadeConductorCollection, cfg.MobilidadeBrandCollection, cfg.MobilidadeModelCollection,
	}
}

// setupIndexTest connects to the test MongoDB and Redis, points the package
// globals at isolated collections and drops them before and after the test.
func setupIndexTest(t *testing.T) (*Config, *redisclient.Client) {
	t.Helper()
	mongoURI := testMongoURI
	if mongoURI == "" {
		t.Skip("Skipping index test: MONGODB_URI not set")
	}
	redisAddr := testRedisAddr
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("connect mongo: %v", err)
	}
	rc := redisclient.NewClient(redis.NewClient(&redis.Options{Addr: redisAddr, Password: testRedisPass, DB: indexTestRedisDB}))
	if err := rc.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect redis: %v", err)
	}

	prevDB, prevConfig, prevRedis := MongoDB, AppConfig, getRedis()
	cfg := indexTestCollections()
	MongoDB = client.Database(cfg.MongoDatabase)
	AppConfig = cfg
	setRedis(rc)

	drop := func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()
		for _, name := range indexTestCollectionNames(cfg) {
			_ = MongoDB.Collection(name).Drop(dropCtx)
		}
		_ = rc.Del(dropCtx, indexCreationLockKey).Err()
	}
	drop()
	t.Cleanup(func() {
		drop()
		MongoDB, AppConfig = prevDB, prevConfig
		setRedis(prevRedis)
		_ = client.Disconnect(context.Background())
	})
	return cfg, rc
}

func listIndexes(t *testing.T, collection string) map[string]bson.M {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cursor, err := MongoDB.Collection(collection).Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes of %s: %v", collection, err)
	}
	var indexes []bson.M
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("decode indexes of %s: %v", collection, err)
	}
	byName := make(map[string]bson.M, len(indexes))
	for _, idx := range indexes {
		byName[idx["name"].(string)] = idx
	}
	return byName
}

func requireIndex(t *testing.T, collection, name string, unique bool) bson.M {
	t.Helper()
	indexes := listIndexes(t, collection)
	idx, ok := indexes[name]
	if !ok {
		t.Fatalf("index %s missing on %s; got %v", name, collection, indexes)
	}
	gotUnique, _ := idx["unique"].(bool)
	if gotUnique != unique {
		t.Fatalf("index %s on %s: unique = %v, want %v", name, collection, gotUnique, unique)
	}
	return idx
}

func TestEnsureIndexes_CreatesRequiredIndexesIdempotently(t *testing.T) {
	cfg, rc := setupIndexTest(t)

	// Two runs: the first creates every index, the second finds them and must not fail.
	for run := 1; run <= 2; run++ {
		if err := ensureIndexes(); err != nil {
			t.Fatalf("run %d: ensureIndexes: %v", run, err)
		}
	}

	// The distributed lock is released after each run.
	exists, err := rc.Exists(context.Background(), indexCreationLockKey).Result()
	if err != nil {
		t.Fatalf("exists lock: %v", err)
	}
	if exists != 0 {
		t.Fatalf("index creation lock was not released")
	}

	// Data integrity: one document per CPF in the citizen-facing collections.
	requireIndex(t, cfg.CitizenCollection, "cpf_1", true)
	requireIndex(t, cfg.SelfDeclaredCollection, "cpf_1", true)
	requireIndex(t, cfg.UserConfigCollection, "cpf_1", true)
	requireIndex(t, cfg.CFLookupCollection, "cpf_1", true)
	requireIndex(t, cfg.PhoneVerificationCollection, "cpf_1_phone_number_1", true)
	requireIndex(t, cfg.BetaGroupCollection, "name_1", true)
	requireIndex(t, cfg.ChatMemoryCollection, "phone_number_1_memory_name_1", true)
	requireIndex(t, cfg.LegalEntityCollection, "idx_cnpj", true)
	requireIndex(t, cfg.DepartmentCollection, "idx_department_cd_ua", true)
	requireIndex(t, cfg.MobilidadeVehicleCollection, "idx_mobilidade_vehicles_registration_number", true)
	requireIndex(t, cfg.MobilidadeConductorCollection, "idx_mobilidade_conductors_vehicle_cpf_active", true)

	// Lookup indexes of the phone flows (opt-in, quarantine, bind).
	for _, name := range []string{"phone_number_1", "cpf_1", "phone_number_1_status_1", "quarantine_until_1", "beta_group_id_1"} {
		requireIndex(t, cfg.PhoneMappingCollection, name, false)
	}
	requireIndex(t, cfg.OptInHistoryCollection, "phone_number_1", false)
	requireIndex(t, cfg.OptInHistoryCollection, "cpf_1", false)
	requireIndex(t, cfg.MaintenanceRequestCollection, "protocolo_1", false)
	requireIndex(t, cfg.PetCollection, "idx_pets_cpf", false)
	requireIndex(t, cfg.PetsSelfRegisteredCollection, "idx_self_registered_pets_cpf", false)
	requireIndex(t, cfg.NotificationCategoryCollection, "idx_notification_category_active_order", false)
	requireIndex(t, cfg.CNAECollection, "idx_cnae_classe", false)
	requireIndex(t, cfg.MobilidadeBrandCollection, "idx_mobilidade_brands_name", false)
	requireIndex(t, cfg.MobilidadeModelCollection, "idx_mobilidade_models_brand_name", false)

	// TTL indexes: verification codes expire at expires_at, audit logs after one year.
	ttl := requireIndex(t, cfg.PhoneVerificationCollection, "expires_at_1", false)
	if v, ok := ttl["expireAfterSeconds"]; !ok || toInt64(v) != 0 {
		t.Fatalf("expires_at_1 expireAfterSeconds = %v, want 0", v)
	}
	auditTTL := requireIndex(t, cfg.AuditLogsCollection, "timestamp_ttl", false)
	if v := toInt64(auditTTL["expireAfterSeconds"]); v != 365*24*60*60 {
		t.Fatalf("timestamp_ttl expireAfterSeconds = %d, want one year", v)
	}

	// Every collection got at least one index besides _id.
	for _, name := range indexTestCollectionNames(cfg) {
		if got := len(listIndexes(t, name)); got < 2 {
			t.Fatalf("collection %s has %d indexes, want at least 2", name, got)
		}
	}
}

func TestEnsureIndexes_UniqueCPFRejectsDuplicateCitizen(t *testing.T) {
	cfg, _ := setupIndexTest(t)
	if err := ensureIndexes(); err != nil {
		t.Fatalf("ensureIndexes: %v", err)
	}

	ctx := context.Background()
	coll := MongoDB.Collection(cfg.SelfDeclaredCollection)
	if _, err := coll.InsertOne(ctx, bson.M{"cpf": "11144477735"}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err := coll.InsertOne(ctx, bson.M{"cpf": "11144477735"})
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("second self_declared with the same CPF: err = %v, want duplicate key", err)
	}
}

func TestEnsureIndexes_SkipsWhenAnotherInstanceHoldsTheLock(t *testing.T) {
	cfg, rc := setupIndexTest(t)
	ctx := context.Background()

	if err := rc.Set(ctx, indexCreationLockKey, "other-pod", time.Minute).Err(); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	if err := ensureIndexes(); err != nil {
		t.Fatalf("ensureIndexes: %v", err)
	}

	if _, ok := listIndexes(t, cfg.CitizenCollection)["cpf_1"]; ok {
		t.Fatalf("indexes were created while another instance held the lock")
	}
	holder, err := rc.Get(ctx, indexCreationLockKey).Result()
	if err != nil {
		t.Fatalf("get lock: %v", err)
	}
	if holder != "other-pod" {
		t.Fatalf("lock held by another instance was overwritten or released: %q", holder)
	}
}

func TestEnsureIndexes_WithoutRedisStillCreatesIndexes(t *testing.T) {
	cfg, _ := setupIndexTest(t)
	setRedis(nil)

	if err := ensureIndexes(); err != nil {
		t.Fatalf("ensureIndexes: %v", err)
	}
	requireIndex(t, cfg.CitizenCollection, "cpf_1", true)
	requireIndex(t, cfg.PhoneMappingCollection, "phone_number_1", false)
}

func TestEnsureIndexes_ExistingIndexesAreKept(t *testing.T) {
	cfg, _ := setupIndexTest(t)
	ctx := context.Background()

	// A collection that already has part of its indexes (e.g. a previous release).
	if _, err := MongoDB.Collection(cfg.PhoneMappingCollection).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "phone_number", Value: 1}},
		Options: options.Index().SetName("phone_number_1"),
	}); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	if err := ensureIndexes(); err != nil {
		t.Fatalf("ensureIndexes: %v", err)
	}
	for _, name := range []string{"phone_number_1", "cpf_1", "phone_number_1_status_1", "quarantine_until_1", "beta_group_id_1"} {
		requireIndex(t, cfg.PhoneMappingCollection, name, false)
	}
}

func TestIsIndexBuildInProgress_NoBuildRunning(t *testing.T) {
	cfg, _ := setupIndexTest(t)
	if isIndexBuildInProgress(context.Background(), cfg.CitizenCollection, "cpf_1", zap.NewNop()) {
		t.Fatalf("reported an index build in progress with none running")
	}
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return -1
	}
}
