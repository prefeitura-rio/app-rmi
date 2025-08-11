package utils

import (
	"strings"
	"unicode"
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

// MaskName masks a full name for privacy (e.g., "João Silva Santos" -> "João S*** Santos")
func MaskName(fullName string) string {
	if fullName == "" {
		return ""
	}

	parts := strings.Fields(strings.TrimSpace(fullName))
	if len(parts) == 0 {
		return ""
	}

	if len(parts) == 1 {
		// Single name - mask all but first character
		name := parts[0]
		if len(name) <= 1 {
			return name
		}
		return name[:1] + strings.Repeat("*", len(name)-1)
	}

	if len(parts) == 2 {
		// Two names - mask middle
		firstName := parts[0]
		lastName := parts[1]
		if len(lastName) <= 1 {
			return firstName + " " + lastName
		}
		return firstName + " " + lastName[:1] + strings.Repeat("*", len(lastName)-1)
	}

	// Three or more names - mask middle names
	firstName := parts[0]
	lastName := parts[len(parts)-1]

	middleMask := ""
	for i := 1; i < len(parts)-1; i++ {
		if len(parts[i]) > 0 {
			middleMask += parts[i][:1] + strings.Repeat("*", len(parts[i])-1) + " "
		}
	}
	middleMask = strings.TrimSpace(middleMask)

	return firstName + " " + middleMask + " " + lastName
}

// MaskCPF masks a CPF for privacy (e.g., "45049725810" -> "450***25810")
func MaskCPF(cpf string) string {
	if len(cpf) != 11 {
		return cpf
	}
	return cpf[:3] + "***" + cpf[6:]
}
