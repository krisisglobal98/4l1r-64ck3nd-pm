-- Global reference masters (Stage FF-1A), the base the Mitra Bisnis master and the modules (Freight Forwarding,
-- finance, procurement, distribution) point to. Each is a standard master of a tenant (RLS); the international code
-- (ISO 3166 for countries, ISO 4217 for currencies) is a column of its own, unique per tenant, beside the generated code.
--   currency      Mata Uang (Keuangan & Pajak)
--   country       Negara (Geografi & Lokasi), with its default currency
--   city          Kota (Geografi & Lokasi), in a country
--   payment_term  Syarat Pembayaran (Keuangan & Pajak): days counted from a basis, optional down payment
--   tax_type      Jenis Pajak (Keuangan & Pajak): PPN / PPh … with the rate and the tax base share, per validity period
--   bank          + SWIFT/BIC and the country (now a Keuangan & Pajak master, used beyond payroll)

CREATE TABLE pm.currency (
    -- Primary Key
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,

    -- Master Data
    code                    varchar(40)  NOT NULL,
    name                    varchar(150) NOT NULL,
    description             varchar(500) NULL,

    -- Status
    is_active               boolean      NOT NULL DEFAULT true,
    is_deleted              boolean      NOT NULL DEFAULT false,

    -- Workflow (state: where the record is in its approval; status: approved, void, cancelled, posted, …)
    state                   varchar(20)  NULL,
    status                  varchar(20)  NULL,
    remark                  varchar(500) NULL,
    approval_level_required integer      NULL,
    approved_at             timestamptz  NULL,
    approved_by             uuid         NULL,

    -- Validity
    valid_from              date         NULL,
    valid_to                date         NULL,

    -- Audit (user ids come from the Authentication service; no foreign key across services)
    created_at              timestamptz  NOT NULL DEFAULT now(),
    created_by              uuid         NOT NULL,
    updated_at              timestamptz  NULL,
    updated_by              uuid         NULL,
    deleted_at              timestamptz  NULL,
    deleted_by              uuid         NULL,
    version                 integer      NOT NULL DEFAULT 1,

    -- Currency Attributes
    iso_code                char(3)      NOT NULL,      -- ISO 4217, e.g. IDR
    symbol                  varchar(5)   NULL,
    decimals                smallint     NOT NULL DEFAULT 2,

    -- Constraints
    CONSTRAINT currency_pkey PRIMARY KEY (id),
    CONSTRAINT currency_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT currency_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT currency_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT currency_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT currency_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT currency_iso_check CHECK (iso_code ~ '^[A-Z]{3}$'),
    CONSTRAINT currency_decimals_check CHECK (decimals BETWEEN 0 AND 4)
);
CREATE UNIQUE INDEX currency_code_ci ON pm.currency (tenant_id, lower(code));
CREATE INDEX idx_currency_active ON pm.currency (tenant_id, is_active, is_deleted);
ALTER TABLE pm.currency ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.currency USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX currency_iso_live ON pm.currency (tenant_id, iso_code) WHERE NOT is_deleted;

CREATE TABLE pm.country (
    -- Primary Key
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,

    -- Master Data
    code                    varchar(40)  NOT NULL,
    name                    varchar(150) NOT NULL,
    description             varchar(500) NULL,

    -- Status
    is_active               boolean      NOT NULL DEFAULT true,
    is_deleted              boolean      NOT NULL DEFAULT false,

    -- Workflow (state: where the record is in its approval; status: approved, void, cancelled, posted, …)
    state                   varchar(20)  NULL,
    status                  varchar(20)  NULL,
    remark                  varchar(500) NULL,
    approval_level_required integer      NULL,
    approved_at             timestamptz  NULL,
    approved_by             uuid         NULL,

    -- Validity
    valid_from              date         NULL,
    valid_to                date         NULL,

    -- Audit (user ids come from the Authentication service; no foreign key across services)
    created_at              timestamptz  NOT NULL DEFAULT now(),
    created_by              uuid         NOT NULL,
    updated_at              timestamptz  NULL,
    updated_by              uuid         NULL,
    deleted_at              timestamptz  NULL,
    deleted_by              uuid         NULL,
    version                 integer      NOT NULL DEFAULT 1,

    -- Country Attributes
    iso2                    char(2)      NOT NULL,      -- ISO 3166-1 alpha-2, e.g. ID
    iso3                    char(3)      NULL,          -- ISO 3166-1 alpha-3, e.g. IDN
    phone_code              varchar(6)   NULL,          -- e.g. +62
    currency_id             uuid         NULL,          -- default currency

    -- Constraints
    CONSTRAINT country_pkey PRIMARY KEY (id),
    CONSTRAINT country_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT country_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT country_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT country_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT country_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT country_iso2_check CHECK (iso2 ~ '^[A-Z]{2}$'),
    CONSTRAINT country_iso3_check CHECK (iso3 IS NULL OR iso3 ~ '^[A-Z]{3}$'),
    CONSTRAINT country_phone_check CHECK (phone_code IS NULL OR phone_code ~ '^\+[0-9]{1,4}$'),
    CONSTRAINT country_currency_fk FOREIGN KEY (currency_id) REFERENCES pm.currency (id)
);
CREATE UNIQUE INDEX country_code_ci ON pm.country (tenant_id, lower(code));
CREATE INDEX idx_country_active ON pm.country (tenant_id, is_active, is_deleted);
ALTER TABLE pm.country ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.country USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX country_iso2_live ON pm.country (tenant_id, iso2) WHERE NOT is_deleted;
CREATE UNIQUE INDEX country_iso3_live ON pm.country (tenant_id, iso3) WHERE NOT is_deleted AND iso3 IS NOT NULL;

