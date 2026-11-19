DELETE FROM pm.code_prefix WHERE code IN ('currency', 'country', 'city', 'payment_term', 'tax_type');
ALTER TABLE pm.bank DROP CONSTRAINT bank_swift_check, DROP COLUMN country_id, DROP COLUMN swift_code;
DROP TABLE pm.tax_type;
DROP TABLE pm.payment_term;
DROP TABLE pm.city;
DROP TABLE pm.country;
DROP TABLE pm.currency;
