-- Add weight column to template_exercises table
ALTER TABLE template_exercises ADD COLUMN IF NOT EXISTS weight NUMERIC(8,2) DEFAULT 0;

