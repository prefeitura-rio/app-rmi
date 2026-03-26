package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateUUID(t *testing.T) {
	// Generate multiple UUIDs and check properties
	t.Run("Generates non-empty UUID", func(t *testing.T) {
		uuid := GenerateUUID()
		assert.NotEmpty(t, uuid, "GenerateUUID() should not return empty string")
	})

	t.Run("Generates hex string", func(t *testing.T) {
		uuid := GenerateUUID()
		// UUID should be 32 hex characters (16 bytes * 2)
		assert.Len(t, uuid, 32, "UUID should be 32 characters")

		// Check if all characters are valid hex
		for _, c := range uuid {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
				"UUID should only contain hex characters, found: %c", c)
		}
	})

	t.Run("Generates unique UUIDs", func(t *testing.T) {
		// Generate multiple UUIDs and check they're different
		uuids := make(map[string]bool)
		iterations := 100

		for i := 0; i < iterations; i++ {
			uuid := GenerateUUID()
			assert.False(t, uuids[uuid], "GenerateUUID() should not generate duplicate UUID: %s", uuid)
			uuids[uuid] = true
		}

		assert.Len(t, uuids, iterations, "Should have %d unique UUIDs", iterations)
	})
}

func TestGenerateUUID_Format(t *testing.T) {
	uuid := GenerateUUID()

	require.NotEmpty(t, uuid)
	assert.Len(t, uuid, 32)

	// Verify it's valid hexadecimal
	for i, c := range uuid {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"Character at position %d (%c) should be valid hex", i, c)
	}
}

func TestGenerateUUID_Consistency(t *testing.T) {
	// Generate multiple UUIDs and verify they all have the same format
	for i := 0; i < 10; i++ {
		uuid := GenerateUUID()
		assert.Len(t, uuid, 32, "All UUIDs should have length 32")
		assert.NotEmpty(t, uuid, "UUID should not be empty")
	}
}

func TestGenerateUUID_LargeScale(t *testing.T) {
	// Test uniqueness with larger sample
	uuids := make(map[string]bool)
	iterations := 1000

	for i := 0; i < iterations; i++ {
		uuid := GenerateUUID()
		assert.False(t, uuids[uuid], "Should not generate duplicate UUIDs")
		uuids[uuid] = true
	}

	assert.Len(t, uuids, iterations, "Should generate %d unique UUIDs", iterations)
}
