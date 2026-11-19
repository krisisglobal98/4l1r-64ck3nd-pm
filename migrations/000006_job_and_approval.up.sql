-- Portal Master → Organisasi & Approval: Jabatan & pejabat (grade, jabatan, posisi) and Alur approval (aturan
-- approval, delegasi, approval custom). Every master table: standard columns first, then the page's own. Lists of a
-- record (amount bands, delegated document types, approval steps) are detail tables owned by it, replaced on save.
--
-- Employees (Karyawan, Employee Management) are not served here yet: the holder, acting holder and delegate columns
-- are nullable uuids without a foreign key, left NULL until that master moves to this service.

-- ---------------------------------------------------------------- Grade Jabatan
CREATE TABLE pm.grade (
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

    -- Grade Attributes
    rank                    integer      NOT NULL, -- higher = more senior

    -- Constraints
    CONSTRAINT grade_pkey PRIMARY KEY (id),
    CONSTRAINT grade_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT grade_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT grade_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT grade_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT grade_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT grade_rank_check CHECK (rank >= 1)
);
CREATE UNIQUE INDEX grade_code_ci ON pm.grade (lower(code));
CREATE INDEX idx_grade_active ON pm.grade (is_active, is_deleted);
CREATE UNIQUE INDEX grade_rank_live ON pm.grade (rank) WHERE NOT is_deleted;

-- ---------------------------------------------------------------- Jabatan
CREATE TABLE pm.jabatan (
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

    -- Jabatan Attributes
    grade_id                uuid         NOT NULL,
    kind                    varchar(20)  NOT NULL, -- struktural (heads a unit) or fungsional (staff)
    org_level_id            uuid         NOT NULL, -- the level of the units it is used in

    -- Constraints
    CONSTRAINT jabatan_pkey PRIMARY KEY (id),
    CONSTRAINT jabatan_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT jabatan_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT jabatan_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT jabatan_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT jabatan_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT jabatan_grade_fk FOREIGN KEY (grade_id) REFERENCES pm.grade (id),
    CONSTRAINT jabatan_level_fk FOREIGN KEY (org_level_id) REFERENCES pm.org_level (id),
    CONSTRAINT jabatan_kind_check CHECK (kind IN ('struktural', 'fungsional'))
);
CREATE UNIQUE INDEX jabatan_code_ci ON pm.jabatan (lower(code));
CREATE INDEX idx_jabatan_active ON pm.jabatan (is_active, is_deleted);
CREATE INDEX idx_jabatan_grade ON pm.jabatan (grade_id);
CREATE INDEX idx_jabatan_level ON pm.jabatan (org_level_id);

-- The head job title of a unit type was waiting for this master.
ALTER TABLE pm.unit_type ADD CONSTRAINT unit_type_head_jabatan_fk FOREIGN KEY (head_jabatan_id) REFERENCES pm.jabatan (id);

-- ---------------------------------------------------------------- Posisi
CREATE TABLE pm.position (
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

    -- Position Attributes
    org_unit_id             uuid         NOT NULL,
    jabatan_id              uuid         NOT NULL, -- same level as the unit
    reports_to_id           uuid         NULL,     -- the reporting line the default approval walks
    is_head                 boolean      NOT NULL DEFAULT false,
    holder_employee_id      uuid         NULL,     -- Karyawan; NULL until Employee Management is served here
    acting_employee_id      uuid         NULL,     -- Plt/Plh; idem

    -- Constraints
    CONSTRAINT position_pkey PRIMARY KEY (id),
    CONSTRAINT position_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT position_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT position_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT position_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT position_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT position_unit_fk FOREIGN KEY (org_unit_id) REFERENCES pm.org_unit (id),
    CONSTRAINT position_jabatan_fk FOREIGN KEY (jabatan_id) REFERENCES pm.jabatan (id),
    CONSTRAINT position_reports_to_fk FOREIGN KEY (reports_to_id) REFERENCES pm.position (id),
    CONSTRAINT position_not_own_superior CHECK (reports_to_id IS NULL OR reports_to_id <> id)
);
CREATE UNIQUE INDEX position_code_ci ON pm.position (lower(code));
CREATE INDEX idx_position_active ON pm.position (is_active, is_deleted);
CREATE INDEX idx_position_unit ON pm.position (org_unit_id);
CREATE INDEX idx_position_jabatan ON pm.position (jabatan_id);
CREATE INDEX idx_position_reports_to ON pm.position (reports_to_id) WHERE NOT is_deleted;
-- One head per unit.
CREATE UNIQUE INDEX position_head_per_unit ON pm.position (org_unit_id) WHERE is_head AND NOT is_deleted;

