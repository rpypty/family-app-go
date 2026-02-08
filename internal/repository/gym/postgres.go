package gym

import (
	"context"
	"errors"

	gymdomain "family-app-go/internal/domain/gym"
	"gorm.io/gorm"
)

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Transaction(ctx context.Context, fn func(gymdomain.Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&PostgresRepository{db: tx})
	})
}

// GymEntry operations

func (r *PostgresRepository) ListGymEntries(ctx context.Context, familyID string, filter gymdomain.ListFilter) ([]gymdomain.GymEntry, int64, error) {
	query := r.db.WithContext(ctx).Model(&gymdomain.GymEntry{}).Where("family_id = ?", familyID)

	if filter.From != nil {
		query = query.Where("date >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("date <= ?", *filter.To)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order("date desc, created_at desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []gymdomain.GymEntry
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *PostgresRepository) GetGymEntryByID(ctx context.Context, familyID, entryID string) (*gymdomain.GymEntry, error) {
	var entry gymdomain.GymEntry
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND id = ?", familyID, entryID).
		First(&entry).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gymdomain.ErrGymEntryNotFound
		}
		return nil, err
	}
	return &entry, nil
}

func (r *PostgresRepository) CreateGymEntry(ctx context.Context, entry *gymdomain.GymEntry) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

func (r *PostgresRepository) UpdateGymEntry(ctx context.Context, entry *gymdomain.GymEntry) error {
	return r.db.WithContext(ctx).
		Model(&gymdomain.GymEntry{}).
		Where("id = ? AND family_id = ?", entry.ID, entry.FamilyID).
		Updates(map[string]interface{}{
			"date":       entry.Date,
			"exercise":   entry.Exercise,
			"weight_kg":  entry.WeightKg,
			"reps":       entry.Reps,
			"updated_at": entry.UpdatedAt,
		}).Error
}

func (r *PostgresRepository) DeleteGymEntry(ctx context.Context, familyID, entryID string) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&gymdomain.GymEntry{}, "family_id = ? AND id = ?", familyID, entryID)
	return result.RowsAffected > 0, result.Error
}

// Workout operations

func (r *PostgresRepository) ListWorkouts(ctx context.Context, familyID string, filter gymdomain.ListFilter) ([]gymdomain.Workout, int64, error) {
	query := r.db.WithContext(ctx).Model(&gymdomain.Workout{}).Where("family_id = ?", familyID)

	if filter.From != nil {
		query = query.Where("date >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("date <= ?", *filter.To)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order("date desc, created_at desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []gymdomain.Workout
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *PostgresRepository) GetWorkoutByID(ctx context.Context, familyID, workoutID string) (*gymdomain.Workout, error) {
	var workout gymdomain.Workout
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND id = ?", familyID, workoutID).
		First(&workout).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gymdomain.ErrWorkoutNotFound
		}
		return nil, err
	}
	return &workout, nil
}

func (r *PostgresRepository) CreateWorkout(ctx context.Context, workout *gymdomain.Workout) error {
	return r.db.WithContext(ctx).Create(workout).Error
}

func (r *PostgresRepository) UpdateWorkout(ctx context.Context, workout *gymdomain.Workout) error {
	return r.db.WithContext(ctx).
		Model(&gymdomain.Workout{}).
		Where("id = ? AND family_id = ?", workout.ID, workout.FamilyID).
		Updates(map[string]interface{}{
			"date":       workout.Date,
			"name":       workout.Name,
			"updated_at": workout.UpdatedAt,
		}).Error
}

func (r *PostgresRepository) DeleteWorkout(ctx context.Context, familyID, workoutID string) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&gymdomain.Workout{}, "family_id = ? AND id = ?", familyID, workoutID)
	return result.RowsAffected > 0, result.Error
}

// WorkoutSet operations

func (r *PostgresRepository) GetSetsByWorkoutIDs(ctx context.Context, workoutIDs []string) (map[string][]gymdomain.WorkoutSet, error) {
	result := make(map[string][]gymdomain.WorkoutSet, len(workoutIDs))
	if len(workoutIDs) == 0 {
		return result, nil
	}

	var sets []gymdomain.WorkoutSet
	if err := r.db.WithContext(ctx).
		Where("workout_id IN ?", workoutIDs).
		Order("set_order asc").
		Find(&sets).Error; err != nil {
		return nil, err
	}

	for _, set := range sets {
		result[set.WorkoutID] = append(result[set.WorkoutID], set)
	}

	return result, nil
}

