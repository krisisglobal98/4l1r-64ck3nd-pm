-- Employee Management masters served here: Karyawan and the masters it uses (status kepegawaian, jenjang
-- pendidikan, shift kerja, bank). With employees served, the holder, acting holder and delegate columns of
-- positions and delegations get their foreign keys (they were left NULL until now).

-- ---------------------------------------------------------------- Status Kepegawaian
CREATE TABLE pm.employment_status (
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

    -- Employment Status Attributes
    is_permanent            boolean      NOT NULL DEFAULT false,
    contract_months         integer      NOT NULL DEFAULT 0,  -- 0 = no fixed term

    -- Constraints
    CONSTRAINT employment_status_pkey PRIMARY KEY (id),
    CONSTRAINT employment_status_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT employment_status_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT employment_status_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT employment_status_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT employment_status_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT employment_status_contract_check CHECK (contract_months BETWEEN 0 AND 120),
    CONSTRAINT employment_status_permanent_check CHECK (NOT is_permanent OR contract_months = 0)
);
CREATE UNIQUE INDEX employment_status_code_ci ON pm.employment_status (lower(code));
CREATE INDEX idx_employment_status_active ON pm.employment_status (is_active, is_deleted);

-- ---------------------------------------------------------------- Jenjang Pendidikan
CREATE TABLE pm.education_level (
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

    -- Education Level Attributes
    rank                    integer      NOT NULL,  -- higher = higher education

    -- Constraints
    CONSTRAINT education_level_pkey PRIMARY KEY (id),
    CONSTRAINT education_level_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT education_level_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT education_level_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT education_level_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT education_level_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT education_level_rank_check CHECK (rank >= 1)
);
CREATE UNIQUE INDEX education_level_code_ci ON pm.education_level (lower(code));
CREATE INDEX idx_education_level_active ON pm.education_level (is_active, is_deleted);
CREATE UNIQUE INDEX education_level_rank_live ON pm.education_level (rank) WHERE NOT is_deleted;

-- ---------------------------------------------------------------- Shift Kerja
CREATE TABLE pm.work_shift (
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

    -- Work Shift Attributes (local wall-clock times of the work place, not instants)
    start_time              time         NOT NULL,
    end_time                time         NOT NULL,
    is_night                boolean      NOT NULL DEFAULT false,

    -- Constraints
    CONSTRAINT work_shift_pkey PRIMARY KEY (id),
    CONSTRAINT work_shift_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT work_shift_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT work_shift_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT work_shift_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT work_shift_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    -- A shift that ends on the next day (end not after start) is a night shift.
    CONSTRAINT work_shift_night_check CHECK (end_time > start_time OR is_night)
);
CREATE UNIQUE INDEX work_shift_code_ci ON pm.work_shift (lower(code));
CREATE INDEX idx_work_shift_active ON pm.work_shift (is_active, is_deleted);

-- ---------------------------------------------------------------- Bank
CREATE TABLE pm.bank (
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

    -- Bank Attributes
    clearing_code           varchar(10)  NULL,

    -- Constraints
    CONSTRAINT bank_pkey PRIMARY KEY (id),
    CONSTRAINT bank_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT bank_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT bank_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT bank_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT bank_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT bank_clearing_check CHECK (clearing_code IS NULL OR clearing_code ~ '^[0-9]{3,7}$')
);
CREATE UNIQUE INDEX bank_code_ci ON pm.bank (lower(code));
CREATE INDEX idx_bank_active ON pm.bank (is_active, is_deleted);

-- ---------------------------------------------------------------- Karyawan (name = full name)
CREATE TABLE pm.employee (
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

    -- Employee Attributes
    nik                     varchar(20)  NOT NULL, -- employee number of the company
    join_date               date         NULL,
    employment_status_id    uuid         NOT NULL,
    education_level_id      uuid         NULL,
    work_shift_id           uuid         NULL,
    bank_id                 uuid         NULL,

    -- Constraints
    CONSTRAINT employee_pkey PRIMARY KEY (id),
    CONSTRAINT employee_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT employee_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT employee_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT employee_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT employee_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT employee_nik_check CHECK (nik ~ '^[A-Za-z0-9][A-Za-z0-9._/-]*$'),
    CONSTRAINT employee_status_fk FOREIGN KEY (employment_status_id) REFERENCES pm.employment_status (id),
    CONSTRAINT employee_education_fk FOREIGN KEY (education_level_id) REFERENCES pm.education_level (id),
    CONSTRAINT employee_shift_fk FOREIGN KEY (work_shift_id) REFERENCES pm.work_shift (id),
    CONSTRAINT employee_bank_fk FOREIGN KEY (bank_id) REFERENCES pm.bank (id)
);
CREATE UNIQUE INDEX employee_code_ci ON pm.employee (lower(code));
CREATE INDEX idx_employee_active ON pm.employee (is_active, is_deleted);
-- One live employee per NIK (any case); a deleted employee's NIK may be given again.
CREATE UNIQUE INDEX employee_nik_live ON pm.employee (lower(nik)) WHERE NOT is_deleted;
CREATE INDEX idx_employee_status ON pm.employee (employment_status_id);
CREATE INDEX idx_employee_education ON pm.employee (education_level_id);
CREATE INDEX idx_employee_shift ON pm.employee (work_shift_id);
CREATE INDEX idx_employee_bank ON pm.employee (bank_id);
CREATE INDEX idx_employee_name ON pm.employee (lower(name));

-- ---------------------------------------------------------------- Holders and delegates are employees now
ALTER TABLE pm.position
    ADD CONSTRAINT position_holder_fk FOREIGN KEY (holder_employee_id) REFERENCES pm.employee (id),
    ADD CONSTRAINT position_acting_fk FOREIGN KEY (acting_employee_id) REFERENCES pm.employee (id),
    ADD CONSTRAINT position_acting_not_holder CHECK (acting_employee_id IS NULL OR acting_employee_id IS DISTINCT FROM holder_employee_id);
CREATE INDEX idx_position_holder ON pm.position (holder_employee_id);
CREATE INDEX idx_position_acting ON pm.position (acting_employee_id);
ALTER TABLE pm.delegation
    ADD CONSTRAINT delegation_delegate_fk FOREIGN KEY (delegate_employee_id) REFERENCES pm.employee (id);
CREATE INDEX idx_delegation_delegate ON pm.delegation (delegate_employee_id);

INSERT INTO pm.code_prefix (code, name, description, prefix, digits, created_by) VALUES
    ('employment_status', 'Status Kepegawaian', 'Kode status baru: MSK001, …',  'MSK', 3, '00000000-0000-7000-8000-000000000000'),
    ('education_level',   'Jenjang Pendidikan', 'Kode jenjang baru: MJP001, …', 'MJP', 3, '00000000-0000-7000-8000-000000000000'),
    ('work_shift',        'Shift Kerja',        'Kode shift baru: MSF001, …',   'MSF', 3, '00000000-0000-7000-8000-000000000000'),
    ('bank',              'Bank',               'Kode bank baru: MBK001, …',    'MBK', 3, '00000000-0000-7000-8000-000000000000'),
    ('employee',          'Karyawan',           'Kode karyawan baru: MKY0001, …', 'MKY', 4, '00000000-0000-7000-8000-000000000000');
