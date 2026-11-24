ALTER TABLE pm.container_type DROP CONSTRAINT container_type_iso_check;
ALTER TABLE pm.container_type ADD CONSTRAINT container_type_iso_check CHECK (iso_code IS NULL OR iso_code ~ '^[0-9][0-9A-Z][A-Z][0-9A-Z]$');
