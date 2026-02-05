package family

import "errors"

var (
	ErrFamilyNotFound       = errors.New("family not found")
	ErrFamilyCodeNotFound   = errors.New("family code not found")
	ErrAlreadyInFamily      = errors.New("already in family")
	ErrCodeGenerationFailed = errors.New("family code generation failed")
)
