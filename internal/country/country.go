// Package country is the master "Negara" (Geografi & Lokasi, menu pm.country): countries by their ISO 3166-1 codes
// (alpha-2 required, alpha-3 optional; unique per tenant beside the generated code), with the dialling code and
// the default currency.
package country

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

type Country struct {
	record.Base
	Iso2         string  `json:"iso2"`
	Iso3         *string `json:"iso3"`
	PhoneCode    *string `json:"phoneCode"`
	CurrencyID   *string `json:"currencyId"`
	CurrencyCode *string `json:"currencyCode"`
}

type Input struct {
	record.Input
	Iso2         string
	Iso3         *string
	PhoneCode    *string
	CurrencyCode *string
	CurrencyID   *string
}

var (
	iso2  = regexp.MustCompile(`^[A-Z]{2}$`)
	iso3  = regexp.MustCompile(`^[A-Z]{3}$`)
	phone = regexp.MustCompile(`^\+[0-9]{1,4}$`)
)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Iso2 = strings.ToUpper(record.RequiredText(v, "iso2", "Kode ISO (2 huruf)", 2, &p))
	if in.Iso2 != "" && !p.Has("iso2") && !iso2.MatchString(in.Iso2) {
		p.Add("iso2", "Kode ISO 3166 berisi 2 huruf, mis. ID.")
	}
	if in.Iso3 = record.OptionalText(v, "iso3", "Kode ISO (3 huruf)", 3, &p); in.Iso3 != nil && !p.Has("iso3") {
		u := strings.ToUpper(*in.Iso3)
		in.Iso3 = &u
		if !iso3.MatchString(u) {
			p.Add("iso3", "Kode ISO 3166 alpha-3 berisi 3 huruf, mis. IDN.")
		}
	}
	if in.PhoneCode = record.OptionalText(v, "phoneCode", "Kode telepon", 6, &p); in.PhoneCode != nil && !p.Has("phoneCode") && !phone.MatchString(*in.PhoneCode) {
		p.Add("phoneCode", "Kode telepon diawali + lalu 1–4 angka, mis. +62.")
	}
	in.CurrencyCode = record.OptionalText(v, "currencyCode", "Mata uang", record.MaxCode, &p)
	return in, p
}

func values(x Country) record.Values {
	v := record.ValuesOf(x.Base)
	v["iso2"], v["iso3"], v["phoneCode"], v["currencyCode"] = x.Iso2, opt(x.Iso3), opt(x.PhoneCode), opt(x.CurrencyCode)
	return v
}

func opt(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func New(db *database.DB) *master.Service[Country, Input] {
	return &master.Service[Country, Input]{
		Spec: master.Spec[Country, Input]{
			Menu: "pm.country", Table: "country", Noun: "Negara",
			Base: func(x *Country) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Country, p record.Problems) (record.Problems, error) {
				self := ""
				var kept *string
				if stored != nil {
					self, kept = stored.ID, stored.CurrencyID
				}
				for _, x := range []struct {
					field, col string
					val        *string
				}{{"iso2", "iso2", &in.Iso2}, {"iso3", "iso3", in.Iso3}} {
					if x.val == nil || *x.val == "" || p.Has(x.field) {
						continue
					}
					other, err := refs.Duplicate(ctx, db, "country", x.col, *x.val, self, false)
					if err != nil {
						return p, err
					}
					if other != "" {
						p.Add(x.field, fmt.Sprintf("Kode %s sudah dipakai negara %s.", *x.val, other))
					}
				}
				c, err := refs.Pick(ctx, db, "currency", "currencyCode", "Mata uang", in.CurrencyCode, kept, &p)
				if c != nil {
					in.CurrencyID = &c.ID
				}
				return p, err
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.iso2, x.iso3, x.phone_code, x.currency_id, c.code
	FROM pm.country x LEFT JOIN pm.currency c ON c.id = x.currency_id`

func scan(row pgx.Row) (Country, error) {
	var x Country
	return x, x.Scan(row, &x.Iso2, &x.Iso3, &x.PhoneCode, &x.CurrencyID, &x.CurrencyCode)
}

func (r repo) List(ctx context.Context) ([]Country, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY lower(x.name)`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Country, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Country, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Country, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.country (code, name, description, is_active, valid_from, valid_to, created_by, iso2, iso3, phone_code, currency_id)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Iso2, in.Iso3, in.PhoneCode, in.CurrencyID).Scan(&id); err != nil {
		return Country{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Country, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.country SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		iso2 = $7, iso3 = $8, phone_code = $9, currency_id = $10, updated_at = now(), updated_by = $11, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Iso2, in.Iso3, in.PhoneCode, in.CurrencyID, actor); err != nil {
		return Country{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "country", id, actor)
}
