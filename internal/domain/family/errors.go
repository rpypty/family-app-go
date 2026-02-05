package family

import "errors"

var (
	ErrFamilyNotFound       = errors.New("family not found")
	ErrFamilyCodeNotFound   = errors.New("family code not found")
	ErrAlreadyInFamily      = errors.New("already in family")
	ErrMemberNotFound       = errors.New("member not found")
	ErrNotOwner             = errors.New("not owner")
	ErrCannotRemoveOwner    = errors.New("cannot remove owner")
	ErrCodeGenerationFailed = errors.New("family code generation failed")
)
