-- Global reference masters: Negara (ISO 3166), Mata Uang (ISO 4217) and Incoterms are one set for every tenant, kept
-- by the platform administrator, no longer a copy per tenant. Each standard key keeps ONE row:
--   the live one, from the migrated tenant (…0a1) when it has it — its codes are those its transactions already
--   store — else the oldest; every reference of every tenant moves to that row, the copies go, and the tables leave
--   Row-Level Security. Codes clashing between rows kept from different tenants are renumbered after the highest.

-- ---------------------------------------------------------------- Mata Uang
CREATE TEMP TABLE keep_currency ON COMMIT DROP AS
SELECT id AS old_id, first_value(id) OVER w AS new_id
FROM pm.currency
WINDOW w AS (PARTITION BY upper(iso_code) ORDER BY is_deleted, (tenant_id = '00000000-0000-7000-8000-0000000000a1') DESC, created_at, id);
DELETE FROM keep_currency WHERE old_id = new_id;

UPDATE pm.country t SET currency_id = m.new_id FROM keep_currency m WHERE t.currency_id = m.old_id;
UPDATE pm.business_partner t SET customer_currency_id = m.new_id FROM keep_currency m WHERE t.customer_currency_id = m.old_id;
UPDATE pm.business_partner t SET vendor_currency_id = m.new_id FROM keep_currency m WHERE t.vendor_currency_id = m.old_id;
UPDATE pm.business_partner_bank_account t SET currency_id = m.new_id FROM keep_currency m WHERE t.currency_id = m.old_id;
UPDATE pm.exchange_rate t SET from_currency_id = m.new_id FROM keep_currency m WHERE t.from_currency_id = m.old_id;
UPDATE pm.exchange_rate t SET to_currency_id = m.new_id FROM keep_currency m WHERE t.to_currency_id = m.old_id;
UPDATE pm.charge_code t SET currency_id = m.new_id FROM keep_currency m WHERE t.currency_id = m.old_id;
UPDATE pm.charge_code_rate t SET currency_id = m.new_id FROM keep_currency m WHERE t.currency_id = m.old_id;
UPDATE pm.company_bank_account t SET currency_id = m.new_id FROM keep_currency m WHERE t.currency_id = m.old_id;
UPDATE pm.account t SET currency_id = m.new_id FROM keep_currency m WHERE t.currency_id = m.old_id;
DELETE FROM pm.currency WHERE id IN (SELECT old_id FROM keep_currency);

WITH d AS (SELECT id, row_number() OVER (PARTITION BY lower(code) ORDER BY (tenant_id = '00000000-0000-7000-8000-0000000000a1') DESC, created_at, id) AS rn FROM pm.currency),
     r AS (SELECT id, row_number() OVER (ORDER BY id) AS k FROM d WHERE rn > 1),
     m AS (SELECT coalesce(max(substring(code FROM 4)::int), 0) AS mx FROM pm.currency WHERE code ~ '^MMU[0-9]+$')
UPDATE pm.currency c SET code = 'MMU' || lpad((m.mx + r.k)::text, 3, '0') FROM r, m WHERE c.id = r.id;

-- ---------------------------------------------------------------- Negara
CREATE TEMP TABLE keep_country ON COMMIT DROP AS
SELECT id AS old_id, first_value(id) OVER w AS new_id
FROM pm.country
WINDOW w AS (PARTITION BY upper(iso2) ORDER BY is_deleted, (tenant_id = '00000000-0000-7000-8000-0000000000a1') DESC, created_at, id);
DELETE FROM keep_country WHERE old_id = new_id;

UPDATE pm.bank t SET country_id = m.new_id FROM keep_country m WHERE t.country_id = m.old_id;
UPDATE pm.city t SET country_id = m.new_id FROM keep_country m WHERE t.country_id = m.old_id;
UPDATE pm.business_partner t SET country_id = m.new_id FROM keep_country m WHERE t.country_id = m.old_id;
UPDATE pm.location t SET country_id = m.new_id FROM keep_country m WHERE t.country_id = m.old_id;
UPDATE pm.company_profile t SET country_id = m.new_id FROM keep_country m WHERE t.country_id = m.old_id;
DELETE FROM pm.country WHERE id IN (SELECT old_id FROM keep_country);

