// Package currency is the master "Mata Uang" (Keuangan & Pajak, menu pm.currency): the currencies of transactions,
// by their ISO 4217 code (unique per tenant beside the generated code). Exchange rates are a master of their own.
package currency

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type Currency struct {
	record.Base
	IsoCode  string  `json:"isoCode"` // ISO 4217, e.g. IDR
	Symbol   *string `json:"symbol"`
	Decimals int     `json:"decimals"` // digits after the decimal point in amounts (IDR 0, USD 2)
}

type Input struct {
	record.Input
	IsoCode  string
	Symbol   *string
	Decimals int
}

var iso = regexp.MustCompile(`^[A-Z]{3}$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.IsoCode = strings.ToUpper(record.RequiredText(v, "isoCode", "Kode ISO", 3, &p))
	if in.IsoCode != "" && !p.Has("isoCode") && !iso.MatchString(in.IsoCode) {
		p.Add("isoCode", "Kode ISO 4217 berisi 3 huruf, mis. IDR atau USD.")
	}
	in.Symbol = record.OptionalText(v, "symbol", "Simbol", 5, &p)
	in.Decimals = 2
	if d := record.OptionalInt(v, "decimals", "Angka desimal", 0, &p); d != nil {
		if *d > 4 {
			p.Add("decimals", "Angka desimal paling banyak 4.")
		}
		in.Decimals = *d
	}
	return in, p
}

func values(x Currency) record.Values {
	v := record.ValuesOf(x.Base)
	v["isoCode"], v["decimals"] = x.IsoCode, x.Decimals
	v["symbol"] = nil
	if x.Symbol != nil {
		v["symbol"] = *x.Symbol
	}
	return v
}

func New(db *database.DB) *master.Service[Currency, Input] {
	return &master.Service[Currency, Input]{
		Spec: master.Spec[Currency, Input]{
			Menu: "pm.currency", Table: "currency", Noun: "Mata uang", Global: true, Import: importSpec(db),
			Base: func(x *Currency) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Currency, p record.Problems) (record.Problems, error) {
				if in.IsoCode == "" || p.Has("isoCode") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				other, err := refs.Duplicate(ctx, db, "currency", "iso_code", in.IsoCode, self, false)
				if err == nil && other != "" {
					p.Add("isoCode", fmt.Sprintf("Kode %s sudah dipakai mata uang %s.", in.IsoCode, other))
				}
				return p, err
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Global{DB: db, Prefix: "MMU", Digits: 3}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var cols = record.Columns + `, iso_code, symbol, decimals`

func scan(row pgx.Row) (Currency, error) {
	var x Currency
	return x, x.Scan(row, &x.IsoCode, &x.Symbol, &x.Decimals)
}

func (r repo) List(ctx context.Context) ([]Currency, error) {
	return master.Rows(ctx, r.db, scan, `SELECT `+cols+` FROM pm.currency WHERE NOT is_deleted ORDER BY iso_code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Currency, error) {
	q := `SELECT ` + cols + ` FROM pm.currency WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Currency, error) {
	return master.One(ctx, r.db, scan, `INSERT INTO pm.currency (code, name, description, is_active, valid_from, valid_to, created_by, iso_code, symbol, decimals)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10) RETURNING `+cols,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.IsoCode, in.Symbol, in.Decimals)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Currency, error) {
	return master.One(ctx, r.db, scan, `UPDATE pm.currency SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		iso_code = $7, symbol = $8, decimals = $9, updated_at = now(), updated_by = $10, version = version + 1
		WHERE id = $1 AND NOT is_deleted RETURNING `+cols, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.IsoCode, in.Symbol, in.Decimals, actor)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "currency", id, actor)
}
