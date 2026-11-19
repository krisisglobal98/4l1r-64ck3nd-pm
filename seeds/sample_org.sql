-- SAMPLE DATA, not a migration: the organization of the ALIR sample (src/services/org/orgSample.ts) — units, grades,
-- job titles, positions, approval rules, delegations and custom approval flows — so a development or demo database
-- has the tree the pages show. Load it into development/demo databases only:  make seed-sample
-- Safe to run again: rows whose code exists (deleted ones included) are left as they are.
-- Position codes are the sample ids with "/" as "-" (C11-E1/EM → C11-E1-EM); employee codes are the sample ids (E001).
BEGIN;
-- The rows belong to the first company (tenant); every table takes tenant_id from this setting.
SET LOCAL app.tenant_id = '00000000-0000-7000-8000-0000000000a1';

CREATE TEMP TABLE s_unit (code text, name text, level_code text, parent_code text, unit_type_code text, location text, active boolean) ON COMMIT DROP;
INSERT INTO s_unit VALUES
    ('GAN', 'Grup Agro Nusantara', 'group', NULL, NULL, 'Jakarta', true),
    ('KAL', 'Region Kalimantan', 'region', 'GAN', NULL, NULL, true),
    ('SUM', 'Region Sumatera', 'region', 'GAN', NULL, NULL, true),
    ('KALBAR', 'Wilayah Kalimantan Barat', 'wilayah', 'KAL', NULL, NULL, true),
    ('KALTENG', 'Wilayah Kalimantan Tengah', 'wilayah', 'KAL', NULL, NULL, true),
    ('RIAU', 'Wilayah Riau', 'wilayah', 'SUM', NULL, NULL, true),
    ('C11', 'PT Sawit Khatulistiwa', 'company', 'KALBAR', NULL, NULL, true),
    ('C12', 'PT Kapuas Agro Lestari', 'company', 'KALBAR', NULL, NULL, true),
    ('C21', 'PT Katingan Hijau', 'company', 'KALTENG', NULL, NULL, true),
    ('C22', 'PT Mentaya Agro Makmur', 'company', 'KALTENG', NULL, NULL, true),
    ('C31', 'PT Riau Palma Lestari', 'company', 'RIAU', NULL, NULL, true),
    ('C32', 'PT Siak Sawit Sejahtera', 'company', 'RIAU', NULL, NULL, true),
    ('C11-E1', 'Kebun Sungai Ayak', 'unit', 'C11', 'EST', 'Sekadau', true),
    ('C11-E2', 'Kebun Nanga Tayap', 'unit', 'C11', 'EST', 'Ketapang', true),
    ('C11-M1', 'PKS Sungai Ayak', 'unit', 'C11', 'PKS', 'Sekadau', true),
    ('C12-E1', 'Kebun Kapuas Hulu', 'unit', 'C12', 'EST', 'Putussibau', true),
    ('C12-P1', 'Kebun Plasma Kapuas', 'unit', 'C12', 'PLS', 'Putussibau', true),
    ('C21-E1', 'Kebun Katingan', 'unit', 'C21', 'EST', 'Kasongan', true),
    ('C21-M1', 'PKS Katingan', 'unit', 'C21', 'PKS', 'Kasongan', true),
    ('C21-B1', 'Bulking Sampit', 'unit', 'C21', 'BLK', 'Sampit', true),
    ('C22-E1', 'Kebun Mentaya', 'unit', 'C22', 'EST', 'Kotawaringin Timur', true),
    ('C22-P1', 'Kebun Plasma Mentaya', 'unit', 'C22', 'PLS', 'Kotawaringin Timur', true),
    ('C31-E1', 'Kebun Kampar', 'unit', 'C31', 'EST', 'Kampar', true),
    ('C31-M1', 'PKS Kampar', 'unit', 'C31', 'PKS', 'Kampar', true),
    ('C31-K1', 'Kantor Pekanbaru', 'unit', 'C31', 'KTR', 'Pekanbaru', true),
    ('C32-E1', 'Kebun Siak', 'unit', 'C32', 'EST', 'Siak', true),
    ('C32-P1', 'Kebun Plasma Siak', 'unit', 'C32', 'PLS', 'Siak', true),
    ('C11-E1-R1', 'Rayon A', 'org1', 'C11-E1', NULL, NULL, true),
    ('C11-E1-R2', 'Rayon B', 'org1', 'C11-E1', NULL, NULL, true),
    ('C11-E2-R1', 'Rayon A', 'org1', 'C11-E2', NULL, NULL, true),
    ('C11-E2-R2', 'Rayon B', 'org1', 'C11-E2', NULL, NULL, true),
    ('C12-E1-R1', 'Rayon A', 'org1', 'C12-E1', NULL, NULL, true),
    ('C12-E1-R2', 'Rayon B', 'org1', 'C12-E1', NULL, NULL, true),
    ('C11-E1-R1-A1', 'Afdeling 01', 'org2', 'C11-E1-R1', NULL, NULL, true),
    ('C11-E1-R1-A2', 'Afdeling 02', 'org2', 'C11-E1-R1', NULL, NULL, true),
    ('C11-E1-R2-A1', 'Afdeling 03', 'org2', 'C11-E1-R2', NULL, NULL, true),
    ('C11-E1-R2-A2', 'Afdeling 04', 'org2', 'C11-E1-R2', NULL, NULL, true),
    ('C11-E2-R1-A1', 'Afdeling 01', 'org2', 'C11-E2-R1', NULL, NULL, true),
    ('C11-E2-R1-A2', 'Afdeling 02', 'org2', 'C11-E2-R1', NULL, NULL, true),
    ('C11-E2-R2-A1', 'Afdeling 03', 'org2', 'C11-E2-R2', NULL, NULL, true),
    ('C11-E2-R2-A2', 'Afdeling 04', 'org2', 'C11-E2-R2', NULL, NULL, true),
    ('C12-E1-R1-A1', 'Afdeling 01', 'org2', 'C12-E1-R1', NULL, NULL, true),
    ('C12-E1-R1-A2', 'Afdeling 02', 'org2', 'C12-E1-R1', NULL, NULL, true),
    ('C12-E1-R2-A1', 'Afdeling 03', 'org2', 'C12-E1-R2', NULL, NULL, true),
    ('C12-E1-R2-A2', 'Afdeling 04', 'org2', 'C12-E1-R2', NULL, NULL, true),
    ('C11-E1-R1-A1-B1', 'Blok 001', 'org3', 'C11-E1-R1-A1', NULL, NULL, true),
    ('C11-E1-R1-A1-B2', 'Blok 002', 'org3', 'C11-E1-R1-A1', NULL, NULL, true),
    ('C11-E1-R1-A2-B1', 'Blok 003', 'org3', 'C11-E1-R1-A2', NULL, NULL, true),
    ('C11-E1-R1-A2-B2', 'Blok 004', 'org3', 'C11-E1-R1-A2', NULL, NULL, true),
    ('C11-E1-R2-A1-B1', 'Blok 005', 'org3', 'C11-E1-R2-A1', NULL, NULL, true),
    ('C11-E1-R2-A1-B2', 'Blok 006', 'org3', 'C11-E1-R2-A1', NULL, NULL, true),
    ('C11-E1-R2-A2-B1', 'Blok 007', 'org3', 'C11-E1-R2-A2', NULL, NULL, true),
    ('C11-E1-R2-A2-B2', 'Blok 008', 'org3', 'C11-E1-R2-A2', NULL, NULL, true),
    ('C11-E2-R1-A1-B1', 'Blok 001', 'org3', 'C11-E2-R1-A1', NULL, NULL, true),
    ('C11-E2-R1-A1-B2', 'Blok 002', 'org3', 'C11-E2-R1-A1', NULL, NULL, true),
    ('C11-E2-R1-A2-B1', 'Blok 003', 'org3', 'C11-E2-R1-A2', NULL, NULL, true),
    ('C11-E2-R1-A2-B2', 'Blok 004', 'org3', 'C11-E2-R1-A2', NULL, NULL, true),
    ('C11-E2-R2-A1-B1', 'Blok 005', 'org3', 'C11-E2-R2-A1', NULL, NULL, true),
    ('C11-E2-R2-A1-B2', 'Blok 006', 'org3', 'C11-E2-R2-A1', NULL, NULL, true),
    ('C11-E2-R2-A2-B1', 'Blok 007', 'org3', 'C11-E2-R2-A2', NULL, NULL, true),
    ('C11-E2-R2-A2-B2', 'Blok 008', 'org3', 'C11-E2-R2-A2', NULL, NULL, true),
    ('C12-E1-R1-A1-B1', 'Blok 001', 'org3', 'C12-E1-R1-A1', NULL, NULL, true),
    ('C12-E1-R1-A1-B2', 'Blok 002', 'org3', 'C12-E1-R1-A1', NULL, NULL, true),
    ('C12-E1-R1-A2-B1', 'Blok 003', 'org3', 'C12-E1-R1-A2', NULL, NULL, true),
    ('C12-E1-R1-A2-B2', 'Blok 004', 'org3', 'C12-E1-R1-A2', NULL, NULL, true),
    ('C12-E1-R2-A1-B1', 'Blok 005', 'org3', 'C12-E1-R2-A1', NULL, NULL, true),
    ('C12-E1-R2-A1-B2', 'Blok 006', 'org3', 'C12-E1-R2-A1', NULL, NULL, true),
    ('C12-E1-R2-A2-B1', 'Blok 007', 'org3', 'C12-E1-R2-A2', NULL, NULL, true),
    ('C12-E1-R2-A2-B2', 'Blok 008', 'org3', 'C12-E1-R2-A2', NULL, NULL, true);
