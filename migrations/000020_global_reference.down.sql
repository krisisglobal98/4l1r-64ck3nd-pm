-- Back to a copy per tenant, as far as it can go: the global rows become the migrated tenant's (…0a1) again; the
-- copies of the other tenants (removed by the up migration) are not rebuilt — their references keep pointing to
-- these rows, which Row-Level Security then hides from them. Restore from a backup to undo fully.
ALTER TABLE pm.currency ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.country ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.incoterm ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.currency ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.country ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.incoterm ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;

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
CREATE UNIQUE INDEX currency_code_ci ON pm.currency (tenant_id, lower(code));
CREATE UNIQUE INDEX currency_iso_live ON pm.currency (tenant_id, iso_code) WHERE NOT is_deleted;
CREATE INDEX idx_currency_active ON pm.currency (tenant_id, is_active, is_deleted);
CREATE UNIQUE INDEX country_code_ci ON pm.country (tenant_id, lower(code));
CREATE UNIQUE INDEX country_iso2_live ON pm.country (tenant_id, iso2) WHERE NOT is_deleted;
CREATE UNIQUE INDEX country_iso3_live ON pm.country (tenant_id, iso3) WHERE NOT is_deleted AND iso3 IS NOT NULL;
CREATE INDEX idx_country_active ON pm.country (tenant_id, is_active, is_deleted);
CREATE UNIQUE INDEX incoterm_code_ci ON pm.incoterm (tenant_id, lower(code));
CREATE UNIQUE INDEX incoterm_term_live ON pm.incoterm (tenant_id, term, edition) WHERE NOT is_deleted;
CREATE INDEX idx_incoterm_active ON pm.incoterm (tenant_id, is_active, is_deleted);

ALTER TABLE pm.currency ENABLE ROW LEVEL SECURITY;
ALTER TABLE pm.country ENABLE ROW LEVEL SECURITY;
ALTER TABLE pm.incoterm ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.currency USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE POLICY tenant_isolation ON pm.country USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE POLICY tenant_isolation ON pm.incoterm USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
