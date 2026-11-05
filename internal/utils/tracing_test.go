package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceOperation(t *testing.T) {
	ctx := context.Background()
	attributes := map[string]interface{}{
		"string_attr":  "value",
		"int_attr":     42,
		"int64_attr":   int64(123),
		"bool_attr":    true,
		"float64_attr": 3.14,
		"unknown_attr": struct{}{},
	}

	spanCtx, span, cleanup := TraceOperation(ctx, "test_operation", attributes)

	if spanCtx == nil {
		t.Error("TraceOperation() returned nil context")
	}

	if span == nil {
		t.Error("TraceOperation() returned nil span")
	}

	if cleanup == nil {
		t.Fatal("TraceOperation() returned nil cleanup function")
	}

	// Execute cleanup
	cleanup()

	// Verify span behavior after cleanup - we just ensure it doesn't panic
	// The span may or may not be recording depending on the tracer implementation
	_ = span.IsRecording()
}

func TestTraceOperation_EmptyAttributes(t *testing.T) {
	ctx := context.Background()
	attributes := map[string]interface{}{}

	spanCtx, span, cleanup := TraceOperation(ctx, "test_operation", attributes)

	if spanCtx == nil {
		t.Error("TraceOperation() with empty attributes returned nil context")
	}

	if span == nil {
		t.Error("TraceOperation() with empty attributes returned nil span")
	}

	cleanup()
}

func TestTraceOperation_NilAttributes(t *testing.T) {
	ctx := context.Background()

	spanCtx, span, cleanup := TraceOperation(ctx, "test_operation", nil)

	if spanCtx == nil {
		t.Error("TraceOperation() with nil attributes returned nil context")
	}

	if span == nil {
		t.Error("TraceOperation() with nil attributes returned nil span")
	}

	cleanup()
}

func TestTraceDatabaseOperation(t *testing.T) {
	ctx := context.Background()

	spanCtx, span, cleanup := TraceDatabaseOperation(ctx, "find", "users", map[string]interface{}{"id": 123})

	if spanCtx == nil {
		t.Error("TraceDatabaseOperation() returned nil context")
	}

	if span == nil {
		t.Error("TraceDatabaseOperation() returned nil span")
	}

	cleanup()
}

func TestTraceDatabaseOperation_NilFilter(t *testing.T) {
	ctx := context.Background()

	spanCtx, span, cleanup := TraceDatabaseOperation(ctx, "find", "users", nil)

	if spanCtx == nil {
		t.Error("TraceDatabaseOperation() with nil filter returned nil context")
	}

	if span == nil {
		t.Error("TraceDatabaseOperation() with nil filter returned nil span")
	}

	cleanup()
}

func TestTraceCacheOperation(t *testing.T) {
	ctx := context.Background()

	spanCtx, span, cleanup := TraceCacheOperation(ctx, "get", "user:123")

	if spanCtx == nil {
		t.Error("TraceCacheOperation() returned nil context")
	}

	if span == nil {
		t.Error("TraceCacheOperation() returned nil span")
	}

	cleanup()
}

func TestTraceHTTPOperation(t *testing.T) {
	ctx := context.Background()

	spanCtx, span, cleanup := TraceHTTPOperation(ctx, "GET", "https://example.com/api", "/api/users")

	if spanCtx == nil {
		t.Error("TraceHTTPOperation() returned nil context")
	}

	if span == nil {
		t.Error("TraceHTTPOperation() returned nil span")
	}

	cleanup()
}

func TestTraceValidationOperation(t *testing.T) {
	ctx := context.Background()

	spanCtx, span, cleanup := TraceValidationOperation(ctx, "cpf", "cpf_field")

	if spanCtx == nil {
		t.Error("TraceValidationOperation() returned nil context")
	}

	if span == nil {
		t.Error("TraceValidationOperation() returned nil span")
	}

	cleanup()
}

