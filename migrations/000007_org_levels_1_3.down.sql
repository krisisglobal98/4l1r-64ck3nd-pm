-- Refused by the foreign keys while units or job titles use these levels.
DELETE FROM pm.org_level WHERE code IN ('org1', 'org2', 'org3');
