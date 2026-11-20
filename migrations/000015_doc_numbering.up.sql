-- Format Penomoran Dokumen (Pengaturan Sistem), now served here: the number pattern of each document type of every
-- module. A pattern is valid from a date (valid_from, required) until the next one of the same document type, so a
-- change never renumbers issued documents. The counter itself lives in the service that owns the document (it makes
-- the number inside the transaction that saves the document, without gaps); this table only says how numbers look.

CREATE TABLE pm.doc_numbering (
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

    -- Numbering Attributes
    doc_type                varchar(40)  NOT NULL,      -- key the owning service asks by, e.g. FF.JOB
    module                  varchar(60)  NULL,          -- Beranda application that issues it, e.g. freight-forwarding
    pattern                 varchar(60)  NOT NULL,      -- tokens {PT} {UNIT} {YYYY} {YY} {MM} {DD} {SEQ:n}
    reset                   varchar(10)  NOT NULL DEFAULT 'never',   -- never, yearly, monthly, daily
    scope                   varchar(10)  NOT NULL DEFAULT 'global',  -- global, company, unit
    start_number            integer      NOT NULL DEFAULT 1,

    -- Constraints
    CONSTRAINT doc_numbering_pkey PRIMARY KEY (id),
    CONSTRAINT doc_numbering_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT doc_numbering_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT doc_numbering_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT doc_numbering_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT doc_numbering_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT doc_numbering_type_check CHECK (doc_type ~ '^[A-Z][A-Z0-9_.-]{1,39}$'),
    CONSTRAINT doc_numbering_reset_check CHECK (reset IN ('never', 'yearly', 'monthly', 'daily')),
    CONSTRAINT doc_numbering_scope_check CHECK (scope IN ('global', 'company', 'unit')),
    CONSTRAINT doc_numbering_start_check CHECK (start_number >= 1),
    CONSTRAINT doc_numbering_from_check CHECK (valid_from IS NOT NULL)
);
CREATE UNIQUE INDEX doc_numbering_code_ci ON pm.doc_numbering (tenant_id, lower(code));
CREATE INDEX idx_doc_numbering_active ON pm.doc_numbering (tenant_id, is_active, is_deleted);
CREATE UNIQUE INDEX doc_numbering_type_from_live ON pm.doc_numbering (tenant_id, doc_type, valid_from) WHERE NOT is_deleted;
ALTER TABLE pm.doc_numbering ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.doc_numbering USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

INSERT INTO pm.code_prefix (tenant_id, code, name, prefix, digits, created_by)
SELECT t.tenant_id, 'doc_numbering', 'Format Penomoran Dokumen', 'MPN', 3, '00000000-0000-7000-8000-000000000000'
FROM (SELECT DISTINCT tenant_id FROM pm.code_prefix) t
WHERE NOT EXISTS (SELECT 1 FROM pm.code_prefix p WHERE p.tenant_id = t.tenant_id AND lower(p.code) = 'doc_numbering');

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alir_pm_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON pm.doc_numbering TO alir_pm_app;
    END IF;
END $$;