INSERT INTO pm.org_unit (code, name, org_level_id, unit_type_id, location, is_active, created_by)
SELECT s.code, s.name, l.id, t.id, s.location, s.active, '00000000-0000-7000-8000-000000000000' FROM s_unit s
JOIN pm.org_level l ON l.code = s.level_code AND NOT l.is_deleted LEFT JOIN pm.unit_type t ON t.code = s.unit_type_code AND NOT t.is_deleted
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;
UPDATE pm.org_unit u SET parent_id = p.id FROM s_unit s JOIN pm.org_unit p ON lower(p.code) = lower(s.parent_code) AND NOT p.is_deleted
WHERE lower(u.code) = lower(s.code) AND u.parent_id IS NULL AND u.created_by = '00000000-0000-7000-8000-000000000000';

INSERT INTO pm.grade (code, name, rank, created_by) VALUES
    ('S1', 'Staf', 1, '00000000-0000-7000-8000-000000000000'),
    ('S2', 'Asisten', 2, '00000000-0000-7000-8000-000000000000'),
    ('M1', 'Manajer', 3, '00000000-0000-7000-8000-000000000000'),
    ('M2', 'Senior Manajer', 4, '00000000-0000-7000-8000-000000000000'),
    ('GM', 'General Manager', 5, '00000000-0000-7000-8000-000000000000'),
    ('D1', 'Direktur', 6, '00000000-0000-7000-8000-000000000000'),
    ('E1', 'Eksekutif', 7, '00000000-0000-7000-8000-000000000000')
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;

INSERT INTO pm.jabatan (code, name, grade_id, kind, org_level_id, created_by)
SELECT s.code, s.name, g.id, s.kind, l.id, '00000000-0000-7000-8000-000000000000' FROM (VALUES
    ('CEO', 'Chief Executive Officer', 'E1', 'struktural', 'group'),
    ('HFIN', 'Head of Finance Group', 'D1', 'fungsional', 'group'),
    ('HHR', 'Head of Human Capital', 'D1', 'fungsional', 'group'),
    ('RD', 'Regional Director', 'D1', 'struktural', 'region'),
    ('RC', 'Regional Controller', 'M2', 'fungsional', 'region'),
    ('GMW', 'General Manager Wilayah', 'GM', 'struktural', 'wilayah'),
    ('KAW', 'Kepala Administrasi Wilayah', 'M1', 'fungsional', 'wilayah'),
    ('SMP', 'Senior Manager Perusahaan', 'M2', 'struktural', 'company'),
    ('KTUP', 'Kepala Tata Usaha Perusahaan', 'M1', 'fungsional', 'company'),
    ('EM', 'Estate Manager', 'M1', 'struktural', 'unit'),
    ('MM', 'Mill Manager', 'M1', 'struktural', 'unit'),
    ('PM', 'Plasma Manager', 'M1', 'struktural', 'unit'),
    ('KBL', 'Kepala Bulking', 'M1', 'struktural', 'unit'),
    ('KKP', 'Kepala Kantor Perwakilan', 'M1', 'struktural', 'unit'),
    ('ASKEP', 'Asisten Kepala', 'S2', 'struktural', 'unit'),
    ('ASDIV', 'Asisten Divisi', 'S2', 'struktural', 'unit'),
    ('ASPRO', 'Asisten Proses', 'S2', 'struktural', 'unit'),
    ('ASMNT', 'Asisten Maintenance', 'S2', 'struktural', 'unit'),
    ('ASPLS', 'Asisten Plasma', 'S2', 'struktural', 'unit'),
    ('KTU', 'Kepala Tata Usaha', 'S2', 'fungsional', 'unit'),
    ('STG', 'Staf Gudang', 'S1', 'fungsional', 'unit'),
    ('STA', 'Staf Administrasi', 'S1', 'fungsional', 'unit')
) s (code, name, grade_code, kind, level_code)
JOIN pm.grade g ON g.code = s.grade_code AND NOT g.is_deleted JOIN pm.org_level l ON l.code = s.level_code AND NOT l.is_deleted
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;

UPDATE pm.unit_type t SET head_jabatan_id = j.id FROM (VALUES
    ('EST', 'EM'),
    ('PKS', 'MM'),
    ('PLS', 'PM'),
    ('BLK', 'KBL'),
    ('KTR', 'KKP')
) s (type_code, jabatan_code) JOIN pm.jabatan j ON j.code = s.jabatan_code AND NOT j.is_deleted
WHERE t.code = s.type_code AND t.head_jabatan_id IS NULL;

