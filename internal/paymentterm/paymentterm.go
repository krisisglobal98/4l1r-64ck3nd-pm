// Package paymentterm is the master "Syarat Pembayaran" (Keuangan & Pajak, menu pm.paymentTerm): how many days after
// which date an invoice is due, an optional down payment, and whether it is for customers, vendors or both.
package paymentterm

import (
	"context"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
)

// Bases are the dates the days count from.
var Bases = []string{"invoice_date", "receipt_date", "bl_date", "etd", "eta", "end_of_month", "delivery"}

type PaymentTerm struct {
	record.Base
	Days           int     `json:"days"`
	Basis          string  `json:"basis"`
	DownPaymentPct float64 `json:"downPaymentPct"`
	AppliesTo      string  `json:"appliesTo"` // customer | vendor | both
}

type Input struct {
	record.Input
	Days           int
	Basis          string
	DownPaymentPct float64
	AppliesTo      string
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Days = record.RequiredInt(v, "days", "Jumlah hari", 0, &p)
	if !p.Has("days") && in.Days > 365 {
		p.Add("days", "Jumlah hari paling banyak 365.")
	}
	in.Basis = record.ChoiceOr(v, "basis", "Dasar perhitungan", Bases, "invoice_date", &p)
	if d := record.Decimal(v, "downPaymentPct", "Uang muka (%)", 0, 100, 2, false, &p); d != nil {
		in.DownPaymentPct = *d
	}
	in.AppliesTo = record.ChoiceOr(v, "appliesTo", "Berlaku untuk", []string{"customer", "vendor", "both"}, "both", &p)
	return in, p
}

func values(x PaymentTerm) record.Values {
	v := record.ValuesOf(x.Base)
	v["days"], v["basis"], v["downPaymentPct"], v["appliesTo"] = x.Days, x.Basis, x.DownPaymentPct, x.AppliesTo
	return v
}

func New(db *database.DB) *master.Service[PaymentTerm, Input] {
	return &master.Service[PaymentTerm, Input]{
		Spec: master.Spec[PaymentTerm, Input]{
			Menu: "pm.paymentTerm", Table: "payment_term", Noun: "Syarat pembayaran",
			Base: func(x *PaymentTerm) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var cols = record.Columns + `, days, basis, down_payment_pct, applies_to`

func scan(row pgx.Row) (PaymentTerm, error) {
	var x PaymentTerm
	return x, x.Scan(row, &x.Days, &x.Basis, &x.DownPaymentPct, &x.AppliesTo)
}

func (r repo) List(ctx context.Context) ([]PaymentTerm, error) {
	return master.Rows(ctx, r.db, scan, `SELECT `+cols+` FROM pm.payment_term WHERE NOT is_deleted ORDER BY days, code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (PaymentTerm, error) {
	q := `SELECT ` + cols + ` FROM pm.payment_term WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (PaymentTerm, error) {
	return master.One(ctx, r.db, scan, `INSERT INTO pm.payment_term (code, name, description, is_active, valid_from, valid_to, created_by, days, basis, down_payment_pct, applies_to)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11) RETURNING `+cols,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Days, in.Basis, in.DownPaymentPct, in.AppliesTo)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (PaymentTerm, error) {
	return master.One(ctx, r.db, scan, `UPDATE pm.payment_term SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		days = $7, basis = $8, down_payment_pct = $9, applies_to = $10, updated_at = now(), updated_by = $11, version = version + 1
		WHERE id = $1 AND NOT is_deleted RETURNING `+cols, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Days, in.Basis, in.DownPaymentPct, in.AppliesTo, actor)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "payment_term", id, actor)
}