-- ---------------------------------------------------------------- Aturan Approval
CREATE TABLE pm.approval_rule (
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

    -- Approval Rule Attributes (name = the document type)
    sop                     varchar(60)  NOT NULL,
    module                  varchar(60)  NOT NULL, -- application id of the document (ALIR Beranda)
    reach_kind              varchar(10)  NOT NULL, -- steps | level | amount
    reach_steps             integer      NULL,     -- steps: approvers above the requester
    reach_level_id          uuid         NULL,     -- level: up to the head of this level
    min_steps               integer      NOT NULL DEFAULT 0,

    -- Constraints
    CONSTRAINT approval_rule_pkey PRIMARY KEY (id),
    CONSTRAINT approval_rule_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT approval_rule_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT approval_rule_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT approval_rule_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT approval_rule_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT approval_rule_reach_level_fk FOREIGN KEY (reach_level_id) REFERENCES pm.org_level (id),
    CONSTRAINT approval_rule_reach_check CHECK (
        (reach_kind = 'steps' AND reach_steps >= 1 AND reach_level_id IS NULL) OR
        (reach_kind = 'level' AND reach_level_id IS NOT NULL AND reach_steps IS NULL) OR
        (reach_kind = 'amount' AND reach_steps IS NULL AND reach_level_id IS NULL)
    ),
    CONSTRAINT approval_rule_min_steps_check CHECK (min_steps >= 0)
);
CREATE UNIQUE INDEX approval_rule_code_ci ON pm.approval_rule (lower(code));
CREATE INDEX idx_approval_rule_active ON pm.approval_rule (is_active, is_deleted);

-- amount: the first band whose max covers the amount decides the level; max NULL = no upper bound (last band).
CREATE TABLE pm.approval_rule_band (
    approval_rule_id        uuid           NOT NULL,
    seq                     smallint       NOT NULL,
    max_amount              numeric(18, 2) NULL,
    org_level_id            uuid           NOT NULL,
    CONSTRAINT approval_rule_band_pkey PRIMARY KEY (approval_rule_id, seq),
    CONSTRAINT approval_rule_band_rule_fk FOREIGN KEY (approval_rule_id) REFERENCES pm.approval_rule (id) ON DELETE CASCADE,
    CONSTRAINT approval_rule_band_level_fk FOREIGN KEY (org_level_id) REFERENCES pm.org_level (id),
    CONSTRAINT approval_rule_band_max_check CHECK (max_amount IS NULL OR max_amount > 0)
);
CREATE INDEX idx_approval_rule_band_level ON pm.approval_rule_band (org_level_id);

-- ---------------------------------------------------------------- Delegasi
-- name = the reason; valid_from/valid_to = the period (required, inclusive).
CREATE TABLE pm.delegation (
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

    -- Delegation Attributes
    position_id             uuid         NOT NULL, -- whose authority is delegated
    delegate_employee_id    uuid         NULL,     -- Karyawan; NULL until Employee Management is served here

    -- Constraints
    CONSTRAINT delegation_pkey PRIMARY KEY (id),
    CONSTRAINT delegation_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT delegation_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT delegation_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT delegation_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT delegation_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT delegation_position_fk FOREIGN KEY (position_id) REFERENCES pm.position (id),
    CONSTRAINT delegation_period_check CHECK (valid_from IS NOT NULL AND valid_to IS NOT NULL)
);
CREATE UNIQUE INDEX delegation_code_ci ON pm.delegation (lower(code));
CREATE INDEX idx_delegation_active ON pm.delegation (is_active, is_deleted);
CREATE INDEX idx_delegation_position ON pm.delegation (position_id);