INSERT INTO pm.employment_status (code, name, is_permanent, contract_months, created_by) VALUES
    ('PKWTT', 'Karyawan Tetap', true, 0, '00000000-0000-7000-8000-000000000000'),
    ('PKWT', 'Karyawan Kontrak', false, 12, '00000000-0000-7000-8000-000000000000'),
    ('PROB', 'Masa Percobaan', false, 3, '00000000-0000-7000-8000-000000000000'),
    ('BHL', 'Buruh Harian Lepas', false, 0, '00000000-0000-7000-8000-000000000000'),
    ('MAG', 'Magang', false, 6, '00000000-0000-7000-8000-000000000000')
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;
INSERT INTO pm.education_level (code, name, rank, created_by) VALUES
    ('SD', 'Sekolah Dasar', 1, '00000000-0000-7000-8000-000000000000'),
    ('SMP', 'SMP', 2, '00000000-0000-7000-8000-000000000000'),
    ('SMA', 'SMA / SMK', 3, '00000000-0000-7000-8000-000000000000'),
    ('D3', 'Diploma 3', 4, '00000000-0000-7000-8000-000000000000'),
    ('S1', 'Sarjana', 5, '00000000-0000-7000-8000-000000000000'),
    ('S2', 'Magister', 6, '00000000-0000-7000-8000-000000000000')
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;
INSERT INTO pm.work_shift (code, name, start_time, end_time, is_night, created_by) VALUES
    ('NS', 'Non-shift', '07:00', '16:00', false, '00000000-0000-7000-8000-000000000000'),
    ('S1', 'Shift 1', '07:00', '15:00', false, '00000000-0000-7000-8000-000000000000'),
    ('S2', 'Shift 2', '15:00', '23:00', false, '00000000-0000-7000-8000-000000000000'),
    ('S3', 'Shift 3', '23:00', '07:00', true, '00000000-0000-7000-8000-000000000000')
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;
INSERT INTO pm.bank (code, name, clearing_code, created_by) VALUES
    ('BRI', 'Bank Rakyat Indonesia', '002', '00000000-0000-7000-8000-000000000000'),
    ('MANDIRI', 'Bank Mandiri', '008', '00000000-0000-7000-8000-000000000000'),
    ('BNI', 'Bank Negara Indonesia', '009', '00000000-0000-7000-8000-000000000000'),
    ('BCA', 'Bank Central Asia', '014', '00000000-0000-7000-8000-000000000000'),
    ('BRK', 'Bank Riau Kepri Syariah', '119', '00000000-0000-7000-8000-000000000000')
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;

INSERT INTO pm.employee (code, name, nik, join_date, employment_status_id, education_level_id, work_shift_id, bank_id, is_active, created_by)
SELECT s.code, s.name, s.nik, s.join_date::date, st.id, ed.id, sh.id, bk.id, s.active, '00000000-0000-7000-8000-000000000000' FROM (VALUES
    ('E001', 'Ratna Wijaya', '102007', '2008-01-01', 'PKWTT', 'S1', 'NS', 'BRI', true),
    ('E002', 'Rudi Rahman', '102014', '2009-02-02', 'PKWTT', 'SMA', 'S1', 'MANDIRI', true),
    ('E003', 'Sari Hasibuan', '102021', '2010-03-03', 'PKWT', 'D3', 'NS', 'BNI', true),
    ('E004', 'Fajar Pratama', '102028', '2011-04-04', 'PKWTT', 'S1', 'S2', 'BRK', true),
    ('E005', 'Rina Lubis', '102035', '2012-05-05', 'PROB', 'S2', 'NS', 'BRI', true),
    ('E006', 'Dedi Simanjuntak', '102042', '2013-06-06', 'PKWTT', 'SMA', 'S1', 'MANDIRI', true),
    ('E007', 'Nur Purba', '102049', '2014-07-07', 'PKWTT', 'S1', 'NS', 'BNI', true),
    ('E008', 'Irwan Siregar', '102056', '2015-08-08', 'PKWT', 'SMA', 'S2', 'BRK', false),
    ('E009', 'Dewi Harahap', '102063', '2016-09-09', 'PKWTT', 'D3', 'NS', 'BRI', true),
    ('E010', 'Andi Gunawan', '102070', '2017-10-10', 'PROB', 'S1', 'S1', 'MANDIRI', true),
    ('E011', 'Wulan Hidayat', '102077', '2018-11-11', 'PKWTT', 'S2', 'NS', 'BNI', true),
    ('E012', 'Hadi Nugroho', '102084', '2019-12-12', 'PKWTT', 'SMA', 'S2', 'BRK', true),
    ('E013', 'Siti Kusuma', '102091', '2020-01-13', 'PKWT', 'S1', 'NS', 'BRI', true),
    ('E014', 'Joko Setiawan', '102098', '2021-02-14', 'PKWTT', 'SMA', 'S1', 'MANDIRI', true),
    ('E015', 'Fitri Utami', '102105', '2022-03-15', 'PROB', 'D3', 'NS', 'BNI', true),
    ('E016', 'Taufik Saputra', '102112', '2023-04-16', 'PKWTT', 'S1', 'S2', 'BRK', true),
    ('E017', 'Nadia Ramadhani', '102119', '2024-05-17', 'PKWTT', 'S2', 'NS', 'BRI', true),
    ('E018', 'Yusuf Santoso', '102126', '2008-06-18', 'PKWT', 'SMA', 'S1', 'MANDIRI', true),
    ('E019', 'Lestari Wijaya', '102133', '2009-07-19', 'PKWTT', 'S1', 'NS', 'BNI', true),
    ('E020', 'Eko Rahman', '102140', '2010-08-20', 'PROB', 'SMA', 'S2', 'BRK', true),
    ('E021', 'Mega Hasibuan', '102147', '2011-09-21', 'PKWTT', 'D3', 'NS', 'BRI', true),
    ('E022', 'Hendra Pratama', '102154', '2012-10-22', 'PKWTT', 'S1', 'S1', 'MANDIRI', true),
    ('E024', 'Bambang Simanjuntak', '102168', '2013-11-23', 'PKWT', 'S2', 'NS', 'BNI', true),
    ('E025', 'Putri Purba', '102175', '2014-12-24', 'PKWTT', 'SMA', 'S2', 'BRK', true),
    ('E026', 'Agus Siregar', '102182', '2015-01-25', 'PROB', 'S1', 'NS', 'BRI', true),
    ('E027', 'Indah Harahap', '102189', '2016-02-26', 'PKWTT', 'SMA', 'S1', 'MANDIRI', true),
    ('E028', 'Arif Gunawan', '102196', '2017-03-27', 'PKWTT', 'D3', 'NS', 'BNI', true),
    ('E029', 'Ayu Hidayat', '102203', '2018-04-01', 'PKWT', 'S1', 'S2', 'BRK', true),
    ('E030', 'Budi Nugroho', '102210', '2019-05-02', 'PKWTT', 'S2', 'NS', 'BRI', true),
    ('E031', 'Ratna Kusuma', '102217', '2020-06-03', 'PROB', 'SMA', 'S1', 'MANDIRI', true),
    ('E032', 'Rudi Setiawan', '102224', '2021-07-04', 'PKWTT', 'S1', 'NS', 'BNI', false),
    ('E033', 'Sari Utami', '102231', '2022-08-05', 'PKWTT', 'SMA', 'S2', 'BRK', true),
    ('E034', 'Fajar Saputra', '102238', '2023-09-06', 'PKWT', 'D3', 'NS', 'BRI', true),
    ('E035', 'Rina Ramadhani', '102245', '2024-10-07', 'PKWTT', 'S1', 'S1', 'MANDIRI', true),
    ('E036', 'Dedi Santoso', '102252', '2008-11-08', 'PROB', 'S2', 'NS', 'BNI', true),
    ('E037', 'Nur Wijaya', '102259', '2009-12-09', 'PKWTT', 'SMA', 'S2', 'BRK', true),
    ('E038', 'Irwan Rahman', '102266', '2010-01-10', 'PKWTT', 'S1', 'NS', 'BRI', true),
    ('E039', 'Dewi Hasibuan', '102273', '2011-02-11', 'PKWT', 'SMA', 'S1', 'MANDIRI', true),
    ('E040', 'Andi Pratama', '102280', '2012-03-12', 'PKWTT', 'D3', 'NS', 'BNI', true),
    ('E041', 'Wulan Lubis', '102287', '2013-04-13', 'PROB', 'S1', 'S2', 'BRK', true),
    ('E042', 'Hadi Simanjuntak', '102294', '2014-05-14', 'PKWTT', 'S2', 'NS', 'BRI', true),
    ('E043', 'Siti Purba', '102301', '2015-06-15', 'PKWTT', 'SMA', 'S1', 'MANDIRI', true),
    ('E044', 'Joko Siregar', '102308', '2016-07-16', 'PKWT', 'S1', 'NS', 'BNI', true),
    ('E045', 'Fitri Harahap', '102315', '2017-08-17', 'PKWTT', 'SMA', 'S2', 'BRK', true),
    ('E046', 'Taufik Gunawan', '102322', '2018-09-18', 'PROB', 'D3', 'NS', 'BRI', true),
    ('E047', 'Nadia Hidayat', '102329', '2019-10-19', 'PKWTT', 'S1', 'S1', 'MANDIRI', true),
    ('E048', 'Yusuf Nugroho', '102336', '2020-11-20', 'PKWTT', 'S2', 'NS', 'BNI', true),
    ('E049', 'Lestari Kusuma', '102343', '2021-12-21', 'PKWT', 'SMA', 'S2', 'BRK', true),
    ('E050', 'Eko Setiawan', '102350', '2022-01-22', 'PKWTT', 'S1', 'NS', 'BRI', true),
    ('E051', 'Mega Utami', '102357', '2023-02-23', 'PROB', 'SMA', 'S1', 'MANDIRI', true),
    ('E052', 'Hendra Saputra', '102364', '2024-03-24', 'PKWTT', 'D3', 'NS', 'BNI', true),
    ('E053', 'Maya Ramadhani', '102371', '2008-04-25', 'PKWTT', 'S1', 'S2', 'BRK', true),
    ('E054', 'Bambang Santoso', '102378', '2009-05-26', 'PKWT', 'S2', 'NS', 'BRI', true),
    ('E055', 'Putri Wijaya', '102385', '2010-06-27', 'PKWTT', 'SMA', 'S1', 'MANDIRI', false),
    ('E056', 'Agus Rahman', '102392', '2011-07-01', 'PROB', 'S1', 'NS', 'BNI', true),
    ('E057', 'Indah Hasibuan', '102399', '2012-08-02', 'PKWTT', 'SMA', 'S2', 'BRK', true),
    ('E058', 'Arif Pratama', '102406', '2013-09-03', 'PKWTT', 'D3', 'NS', 'BRI', true),
    ('E059', 'Ayu Lubis', '102413', '2014-10-04', 'PKWT', 'S1', 'S1', 'MANDIRI', true),
    ('E060', 'Budi Simanjuntak', '102420', '2015-11-05', 'PKWTT', 'S2', 'NS', 'BNI', true),
    ('E061', 'Ratna Purba', '102427', '2016-12-06', 'PROB', 'SMA', 'S2', 'BRK', true),
    ('E062', 'Rudi Siregar', '102434', '2017-01-07', 'PKWTT', 'S1', 'NS', 'BRI', true),
    ('E063', 'Sari Harahap', '102441', '2018-02-08', 'PKWTT', 'SMA', 'S1', 'MANDIRI', true),
    ('E064', 'Fajar Gunawan', '102448', '2019-03-09', 'PKWT', 'D3', 'NS', 'BNI', true),
    ('E065', 'Rina Hidayat', '102455', '2020-04-10', 'PKWTT', 'S1', 'S2', 'BRK', true),
    ('E066', 'Dedi Nugroho', '102462', '2021-05-11', 'PROB', 'S2', 'NS', 'BRI', true),
    ('E067', 'Nur Kusuma', '102469', '2022-06-12', 'PKWTT', 'SMA', 'S1', 'MANDIRI', true),
    ('E068', 'Irwan Setiawan', '102476', '2023-07-13', 'PKWTT', 'S1', 'NS', 'BNI', true),
    ('E069', 'Dewi Utami', '102483', '2024-08-14', 'PKWT', 'SMA', 'S2', 'BRK', true),
    ('E070', 'Andi Saputra', '102490', '2008-09-15', 'PKWTT', 'D3', 'NS', 'BRI', true),
    ('E071', 'Wulan Ramadhani', '102497', '2009-10-16', 'PROB', 'S1', 'S1', 'MANDIRI', true),
    ('E072', 'Hadi Santoso', '102504', '2010-11-17', 'PKWTT', 'S2', 'NS', 'BNI', true),
    ('E073', 'Siti Wijaya', '102511', '2011-12-18', 'PKWTT', 'SMA', 'S2', 'BRK', true),
    ('E075', 'Fitri Hasibuan', '102525', '2012-01-19', 'PKWT', 'S1', 'NS', 'BRI', true),
    ('E076', 'Taufik Pratama', '102532', '2013-02-20', 'PKWTT', 'SMA', 'S1', 'MANDIRI', true),
    ('E077', 'Nadia Lubis', '102539', '2014-03-21', 'PROB', 'D3', 'NS', 'BNI', true),
    ('E078', 'Yusuf Simanjuntak', '102546', '2015-04-22', 'PKWTT', 'S1', 'S2', 'BRK', true),
    ('E079', 'Lestari Purba', '102553', '2016-05-23', 'PKWTT', 'S2', 'NS', 'BRI', false),
    ('E080', 'Eko Siregar', '102560', '2017-06-24', 'PKWT', 'SMA', 'S1', 'MANDIRI', true),
    ('E081', 'Mega Harahap', '102567', '2018-07-25', 'PKWTT', 'S1', 'NS', 'BNI', true),
    ('E082', 'Hendra Gunawan', '102574', '2019-08-26', 'PROB', 'SMA', 'S2', 'BRK', true),
    ('E083', 'Maya Hidayat', '102581', '2020-09-27', 'PKWTT', 'D3', 'NS', 'BRI', true),
    ('E084', 'Bambang Nugroho', '102588', '2021-10-01', 'PKWTT', 'S1', 'S1', 'MANDIRI', true),
    ('E085', 'Putri Kusuma', '102595', '2022-11-02', 'PKWT', 'S2', 'NS', 'BNI', true)
) s (code, name, nik, join_date, status, education, shift, bank, active)
JOIN pm.employment_status st ON st.code = s.status LEFT JOIN pm.education_level ed ON ed.code = s.education
LEFT JOIN pm.work_shift sh ON sh.code = s.shift LEFT JOIN pm.bank bk ON bk.code = s.bank
ON CONFLICT DO NOTHING; -- an employee whose code or NIK is already there is left as it is

