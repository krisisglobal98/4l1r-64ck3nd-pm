-- Mitra Bisnis and Tenaga Penjualan (Stage FF-1B), the Mitra Bisnis category of Portal Master.
--   sales_person        Tenaga Penjualan: an employee who sells (one live record per employee), with a sales unit
--   business_partner    Mitra Bisnis: ONE record per legal party, with the roles it plays for the company (customer,
--                       vendor, shipper, consignee, notify party, overseas agent, carrier, trucking, PPJK, warehouse),
--                       its tax identity (NPWP 16 digits, NITKU 22, PKP), and the terms as customer and as vendor.
--                       Modules (Freight Forwarding, finance, procurement, distribution) point to it, never copy it.
--   business_partner_address / _contact / _bank_account   its lists, replaced as a whole on every save; at most one
--                       primary row per list.

CREATE TABLE pm.sales_person (
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

    -- Sales Person Attributes
    employee_id             uuid         NOT NULL,           -- the employee (Karyawan) who sells
    org_unit_id             uuid         NULL,               -- sales unit / branch, optional
    email                   varchar(150) NULL,               -- work contact for customers (else the employee's)
    phone                   varchar(30)  NULL,
    commission_pct          numeric(5,2) NULL,

    -- Constraints
    CONSTRAINT sales_person_pkey PRIMARY KEY (id),
    CONSTRAINT sales_person_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT sales_person_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT sales_person_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT sales_person_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT sales_person_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT sales_person_employee_fk FOREIGN KEY (employee_id) REFERENCES pm.employee (id),
    CONSTRAINT sales_person_unit_fk FOREIGN KEY (org_unit_id) REFERENCES pm.org_unit (id),
    CONSTRAINT sales_person_commission_check CHECK (commission_pct IS NULL OR commission_pct BETWEEN 0 AND 100)
);
CREATE UNIQUE INDEX sales_person_code_ci ON pm.sales_person (tenant_id, lower(code));
CREATE INDEX idx_sales_person_active ON pm.sales_person (tenant_id, is_active, is_deleted);
ALTER TABLE pm.sales_person ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.sales_person USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX sales_person_employee_live ON pm.sales_person (tenant_id, employee_id) WHERE NOT is_deleted;
CREATE INDEX idx_sales_person_unit ON pm.sales_person (org_unit_id);

CREATE TABLE pm.business_partner (
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

    -- Identity
    trade_name              varchar(150) NULL,               -- brand / known-as name; name is the legal name
    entity_type             varchar(20)  NOT NULL,           -- pt, cv, firma, ud, koperasi, yayasan, bumn, government, individual, foreign, other
    roles                   text[]       NOT NULL,           -- what it is for the company, at least one
    country_id              uuid         NOT NULL,           -- domicile
    email                   varchar(150) NULL,
    phone                   varchar(30)  NULL,
    website                 varchar(200) NULL,
    nib                     varchar(13)  NULL,               -- Nomor Induk Berusaha (OSS), 13 digits

    -- Tax (Indonesian identity: NPWP 16 digits since 2024, NITKU = NPWP + 6 digits per place of business)
    npwp                    char(16)     NULL,
    nitku                   char(22)     NULL,
    is_pkp                  boolean      NOT NULL DEFAULT false,  -- Pengusaha Kena Pajak: issues e-Faktur
    tax_name                varchar(150) NULL,               -- name as registered for tax, when not the legal name
    tax_address             varchar(500) NULL,
    foreign_tax_id          varchar(30)  NULL,               -- TIN / VAT number of a foreign partner

    -- As customer (kept only while it has the customer or agent role)
    customer_payment_term_id uuid        NULL,
    customer_currency_id    uuid         NULL,
    credit_limit            numeric(18,2) NULL,              -- in the customer currency
    credit_hold             boolean      NOT NULL DEFAULT false,  -- no new orders until finance releases it
    sales_person_id         uuid         NULL,

    -- As vendor (kept only while it has a payable role: vendor, agent, carrier, trucking, PPJK, warehouse)
    vendor_payment_term_id  uuid         NULL,
    vendor_currency_id      uuid         NULL,
    vendor_tax_type_id      uuid         NULL,               -- PPh the company withholds when paying it

    -- Constraints
    CONSTRAINT business_partner_pkey PRIMARY KEY (id),
    CONSTRAINT business_partner_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT business_partner_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT business_partner_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT business_partner_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT business_partner_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT business_partner_entity_check CHECK (entity_type IN ('pt', 'cv', 'firma', 'ud', 'koperasi', 'yayasan', 'bumn', 'government', 'individual', 'foreign', 'other')),
    CONSTRAINT business_partner_roles_check CHECK (cardinality(roles) >= 1 AND roles <@ ARRAY['customer', 'vendor', 'shipper', 'consignee', 'notify_party', 'agent', 'carrier', 'trucking', 'customs_broker', 'warehouse']::text[]),
    CONSTRAINT business_partner_npwp_check CHECK (npwp IS NULL OR npwp ~ '^[0-9]{16}$'),
    CONSTRAINT business_partner_nitku_check CHECK (nitku IS NULL OR (nitku ~ '^[0-9]{22}$' AND npwp IS NOT NULL AND left(nitku, 16) = npwp)),
    CONSTRAINT business_partner_pkp_check CHECK (NOT is_pkp OR npwp IS NOT NULL),
    CONSTRAINT business_partner_nib_check CHECK (nib IS NULL OR nib ~ '^[0-9]{13}$'),
    CONSTRAINT business_partner_credit_check CHECK (credit_limit IS NULL OR credit_limit >= 0),
    CONSTRAINT business_partner_country_fk FOREIGN KEY (country_id) REFERENCES pm.country (id),
    CONSTRAINT business_partner_cterm_fk FOREIGN KEY (customer_payment_term_id) REFERENCES pm.payment_term (id),
    CONSTRAINT business_partner_ccur_fk FOREIGN KEY (customer_currency_id) REFERENCES pm.currency (id),
    CONSTRAINT business_partner_sales_fk FOREIGN KEY (sales_person_id) REFERENCES pm.sales_person (id),
    CONSTRAINT business_partner_vterm_fk FOREIGN KEY (vendor_payment_term_id) REFERENCES pm.payment_term (id),
    CONSTRAINT business_partner_vcur_fk FOREIGN KEY (vendor_currency_id) REFERENCES pm.currency (id),
    CONSTRAINT business_partner_vtax_fk FOREIGN KEY (vendor_tax_type_id) REFERENCES pm.tax_type (id)
);
CREATE UNIQUE INDEX business_partner_code_ci ON pm.business_partner (tenant_id, lower(code));
CREATE INDEX idx_business_partner_active ON pm.business_partner (tenant_id, is_active, is_deleted);
ALTER TABLE pm.business_partner ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.business_partner USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
-- One live partner per place of business: per NITKU, or per NPWP when it has no NITKU (a head office).
CREATE UNIQUE INDEX business_partner_nitku_live ON pm.business_partner (tenant_id, nitku) WHERE NOT is_deleted AND nitku IS NOT NULL;
CREATE UNIQUE INDEX business_partner_npwp_live ON pm.business_partner (tenant_id, npwp) WHERE NOT is_deleted AND nitku IS NULL AND npwp IS NOT NULL;
CREATE INDEX idx_business_partner_roles ON pm.business_partner USING gin (roles);
CREATE INDEX idx_business_partner_sales ON pm.business_partner (sales_person_id);

CREATE TABLE pm.business_partner_address (
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,
    business_partner_id     uuid         NOT NULL,
    seq                     smallint     NOT NULL,           -- order on the form, from 1
    kind                    varchar(20)  NOT NULL,           -- office, billing, delivery, pickup, warehouse, other
    label                   varchar(60)  NULL,               -- e.g. "Gudang Dumai"
    address                 varchar(500) NOT NULL,
    city_id                 uuid         NULL,
    postal_code             varchar(10)  NULL,
    is_primary              boolean      NOT NULL DEFAULT false,
    CONSTRAINT business_partner_address_pkey PRIMARY KEY (id),
    CONSTRAINT business_partner_address_seq UNIQUE (business_partner_id, seq),
    CONSTRAINT business_partner_address_partner_fk FOREIGN KEY (business_partner_id) REFERENCES pm.business_partner (id) ON DELETE CASCADE,
    CONSTRAINT business_partner_address_city_fk FOREIGN KEY (city_id) REFERENCES pm.city (id),
    CONSTRAINT business_partner_address_kind_check CHECK (kind IN ('office', 'billing', 'delivery', 'pickup', 'warehouse', 'other')),
    CONSTRAINT business_partner_address_text_check CHECK (btrim(address) <> '')
);
CREATE UNIQUE INDEX business_partner_address_primary ON pm.business_partner_address (business_partner_id) WHERE is_primary;
CREATE INDEX idx_business_partner_address_city ON pm.business_partner_address (city_id);
ALTER TABLE pm.business_partner_address ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.business_partner_address USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

CREATE TABLE pm.business_partner_contact (
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,
    business_partner_id     uuid         NOT NULL,
    seq                     smallint     NOT NULL,           -- order on the form, from 1
    name                    varchar(100) NOT NULL,
    job_title               varchar(80)  NULL,
    purpose                 varchar(20)  NOT NULL DEFAULT 'general',  -- general, operations, finance, sales, management, customs
    email                   varchar(150) NULL,
    phone                   varchar(30)  NULL,
    is_primary              boolean      NOT NULL DEFAULT false,
    CONSTRAINT business_partner_contact_pkey PRIMARY KEY (id),
    CONSTRAINT business_partner_contact_seq UNIQUE (business_partner_id, seq),
    CONSTRAINT business_partner_contact_partner_fk FOREIGN KEY (business_partner_id) REFERENCES pm.business_partner (id) ON DELETE CASCADE,
    CONSTRAINT business_partner_contact_purpose_check CHECK (purpose IN ('general', 'operations', 'finance', 'sales', 'management', 'customs')),
    CONSTRAINT business_partner_contact_name_check CHECK (btrim(name) <> '')
);
CREATE UNIQUE INDEX business_partner_contact_primary ON pm.business_partner_contact (business_partner_id) WHERE is_primary;
ALTER TABLE pm.business_partner_contact ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.business_partner_contact USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

CREATE TABLE pm.business_partner_bank_account (
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,
    business_partner_id     uuid         NOT NULL,
    seq                     smallint     NOT NULL,           -- order on the form, from 1
    bank_id                 uuid         NOT NULL,
    account_number          varchar(34)  NOT NULL,           -- digits, or an IBAN
    account_name            varchar(150) NOT NULL,           -- holder name as at the bank
    currency_id             uuid         NOT NULL,
    branch                  varchar(80)  NULL,
    is_primary              boolean      NOT NULL DEFAULT false,
    CONSTRAINT business_partner_bank_account_pkey PRIMARY KEY (id),
    CONSTRAINT business_partner_bank_account_seq UNIQUE (business_partner_id, seq),
    CONSTRAINT business_partner_bank_account_once UNIQUE (business_partner_id, bank_id, account_number),
    CONSTRAINT business_partner_bank_account_partner_fk FOREIGN KEY (business_partner_id) REFERENCES pm.business_partner (id) ON DELETE CASCADE,
    CONSTRAINT business_partner_bank_account_bank_fk FOREIGN KEY (bank_id) REFERENCES pm.bank (id),
    CONSTRAINT business_partner_bank_account_currency_fk FOREIGN KEY (currency_id) REFERENCES pm.currency (id),
    CONSTRAINT business_partner_bank_account_number_check CHECK (account_number ~ '^[A-Z0-9-]{4,34}$'),
    CONSTRAINT business_partner_bank_account_name_check CHECK (btrim(account_name) <> '')
);
CREATE UNIQUE INDEX business_partner_bank_account_primary ON pm.business_partner_bank_account (business_partner_id) WHERE is_primary;
CREATE INDEX idx_business_partner_bank_account_bank ON pm.business_partner_bank_account (bank_id);
ALTER TABLE pm.business_partner_bank_account ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.business_partner_bank_account USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

-- ---------------------------------------------------------------- Generated codes of every existing tenant
INSERT INTO pm.code_prefix (tenant_id, code, name, prefix, digits, created_by)
SELECT t.tenant_id, x.code, x.name, x.prefix, x.digits, '00000000-0000-7000-8000-000000000000'
FROM (SELECT DISTINCT tenant_id FROM pm.code_prefix) t
CROSS JOIN (VALUES ('business_partner', 'Mitra Bisnis', 'MMB', 4), ('sales_person', 'Tenaga Penjualan', 'MTJ', 3)) AS x(code, name, prefix, digits)
WHERE NOT EXISTS (SELECT 1 FROM pm.code_prefix p WHERE p.tenant_id = t.tenant_id AND lower(p.code) = x.code);

-- The application role reads and writes the new tables (deploy/grants.sql does the same on every migrate).
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alir_pm_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON pm.sales_person, pm.business_partner, pm.business_partner_address,
            pm.business_partner_contact, pm.business_partner_bank_account TO alir_pm_app;
    END IF;
END $$;
