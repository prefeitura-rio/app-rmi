package config

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.uber.org/zap"
)

func TestEnsureMaintenanceRequestIndex_CreatesCPFAndProtocoloIndexes(t *testing.T) {
	mongoURI := os.Getenv("MONGODB_URI")
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

	collectionName := "maintenance_requests_index_test"
	MongoDB = client.Database("rmi_test")
	AppConfig = &Config{MaintenanceRequestCollection: collectionName}
	coll := MongoDB.Collection(collectionName)
	_ = coll.Drop(ctx)
	t.Cleanup(func() { _ = coll.Drop(context.Background()) })

	if _, err := coll.InsertOne(ctx, bson.M{"cpf": "11144477735", "protocolo": "RIO-12345678-9"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for run := 1; run <= 2; run++ {
		if err := ensureMaintenanceRequestIndex(ctx, zap.NewNop()); err != nil {
			t.Fatalf("run %d: ensureMaintenanceRequestIndex: %v", run, err)
		}
	}

	cursor, err := coll.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	var indexes []bson.M
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("decode indexes: %v", err)
	}

	found := map[string]bson.M{}
	for _, idx := range indexes {
		found[idx["name"].(string)] = idx
	}
	for name, field := range map[string]string{"cpf_1": "cpf", "protocolo_1": "protocolo"} {
		idx, ok := found[name]
		if !ok {
			t.Fatalf("index %s missing; indexes = %v", name, indexes)
		}
		key, _ := idx["key"].(bson.M)
		if _, ok := key[field]; !ok || len(key) != 1 {
			t.Fatalf("index %s key = %v, want {%s: 1}", name, key, field)
		}
		if unique, _ := idx["unique"].(bool); unique {
			t.Fatalf("index %s must not be unique: a protocolo may group several chamados", name)
		}
	}

	if _, err := coll.InsertOne(ctx, bson.M{"cpf": "11144477735", "protocolo": "RIO-12345678-9"}); err != nil {
		t.Fatalf("second chamado of the same protocolo rejected: %v", err)
	}
}