UPDATE pm.employee e SET email = s.email FROM (VALUES
    ('E001', 'ratna.wijaya@agronusantara.co.id'),
    ('E002', 'rudi.rahman@agronusantara.co.id'),
    ('E003', 'sari.hasibuan@agronusantara.co.id'),
    ('E004', 'fajar.pratama@agronusantara.co.id'),
    ('E005', 'rina.lubis@agronusantara.co.id'),
    ('E006', 'dedi.simanjuntak@agronusantara.co.id'),
    ('E007', 'nur.purba@agronusantara.co.id'),
    ('E008', 'irwan.siregar@agronusantara.co.id'),
    ('E009', 'dewi.harahap@agronusantara.co.id'),
    ('E010', 'andi.gunawan@agronusantara.co.id'),
    ('E011', 'wulan.hidayat@agronusantara.co.id'),
    ('E012', 'hadi.nugroho@agronusantara.co.id'),
    ('E013', 'siti.kusuma@agronusantara.co.id'),
    ('E014', 'joko.setiawan@agronusantara.co.id'),
    ('E015', 'fitri.utami@agronusantara.co.id'),
    ('E016', 'taufik.saputra@agronusantara.co.id'),
    ('E017', 'nadia.ramadhani@agronusantara.co.id'),
    ('E018', 'yusuf.santoso@agronusantara.co.id'),
    ('E019', 'lestari.wijaya@agronusantara.co.id'),
    ('E020', 'eko.rahman@agronusantara.co.id'),
    ('E021', 'mega.hasibuan@agronusantara.co.id'),
    ('E022', 'hendra.pratama@agronusantara.co.id'),
    ('E024', 'bambang.simanjuntak@agronusantara.co.id'),
    ('E025', 'putri.purba@agronusantara.co.id'),
    ('E026', 'agus.siregar@agronusantara.co.id'),
    ('E027', 'indah.harahap@agronusantara.co.id'),
    ('E028', 'arif.gunawan@agronusantara.co.id'),
    ('E029', 'ayu.hidayat@agronusantara.co.id'),
    ('E030', 'budi.nugroho@agronusantara.co.id'),
    ('E031', 'ratna.kusuma@agronusantara.co.id'),
    ('E032', 'rudi.setiawan@agronusantara.co.id'),
    ('E033', 'sari.utami@agronusantara.co.id'),
    ('E034', 'fajar.saputra@agronusantara.co.id'),
    ('E035', 'rina.ramadhani@agronusantara.co.id'),
    ('E036', 'dedi.santoso@agronusantara.co.id'),
    ('E037', 'nur.wijaya@agronusantara.co.id'),
    ('E038', 'irwan.rahman@agronusantara.co.id'),
    ('E039', 'dewi.hasibuan@agronusantara.co.id'),
    ('E040', 'andi.pratama@agronusantara.co.id'),
    ('E041', 'wulan.lubis@agronusantara.co.id'),
    ('E042', 'hadi.simanjuntak@agronusantara.co.id'),
    ('E043', 'siti.purba@agronusantara.co.id'),
    ('E044', 'joko.siregar@agronusantara.co.id'),
    ('E045', 'fitri.harahap@agronusantara.co.id'),
    ('E046', 'taufik.gunawan@agronusantara.co.id'),
    ('E047', 'nadia.hidayat@agronusantara.co.id'),
    ('E048', 'yusuf.nugroho@agronusantara.co.id'),
    ('E049', 'lestari.kusuma@agronusantara.co.id'),
    ('E050', 'eko.setiawan@agronusantara.co.id'),
    ('E051', 'mega.utami@agronusantara.co.id'),
    ('E052', 'hendra.saputra@agronusantara.co.id'),
    ('E053', 'maya.ramadhani@agronusantara.co.id'),
    ('E054', 'bambang.santoso@agronusantara.co.id'),
    ('E055', 'putri.wijaya@agronusantara.co.id'),
    ('E056', 'agus.rahman@agronusantara.co.id'),
    ('E057', 'indah.hasibuan@agronusantara.co.id'),
    ('E058', 'arif.pratama@agronusantara.co.id'),
    ('E059', 'ayu.lubis@agronusantara.co.id'),
    ('E060', 'budi.simanjuntak@agronusantara.co.id'),
    ('E061', 'ratna.purba@agronusantara.co.id'),
    ('E062', 'rudi.siregar@agronusantara.co.id'),
    ('E063', 'sari.harahap@agronusantara.co.id'),
    ('E064', 'fajar.gunawan@agronusantara.co.id'),
    ('E065', 'rina.hidayat@agronusantara.co.id'),
    ('E066', 'dedi.nugroho@agronusantara.co.id'),
    ('E067', 'nur.kusuma@agronusantara.co.id'),
    ('E068', 'irwan.setiawan@agronusantara.co.id'),
    ('E069', 'dewi.utami@agronusantara.co.id'),
    ('E070', 'andi.saputra@agronusantara.co.id'),
    ('E071', 'wulan.ramadhani@agronusantara.co.id'),
    ('E072', 'hadi.santoso@agronusantara.co.id'),
    ('E073', 'siti.wijaya@agronusantara.co.id'),
    ('E075', 'fitri.hasibuan@agronusantara.co.id'),
    ('E076', 'taufik.pratama@agronusantara.co.id'),
    ('E077', 'nadia.lubis@agronusantara.co.id'),
    ('E078', 'yusuf.simanjuntak@agronusantara.co.id'),
    ('E079', 'lestari.purba@agronusantara.co.id'),
    ('E080', 'eko.siregar@agronusantara.co.id'),
    ('E081', 'mega.harahap@agronusantara.co.id'),
    ('E082', 'hendra.gunawan@agronusantara.co.id'),
    ('E083', 'maya.hidayat@agronusantara.co.id'),
    ('E084', 'bambang.nugroho@agronusantara.co.id'),
    ('E085', 'putri.kusuma@agronusantara.co.id')
) s (code, email)
WHERE e.code = s.code AND e.email IS NULL AND e.created_by = '00000000-0000-7000-8000-000000000000'
  AND NOT EXISTS (SELECT 1 FROM pm.employee x WHERE lower(x.email) = lower(s.email) AND NOT x.is_deleted);