func TestTraceAuditOperation(t *testing.T) {
	ctx := context.Background()

	spanCtx, span, cleanup := TraceAuditOperation(ctx, "CREATE", "user", "user123")

	if spanCtx == nil {
		t.Error("TraceAuditOperation() returned nil context")
	}

	if span == nil {
		t.Error("TraceAuditOperation() returned nil span")
	}

	cleanup()
}

func TestTraceEndpointStep(t *testing.T) {
	ctx := context.Background()
	attributes := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}

	spanCtx, span := TraceEndpointStep(ctx, "validate_input", attributes)

	if spanCtx == nil {
		t.Error("TraceEndpointStep() returned nil context")
	}

	if span == nil {
		t.Error("TraceEndpointStep() returned nil span")
	}

	span.End()
}

func TestTraceInputParsing(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceInputParsing(ctx, "json")

	if spanCtx == nil {
		t.Error("TraceInputParsing() returned nil context")
	}

	if span == nil {
		t.Error("TraceInputParsing() returned nil span")
	}

	span.End()
}

func TestTraceInputValidation(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceInputValidation(ctx, "email", "email_field")

	if spanCtx == nil {
		t.Error("TraceInputValidation() returned nil context")
	}

	if span == nil {
		t.Error("TraceInputValidation() returned nil span")
	}

	span.End()
}

func TestTraceDatabaseFind(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceDatabaseFind(ctx, "users", "id=123")

	if spanCtx == nil {
		t.Error("TraceDatabaseFind() returned nil context")
	}

	if span == nil {
		t.Error("TraceDatabaseFind() returned nil span")
	}

	span.End()
}

func TestTraceDatabaseCount(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceDatabaseCount(ctx, "users", "active=true")

	if spanCtx == nil {
		t.Error("TraceDatabaseCount() returned nil context")
	}

	if span == nil {
		t.Error("TraceDatabaseCount() returned nil span")
	}

	span.End()
}

func TestTraceDatabaseTransaction(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceDatabaseTransaction(ctx, "update_user")

	if spanCtx == nil {
		t.Error("TraceDatabaseTransaction() returned nil context")
	}

	if span == nil {
		t.Error("TraceDatabaseTransaction() returned nil span")
	}

	span.End()
}

func TestTraceDatabaseUpdate(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceDatabaseUpdate(ctx, "users", "id=123", false)

	if spanCtx == nil {
		t.Error("TraceDatabaseUpdate() returned nil context")
	}

	if span == nil {
		t.Error("TraceDatabaseUpdate() returned nil span")
	}

	span.End()
}

func TestTraceDatabaseUpsert(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceDatabaseUpsert(ctx, "users", "id=123")

	if spanCtx == nil {
		t.Error("TraceDatabaseUpsert() returned nil context")
	}

	if span == nil {
		t.Error("TraceDatabaseUpsert() returned nil span")
	}

	span.End()
}

func TestTraceCacheInvalidation(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceCacheInvalidation(ctx, "user:123")

	if spanCtx == nil {
		t.Error("TraceCacheInvalidation() returned nil context")
	}

	if span == nil {
		t.Error("TraceCacheInvalidation() returned nil span")
	}

	span.End()
}

func TestTraceCacheGet(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceCacheGet(ctx, "user:123")

	if spanCtx == nil {
		t.Error("TraceCacheGet() returned nil context")
	}

	if span == nil {
		t.Error("TraceCacheGet() returned nil span")
	}

	span.End()
}

func TestTraceCacheSet(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceCacheSet(ctx, "user:123", 5*time.Minute)

	if spanCtx == nil {
		t.Error("TraceCacheSet() returned nil context")
	}

	if span == nil {
		t.Error("TraceCacheSet() returned nil span")
	}

	span.End()
}

func TestTraceDataComparison(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceDataComparison(ctx, "user_data_diff")

	if spanCtx == nil {
		t.Error("TraceDataComparison() returned nil context")
	}

	if span == nil {
		t.Error("TraceDataComparison() returned nil span")
	}

	span.End()
}

