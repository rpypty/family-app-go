ALTER TABLE currencies
  ADD COLUMN IF NOT EXISTS icon text NOT NULL DEFAULT '';

UPDATE currencies
SET
  icon = CASE code
    WHEN 'BYN' THEN '🇧🇾'
    WHEN 'USD' THEN '🇺🇸'
    WHEN 'EUR' THEN '🇪🇺'
    WHEN 'RUB' THEN '🇷🇺'
    WHEN 'AUD' THEN '🇦🇺'
    WHEN 'AMD' THEN '🇦🇲'
    WHEN 'BRL' THEN '🇧🇷'
    WHEN 'UAH' THEN '🇺🇦'
    WHEN 'DKK' THEN '🇩🇰'
    WHEN 'AED' THEN '🇦🇪'
    WHEN 'VND' THEN '🇻🇳'
    WHEN 'PLN' THEN '🇵🇱'
    WHEN 'JPY' THEN '🇯🇵'
    WHEN 'INR' THEN '🇮🇳'
    WHEN 'IRR' THEN '🇮🇷'
    WHEN 'ISK' THEN '🇮🇸'
    WHEN 'CAD' THEN '🇨🇦'
    WHEN 'CNY' THEN '🇨🇳'
    WHEN 'KWD' THEN '🇰🇼'
    WHEN 'MDL' THEN '🇲🇩'
    WHEN 'NZD' THEN '🇳🇿'
    WHEN 'NOK' THEN '🇳🇴'
    WHEN 'SGD' THEN '🇸🇬'
    WHEN 'KGS' THEN '🇰🇬'
    WHEN 'KZT' THEN '🇰🇿'
    WHEN 'TRY' THEN '🇹🇷'
    WHEN 'GBP' THEN '🇬🇧'
    WHEN 'CZK' THEN '🇨🇿'
    WHEN 'SEK' THEN '🇸🇪'
    WHEN 'CHF' THEN '🇨🇭'
    ELSE '🏳️'
  END,
  updated_at = now();
