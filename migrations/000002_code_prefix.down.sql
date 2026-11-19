DROP TABLE pm.code_prefix;

ALTER TABLE pm.org_level ADD COLUMN scope_level varchar(60) NULL;
UPDATE pm.org_level SET scope_level = name;
CREATE INDEX idx_org_level_scope_level ON pm.org_level (scope_level);
