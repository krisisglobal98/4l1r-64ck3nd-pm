// Package bank is the master "Bank" (Employee Management, menu pm.bank): banks for salary and claim transfers.
package bank

import (
	"context"
	"regexp"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
)

type Bank struct {
	record.Base
	Clearing *string `json:"clearing"` // clearing code, 3–7 digits
}

type Input struct {
	record.Input
	Clearing *string
}

var digits = regexp.MustCompile(`^[0-9]{3,7}$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Clearing = record.OptionalText(v, "clearing", "Kode kliring", 10, &p)
	if in.Clearing != nil && !p.Has("clearing") && !digits.MatchString(*in.Clearing) {
		p.Add("clearing", "Kode kliring berisi 3–7 angka, mis. 002.")
	}
	return in, p
}

func values(x Bank) record.Values {
	v := record.ValuesOf(x.Base)
	v["clearing"] = nil
	if x.Clearing != nil {
		v["clearing"] = *x.Clearing
	}
	return v
}

func New(db *database.DB) *master.Service[Bank, Input] {
	return &master.Service[Bank, Input]{
		Spec: master.Spec[Bank, Input]{
			Menu: "pm.bank", Table: "bank", Noun: "Bank",
			Base: func(x *Bank) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var cols = record.Columns + `, clearing_code`

func scan(row pgx.Row) (Bank, error) {
	var x Bank
	return x, x.Scan(row, &x.Clearing)
}

func (r repo) List(ctx context.Context) ([]Bank, error) {
	return master.Rows(ctx, r.db, scan, `SELECT `+cols+` FROM pm.bank WHERE NOT is_deleted ORDER BY code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Bank, error) {
	q := `SELECT ` + cols + ` FROM pm.bank WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Bank, error) {
	return master.One(ctx, r.db, scan, `INSERT INTO pm.bank (code, name, description, is_active, valid_from, valid_to, created_by, clearing_code)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8) RETURNING `+cols, in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Clearing)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Bank, error) {
	return master.One(ctx, r.db, scan, `UPDATE pm.bank SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, clearing_code = $7, updated_at = now(), updated_by = $8, version = version + 1
		WHERE id = $1 AND NOT is_deleted RETURNING `+cols, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Clearing, actor)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "bank", id, actor)
}
