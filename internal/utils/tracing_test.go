package utils

import (
	"context"
	"errors"
	"testing"
	"time"

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
