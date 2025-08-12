package utils

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TraceOperation traces an operation with timing and attributes
func TraceOperation(ctx context.Context, operationName string, attributes map[string]interface{}) (context.Context, trace.Span, func()) {
	start := time.Now()

	// Convert attributes to OpenTelemetry attributes
	otelAttrs := make([]attribute.KeyValue, 0, len(attributes))
	for k, v := range attributes {
		switch val := v.(type) {
		case string:
			otelAttrs = append(otelAttrs, attribute.String(k, val))
		case int:
			otelAttrs = append(otelAttrs, attribute.Int(k, val))
		case int64:
			otelAttrs = append(otelAttrs, attribute.Int64(k, val))
		case bool:
			otelAttrs = append(otelAttrs, attribute.Bool(k, val))
		case float64:
			otelAttrs = append(otelAttrs, attribute.Float64(k, val))
		default:
			otelAttrs = append(otelAttrs, attribute.String(k, "unknown_type"))
		}
	}

	// Start span
	spanCtx, span := otel.Tracer("app-rmi").Start(ctx, operationName, trace.WithAttributes(otelAttrs...))

	// Return cleanup function that adds timing
	cleanup := func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("duration_ms", duration.Milliseconds()),
			attribute.String("duration", duration.String()),
		)
		span.End()
	}

	return spanCtx, span, cleanup
}

// TraceDatabaseOperation traces a database operation
func TraceDatabaseOperation(ctx context.Context, operation, collection string, filter interface{}) (context.Context, trace.Span, func()) {
	attributes := map[string]interface{}{
		"db.operation":  operation,
		"db.collection": collection,
		"db.system":     "mongodb",
	}

	if filter != nil {
		attributes["db.filter"] = "present"
	}

	return TraceOperation(ctx, "db."+operation, attributes)
}

// TraceCacheOperation traces a cache operation
func TraceCacheOperation(ctx context.Context, operation, key string) (context.Context, trace.Span, func()) {
	attributes := map[string]interface{}{
		"cache.operation": operation,
		"cache.key":       key,
		"cache.system":    "redis",
	}

	return TraceOperation(ctx, "cache."+operation, attributes)
}

// TraceHTTPOperation traces an HTTP operation
func TraceHTTPOperation(ctx context.Context, method, url, route string) (context.Context, trace.Span, func()) {
	attributes := map[string]interface{}{
		"http.method": method,
		"http.url":    url,
		"http.route":  route,
	}

	return TraceOperation(ctx, "http."+method, attributes)
}

// TraceValidationOperation traces a validation operation
func TraceValidationOperation(ctx context.Context, validationType, field string) (context.Context, trace.Span, func()) {
	attributes := map[string]interface{}{
		"validation.type":  validationType,
		"validation.field": field,
	}

	return TraceOperation(ctx, "validation."+validationType, attributes)
}

// TraceAuditOperation traces an audit operation
func TraceAuditOperation(ctx context.Context, action, resource, resourceID string) (context.Context, trace.Span, func()) {
	attributes := map[string]interface{}{
		"audit.action":      action,
		"audit.resource":    resource,
		"audit.resource_id": resourceID,
	}

	return TraceOperation(ctx, "audit."+action, attributes)
}

// TraceEndpointStep traces a specific step within an endpoint
func TraceEndpointStep(ctx context.Context, stepName string, attributes map[string]interface{}) (context.Context, trace.Span) {
	// Add endpoint context to step name
	stepAttributes := map[string]interface{}{
		"step.name": stepName,
		"step.type": "endpoint_operation",
	}

	// Merge with provided attributes
	for k, v := range attributes {
		stepAttributes[k] = v
	}

	// Convert attributes to OpenTelemetry attributes
	otelAttrs := make([]attribute.KeyValue, 0, len(stepAttributes))
	for k, v := range stepAttributes {
		switch val := v.(type) {
		case string:
			otelAttrs = append(otelAttrs, attribute.String(k, val))
		case int:
			otelAttrs = append(otelAttrs, attribute.Int(k, val))
		case int64:
			otelAttrs = append(otelAttrs, attribute.Int64(k, val))
		case bool:
			otelAttrs = append(otelAttrs, attribute.Bool(k, val))
		case float64:
			otelAttrs = append(otelAttrs, attribute.Float64(k, val))
		default:
			otelAttrs = append(otelAttrs, attribute.String(k, "unknown_type"))
		}
	}

	// Start span
	spanCtx, span := otel.Tracer("app-rmi").Start(ctx, "endpoint.step."+stepName, trace.WithAttributes(otelAttrs...))

	return spanCtx, span
}

// TraceInputParsing traces input parsing operations
func TraceInputParsing(ctx context.Context, inputType string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "parse_input", map[string]interface{}{
		"input.type": inputType,
	})
}