func TestTraceResponseSerialization(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceResponseSerialization(ctx, "json")

	if spanCtx == nil {
		t.Error("TraceResponseSerialization() returned nil context")
	}

	if span == nil {
		t.Error("TraceResponseSerialization() returned nil span")
	}

	span.End()
}

func TestTraceAuditLogging(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceAuditLogging(ctx, "CREATE", "user")

	if spanCtx == nil {
		t.Error("TraceAuditLogging() returned nil context")
	}

	if span == nil {
		t.Error("TraceAuditLogging() returned nil span")
	}

	span.End()
}

func TestTraceBusinessLogic(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceBusinessLogic(ctx, "calculate_discount")

	if spanCtx == nil {
		t.Error("TraceBusinessLogic() returned nil context")
	}

	if span == nil {
		t.Error("TraceBusinessLogic() returned nil span")
	}

	span.End()
}

func TestTraceExternalService(t *testing.T) {
	ctx := context.Background()

	spanCtx, span := TraceExternalService(ctx, "payment_gateway", "charge")

	if spanCtx == nil {
		t.Error("TraceExternalService() returned nil context")
	}

	if span == nil {
		t.Error("TraceExternalService() returned nil span")
	}

	span.End()
}

func TestAddTimingToSpan(t *testing.T) {
	ctx := context.Background()
	_, span, cleanup := TraceOperation(ctx, "test", nil)
	defer cleanup()

	startTime := time.Now().Add(-100 * time.Millisecond)

	// Should not panic
	AddTimingToSpan(span, startTime)
}

func TestRecordErrorInSpan(t *testing.T) {
	ctx := context.Background()
	_, span, cleanup := TraceOperation(ctx, "test", nil)
	defer cleanup()

	err := errors.New("test error")
	context := map[string]interface{}{
		"error_code":    "TEST_ERROR",
		"retry_count":   3,
		"retry_allowed": true,
		"unknown_type":  struct{}{},
	}

	// Should not panic
	RecordErrorInSpan(span, err, context)
}

func TestAddSpanAttribute(t *testing.T) {
	ctx := context.Background()
	_, span, cleanup := TraceOperation(ctx, "test", nil)
	defer cleanup()

	tests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{"string attribute", "attr_string", "value"},
		{"int attribute", "attr_int", 42},
		{"int64 attribute", "attr_int64", int64(123)},
		{"bool attribute", "attr_bool", true},
		{"float64 attribute", "attr_float", 3.14},
		{"unknown attribute", "attr_unknown", struct{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			AddSpanAttribute(span, tt.key, tt.value)
		})
	}
}

func TestTraceOperation_WithCleanup(t *testing.T) {
	ctx := context.Background()
	attributes := map[string]interface{}{
		"test": "value",
	}

	_, span, cleanup := TraceOperation(ctx, "test_with_cleanup", attributes)

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	// Cleanup should add timing
	cleanup()

	// Verify span behavior after cleanup - we just ensure it doesn't panic
	// The span may or may not be recording depending on the tracer implementation
	_ = span.IsRecording()
}

func TestTraceOperation_MultipleTypes(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		operation  string
		attributes map[string]interface{}
	}{
		{
			name:      "database operation",
			operation: "db.query",
			attributes: map[string]interface{}{
				"collection": "users",
				"count":      int64(100),
			},
		},
		{
			name:      "cache operation",
			operation: "cache.get",
			attributes: map[string]interface{}{
				"key":   "user:123",
				"found": true,
			},
		},
		{
			name:      "http operation",
			operation: "http.request",
			attributes: map[string]interface{}{
				"method": "GET",
				"status": 200,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, span, cleanup := TraceOperation(ctx, tt.operation, tt.attributes)
			if span == nil {
				t.Error("TraceOperation() returned nil span")
			}
			cleanup()
		})
	}
}

func mockSpan() trace.Span {
	ctx := context.Background()
	_, span, _ := TraceOperation(ctx, "mock", nil)
	return span
}

