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
		"db.operation": operation,
		"db.collection": collection,
		"db.system": "mongodb",
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
		"cache.key": key,
		"cache.system": "redis",
	}
	
	return TraceOperation(ctx, "cache."+operation, attributes)
}

// TraceHTTPOperation traces an HTTP operation
func TraceHTTPOperation(ctx context.Context, method, url, route string) (context.Context, trace.Span, func()) {
	attributes := map[string]interface{}{
		"http.method": method,
		"http.url": url,
		"http.route": route,
	}
	
	return TraceOperation(ctx, "http."+method, attributes)
}

// TraceValidationOperation traces a validation operation
func TraceValidationOperation(ctx context.Context, validationType, field string) (context.Context, trace.Span, func()) {
	attributes := map[string]interface{}{
		"validation.type": validationType,
		"validation.field": field,
	}
	
	return TraceOperation(ctx, "validation."+validationType, attributes)
}

// TraceAuditOperation traces an audit operation
func TraceAuditOperation(ctx context.Context, action, resource, resourceID string) (context.Context, trace.Span, func()) {
	attributes := map[string]interface{}{
		"audit.action": action,
		"audit.resource": resource,
		"audit.resource_id": resourceID,
	}
	
	return TraceOperation(ctx, "audit."+action, attributes)
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
		}
	}
}
