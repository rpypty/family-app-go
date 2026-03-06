CREATE TABLE IF NOT EXISTS currencies (
  code varchar(3) PRIMARY KEY,
  name text NOT NULL,
  scale integer NOT NULL DEFAULT 1 CHECK (scale > 0),
  periodicity smallint NOT NULL DEFAULT 0 CHECK (periodicity IN (0, 1)),
  sort_order integer NOT NULL DEFAULT 1000,
  is_active boolean NOT NULL DEFAULT true,
  source text NOT NULL DEFAULT 'nbrb',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_currencies_sort_order ON currencies (sort_order, code);
CREATE INDEX IF NOT EXISTS idx_currencies_active ON currencies (is_active);

CREATE TABLE IF NOT EXISTS fx_rates (
  id bigserial PRIMARY KEY,
  rate_date date NOT NULL,
  from_currency varchar(3) NOT NULL REFERENCES currencies(code),
  to_currency varchar(3) NOT NULL REFERENCES currencies(code),
  scale integer NOT NULL CHECK (scale > 0),
  rate numeric(20,8) NOT NULL CHECK (rate > 0),
  source text NOT NULL DEFAULT 'nbrb',
  fetched_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (rate_date, from_currency, to_currency, source)
);

CREATE INDEX IF NOT EXISTS idx_fx_rates_pair_date ON fx_rates (from_currency, to_currency, rate_date DESC);
