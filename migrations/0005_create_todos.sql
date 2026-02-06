CREATE TABLE IF NOT EXISTS todo_lists (
  id uuid PRIMARY KEY,
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  title text NOT NULL,
  archive_completed boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_todo_lists_family_id ON todo_lists (family_id);
CREATE INDEX IF NOT EXISTS idx_todo_lists_family_deleted_at ON todo_lists (family_id, deleted_at);

CREATE TABLE IF NOT EXISTS todo_items (
  id uuid PRIMARY KEY,
  list_id uuid NOT NULL REFERENCES todo_lists(id) ON DELETE CASCADE,
  title text NOT NULL,
  is_completed boolean NOT NULL DEFAULT false,
  is_archived boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  completed_by_id uuid,
  completed_by_name text,
  completed_by_email text,
  completed_by_avatar_url text,
  deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_todo_items_list_id ON todo_items (list_id);
CREATE INDEX IF NOT EXISTS idx_todo_items_list_archived ON todo_items (list_id, is_archived);
CREATE INDEX IF NOT EXISTS idx_todo_items_list_completed ON todo_items (list_id, is_completed);
