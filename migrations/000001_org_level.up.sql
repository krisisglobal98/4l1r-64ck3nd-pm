-- Portal Master: organization levels, the transactional outbox, and the helpers every master table uses.
--
-- Every master table starts with the standard columns (id, master data, status, workflow, validity, audit, version)
-- and puts the columns of its own page after them. Times are timestamptz: stored as an absolute instant (UTC),
-- never in a regional zone; the service connects with timezone=UTC and takes every time from the database's now().
CREATE SCHEMA IF NOT EXISTS pm;

-- UUID version 7 (time-ordered, RFC 9562): PostgreSQL 14 has no uuidv7(), so it is built from gen_random_uuid()
-- with the first 48 bits replaced by the Unix time in milliseconds.
CREATE FUNCTION pm.uuid_v7() RETURNS uuid
    LANGUAGE plpgsql VOLATILE AS
$$
DECLARE
    u  bytea := uuid_send(gen_random_uuid());
    ms bytea := substring(int8send(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint) FROM 3);
BEGIN
    u := overlay(u PLACING ms FROM 1 FOR 6);
    u := set_byte(u, 6, (get_byte(u, 6) & 15) | 112); -- version 7; the variant bits stay those of v4
    RETURN encode(u, 'hex')::uuid;
END
$$;

CREATE TABLE pm.org_level (
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

    -- Organization Level Attributes
    rank                    integer      NOT NULL,
    scope_level             varchar(60)  NULL,

    -- Constraints
    CONSTRAINT org_level_pkey PRIMARY KEY (id),
    CONSTRAINT org_level_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT org_level_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT org_level_rank_check CHECK (rank >= 1),
    CONSTRAINT org_level_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT org_level_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT org_level_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    )
);

-- Case-insensitive unique code, deleted rows included: units and job titles refer to a level by its code, so a
-- deleted code is never handed to a new level.
CREATE UNIQUE INDEX org_level_code_ci ON pm.org_level (lower(code));

-- Lookup indexes
CREATE INDEX idx_org_level_active ON pm.org_level (is_active, is_deleted);
CREATE INDEX idx_org_level_rank ON pm.org_level (rank);
CREATE INDEX idx_org_level_scope_level ON pm.org_level (scope_level);

-- Written in the same transaction as the change; a relay publishes rows with published_at NULL to the event bus.
CREATE TABLE pm.outbox (
    id             uuid         NOT NULL DEFAULT pm.uuid_v7(),
    aggregate_type varchar(50)  NOT NULL,
    aggregate_id   uuid         NOT NULL,
    aggregate_code varchar(40)  NULL,
    event_type     varchar(100) NOT NULL,
    payload        jsonb        NOT NULL,
    actor_id       uuid         NOT NULL,
    request_id     varchar(64)  NULL,
    occurred_at    timestamptz  NOT NULL DEFAULT now(),
    published_at   timestamptz  NULL,
    CONSTRAINT outbox_pkey PRIMARY KEY (id)
);
CREATE INDEX outbox_unpublished ON pm.outbox (occurred_at) WHERE published_at IS NULL;

-- The five levels the tree is built on, created by the system user.
INSERT INTO pm.org_level (code, name, rank, scope_level, created_by) VALUES
    ('group',   'Group',            1, 'Group',            '00000000-0000-7000-8000-000000000000'),
    ('region',  'Region',           2, 'Region',           '00000000-0000-7000-8000-000000000000'),
    ('wilayah', 'Wilayah',          3, 'Wilayah',          '00000000-0000-7000-8000-000000000000'),
    ('company', 'Perusahaan',       4, 'Perusahaan',       '00000000-0000-7000-8000-000000000000'),
    ('unit',    'Unit Operasional', 5, 'Unit Operasional', '00000000-0000-7000-8000-000000000000');
