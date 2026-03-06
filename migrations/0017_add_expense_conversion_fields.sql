ALTER TABLE expenses
  ADD COLUMN IF NOT EXISTS base_currency varchar(3),
  ADD COLUMN IF NOT EXISTS exchange_rate numeric(18,8),
  ADD COLUMN IF NOT EXISTS amount_in_base numeric(14,2),
  ADD COLUMN IF NOT EXISTS rate_date date,
  ADD COLUMN IF NOT EXISTS rate_source text;

CREATE INDEX IF NOT EXISTS idx_expenses_family_date_amount_in_base
  ON expenses (family_id, date, amount_in_base);
