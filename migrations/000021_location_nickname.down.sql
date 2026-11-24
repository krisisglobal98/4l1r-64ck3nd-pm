UPDATE pm.doc_numbering SET scope = 'global' WHERE scope = 'port';
ALTER TABLE pm.doc_numbering DROP CONSTRAINT doc_numbering_scope_check;
ALTER TABLE pm.doc_numbering ADD CONSTRAINT doc_numbering_scope_check CHECK (scope IN ('global', 'company', 'unit'));
DROP INDEX pm.location_nickname_live;
ALTER TABLE pm.location DROP CONSTRAINT location_nickname_check;
ALTER TABLE pm.location DROP COLUMN nickname;
