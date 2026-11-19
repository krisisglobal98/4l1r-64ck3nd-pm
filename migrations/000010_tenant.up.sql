-- Multi-tenant: one installation serves many companies (tenants). Every table gets tenant_id, unique keys become
-- unique per tenant, and Row-Level Security keeps each tenant's rows to itself: the service connects as the
-- application role alir_pm_app (not the owner, not a superuser: both bypass RLS) and sets app.tenant_id on its
-- connection from the access token (claim tid) for every request.
--
-- New rows take tenant_id from app.tenant_id (column default); with no tenant set the insert fails (NOT NULL).
-- The rows that exist belong to the first tenant, the company migrated from before tenants existed.
-- Migrations and seeds run as the owner (no RLS) and set app.tenant_id themselves when they insert.

ALTER TABLE pm.org_level ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.org_level ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.unit_type ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.unit_type ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.org_unit ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.org_unit ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.code_prefix ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.code_prefix ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.outbox ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.outbox ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.grade ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.grade ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.jabatan ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.jabatan ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.position ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.position ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.approval_rule ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.approval_rule ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.approval_rule_band ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.approval_rule_band ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.delegation ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.delegation ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.delegation_rule ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.delegation_rule ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.custom_flow ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.custom_flow ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.custom_flow_step ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.custom_flow_step ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.employment_status ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.employment_status ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.education_level ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.education_level ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.work_shift ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.work_shift ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.bank ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.bank ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;
ALTER TABLE pm.employee ADD COLUMN tenant_id uuid NOT NULL DEFAULT '00000000-0000-7000-8000-0000000000a1';
ALTER TABLE pm.employee ALTER COLUMN tenant_id SET DEFAULT (nullif(current_setting('app.tenant_id', true), ''))::uuid;

-- Unique per tenant
DROP INDEX pm.org_level_code_ci;
CREATE UNIQUE INDEX org_level_code_ci ON pm.org_level (tenant_id, lower(code));
DROP INDEX pm.org_level_rank_live;
CREATE UNIQUE INDEX org_level_rank_live ON pm.org_level (tenant_id, rank) WHERE NOT is_deleted;
DROP INDEX pm.unit_type_code_ci;
CREATE UNIQUE INDEX unit_type_code_ci ON pm.unit_type (tenant_id, lower(code));
DROP INDEX pm.org_unit_code_ci;
CREATE UNIQUE INDEX org_unit_code_ci ON pm.org_unit (tenant_id, lower(code));
DROP INDEX pm.code_prefix_code_ci;
CREATE UNIQUE INDEX code_prefix_code_ci ON pm.code_prefix (tenant_id, lower(code));
DROP INDEX pm.grade_code_ci;
CREATE UNIQUE INDEX grade_code_ci ON pm.grade (tenant_id, lower(code));
DROP INDEX pm.grade_rank_live;
CREATE UNIQUE INDEX grade_rank_live ON pm.grade (tenant_id, rank) WHERE NOT is_deleted;
DROP INDEX pm.jabatan_code_ci;
CREATE UNIQUE INDEX jabatan_code_ci ON pm.jabatan (tenant_id, lower(code));
DROP INDEX pm.position_code_ci;
CREATE UNIQUE INDEX position_code_ci ON pm.position (tenant_id, lower(code));
DROP INDEX pm.approval_rule_code_ci;
CREATE UNIQUE INDEX approval_rule_code_ci ON pm.approval_rule (tenant_id, lower(code));
DROP INDEX pm.delegation_code_ci;
CREATE UNIQUE INDEX delegation_code_ci ON pm.delegation (tenant_id, lower(code));
DROP INDEX pm.custom_flow_code_ci;
CREATE UNIQUE INDEX custom_flow_code_ci ON pm.custom_flow (tenant_id, lower(code));
DROP INDEX pm.employment_status_code_ci;
CREATE UNIQUE INDEX employment_status_code_ci ON pm.employment_status (tenant_id, lower(code));
DROP INDEX pm.education_level_code_ci;
CREATE UNIQUE INDEX education_level_code_ci ON pm.education_level (tenant_id, lower(code));
DROP INDEX pm.education_level_rank_live;
CREATE UNIQUE INDEX education_level_rank_live ON pm.education_level (tenant_id, rank) WHERE NOT is_deleted;
DROP INDEX pm.work_shift_code_ci;
CREATE UNIQUE INDEX work_shift_code_ci ON pm.work_shift (tenant_id, lower(code));
DROP INDEX pm.bank_code_ci;
CREATE UNIQUE INDEX bank_code_ci ON pm.bank (tenant_id, lower(code));
DROP INDEX pm.employee_code_ci;
CREATE UNIQUE INDEX employee_code_ci ON pm.employee (tenant_id, lower(code));
DROP INDEX pm.employee_nik_live;
CREATE UNIQUE INDEX employee_nik_live ON pm.employee (tenant_id, lower(nik)) WHERE NOT is_deleted;
DROP INDEX pm.employee_email_live;
CREATE UNIQUE INDEX employee_email_live ON pm.employee (tenant_id, lower(email)) WHERE NOT is_deleted AND email IS NOT NULL;
CREATE INDEX idx_position_head_tenant ON pm.position (tenant_id);
DROP INDEX pm.position_head_per_unit;
CREATE UNIQUE INDEX position_head_per_unit ON pm.position (tenant_id, org_unit_id) WHERE is_head AND NOT is_deleted;
DROP INDEX pm.custom_flow_rule_scope_live;
CREATE UNIQUE INDEX custom_flow_rule_scope_live ON pm.custom_flow (tenant_id, approval_rule_id, scope_unit_id) WHERE NOT is_deleted;
CREATE INDEX idx_approval_rule_band_tenant ON pm.approval_rule_band (tenant_id);
CREATE INDEX idx_delegation_rule_tenant ON pm.delegation_rule (tenant_id);
CREATE INDEX idx_custom_flow_step_tenant ON pm.custom_flow_step (tenant_id);
CREATE INDEX idx_outbox_tenant ON pm.outbox (tenant_id);

-- Row-Level Security: a tenant sees and writes only its rows (the owner and superusers are not subject to it)
ALTER TABLE pm.org_level ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.org_level USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.unit_type ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.unit_type USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.org_unit ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.org_unit USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.code_prefix ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.code_prefix USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.outbox ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.outbox USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.grade ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.grade USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.jabatan ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.jabatan USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.position ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.position USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.approval_rule ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.approval_rule USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.approval_rule_band ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.approval_rule_band USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.delegation ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.delegation USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.delegation_rule ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.delegation_rule USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.custom_flow ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.custom_flow USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.custom_flow_step ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.custom_flow_step USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.employment_status ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.employment_status USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.education_level ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.education_level USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.work_shift ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.work_shift USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.bank ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.bank USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);
ALTER TABLE pm.employee ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pm.employee USING (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid)
    WITH CHECK (tenant_id = (nullif(current_setting('app.tenant_id', true), ''))::uuid);

-- The application role gets data access only (no DDL), when it exists (make db-roles makes it and grants again).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alir_pm_app') THEN
        GRANT USAGE ON SCHEMA pm TO alir_pm_app;
        GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA pm TO alir_pm_app;
        GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA pm TO alir_pm_app;
    END IF;
END $$;
