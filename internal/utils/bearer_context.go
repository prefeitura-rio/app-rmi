package utils

import "context"

type bearerTokenContextKey struct{}

// ContextWithBearerToken stores a JWT access token on the context for downstream writers/sync.
func ContextWithBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, bearerTokenContextKey{}, token)
}

// BearerTokenFromContext returns the JWT previously stored with ContextWithBearerToken.
func BearerTokenFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if token, ok := ctx.Value(bearerTokenContextKey{}).(string); ok {
		return token
	}
	return ""
}
