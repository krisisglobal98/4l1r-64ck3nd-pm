# ALIR Portal Master service

Backend of Portal Master for the ALIR frontend (`../ALIR`). Go 1.25, PostgreSQL, standard library HTTP.
It serves every master of **Organisasi & Approval**: Level Organisasi, Jenis Unit Operasional, Unit Organisasi
(Struktur organisasi), Grade Jabatan, Jabatan, Posisi (Jabatan & pejabat), Aturan Approval, Delegasi and Approval
Custom (Alur approval); and Karyawan with the Employee Management masters it uses (Status Kepegawaian, Jenjang
Pendidikan, Shift Kerja); and the global reference masters: **Keuangan & Pajak** (Mata Uang, Bank, Syarat
Pembayaran, Jenis Pajak), **Geografi & Lokasi** (Negara, Kota) **Mitra Bisnis** (Mitra Bisnis, Tenaga Penjualan) and **Logistik & Pengiriman** (Incoterms, Jenis Kontainer, Kemasan,
Satuan, Komoditas, Moda & Layanan) with Pelabuhan & Lokasi; Kurs, Kode Biaya and Rekening Bank Perusahaan
(Keuangan & Pajak); and **Dokumen** (Jenis Dokumen).
The other masters still come from the frontend's sample data until they move here.

```bash
cp .env.example .env          # MIGRATE_DATABASE_URL (owner), DATABASE_URL (app role alir_pm_app), PM_APP_PASSWORD
make db-roles                 # once per Postgres cluster: the application role alir_pm_app
make migrate-up               # schema pm, built-in levels, code prefixes, then grants for alir_pm_app
make seed-sample              # optional, development/demo only: the ALIR sample organization (seeds/sample_org.sql)
make run                      # http://localhost:8081
```

In ALIR, set `VITE_PM_API_URL=/pm-api` in `.env` and run `npm run dev`. The Vite dev server proxies `/pm-api` here.

Other commands: `make test` (unit tests), `make test-integration` (also Postgres, against `TEST_DATABASE_URL`;
each test rolls back), `make test-e2e` (black-box suite over HTTP, below), `make build`, `make migrate-down`.

## Multi-tenant (one installation, many companies)
Every table has `tenant_id`; codes, NIK and emails are unique **per tenant**; generated codes are numbered per
tenant. Postgres **Row-Level Security** (`tenant_isolation` policy on every table) keeps each company's rows to
itself, so a missed filter in a query cannot leak another company's data:

| Piece | How |
|---|---|
| Who the caller's company is | the `tid` claim of the Authentication token (required); `PM_AUTH_MODE=default-user` (dev/e2e) uses `PM_DEFAULT_TENANT_ID`, or an `X-Tenant-ID` header |
| Per request | one connection, `set_config('app.tenant_id', tid)`; cleared when the connection goes back to the pool |
| The service's database role | `alir_pm_app`: not the owner, not a superuser. **The service refuses to start** on a role that bypasses RLS |
| Migrations, seeds | run as the owner (`MIGRATE_DATABASE_URL`); seeds set `app.tenant_id` themselves |
| A new company | prepared on its first request: the 8 built-in levels and every code prefix (nothing else) |
| Existing data | migration 000010 put every row in the first tenant, "Grup Agro Nusantara" (`00000000-0000-7000-8000-0000000000a1`) |

## API
Contract: `api/openapi.yaml`.

| Method | Path | Answer |
|---|---|---|
| GET | `/pm/masters/levels` | levels by rank |
| GET | `/pm/masters/levels/{code}` | one level (code in any case), 404 |
| POST | `/pm/masters/levels` | 201 with the generated code; 422 `{details: {problems: [{field, message}]}}`; 409 `NO_PREFIX` |
| PUT | `/pm/masters/levels/{code}` | 200; fields sent replace the stored ones, `code` stays; with `version`: 409 `STALE` when someone saved in between |
| DELETE | `/pm/masters/levels/{code}` | 204, a soft delete; 409 for the built-in levels |
| GET, POST | `/pm/masters/unit-types` | as levels (sorted by code; codes MJU001, …) |
| GET, PUT, DELETE | `/pm/masters/unit-types/{code}` | as levels; DELETE 409 while units use the type |
| GET, POST | `/pm/masters/org-units` | units with `levelCode`, `parentCode`, `unitTypeCode` (and their ids); codes MUO001, … |
| GET, PUT, DELETE | `/pm/masters/org-units/{code}` | as levels; DELETE 409 while it is used |
| GET, POST, GET/PUT/DELETE `/{code}` | `/pm/masters/grades`, `/jabatan`, `/positions`, `/approval-rules`, `/delegations`, `/custom-flows`, `/employees`, `/employment-status`, `/education-levels`, `/work-shifts`, `/banks`, `/currencies`, `/countries`, `/cities`, `/payment-terms`, `/tax-types`, `/sales-persons`, `/business-partners`, `/ports`, `/incoterms`, `/containers`, `/packagings`, `/uoms`, `/commodities`, `/service-types`, `/exchange-rates`, `/charges`, `/accounts`, `/company-bank-accounts`, `/company-profiles`, `/system-parameters`, `/document-types` | the same contract; relations by code (`gradeCode`, `levelCode`, `unitCode`, `jabatanCode`, `reportsToCode`, `positionCode`, `ruleCodes`, `ruleCode`, `scopeUnitCode`, `stepCodes`) |
| GET | `/healthz`, `/readyz` | process up; database reachable |