// TraceInputValidation traces input validation operations
func TraceInputValidation(ctx context.Context, validationType, field string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "validate_input", map[string]interface{}{
		"validation.type":  validationType,
		"validation.field": field,
	})
}

// TraceDatabaseFind traces database find operations
func TraceDatabaseFind(ctx context.Context, collection, filter string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "database_find", map[string]interface{}{
		"db.collection": collection,
		"db.filter":     filter,
		"db.operation":  "find",
	})
}

// TraceDatabaseCount traces database count operations
func TraceDatabaseCount(ctx context.Context, collection, filter string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "database_count", map[string]interface{}{
		"db.collection": collection,
		"db.filter":     filter,
		"db.operation":  "count",
	})
}

// TraceDatabaseTransaction traces database transaction operations
func TraceDatabaseTransaction(ctx context.Context, transactionType string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "database_transaction", map[string]interface{}{
		"transaction.type": transactionType,
		"db.operation":     "transaction",
	})
}

// TraceDatabaseUpdate traces database update operations
func TraceDatabaseUpdate(ctx context.Context, collection, filter string, upsert bool) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "database_update", map[string]interface{}{
		"db.collection": collection,
		"db.filter":     filter,
		"db.operation":  "update",
		"db.upsert":     upsert,
	})
}

// TraceDatabaseUpsert traces database upsert operations
func TraceDatabaseUpsert(ctx context.Context, collection, filter string) (context.Context, trace.Span) {
	return TraceDatabaseUpdate(ctx, collection, filter, true)
}

// TraceCacheInvalidation traces cache invalidation operations
func TraceCacheInvalidation(ctx context.Context, cacheKey string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "cache_invalidation", map[string]interface{}{
		"cache.key":       cacheKey,
		"cache.operation": "delete",
	})
}

// TraceCacheGet traces cache get operations
func TraceCacheGet(ctx context.Context, cacheKey string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "cache_get", map[string]interface{}{
		"cache.key":       cacheKey,
		"cache.operation": "get",
	})
}

// TraceCacheSet traces cache set operations
func TraceCacheSet(ctx context.Context, cacheKey string, ttl time.Duration) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "cache_set", map[string]interface{}{
		"cache.key":       cacheKey,
		"cache.operation": "set",
		"cache.ttl":       ttl.String(),
	})
}

// TraceDataComparison traces data comparison operations
func TraceDataComparison(ctx context.Context, comparisonType string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "data_comparison", map[string]interface{}{
		"comparison.type": comparisonType,
	})
}

// TraceResponseSerialization traces response serialization operations
func TraceResponseSerialization(ctx context.Context, responseType string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "serialize_response", map[string]interface{}{
		"response.type": responseType,
	})
}

// TraceAuditLogging traces audit logging operations
func TraceAuditLogging(ctx context.Context, action, resource string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "audit_logging", map[string]interface{}{
		"audit.action":   action,
		"audit.resource": resource,
	})
}

// TraceBusinessLogic traces business logic operations
func TraceBusinessLogic(ctx context.Context, logicType string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "business_logic", map[string]interface{}{
		"logic.type": logicType,
	})
}

// TraceExternalService traces external service calls
func TraceExternalService(ctx context.Context, serviceName, operation string) (context.Context, trace.Span) {
	return TraceEndpointStep(ctx, "external_service", map[string]interface{}{
		"service.name":      serviceName,
		"service.operation": operation,
	})
}

// AddTimingToSpan adds timing information to an existing span
func AddTimingToSpan(span trace.Span, startTime time.Time) {
	duration := time.Since(startTime)
	span.SetAttributes(
		attribute.Int64("duration_ms", duration.Milliseconds()),
		attribute.String("duration", duration.String()),
	)
}

// RecordErrorInSpan records an error in a span with additional context
func RecordErrorInSpan(span trace.Span, err error, context map[string]interface{}) {
	span.RecordError(err)

	// Add context attributes
	for k, v := range context {
		switch val := v.(type) {
		case string:
			span.SetAttributes(attribute.String(k, val))
		case int:
			span.SetAttributes(attribute.Int(k, val))
		case bool:
			span.SetAttributes(attribute.Bool(k, val))
		default:
			span.SetAttributes(attribute.String(k, "unknown_type"))
		}
	}
}

// AddSpanAttribute adds a single attribute to a span
func AddSpanAttribute(span trace.Span, key string, value interface{}) {
	switch val := value.(type) {
	case string:
		span.SetAttributes(attribute.String(key, val))
	case int:
		span.SetAttributes(attribute.Int(key, val))
	case int64:
		span.SetAttributes(attribute.Int64(key, val))
	case bool:
		span.SetAttributes(attribute.Bool(key, val))
	case float64:
		span.SetAttributes(attribute.Float64(key, val))
	default:
		span.SetAttributes(attribute.String(key, "unknown_type"))
	}
}
