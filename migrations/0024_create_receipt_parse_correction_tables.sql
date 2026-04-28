CREATE TABLE IF NOT EXISTS receipt_parse_category_correction_events (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  receipt_parse_job_id UUID NOT NULL REFERENCES receipt_parse_jobs(id) ON DELETE CASCADE,
  receipt_parse_item_id UUID NOT NULL REFERENCES receipt_parse_items(id) ON DELETE CASCADE,
  source_item_text TEXT NOT NULL,
  normalized_item_text TEXT NOT NULL,
  llm_category_id UUID NULL,
  final_category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  processed_at TIMESTAMPTZ NULL,
  materialize_attempt_count INTEGER NOT NULL DEFAULT 0,
  last_materialize_attempt_at TIMESTAMPTZ NULL,
  next_materialize_attempt_at TIMESTAMPTZ NULL,
  locked_at TIMESTAMPTZ NULL,
  locked_by TEXT NULL,
  materialize_error_code TEXT NULL,
  materialize_error_message TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(receipt_parse_item_id)
);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_correction_events_family_created
  ON receipt_parse_category_correction_events(family_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_correction_events_unprocessed
  ON receipt_parse_category_correction_events(processed_at)
  WHERE processed_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_receipt_parse_correction_events_worker_queue
  ON receipt_parse_category_correction_events(processed_at, locked_at, next_materialize_attempt_at, created_at)
  WHERE processed_at IS NULL;

CREATE TABLE IF NOT EXISTS receipt_parse_family_hints (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  canonical_name TEXT NOT NULL,
  final_category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  times_confirmed INTEGER NOT NULL DEFAULT 1,
  last_confirmed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(family_id, canonical_name, final_category_id)
);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_family_hints_family_rank
  ON receipt_parse_family_hints(family_id, times_confirmed DESC, last_confirmed_at DESC);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_family_hints_family_category
  ON receipt_parse_family_hints(family_id, final_category_id);

CREATE TABLE IF NOT EXISTS receipt_parse_family_hint_examples (
  id UUID PRIMARY KEY,
  hint_id UUID NOT NULL REFERENCES receipt_parse_family_hints(id) ON DELETE CASCADE,
  correction_event_id UUID NOT NULL REFERENCES receipt_parse_category_correction_events(id) ON DELETE CASCADE,
  source_item_text TEXT NOT NULL,
  normalized_item_text TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(hint_id, correction_event_id)
);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_family_hint_examples_hint_created
  ON receipt_parse_family_hint_examples(hint_id, created_at DESC);
