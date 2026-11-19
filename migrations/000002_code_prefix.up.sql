-- 1. Level cakupan akses (scope_level) leaves pm.org_level: nothing read it. The scope levels of user accounts are
--    fixed in Authentication (group, region, wilayah, company, unit, …) and the column only repeated the level name.
DROP INDEX pm.idx_org_level_scope_level;
ALTER TABLE pm.org_level DROP COLUMN scope_level;

-- 2. Code prefixes: how the service generates the code of a new record, one row per master table. This is the
--    setup data of the future master "Prefix Kode Master"; until its page exists the rows are kept here.
--    Code of a new record = prefix || last_number + 1, left-padded with zeros to `digits` (ML001, ML002, …).
CREATE TABLE pm.code_prefix (
    -- Primary Key
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),

    -- Master Data (code = the master table the prefix numbers, e.g. org_level)
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

    -- Code Prefix Attributes
    prefix                  varchar(10)  NOT NULL,
    digits                  smallint     NOT NULL DEFAULT 3,
    last_number             bigint       NOT NULL DEFAULT 0,

    -- Constraints
    CONSTRAINT code_prefix_pkey PRIMARY KEY (id),
    CONSTRAINT code_prefix_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT code_prefix_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT code_prefix_prefix_check CHECK (prefix ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT code_prefix_digits_check CHECK (digits BETWEEN 1 AND 9),
    CONSTRAINT code_prefix_last_number_check CHECK (last_number >= 0),
    CONSTRAINT code_prefix_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT code_prefix_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT code_prefix_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    )
);

CREATE UNIQUE INDEX code_prefix_code_ci ON pm.code_prefix (lower(code));
CREATE INDEX idx_code_prefix_active ON pm.code_prefix (is_active, is_deleted);

INSERT INTO pm.code_prefix (code, name, description, prefix, digits, created_by) VALUES
    ('org_level', 'Level Organisasi', 'Kode level baru: ML001, ML002, …', 'ML', 3, '00000000-0000-7000-8000-000000000000');
