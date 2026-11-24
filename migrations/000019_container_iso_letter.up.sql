-- ISO 6346 size codes start with a letter for the longer boxes (L = 45', M = 48', P = 53'…), e.g. L5G1 for a
-- 45' high cube: the first character may be a letter too.
ALTER TABLE pm.container_type DROP CONSTRAINT container_type_iso_check;
ALTER TABLE pm.container_type ADD CONSTRAINT container_type_iso_check CHECK (iso_code IS NULL OR iso_code ~ '^[0-9A-Z][0-9A-Z][A-Z][0-9A-Z]$');
