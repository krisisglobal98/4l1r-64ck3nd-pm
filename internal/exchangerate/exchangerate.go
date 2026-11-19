// Package exchangerate is the master "Kurs" (Keuangan & Pajak, menu pm.exchangeRate): the rate of one currency in
// another, by type (tax: the weekly KMK rate for tax documents; BI middle rate; bank buy/sell; internal), valid from a
// date. Rates are history: a new rate is a new record from its date, and the rate on a date is the latest one of the
// pair and type valid then (GET …/lookup). The target currency defaults to the rupiah; the name is made from the pair,
// type and date.
package exchangerate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/httpx"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// Types of rate and their names.
var Types = []string{"tax", "bi_middle", "bank_buy", "bank_sell", "internal"}

var typeName = map[string]string{"tax": "Kurs pajak", "bi_middle": "Kurs tengah BI", "bank_buy": "Kurs beli bank", "bank_sell": "Kurs jual bank", "internal": "Kurs internal"}

const menu = "pm.exchangeRate"

type Rate struct {
	record.Base
	FromCurrencyID   string  `json:"fromCurrencyId"`
	FromCurrencyCode string  `json:"fromCurrencyCode"`
	FromIso          string  `json:"fromIso"`
	ToCurrencyID     string  `json:"toCurrencyId"`
	ToCurrencyCode   string  `json:"toCurrencyCode"`
	ToIso            string  `json:"toIso"`
	RateType         string  `json:"rateType"`
	Rate             float64 `json:"rate"`
	Source           *string `json:"source"`
}

type Input struct {
	record.Input
	FromCode string
	ToCode   *string
	RateType string
	Rate     float64
	Source   *string
	FromID   string
	ToID     string
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	if s, _ := record.Text(v["name"]); s == "" {
		// The name is made from the pair, type and date (check); the form does not ask for it.
		v = cloneWith(v, "name", "-")
	}
	in := Input{Input: record.ParseInput(v, &p)}
	in.FromCode = record.RequiredText(v, "fromCurrencyCode", "Mata uang asal", record.MaxCode, &p)
	in.ToCode = record.OptionalText(v, "toCurrencyCode", "Mata uang tujuan", record.MaxCode, &p)
	in.RateType = record.ChoiceOr(v, "rateType", "Jenis kurs", Types, "tax", &p)
	if r := record.Decimal(v, "rate", "Kurs", 0.00000001, 999999999999, 8, true, &p); r != nil {
		if *r <= 0 {
			p.Add("rate", "Kurs harus lebih dari 0.")
		}
		in.Rate = *r
	}
	in.Source = record.OptionalText(v, "source", "Sumber", 100, &p)
	if in.ValidFrom == nil && !p.Has("validFrom") {
		p.Add("validFrom", "Tanggal berlaku kurs wajib diisi.")
	}
	return in, p
}

func cloneWith(v record.Values, k string, x any) record.Values {
	out := record.Values{}
	for a, b := range v {
		out[a] = b
	}
	out[k] = x
	return out
}

func values(x Rate) record.Values {
	v := record.ValuesOf(x.Base)
	v["fromCurrencyCode"], v["toCurrencyCode"], v["rateType"], v["rate"], v["source"] = x.FromCurrencyCode, x.ToCurrencyCode, x.RateType, x.Rate, record.Opt(x.Source)
	return v
}