CREATE TEMP TABLE s_position (code text, name text, unit_code text, jabatan_code text, reports_to_code text, is_head boolean) ON COMMIT DROP;
INSERT INTO s_position VALUES
    ('GAN-CEO', 'Chief Executive Officer · Grup Agro Nusantara', 'GAN', 'CEO', NULL, true),
    ('GAN-HFIN', 'Head of Finance Group · Grup Agro Nusantara', 'GAN', 'HFIN', 'GAN-CEO', false),
    ('GAN-HHR', 'Head of Human Capital · Grup Agro Nusantara', 'GAN', 'HHR', 'GAN-CEO', false),
    ('KAL-RD', 'Regional Director · Region Kalimantan', 'KAL', 'RD', 'GAN-CEO', true),
    ('KAL-RC', 'Regional Controller · Region Kalimantan', 'KAL', 'RC', 'KAL-RD', false),
    ('KALBAR-GMW', 'General Manager Wilayah · Wilayah Kalimantan Barat', 'KALBAR', 'GMW', 'KAL-RD', true),
    ('KALBAR-KAW', 'Kepala Administrasi Wilayah · Wilayah Kalimantan Barat', 'KALBAR', 'KAW', 'KALBAR-GMW', false),
    ('C11-SMP', 'Senior Manager Perusahaan · PT Sawit Khatulistiwa', 'C11', 'SMP', 'KALBAR-GMW', true),
    ('C11-KTUP', 'Kepala Tata Usaha Perusahaan · PT Sawit Khatulistiwa', 'C11', 'KTUP', 'C11-SMP', false),
    ('C11-E1-EM', 'Estate Manager · Kebun Sungai Ayak', 'C11-E1', 'EM', 'C11-SMP', true),
    ('C11-E1-ASKEP', 'Asisten Kepala · Kebun Sungai Ayak', 'C11-E1', 'ASKEP', 'C11-E1-EM', false),
    ('C11-E1-ASDIV-1', 'Asisten Divisi · Kebun Sungai Ayak', 'C11-E1', 'ASDIV', 'C11-E1-ASKEP', false),
    ('C11-E1-ASDIV-2', 'Asisten Divisi · Kebun Sungai Ayak', 'C11-E1', 'ASDIV', 'C11-E1-ASKEP', false),
    ('C11-E1-KTU', 'Kepala Tata Usaha · Kebun Sungai Ayak', 'C11-E1', 'KTU', 'C11-E1-EM', false),
    ('C11-E2-EM', 'Estate Manager · Kebun Nanga Tayap', 'C11-E2', 'EM', 'C11-SMP', true),
    ('C11-E2-ASKEP', 'Asisten Kepala · Kebun Nanga Tayap', 'C11-E2', 'ASKEP', 'C11-E2-EM', false),
    ('C11-E2-ASDIV-1', 'Asisten Divisi · Kebun Nanga Tayap', 'C11-E2', 'ASDIV', 'C11-E2-ASKEP', false),
    ('C11-E2-ASDIV-2', 'Asisten Divisi · Kebun Nanga Tayap', 'C11-E2', 'ASDIV', 'C11-E2-ASKEP', false),
    ('C11-E2-KTU', 'Kepala Tata Usaha · Kebun Nanga Tayap', 'C11-E2', 'KTU', 'C11-E2-EM', false),
    ('C11-M1-MM', 'Mill Manager · PKS Sungai Ayak', 'C11-M1', 'MM', 'C11-SMP', true),
    ('C11-M1-ASPRO', 'Asisten Proses · PKS Sungai Ayak', 'C11-M1', 'ASPRO', 'C11-M1-MM', false),
    ('C11-M1-ASMNT', 'Asisten Maintenance · PKS Sungai Ayak', 'C11-M1', 'ASMNT', 'C11-M1-MM', false),
    ('C11-M1-KTU', 'Kepala Tata Usaha · PKS Sungai Ayak', 'C11-M1', 'KTU', 'C11-M1-MM', false),
    ('C12-SMP', 'Senior Manager Perusahaan · PT Kapuas Agro Lestari', 'C12', 'SMP', 'KALBAR-GMW', true),
    ('C12-KTUP', 'Kepala Tata Usaha Perusahaan · PT Kapuas Agro Lestari', 'C12', 'KTUP', 'C12-SMP', false),
    ('C12-E1-EM', 'Estate Manager · Kebun Kapuas Hulu', 'C12-E1', 'EM', 'C12-SMP', true),
    ('C12-E1-ASKEP', 'Asisten Kepala · Kebun Kapuas Hulu', 'C12-E1', 'ASKEP', 'C12-E1-EM', false),
    ('C12-E1-ASDIV-1', 'Asisten Divisi · Kebun Kapuas Hulu', 'C12-E1', 'ASDIV', 'C12-E1-ASKEP', false),
    ('C12-E1-ASDIV-2', 'Asisten Divisi · Kebun Kapuas Hulu', 'C12-E1', 'ASDIV', 'C12-E1-ASKEP', false),
    ('C12-E1-KTU', 'Kepala Tata Usaha · Kebun Kapuas Hulu', 'C12-E1', 'KTU', 'C12-E1-EM', false),
    ('C12-P1-PM', 'Plasma Manager · Kebun Plasma Kapuas', 'C12-P1', 'PM', 'C12-SMP', true),
    ('C12-P1-ASPLS-1', 'Asisten Plasma · Kebun Plasma Kapuas', 'C12-P1', 'ASPLS', 'C12-P1-PM', false),
    ('C12-P1-ASPLS-2', 'Asisten Plasma · Kebun Plasma Kapuas', 'C12-P1', 'ASPLS', 'C12-P1-PM', false),
    ('C12-P1-KTU', 'Kepala Tata Usaha · Kebun Plasma Kapuas', 'C12-P1', 'KTU', 'C12-P1-PM', false),
    ('KALTENG-GMW', 'General Manager Wilayah · Wilayah Kalimantan Tengah', 'KALTENG', 'GMW', 'KAL-RD', true),
    ('KALTENG-KAW', 'Kepala Administrasi Wilayah · Wilayah Kalimantan Tengah', 'KALTENG', 'KAW', 'KALTENG-GMW', false),
    ('C21-SMP', 'Senior Manager Perusahaan · PT Katingan Hijau', 'C21', 'SMP', 'KALTENG-GMW', true),
    ('C21-KTUP', 'Kepala Tata Usaha Perusahaan · PT Katingan Hijau', 'C21', 'KTUP', 'C21-SMP', false),
    ('C21-E1-EM', 'Estate Manager · Kebun Katingan', 'C21-E1', 'EM', 'C21-SMP', true),
    ('C21-E1-ASKEP', 'Asisten Kepala · Kebun Katingan', 'C21-E1', 'ASKEP', 'C21-E1-EM', false),
    ('C21-E1-ASDIV-1', 'Asisten Divisi · Kebun Katingan', 'C21-E1', 'ASDIV', 'C21-E1-ASKEP', false),
    ('C21-E1-ASDIV-2', 'Asisten Divisi · Kebun Katingan', 'C21-E1', 'ASDIV', 'C21-E1-ASKEP', false),
    ('C21-E1-KTU', 'Kepala Tata Usaha · Kebun Katingan', 'C21-E1', 'KTU', 'C21-E1-EM', false),
    ('C21-M1-MM', 'Mill Manager · PKS Katingan', 'C21-M1', 'MM', 'C21-SMP', true),
    ('C21-M1-ASPRO', 'Asisten Proses · PKS Katingan', 'C21-M1', 'ASPRO', 'C21-M1-MM', false),
    ('C21-M1-ASMNT', 'Asisten Maintenance · PKS Katingan', 'C21-M1', 'ASMNT', 'C21-M1-MM', false),
    ('C21-M1-KTU', 'Kepala Tata Usaha · PKS Katingan', 'C21-M1', 'KTU', 'C21-M1-MM', false),
    ('C21-B1-KBL', 'Kepala Bulking · Bulking Sampit', 'C21-B1', 'KBL', 'C21-SMP', true),
    ('C21-B1-STG-1', 'Staf Gudang · Bulking Sampit', 'C21-B1', 'STG', 'C21-B1-KBL', false),
    ('C21-B1-STG-2', 'Staf Gudang · Bulking Sampit', 'C21-B1', 'STG', 'C21-B1-KBL', false),
    ('C22-SMP', 'Senior Manager Perusahaan · PT Mentaya Agro Makmur', 'C22', 'SMP', 'KALTENG-GMW', true),
    ('C22-KTUP', 'Kepala Tata Usaha Perusahaan · PT Mentaya Agro Makmur', 'C22', 'KTUP', 'C22-SMP', false),
    ('C22-E1-EM', 'Estate Manager · Kebun Mentaya', 'C22-E1', 'EM', 'C22-SMP', true),
    ('C22-E1-ASKEP', 'Asisten Kepala · Kebun Mentaya', 'C22-E1', 'ASKEP', 'C22-E1-EM', false),
    ('C22-E1-ASDIV-1', 'Asisten Divisi · Kebun Mentaya', 'C22-E1', 'ASDIV', 'C22-E1-ASKEP', false),
    ('C22-E1-ASDIV-2', 'Asisten Divisi · Kebun Mentaya', 'C22-E1', 'ASDIV', 'C22-E1-ASKEP', false),
    ('C22-E1-KTU', 'Kepala Tata Usaha · Kebun Mentaya', 'C22-E1', 'KTU', 'C22-E1-EM', false),
    ('C22-P1-PM', 'Plasma Manager · Kebun Plasma Mentaya', 'C22-P1', 'PM', 'C22-SMP', true),
    ('C22-P1-ASPLS-1', 'Asisten Plasma · Kebun Plasma Mentaya', 'C22-P1', 'ASPLS', 'C22-P1-PM', false),
    ('C22-P1-ASPLS-2', 'Asisten Plasma · Kebun Plasma Mentaya', 'C22-P1', 'ASPLS', 'C22-P1-PM', false),
    ('C22-P1-KTU', 'Kepala Tata Usaha · Kebun Plasma Mentaya', 'C22-P1', 'KTU', 'C22-P1-PM', false),
    ('SUM-RD', 'Regional Director · Region Sumatera', 'SUM', 'RD', 'GAN-CEO', true),
    ('SUM-RC', 'Regional Controller · Region Sumatera', 'SUM', 'RC', 'SUM-RD', false),
    ('RIAU-GMW', 'General Manager Wilayah · Wilayah Riau', 'RIAU', 'GMW', 'SUM-RD', true),
    ('RIAU-KAW', 'Kepala Administrasi Wilayah · Wilayah Riau', 'RIAU', 'KAW', 'RIAU-GMW', false),
    ('C31-SMP', 'Senior Manager Perusahaan · PT Riau Palma Lestari', 'C31', 'SMP', 'RIAU-GMW', true),
    ('C31-KTUP', 'Kepala Tata Usaha Perusahaan · PT Riau Palma Lestari', 'C31', 'KTUP', 'C31-SMP', false),
    ('C31-E1-EM', 'Estate Manager · Kebun Kampar', 'C31-E1', 'EM', 'C31-SMP', true),
    ('C31-E1-ASKEP', 'Asisten Kepala · Kebun Kampar', 'C31-E1', 'ASKEP', 'C31-E1-EM', false),
    ('C31-E1-ASDIV-1', 'Asisten Divisi · Kebun Kampar', 'C31-E1', 'ASDIV', 'C31-E1-ASKEP', false),
    ('C31-E1-ASDIV-2', 'Asisten Divisi · Kebun Kampar', 'C31-E1', 'ASDIV', 'C31-E1-ASKEP', false),
    ('C31-E1-KTU', 'Kepala Tata Usaha · Kebun Kampar', 'C31-E1', 'KTU', 'C31-E1-EM', false),
    ('C31-M1-MM', 'Mill Manager · PKS Kampar', 'C31-M1', 'MM', 'C31-SMP', true),
    ('C31-M1-ASPRO', 'Asisten Proses · PKS Kampar', 'C31-M1', 'ASPRO', 'C31-M1-MM', false),
    ('C31-M1-ASMNT', 'Asisten Maintenance · PKS Kampar', 'C31-M1', 'ASMNT', 'C31-M1-MM', false),
    ('C31-M1-KTU', 'Kepala Tata Usaha · PKS Kampar', 'C31-M1', 'KTU', 'C31-M1-MM', false),
    ('C31-K1-KKP', 'Kepala Kantor Perwakilan · Kantor Pekanbaru', 'C31-K1', 'KKP', 'C31-SMP', true),
    ('C31-K1-STA', 'Staf Administrasi · Kantor Pekanbaru', 'C31-K1', 'STA', 'C31-K1-KKP', false),
    ('C32-SMP', 'Senior Manager Perusahaan · PT Siak Sawit Sejahtera', 'C32', 'SMP', 'RIAU-GMW', true),
    ('C32-KTUP', 'Kepala Tata Usaha Perusahaan · PT Siak Sawit Sejahtera', 'C32', 'KTUP', 'C32-SMP', false),
    ('C32-E1-EM', 'Estate Manager · Kebun Siak', 'C32-E1', 'EM', 'C32-SMP', true),
    ('C32-E1-ASKEP', 'Asisten Kepala · Kebun Siak', 'C32-E1', 'ASKEP', 'C32-E1-EM', false),
    ('C32-E1-ASDIV-1', 'Asisten Divisi · Kebun Siak', 'C32-E1', 'ASDIV', 'C32-E1-ASKEP', false),
    ('C32-E1-ASDIV-2', 'Asisten Divisi · Kebun Siak', 'C32-E1', 'ASDIV', 'C32-E1-ASKEP', false),
    ('C32-E1-KTU', 'Kepala Tata Usaha · Kebun Siak', 'C32-E1', 'KTU', 'C32-E1-EM', false),
    ('C32-P1-PM', 'Plasma Manager · Kebun Plasma Siak', 'C32-P1', 'PM', 'C32-SMP', true),
    ('C32-P1-ASPLS-1', 'Asisten Plasma · Kebun Plasma Siak', 'C32-P1', 'ASPLS', 'C32-P1-PM', false),
    ('C32-P1-ASPLS-2', 'Asisten Plasma · Kebun Plasma Siak', 'C32-P1', 'ASPLS', 'C32-P1-PM', false),
    ('C32-P1-KTU', 'Kepala Tata Usaha · Kebun Plasma Siak', 'C32-P1', 'KTU', 'C32-P1-PM', false);
