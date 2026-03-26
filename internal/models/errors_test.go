package models

import (
	"errors"
	"testing"
)

func TestErrorConstants(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		expectedMsg   string
		shouldBeError bool
	}{
		{
			name:          "ErrInvalidGroupName",
			err:           ErrInvalidGroupName,
			expectedMsg:   "invalid group name",
			shouldBeError: true,
		},
		{
			name:          "ErrGroupNameTooLong",
			err:           ErrGroupNameTooLong,
			expectedMsg:   "group name too long (max 100 characters)",
			shouldBeError: true,
		},
		{
			name:          "ErrGroupNotFound",
			err:           ErrGroupNotFound,
			expectedMsg:   "beta group not found",
			shouldBeError: true,
		},
		{
			name:          "ErrGroupNameExists",
			err:           ErrGroupNameExists,
			expectedMsg:   "beta group name already exists",
			shouldBeError: true,
		},
		{
			name:          "ErrGroupHasMembers",
			err:           ErrGroupHasMembers,
			expectedMsg:   "cannot delete group with members",
			shouldBeError: true,
		},
		{
			name:          "ErrPhoneNotWhitelisted",
			err:           ErrPhoneNotWhitelisted,
			expectedMsg:   "phone number not whitelisted",
			shouldBeError: true,
		},
		{
			name:          "ErrPhoneAlreadyWhitelisted",
			err:           ErrPhoneAlreadyWhitelisted,
			expectedMsg:   "phone number already whitelisted",
			shouldBeError: true,
		},
		{
			name:          "ErrInvalidGroupID",
			err:           ErrInvalidGroupID,
			expectedMsg:   "invalid group ID",
			shouldBeError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil && tt.shouldBeError {
				t.Errorf("%s is nil, expected an error", tt.name)
				return
			}

			if tt.err.Error() != tt.expectedMsg {
				t.Errorf("%s error message = %q, want %q", tt.name, tt.err.Error(), tt.expectedMsg)
			}

			// Verify errors can be compared using errors.Is
			if !errors.Is(tt.err, tt.err) {
				t.Errorf("%s should match itself using errors.Is", tt.name)
			}
		})
	}
}

func TestErrorUniqueness(t *testing.T) {
	// Verify that each error constant is distinct
	errorVars := []error{
		ErrInvalidGroupName,
		ErrGroupNameTooLong,
		ErrGroupNotFound,
		ErrGroupNameExists,
		ErrGroupHasMembers,
		ErrPhoneNotWhitelisted,
		ErrPhoneAlreadyWhitelisted,
		ErrInvalidGroupID,
	}

	for i, err1 := range errorVars {
		for j, err2 := range errorVars {
			if i != j && err1 == err2 {
				t.Errorf("Error at index %d and %d are the same: %v", i, j, err1)
			}
		}
	}
}
