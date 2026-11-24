-- Singkatan pelabuhan / lokasi (e.g. JKT, BLW): the short name documents print and the Operation job number ends with
-- ({SEQ:4}/{MM}/{YY}/{PORT}, PORT = the origin port's nickname). One live location per nickname in a tenant, so two
-- ports never give the same job number.
ALTER TABLE pm.location ADD COLUMN nickname varchar(20) NULL;
ALTER TABLE pm.location ADD CONSTRAINT location_nickname_check CHECK (nickname IS NULL OR nickname ~ '^[A-Z0-9][A-Z0-9 .-]{0,19}$');
CREATE UNIQUE INDEX location_nickname_live ON pm.location (tenant_id, nickname) WHERE NOT is_deleted AND nickname IS NOT NULL;

-- Numbering: a counter per origin port (scope port, with {PORT} in the pattern), as the Operation job number of
-- LKT counts per origin port and year: 0001/10/26/BLW.
ALTER TABLE pm.doc_numbering DROP CONSTRAINT doc_numbering_scope_check;
ALTER TABLE pm.doc_numbering ADD CONSTRAINT doc_numbering_scope_check CHECK (scope IN ('global', 'company', 'unit', 'port'));
