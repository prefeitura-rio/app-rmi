package redisclient

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Client wraps a Redis client with OpenTelemetry tracing
type Client struct {
	*redis.Client
}

// NewClient creates a new traced Redis client
func NewClient(client *redis.Client) *Client {
	return &Client{Client: client}
}

// Get wraps Redis Get with comprehensive tracing
func (c *Client) Get(ctx context.Context, key string) *redis.StringCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.get",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.operation", "get"),
			attribute.String("redis.client", "app-rmi"),
			attribute.String("redis.type", "string"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.Get(ctx, key)
	if err := cmd.Err(); err != nil && err != redis.Nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// Set wraps Redis Set with comprehensive tracing
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.set",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.operation", "set"),
			attribute.String("redis.expiration", expiration.String()),
			attribute.String("redis.client", "app-rmi"),
			attribute.String("redis.type", "string"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.Set(ctx, key, value, expiration)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// Del wraps Redis Del with comprehensive tracing
func (c *Client) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.del",
		trace.WithAttributes(
			attribute.StringSlice("redis.keys", keys),
			attribute.String("redis.operation", "del"),
			attribute.Int("redis.key_count", len(keys)),
			attribute.String("redis.client", "app-rmi"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.Del(ctx, keys...)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// Ping wraps Redis Ping with comprehensive tracing
func (c *Client) Ping(ctx context.Context) *redis.StatusCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.ping",
		trace.WithAttributes(
			attribute.String("redis.operation", "ping"),
			attribute.String("redis.client", "app-rmi"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.Ping(ctx)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// Exists wraps Redis Exists with comprehensive tracing
func (c *Client) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.exists",
		trace.WithAttributes(
			attribute.StringSlice("redis.keys", keys),
			attribute.String("redis.operation", "exists"),
			attribute.Int("redis.key_count", len(keys)),
			attribute.String("redis.client", "app-rmi"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.Exists(ctx, keys...)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// TTL wraps Redis TTL with comprehensive tracing
func (c *Client) TTL(ctx context.Context, key string) *redis.DurationCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.ttl",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.operation", "ttl"),
			attribute.String("redis.client", "app-rmi"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.TTL(ctx, key)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// Keys wraps Redis Keys with comprehensive tracing
func (c *Client) Keys(ctx context.Context, pattern string) *redis.StringSliceCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.keys",
		trace.WithAttributes(
			attribute.String("redis.pattern", pattern),
			attribute.String("redis.operation", "keys"),
			attribute.String("redis.client", "app-rmi"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.Keys(ctx, pattern)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// FlushDB wraps Redis FlushDB with comprehensive tracing
func (c *Client) FlushDB(ctx context.Context) *redis.StatusCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.flushdb",
		trace.WithAttributes(
			attribute.String("redis.operation", "flushdb"),
			attribute.String("redis.client", "app-rmi"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.FlushDB(ctx)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// Info wraps Redis Info with comprehensive tracing
func (c *Client) Info(ctx context.Context, section ...string) *redis.StringCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.info",
		trace.WithAttributes(
			attribute.StringSlice("redis.sections", section),
			attribute.String("redis.operation", "info"),
			attribute.String("redis.client", "app-rmi"),
		),
	)
	defer func() {
		duration := time.Since(start)
		span.SetAttributes(
			attribute.Int64("redis.duration_ms", duration.Milliseconds()),
			attribute.String("redis.duration", duration.String()),
		)
		span.End()
	}()

	cmd := c.Client.Info(ctx, section...)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}
