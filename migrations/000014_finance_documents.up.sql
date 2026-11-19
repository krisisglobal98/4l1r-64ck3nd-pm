-- Finance, document and service masters (Stage FF-1C-2), global in Portal Master:
--   exchange_rate         Kurs (Keuangan & Pajak): one rate of a currency into another, of a type (tax: KMK weekly,
--                         BI middle rate, bank buy/sell, internal), valid from a date (valid_from, required) until the
--                         next one; the rate on a date is the latest one valid then
--   charge_code           Kode Biaya (Keuangan & Pajak): ONE code for what is billed to customers and/or paid to vendors,
--                         with its group, basis, default currency and rate, PPN and PPh, and rates per container type
--   company_bank_account  Rekening Bank Perusahaan (Keuangan & Pajak): the company's own accounts, per company unit
--   document_type         Jenis Dokumen (Dokumen): shipping, customs, commercial and certificate documents
--   service_type          Moda & Layanan (Logistik & Pengiriman): transport mode and load type of a service

CREATE TABLE pm.exchange_rate (
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

    -- Exchange Rate Attributes
    from_currency_id        uuid         NOT NULL,
    to_currency_id          uuid         NOT NULL,
    rate_type               varchar(20)  NOT NULL,      -- tax, bi_middle, bank_buy, bank_sell, internal
    rate                    numeric(20,8) NOT NULL,     -- 1 from-currency = rate to-currency
    source                  varchar(100) NULL,          -- e.g. KMK number, bank name

    -- Constraints
    CONSTRAINT exchange_rate_pkey PRIMARY KEY (id),
    CONSTRAINT exchange_rate_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT exchange_rate_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT exchange_rate_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT exchange_rate_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT exchange_rate_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT exchange_rate_from_fk FOREIGN KEY (from_currency_id) REFERENCES pm.currency (id),
    CONSTRAINT exchange_rate_to_fk FOREIGN KEY (to_currency_id) REFERENCES pm.currency (id),
    CONSTRAINT exchange_rate_pair_check CHECK (from_currency_id <> to_currency_id),
    CONSTRAINT exchange_rate_type_check CHECK (rate_type IN ('tax', 'bi_middle', 'bank_buy', 'bank_sell', 'internal')),
    CONSTRAINT exchange_rate_rate_check CHECK (rate > 0),
    CONSTRAINT exchange_rate_from_date_check CHECK (valid_from IS NOT NULL)
);
CREATE UNIQUE INDEX exchange_rate_code_ci ON pm.exchange_rate (tenant_id, lower(code));
CREATE INDEX idx_exchange_rate_active ON pm.exchange_rate (tenant_id, is_active, is_deleted);
ALTER TABLE pm.exchange_rate ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.exchange_rate USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX exchange_rate_live ON pm.exchange_rate (tenant_id, from_currency_id, to_currency_id, rate_type, valid_from) WHERE NOT is_deleted;
CREATE INDEX idx_exchange_rate_lookup ON pm.exchange_rate (tenant_id, from_currency_id, to_currency_id, rate_type, valid_from DESC);
CREATE INDEX idx_exchange_rate_to ON pm.exchange_rate (to_currency_id);
CREATE TABLE pm.charge_code (
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

    -- Charge Code Attributes
    charge_group            varchar(20)  NOT NULL,      -- freight, origin, destination, customs, trucking, warehouse, document, insurance, surcharge, other
    is_receivable           boolean      NOT NULL DEFAULT true,   -- billed to customers (invoice)
    is_payable              boolean      NOT NULL DEFAULT true,   -- paid to vendors (bill)
    basis                   varchar(20)  NOT NULL,      -- shipment, container, teu, kg, ton, cbm, wm, document, day, percent
    currency_id             uuid         NULL,          -- default currency
    default_rate            numeric(18,4) NULL,
    ppn_tax_type_id         uuid         NULL,          -- PPN on the invoice (added)
    pph_tax_type_id         uuid         NULL,          -- PPh withheld when paying a vendor
    account_code            varchar(30)  NULL,          -- chart-of-accounts code, until the accounting master exists

    -- Constraints
    CONSTRAINT charge_code_pkey PRIMARY KEY (id),
    CONSTRAINT charge_code_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT charge_code_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT charge_code_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT charge_code_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT charge_code_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT charge_code_group_check CHECK (charge_group IN ('freight', 'origin', 'destination', 'customs', 'trucking', 'warehouse', 'document', 'insurance', 'surcharge', 'other')),
    CONSTRAINT charge_code_side_check CHECK (is_receivable OR is_payable),
    CONSTRAINT charge_code_basis_check CHECK (basis IN ('shipment', 'container', 'teu', 'kg', 'ton', 'cbm', 'wm', 'document', 'day', 'percent')),
    CONSTRAINT charge_code_rate_check CHECK (default_rate IS NULL OR default_rate >= 0),
    CONSTRAINT charge_code_currency_fk FOREIGN KEY (currency_id) REFERENCES pm.currency (id),
    CONSTRAINT charge_code_ppn_fk FOREIGN KEY (ppn_tax_type_id) REFERENCES pm.tax_type (id),
    CONSTRAINT charge_code_pph_fk FOREIGN KEY (pph_tax_type_id) REFERENCES pm.tax_type (id)
);
CREATE UNIQUE INDEX charge_code_code_ci ON pm.charge_code (tenant_id, lower(code));
CREATE INDEX idx_charge_code_active ON pm.charge_code (tenant_id, is_active, is_deleted);
ALTER TABLE pm.charge_code ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.charge_code USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE INDEX idx_charge_code_currency ON pm.charge_code (currency_id);

