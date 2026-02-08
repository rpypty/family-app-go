package gym

import "errors"

var (
	ErrGymEntryNotFound = errors.New("gym entry not found")
	ErrWorkoutNotFound  = errors.New("workout not found")
	ErrTemplateNotFound = errors.New("workout template not found")
)
