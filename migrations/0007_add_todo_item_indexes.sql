CREATE INDEX IF NOT EXISTS idx_todo_items_list_archived_created_at
  ON todo_items (list_id, is_archived, created_at)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_todo_items_list_created_at
  ON todo_items (list_id, created_at)
  WHERE deleted_at IS NULL;