INSERT INTO pm.position (code, name, org_unit_id, jabatan_id, is_head, created_by)
SELECT s.code, s.name, u.id, j.id, s.is_head, '00000000-0000-7000-8000-000000000000' FROM s_position s
JOIN pm.org_unit u ON u.code = s.unit_code AND NOT u.is_deleted JOIN pm.jabatan j ON j.code = s.jabatan_code AND NOT j.is_deleted
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;
UPDATE pm.position x SET reports_to_id = b.id FROM s_position s JOIN pm.position b ON b.code = s.reports_to_code AND NOT b.is_deleted
WHERE x.code = s.code AND x.reports_to_id IS NULL AND x.created_by = '00000000-0000-7000-8000-000000000000';

UPDATE pm.position x SET holder_employee_id = h.id, acting_employee_id = a.id FROM (VALUES
    ('GAN-CEO', 'E001', NULL),
    ('GAN-HFIN', 'E002', NULL),
    ('GAN-HHR', 'E003', NULL),
    ('KAL-RD', 'E004', NULL),
    ('KAL-RC', 'E005', NULL),
    ('KALBAR-GMW', 'E006', NULL),
    ('KALBAR-KAW', 'E007', NULL),
    ('C11-SMP', 'E008', NULL),
    ('C11-KTUP', 'E009', NULL),
    ('C11-E1-EM', 'E010', NULL),
    ('C11-E1-ASKEP', 'E011', NULL),
    ('C11-E1-ASDIV-1', 'E012', NULL),
    ('C11-E1-ASDIV-2', 'E013', NULL),
    ('C11-E1-KTU', 'E014', NULL),
    ('C11-E2-EM', 'E015', NULL),
    ('C11-E2-ASKEP', 'E016', NULL),
    ('C11-E2-ASDIV-1', 'E017', NULL),
    ('C11-E2-KTU', 'E018', NULL),
    ('C11-M1-MM', 'E019', NULL),
    ('C11-M1-ASPRO', 'E020', NULL),
    ('C11-M1-ASMNT', 'E021', NULL),
    ('C11-M1-KTU', 'E022', NULL),
    ('C12-SMP', 'E008', NULL),
    ('C12-KTUP', 'E024', NULL),
    ('C12-E1-EM', 'E025', NULL),
    ('C12-E1-ASKEP', 'E026', NULL),
    ('C12-E1-ASDIV-1', 'E027', NULL),
    ('C12-E1-ASDIV-2', 'E028', NULL),
    ('C12-E1-KTU', 'E029', NULL),
    ('C12-P1-PM', 'E030', NULL),
    ('C12-P1-ASPLS-1', 'E031', NULL),
    ('C12-P1-ASPLS-2', 'E032', NULL),
    ('C12-P1-KTU', 'E033', NULL),
    ('KALTENG-GMW', NULL, 'E035'),
    ('KALTENG-KAW', 'E034', NULL),
    ('C21-SMP', 'E035', NULL),
    ('C21-KTUP', 'E036', NULL),
    ('C21-E1-EM', 'E037', NULL),
    ('C21-E1-ASKEP', 'E038', NULL),
    ('C21-E1-ASDIV-1', 'E039', NULL),
    ('C21-E1-ASDIV-2', 'E040', NULL),
    ('C21-E1-KTU', 'E041', NULL),
    ('C21-M1-MM', 'E042', NULL),
    ('C21-M1-ASPRO', 'E043', NULL),
    ('C21-M1-ASMNT', 'E044', NULL),
    ('C21-M1-KTU', 'E045', NULL),
    ('C21-B1-KBL', 'E046', NULL),
    ('C21-B1-STG-1', 'E047', NULL),
    ('C21-B1-STG-2', 'E048', NULL),
    ('C22-SMP', 'E049', NULL),
    ('C22-KTUP', 'E050', NULL),
    ('C22-E1-ASKEP', 'E051', NULL),
    ('C22-E1-ASDIV-1', 'E052', NULL),
    ('C22-E1-ASDIV-2', 'E053', NULL),
    ('C22-E1-KTU', 'E054', NULL),
    ('C22-P1-PM', 'E055', NULL),
    ('C22-P1-ASPLS-1', 'E056', NULL),
    ('C22-P1-ASPLS-2', 'E057', NULL),
    ('C22-P1-KTU', 'E058', NULL),
    ('SUM-RD', 'E059', NULL),
    ('SUM-RC', 'E060', NULL),
    ('RIAU-GMW', 'E061', NULL),
    ('RIAU-KAW', 'E062', NULL),
    ('C31-SMP', 'E063', NULL),
    ('C31-KTUP', 'E064', NULL),
    ('C31-E1-EM', 'E065', NULL),
    ('C31-E1-ASKEP', 'E066', NULL),
    ('C31-E1-ASDIV-1', 'E067', NULL),
    ('C31-E1-ASDIV-2', 'E068', NULL),
    ('C31-E1-KTU', 'E069', NULL),
    ('C31-M1-MM', 'E070', NULL),
    ('C31-M1-ASPRO', 'E071', NULL),
    ('C31-M1-ASMNT', 'E072', NULL),
    ('C31-M1-KTU', 'E073', NULL),
    ('C31-K1-KKP', 'E063', NULL),
    ('C31-K1-STA', 'E075', NULL),
    ('C32-SMP', 'E076', NULL),
    ('C32-KTUP', 'E077', NULL),
    ('C32-E1-EM', 'E078', NULL),
    ('C32-E1-ASKEP', 'E079', NULL),
    ('C32-E1-ASDIV-1', 'E080', NULL),
    ('C32-E1-ASDIV-2', 'E081', NULL),
    ('C32-E1-KTU', 'E082', NULL),
    ('C32-P1-PM', 'E083', NULL),
    ('C32-P1-ASPLS-1', 'E084', NULL),
    ('C32-P1-KTU', 'E085', NULL)
) s (code, holder, acting) LEFT JOIN pm.employee h ON h.code = s.holder LEFT JOIN pm.employee a ON a.code = s.acting
WHERE x.code = s.code AND x.created_by = '00000000-0000-7000-8000-000000000000' AND x.holder_employee_id IS NULL AND x.acting_employee_id IS NULL;

