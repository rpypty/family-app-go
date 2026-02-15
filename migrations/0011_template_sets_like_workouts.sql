-- Change template structure to match workouts structure
-- Each template exercise should have multiple sets, each with its own weight and reps

-- Create template_sets table (similar to workout_sets)
CREATE TABLE IF NOT EXISTS template_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id UUID NOT NULL,
    exercise VARCHAR NOT NULL,
    weight_kg NUMERIC(8,2) NOT NULL DEFAULT 0,
    reps INT NOT NULL DEFAULT 8,
    set_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_template FOREIGN KEY (template_id) REFERENCES workout_templates(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_template_sets_template_id ON template_sets(template_id);

-- Drop old template_exercises table as we'll use template_sets instead
DROP TABLE IF EXISTS template_exercises;