WITH d AS (SELECT id, row_number() OVER (PARTITION BY lower(code) ORDER BY (tenant_id = '00000000-0000-7000-8000-0000000000a1') DESC, created_at, id) AS rn FROM pm.country),
     r AS (SELECT id, row_number() OVER (ORDER BY id) AS k FROM d WHERE rn > 1),
     m AS (SELECT coalesce(max(substring(code FROM 4)::int), 0) AS mx FROM pm.country WHERE code ~ '^MNG[0-9]+$')
UPDATE pm.country c SET code = 'MNG' || lpad((m.mx + r.k)::text, 3, '0') FROM r, m WHERE c.id = r.id;

-- ---------------------------------------------------------------- Incoterms (no table points to it by id: jobs keep the code)
CREATE TEMP TABLE keep_incoterm ON COMMIT DROP AS
SELECT id AS old_id, first_value(id) OVER w AS new_id
FROM pm.incoterm
WINDOW w AS (PARTITION BY term, edition ORDER BY is_deleted, (tenant_id = '00000000-0000-7000-8000-0000000000a1') DESC, created_at, id);
DELETE FROM keep_incoterm WHERE old_id = new_id;
DELETE FROM pm.incoterm WHERE id IN (SELECT old_id FROM keep_incoterm);

WITH d AS (SELECT id, row_number() OVER (PARTITION BY lower(code) ORDER BY (tenant_id = '00000000-0000-7000-8000-0000000000a1') DESC, created_at, id) AS rn FROM pm.incoterm),
     r AS (SELECT id, row_number() OVER (ORDER BY id) AS k FROM d WHERE rn > 1),
     m AS (SELECT coalesce(max(substring(code FROM 4)::int), 0) AS mx FROM pm.incoterm WHERE code ~ '^MIC[0-9]+$')
UPDATE pm.incoterm c SET code = 'MIC' || lpad((m.mx + r.k)::text, 3, '0') FROM r, m WHERE c.id = r.id;

-- ---------------------------------------------------------------- out of the tenant scope
DROP POLICY tenant_isolation ON pm.currency;
DROP POLICY tenant_isolation ON pm.country;
DROP POLICY tenant_isolation ON pm.incoterm;
ALTER TABLE pm.currency DISABLE ROW LEVEL SECURITY;
ALTER TABLE pm.country DISABLE ROW LEVEL SECURITY;
ALTER TABLE pm.incoterm DISABLE ROW LEVEL SECURITY;

DROP INDEX pm.currency_code_ci;
DROP INDEX pm.currency_iso_live;
DROP INDEX pm.idx_currency_active;
DROP INDEX pm.country_code_ci;
DROP INDEX pm.country_iso2_live;
DROP INDEX pm.country_iso3_live;
DROP INDEX pm.idx_country_active;
DROP INDEX pm.incoterm_code_ci;
DROP INDEX pm.incoterm_term_live;
DROP INDEX pm.idx_incoterm_active;
ALTER TABLE pm.currency DROP COLUMN tenant_id;
ALTER TABLE pm.country DROP COLUMN tenant_id;
ALTER TABLE pm.incoterm DROP COLUMN tenant_id;

CREATE UNIQUE INDEX currency_code_ci ON pm.currency (lower(code));
CREATE UNIQUE INDEX currency_iso_live ON pm.currency (iso_code) WHERE NOT is_deleted;
CREATE INDEX idx_currency_active ON pm.currency (is_active, is_deleted);
CREATE UNIQUE INDEX country_code_ci ON pm.country (lower(code));
CREATE UNIQUE INDEX country_iso2_live ON pm.country (iso2) WHERE NOT is_deleted;
CREATE UNIQUE INDEX country_iso3_live ON pm.country (iso3) WHERE NOT is_deleted AND iso3 IS NOT NULL;
CREATE INDEX idx_country_active ON pm.country (is_active, is_deleted);
CREATE UNIQUE INDEX incoterm_code_ci ON pm.incoterm (lower(code));
CREATE UNIQUE INDEX incoterm_term_live ON pm.incoterm (term, edition) WHERE NOT is_deleted;
CREATE INDEX idx_incoterm_active ON pm.incoterm (is_active, is_deleted);

-- Their codes are no longer numbered per tenant.
DELETE FROM pm.code_prefix WHERE lower(code) IN ('currency', 'country', 'incoterm');
