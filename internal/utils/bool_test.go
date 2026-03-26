package utils

import (
	"testing"
)

func TestBoolPtr(t *testing.T) {
	tests := []struct {
		name  string
		value bool
	}{
		{
			name:  "True value",
			value: true,
		},
		{
			name:  "False value",
			value: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BoolPtr(tt.value)

			if result == nil {
				t.Errorf("BoolPtr(%v) returned nil", tt.value)
				return
			}

			if *result != tt.value {
				t.Errorf("BoolPtr(%v) = %v, want %v", tt.value, *result, tt.value)
			}

			// Verify it's a new pointer each time
			result2 := BoolPtr(tt.value)
			if result == result2 {
				t.Errorf("BoolPtr(%v) returned same pointer twice", tt.value)
			}
		})
	}
}