-- Rates of a per-container charge by container type (replaced as a whole on every save).
CREATE TABLE pm.charge_code_rate (
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,
    charge_code_id          uuid         NOT NULL,
    seq                     smallint     NOT NULL,
    container_type_id       uuid         NOT NULL,
    currency_id             uuid         NOT NULL,
    rate                    numeric(18,4) NOT NULL,
    CONSTRAINT charge_code_rate_pkey PRIMARY KEY (id),
    CONSTRAINT charge_code_rate_seq UNIQUE (charge_code_id, seq),
    CONSTRAINT charge_code_rate_once UNIQUE (charge_code_id, container_type_id, currency_id),
    CONSTRAINT charge_code_rate_charge_fk FOREIGN KEY (charge_code_id) REFERENCES pm.charge_code (id) ON DELETE CASCADE,
    CONSTRAINT charge_code_rate_container_fk FOREIGN KEY (container_type_id) REFERENCES pm.container_type (id),
    CONSTRAINT charge_code_rate_currency_fk FOREIGN KEY (currency_id) REFERENCES pm.currency (id),
    CONSTRAINT charge_code_rate_rate_check CHECK (rate >= 0)
);
CREATE INDEX idx_charge_code_rate_container ON pm.charge_code_rate (container_type_id);
ALTER TABLE pm.charge_code_rate ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.charge_code_rate USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE TABLE pm.company_bank_account (
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

    -- Company Bank Account Attributes
    bank_id                 uuid         NOT NULL,
    account_number          varchar(34)  NOT NULL,
    account_name            varchar(150) NOT NULL,      -- holder name as at the bank
    currency_id             uuid         NOT NULL,
    branch                  varchar(80)  NULL,
    org_unit_id             uuid         NULL,          -- the company (unit) that owns it
    purpose                 varchar(20)  NOT NULL DEFAULT 'operational',  -- operational, receipt, payment, payroll, tax
    is_invoice_default      boolean      NOT NULL DEFAULT false,  -- printed on invoices of its currency and unit
    account_code            varchar(30)  NULL,

    -- Constraints
    CONSTRAINT company_bank_account_pkey PRIMARY KEY (id),
    CONSTRAINT company_bank_account_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT company_bank_account_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT company_bank_account_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT company_bank_account_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT company_bank_account_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT company_bank_account_bank_fk FOREIGN KEY (bank_id) REFERENCES pm.bank (id),
    CONSTRAINT company_bank_account_currency_fk FOREIGN KEY (currency_id) REFERENCES pm.currency (id),
    CONSTRAINT company_bank_account_unit_fk FOREIGN KEY (org_unit_id) REFERENCES pm.org_unit (id),
    CONSTRAINT company_bank_account_number_check CHECK (account_number ~ '^[A-Z0-9-]{4,34}$'),
    CONSTRAINT company_bank_account_purpose_check CHECK (purpose IN ('operational', 'receipt', 'payment', 'payroll', 'tax'))
);
CREATE UNIQUE INDEX company_bank_account_code_ci ON pm.company_bank_account (tenant_id, lower(code));
CREATE INDEX idx_company_bank_account_active ON pm.company_bank_account (tenant_id, is_active, is_deleted);
ALTER TABLE pm.company_bank_account ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.company_bank_account USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX company_bank_account_number_live ON pm.company_bank_account (tenant_id, bank_id, account_number) WHERE NOT is_deleted;
CREATE UNIQUE INDEX company_bank_account_invoice_live ON pm.company_bank_account (tenant_id, currency_id, coalesce(org_unit_id, '00000000-0000-0000-0000-000000000000'::uuid))
    WHERE NOT is_deleted AND is_invoice_default;
