package redisclient

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Client wraps a Redis client with OpenTelemetry tracing
type Client struct {
	cmdable redis.Cmdable
}

// NewClient creates a new traced Redis client for single Redis instance
func NewClient(client *redis.Client) *Client {
	return &Client{cmdable: client}
}

// NewClusterClient creates a new traced Redis client for Redis cluster
func NewClusterClient(client *redis.ClusterClient) *Client {
	return &Client{cmdable: client}
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

	cmd := c.cmdable.Get(ctx, key)
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

	cmd := c.cmdable.Set(ctx, key, value, expiration)
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

	cmd := c.cmdable.Del(ctx, keys...)
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

	cmd := c.cmdable.Ping(ctx)
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

	cmd := c.cmdable.Exists(ctx, keys...)
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

	cmd := c.cmdable.TTL(ctx, key)
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

	cmd := c.cmdable.Keys(ctx, pattern)
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

	cmd := c.cmdable.FlushDB(ctx)
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

	cmd := c.cmdable.Info(ctx, section...)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// PoolStats wraps Redis pool statistics with proper interface handling
func (c *Client) PoolStats() *redis.PoolStats {
	// Try to get pool stats from single client first
	if singleClient, ok := c.cmdable.(*redis.Client); ok {
		return singleClient.PoolStats()
	}
	
	// Try to get pool stats from cluster client
	if clusterClient, ok := c.cmdable.(*redis.ClusterClient); ok {
		return clusterClient.PoolStats()
	}
	
	// Return empty stats if neither type matches (should not happen)
	return &redis.PoolStats{}
}

// Pipeline wraps Redis pipeline with proper interface handling
func (c *Client) Pipeline() redis.Pipeliner {
	return c.cmdable.Pipeline()
}

// LLen wraps Redis LLen with comprehensive tracing
func (c *Client) LLen(ctx context.Context, key string) *redis.IntCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.llen",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.operation", "llen"),
			attribute.String("redis.client", "app-rmi"),
			attribute.String("redis.type", "list"),
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

	cmd := c.cmdable.LLen(ctx, key)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// LPush wraps Redis LPush with comprehensive tracing
func (c *Client) LPush(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.lpush",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.operation", "lpush"),
			attribute.String("redis.client", "app-rmi"),
			attribute.String("redis.type", "list"),
			attribute.Int("redis.value_count", len(values)),
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

	cmd := c.cmdable.LPush(ctx, key, values...)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}

// BRPop wraps Redis BRPop with comprehensive tracing
func (c *Client) BRPop(ctx context.Context, timeout time.Duration, keys ...string) *redis.StringSliceCmd {
	start := time.Now()
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.brpop",
		trace.WithAttributes(
			attribute.StringSlice("redis.keys", keys),
			attribute.String("redis.operation", "brpop"),
			attribute.String("redis.client", "app-rmi"),
			attribute.String("redis.type", "list"),
			attribute.String("redis.timeout", timeout.String()),
			attribute.Int("redis.key_count", len(keys)),
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

	cmd := c.cmdable.BRPop(ctx, timeout, keys...)
	if err := cmd.Err(); err != nil && err != redis.Nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("redis.error", err.Error()))
	} else {
		span.SetStatus(codes.Ok, "success")
	}
	return cmd
}
