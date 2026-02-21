CREATE INDEX IF NOT EXISTS idx_sync_batches_created_at
  ON sync_batches (created_at);

CREATE INDEX IF NOT EXISTS idx_sync_operations_mapping_lookup
  ON sync_operations (family_id, user_id, entity, local_id)
  WHERE local_id IS NOT NULL AND server_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sync_operations_created_at
  ON sync_operations (created_at);

CREATE INDEX IF NOT EXISTS idx_sync_operations_status_created_at
  ON sync_operations (status, created_at);