CREATE INDEX idx_company_bank_account_unit ON pm.company_bank_account (org_unit_id);
CREATE TABLE pm.document_type (
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

    -- Document Type Attributes
    doc_group               varchar(20)  NOT NULL,      -- shipping, customs, commercial, certificate, insurance, other
    customs_code            varchar(10)  NULL,          -- BC form, e.g. BC 2.0
    issuer                  varchar(20)  NOT NULL DEFAULT 'other',  -- carrier, forwarder, customs, shipper, government, company, other
    for_export              boolean      NOT NULL DEFAULT true,
    for_import              boolean      NOT NULL DEFAULT true,
    for_domestic            boolean      NOT NULL DEFAULT false,
    number_required         boolean      NOT NULL DEFAULT true,
    has_expiry              boolean      NOT NULL DEFAULT false,
    original_required       boolean      NOT NULL DEFAULT false,

    -- Constraints
    CONSTRAINT document_type_pkey PRIMARY KEY (id),
    CONSTRAINT document_type_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT document_type_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT document_type_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT document_type_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT document_type_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT document_type_group_check CHECK (doc_group IN ('shipping', 'customs', 'commercial', 'certificate', 'insurance', 'other')),
    CONSTRAINT document_type_issuer_check CHECK (issuer IN ('carrier', 'forwarder', 'customs', 'shipper', 'government', 'company', 'other')),
    CONSTRAINT document_type_scope_check CHECK (for_export OR for_import OR for_domestic)
);
CREATE UNIQUE INDEX document_type_code_ci ON pm.document_type (tenant_id, lower(code));
CREATE INDEX idx_document_type_active ON pm.document_type (tenant_id, is_active, is_deleted);
ALTER TABLE pm.document_type ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.document_type USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX document_type_customs_live ON pm.document_type (tenant_id, customs_code) WHERE NOT is_deleted AND customs_code IS NOT NULL;
CREATE TABLE pm.service_type (
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

    -- Service Type Attributes
    mode                    varchar(20)  NOT NULL,      -- sea, air, land, rail, multimodal, none (no transport: customs, warehousing)
    load_type               varchar(20)  NOT NULL DEFAULT 'none',  -- fcl, lcl, bulk, breakbulk, ftl, ltl, general, none
    needs_container         boolean      NOT NULL DEFAULT false,
    includes_customs        boolean      NOT NULL DEFAULT false,

    -- Constraints
    CONSTRAINT service_type_pkey PRIMARY KEY (id),
    CONSTRAINT service_type_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT service_type_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT service_type_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT service_type_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT service_type_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT service_type_mode_check CHECK (mode IN ('sea', 'air', 'land', 'rail', 'multimodal', 'none')),
    CONSTRAINT service_type_load_check CHECK (load_type IN ('fcl', 'lcl', 'bulk', 'breakbulk', 'ftl', 'ltl', 'general', 'none')),
    CONSTRAINT service_type_mode_load_check CHECK (
        (load_type IN ('fcl', 'lcl', 'bulk', 'breakbulk') AND mode IN ('sea', 'multimodal'))
        OR (load_type IN ('ftl', 'ltl') AND mode IN ('land', 'multimodal'))
        OR (load_type = 'fcl' AND mode = 'rail')
        OR load_type IN ('general', 'none'))
);
CREATE UNIQUE INDEX service_type_code_ci ON pm.service_type (tenant_id, lower(code));
CREATE INDEX idx_service_type_active ON pm.service_type (tenant_id, is_active, is_deleted);
ALTER TABLE pm.service_type ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.service_type USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

-- ---------------------------------------------------------------- Generated codes of every existing tenant
INSERT INTO pm.code_prefix (tenant_id, code, name, prefix, digits, created_by)
SELECT t.tenant_id, x.code, x.name, x.prefix, x.digits, '00000000-0000-7000-8000-000000000000'
FROM (SELECT DISTINCT tenant_id FROM pm.code_prefix) t
CROSS JOIN (VALUES ('exchange_rate', 'Kurs', 'MKR', 6), ('charge_code', 'Kode Biaya', 'MKB', 3), ('company_bank_account', 'Rekening Bank Perusahaan', 'MRP', 3),
                   ('document_type', 'Jenis Dokumen', 'MJD', 3), ('service_type', 'Moda & Layanan', 'MML', 3)) AS x(code, name, prefix, digits)
WHERE NOT EXISTS (SELECT 1 FROM pm.code_prefix p WHERE p.tenant_id = t.tenant_id AND lower(p.code) = x.code);

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alir_pm_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON pm.exchange_rate, pm.charge_code, pm.charge_code_rate, pm.company_bank_account,
            pm.document_type, pm.service_type TO alir_pm_app;
    END IF;
END $$;