CREATE TABLE pm.city (
    -- Primary Key
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,

    -- Master Data
    code                    varchar(40)  NOT NULL,
    name                    varchar(150) NOT NULL,
    description             varchar(500) NULL,

    -- Status
    is_active               boolean      NOT NULL DEFAULT true,
    is_deleted              boolean      NOT NULL DEFAULT false,

    -- Workflow (state: where the record is in its approval; status: approved, void, cancelled, posted, …)
    state                   varchar(20)  NULL,
    status                  varchar(20)  NULL,
    remark                  varchar(500) NULL,
    approval_level_required integer      NULL,
    approved_at             timestamptz  NULL,
    approved_by             uuid         NULL,

    -- Validity
    valid_from              date         NULL,
    valid_to                date         NULL,

    -- Audit (user ids come from the Authentication service; no foreign key across services)
    created_at              timestamptz  NOT NULL DEFAULT now(),
    created_by              uuid         NOT NULL,
    updated_at              timestamptz  NULL,
    updated_by              uuid         NULL,
    deleted_at              timestamptz  NULL,
    deleted_by              uuid         NULL,
    version                 integer      NOT NULL DEFAULT 1,

    -- City Attributes
    country_id              uuid         NOT NULL,
    province                varchar(100) NULL,          -- province / state
    kind                    varchar(10)  NOT NULL DEFAULT 'city', -- city | regency (kabupaten) | other
    area_code               varchar(10)  NULL,          -- official region code (e.g. Kemendagri/BPS), optional

    -- Constraints
    CONSTRAINT city_pkey PRIMARY KEY (id),
    CONSTRAINT city_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT city_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT city_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT city_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT city_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT city_country_fk FOREIGN KEY (country_id) REFERENCES pm.country (id),
    CONSTRAINT city_kind_check CHECK (kind IN ('city', 'regency', 'other'))
);
CREATE UNIQUE INDEX city_code_ci ON pm.city (tenant_id, lower(code));
CREATE INDEX idx_city_active ON pm.city (tenant_id, is_active, is_deleted);
ALTER TABLE pm.city ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.city USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX city_name_live ON pm.city (tenant_id, country_id, lower(name), lower(coalesce(province, ''))) WHERE NOT is_deleted;
CREATE INDEX idx_city_country ON pm.city (country_id);

