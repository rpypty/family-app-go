UPDATE currencies
SET
  is_active = false,
  updated_at = now()
WHERE code = 'XDR';
