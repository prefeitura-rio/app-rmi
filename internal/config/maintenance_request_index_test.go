package config

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func TestEnsureMaintenanceRequestIndex_CreatesCPFAndProtocoloIndexes(t *testing.T) {
	mongoURI := testMongoURI
	if mongoURI == "" {
		t.Skip("Skipping maintenance request index test: MONGODB_URI not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Disconnect(context.Background()) }()

	prevDB, prevConfig := MongoDB, AppConfig
	t.Cleanup(func() { MongoDB, AppConfig = prevDB, prevConfig })

	legacyName := "maintenance_requests_index_test"
	sgrcName := "maintenance_requests_sgrc_index_test"
	MongoDB = client.Database("rmi_test")
	AppConfig = &Config{
		MaintenanceRequestCollection:     legacyName,
		MaintenanceRequestSGRCCollection: sgrcName,
	}
	legacyColl := MongoDB.Collection(legacyName)
	sgrcColl := MongoDB.Collection(sgrcName)
	_ = legacyColl.Drop(ctx)
	_ = sgrcColl.Drop(ctx)
	t.Cleanup(func() {
		_ = legacyColl.Drop(context.Background())
		_ = sgrcColl.Drop(context.Background())
	})

	if _, err := legacyColl.InsertOne(ctx, bson.M{"cpf": "11144477735", "protocolo": "RIO-12345678-9"}); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}
	if _, err := sgrcColl.InsertOne(ctx, bson.M{
		"id_chamado": int64(1),
		"protocolos": []bson.M{{"codigo_protocolo": "RIO-12345678-9", "cpf": "11144477735"}},
	}); err != nil {
		t.Fatalf("seed sgrc: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := ensureMaintenanceRequestIndex(ctx, zap.NewNop()); err != nil {
			t.Fatalf("run %d: ensureMaintenanceRequestIndex: %v", run, err)
		}
	}

	assertNonUniqueIndex(t, legacyColl, ctx, "cpf_1", "cpf")
	assertNonUniqueIndex(t, legacyColl, ctx, "protocolo_1", "protocolo")
	assertNonUniqueIndex(t, sgrcColl, ctx, "protocolos.cpf_1", "protocolos.cpf")
	assertNonUniqueIndex(t, sgrcColl, ctx, "protocolos.codigo_protocolo_1", "protocolos.codigo_protocolo")

	if _, err := legacyColl.InsertOne(ctx, bson.M{"cpf": "11144477735", "protocolo": "RIO-12345678-9"}); err != nil {
		t.Fatalf("second chamado of the same protocolo rejected on legacy: %v", err)
	}
	if _, err := sgrcColl.InsertOne(ctx, bson.M{
		"id_chamado": int64(2),
		"protocolos": []bson.M{{"codigo_protocolo": "RIO-12345678-9", "cpf": "22233344456"}},
	}); err != nil {
		t.Fatalf("second chamado of the same protocolo rejected on sgrc: %v", err)
	}
}

func assertNonUniqueIndex(t *testing.T, coll *mongo.Collection, ctx context.Context, name, field string) {
	t.Helper()
	cursor, err := coll.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes on %s: %v", coll.Name(), err)
	}
	var indexes []bson.M
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("decode indexes on %s: %v", coll.Name(), err)
	}
	found := map[string]bson.M{}
	for _, idx := range indexes {
		found[idx["name"].(string)] = idx
	}
	idx, ok := found[name]
	if !ok {
		t.Fatalf("index %s missing on %s; indexes = %v", name, coll.Name(), indexes)
	}
	key, _ := idx["key"].(bson.M)
	if _, ok := key[field]; !ok || len(key) != 1 {
		t.Fatalf("index %s key = %v, want {%s: 1}", name, key, field)
	}
	if unique, _ := idx["unique"].(bool); unique {
		t.Fatalf("index %s must not be unique: a protocolo may group several chamados", name)
	}
}
