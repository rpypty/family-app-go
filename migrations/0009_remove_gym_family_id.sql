-- Remove family scoping from gym tables
ALTER TABLE gym_entries DROP CONSTRAINT IF EXISTS gym_entries_family_id_fkey;
ALTER TABLE workouts DROP CONSTRAINT IF EXISTS workouts_family_id_fkey;
ALTER TABLE workout_templates DROP CONSTRAINT IF EXISTS workout_templates_family_id_fkey;

DROP INDEX IF EXISTS idx_gym_entries_family_id;
DROP INDEX IF EXISTS idx_gym_entries_family_date;
DROP INDEX IF EXISTS idx_workouts_family_id;
DROP INDEX IF EXISTS idx_workouts_family_date;
DROP INDEX IF EXISTS idx_workout_templates_family_id;

ALTER TABLE gym_entries DROP COLUMN IF EXISTS family_id;
ALTER TABLE workouts DROP COLUMN IF EXISTS family_id;
ALTER TABLE workout_templates DROP COLUMN IF EXISTS family_id;