func TestAddSpanAttribute_AllTypes(t *testing.T) {
	span := mockSpan()
	defer span.End()

	// Test all supported types
	AddSpanAttribute(span, "string_key", "string_value")
	AddSpanAttribute(span, "int_key", 42)
	AddSpanAttribute(span, "int64_key", int64(123))
	AddSpanAttribute(span, "bool_key", true)
	AddSpanAttribute(span, "float64_key", 3.14)
	AddSpanAttribute(span, "unknown_key", []int{1, 2, 3})
}

func recordSpanAttributes(t *testing.T, fn func(span trace.Span)) map[string]string {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	_, span := provider.Tracer("pii-test").Start(context.Background(), "pii")
	fn(span)
	span.End()

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	attrs := make(map[string]string)
	for _, kv := range ended[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	return attrs
}

func TestAddSpanAttribute_MasksPersonalData(t *testing.T) {
	attrs := recordSpanAttributes(t, func(span trace.Span) {
		AddSpanAttribute(span, "cpf", "12345678901")
		AddSpanAttribute(span, "input.cpf", "12345678901")
		AddSpanAttribute(span, "response.matched_cpf", "12345678901")
		AddSpanAttribute(span, "phone", "5521987654321")
		AddSpanAttribute(span, "full_phone_number", "5521987654321")
		AddSpanAttribute(span, "verification.phone_number", "21987654321")
		AddSpanAttribute(span, "national_number", "21987654321")
		AddSpanAttribute(span, "subscriber", "987654321")
		AddSpanAttribute(span, "input.name", "Maria Clara Souza Lima")
		AddSpanAttribute(span, "response.first_name", "Maria")
		AddSpanAttribute(span, "response.matched_name", "Maria Souza")
		AddSpanAttribute(span, "input.channel", "whatsapp")
		AddSpanAttribute(span, "input.group_name", "Beta Testers")
	})

	expected := map[string]string{
		"cpf":                       "123.***.789-**",
		"input.cpf":                 "123.***.789-**",
		"response.matched_cpf":      "123.***.789-**",
		"phone":                     "+55 21 *****4321",
		"full_phone_number":         "+55 21 *****4321",
		"verification.phone_number": "21 *****4321",
		"national_number":           "21 *****4321",
		"subscriber":                "*****4321",
		"input.name":                "Maria C**** S**** Lima",
		"response.first_name":       "M****",
		"response.matched_name":     "Maria S****",
		"input.channel":             "whatsapp",
		"input.group_name":          "Beta Testers",
	}
	for key, want := range expected {
		if got := attrs[key]; got != want {
			t.Errorf("attribute %q = %q, want %q", key, got, want)
		}
	}
}

func TestAddSpanAttribute_DropsBirthDate(t *testing.T) {
	attrs := recordSpanAttributes(t, func(span trace.Span) {
		AddSpanAttribute(span, "input.birth_date", "1990-01-31")
		AddSpanAttribute(span, "birthDate", "1990-01-31")
		AddSpanAttribute(span, "data_nascimento", "1990-01-31")
		AddSpanAttribute(span, "input.channel", "whatsapp")
	})

	for _, key := range []string{"input.birth_date", "birthDate", "data_nascimento"} {
		if v, ok := attrs[key]; ok {
			t.Errorf("birth date attribute %q must not be recorded, got %q", key, v)
		}
	}
	if attrs["input.channel"] != "whatsapp" {
		t.Errorf("non-personal attribute dropped: %v", attrs)
	}
}

func TestRecordErrorInSpan_MasksPersonalDataAndDropsBirthDate(t *testing.T) {
	attrs := recordSpanAttributes(t, func(span trace.Span) {
		RecordErrorInSpan(span, errors.New("boom"), map[string]interface{}{
			"phone":          "21987654321",
			"phone_number":   "5521987654321",
			"parse.phone":    "+5521987654321",
			"invalid_birth":  "1990-01-31",
			"nascimento":     "1990-01-31",
			"error.type":     "input_parsing",
			"operation.name": "x",
		})
	})

	if attrs["phone"] != "21 *****4321" || attrs["phone_number"] != "+55 21 *****4321" || attrs["parse.phone"] != "+55 21 *****4321" {
		t.Errorf("phone attributes not masked: %v", attrs)
	}
	if _, ok := attrs["invalid_birth"]; ok {
		t.Errorf("birth date attribute recorded: %v", attrs)
	}
	if _, ok := attrs["nascimento"]; ok {
		t.Errorf("birth date attribute recorded: %v", attrs)
	}
	if attrs["error.type"] != "input_parsing" {
		t.Errorf("non-personal attribute changed: %v", attrs)
	}
}

func TestTraceEndpointStep_MasksPersonalData(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(context.Background()) }()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)

	_, span := TraceEndpointStep(context.Background(), "step", map[string]interface{}{
		"phone_number": "5521987654321",
		"birth_date":   "1990-01-31",
		"db.filter":    "cpf",
	})
	span.End()

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(ended))
	}
	attrs := make(map[string]string)
	for _, kv := range ended[0].Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	if attrs["phone_number"] != "+55 21 *****4321" {
		t.Errorf("phone_number = %q, want masked", attrs["phone_number"])
	}
	if _, ok := attrs["birth_date"]; ok {
		t.Errorf("birth_date must not be recorded: %v", attrs)
	}
	if attrs["db.filter"] != "cpf" {
		t.Errorf("db.filter = %q, want cpf", attrs["db.filter"])
	}
}

