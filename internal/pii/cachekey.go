// Package pii holds dependency-free masking helpers for personal data (LGPD)
// that low-level packages (e.g. redisclient) can use without importing
// observability, which would create an import cycle through config.
package pii

import "strings"

// CacheKeyRedacted replaces a cache key segment that carries personal data.
const CacheKeyRedacted = "***"

// MaskCacheKey masks the personal data in a Redis/cache key for logs and traces,
// keeping the type and prefix segments. The key is split on ":" and every segment
// that looks like a CPF, a phone number or an e-mail is replaced by "***":
//
//	"citizen:write:12345678901"        → "citizen:write:***"
//	"memory:5521987654321:preferencias" → "memory:***:preferencias"
//	"12345678901"                       → "***"
//
// A segment is personal data when it contains "@" or when, ignoring the
// separators "+", ".", "-", "(", ")", "/", spaces and the glob characters "*"
// and "?", it has only digits and at least 8 of them. Identifiers such as ObjectIDs, UUIDs and names are kept.
func MaskCacheKey(key string) string {
	if key == "" {
		return ""
	}
	segments := strings.Split(key, ":")
	for i, segment := range segments {
		if isPersonalDataSegment(segment) {
			segments[i] = CacheKeyRedacted
		}
	}
	return strings.Join(segments, ":")
}

// MaskCacheKeys masks every key of the slice with MaskCacheKey.
func MaskCacheKeys(keys []string) []string {
	if keys == nil {
		return nil
	}
	masked := make([]string, len(keys))
	for i, key := range keys {
		masked[i] = MaskCacheKey(key)
	}
	return masked
}

func isPersonalDataSegment(segment string) bool {
	if strings.Contains(segment, "@") {
		return true
	}
	digits := 0
	for _, r := range segment {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' || r == '.' || r == '-' || r == '(' || r == ')' || r == '/' || r == ' ' || r == '*' || r == '?':
		default:
			return false
		}
	}
	return digits >= 8
}
