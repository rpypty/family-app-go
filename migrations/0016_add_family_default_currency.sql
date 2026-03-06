ALTER TABLE families
  ADD COLUMN IF NOT EXISTS default_currency varchar(3) NOT NULL DEFAULT 'USD';