func (r *PostgresRepository) ReplaceWorkoutSets(ctx context.Context, workoutID string, sets []gymdomain.WorkoutSet) error {
	if err := r.db.WithContext(ctx).Where("workout_id = ?", workoutID).Delete(&gymdomain.WorkoutSet{}).Error; err != nil {
		return err
	}

	if len(sets) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Create(&sets).Error
}

// WorkoutTemplate operations

func (r *PostgresRepository) ListTemplates(ctx context.Context, familyID string) ([]gymdomain.WorkoutTemplate, error) {
	var templates []gymdomain.WorkoutTemplate
	if err := r.db.WithContext(ctx).
		Where("family_id = ?", familyID).
		Order("created_at desc").
		Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

func (r *PostgresRepository) GetTemplateByID(ctx context.Context, familyID, templateID string) (*gymdomain.WorkoutTemplate, error) {
	var template gymdomain.WorkoutTemplate
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND id = ?", familyID, templateID).
		First(&template).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gymdomain.ErrTemplateNotFound
		}
		return nil, err
	}
	return &template, nil
}

func (r *PostgresRepository) CreateTemplate(ctx context.Context, template *gymdomain.WorkoutTemplate) error {
	return r.db.WithContext(ctx).Create(template).Error
}

func (r *PostgresRepository) UpdateTemplate(ctx context.Context, template *gymdomain.WorkoutTemplate) error {
	return r.db.WithContext(ctx).
		Model(&gymdomain.WorkoutTemplate{}).
		Where("id = ? AND family_id = ?", template.ID, template.FamilyID).
		Updates(map[string]interface{}{
			"name":       template.Name,
			"updated_at": template.UpdatedAt,
		}).Error
}

func (r *PostgresRepository) DeleteTemplate(ctx context.Context, familyID, templateID string) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&gymdomain.WorkoutTemplate{}, "family_id = ? AND id = ?", familyID, templateID)
	return result.RowsAffected > 0, result.Error
}

// TemplateExercise operations

func (r *PostgresRepository) GetExercisesByTemplateIDs(ctx context.Context, templateIDs []string) (map[string][]gymdomain.TemplateExercise, error) {
	result := make(map[string][]gymdomain.TemplateExercise, len(templateIDs))
	if len(templateIDs) == 0 {
		return result, nil
	}

	var exercises []gymdomain.TemplateExercise
	if err := r.db.WithContext(ctx).
		Where("template_id IN ?", templateIDs).
		Order("exercise_order asc").
		Find(&exercises).Error; err != nil {
		return nil, err
	}

	for _, exercise := range exercises {
		result[exercise.TemplateID] = append(result[exercise.TemplateID], exercise)
	}

	return result, nil
}

func (r *PostgresRepository) ReplaceTemplateExercises(ctx context.Context, templateID string, exercises []gymdomain.TemplateExercise) error {
	if err := r.db.WithContext(ctx).Where("template_id = ?", templateID).Delete(&gymdomain.TemplateExercise{}).Error; err != nil {
		return err
	}

	if len(exercises) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Create(&exercises).Error
}

// Exercise list

func (r *PostgresRepository) ListExercises(ctx context.Context, familyID string) ([]string, error) {
	var exercises []string

	// Get unique exercises from gym_entries
	var entryExercises []string
	if err := r.db.WithContext(ctx).
		Model(&gymdomain.GymEntry{}).
		Where("family_id = ?", familyID).
		Distinct("exercise").
		Pluck("exercise", &entryExercises).Error; err != nil {
		return nil, err
	}

	// Get unique exercises from workout_sets via workouts
	var setExercises []string
	if err := r.db.WithContext(ctx).
		Model(&gymdomain.WorkoutSet{}).
		Select("DISTINCT workout_sets.exercise").
		Joins("JOIN workouts ON workouts.id = workout_sets.workout_id").
		Where("workouts.family_id = ?", familyID).
		Pluck("exercise", &setExercises).Error; err != nil {
		return nil, err
	}

	// Merge and deduplicate
	exerciseSet := make(map[string]struct{})
	for _, ex := range entryExercises {
		exerciseSet[ex] = struct{}{}
	}
	for _, ex := range setExercises {
		exerciseSet[ex] = struct{}{}
	}

	for ex := range exerciseSet {
		exercises = append(exercises, ex)
	}

	return exercises, nil
}
