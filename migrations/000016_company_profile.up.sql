-- Profil Perusahaan: the identity a company prints on its documents (invoices, job sheets): legal and trade name, NPWP,
-- address, contacts and logo. One profile per owning company unit (or one for the tenant); the one marked
-- print_default heads documents that name no company.
CREATE TABLE pm.company_profile (
    -- Primary Key
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,

    -- Master Data (name: the legal name)
    code                    varchar(40)  NOT NULL,
    name                    varchar(150) NOT NULL,
    description             varchar(500) NULL,

    -- Status
    is_active               boolean      NOT NULL DEFAULT true,
    is_deleted              boolean      NOT NULL DEFAULT false,

    -- Workflow
    state                   varchar(20)  NULL,
    status                  varchar(20)  NULL,
    remark                  varchar(500) NULL,
    approval_level_required integer      NULL,
    approved_at             timestamptz  NULL,
    approved_by             uuid         NULL,

    -- Validity
    valid_from              date         NULL,
    valid_to                date         NULL,

    -- Audit
    created_at              timestamptz  NOT NULL DEFAULT now(),
    created_by              uuid         NOT NULL,
    updated_at              timestamptz  NULL,
    updated_by              uuid         NULL,
    deleted_at              timestamptz  NULL,
    deleted_by              uuid         NULL,
    version                 integer      NOT NULL DEFAULT 1,

    -- Company Profile Attributes
    trade_name              varchar(150) NULL,          -- printed as the company name when set
    npwp                    char(16)     NULL,
    address                 varchar(500) NULL,
    city                    varchar(100) NULL,
    country_id              uuid         NULL,
    phone                   varchar(40)  NULL,
    fax                     varchar(40)  NULL,
    email                   varchar(150) NULL,
    website                 varchar(200) NULL,
    logo                    text         NULL,          -- data URL (PNG, JPEG or SVG), at most ~400 KB
    org_unit_id             uuid         NULL,          -- the company unit it describes
    print_default           boolean      NOT NULL DEFAULT false,

    -- Constraints
    CONSTRAINT company_profile_pkey PRIMARY KEY (id),
    CONSTRAINT company_profile_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT company_profile_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT company_profile_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT company_profile_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT company_profile_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT company_profile_country_fk FOREIGN KEY (country_id) REFERENCES pm.country (id),
    CONSTRAINT company_profile_unit_fk FOREIGN KEY (org_unit_id) REFERENCES pm.org_unit (id),
    CONSTRAINT company_profile_npwp_check CHECK (npwp IS NULL OR npwp ~ '^[0-9]{16}$'),
    CONSTRAINT company_profile_logo_check CHECK (logo IS NULL OR (logo LIKE 'data:image/%' AND length(logo) <= 560000))
);
CREATE UNIQUE INDEX company_profile_code_ci ON pm.company_profile (tenant_id, lower(code));
CREATE INDEX idx_company_profile_active ON pm.company_profile (tenant_id, is_active, is_deleted);
CREATE UNIQUE INDEX company_profile_unit_live ON pm.company_profile (tenant_id, coalesce(org_unit_id, '00000000-0000-0000-0000-000000000000'::uuid)) WHERE NOT is_deleted;
CREATE UNIQUE INDEX company_profile_default_live ON pm.company_profile (tenant_id) WHERE NOT is_deleted AND print_default;
ALTER TABLE pm.company_profile ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.company_profile USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
