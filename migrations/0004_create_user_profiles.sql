CREATE TABLE IF NOT EXISTS user_profiles (
  user_id uuid PRIMARY KEY,
  email text,
  avatar_url text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_profiles_email ON user_profiles (email);
