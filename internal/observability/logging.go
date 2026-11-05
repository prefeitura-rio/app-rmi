package observability

import (
	"net/url"
	"strings"

	"github.com/prefeitura-rio/app-rmi/internal/logging"
	"github.com/prefeitura-rio/app-rmi/internal/pii"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// Logger returns the global safe logger instance
func Logger() *logging.SafeLogger {
	return logging.GetLogger()
}

// MaskCPF masks a CPF for logs and traces (LGPD).
// Accepts digits-only or formatted input (e.g. "123.456.789-01").
// Valid 11-digit CPF → "123.***.789-**"; invalid → "***.***.***-**".
func MaskCPF(cpf string) string {
	digits := make([]byte, 0, 11)
	for i := 0; i < len(cpf); i++ {
		if cpf[i] >= '0' && cpf[i] <= '9' {
			digits = append(digits, cpf[i])
		}
	}
	if len(digits) != 11 {
		return "***.***.***-**"
	}
	s := string(digits)
	return s[:3] + ".***." + s[6:9] + "-**"
}

// CPFField returns a zap field with a masked CPF (use instead of zap.String("cpf", ...)).
func CPFField(cpf string) zap.Field {
	return zap.String("cpf", MaskCPF(cpf))
}

// CPFAttr returns an OpenTelemetry attribute with a masked CPF
// (use instead of attribute.String("cpf", ...)).
func CPFAttr(cpf string) attribute.KeyValue {
	return attribute.String("cpf", MaskCPF(cpf))
}

// MaskPhone masks a phone number for logs and traces (LGPD), keeping only the
// DDI, the DDD and the last 4 digits. Non-digit characters are ignored.
//   - Brazilian with DDI (12-13 digits starting with 55): "5521987654321" → "+55 21 *****4321"
//   - Brazilian without DDI (10-11 digits, no leading "+"): "21987654321" → "21 *****4321"
//   - other lengths with at least 8 digits (unknown DDI/DDD): only the last 4 digits are kept
//   - fewer than 8 digits: "****"
//   - empty input: ""
func MaskPhone(phone string) string {
	if phone == "" {
		return ""
	}
	digits := make([]byte, 0, len(phone))
	for i := 0; i < len(phone); i++ {
		if phone[i] >= '0' && phone[i] <= '9' {
			digits = append(digits, phone[i])
		}
	}
	n := len(digits)
	if n < 8 {
		return "****"
	}
	s := string(digits)
	last4 := s[n-4:]
	hasDDI := strings.HasPrefix(strings.TrimSpace(phone), "+")
	switch {
	case (n == 12 || n == 13) && s[:2] == "55":
		return "+55 " + s[2:4] + " " + strings.Repeat("*", n-8) + last4
	case !hasDDI && (n == 10 || n == 11):
		return s[:2] + " " + strings.Repeat("*", n-6) + last4
	default:
		return strings.Repeat("*", n-4) + last4
	}
}

// PhoneField returns a zap field with a masked phone number
// (use instead of zap.String("phone_number", ...) or zap.String("phone", ...)).
func PhoneField(phone string) zap.Field {
	return zap.String("phone_number", MaskPhone(phone))
}

// PhonesField returns a zap field with a list of masked phone numbers
// (use instead of zap.Strings("phones", ...)).
func PhonesField(phones []string) zap.Field {
	masked := make([]string, len(phones))
	for i, p := range phones {
		masked[i] = MaskPhone(p)
	}
	return zap.Strings("phones", masked)
}

// PhoneAttr returns an OpenTelemetry attribute with a masked phone number
// (use instead of attribute.String("phone_number", ...)).
func PhoneAttr(phone string) attribute.KeyValue {
	return attribute.String("phone_number", MaskPhone(phone))
}

// MaskName masks a person's name for privacy, keeping the first name and the
// initial of every other name (e.g. "João Silva Santos" → "João S**** Santos").
// It works on runes, so accented initials ("Ângela") stay valid UTF-8.
// utils.MaskName delegates to this function.
func MaskName(fullName string) string {
	parts := strings.Fields(fullName)
	switch len(parts) {
	case 0:
		return ""
	case 1:
		// Single name - mask all but first character
		return maskNamePart(parts[0])
	case 2:
		// Two names - mask last name
		return parts[0] + " " + maskNamePart(parts[1])
	}

	// Three or more names - mask middle names
	masked := make([]string, 0, len(parts))
	masked = append(masked, parts[0])
	for _, part := range parts[1 : len(parts)-1] {
		masked = append(masked, maskNamePart(part))
	}
	masked = append(masked, parts[len(parts)-1])
	return strings.Join(masked, " ")
}

// maskNamePart keeps the first rune of a name and replaces every other rune by "*".
func maskNamePart(part string) string {
	runes := []rune(part)
	if len(runes) <= 1 {
		return part
	}
	return string(runes[0]) + strings.Repeat("*", len(runes)-1)
}

// NameField returns a zap field with a masked person name
// (use instead of zap.String("name", ...) or zap.String("nome", ...)).
func NameField(name string) zap.Field {
	return zap.String("name", MaskName(name))
}

// MaskCacheKey masks the CPF, phone and e-mail segments of a cache/Redis key,
// keeping its type and prefix (e.g. "citizen:write:12345678901" → "citizen:write:***").
// Use it whenever a cache key, a data key or a Redis key goes to a log or a span.
func MaskCacheKey(key string) string {
	return pii.MaskCacheKey(key)
}

// KeyField returns a zap field "key" with a masked cache/data key
// (use instead of zap.String("key", ...)).
func KeyField(key string) zap.Field {
	return zap.String("key", MaskCacheKey(key))
}

// CacheKeyField returns a zap field "cache_key" with a masked cache key
// (use instead of zap.String("cache_key", ...)).
func CacheKeyField(key string) zap.Field {
	return zap.String("cache_key", MaskCacheKey(key))
}

// MaskURLPath masks the CPF, phone and e-mail segments of a request path, keeping
// the route segments (e.g. "/v1/citizen/12345678901/wallet" → "/v1/citizen/***/wallet").
// Use it whenever a request path (c.Request.URL.Path) goes to a log, a span or a metric label.
func MaskURLPath(path string) string {
	return pii.MaskURLPath(path)
}

// MaskQuery masks the CPF, phone and e-mail values of a raw query string, keeping
// the parameter names (e.g. "cpf=12345678901&page=2" → "cpf=***&page=2").
func MaskQuery(rawQuery string) string {
	return pii.MaskQuery(rawQuery)
}

// MaskURL returns the request path and query with MaskURLPath and MaskQuery applied
// (use instead of u.String() in logs and spans). A nil URL returns "".
func MaskURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	masked := MaskURLPath(u.Path)
	if u.RawQuery != "" {
		masked += "?" + MaskQuery(u.RawQuery)
	}
	return masked
}

// PathField returns a zap field "path" with a masked request path
// (use instead of zap.String("path", c.Request.URL.Path)).
func PathField(path string) zap.Field {
	return zap.String("path", MaskURLPath(path))
}

// QueryField returns a zap field "query" with a masked raw query string
// (use instead of zap.String("query", c.Request.URL.RawQuery)).
func QueryField(rawQuery string) zap.Field {
	return zap.String("query", MaskQuery(rawQuery))
}

// MaskSensitiveData masks sensitive data in a map
func MaskSensitiveData(data map[string]interface{}) map[string]interface{} {
	sensitiveFields := []string{"nome_mae", "nome_pai", "cpf", "telefone"}
	masked := make(map[string]interface{})

	for k, v := range data {
		if contains(sensitiveFields, k) {
			masked[k] = "********"
		} else {
			masked[k] = v
		}
	}

	return masked
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
