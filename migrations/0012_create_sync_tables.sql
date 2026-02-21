CREATE TABLE IF NOT EXISTS sync_batches (
  id uuid PRIMARY KEY,
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  user_id uuid NOT NULL,
  idempotency_key text,
  request_hash text NOT NULL,
  status text NOT NULL,
  response_json jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_batches_family_user_idempotency_key_unique
  ON sync_batches (family_id, user_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS sync_operations (
  id uuid PRIMARY KEY,
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  user_id uuid NOT NULL,
  operation_id uuid NOT NULL,
  operation_type text NOT NULL,
  payload_hash text NOT NULL,
  local_id text,
  status text NOT NULL,
  entity text,
  server_id uuid,
  error_code text,
  error_message text,
  retryable boolean,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_operations_family_user_operation_unique
  ON sync_operations (family_id, user_id, operation_id);
