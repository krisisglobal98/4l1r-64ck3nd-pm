-- Masters of the Meeting Room module (its bookings live in their own service, ALIR_MR_SERVICE):
--   building           Gedung & Kantor (Geografi & Lokasi): where rooms are, in a city, with the time zone its wall
--                      clock runs on (opening hours of its rooms are that clock; instants stay UTC everywhere).
--   room_amenity       Fasilitas Ruang: projector, video conference, whiteboard, ... (what a room offers, a filter).
--   meeting_room       Ruang Rapat: a bookable room of a building: seats, floor, opening hours, longest booking,
--                      whether a booking waits for approval, and who looks after it.
--   meeting_room_amenity  the amenities of a room.

CREATE TABLE pm.building (
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
    -- Building Attributes
    city_id                 uuid         NOT NULL,           -- Kota
    org_unit_id             uuid         NULL,               -- Unit Organisasi that runs it (optional)
    address                 varchar(500) NULL,
    time_zone               varchar(40)  NOT NULL DEFAULT 'Asia/Jakarta',  -- IANA zone of its wall clock
    floors                  smallint     NULL,               -- number of floors, for the room's floor
    CONSTRAINT building_pkey PRIMARY KEY (id),
    CONSTRAINT building_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT building_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT building_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT building_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT building_delete_audit_check CHECK (is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)),
    CONSTRAINT building_zone_check CHECK (time_zone ~ '^[A-Za-z_]+(/[A-Za-z0-9_+-]+){0,2}$'),
    CONSTRAINT building_floors_check CHECK (floors IS NULL OR floors BETWEEN 1 AND 200),
    CONSTRAINT building_city_fk FOREIGN KEY (city_id) REFERENCES pm.city (id),
    CONSTRAINT building_unit_fk FOREIGN KEY (org_unit_id) REFERENCES pm.org_unit (id)
);
CREATE UNIQUE INDEX building_code_ci ON pm.building (tenant_id, lower(code));
CREATE UNIQUE INDEX building_name_live ON pm.building (tenant_id, city_id, lower(name)) WHERE NOT is_deleted;
CREATE INDEX idx_building_active ON pm.building (tenant_id, is_active, is_deleted);
CREATE INDEX idx_building_city ON pm.building (city_id);
CREATE INDEX idx_building_unit ON pm.building (org_unit_id);
ALTER TABLE pm.building ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.building USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

CREATE TABLE pm.room_amenity (
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
    -- Room Amenity Attributes
    icon                    varchar(20)  NOT NULL DEFAULT 'other',   -- the picture the pages show
    CONSTRAINT room_amenity_pkey PRIMARY KEY (id),
    CONSTRAINT room_amenity_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT room_amenity_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT room_amenity_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT room_amenity_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT room_amenity_delete_audit_check CHECK (is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)),
    CONSTRAINT room_amenity_icon_check CHECK (icon IN ('projector', 'screen', 'video', 'audio', 'whiteboard', 'wifi', 'phone', 'ac', 'pantry', 'accessible', 'other'))
);
CREATE UNIQUE INDEX room_amenity_code_ci ON pm.room_amenity (tenant_id, lower(code));
CREATE UNIQUE INDEX room_amenity_name_live ON pm.room_amenity (tenant_id, lower(name)) WHERE NOT is_deleted;
CREATE INDEX idx_room_amenity_active ON pm.room_amenity (tenant_id, is_active, is_deleted);
ALTER TABLE pm.room_amenity ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.room_amenity USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

