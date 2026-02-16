package gym

import "time"

// GymEntry represents a single set in a workout
type GymEntry struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	UserID    string    `gorm:"type:uuid;index;not null"`
	Date      time.Time `gorm:"type:date;not null"`
	Exercise  string    `gorm:"not null"`
	WeightKg  float64   `gorm:"type:numeric(8,2);not null"`
	Reps      int       `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// Workout represents a collection of sets grouped together
type Workout struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	UserID    string    `gorm:"type:uuid;index;not null"`
	Date      time.Time `gorm:"type:date;not null"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// WorkoutSet represents a single set within a workout
type WorkoutSet struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	WorkoutID string    `gorm:"type:uuid;index;not null"`
	Exercise  string    `gorm:"not null"`
	WeightKg  float64   `gorm:"type:numeric(8,2);not null"`
	Reps      int       `gorm:"not null"`
	SetOrder  int       `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// WorkoutTemplate represents a reusable workout template
type WorkoutTemplate struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	UserID    string    `gorm:"type:uuid;index;not null"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TemplateSet represents a single set within a workout template (similar to WorkoutSet)
type TemplateSet struct {
	ID         string    `gorm:"type:uuid;primaryKey"`
	TemplateID string    `gorm:"type:uuid;index;not null"`
	Exercise   string    `gorm:"not null"`
	WeightKg   float64   `gorm:"type:numeric(8,2);not null"`
	Reps       int       `gorm:"not null"`
	SetOrder   int       `gorm:"not null;default:0"`
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}

// WorkoutWithSets combines Workout with its WorkoutSets
type WorkoutWithSets struct {
	Workout
	Sets []WorkoutSet
}

// TemplateWithSets combines WorkoutTemplate with its sets
type TemplateWithSets struct {
	WorkoutTemplate
	Sets []TemplateSet
}

// ListFilter defines filtering options for listing gym entries/workouts
type ListFilter struct {
	From   *time.Time
	To     *time.Time
	Limit  int
	Offset int
}

// CreateGymEntryInput represents input for creating a gym entry
type CreateGymEntryInput struct {
	UserID   string
	Date     time.Time
	Exercise string
	WeightKg float64
	Reps     int
}

// UpdateGymEntryInput represents input for updating a gym entry
type UpdateGymEntryInput struct {
	ID       string
	UserID   string
	Date     time.Time
	Exercise string
	WeightKg float64
	Reps     int
}

// CreateWorkoutInput represents input for creating a workout
type CreateWorkoutInput struct {
	UserID     string
	Date       time.Time
	Name       string
	Sets       []CreateWorkoutSetInput
	TemplateID string // Optional: if provided, copy sets from template
}

// CreateWorkoutSetInput represents input for creating a workout set
type CreateWorkoutSetInput struct {
	Exercise string
	WeightKg float64
	Reps     int
}

// UpdateWorkoutInput represents input for updating a workout
type UpdateWorkoutInput struct {
	ID     string
	UserID string
	Date   time.Time
	Name   string
	Sets   []CreateWorkoutSetInput
}

// CreateTemplateInput represents input for creating a workout template
type CreateTemplateInput struct {
	UserID string
	Name   string
	Sets   []CreateTemplateSetInput
}

// CreateTemplateSetInput represents input for creating a template set
type CreateTemplateSetInput struct {
	Exercise string
	WeightKg float64
	Reps     int
}

// UpdateTemplateInput represents input for updating a workout template
type UpdateTemplateInput struct {
	ID     string
	UserID string
	Name   string
	Sets   []CreateTemplateSetInput
}
