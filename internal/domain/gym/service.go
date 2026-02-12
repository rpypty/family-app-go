package gym

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GymEntry operations

func (s *Service) ListGymEntries(ctx context.Context, userID string, filter ListFilter) ([]GymEntry, int64, error) {
	return s.repo.ListGymEntries(ctx, userID, filter)
}

func (s *Service) CreateGymEntry(ctx context.Context, input CreateGymEntryInput) (*GymEntry, error) {
	if err := s.validateGymEntryInput(input.Exercise); err != nil {
		return nil, err
	}

	entryID, err := newUUID()
	if err != nil {
		return nil, err
	}

	entry := GymEntry{
		ID:       entryID,
		FamilyID: input.FamilyID,
		UserID:   input.UserID,
		Date:     input.Date,
		Exercise: strings.TrimSpace(input.Exercise),
		WeightKg: input.WeightKg,
		Reps:     input.Reps,
	}

	if err := s.repo.CreateGymEntry(ctx, &entry); err != nil {
		return nil, err
	}

	return &entry, nil
}

func (s *Service) UpdateGymEntry(ctx context.Context, input UpdateGymEntryInput) (*GymEntry, error) {
	if err := s.validateGymEntryInput(input.Exercise); err != nil {
		return nil, err
	}

	entry, err := s.repo.GetGymEntryByID(ctx, input.UserID, input.ID)
	if err != nil {
		return nil, err
	}

	entry.Date = input.Date
	entry.Exercise = strings.TrimSpace(input.Exercise)
	entry.WeightKg = input.WeightKg
	entry.Reps = input.Reps
	entry.UpdatedAt = time.Now().UTC()

	if err := s.repo.UpdateGymEntry(ctx, entry); err != nil {
		return nil, err
	}

	return entry, nil
}

func (s *Service) DeleteGymEntry(ctx context.Context, userID, entryID string) error {
	deleted, err := s.repo.DeleteGymEntry(ctx, userID, entryID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrGymEntryNotFound
	}
	return nil
}

// Workout operations

func (s *Service) ListWorkouts(ctx context.Context, userID string, filter ListFilter) ([]WorkoutWithSets, int64, error) {
	workouts, total, err := s.repo.ListWorkouts(ctx, userID, filter)
	if err != nil {
		return nil, 0, err
	}

	if len(workouts) == 0 {
		return []WorkoutWithSets{}, total, nil
	}

	workoutIDs := make([]string, 0, len(workouts))
	for _, workout := range workouts {
		workoutIDs = append(workoutIDs, workout.ID)
	}

	setsByWorkout, err := s.repo.GetSetsByWorkoutIDs(ctx, workoutIDs)
	if err != nil {
		return nil, 0, err
	}

	items := make([]WorkoutWithSets, 0, len(workouts))
	for _, workout := range workouts {
		items = append(items, WorkoutWithSets{
			Workout: workout,
			Sets:    setsByWorkout[workout.ID],
		})
	}

	return items, total, nil
}

func (s *Service) GetWorkoutByID(ctx context.Context, userID, workoutID string) (*WorkoutWithSets, error) {
	workout, err := s.repo.GetWorkoutByID(ctx, userID, workoutID)
	if err != nil {
		return nil, err
	}

	setsByWorkout, err := s.repo.GetSetsByWorkoutIDs(ctx, []string{workoutID})
	if err != nil {
		return nil, err
	}

	return &WorkoutWithSets{
		Workout: *workout,
		Sets:    setsByWorkout[workoutID],
	}, nil
}

