-- Portal Master → Organisasi & Approval → Struktur organisasi → Jenis Unit Operasional.
-- Standard columns first (as every master table), then the page's own.
CREATE TABLE pm.unit_type (
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

    -- Unit Type Attributes
    -- Head job title of this kind of unit (pm.jabatan.id). NULL until the Jabatan master moves to this service;
    -- the foreign key is added then.
    head_jabatan_id         uuid         NULL,

    -- Constraints
    CONSTRAINT unit_type_pkey PRIMARY KEY (id),
    CONSTRAINT unit_type_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT unit_type_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT unit_type_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT unit_type_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT unit_type_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    )
);

-- Case-insensitive unique code, deleted rows included: units refer to a unit type by its code.
CREATE UNIQUE INDEX unit_type_code_ci ON pm.unit_type (lower(code));

-- Lookup indexes
CREATE INDEX idx_unit_type_active ON pm.unit_type (is_active, is_deleted);
CREATE INDEX idx_unit_type_head_jabatan ON pm.unit_type (head_jabatan_id);

-- The unit types the sample units are built on keep their codes; the head job title waits for Jabatan.
INSERT INTO pm.unit_type (code, name, created_by) VALUES
    ('EST', 'Kebun Inti',          '00000000-0000-7000-8000-000000000000'),
    ('PKS', 'Pabrik Kelapa Sawit', '00000000-0000-7000-8000-000000000000'),
    ('PLS', 'Kebun Plasma',        '00000000-0000-7000-8000-000000000000'),
    ('BLK', 'Gudang Bulking',      '00000000-0000-7000-8000-000000000000'),
    ('KTR', 'Kantor Perwakilan',   '00000000-0000-7000-8000-000000000000');

-- Generated codes of new unit types: MJU001, MJU002, … (Master Jenis Unit).
INSERT INTO pm.code_prefix (code, name, description, prefix, digits, created_by) VALUES
    ('unit_type', 'Jenis Unit Operasional', 'Kode jenis unit baru: MJU001, MJU002, …', 'MJU', 3, '00000000-0000-7000-8000-000000000000');
