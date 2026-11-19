-- Logistics masters (Stage FF-1C-1), global in Portal Master: Pelabuhan & Lokasi (Geografi & Lokasi); Incoterms,
-- Jenis Kontainer, Kemasan, Satuan and Komoditas (Logistik & Pengiriman). Each keeps its international code (UN/LOCODE,
-- ICC Incoterms, ISO 6346, UN/ECE Rec 21 and Rec 20, HS) in a column of its own beside the generated code.
--   location        a port, airport, dry port (ICD), rail terminal or border crossing in a country (and city)
--   incoterm        a delivery term: who pays the main carriage and insurance, who clears import, where risk passes
--   container_type  a container size/type: length, height, group, TEU, weights, volume
--   packaging       a kind of package (carton, bag, drum, pallet, …)
--   uom             a unit of measure with its dimension and factor to the dimension's base unit (kg, m³, m, m², piece, hour)
--   commodity       a kind of cargo with its HS code, dangerous goods data and default unit and packaging

CREATE TABLE pm.location (
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

    -- Location Attributes
    unlocode                char(5)      NULL,          -- UN/LOCODE, e.g. IDTPP (country + 3)
    country_id              uuid         NOT NULL,
    city_id                 uuid         NULL,
    kind                    varchar(20)  NOT NULL,      -- seaport, river_port, airport, dry_port, rail_terminal, border, other
    iata_code               char(3)      NULL,          -- airports, e.g. CGK
    customs_office          varchar(6)   NULL,          -- kode kantor pabean (KPPBC), 6 digits
    latitude                numeric(9,6) NULL,
    longitude               numeric(9,6) NULL,

    -- Constraints
    CONSTRAINT location_pkey PRIMARY KEY (id),
    CONSTRAINT location_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT location_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT location_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT location_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT location_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT location_country_fk FOREIGN KEY (country_id) REFERENCES pm.country (id),
    CONSTRAINT location_city_fk FOREIGN KEY (city_id) REFERENCES pm.city (id),
    CONSTRAINT location_unlocode_check CHECK (unlocode IS NULL OR unlocode ~ '^[A-Z]{2}[A-Z2-9]{3}$'),
    CONSTRAINT location_kind_check CHECK (kind IN ('seaport', 'river_port', 'airport', 'dry_port', 'rail_terminal', 'border', 'other')),
    CONSTRAINT location_iata_check CHECK (iata_code IS NULL OR (iata_code ~ '^[A-Z]{3}$' AND kind = 'airport')),
    CONSTRAINT location_customs_check CHECK (customs_office IS NULL OR customs_office ~ '^[0-9]{6}$'),
    CONSTRAINT location_position_check CHECK ((latitude IS NULL) = (longitude IS NULL)
        AND (latitude IS NULL OR (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180)))
);
CREATE UNIQUE INDEX location_code_ci ON pm.location (tenant_id, lower(code));
CREATE INDEX idx_location_active ON pm.location (tenant_id, is_active, is_deleted);
ALTER TABLE pm.location ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.location USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX location_unlocode_live ON pm.location (tenant_id, unlocode) WHERE NOT is_deleted AND unlocode IS NOT NULL;
CREATE UNIQUE INDEX location_iata_live ON pm.location (tenant_id, iata_code) WHERE NOT is_deleted AND iata_code IS NOT NULL;
CREATE INDEX idx_location_country ON pm.location (country_id);
CREATE INDEX idx_location_city ON pm.location (city_id);
CREATE TABLE pm.incoterm (
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

    -- Incoterm Attributes
    term                    char(3)      NOT NULL,      -- EXW, FCA, FAS, FOB, CFR, CIF, CPT, CIP, DAP, DPU, DDP
    edition                 smallint     NOT NULL DEFAULT 2020,
    any_mode                boolean      NOT NULL DEFAULT true,   -- false: sea and inland waterway only
    seller_pays_carriage    boolean      NOT NULL DEFAULT false,  -- the main carriage
    seller_insures          boolean      NOT NULL DEFAULT false,
    seller_clears_import    boolean      NOT NULL DEFAULT false,
    risk_point              varchar(200) NULL,          -- where the risk passes to the buyer

    -- Constraints
    CONSTRAINT incoterm_pkey PRIMARY KEY (id),
    CONSTRAINT incoterm_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT incoterm_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT incoterm_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT incoterm_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT incoterm_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT incoterm_term_check CHECK (term ~ '^[A-Z]{3}$'),
    CONSTRAINT incoterm_edition_check CHECK (edition IN (2010, 2020))
);
CREATE UNIQUE INDEX incoterm_code_ci ON pm.incoterm (tenant_id, lower(code));
CREATE INDEX idx_incoterm_active ON pm.incoterm (tenant_id, is_active, is_deleted);
ALTER TABLE pm.incoterm ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.incoterm USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX incoterm_term_live ON pm.incoterm (tenant_id, term, edition) WHERE NOT is_deleted;
CREATE TABLE pm.container_type (
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

    -- Container Type Attributes
    iso_code                char(4)      NULL,          -- ISO 6346 size-type, e.g. 45G1
    length_ft               smallint     NOT NULL,
    height                  varchar(10)  NOT NULL DEFAULT 'standard',  -- standard (8'6") | high_cube (9'6")
    type_group              varchar(20)  NOT NULL,      -- general, reefer, open_top, flat_rack, tank, platform, ventilated, bulk, other
    teu                     numeric(4,2) NOT NULL,
    max_gross_kg            integer      NULL,
    tare_kg                 integer      NULL,
    inner_volume_cbm        numeric(6,2) NULL,

    -- Constraints
    CONSTRAINT container_type_pkey PRIMARY KEY (id),
    CONSTRAINT container_type_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT container_type_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT container_type_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT container_type_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT container_type_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT container_type_iso_check CHECK (iso_code IS NULL OR iso_code ~ '^[0-9][0-9A-Z][A-Z][0-9A-Z]$'),
    CONSTRAINT container_type_length_check CHECK (length_ft IN (10, 20, 40, 45, 48, 53)),
    CONSTRAINT container_type_height_check CHECK (height IN ('standard', 'high_cube')),
    CONSTRAINT container_type_group_check CHECK (type_group IN ('general', 'reefer', 'open_top', 'flat_rack', 'tank', 'platform', 'ventilated', 'bulk', 'other')),
    CONSTRAINT container_type_teu_check CHECK (teu > 0),
    CONSTRAINT container_type_weight_check CHECK ((max_gross_kg IS NULL OR max_gross_kg > 0) AND (tare_kg IS NULL OR tare_kg >= 0)
        AND (max_gross_kg IS NULL OR tare_kg IS NULL OR tare_kg < max_gross_kg)),
    CONSTRAINT container_type_volume_check CHECK (inner_volume_cbm IS NULL OR inner_volume_cbm > 0)
);
CREATE UNIQUE INDEX container_type_code_ci ON pm.container_type (tenant_id, lower(code));
CREATE INDEX idx_container_type_active ON pm.container_type (tenant_id, is_active, is_deleted);
ALTER TABLE pm.container_type ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.container_type USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX container_type_iso_live ON pm.container_type (tenant_id, iso_code) WHERE NOT is_deleted AND iso_code IS NOT NULL;
CREATE TABLE pm.packaging (
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

    -- Packaging Attributes
    unece_code              char(2)      NULL,          -- UN/ECE Recommendation 21, e.g. CT carton
    max_weight_kg           numeric(10,2) NULL,

    -- Constraints
    CONSTRAINT packaging_pkey PRIMARY KEY (id),
    CONSTRAINT packaging_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT packaging_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT packaging_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT packaging_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT packaging_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT packaging_unece_check CHECK (unece_code IS NULL OR unece_code ~ '^[0-9A-Z]{2}$'),
    CONSTRAINT packaging_weight_check CHECK (max_weight_kg IS NULL OR max_weight_kg > 0)
);
CREATE UNIQUE INDEX packaging_code_ci ON pm.packaging (tenant_id, lower(code));
CREATE INDEX idx_packaging_active ON pm.packaging (tenant_id, is_active, is_deleted);
ALTER TABLE pm.packaging ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.packaging USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX packaging_unece_live ON pm.packaging (tenant_id, unece_code) WHERE NOT is_deleted AND unece_code IS NOT NULL;
CREATE TABLE pm.uom (
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

    -- Unit of Measure Attributes
    unece_code              varchar(3)   NULL,          -- UN/ECE Recommendation 20, e.g. KGM
    dimension               varchar(10)  NOT NULL,      -- mass, volume, length, area, count, time, other
    factor                  numeric(24,10) NULL,        -- in the dimension's base unit (kg, m³, m, m², piece, hour); NULL: no fixed conversion

    -- Constraints
    CONSTRAINT uom_pkey PRIMARY KEY (id),
    CONSTRAINT uom_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT uom_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT uom_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT uom_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT uom_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT uom_unece_check CHECK (unece_code IS NULL OR unece_code ~ '^[0-9A-Z]{2,3}$'),
    CONSTRAINT uom_dimension_check CHECK (dimension IN ('mass', 'volume', 'length', 'area', 'count', 'time', 'other')),
    CONSTRAINT uom_factor_check CHECK (factor IS NULL OR factor > 0)
);
CREATE UNIQUE INDEX uom_code_ci ON pm.uom (tenant_id, lower(code));
CREATE INDEX idx_uom_active ON pm.uom (tenant_id, is_active, is_deleted);
ALTER TABLE pm.uom ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.uom USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE UNIQUE INDEX uom_unece_live ON pm.uom (tenant_id, unece_code) WHERE NOT is_deleted AND unece_code IS NOT NULL;
CREATE TABLE pm.commodity (
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

    -- Commodity Attributes
    hs_code                 varchar(10)  NULL,          -- HS / BTKI, digits only (4, 6, 8 or 10)
    is_dangerous            boolean      NOT NULL DEFAULT false,
    un_number               char(4)      NULL,          -- dangerous goods: UN number, e.g. 1203
    dg_class                varchar(3)   NULL,          -- dangerous goods: IMDG/IATA class, e.g. 3 or 2.1
    temperature_controlled  boolean      NOT NULL DEFAULT false,
    uom_id                  uuid         NULL,          -- default unit
    packaging_id            uuid         NULL,          -- default packaging

    -- Constraints
    CONSTRAINT commodity_pkey PRIMARY KEY (id),
    CONSTRAINT commodity_code_check CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9_-]*$'),
    CONSTRAINT commodity_name_check CHECK (btrim(name) <> ''),
    CONSTRAINT commodity_approval_level_check CHECK (approval_level_required IS NULL OR approval_level_required >= 1),
    CONSTRAINT commodity_validity_check CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT commodity_delete_audit_check CHECK (
        is_deleted = (deleted_at IS NOT NULL) AND (NOT is_deleted OR deleted_by IS NOT NULL)
    ),
    CONSTRAINT commodity_hs_check CHECK (hs_code IS NULL OR hs_code ~ '^([0-9]{4}|[0-9]{6}|[0-9]{8}|[0-9]{10})$'),
    CONSTRAINT commodity_dg_check CHECK (CASE WHEN is_dangerous THEN un_number ~ '^[0-9]{4}$' AND dg_class ~ '^[1-9](\.[1-6])?$'
        ELSE un_number IS NULL AND dg_class IS NULL END),
    CONSTRAINT commodity_uom_fk FOREIGN KEY (uom_id) REFERENCES pm.uom (id),
    CONSTRAINT commodity_packaging_fk FOREIGN KEY (packaging_id) REFERENCES pm.packaging (id)
);
CREATE UNIQUE INDEX commodity_code_ci ON pm.commodity (tenant_id, lower(code));
CREATE INDEX idx_commodity_active ON pm.commodity (tenant_id, is_active, is_deleted);
ALTER TABLE pm.commodity ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.commodity USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
CREATE INDEX idx_commodity_hs ON pm.commodity (tenant_id, hs_code);
CREATE INDEX idx_commodity_uom ON pm.commodity (uom_id);
CREATE INDEX idx_commodity_packaging ON pm.commodity (packaging_id);
-- ---------------------------------------------------------------- Generated codes of every existing tenant
INSERT INTO pm.code_prefix (tenant_id, code, name, prefix, digits, created_by)
SELECT t.tenant_id, x.code, x.name, x.prefix, 3, '00000000-0000-7000-8000-000000000000'
FROM (SELECT DISTINCT tenant_id FROM pm.code_prefix) t
CROSS JOIN (VALUES ('location', 'Pelabuhan & Lokasi', 'MLK'), ('incoterm', 'Incoterms', 'MIC'), ('container_type', 'Jenis Kontainer', 'MKN'),
                   ('packaging', 'Kemasan', 'MKM'), ('uom', 'Satuan', 'MST'), ('commodity', 'Komoditas', 'MKD')) AS x(code, name, prefix)
WHERE NOT EXISTS (SELECT 1 FROM pm.code_prefix p WHERE p.tenant_id = t.tenant_id AND lower(p.code) = x.code);

-- The application role reads and writes the new tables (deploy/grants.sql does the same on every migrate).
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alir_pm_app') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON pm.location, pm.incoterm, pm.container_type, pm.packaging, pm.uom, pm.commodity TO alir_pm_app;
    END IF;
END $$;