INSERT INTO pm.approval_rule (code, name, sop, module, reach_kind, reach_steps, reach_level_id, min_steps, is_active, created_by)
SELECT s.code, s.name, s.sop, s.module, s.kind, s.steps, l.id, s.min_steps, s.active, '00000000-0000-7000-8000-000000000000' FROM (VALUES
    ('CUTI', 'Cuti Karyawan', 'SOP-HC-004', 'employee-self-service', 'steps', 1, NULL, 1, true),
    ('DINAS', 'Perjalanan Dinas', 'SOP-HC-011', 'business-travel', 'steps', 2, NULL, 1, true),
    ('REIMB', 'Reimbursement', 'SOP-FIN-007', 'reimbursement', 'level', NULL, 'unit', 1, true),
    ('PR', 'Permintaan Pembelian', 'SOP-PRC-002', 'procurement', 'amount', NULL, NULL, 1, true),
    ('BUDREV', 'Revisi Anggaran', 'SOP-FIN-015', 'budget-office', 'level', NULL, 'wilayah', 1, true),
    ('CAPEX', 'Pengajuan Investasi (Capex)', 'SOP-FIN-020', 'budget-office', 'level', NULL, 'group', 1, true)
) s (code, name, sop, module, kind, steps, level_code, min_steps, active)
LEFT JOIN pm.org_level l ON l.code = s.level_code AND NOT l.is_deleted
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;

