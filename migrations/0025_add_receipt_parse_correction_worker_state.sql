ALTER TABLE IF EXISTS receipt_parse_category_correction_events
  ADD COLUMN IF NOT EXISTS materialize_attempt_count INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_materialize_attempt_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS next_materialize_attempt_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ NULL,
  ADD COLUMN IF NOT EXISTS locked_by TEXT NULL,
  ADD COLUMN IF NOT EXISTS materialize_error_code TEXT NULL,
  ADD COLUMN IF NOT EXISTS materialize_error_message TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_receipt_parse_correction_events_worker_queue
  ON receipt_parse_category_correction_events(processed_at, locked_at, next_materialize_attempt_at, created_at)
  WHERE processed_at IS NULL;
