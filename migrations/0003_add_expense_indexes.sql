CREATE INDEX IF NOT EXISTS idx_expenses_family_currency ON expenses (family_id, currency);
CREATE INDEX IF NOT EXISTS idx_expenses_family_date_currency ON expenses (family_id, date, currency);
CREATE INDEX IF NOT EXISTS idx_expense_tags_tag_id_expense_id ON expense_tags (tag_id, expense_id);
