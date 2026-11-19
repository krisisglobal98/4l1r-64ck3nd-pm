-- Generic organisation levels under the operational unit, for every kind of company: a plantation names them
-- Rayon, Afdeling, Blok; another company something else, or does not use them. Their units are made in Unit
-- Organisasi under an operational unit (Unit Operasional → Org 1 → Org 2 → Org 3); user scopes reach down to them
-- (Authentication: Organisasi Level 1–3). Built in like the five above: renamed, never deleted.
INSERT INTO pm.org_level (code, name, description, rank, created_by) VALUES
    ('org1', 'Organisasi Level 1', 'Di bawah Unit Operasional. Beri nama sesuai perusahaan, mis. Rayon.',   6, '00000000-0000-7000-8000-000000000000'),
    ('org2', 'Organisasi Level 2', 'Di bawah Organisasi Level 1. Mis. Afdeling.',                          7, '00000000-0000-7000-8000-000000000000'),
    ('org3', 'Organisasi Level 3', 'Di bawah Organisasi Level 2. Mis. Blok.',                              8, '00000000-0000-7000-8000-000000000000');
