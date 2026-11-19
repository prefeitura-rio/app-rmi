package middleware

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS returns middleware that emits Access-Control-* headers only for Origins
// present in allowedOrigins. Origins outside the allowlist do not receive CORS
// headers (gin-contrib/cors aborts those requests with 403). An empty allowlist
// rejects every cross-origin browser request (fail-closed). Same-origin and
// non-browser requests without an Origin header are unaffected.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	cfg := cors.DefaultConfig()
	if len(allowedOrigins) == 0 {
		cfg.AllowOriginFunc = func(string) bool { return false }
	} else {
		cfg.AllowOrigins = append([]string(nil), allowedOrigins...)
	}
	return cors.New(cfg)
}