Errors are `{code, message, details?}` with Indonesian messages, the shape the ALIR API client reads, also for an
unknown address (404 `NOT_FOUND`), a wrong method (405 `METHOD_NOT_ALLOWED`), a body over 1 MiB (413 `TOO_LARGE`)
and malformed JSON (400). A value the database refuses is a 400, never a 500. Every response
carries `X-Request-ID`; a caller's id is kept and logged.

## Layout (hexagonal, one folder per master)
```
cmd/pm-service/        wiring: config, database, routes, server
internal/platform/     shared by every master
  record/              standard columns of every master table and their checks
  codeprefix/          generated codes from pm.code_prefix (ML001, …)
  config/              .env and environment
  database/            pgx pool; InTx puts the transaction in the context
  httpx/               JSON, error body, request id, logging, recover
  access/              who calls and may(user, menu, action)
  apperr/              errors of the use cases (status, code, message)
  outbox/              events written in the same transaction
  master/              the shared use-case shell (codes, version, merge, soft delete with usage, events, routes)
  refs/                lookups of related rows by code (locked for the save) and who uses a row (delete checks)
internal/orglevel/      Level Organisasi (the pilot every master copies)
  domain.go            Level and its rules (no database, no HTTP)
  service.go           use cases and the ports they need (Repository, TxRunner, Codes, Events)
  postgres.go          Repository on pm.org_level
  http.go              routes
internal/unittype/      Jenis Unit Operasional, same files on pm.unit_type
internal/orgunit/       Unit Organisasi on pm.org_unit: the tree and its relations (levels, unit types, parent)
internal/grade/ … customflow/  the other masters on the shared shell: one file each (Spec + Repo)
seeds/                 sample data for development (make seed-sample), never run by migrations
migrations/            golang-migrate
test/e2e/              black-box suite: main_test.go (shared), one file per master
```
Dependencies point inward: `http.go` → `service.go` → `domain.go`. `postgres.go` implements the port that
`service.go` declares.

### Adding a master
1. Add a migration for its table: the standard columns first (copy them from `000006`), then the page's own.
   Add its row to `pm.code_prefix` (code = table name, prefix, digits). Lists of a record are detail tables with
   `ON DELETE CASCADE`, replaced on save.
2. Write `internal/<master>/<master>.go` on the shared shell, as `internal/grade` (the smallest) or
   `internal/position` (relations): the record (`record.Base` + its own fields), `Parse`, `values`, a `Spec`
   (`Check` resolves relations with `refs.Pick`) and a `repo`. Add who points to its rows to `usage` in
   `internal/platform/refs`, so deletes are checked.
   A relation to a master that is not in this service yet is a nullable `<name>_id uuid` column without a foreign
   key, left NULL and not read from the body (see `unit_type.head_jabatan_id`); add the key when that master moves.