CREATE TABLE pm.payment_term (
    -- Primary Key
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,

    -- Master Data
    code                    varchar(40)  NOT NULL,
    name                    varchar(150) NOT NULL,
    description             varchar(500) NULL,

    -- Status
    is_active               boolean      NOT NULL DEFAULT true,
    is_deleted              boolean      NOT NULL DEFAULT false,

    -- Workflow (state: where the record is in its approval; status: approved, void, cancelled, posted, …)
    state                   varchar(20)  NULL,
    status                  varchar(20)  NULL,
    remark                  varchar(500) NULL,
    approval_level_required integer      NULL,
    approved_at             timestamptz  NULL,
    approved_by             uuid         NULL,

    -- Validity
    valid_from              date         NULL,
    valid_to                date         NULL,

    -- Audit (user ids come from the Authentication service; no foreign key across services)
    created_at              timestamptz  NOT NULL DEFAULT now(),
    created_by              uuid         NOT NULL,
    updated_at              timestamptz  NULL,
    updated_by              uuid         NULL,
    deleted_at              timestamptz  NULL,
    deleted_by              uuid         NULL,
    version                 integer      NOT NULL DEFAULT 1,

    -- Payment Term Attributes
    days                    integer      NOT NULL,      -- 0 = cash / on delivery
    basis                   varchar(20)  NOT NULL DEFAULT 'invoice_date', -- from when the days count
    down_payment_pct        numeric(5,2) NOT NULL DEFAULT 0,
    applies_to              varchar(10)  NOT NULL DEFAULT 'both', -- customer | vendor | both

    -- Constraints
    CONSTRAINT payment_term_pkey PRIMARY KEY (id),
    CONSTRAINT payment_term_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT payment_term_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT payment_term_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT payment_term_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT payment_term_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT payment_term_days_check CHECK (days BETWEEN 0 AND 365),
    CONSTRAINT payment_term_basis_check CHECK (basis IN ('invoice_date', 'receipt_date', 'bl_date', 'etd', 'eta', 'end_of_month', 'delivery')),
    CONSTRAINT payment_term_dp_check CHECK (down_payment_pct BETWEEN 0 AND 100),
    CONSTRAINT payment_term_applies_check CHECK (applies_to IN ('customer', 'vendor', 'both'))
);
CREATE UNIQUE INDEX payment_term_code_ci ON pm.payment_term (tenant_id, lower(code));
CREATE INDEX idx_payment_term_active ON pm.payment_term (tenant_id, is_active, is_deleted);
ALTER TABLE pm.payment_term ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.payment_term USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE TABLE pm.tax_type (
    -- Primary Key
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,

    -- Master Data
    code                    varchar(40)  NOT NULL,
    name                    varchar(150) NOT NULL,
    description             varchar(500) NULL,

    -- Status
    is_active               boolean      NOT NULL DEFAULT true,
    is_deleted              boolean      NOT NULL DEFAULT false,

    -- Workflow (state: where the record is in its approval; status: approved, void, cancelled, posted, …)
    state                   varchar(20)  NULL,
    status                  varchar(20)  NULL,
    remark                  varchar(500) NULL,
    approval_level_required integer      NULL,
    approved_at             timestamptz  NULL,
    approved_by             uuid         NULL,

    -- Validity
    valid_from              date         NULL,
    valid_to                date         NULL,

    -- Audit (user ids come from the Authentication service; no foreign key across services)
    created_at              timestamptz  NOT NULL DEFAULT now(),
    created_by              uuid         NOT NULL,
    updated_at              timestamptz  NULL,
    updated_by              uuid         NULL,
    deleted_at              timestamptz  NULL,
    deleted_by              uuid         NULL,
    version                 integer      NOT NULL DEFAULT 1,

    -- Tax Type Attributes
    category                varchar(10)  NOT NULL,      -- PPN | PPNBM | PPH21 | PPH22 | PPH23 | PPH4_2 | PPH15 | PPH26 | OTHER
    rate_pct                numeric(7,4) NOT NULL,      -- the tariff, e.g. 12.0000
    base_pct                numeric(7,4) NOT NULL DEFAULT 100, -- share of the amount taxed (DPP), e.g. 91.6667 (11/12)
    method                  varchar(10)  NOT NULL,      -- added (on top: PPN) | withheld (cut by the payer: PPh)
    tax_code                varchar(20)  NULL,          -- code in the tax authority's system (e-Faktur/e-Bupot), optional

    -- Constraints
    CONSTRAINT tax_type_pkey PRIMARY KEY (id),
    CONSTRAINT tax_type_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT tax_type_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT tax_type_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT tax_type_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT tax_type_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT tax_type_category_check CHECK (category IN ('PPN', 'PPNBM', 'PPH21', 'PPH22', 'PPH23', 'PPH4_2', 'PPH15', 'PPH26', 'OTHER')),
    CONSTRAINT tax_type_rate_check CHECK (rate_pct BETWEEN 0 AND 100),
    CONSTRAINT tax_type_base_check CHECK (base_pct > 0 AND base_pct <= 100),
    CONSTRAINT tax_type_method_check CHECK (method IN ('added', 'withheld'))
);
CREATE UNIQUE INDEX tax_type_code_ci ON pm.tax_type (tenant_id, lower(code));
CREATE INDEX idx_tax_type_active ON pm.tax_type (tenant_id, is_active, is_deleted);
ALTER TABLE pm.tax_type ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.tax_type USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
-- ---------------------------------------------------------------- Bank: an international master now
ALTER TABLE pm.bank ADD COLUMN swift_code varchar(11) NULL, ADD COLUMN country_id uuid NULL REFERENCES pm.country (id),
    ADD CONSTRAINT bank_swift_check CHECK (swift_code IS NULL OR swift_code ~ '^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$');

-- ---------------------------------------------------------------- Generated codes of every existing tenant
INSERT INTO pm.code_prefix (tenant_id, code, name, prefix, digits, created_by)
SELECT t.tenant_id, x.code, x.name, x.prefix, 3, '00000000-0000-7000-8000-000000000000'
FROM (SELECT DISTINCT tenant_id FROM pm.code_prefix) t
CROSS JOIN (VALUES ('currency', 'Mata Uang', 'MMU'), ('country', 'Negara', 'MNG'), ('city', 'Kota', 'MKT'),
                   ('payment_term', 'Syarat Pembayaran', 'MTP'), ('tax_type', 'Jenis Pajak', 'MPJ')) AS x(code, name, prefix)
WHERE NOT EXISTS (SELECT 1 FROM pm.code_prefix p WHERE p.tenant_id = t.tenant_id AND lower(p.code) = x.code);

-- The application role reads and writes the new tables (deploy/grants.sql does the same on every migrate).
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alir_pm_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON pm.currency, pm.country, pm.city, pm.payment_term, pm.tax_type TO alir_pm_app;
    END IF;
END $$;
