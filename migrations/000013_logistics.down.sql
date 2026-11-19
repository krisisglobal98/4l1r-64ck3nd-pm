DELETE FROM pm.code_prefix WHERE code IN ('location', 'incoterm', 'container_type', 'packaging', 'uom', 'commodity');
DROP TABLE pm.commodity;
DROP TABLE pm.uom;
DROP TABLE pm.packaging;
DROP TABLE pm.container_type;
DROP TABLE pm.incoterm;
DROP TABLE pm.location;