3. Register it in `cmd/pm-service/main.go`.
4. Copy `test/e2e/unit_types_test.go` for the master and add its table to `reset()` in `test/e2e/main_test.go`.
5. In ALIR, add it to `PM_MASTERS` in `src/services/api/remoteApi.ts` (path and collection, plus `fromApi`/`toApi`/
   `fields` when the page's model differs from the API, as `src/services/org/orgUnitApi.ts`); the structure answers
   (`/pm/data`, `/org/structure`) then take its list from here. Give the form the standard fields (`auto` code,
   description, validity, active) and mark relations not served yet with `pending`.

## Standard columns of every master table
Every master table starts with the same blocks. The columns of its own page come after them, in the table and in
the JSON (see `migrations/000001_org_level.up.sql` and `internal/platform/record`):

| Block | Columns | Set by |
|---|---|---|
| Primary key | `id uuid` (UUIDv7, `pm.uuid_v7()`) | database |
| Master data | `code` (40, unique in any case), `name` (150), `description` (500) | code: service; rest: form |
| Status | `is_active`, `is_deleted` | form / delete |
| Workflow | `state` (where the record is in its approval), `status` (approved, void, cancelled, posted, …), `remark`, `approval_level_required`, `approved_at`, `approved_by` | approval flow, never the form |
| Validity | `valid_from`, `valid_to` (dates, inclusive) | form |
| Audit | `created_at/by`, `updated_at/by`, `deleted_at/by`, `version` | service |

- **Ids and codes:** relations inside a service use `id`. A UUID is copied as is when data moves to another
  database, so a migration keeps relations intact. `code` is the business key: unique, fixed once saved, and used
  for import, export, integration and mapping data from legacy systems.
- **Time:** every instant is `timestamptz`, an absolute moment stored in UTC and never in a regional zone. The pool
  sets `timezone=UTC` on every session, every time comes from the database's `now()` (one clock for all instances),
  and the API answers RFC 3339 in UTC (`…Z`). `valid_from`/`valid_to` are business dates and have no zone.
- **User ids** in `*_by` are UUIDs from the Authentication service, with no foreign key across services. For now
  the id is `PM_DEFAULT_USER_ID`. The seed rows use the system user `00000000-0000-7000-8000-000000000000`.
- **Codes are generated**, never typed: prefix + next number from the master's row in `pm.code_prefix`, padded to
  `digits` (`org_level` → `ML001`). The number is taken inside the save transaction; the row lock keeps parallel
  saves apart and a failed save gives its number back. A number whose code is already taken is skipped. The table
  is the setup data of the future master *Prefix Kode Master*. The five built-in levels keep their codes (`group`,
  …, `unit`) because the frontend and Authentication refer to them by name.
- **Delete is soft:** the row is marked deleted and inactive and leaves every answer. Its code stays taken, because
  other masters refer to a level by its code.

## Rules worth knowing
- **Access:** every use case calls `access.Require(ctx, menu, action)`. The caller comes from the access token of
  the Authentication service (`PM_AUTH_MODE=jwt`, the default): EdDSA signature by a key of its JWKS
  (`PM_AUTH_JWKS_URL`), issuer `PM_AUTH_ISSUER`, audience `alir`, expiry; the grants travel in the token. No token:
  401 `SIGNED_OUT`; expired: 401 `TOKEN_EXPIRED` (ALIR refreshes and repeats); a missing grant: 403.
  `PM_AUTH_MODE=default-user` gives every request `PM_DEFAULT_USER` with full access: development and `make test-e2e`
  only.
- **Events:** every create, update or delete writes `pm.org_level.created|updated|deleted` (with the record id, code
  and actor id) to `pm.outbox` in the same
  transaction. No relay publishes them yet; it comes with the event bus (Authentication's copy of the org structure
  will subscribe).
- **Built-in levels** (`group`, `region`, `wilayah`, `company`, `unit`) cannot be deleted while units and job titles
  live outside this service. Their names and ranks can change.
- **Seeded unit types** (`EST`, `PKS`, `PLS`, `BLK`, `KTR`) keep their codes (the sample units use them).
- **Usage is checked, not assumed:** a unit type or level used by a live unit cannot be deleted (409, deactivate it
  instead), and a level in use keeps its rank. A unit with units under it cannot be deleted.
- **The tree (Unit Organisasi):** relations are foreign keys by id; the API names them by code (`levelCode`,
  `parentCode`, `unitTypeCode`) and answers both. The parent is a unit on the level just above (the next lower
  rank; none at the top), the unit type is required at the Unit Operasional level and refused elsewhere, a unit's
  level cannot change while units hang under it, and it cannot sit under itself. A new choice of level, parent or
  type must be active; an existing one may stay after it is deactivated. The level, parent and type read for a
  save are locked (`FOR SHARE`) until it commits, so they cannot be deleted under it.
- **Racing saves:** a transaction Postgres aborts for a deadlock or a serialization conflict is run again (up to 4
  times) by `database.InTx`; a unique index that refuses a racing save answers 409 `CONFLICT`. Masters whose rows
  lock each other in both directions (Posisi: A reports to B while B reports to A) set `Spec.Serialize`: every write
  takes one advisory lock first, one fixed order instead of row locks that deadlock.
- **Jabatan & pejabat:** one live grade per rank; a position's job title is at its unit's level; unit and job title
  are fixed once the position exists; one head per unit; the reporting line has no loop; an empty name becomes
  "<jabatan> · <unit>". A job title used by positions keeps its level.
- **Alur approval:** the reach is `steps`, `level` or `amount` bands (rising amounts, the same or a higher level,
  an open band only last); a delegation needs a period; one custom flow per document type and branch, steps in
  order without repeats.
- **Organisation levels 1–3** (`org1`–`org3`, built in, ranks 6–8) are generic levels under the operational
  unit; a company renames them (a plantation: Rayon, Afdeling, Blok). Their units are made in Unit Organisasi under
  an operational unit; user scopes (Authentication) reach down to them.
- **Karyawan:** one live employee per NIK; status kepegawaian required, education, shift and bank optional. A
  position's holder and Plt and a delegation's delegate are employees (Plt never the holder; a delegate never the
  holder of the position). Shift times are wall-clock HH:MM; a shift ending the next day is a night shift.
- **Reference masters:** ISO codes (`isoCode` ISO 4217; `iso2`/`iso3` ISO 3166) are separate unique columns; the
  business `code` is still generated (MMU, MNG, MKT, MTP, MPJ). A new tenant gets 36 currencies and 42 countries,
  seeded only while those tables are empty. Bank adds `swiftCode` (BIC 8 or 11) and `countryCode`. Syarat
  Pembayaran: `days` 0–365 from a `basis` (invoice/receipt/BL date, ETD, ETA, end of month, delivery), optional down
  payment. Jenis Pajak: `ratePct` on `basePct` of the base gives `effectivePct`; PPN/PPnBM are added, PPh withheld.
- **Mitra Bisnis:** one record per legal party with its `roles` (customer, vendor, shipper, consignee, notify_party,
  agent, carrier, trucking, customs_broker, warehouse), never one per role. The lists `addresses`, `contacts` and
  `bankAccounts` are arrays inside the record, replaced as a whole when sent (left out: kept); a problem in a row is
  reported on `<list>.<row>.<cell>`, rows from 0; at most one `primary` row per list, the first when none is marked.
  NPWP 16 digits (a 15-digit one gets a leading 0), NITKU = NPWP + 6 digits, PKP needs NPWP; one live partner per
  NITKU, or per NPWP without NITKU (head office). Customer terms (payment term, currency and an address required for
  the customer role; credit limit, credit hold, sales person) are kept only with the customer or agent role; vendor
  terms (payment term, currency, a withheld PPh) only with a payable role; removing the role clears them.
- **Logistics masters:** each keeps its international code in its own unique column (UN/LOCODE, IATA, ISO 6346,
  UN/ECE Rec. 21 and 20; HS digits only, not unique). A location's UN/LOCODE starts with its country's ISO code, an
  IATA code is for airports only, its city lies in its country. A new tenant gets Incoterms 2020, 10 container types,
  12 packagings and 11 units (seeded while each table is empty). A container's TEU defaults from its length and
  `payloadKg` = max gross − tare. Dangerous goods need a UN number and a class; otherwise both are cleared. A unit's
  `factor` is in its dimension's base (kg, m³, m, m², piece, hour); dimension `other` has none.
- **Kurs:** history, one record per pair, type (`tax`, `bi_middle`, `bank_buy`, `bank_sell`, `internal`) and
  `validFrom` (required); the target currency defaults to IDR and the name is made from pair, type and date.
  `GET /pm/masters/exchange-rates/lookup?from=USD&to=IDR&type=tax&date=YYYY-MM-DD` answers the latest active rate
  valid on that date (404 when none; codes or ISO codes; defaults to=IDR, type=tax, date=today UTC).
- **Kode Biaya:** one code for both sides (`receivable` billed to customers, `payable` paid to vendors; at least one).
  PPN (added) only with receivable, PPh (withheld) only with payable, otherwise cleared; `rates` per container type
  and currency only for basis `container`; a default rate needs a currency except basis `percent` (≤ 100).
- **Bagan Akun (COA)** (`/accounts`, menu `pm.account`): `accountNo` (the company's number, unique), `accountType`
  (cash_bank … other_expense; sets `normalBalance`), `header` (groups detail accounts, never journaled), `parentCode`
  (a header of the same type), `currencyCode` (foreign-currency accounts). **Kode Biaya** points to
  `revenueAccountCode` (when receivable: Pendapatan / Pendapatan Lain-lain) and `costAccountCode` (when payable: HPP /
  Beban / Beban Lain-lain), detail accounts only.
- **Excel import** (masters with `Spec.Import`; all Keuangan & Pajak masters: Kode Biaya, Mata Uang, Kurs, Bank,
  Rekening Bank Perusahaan, Bagan Akun, Syarat Pembayaran, Jenis Pajak — each in its package's `import.go`, built from
  `master.CodeColumn` / `NameColumn` / `ValidityColumns` / `TailColumns` and the converters `Lookup`, `Number`,
  `Integer`, `Date`, `YesNo`, `ByLabel`; cells are read raw, so an Excel date cell and DD/MM/YYYY text both read, and
  `Text` columns keep leading zeros): `GET /{master}/template.xlsx` — sheets Data (header
  row, drop-downs), Petunjuk (each column), and reference sheets (Mata Uang, Pajak, Akun); `POST /{master}/upload`
  (the .xlsx as the body, ≤ 5 MB, ≤ 2000 rows) checks every row through the form's own create/update in one
  transaction and answers `{rows[{row, action, code, name, problems}], added, updated, failed, committed}`;
  `?commit=true` keeps it only when no row fails (a row with a Kode changes only its filled cells). With `Import.Unique`
  (names; Mata Uang also the ISO code, Bank the SWIFT; Bagan Akun the account number, and the name only among the
  accounts under one parent — `ImportUnique.Scope`, the same rule as the form) a row whose value another
  record or an earlier row of the file already has (any case) is never added: the row fails with the existing code,
  and the user chooses to fill Kode with it (update) or fix the file and upload again.
  Mitra Bisnis has one row per partner: its own fields, roles as several labels in one cell (`master.Multi`), and the
  PRIMARY address, contact and bank account as flat columns that `Import.Assemble` puts into the lists (a change by Kode
  replaces the filled cells of the primary rows only); `Import.Alias` names the column of a list field in a problem.
  Masters without an import answer 501 `NO_IMPORT`.
- **System Parameter** (`/system-parameters`, menu `pm.systemParameter`): named values the modules read by `key`
  (capitals, digits, underscores; unique, never changed), `value`, `module`; known keys are checked
  (`INVOICE_DEFAULT_DUE_DAYS`: 0–365). `GET /pm/parameters/{key}` answers the active one, open to every signed-in user.
- **Profil Perusahaan** (`/company-profiles`, menu `pm.orgUnit`): legal and trade name, NPWP (16 digits; 15 accepted),
  address, city, country, phone, fax, email, website and a logo (data URL: PNG, JPEG, SVG or WebP, about 400 KB at most);
  one per company unit or one for the tenant, and one marked `printDefault`. `GET /pm/company-profile/print?unit=` is
  what documents print (the unit's own, else the default, else the only one; 404 otherwise), open to every signed-in
  user of the tenant.
- **Rekening Bank Perusahaan:** once per bank and number; one `invoiceDefault` per currency and owning unit, never a
  payroll or tax account. **Jenis Dokumen:** a BC code once; export, import or domestic. **Moda & Layanan:** the load
  type fits the mode; FCL always needs containers. New tenants get 17 document types and 10 services.
- **Tenaga Penjualan:** one live record per employee, optional sales unit and commission.
- **Pending relations:** `unit_type.head_jabatan_id` stays NULL until the Jabatan master is served here; the API
  answers `headJabatanId: null` and ignores the head job title the form may send.
- **Codes** are unique in any case, deleted rows included (unique index on `lower(code)`), and looked up in any case.
- **One live level per rank** (`org_level_rank_live`): rank orders the tree and "one level above" is rank − 1.
  A deleted level frees its rank. To swap two ranks, move one to a free rank first.
- **Lost updates:** a change may carry the `version` it was read at (ALIR sends it from the form and the list);
  a different stored version answers 409 `STALE` and nothing is written. Without `version` the last save wins.
- **Text** is trimmed (names), must be valid UTF-8 without NUL, and keeps its line breaks (descriptions).

## Tests
| Command | What | Database |
|---|---|---|
| `make test` | rules and use cases with an in-memory repository | none |
| `make test-integration` | plus the Postgres repository, each test in a rolled-back transaction | `TEST_DATABASE_URL` |
| `make test-e2e` | black-box over HTTP: positive (P), negative (N), stress (S) scenarios | `TEST_DATABASE_URL`, reset before and after |

`test/e2e/levels_test.go` is the pilot suite for every master: copy it with the master. It starts the service in the
Asia/Jakarta zone on purpose, and its stress part runs 300 parallel creates (codes unique and gap-free), 100 parallel
updates of one record (none lost), deletes racing updates, parallel creates on one rank (exactly one wins), and
3000 reads during writes (p95 under 500 ms; about 50 ms locally).