CREATE TABLE pm.meeting_room (
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
    -- Meeting Room Attributes
    building_id             uuid         NOT NULL,           -- Gedung & Kantor
    floor                   varchar(20)  NULL,               -- e.g. 3, LG, Mezzanine
    capacity                smallint     NOT NULL,           -- seats
    open_time               time         NOT NULL DEFAULT '08:00',   -- wall clock of the building
    close_time              time         NOT NULL DEFAULT '18:00',
    max_duration_minutes    integer      NULL,               -- longest booking; NULL = until closing
    needs_approval          boolean      NOT NULL DEFAULT false,     -- a booking waits for Persetujuan
    weekend_bookable        boolean      NOT NULL DEFAULT false,
    pic_employee_id         uuid         NULL,               -- Karyawan who looks after the room
    CONSTRAINT meeting_room_pkey PRIMARY KEY (id),
    CONSTRAINT meeting_room_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT meeting_room_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT meeting_room_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT meeting_room_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT meeting_room_delete_audit_check CHECK (is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)),
    CONSTRAINT meeting_room_capacity_check CHECK (capacity BETWEEN 1 AND 1000),
    CONSTRAINT meeting_room_hours_check CHECK (close_time > open_time AND extract(minute FROM open_time)::int % 15 = 0
        AND extract(minute FROM close_time)::int % 15 = 0),
    CONSTRAINT meeting_room_duration_check CHECK (max_duration_minutes IS NULL OR (max_duration_minutes BETWEEN 15 AND 1440 AND max_duration_minutes % 15 = 0)),
    CONSTRAINT meeting_room_building_fk FOREIGN KEY (building_id) REFERENCES pm.building (id),
    CONSTRAINT meeting_room_pic_fk FOREIGN KEY (pic_employee_id) REFERENCES pm.employee (id)
);
CREATE UNIQUE INDEX meeting_room_code_ci ON pm.meeting_room (tenant_id, lower(code));
CREATE UNIQUE INDEX meeting_room_name_live ON pm.meeting_room (tenant_id, building_id, lower(name)) WHERE NOT is_deleted;
CREATE INDEX idx_meeting_room_active ON pm.meeting_room (tenant_id, is_active, is_deleted);
CREATE INDEX idx_meeting_room_building ON pm.meeting_room (building_id);
CREATE INDEX idx_meeting_room_pic ON pm.meeting_room (pic_employee_id);
ALTER TABLE pm.meeting_room ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.meeting_room USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

CREATE TABLE pm.meeting_room_amenity (
    tenant_id               uuid         NOT NULL DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid,
    meeting_room_id         uuid         NOT NULL,
    room_amenity_id         uuid         NOT NULL,
    seq                     smallint     NOT NULL,           -- order on the form, from 1
    CONSTRAINT meeting_room_amenity_pkey PRIMARY KEY (meeting_room_id, room_amenity_id),
    CONSTRAINT meeting_room_amenity_room_fk FOREIGN KEY (meeting_room_id) REFERENCES pm.meeting_room (id) ON DELETE CASCADE,
    CONSTRAINT meeting_room_amenity_amenity_fk FOREIGN KEY (room_amenity_id) REFERENCES pm.room_amenity (id)
);
CREATE INDEX idx_meeting_room_amenity_amenity ON pm.meeting_room_amenity (room_amenity_id);
ALTER TABLE pm.meeting_room_amenity ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.meeting_room_amenity USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

-- ---------------------------------------------------------------- Generated codes of every existing tenant
INSERT INTO pm.code_prefix (tenant_id, code, name, prefix, digits, created_by)
SELECT t.tenant_id, x.code, x.name, x.prefix, 3, '00000000-0000-7000-8000-000000000000'
FROM (SELECT DISTINCT tenant_id FROM pm.code_prefix) t
CROSS JOIN (VALUES ('building', 'Gedung & Kantor', 'MGD'), ('room_amenity', 'Fasilitas Ruang', 'MFR'),
                   ('meeting_room', 'Ruang Rapat', 'MRR')) AS x(code, name, prefix)
WHERE NOT EXISTS (SELECT 1 FROM pm.code_prefix p WHERE p.tenant_id = t.tenant_id AND lower(p.code) = x.code);

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alir_pm_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON pm.building, pm.room_amenity, pm.meeting_room, pm.meeting_room_amenity TO alir_pm_app;
    END IF;
END $$;