func New(db *database.DB) *master.Service[Rate, Input] {
	return &master.Service[Rate, Input]{
		Spec: master.Spec[Rate, Input]{
			Menu: menu, Table: "exchange_rate", Noun: "Kurs",
			Base: func(x *Rate) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Rate, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func check(ctx context.Context, db *database.DB, in *Input, stored *Rate, p record.Problems) (record.Problems, error) {
	self := ""
	var keptFrom, keptTo *string
	if stored != nil {
		self, keptFrom, keptTo = stored.ID, &stored.FromCurrencyID, &stored.ToCurrencyID
	}
	var from *refs.Ref
	var err error
	if in.FromCode != "" {
		if from, err = refs.Pick(ctx, db, "currency", "fromCurrencyCode", "Mata uang", &in.FromCode, keptFrom, &p); err != nil {
			return p, err
		}
	}
	to := in.ToCode
	if to == nil && !p.Has("toCurrencyCode") {
		var idr string
		switch err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.currency WHERE iso_code = 'IDR' AND NOT is_deleted`).Scan(&idr); {
		case errors.Is(err, pgx.ErrNoRows):
			p.Add("toCurrencyCode", "Mata uang tujuan wajib diisi (rupiah tidak ada di master Mata Uang).")
		case err != nil:
			return p, err
		default:
			to = &idr
		}
	}
	target, err := refs.Pick(ctx, db, "currency", "toCurrencyCode", "Mata uang", to, keptTo, &p)
	if err != nil || from == nil || target == nil {
		return p, err
	}
	if from.ID == target.ID {
		p.Add("toCurrencyCode", "Mata uang tujuan harus beda dari mata uang asal.")
		return p, nil
	}
	in.FromID, in.ToID = from.ID, target.ID
	var fromIso, toIso string
	if err := db.Q(ctx).QueryRow(ctx, `SELECT (SELECT iso_code FROM pm.currency WHERE id = $1), (SELECT iso_code FROM pm.currency WHERE id = $2)`, from.ID, target.ID).Scan(&fromIso, &toIso); err != nil {
		return p, err
	}
	if in.ValidFrom != nil {
		in.Name = fmt.Sprintf("%s/%s %s %s", fromIso, toIso, typeName[in.RateType], *in.ValidFrom)
		var other string
		err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.exchange_rate WHERE from_currency_id = $1 AND to_currency_id = $2 AND rate_type = $3
			AND valid_from = $4::date AND NOT is_deleted AND id::text <> $5`, from.ID, target.ID, in.RateType, *in.ValidFrom, self).Scan(&other)
		switch {
		case err == nil:
			p.Add("validFrom", fmt.Sprintf("%s %s/%s mulai %s sudah ada (%s); ubah yang itu.", typeName[in.RateType], fromIso, toIso, *in.ValidFrom, other))
		case !errors.Is(err, pgx.ErrNoRows):
			return p, err
		}
	}
	return p, nil
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.from_currency_id, f.code, f.iso_code, x.to_currency_id, t.code, t.iso_code, x.rate_type, x.rate::float8, x.source
	FROM pm.exchange_rate x JOIN pm.currency f ON f.id = x.from_currency_id JOIN pm.currency t ON t.id = x.to_currency_id`

func scan(row pgx.Row) (Rate, error) {
	var x Rate
	return x, x.Scan(row, &x.FromCurrencyID, &x.FromCurrencyCode, &x.FromIso, &x.ToCurrencyID, &x.ToCurrencyCode, &x.ToIso, &x.RateType, &x.Rate, &x.Source)
}

func (r repo) List(ctx context.Context) ([]Rate, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.valid_from DESC, f.iso_code, x.rate_type`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Rate, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Rate, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Rate, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.exchange_rate (code, name, description, is_active, valid_from, valid_to, created_by,
		from_currency_id, to_currency_id, rate_type, rate, source) VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.FromID, in.ToID, in.RateType, in.Rate, in.Source).Scan(&id); err != nil {
		return Rate{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Rate, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.exchange_rate SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		from_currency_id = $7, to_currency_id = $8, rate_type = $9, rate = $10, source = $11, updated_at = now(), updated_by = $12, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.FromID, in.ToID, in.RateType, in.Rate, in.Source, actor); err != nil {
		return Rate{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "exchange_rate", id, actor)
}

// On is the rate of a pair and type on a date: the latest active one valid from that date or before (and not ended).
// from and to are currency codes or ISO codes; to defaults to IDR and type to tax.
func On(ctx context.Context, db *database.DB, from, to, rateType, date string) (Rate, error) {
	return master.One(ctx, db, scan, sel+` WHERE NOT x.is_deleted AND x.is_active AND x.rate_type = $3
		AND (lower(f.code) = lower($1) OR f.iso_code = upper($1)) AND (lower(t.code) = lower($2) OR t.iso_code = upper($2))
		AND x.valid_from <= $4::date AND (x.valid_to IS NULL OR x.valid_to >= $4::date)
		ORDER BY x.valid_from DESC LIMIT 1`, from, to, rateType, date)
}

// Routes registers the master and GET <base>/lookup?from=USD&to=IDR&type=tax&date=YYYY-MM-DD (date: today in UTC).
func Routes(mux *http.ServeMux, base string, db *database.DB) {
	mux.Handle("GET "+base+"/lookup", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if _, err := access.Require(r.Context(), menu, "read"); err != nil {
			return err
		}
		q := r.URL.Query()
		from, to, typ, date := q.Get("from"), q.Get("to"), q.Get("type"), q.Get("date")
		if to == "" {
			to = "IDR"
		}
		if typ == "" {
			typ = "tax"
		}
		if date == "" {
			date = time.Now().UTC().Format(time.DateOnly)
		}
		var p record.Problems
		if from == "" || !record.LookupKey(from) {
			p.Add("from", "Mata uang asal wajib diisi, mis. from=USD.")
		}
		if !record.LookupKey(to) {
			p.Add("to", "Mata uang tujuan tidak sah.")
		}
		if _, ok := typeName[typ]; !ok {
			p.Add("type", "Jenis kurs: tax, bi_middle, bank_buy, bank_sell atau internal.")
		}
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			p.Add("date", "Tanggal berformat YYYY-MM-DD.")
		}
		if len(p) > 0 {
			return apperr.Validation(p)
		}
		rate, err := On(r.Context(), db, from, to, typ, date)
		if errors.Is(err, master.ErrNotFound) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, rate)
	}))
	master.Routes(mux, base, New(db))
}
