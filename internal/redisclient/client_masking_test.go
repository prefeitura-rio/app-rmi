package redisclient

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestSpanAttributesMaskPersonalDataInKeys checks that the CPF and the phone carried by
// Redis keys never reach the span attributes (LGPD). The client points to a closed port:
// the command fails, but the span is still recorded with its attributes.
func TestSpanAttributesMaskPersonalDataInKeys(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(context.Background()) }()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)

	raw := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 50 * time.Millisecond,
		MaxRetries:  -1,
	})
	defer func() { _ = raw.Close() }()
	client := NewClient(raw)

	ctx := context.Background()
	_ = client.Get(ctx, "citizen:write:12345678901").Err()
	_ = client.Set(ctx, "memory:5521987654321:preferencias", "v", time.Minute).Err()
	_ = client.Del(ctx, "citizen:12345678901", "phone_mapping:5521987654321").Err()
	_ = client.Keys(ctx, "phone_verification:552198765*").Err()

	expected := map[string]map[string]string{
		"redis.get":  {"redis.key": "citizen:write:***"},
		"redis.set":  {"redis.key": "memory:***:preferencias"},
		"redis.del":  {"redis.keys": `["citizen:***","phone_mapping:***"]`},
		"redis.keys": {"redis.pattern": "phone_verification:***"},
	}

	seen := make(map[string]bool)
	for _, span := range recorder.Ended() {
		want, ok := expected[span.Name()]
		if !ok {
			continue
		}
		seen[span.Name()] = true
		attrs := make(map[string]string)
		for _, kv := range span.Attributes() {
			attrs[string(kv.Key)] = kv.Value.Emit()
		}
		for key, value := range want {
			if attrs[key] != value {
				t.Errorf("span %s attribute %s = %q, want %q", span.Name(), key, attrs[key], value)
			}
		}
	}
	for name := range expected {
		if !seen[name] {
			t.Errorf("span %s not recorded", name)
		}
	}
}
