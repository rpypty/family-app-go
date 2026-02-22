ALTER TABLE tags
  ADD COLUMN IF NOT EXISTS color text NULL,
  ADD COLUMN IF NOT EXISTS emoji text NULL;

ALTER TABLE tags
  DROP CONSTRAINT IF EXISTS tags_color_format;

ALTER TABLE tags
  ADD CONSTRAINT tags_color_format
  CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$');
