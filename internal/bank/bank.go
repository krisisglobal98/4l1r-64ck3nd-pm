// Package bank is the master "Bank" (Keuangan & Pajak, menu pm.bank): banks for payroll, claims and the payments of
// customers and vendors, with the clearing code (domestic) and SWIFT/BIC (international) and the bank's country.
package bank

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

type Bank struct {
	record.Base
	Clearing    *string `json:"clearing"`  // clearing code, 3–7 digits
	SwiftCode   *string `json:"swiftCode"` // SWIFT/BIC, 8 or 11 characters
	CountryID   *string `json:"countryId"`
	CountryCode *string `json:"countryCode"`
}

type Input struct {
	record.Input
	Clearing    *string
	SwiftCode   *string
	CountryCode *string
	CountryID   *string
}

var (
	digits = regexp.MustCompile(`^[0-9]{3,7}$`)
	swift  = regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`)
)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Clearing = record.OptionalText(v, "clearing", "Kode kliring", 10, &p)
	if in.Clearing != nil && !p.Has("clearing") && !digits.MatchString(*in.Clearing) {
		p.Add("clearing", "Kode kliring berisi 3–7 angka, mis. 002.")
	}
	if in.SwiftCode = record.OptionalText(v, "swiftCode", "Kode SWIFT/BIC", 11, &p); in.SwiftCode != nil && !p.Has("swiftCode") {
		u := strings.ToUpper(*in.SwiftCode)
		in.SwiftCode = &u
		if !swift.MatchString(u) {
			p.Add("swiftCode", "Kode SWIFT/BIC berisi 8 atau 11 karakter, mis. BRINIDJA.")
		}
	}
	in.CountryCode = record.OptionalText(v, "countryCode", "Negara", record.MaxCode, &p)
	return in, p
}

func values(x Bank) record.Values {
	v := record.ValuesOf(x.Base)
	for k, x := range map[string]*string{"clearing": x.Clearing, "swiftCode": x.SwiftCode, "countryCode": x.CountryCode} {
		v[k] = nil
		if x != nil {
			v[k] = *x
		}
	}
	return v
}

func New(db *database.DB) *master.Service[Bank, Input] {
	return &master.Service[Bank, Input]{
		Spec: master.Spec[Bank, Input]{
			Menu: "pm.bank", Table: "bank", Noun: "Bank", Import: importSpec(db),
			Base: func(x *Bank) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Bank, p record.Problems) (record.Problems, error) {
				self := ""
				var kept *string
				if stored != nil {
					self, kept = stored.ID, stored.CountryID
				}
				if in.SwiftCode != nil && !p.Has("swiftCode") {
					other, err := refs.Duplicate(ctx, db, "bank", "swift_code", *in.SwiftCode, self, false)
					if err != nil {
						return p, err
					}
					if other != "" {
						p.Add("swiftCode", fmt.Sprintf("Kode SWIFT %s sudah dipakai bank %s.", *in.SwiftCode, other))
					}
				}
				c, err := refs.Pick(ctx, db, "country", "countryCode", "Negara", in.CountryCode, kept, &p)
				if c != nil {
					in.CountryID = &c.ID
				}
				return p, err
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.clearing_code, x.swift_code, x.country_id, c.code FROM pm.bank x LEFT JOIN pm.country c ON c.id = x.country_id`

func scan(row pgx.Row) (Bank, error) {
	var x Bank
	return x, x.Scan(row, &x.Clearing, &x.SwiftCode, &x.CountryID, &x.CountryCode)
}

func (r repo) List(ctx context.Context) ([]Bank, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Bank, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Bank, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Bank, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.bank (code, name, description, is_active, valid_from, valid_to, created_by, clearing_code, swift_code, country_id)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Clearing, in.SwiftCode, in.CountryID).Scan(&id); err != nil {
		return Bank{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Bank, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.bank SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, clearing_code = $7, swift_code = $8, country_id = $9, updated_at = now(), updated_by = $10, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Clearing, in.SwiftCode, in.CountryID, actor); err != nil {
		return Bank{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "bank", id, actor)
}