func TestAddSpanAttribute_MasksCacheKeysAndDropsOTP(t *testing.T) {
	attrs := recordSpanAttributes(t, func(span trace.Span) {
		AddSpanAttribute(span, "cache.key", "memory:5521987654321:preferencias")
		AddSpanAttribute(span, "cache_key", "citizen:write:12345678901")
		AddSpanAttribute(span, "key", "12345678901")
		AddSpanAttribute(span, "input.code", "123456")
		AddSpanAttribute(span, "verification.otp", "654321")
		AddSpanAttribute(span, "input.valor", "fulano@example.com")
		AddSpanAttribute(span, "cache.operation", "get")
	})

	expected := map[string]string{
		"cache.key":        "memory:***:preferencias",
		"cache_key":        "citizen:write:***",
		"key":              "***",
		"input.code":       "[REDACTED]",
		"verification.otp": "[REDACTED]",
		"input.valor":      "[REDACTED]",
		"cache.operation":  "get",
	}
	for key, want := range expected {
		if got := attrs[key]; got != want {
			t.Errorf("attribute %q = %q, want %q", key, got, want)
		}
	}
}

func TestTraceCacheHelpers_MaskPersonalDataInKey(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(context.Background()) }()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer otel.SetTracerProvider(previous)

	ctx := context.Background()
	_, getSpan := TraceCacheGet(ctx, "memory:5521987654321:preferencias")
	getSpan.End()
	_, setSpan := TraceCacheSet(ctx, "citizen:12345678901", time.Minute)
	setSpan.End()
	_, delSpan := TraceCacheInvalidation(ctx, "user_config:12345678901")
	delSpan.End()
	_, opSpan, cleanup := TraceCacheOperation(ctx, "get", "memory_list:5521987654321")
	_ = opSpan
	cleanup()

	want := map[string]bool{
		"memory:***:preferencias": false,
		"citizen:***":             false,
		"user_config:***":         false,
		"memory_list:***":         false,
	}
	for _, span := range recorder.Ended() {
		for _, kv := range span.Attributes() {
			if string(kv.Key) != "cache.key" {
				continue
			}
			value := kv.Value.Emit()
			if _, ok := want[value]; !ok {
				t.Errorf("span %s cache.key = %q, want a masked key", span.Name(), value)
				continue
			}
			want[value] = true
		}
	}
	for value, seen := range want {
		if !seen {
			t.Errorf("cache.key %q not recorded", value)
		}
	}
}
