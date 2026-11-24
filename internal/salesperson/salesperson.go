// Package salesperson is the master "Tenaga Penjualan" (Mitra Bisnis, menu pm.salesPerson): an employee who sells,
// with the sales unit (branch) and an optional commission. A customer (Mitra Bisnis) names its sales person here,
// so one live record per employee; the person's name, NIK and job stay in Karyawan.
package salesperson

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type SalesPerson struct {
	record.Base
	EmployeeID    string   `json:"employeeId"`
	EmployeeCode  string   `json:"employeeCode"`
	UnitID        *string  `json:"unitId"`
	UnitCode      *string  `json:"unitCode"`
	Email         *string  `json:"email"`
	Phone         *string  `json:"phone"`
	CommissionPct *float64 `json:"commissionPct"`
}

type Input struct {
	record.Input
	EmployeeCode  string
	UnitCode      *string
	Email, Phone  *string
	CommissionPct *float64
	EmployeeID    string
	UnitID        *string
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.EmployeeCode = record.RequiredText(v, "employeeCode", "Karyawan", record.MaxCode, &p)
	in.UnitCode = record.OptionalText(v, "unitCode", "Unit penjualan", record.MaxCode, &p)
	in.Email = record.OptionalEmail(v, "email", "Email", &p)
	in.Phone = record.OptionalPhone(v, "phone", "Telepon", &p)
	in.CommissionPct = record.Decimal(v, "commissionPct", "Komisi (%)", 0, 100, 2, false, &p)
	return in, p
}

func values(x SalesPerson) record.Values {
	v := record.ValuesOf(x.Base)
	v["employeeCode"], v["unitCode"], v["email"], v["phone"], v["commissionPct"] = x.EmployeeCode, record.Opt(x.UnitCode), record.Opt(x.Email), record.Opt(x.Phone), record.Opt(x.CommissionPct)
	return v
}

func New(db *database.DB) *master.Service[SalesPerson, Input] {
	return &master.Service[SalesPerson, Input]{
		Spec: master.Spec[SalesPerson, Input]{
			Menu: "pm.salesPerson", Table: "sales_person", Noun: "Tenaga penjualan", Import: importSpec(db),
			Base: func(x *SalesPerson) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *SalesPerson, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func check(ctx context.Context, db *database.DB, in *Input, stored *SalesPerson, p record.Problems) (record.Problems, error) {
	self := ""
	var keptEmp, keptUnit *string
	if stored != nil {
		self, keptEmp, keptUnit = stored.ID, &stored.EmployeeID, stored.UnitID
	}
	// An inactive sales person may point to an inactive employee (the history of one who left, e.g. migrated); an
	// active one needs an active employee.
	if !in.Active && in.EmployeeCode != "" {
		if r, err := refs.Lookup(ctx, db, "employee", in.EmployeeCode); err == nil {
			keptEmp = &r.ID
		}
	}
	emp, err := refs.Pick(ctx, db, "employee", "employeeCode", "Karyawan", nonEmpty(in.EmployeeCode), keptEmp, &p)
	if err != nil {
		return p, err
	}
	unit, err := refs.Pick(ctx, db, "org_unit", "unitCode", "Unit", in.UnitCode, keptUnit, &p)
	if err != nil {
		return p, err
	}
	if unit != nil {
		in.UnitID = &unit.ID
	}
	if emp == nil {
		return p, nil
	}
	in.EmployeeID = emp.ID
	var other string
	err = db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.sales_person WHERE employee_id = $1 AND NOT is_deleted AND id::text <> $2`, emp.ID, self).Scan(&other)
	switch {
	case err == nil:
		p.Add("employeeCode", fmt.Sprintf("Karyawan %s sudah menjadi tenaga penjualan %s.", emp.Name, other))
	case !errors.Is(err, pgx.ErrNoRows):
		return p, err
	}
	return p, nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.employee_id, e.code, x.org_unit_id, u.code, x.email, x.phone, x.commission_pct::float8
	FROM pm.sales_person x JOIN pm.employee e ON e.id = x.employee_id LEFT JOIN pm.org_unit u ON u.id = x.org_unit_id`

func scan(row pgx.Row) (SalesPerson, error) {
	var x SalesPerson
	return x, x.Scan(row, &x.EmployeeID, &x.EmployeeCode, &x.UnitID, &x.UnitCode, &x.Email, &x.Phone, &x.CommissionPct)
}

func (r repo) List(ctx context.Context) ([]SalesPerson, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (SalesPerson, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (SalesPerson, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (SalesPerson, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.sales_person (code, name, description, is_active, valid_from, valid_to, created_by,
		employee_id, org_unit_id, email, phone, commission_pct) VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.EmployeeID, in.UnitID, in.Email, in.Phone, in.CommissionPct).Scan(&id); err != nil {
		return SalesPerson{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (SalesPerson, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.sales_person SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, employee_id = $7, org_unit_id = $8, email = $9, phone = $10, commission_pct = $11,
		updated_at = now(), updated_by = $12, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.EmployeeID, in.UnitID, in.Email, in.Phone, in.CommissionPct, actor); err != nil {
		return SalesPerson{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "sales_person", id, actor)
}
