package gym

import "context"

type Repository interface {
	Transaction(ctx context.Context, fn func(Repository) error) error

	// GymEntry operations
	ListGymEntries(ctx context.Context, userID string, filter ListFilter) ([]GymEntry, int64, error)
	GetGymEntryByID(ctx context.Context, userID, entryID string) (*GymEntry, error)
	CreateGymEntry(ctx context.Context, entry *GymEntry) error
	UpdateGymEntry(ctx context.Context, entry *GymEntry) error
	DeleteGymEntry(ctx context.Context, userID, entryID string) (bool, error)

	// Workout operations
	ListWorkouts(ctx context.Context, userID string, filter ListFilter) ([]Workout, int64, error)
	GetWorkoutByID(ctx context.Context, userID, workoutID string) (*Workout, error)
	CreateWorkout(ctx context.Context, workout *Workout) error
	UpdateWorkout(ctx context.Context, workout *Workout) error
	DeleteWorkout(ctx context.Context, userID, workoutID string) (bool, error)

	// WorkoutSet operations
	GetSetsByWorkoutIDs(ctx context.Context, workoutIDs []string) (map[string][]WorkoutSet, error)
	ReplaceWorkoutSets(ctx context.Context, workoutID string, sets []WorkoutSet) error

	// WorkoutTemplate operations
	ListTemplates(ctx context.Context, userID string) ([]WorkoutTemplate, error)
	GetTemplateByID(ctx context.Context, userID, templateID string) (*WorkoutTemplate, error)
	CreateTemplate(ctx context.Context, template *WorkoutTemplate) error
	UpdateTemplate(ctx context.Context, template *WorkoutTemplate) error
	DeleteTemplate(ctx context.Context, userID, templateID string) (bool, error)

	// TemplateExercise operations
	GetExercisesByTemplateIDs(ctx context.Context, templateIDs []string) (map[string][]TemplateExercise, error)
	ReplaceTemplateExercises(ctx context.Context, templateID string, exercises []TemplateExercise) error

	// Exercise list
	ListExercises(ctx context.Context, userID string) ([]string, error)
}
