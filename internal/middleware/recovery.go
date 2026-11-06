package middleware

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prefeitura-rio/app-rmi/internal/observability"
	"go.uber.org/zap"
)

// Recovery recovers from a panic in a later handler, logs it and answers 500.
//
// It replaces gin.Recovery(), which writes the raw request (request line with the
// path, e.g. "GET /v1/citizen/<cpf>/wallet HTTP/1.1", and the headers) to stderr.
// Here the panic goes to the structured logger only, with the route (c.FullPath()),
// the masked path and the panic value with CPF, phone and e-mail tokens masked; the
// request itself is never dumped. The stack trace comes from the logger (zap adds it
// to Error entries in production) and holds no argument values.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			fields := []zap.Field{
				zap.String("route", c.FullPath()),
				observability.PathField(c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.String("request_id", c.GetString("RequestID")),
				zap.String("panic_type", fmt.Sprintf("%T", recovered)),
				zap.String("panic", maskPanicValue(recovered)),
			}

			if err, ok := recovered.(error); ok && isBrokenConnection(err) {
				// The client is gone: no status can be written, and it is not a bug.
				observability.Logger().Warn("client connection closed during request", fields...)
				_ = c.Error(err)
				c.Abort()
				return
			}

			observability.Logger().Error("panic recovered", fields...)
			if c.Writer.Written() {
				c.Abort()
				return
			}
			c.AbortWithStatus(http.StatusInternalServerError)
		}()
		c.Next()
	}
}

// maskPanicValue renders a recovered panic value with every whitespace-separated
// token that carries a CPF, a phone or an e-mail replaced by "***" (same rule as
// observability.MaskURLPath, applied to each "/"-separated part of the token), since
// a panic message may format request data.
func maskPanicValue(recovered any) string {
	var text string
	switch v := recovered.(type) {
	case error:
		text = v.Error()
	case string:
		text = v
	default:
		text = fmt.Sprint(v)
	}
	tokens := strings.Fields(text)
	for i, token := range tokens {
		tokens[i] = observability.MaskURLPath(token)
	}
	return strings.Join(tokens, " ")
}

// isBrokenConnection reports whether the panic is a write to a client that already
// closed the connection (broken pipe or connection reset), as gin.Recovery() does.
func isBrokenConnection(err error) bool {
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return false
	}
	var syscallErr *os.SyscallError
	if !errors.As(opErr, &syscallErr) {
		return false
	}
	message := strings.ToLower(syscallErr.Error())
	return strings.Contains(message, "broken pipe") || strings.Contains(message, "connection reset by peer")
}
