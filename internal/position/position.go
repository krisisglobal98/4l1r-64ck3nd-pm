// Package position is the Portal Master master "Posisi" (menu pm.position): one job title in one unit, its
// reporting line (what the default approval walks), whether it heads the unit, and who holds it: the holder and
// the acting holder (Plt), employees (Karyawan). Empty holder = vacant.
package position

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type Position struct {
	record.Base
	UnitID             string  `json:"unitId"`
	UnitCode           string  `json:"unitCode"`
	JabatanID          string  `json:"jabatanId"`
	JabatanCode        string  `json:"jabatanCode"`
	ReportsToID        *string `json:"reportsToId"`
	ReportsToCode      *string `json:"reportsToCode"`
	Head               bool    `json:"head"`
	HolderEmployeeID   *string `json:"holderEmployeeId"`
	HolderEmployeeCode *string `json:"holderEmployeeCode"`
	ActingEmployeeID   *string `json:"actingEmployeeId"`
	ActingEmployeeCode *string `json:"actingEmployeeCode"`
}

type Input struct {
	record.Input
	UnitCode, JabatanCode string
	ReportsToCode         *string
	HolderCode            *string
	ActingCode            *string
	HolderID, ActingID    *string
	Head                  bool
	AutoName              bool // the name was left empty: "<jabatan> · <unit>"
	UnitID, JabatanID     string
	ReportsToID           *string
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	auto := false
	if s, ok := v["name"].(string); v["name"] == nil || ok && strings.TrimSpace(s) == "" {
		auto = true
		v = clone(v)
		v["name"] = "x"
	}
	in := Input{Input: record.ParseInput(v, &p), AutoName: auto}
	in.UnitCode = record.RequiredText(v, "unitCode", "Unit", record.MaxCode, &p)
	in.JabatanCode = record.RequiredText(v, "jabatanCode", "Jabatan", record.MaxCode, &p)
	in.ReportsToCode = record.OptionalText(v, "reportsToCode", "Melapor ke", record.MaxCode, &p)
	in.Head = record.Bool(v, "head", "Kepala unit", false, &p)
	in.HolderCode = record.OptionalText(v, "holderEmployeeCode", "Pemegang", record.MaxCode, &p)
	in.ActingCode = record.OptionalText(v, "actingEmployeeCode", "Plt", record.MaxCode, &p)
	if in.HolderCode != nil && in.ActingCode != nil && strings.EqualFold(*in.HolderCode, *in.ActingCode) {
		p.Add("actingEmployeeCode", "Plt tidak boleh pemegang posisi itu sendiri.")
	}
	return in, p
}

func clone(v record.Values) record.Values {
	out := make(record.Values, len(v))
	for k, x := range v {
		out[k] = x
	}
	return out
}

func values(x Position) record.Values {
	v := record.ValuesOf(x.Base)
	v["unitCode"], v["jabatanCode"], v["head"] = x.UnitCode, x.JabatanCode, x.Head
	v["reportsToCode"], v["holderEmployeeCode"], v["actingEmployeeCode"] = opt(x.ReportsToCode), opt(x.HolderEmployeeCode), opt(x.ActingEmployeeCode)
	return v
}