INSERT INTO pm.approval_rule_band (approval_rule_id, seq, max_amount, org_level_id)
SELECT r.id, s.seq, s.max_amount, l.id FROM (VALUES
    ('PR', 1, 25000000, 'unit'),
    ('PR', 2, 250000000, 'company'),
    ('PR', 3, 1000000000, 'wilayah'),
    ('PR', 4, 5000000000, 'region'),
    ('PR', 5, NULL, 'group')
) s (rule_code, seq, max_amount, level_code) JOIN pm.approval_rule r ON r.code = s.rule_code JOIN pm.org_level l ON l.code = s.level_code
WHERE r.created_by = '00000000-0000-7000-8000-000000000000'
ON CONFLICT DO NOTHING;

INSERT INTO pm.delegation (code, name, valid_from, valid_to, position_id, is_active, created_by)
SELECT s.code, s.name, s.valid_from::date, s.valid_to::date, x.id, s.active, '00000000-0000-7000-8000-000000000000' FROM (VALUES
    ('DLG-001', 'Cuti tahunan', '2026-11-17', '2026-11-25', 'KALBAR-GMW', true),
    ('DLG-002', 'Dinas luar kota', '2026-11-10', '2026-11-20', 'C11-E1-EM', true),
    ('DLG-003', 'Pelatihan', '2026-12-01', '2026-12-05', 'RIAU-GMW', true)
) s (code, name, valid_from, valid_to, position_code, active) JOIN pm.position x ON x.code = s.position_code AND NOT x.is_deleted
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;
UPDATE pm.delegation d SET delegate_employee_id = e.id FROM (VALUES
    ('DLG-001', 'E005'),
    ('DLG-002', 'E011'),
    ('DLG-003', 'E062')
) s (code, delegate) JOIN pm.employee e ON e.code = s.delegate
WHERE d.code = s.code AND d.delegate_employee_id IS NULL AND d.created_by = '00000000-0000-7000-8000-000000000000';

INSERT INTO pm.delegation_rule (delegation_id, approval_rule_id)
SELECT d.id, r.id FROM (VALUES
    ('DLG-002', 'CUTI'),
    ('DLG-002', 'REIMB')
) s (delegation_code, rule_code) JOIN pm.delegation d ON d.code = s.delegation_code JOIN pm.approval_rule r ON r.code = s.rule_code
WHERE d.created_by = '00000000-0000-7000-8000-000000000000'
ON CONFLICT DO NOTHING;

INSERT INTO pm.custom_flow (code, name, sop, approval_rule_id, scope_unit_id, is_active, created_by)
SELECT s.code, s.name, s.sop, r.id, u.id, s.active, '00000000-0000-7000-8000-000000000000' FROM (VALUES
    ('CF-CAPEX-SUM', 'Capex Region Sumatera', 'SOP-FIN-020 Lamp. 3', 'CAPEX', 'SUM', true),
    ('CF-PR-BLK', 'Pembelian Bulking Sampit', 'SOP-PRC-002 Lamp. 1', 'PR', 'C21-B1', true)
) s (code, name, sop, rule_code, unit_code, active)
JOIN pm.approval_rule r ON r.code = s.rule_code AND NOT r.is_deleted JOIN pm.org_unit u ON u.code = s.unit_code AND NOT u.is_deleted
ON CONFLICT (tenant_id, (lower(code))) DO NOTHING;
INSERT INTO pm.custom_flow_step (custom_flow_id, seq, position_id)
SELECT f.id, s.seq, x.id FROM (VALUES
    ('CF-CAPEX-SUM', 1, 'SUM-RC'),
    ('CF-CAPEX-SUM', 2, 'SUM-RD'),
    ('CF-CAPEX-SUM', 3, 'GAN-HFIN'),
    ('CF-CAPEX-SUM', 4, 'GAN-CEO'),
    ('CF-PR-BLK', 1, 'C21-B1-KBL'),
    ('CF-PR-BLK', 2, 'KALTENG-KAW'),
    ('CF-PR-BLK', 3, 'C21-SMP')
) s (flow_code, seq, position_code) JOIN pm.custom_flow f ON f.code = s.flow_code JOIN pm.position x ON x.code = s.position_code
WHERE f.created_by = '00000000-0000-7000-8000-000000000000'
ON CONFLICT DO NOTHING;

COMMIT;

