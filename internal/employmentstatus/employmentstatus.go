// Package employmentstatus is the master "Status Kepegawaian" (Employee Management, menu pm.employmentStatus).
package employmentstatus

import (
	"context"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
)

type Status struct {
	record.Base
	Permanent      bool `json:"permanent"`
	ContractMonths int  `json:"contractMonths"` // 0 = no fixed term
}

type Input struct {
	record.Input
	Permanent      bool
	ContractMonths int
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Permanent = record.Bool(v, "permanent", "Karyawan tetap", false, &p)
	if n := record.OptionalInt(v, "contractMonths", "Lama kontrak (bulan)", 0, &p); n != nil {
		in.ContractMonths = *n
	}
	switch {
	case p.Has("contractMonths"):
	case in.ContractMonths > 120:
		p.Add("contractMonths", "Lama kontrak (bulan) paling banyak 120.")
	case in.Permanent && in.ContractMonths > 0:
		p.Add("contractMonths", "Karyawan tetap tidak punya lama kontrak; isi 0.")
	}
	return in, p
}

func values(x Status) record.Values {
	v := record.ValuesOf(x.Base)
	v["permanent"], v["contractMonths"] = x.Permanent, x.ContractMonths
	return v
}

func New(db *database.DB) *master.Service[Status, Input] {
	return &master.Service[Status, Input]{
		Spec: master.Spec[Status, Input]{
			Menu: "pm.employmentStatus", Table: "employment_status", Noun: "Status kepegawaian",
			Base: func(x *Status) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var cols = record.Columns + `, is_permanent, contract_months`

func scan(row pgx.Row) (Status, error) {
	var x Status
	return x, x.Scan(row, &x.Permanent, &x.ContractMonths)
}

func (r repo) List(ctx context.Context) ([]Status, error) {
	return master.Rows(ctx, r.db, scan, `SELECT `+cols+` FROM pm.employment_status WHERE NOT is_deleted ORDER BY code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Status, error) {
	q := `SELECT ` + cols + ` FROM pm.employment_status WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Status, error) {
	return master.One(ctx, r.db, scan, `INSERT INTO pm.employment_status (code, name, description, is_active, valid_from, valid_to, created_by,
		is_permanent, contract_months) VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9) RETURNING `+cols,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Permanent, in.ContractMonths)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Status, error) {
	return master.One(ctx, r.db, scan, `UPDATE pm.employment_status SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, is_permanent = $7, contract_months = $8, updated_at = now(), updated_by = $9, version = version + 1
		WHERE id = $1 AND NOT is_deleted RETURNING `+cols, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Permanent, in.ContractMonths, actor)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "employment_status", id, actor)
}
