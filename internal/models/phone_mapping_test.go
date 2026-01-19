package models

import (
	"testing"
)

func TestMappingStatusConstants(t *testing.T) {
	// Verify constant values are as expected
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "MappingStatusActive",
			constant: MappingStatusActive,
			expected: "active",
		},
		{
			name:     "MappingStatusBlocked",
			constant: MappingStatusBlocked,
			expected: "blocked",
		},
		{
			name:     "MappingStatusQuarantined",
			constant: MappingStatusQuarantined,
			expected: "quarantined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestChannelConstants(t *testing.T) {
	// Verify constant values are as expected
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "ChannelWhatsApp",
			constant: ChannelWhatsApp,
			expected: "whatsapp",
		},
		{
			name:     "ChannelWeb",
			constant: ChannelWeb,
			expected: "web",
		},
		{
			name:     "ChannelMobile",
			constant: ChannelMobile,
			expected: "mobile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}