func New(db *database.DB) *master.Service[Position, Input] {
	return &master.Service[Position, Input]{
		Spec: master.Spec[Position, Input]{
			Menu: "pm.position", Table: "position", Noun: "Posisi",
			Base:    func(x *Position) *record.Base { return &x.Base },
			Values:  values,
			Parse:   Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Position, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
			// A reporting line locks two positions; saves crossing lines (A under B, B under A) would lock them in
			// opposite orders. One line for every position write keeps the loop check and the one-head rule exact.
			Serialize: true,
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func opt(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func check(ctx context.Context, db *database.DB, in *Input, stored *Position, p record.Problems) (record.Problems, error) {
	var keptHolder, keptActing *string
	if stored != nil {
		keptHolder, keptActing = stored.HolderEmployeeID, stored.ActingEmployeeID
	}
	if h, err := refs.Pick(ctx, db, "employee", "holderEmployeeCode", "Karyawan", in.HolderCode, keptHolder, &p); err != nil {
		return p, err
	} else if h != nil {
		in.HolderID = &h.ID
	}
	if a, err := refs.Pick(ctx, db, "employee", "actingEmployeeCode", "Karyawan", in.ActingCode, keptActing, &p); err != nil {
		return p, err
	} else if a != nil {
		in.ActingID = &a.ID
	}
	var keptUnit, keptJabatan, keptBoss *string
	if stored != nil {
		// The unit and the job title make the position: fixed once it exists.
		in.UnitCode, in.JabatanCode = stored.UnitCode, stored.JabatanCode
		keptUnit, keptJabatan, keptBoss = &stored.UnitID, &stored.JabatanID, stored.ReportsToID
	}
	unit, err := refs.Pick(ctx, db, "org_unit", "unitCode", "Unit", nonEmpty(in.UnitCode), keptUnit, &p)
	if err != nil {
		return p, err
	}
	jab, err := refs.Pick(ctx, db, "jabatan", "jabatanCode", "Jabatan", nonEmpty(in.JabatanCode), keptJabatan, &p)
	if err != nil {
		return p, err
	}
	if unit != nil && jab != nil {
		in.UnitID, in.JabatanID = unit.ID, jab.ID
		var unitLevel, jabLevel, levelName string
		if err := db.Q(ctx).QueryRow(ctx, `SELECT u.org_level_id, j.org_level_id, l.name FROM pm.org_unit u, pm.jabatan j
			JOIN pm.org_level l ON l.id = j.org_level_id WHERE u.id = $1 AND j.id = $2`, unit.ID, jab.ID).Scan(&unitLevel, &jabLevel, &levelName); err != nil {
			return p, err
		}
		if unitLevel != jabLevel {
			p.Add("jabatanCode", fmt.Sprintf("Jabatan %s dipakai di level %s; pilih jabatan untuk level unit %s.", jab.Name, levelName, unit.Name))
		}
		if in.AutoName {
			in.Name = truncate(jab.Name+" · "+unit.Name, record.MaxName)
		}
	}

	if boss, err := refs.Pick(ctx, db, "position", "reportsToCode", "Posisi atasan", in.ReportsToCode, keptBoss, &p); err != nil {
		return p, err
	} else if boss != nil {
		in.ReportsToID = &boss.ID
		if stored != nil {
			// The reporting line must not come back to this position.
			var loop bool
			if err := db.Q(ctx).QueryRow(ctx, `WITH RECURSIVE up AS (
				SELECT id, reports_to_id, 1 AS depth FROM pm.position WHERE id = $1
				UNION ALL SELECT x.id, x.reports_to_id, up.depth + 1 FROM pm.position x JOIN up ON x.id = up.reports_to_id WHERE up.depth < 64)
				SELECT EXISTS (SELECT 1 FROM up WHERE id = $2)`, boss.ID, stored.ID).Scan(&loop); err != nil {
				return p, err
			}
			if loop {
				p.Add("reportsToCode", "Garis lapor tidak boleh membentuk lingkaran.")
			}
		}
	}

	if in.Head && unit != nil {
		self := ""
		if stored != nil {
			self = stored.ID
		}
		var other string
		err := db.Q(ctx).QueryRow(ctx, `SELECT name FROM pm.position WHERE org_unit_id = $1 AND is_head AND NOT is_deleted AND id::text <> $2`, unit.ID, self).Scan(&other)
		switch {
		case err == nil:
			p.Add("head", fmt.Sprintf("Unit %s sudah punya kepala: %s.", unit.Name, other))
		case !errors.Is(err, pgx.ErrNoRows):
			return p, err
		}
	}
	return p, nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type repo struct{ db *database.DB }

var selectPosition = `SELECT ` + record.ColumnsAs("x") + `, x.org_unit_id, u.code, x.jabatan_id, j.code, x.reports_to_id, b.code,
	x.is_head, x.holder_employee_id, h.code, x.acting_employee_id, a.code
	FROM pm.position x JOIN pm.org_unit u ON u.id = x.org_unit_id JOIN pm.jabatan j ON j.id = x.jabatan_id
	LEFT JOIN pm.position b ON b.id = x.reports_to_id
	LEFT JOIN pm.employee h ON h.id = x.holder_employee_id LEFT JOIN pm.employee a ON a.id = x.acting_employee_id`

func scan(row pgx.Row) (Position, error) {
	var x Position
	return x, x.Scan(row, &x.UnitID, &x.UnitCode, &x.JabatanID, &x.JabatanCode, &x.ReportsToID, &x.ReportsToCode, &x.Head, &x.HolderEmployeeID, &x.HolderEmployeeCode, &x.ActingEmployeeID, &x.ActingEmployeeCode)
}

func (r repo) List(ctx context.Context) ([]Position, error) {
	return master.Rows(ctx, r.db, scan, selectPosition+` WHERE NOT x.is_deleted ORDER BY u.code, NOT x.is_head, x.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Position, error) {
	q := selectPosition + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Position, error) {
	return master.One(ctx, r.db, scan, selectPosition+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Position, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.position (code, name, description, is_active, valid_from, valid_to, created_by,
		org_unit_id, jabatan_id, reports_to_id, is_head, holder_employee_id, acting_employee_id)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.UnitID, in.JabatanID, in.ReportsToID, in.Head, in.HolderID, in.ActingID).Scan(&id); err != nil {
		return Position{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Position, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.position SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, reports_to_id = $7, is_head = $8, holder_employee_id = $10, acting_employee_id = $11,
		updated_at = now(), updated_by = $9, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.ReportsToID, in.Head, actor, in.HolderID, in.ActingID); err != nil {
		return Position{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "position", id, actor)
}
