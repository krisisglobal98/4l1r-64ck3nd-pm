-- System Parameter (Pengaturan Sistem): named values the application modules read, e.g. INVOICE_DEFAULT_DUE_DAYS
-- (Freight Forwarding: due days of an invoice whose payment term counts 0 days). The key is what a module asks by.
CREATE TABLE pm.system_parameter (
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,
    code                    varchar(40)  NOT NULL,
    name                    varchar(150) NOT NULL,
    description             varchar(500) NULL,
    is_active               boolean      NOT NULL DEFAULT true,
    is_deleted              boolean      NOT NULL DEFAULT false,
    state                   varchar(20)  NULL,
    status                  varchar(20)  NULL,
    remark                  varchar(500) NULL,
    approval_level_required integer      NULL,
    approved_at             timestamptz  NULL,
    approved_by             uuid         NULL,
    valid_from              date         NULL,
    valid_to                date         NULL,
    created_at              timestamptz  NOT NULL DEFAULT now(),
    created_by              uuid         NOT NULL,
    updated_at              timestamptz  NULL,
    updated_by              uuid         NULL,
    deleted_at              timestamptz  NULL,
    deleted_by              uuid         NULL,
    version                 integer      NOT NULL DEFAULT 1,
    -- System Parameter Attributes
    param_key               varchar(60)  NOT NULL,      -- INVOICE_DEFAULT_DUE_DAYS
    param_value             varchar(500) NOT NULL,
    module                  varchar(40)  NOT NULL DEFAULT 'umum',  -- the module that reads it
    CONSTRAINT system_parameter_pkey PRIMARY KEY (id),
    CONSTRAINT system_parameter_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT system_parameter_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT system_parameter_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT system_parameter_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT system_parameter_delete_audit_check CHECK (is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)),
    CONSTRAINT system_parameter_key_check CHECK (param_key ~ '^[A-Z][A-Z0-9_]*$')
);
CREATE UNIQUE INDEX system_parameter_code_ci ON pm.system_parameter (tenant_id, lower(code));
CREATE UNIQUE INDEX system_parameter_key_live ON pm.system_parameter (tenant_id, param_key) WHERE NOT is_deleted;
CREATE INDEX idx_system_parameter_active ON pm.system_parameter (tenant_id, is_active, is_deleted);
ALTER TABLE pm.system_parameter ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.system_parameter USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
