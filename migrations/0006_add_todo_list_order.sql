ALTER TABLE todo_lists
  ADD COLUMN IF NOT EXISTS is_collapsed boolean NOT NULL DEFAULT false;

ALTER TABLE todo_lists
  ADD COLUMN IF NOT EXISTS order_index integer;

WITH ranked AS (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY family_id ORDER BY created_at ASC, id ASC) - 1 AS rn
  FROM todo_lists
  WHERE order_index IS NULL
)
UPDATE todo_lists
SET order_index = ranked.rn
FROM ranked
WHERE todo_lists.id = ranked.id;

ALTER TABLE todo_lists
  ALTER COLUMN order_index SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_todo_lists_family_order_unique
  ON todo_lists (family_id, order_index)
  WHERE deleted_at IS NULL;
