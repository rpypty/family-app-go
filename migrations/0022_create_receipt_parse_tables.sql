CREATE TABLE IF NOT EXISTS receipt_parse_jobs (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  status TEXT NOT NULL,
  category_mode TEXT NOT NULL,
  selected_category_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
  requested_date DATE,
  requested_currency TEXT,
  merchant_name TEXT,
  purchased_at DATE,
  currency TEXT,
  detected_total NUMERIC(12, 2),
  items_total NUMERIC(12, 2),
  provider TEXT,
  model TEXT,
  raw_llm_response JSONB,
  error_code TEXT,
  error_message TEXT,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  last_attempt_at TIMESTAMPTZ,
  next_attempt_at TIMESTAMPTZ,
  locked_at TIMESTAMPTZ,
  locked_by TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMPTZ,
  approved_at TIMESTAMPTZ,
  cancelled_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_jobs_family_status
  ON receipt_parse_jobs(family_id, status);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_jobs_family_created
  ON receipt_parse_jobs(family_id, created_at DESC);

CREATE TABLE IF NOT EXISTS receipt_parse_files (
  id UUID PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES receipt_parse_jobs(id) ON DELETE CASCADE,
  ordinal INTEGER NOT NULL,
  file_name TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  storage_key TEXT,
  sha256 TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(job_id, ordinal)
);

CREATE TABLE IF NOT EXISTS receipt_parse_items (
  id UUID PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES receipt_parse_jobs(id) ON DELETE CASCADE,
  line_index INTEGER NOT NULL,
  raw_name TEXT NOT NULL,
  normalized_name TEXT,
  quantity NUMERIC(12, 3),
  unit_price NUMERIC(12, 2),
  line_total NUMERIC(12, 2) NOT NULL,
  llm_category_id UUID,
  llm_category_confidence NUMERIC(4, 3),
  final_category_id UUID,
  final_line_total NUMERIC(12, 2),
  is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
  edited_by_user BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(job_id, line_index)
);

CREATE TABLE IF NOT EXISTS receipt_parse_draft_expenses (
  id UUID PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES receipt_parse_jobs(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  amount NUMERIC(12, 2) NOT NULL,
  currency TEXT NOT NULL,
  category_id UUID NOT NULL REFERENCES categories(id),
  confidence NUMERIC(4, 3),
  warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
  final_title TEXT,
  final_amount NUMERIC(12, 2),
  final_category_id UUID REFERENCES categories(id),
  is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
  edited_by_user BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
