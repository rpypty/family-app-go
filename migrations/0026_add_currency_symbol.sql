ALTER TABLE currencies
  ADD COLUMN IF NOT EXISTS symbol text NOT NULL DEFAULT '';

UPDATE currencies
SET
  symbol = CASE code
    WHEN 'BYN' THEN 'ƃ'
    WHEN 'USD' THEN '$'
    WHEN 'EUR' THEN '€'
    WHEN 'RUB' THEN '₽'
    WHEN 'AUD' THEN '$'
    WHEN 'AMD' THEN '֏'
    WHEN 'BRL' THEN 'R$'
    WHEN 'UAH' THEN '₴'
    WHEN 'DKK' THEN 'kr'
    WHEN 'AED' THEN 'د.إ'
    WHEN 'VND' THEN '₫'
    WHEN 'PLN' THEN 'zł'
    WHEN 'JPY' THEN '¥'
    WHEN 'INR' THEN '₹'
    WHEN 'IRR' THEN '﷼'
    WHEN 'ISK' THEN 'kr'
    WHEN 'CAD' THEN '$'
    WHEN 'CNY' THEN '¥'
    WHEN 'KWD' THEN 'د.ك'
    WHEN 'MDL' THEN 'L'
    WHEN 'NZD' THEN '$'
    WHEN 'NOK' THEN 'kr'
    WHEN 'SGD' THEN '$'
    WHEN 'KGS' THEN 'с'
    WHEN 'KZT' THEN '₸'
    WHEN 'TRY' THEN '₺'
    WHEN 'GBP' THEN '£'
    WHEN 'CZK' THEN 'Kč'
    WHEN 'SEK' THEN 'kr'
    WHEN 'CHF' THEN 'Fr'
    ELSE code
  END,
  updated_at = now();
