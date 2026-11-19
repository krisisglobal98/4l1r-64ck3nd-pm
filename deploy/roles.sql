-- The application role of the service: data access only, subject to Row-Level Security (not the owner, not a
-- superuser). Run once per database cluster as a superuser:  make db-roles  (password from PM_APP_PASSWORD).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'alir_pm_app') THEN
        EXECUTE format('CREATE ROLE alir_pm_app LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS', current_setting('pm.app_password'));
    ELSE
        EXECUTE format('ALTER ROLE alir_pm_app LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS', current_setting('pm.app_password'));
    END IF;
END $$;
