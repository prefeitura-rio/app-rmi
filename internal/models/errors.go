package models

import "errors"

// Error constants for beta group operations
var (
	ErrInvalidGroupName    = errors.New("invalid group name")
	ErrGroupNameTooLong    = errors.New("group name too long (max 100 characters)")
	ErrGroupNotFound       = errors.New("beta group not found")
	ErrGroupNameExists     = errors.New("beta group name already exists")
	ErrGroupHasMembers     = errors.New("cannot delete group with members")
	ErrPhoneNotWhitelisted = errors.New("phone number not whitelisted")
	ErrPhoneAlreadyWhitelisted = errors.New("phone number already whitelisted")
	ErrInvalidGroupID      = errors.New("invalid group ID")
) 