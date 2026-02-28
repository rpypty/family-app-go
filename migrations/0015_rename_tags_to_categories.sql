-- Backward-compatible rename: keep old migrations intact and migrate existing data in-place.
DO $$
BEGIN
  IF to_regclass('public.tags') IS NOT NULL AND to_regclass('public.categories') IS NULL THEN
    ALTER TABLE tags RENAME TO categories;
  END IF;
END $$;

DO $$
BEGIN
  IF to_regclass('public.expense_tags') IS NOT NULL AND to_regclass('public.expense_categories') IS NULL THEN
    ALTER TABLE expense_tags RENAME TO expense_categories;
  END IF;
END $$;

DO $$
BEGIN
  IF to_regclass('public.expense_categories') IS NOT NULL AND EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'expense_categories'
      AND column_name = 'tag_id'
  ) THEN
    ALTER TABLE expense_categories RENAME COLUMN tag_id TO category_id;
  END IF;
END $$;

ALTER TABLE IF EXISTS categories
  ADD COLUMN IF NOT EXISTS color text NULL,
  ADD COLUMN IF NOT EXISTS emoji text NULL;

ALTER INDEX IF EXISTS idx_tags_family_id RENAME TO idx_categories_family_id;
ALTER INDEX IF EXISTS idx_expense_tags_tag_id RENAME TO idx_expense_categories_category_id;
ALTER INDEX IF EXISTS idx_expense_tags_tag_id_expense_id RENAME TO idx_expense_categories_category_id_expense_id;

ALTER TABLE IF EXISTS categories
  DROP CONSTRAINT IF EXISTS tags_color_format;

ALTER TABLE IF EXISTS categories
  DROP CONSTRAINT IF EXISTS categories_color_format;

ALTER TABLE IF EXISTS categories
  ADD CONSTRAINT categories_color_format
  CHECK (color IS NULL OR color ~ '^#[0-9A-Fa-f]{6}$');

CREATE INDEX IF NOT EXISTS idx_categories_family_id ON categories (family_id);
CREATE INDEX IF NOT EXISTS idx_expense_categories_category_id ON expense_categories (category_id);
CREATE INDEX IF NOT EXISTS idx_expense_categories_category_id_expense_id ON expense_categories (category_id, expense_id);
