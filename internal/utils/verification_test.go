package utils

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateVerificationCode(t *testing.T) {
	t.Run("Generates 6-digit code", func(t *testing.T) {
		code := GenerateVerificationCode()
		assert.Len(t, code, 6, "Code should be 6 digits")
	})

	t.Run("Generates only numeric characters", func(t *testing.T) {
		code := GenerateVerificationCode()
		for i, c := range code {
			assert.True(t, c >= '0' && c <= '9',
				"Character at position %d (%c) should be numeric", i, c)
		}
	})

	t.Run("Generates different codes", func(t *testing.T) {
		// Generate multiple codes and check they're not all the same
		codes := make(map[string]bool)
		iterations := 50

		for i := 0; i < iterations; i++ {
			code := GenerateVerificationCode()
			codes[code] = true
		}

		// With random generation, we should get at least some different codes
		assert.Greater(t, len(codes), 1,
			"Should generate different codes (got %d unique out of %d)", len(codes), iterations)
	})

	t.Run("Code is valid numeric string", func(t *testing.T) {
		code := GenerateVerificationCode()

		// Should be parseable as a number
		var parsed int
		_, err := fmt.Sscanf(code, "%d", &parsed)
		require.NoError(t, err, "Code should be parseable as number")
	})
}

func TestGenerateVerificationCode_Format(t *testing.T) {
	for i := 0; i < 10; i++ {
		code := GenerateVerificationCode()

		assert.Len(t, code, 6, "All codes should be 6 digits")

		// Verify it's a valid number
		num, err := strconv.Atoi(code)
		require.NoError(t, err, "Code should be numeric")
		assert.GreaterOrEqual(t, num, 0, "Code should be non-negative")
		assert.LessOrEqual(t, num, 999999, "Code should be at most 999999")
	}
}

func TestGenerateVerificationCode_Distribution(t *testing.T) {
	// Test that generated codes have good distribution
	codes := make(map[string]bool)
	iterations := 100

	for i := 0; i < iterations; i++ {
		code := GenerateVerificationCode()
		codes[code] = true
	}

	// Should have high diversity (at least 80% unique)
	uniqueRatio := float64(len(codes)) / float64(iterations)
	assert.Greater(t, uniqueRatio, 0.8,
		"Should have high code diversity (got %.2f%% unique)", uniqueRatio*100)
}

func TestGenerateVerificationCode_NoLeadingZeros(t *testing.T) {
	// Generate several codes and check format
	for i := 0; i < 20; i++ {
		code := GenerateVerificationCode()

		// All codes should be 6 characters, even if they start with 0
		assert.Len(t, code, 6, "Code should preserve leading zeros")

		// Examples: "000000", "000123", "100000", etc.
		for _, c := range code {
			assert.True(t, c >= '0' && c <= '9', "All characters should be digits")
		}
	}
}
