CREATE TABLE IF NOT EXISTS tags (
  id uuid PRIMARY KEY,
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_tags_family_id ON tags (family_id);

CREATE TABLE IF NOT EXISTS expenses (
  id uuid PRIMARY KEY,
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  user_id uuid NOT NULL,
  date date NOT NULL,
  amount numeric(12,2) NOT NULL,
  currency varchar(3) NOT NULL,
  title text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_expenses_family_date ON expenses (family_id, date);
CREATE INDEX IF NOT EXISTS idx_expenses_user_id ON expenses (user_id);

CREATE TABLE IF NOT EXISTS expense_tags (
  expense_id uuid NOT NULL REFERENCES expenses(id) ON DELETE CASCADE,
  tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
  PRIMARY KEY (expense_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_expense_tags_tag_id ON expense_tags (tag_id);
