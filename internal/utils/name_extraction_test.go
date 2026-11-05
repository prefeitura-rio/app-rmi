package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractFirstName(t *testing.T) {
	tests := []struct {
		name     string
		fullName string
		want     string
	}{
		{
			name:     "Simple two-part name",
			fullName: "João Silva",
			want:     "João",
		},
		{
			name:     "Three-part name",
			fullName: "Maria Silva Santos",
			want:     "Maria",
		},
		{
			name:     "Name with middle initials",
			fullName: "Pedro A. B. Costa",
			want:     "Pedro",
		},
		{
			name:     "Single name",
			fullName: "Madonna",
			want:     "Madonna",
		},
		{
			name:     "Name with leading/trailing spaces",
			fullName: "  Carlos  Souza  ",
			want:     "Carlos",
		},
		{
			name:     "Hyphenated first name",
			fullName: "Ana-Paula Silva",
			want:     "Ana",
		},
		{
			name:     "Name with underscores",
			fullName: "João_Pedro Santos",
			want:     "João",
		},
		{
			name:     "Empty string",
			fullName: "",
			want:     "",
		},
		{
			name:     "Only spaces",
			fullName: "   ",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFirstName(tt.fullName)
			assert.Equal(t, tt.want, result, "ExtractFirstName(%q) should return %q", tt.fullName, tt.want)
		})
	}
}

func TestExtractFirstName_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Multiple spaces between names", "João     Silva", "João"},
		{"Tab characters", "João\tSilva", "João"},
		{"Mixed separators", "João-Pedro_Silva Santos", "João"},
		{"Numbers in name", "João123 Silva", "João123"},
		{"Only separators", "---___", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFirstName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMaskName(t *testing.T) {
	tests := []struct {
		name     string
		fullName string
		want     string
	}{
		{
			name:     "Two-part name",
			fullName: "João Silva",
			want:     "João S****",
		},
		{
			name:     "Three-part name",
			fullName: "João Silva Santos",
			want:     "João S**** Santos",
		},
		{
			name:     "Single name",
			fullName: "Madonna",
			want:     "M******",
		},
		{
			name:     "Single letter name",
			fullName: "A",
			want:     "A",
		},
		{
			name:     "Four-part name",
			fullName: "João Pedro Silva Santos",
			want:     "João P**** S**** Santos",
		},
		{
			name:     "Empty string",
			fullName: "",
			want:     "",
		},
		{
			name:     "Name with spaces",
			fullName: "  Maria  Costa  ",
			want:     "Maria C****",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskName(tt.fullName)
			assert.Equal(t, tt.want, result, "MaskName(%q) should return %q", tt.fullName, tt.want)
		})
	}
}

func TestMaskName_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Two letter last name", "João Si", "João S*"},
		{"Single letter middle and last", "A B C", "A B C"},
		{"Five part name", "João Pedro Silva Santos Costa", "João P**** S**** S***** Costa"},
		{"Only spaces input", "     ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
