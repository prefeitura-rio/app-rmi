package utils

import (
	"strings"
	"unicode"

	"github.com/prefeitura-rio/app-rmi/internal/observability"
)

// ExtractFirstName extracts the first name from a full name
func ExtractFirstName(fullName string) string {
	if fullName == "" {
		return ""
	}

	// Clean the name
	cleanName := strings.TrimSpace(fullName)

	// Split by spaces and common separators
	parts := strings.FieldsFunc(cleanName, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == '_'
	})

	if len(parts) == 0 {
		return ""
	}

	// Return the first part (first name)
	return strings.TrimSpace(parts[0])
}

// MaskName masks a full name for privacy (e.g., "João Silva Santos" -> "João S**** Santos").
// The implementation lives in observability so logs and traces can use it too.
func MaskName(fullName string) string {
	return observability.MaskName(fullName)
}
