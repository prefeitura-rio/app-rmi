package pii

import (
	"net/url"
	"strings"
)

// MaskURLPath masks the personal data in a request path for logs, traces and
// metrics, keeping the route segments. The path is split on "/" and every segment
// that looks like a CPF, a phone number or an e-mail (same rule as MaskCacheKey)
// is replaced by "***":
//
//	"/v1/citizen/12345678901/phone/validate" → "/v1/citizen/***/phone/validate"
//	"/v1/memory/5521987654321/preferencias"  → "/v1/memory/***/preferencias"
//	"/v1/health"                             → "/v1/health"
//
// Any other identifier with 8 or more digits (e.g. a protocol number) is masked as well,
// and so is a segment that only embeds a CPF or a phone ("12345678901x", "cpf-12345678901"):
// see hasPersonalDataDigitRun.
func MaskURLPath(path string) string {
	if path == "" {
		return ""
	}
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if isPersonalDataSegment(segment) || hasPersonalDataDigitRun(segment) {
			segments[i] = CacheKeyRedacted
		}
	}
	return strings.Join(segments, "/")
}

// MaskQuery masks the personal data in a raw URL query string, keeping the
// parameter names and their order. Every value that, once unescaped, looks like a
// CPF, a phone number or an e-mail is replaced by "***":
//
//	"cpf=12345678901&page=2" → "cpf=***&page=2"
//	"email=fulano%40example.com" → "email=***"
//
// A parameter without "=" is treated as a value and masked by the same rule.
func MaskQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	pairs := strings.Split(rawQuery, "&")
	for i, pair := range pairs {
		name, value, hasValue := strings.Cut(pair, "=")
		if !hasValue {
			if isPersonalDataValue(name) {
				pairs[i] = CacheKeyRedacted
			}
			continue
		}
		if isPersonalDataValue(value) {
			pairs[i] = name + "=" + CacheKeyRedacted
		}
	}
	return strings.Join(pairs, "&")
}

// isPersonalDataValue unescapes a query value and checks it as a whole, by its
// ","/";"/":"/"/"/"|" separated parts and for an embedded CPF or phone, so
// "a,12345678901", "x%2F12345678901" and a value that fails to unescape are caught.
func isPersonalDataValue(value string) bool {
	unescaped, err := url.QueryUnescape(value)
	if err != nil {
		unescaped = value
	}
	if isPersonalDataSegment(unescaped) || hasPersonalDataDigitRun(unescaped) || hasPersonalDataDigitRun(value) {
		return true
	}
	for _, part := range strings.FieldsFunc(unescaped, func(r rune) bool {
		return r == ',' || r == ';' || r == ':' || r == '/' || r == '|'
	}) {
		if isPersonalDataSegment(part) {
			return true
		}
	}
	return false
}

// minEmbeddedDigits is the shortest digit run treated as an embedded CPF (11 digits)
// or phone with DDD (10 or 11 digits) inside a longer path segment or query value.
const minEmbeddedDigits = 10

// hasPersonalDataDigitRun reports whether s embeds a run of at least
// minEmbeddedDigits digits, ignoring the separators a formatted CPF or phone
// uses ("+", ".", "-", "(", ")" and spaces). Letters break the run, so ObjectIDs
// and UUIDs are kept unless they happen to hold 10 consecutive decimal digits.
func hasPersonalDataDigitRun(s string) bool {
	digits := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
			if digits >= minEmbeddedDigits {
				return true
			}
		case r == '+' || r == '.' || r == '-' || r == '(' || r == ')' || r == ' ':
		default:
			digits = 0
		}
	}
	return false
}