func (s *Service) CreateWorkout(ctx context.Context, input CreateWorkoutInput) (*WorkoutWithSets, error) {
	if err := s.validateWorkoutInput(input.Name); err != nil {
		return nil, err
	}

	workoutID, err := newUUID()
	if err != nil {
		return nil, err
	}

	workout := Workout{
		ID:       workoutID,
		FamilyID: input.FamilyID,
		UserID:   input.UserID,
		Date:     input.Date,
		Name:     strings.TrimSpace(input.Name),
	}

	sets := make([]WorkoutSet, 0, len(input.Sets))
	for i, setInput := range input.Sets {
		if err := s.validateGymEntryInput(setInput.Exercise); err != nil {
			return nil, err
		}

		setID, err := newUUID()
		if err != nil {
			return nil, err
		}

		sets = append(sets, WorkoutSet{
			ID:        setID,
			WorkoutID: workoutID,
			Exercise:  strings.TrimSpace(setInput.Exercise),
			WeightKg:  setInput.WeightKg,
			Reps:      setInput.Reps,
			SetOrder:  i,
		})
	}

	err = s.repo.Transaction(ctx, func(tx Repository) error {
		if err := tx.CreateWorkout(ctx, &workout); err != nil {
			return err
		}

		if len(sets) > 0 {
			if err := tx.ReplaceWorkoutSets(ctx, workout.ID, sets); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &WorkoutWithSets{Workout: workout, Sets: sets}, nil
}

func (s *Service) UpdateWorkout(ctx context.Context, input UpdateWorkoutInput) (*WorkoutWithSets, error) {
	if err := s.validateWorkoutInput(input.Name); err != nil {
		return nil, err
	}

	var updated Workout
	var updatedSets []WorkoutSet

	err := s.repo.Transaction(ctx, func(tx Repository) error {
		workout, err := tx.GetWorkoutByID(ctx, input.UserID, input.ID)
		if err != nil {
			return err
		}

		workout.Date = input.Date
		workout.Name = strings.TrimSpace(input.Name)
		workout.UpdatedAt = time.Now().UTC()

		if err := tx.UpdateWorkout(ctx, workout); err != nil {
			return err
		}

		sets := make([]WorkoutSet, 0, len(input.Sets))
		for i, setInput := range input.Sets {
			if err := s.validateGymEntryInput(setInput.Exercise); err != nil {
				return err
			}

			setID, err := newUUID()
			if err != nil {
				return err
			}

			sets = append(sets, WorkoutSet{
				ID:        setID,
				WorkoutID: workout.ID,
				Exercise:  strings.TrimSpace(setInput.Exercise),
				WeightKg:  setInput.WeightKg,
				Reps:      setInput.Reps,
				SetOrder:  i,
			})
		}

		if err := tx.ReplaceWorkoutSets(ctx, workout.ID, sets); err != nil {
			return err
		}

		updated = *workout
		updatedSets = sets
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &WorkoutWithSets{Workout: updated, Sets: updatedSets}, nil
}

func (s *Service) DeleteWorkout(ctx context.Context, userID, workoutID string) error {
	deleted, err := s.repo.DeleteWorkout(ctx, userID, workoutID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrWorkoutNotFound
	}
	return nil
}

// WorkoutTemplate operations

func (s *Service) ListTemplates(ctx context.Context, userID string) ([]TemplateWithExercises, error) {
	templates, err := s.repo.ListTemplates(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(templates) == 0 {
		return []TemplateWithExercises{}, nil
	}

	templateIDs := make([]string, 0, len(templates))
	for _, template := range templates {
		templateIDs = append(templateIDs, template.ID)
	}

	exercisesByTemplate, err := s.repo.GetExercisesByTemplateIDs(ctx, templateIDs)
	if err != nil {
		return nil, err
	}

	items := make([]TemplateWithExercises, 0, len(templates))
	for _, template := range templates {
		items = append(items, TemplateWithExercises{
			WorkoutTemplate: template,
			Exercises:       exercisesByTemplate[template.ID],
		})
	}

	return items, nil
}

func (s *Service) GetTemplateByID(ctx context.Context, userID, templateID string) (*TemplateWithExercises, error) {
	template, err := s.repo.GetTemplateByID(ctx, userID, templateID)
	if err != nil {
		return nil, err
	}

	exercisesByTemplate, err := s.repo.GetExercisesByTemplateIDs(ctx, []string{templateID})
	if err != nil {
		return nil, err
	}

	return &TemplateWithExercises{
		WorkoutTemplate: *template,
		Exercises:       exercisesByTemplate[templateID],
	}, nil
}

func (s *Service) CreateTemplate(ctx context.Context, input CreateTemplateInput) (*TemplateWithExercises, error) {
	if err := s.validateTemplateName(input.Name); err != nil {
		return nil, err
	}

	templateID, err := newUUID()
	if err != nil {
		return nil, err
	}

	template := WorkoutTemplate{
		ID:       templateID,
		FamilyID: input.FamilyID,
		UserID:   input.UserID,
		Name:     strings.TrimSpace(input.Name),
	}

	exercises := make([]TemplateExercise, 0, len(input.Exercises))
	for i, exerciseInput := range input.Exercises {
		if err := s.validateGymEntryInput(exerciseInput.Name); err != nil {
			return nil, err
		}

		exerciseID, err := newUUID()
		if err != nil {
			return nil, err
		}

		exercises = append(exercises, TemplateExercise{
			ID:            exerciseID,
			TemplateID:    templateID,
			Name:          strings.TrimSpace(exerciseInput.Name),
			Reps:          exerciseInput.Reps,
			Sets:          exerciseInput.Sets,
			ExerciseOrder: i,
		})
	}

	err = s.repo.Transaction(ctx, func(tx Repository) error {
		if err := tx.CreateTemplate(ctx, &template); err != nil {
			return err
		}

		if len(exercises) > 0 {
			if err := tx.ReplaceTemplateExercises(ctx, template.ID, exercises); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &TemplateWithExercises{WorkoutTemplate: template, Exercises: exercises}, nil
}

func (s *Service) UpdateTemplate(ctx context.Context, input UpdateTemplateInput) (*TemplateWithExercises, error) {
	if err := s.validateTemplateName(input.Name); err != nil {
		return nil, err
	}

	var updated WorkoutTemplate
	var updatedExercises []TemplateExercise

	err := s.repo.Transaction(ctx, func(tx Repository) error {
		template, err := tx.GetTemplateByID(ctx, input.UserID, input.ID)
		if err != nil {
			return err
		}

		template.Name = strings.TrimSpace(input.Name)
		template.UpdatedAt = time.Now().UTC()

		if err := tx.UpdateTemplate(ctx, template); err != nil {
			return err
		}

		exercises := make([]TemplateExercise, 0, len(input.Exercises))
		for i, exerciseInput := range input.Exercises {
			if err := s.validateGymEntryInput(exerciseInput.Name); err != nil {
				return err
			}

			exerciseID, err := newUUID()
			if err != nil {
				return err
			}

			exercises = append(exercises, TemplateExercise{
				ID:            exerciseID,
				TemplateID:    template.ID,
				Name:          strings.TrimSpace(exerciseInput.Name),
				Reps:          exerciseInput.Reps,
				Sets:          exerciseInput.Sets,
				ExerciseOrder: i,
			})
		}

		if err := tx.ReplaceTemplateExercises(ctx, template.ID, exercises); err != nil {
			return err
		}

		updated = *template
		updatedExercises = exercises
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &TemplateWithExercises{WorkoutTemplate: updated, Exercises: updatedExercises}, nil
}

func (s *Service) DeleteTemplate(ctx context.Context, userID, templateID string) error {
	deleted, err := s.repo.DeleteTemplate(ctx, userID, templateID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrTemplateNotFound
	}
	return nil
}

// Exercise list

func (s *Service) ListExercises(ctx context.Context, userID string) ([]string, error) {
	return s.repo.ListExercises(ctx, userID)
}

// Validation helpers

func (s *Service) validateGymEntryInput(exercise string) error {
	if strings.TrimSpace(exercise) == "" {
		return fmt.Errorf("exercise is required")
	}
	return nil
}

func (s *Service) validateWorkoutInput(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("workout name is required")
	}
	return nil
}

func (s *Service) validateTemplateName(name string) error {
	const maxLen = 100
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("template name is required")
	}
	if len([]rune(name)) > maxLen {
		return fmt.Errorf("template name must be at most %d characters", maxLen)
	}
	return nil
}

// UUID generation

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
