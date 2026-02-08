-- Create gym_entries table
CREATE TABLE IF NOT EXISTS gym_entries (
    id UUID PRIMARY KEY,
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    date DATE NOT NULL,
    exercise VARCHAR(255) NOT NULL,
    weight_kg NUMERIC(8,2) NOT NULL,
    reps INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create workouts table
CREATE TABLE IF NOT EXISTS workouts (
    id UUID PRIMARY KEY,
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    date DATE NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create workout_sets table
CREATE TABLE IF NOT EXISTS workout_sets (
    id UUID PRIMARY KEY,
    workout_id UUID NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    exercise VARCHAR(255) NOT NULL,
    weight_kg NUMERIC(8,2) NOT NULL,
    reps INTEGER NOT NULL,
    set_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create workout_templates table
CREATE TABLE IF NOT EXISTS workout_templates (
    id UUID PRIMARY KEY,
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create template_exercises table
CREATE TABLE IF NOT EXISTS template_exercises (
    id UUID PRIMARY KEY,
    template_id UUID NOT NULL REFERENCES workout_templates(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    reps INTEGER NOT NULL,
    sets INTEGER NOT NULL,
    exercise_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for gym_entries
CREATE INDEX IF NOT EXISTS idx_gym_entries_family_id ON gym_entries(family_id);
CREATE INDEX IF NOT EXISTS idx_gym_entries_user_id ON gym_entries(user_id);
CREATE INDEX IF NOT EXISTS idx_gym_entries_date ON gym_entries(date);
CREATE INDEX IF NOT EXISTS idx_gym_entries_family_date ON gym_entries(family_id, date DESC);

-- Create indexes for workouts
CREATE INDEX IF NOT EXISTS idx_workouts_family_id ON workouts(family_id);
CREATE INDEX IF NOT EXISTS idx_workouts_user_id ON workouts(user_id);
CREATE INDEX IF NOT EXISTS idx_workouts_date ON workouts(date);
CREATE INDEX IF NOT EXISTS idx_workouts_family_date ON workouts(family_id, date DESC);

-- Create indexes for workout_sets
CREATE INDEX IF NOT EXISTS idx_workout_sets_workout_id ON workout_sets(workout_id);
CREATE INDEX IF NOT EXISTS idx_workout_sets_order ON workout_sets(workout_id, set_order);

-- Create indexes for workout_templates
CREATE INDEX IF NOT EXISTS idx_workout_templates_family_id ON workout_templates(family_id);
CREATE INDEX IF NOT EXISTS idx_workout_templates_user_id ON workout_templates(user_id);

-- Create indexes for template_exercises
CREATE INDEX IF NOT EXISTS idx_template_exercises_template_id ON template_exercises(template_id);
CREATE INDEX IF NOT EXISTS idx_template_exercises_order ON template_exercises(template_id, exercise_order);