-- Document types delegated; none = every type.
CREATE TABLE pm.delegation_rule (
    delegation_id           uuid         NOT NULL,
    approval_rule_id        uuid         NOT NULL,
    CONSTRAINT delegation_rule_pkey PRIMARY KEY (delegation_id, approval_rule_id),
    CONSTRAINT delegation_rule_delegation_fk FOREIGN KEY (delegation_id) REFERENCES pm.delegation (id) ON DELETE CASCADE,
    CONSTRAINT delegation_rule_rule_fk FOREIGN KEY (approval_rule_id) REFERENCES pm.approval_rule (id)
);
CREATE INDEX idx_delegation_rule_rule ON pm.delegation_rule (approval_rule_id);

-- ---------------------------------------------------------------- Approval Custom
CREATE TABLE pm.custom_flow (
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

    -- Custom Flow Attributes
    sop                     varchar(60)  NOT NULL,
    approval_rule_id        uuid         NOT NULL, -- the document type it overrides
    scope_unit_id           uuid         NOT NULL, -- this unit and every unit under it

    -- Constraints
    CONSTRAINT custom_flow_pkey PRIMARY KEY (id),
    CONSTRAINT custom_flow_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT custom_flow_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT custom_flow_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT custom_flow_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT custom_flow_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT custom_flow_rule_fk FOREIGN KEY (approval_rule_id) REFERENCES pm.approval_rule (id),
    CONSTRAINT custom_flow_scope_fk FOREIGN KEY (scope_unit_id) REFERENCES pm.org_unit (id)
);
CREATE UNIQUE INDEX custom_flow_code_ci ON pm.custom_flow (lower(code));
CREATE INDEX idx_custom_flow_active ON pm.custom_flow (is_active, is_deleted);
CREATE INDEX idx_custom_flow_scope ON pm.custom_flow (scope_unit_id);
-- One flow per document type and branch: the most specific branch wins, so two on one branch would be ambiguous.
CREATE UNIQUE INDEX custom_flow_rule_scope_live ON pm.custom_flow (approval_rule_id, scope_unit_id) WHERE NOT is_deleted;

CREATE TABLE pm.custom_flow_step (
    custom_flow_id          uuid         NOT NULL,
    seq                     smallint     NOT NULL,
    position_id             uuid         NOT NULL,
    CONSTRAINT custom_flow_step_pkey PRIMARY KEY (custom_flow_id, seq),
    CONSTRAINT custom_flow_step_flow_fk FOREIGN KEY (custom_flow_id) REFERENCES pm.custom_flow (id) ON DELETE CASCADE,
    CONSTRAINT custom_flow_step_position_fk FOREIGN KEY (position_id) REFERENCES pm.position (id),
    CONSTRAINT custom_flow_step_once UNIQUE (custom_flow_id, position_id)
);
CREATE INDEX idx_custom_flow_step_position ON pm.custom_flow_step (position_id);

-- ---------------------------------------------------------------- Generated codes
INSERT INTO pm.code_prefix (code, name, description, prefix, digits, created_by) VALUES
    ('grade',         'Grade Jabatan',   'Kode grade baru: MGR001, …',          'MGR', 3, '00000000-0000-7000-8000-000000000000'),
    ('jabatan',       'Jabatan',         'Kode jabatan baru: MJB001, …',        'MJB', 3, '00000000-0000-7000-8000-000000000000'),
    ('position',      'Posisi',          'Kode posisi baru: MPS001, …',         'MPS', 3, '00000000-0000-7000-8000-000000000000'),
    ('approval_rule', 'Aturan Approval', 'Kode aturan baru: MAR001, …',         'MAR', 3, '00000000-0000-7000-8000-000000000000'),
    ('delegation',    'Delegasi',        'Kode delegasi baru: MDL001, …',       'MDL', 3, '00000000-0000-7000-8000-000000000000'),
    ('custom_flow',   'Approval Custom', 'Kode approval custom baru: MAC001, …', 'MAC', 3, '00000000-0000-7000-8000-000000000000');
