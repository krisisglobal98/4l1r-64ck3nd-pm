-- Portal Master → Organisasi & Approval → Struktur organisasi → Unit Organisasi: the organization tree (group,
-- region, wilayah, company and operational units with their parent). Standard columns first, then the page's own.
-- Its levels and unit types live in this service too, so the relations are real foreign keys (by id). A deleted
-- row stays (soft delete), so a key never points to a missing row.
CREATE TABLE pm.org_unit (
    -- Primary Key
    id                      uuid         NOT NULL DEFAULT pm.uuid_v7(),

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

    -- Organization Unit Attributes
    org_level_id            uuid         NOT NULL,
    parent_id               uuid         NULL,     -- one level above; NULL only at the top level
    unit_type_id            uuid         NULL,     -- only (and always) at the Unit Operasional level
    location                varchar(80)  NULL,

    -- Constraints
    CONSTRAINT org_unit_pkey PRIMARY KEY (id),
    CONSTRAINT org_unit_level_fk FOREIGN KEY (org_level_id) REFERENCES pm.org_level (id),
    CONSTRAINT org_unit_parent_fk FOREIGN KEY (parent_id) REFERENCES pm.org_unit (id),
    CONSTRAINT org_unit_unit_type_fk FOREIGN KEY (unit_type_id) REFERENCES pm.unit_type (id),
    CONSTRAINT org_unit_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT org_unit_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT org_unit_not_own_parent CHECK (parent_id IS NULL OR parent_id <> id),
    CONSTRAINT org_unit_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT org_unit_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT org_unit_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    )
);

-- Case-insensitive unique code, deleted rows included: positions and user scopes refer to a unit by its code.
CREATE UNIQUE INDEX org_unit_code_ci ON pm.org_unit (lower(code));

-- Lookup indexes (the foreign keys, and the children of a unit)
CREATE INDEX idx_org_unit_active ON pm.org_unit (is_active, is_deleted);
CREATE INDEX idx_org_unit_level ON pm.org_unit (org_level_id);
CREATE INDEX idx_org_unit_parent ON pm.org_unit (parent_id) WHERE NOT is_deleted;
CREATE INDEX idx_org_unit_unit_type ON pm.org_unit (unit_type_id);

-- Generated codes of new units: MUO001, MUO002, … (Master Unit Organisasi).
INSERT INTO pm.code_prefix (code, name, description, prefix, digits, created_by) VALUES
    ('org_unit', 'Unit Organisasi', 'Kode unit baru: MUO001, MUO002, …', 'MUO', 3, '00000000-0000-7000-8000-000000000000');
