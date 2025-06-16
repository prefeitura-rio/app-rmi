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

// Get wraps Redis Get with tracing
func (c *Client) Get(ctx context.Context, key string) *redis.StringCmd {
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.get",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.operation", "get"),
		),
	)
	defer span.End()

	cmd := c.Client.Get(ctx, key)
	if err := cmd.Err(); err != nil && err != redis.Nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return cmd
}

// Set wraps Redis Set with tracing
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.set",
		trace.WithAttributes(
			attribute.String("redis.key", key),
			attribute.String("redis.operation", "set"),
			attribute.String("redis.expiration", expiration.String()),
		),
	)
	defer span.End()

	cmd := c.Client.Set(ctx, key, value, expiration)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return cmd
}

// Del wraps Redis Del with tracing
func (c *Client) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.del",
		trace.WithAttributes(
			attribute.StringSlice("redis.keys", keys),
			attribute.String("redis.operation", "del"),
		),
	)
	defer span.End()

	cmd := c.Client.Del(ctx, keys...)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return cmd
}

// Ping wraps Redis Ping with tracing
func (c *Client) Ping(ctx context.Context) *redis.StatusCmd {
	ctx, span := otel.Tracer("redis").Start(ctx, "redis.ping",
		trace.WithAttributes(
			attribute.String("redis.operation", "ping"),
		),
	)
	defer span.End()

	cmd := c.Client.Ping(ctx)
	if err := cmd.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return cmd
}
