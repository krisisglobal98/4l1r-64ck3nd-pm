-- Data access of the application role on this database (make db-roles runs it after the migrations).
DO $$ BEGIN EXECUTE format('GRANT CONNECT ON DATABASE %I TO alir_pm_app', current_database()); END $$;
GRANT USAGE ON SCHEMA pm TO alir_pm_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA pm TO alir_pm_app;
GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA pm TO alir_pm_app;
