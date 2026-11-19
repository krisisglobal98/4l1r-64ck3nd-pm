// Package chargecode is the master "Kode Biaya" (Keuangan & Pajak, menu pm.charge): ONE code per kind of charge, used
// on the billing side (customer invoice), the paying side (vendor bill) or both: ocean freight is sold and bought under
// the same code, so a job's margin compares like with like. A code has its group, the basis it is charged on, a default
// currency and rate, the PPN put on the invoice, the PPh withheld when paying a vendor, and, for a per-container
// charge, rates per container type.
//
// Rules: at least one side; PPN (added) only on the billing side, PPh (withheld) only on the paying side; a default
// rate needs its currency (a percentage none, at most 100); container rates only for the container basis, one per
// container type and currency.
package chargecode

import (
	"context"
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

var (
	Groups = []string{"freight", "origin", "destination", "customs", "trucking", "warehouse", "document", "insurance", "surcharge", "other"}
	Bases  = []string{"shipment", "container", "teu", "kg", "ton", "cbm", "wm", "document", "day", "percent"}
)

const maxRates = 30

type ContainerRate struct {
	ContainerTypeID   string  `json:"containerTypeId"`
	ContainerTypeCode string  `json:"containerTypeCode"`
	CurrencyID        string  `json:"currencyId"`
	CurrencyCode      string  `json:"currencyCode"`
	Rate              float64 `json:"rate"`
}

type Charge struct {
	record.Base
	Group          string          `json:"group"`
	Receivable     bool            `json:"receivable"`
	Payable        bool            `json:"payable"`
	Basis          string          `json:"basis"`
	CurrencyID     *string         `json:"currencyId"`
	CurrencyCode   *string         `json:"currencyCode"`
	DefaultRate    *float64        `json:"defaultRate"`
	PpnTaxTypeID   *string         `json:"ppnTaxTypeId"`
	PpnTaxTypeCode *string         `json:"ppnTaxTypeCode"`
	PphTaxTypeID   *string         `json:"pphTaxTypeId"`
	PphTaxTypeCode *string         `json:"pphTaxTypeCode"`
	AccountCode    *string         `json:"accountCode"`
	Rates          []ContainerRate `json:"rates"`
}

type Input struct {
	record.Input
	Group, Basis                            string
	Receivable, Payable                     bool
	CurrencyCode, PpnCode, PphCode, Account *string
	DefaultRate                             *float64
	Rates                                   []ContainerRate
	CurrencyID, PpnID, PphID                *string
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Group = record.Choice(v, "group", "Kelompok biaya", Groups, &p)
	in.Receivable = record.Bool(v, "receivable", "Ditagihkan ke customer", true, &p)
	in.Payable = record.Bool(v, "payable", "Dibayar ke vendor", true, &p)
	if !in.Receivable && !in.Payable && !p.Has("receivable") && !p.Has("payable") {
		p.Add("receivable", "Pilih minimal satu: ditagihkan ke customer atau dibayar ke vendor.")
	}
	in.Basis = record.Choice(v, "basis", "Dasar tarif", Bases, &p)
	in.CurrencyCode = record.OptionalText(v, "currencyCode", "Mata uang", record.MaxCode, &p)
	max := 99999999999999.0
	if in.Basis == "percent" {
		max = 100
	}
	in.DefaultRate = record.Decimal(v, "defaultRate", "Tarif dasar", 0, max, 4, false, &p)
	if in.DefaultRate != nil && in.CurrencyCode == nil && in.Basis != "percent" && !p.Has("currencyCode") {
		p.Add("currencyCode", "Tarif dasar perlu mata uang.")
	}
	if in.Receivable {
		in.PpnCode = record.OptionalText(v, "ppnTaxTypeCode", "PPN", record.MaxCode, &p)
	}
	if in.Payable {
		in.PphCode = record.OptionalText(v, "pphTaxTypeCode", "PPh", record.MaxCode, &p)
	}
	in.Account = record.OptionalText(v, "accountCode", "Kode akun", 30, &p)
	if in.Basis == "container" {
		in.Rates = parseRates(v, &p)
	}
	return in, p
}

func parseRates(v record.Values, p *record.Problems) []ContainerRate {
	rows := record.Rows(v, "rates", "Tarif", maxRates, p)
	out := make([]ContainerRate, 0, len(rows))
	seen := map[string]int{}
	for i, r := range rows {
		var q record.Problems
		x := ContainerRate{
			ContainerTypeCode: record.RequiredText(r, "containerTypeCode", "Jenis kontainer", record.MaxCode, &q),
			CurrencyCode:      record.RequiredText(r, "currencyCode", "Mata uang", record.MaxCode, &q),
		}
		if t := record.Decimal(r, "rate", "Tarif", 0, 99999999999999, 4, true, &q); t != nil {
			x.Rate = *t
		}
		key := strings.ToLower(x.ContainerTypeCode + "/" + x.CurrencyCode)
		if j, dup := seen[key]; dup && x.ContainerTypeCode != "" && x.CurrencyCode != "" {
			q.Add("containerTypeCode", fmt.Sprintf("Jenis kontainer dan mata uang ini sama dengan baris %d.", j+1))
		}
		seen[key] = i
		record.AddRow(p, "rates", "Tarif", i, q)
		out = append(out, x)
	}
	return out
}

func values(x Charge) record.Values {
	v := record.ValuesOf(x.Base)
	v["group"], v["receivable"], v["payable"], v["basis"] = x.Group, x.Receivable, x.Payable, x.Basis
	v["currencyCode"], v["defaultRate"], v["ppnTaxTypeCode"], v["pphTaxTypeCode"], v["accountCode"] = record.Opt(x.CurrencyCode), record.Opt(x.DefaultRate), record.Opt(x.PpnTaxTypeCode), record.Opt(x.PphTaxTypeCode), record.Opt(x.AccountCode)
	var rows []any
	for _, r := range x.Rates {
		rows = append(rows, map[string]any{"containerTypeCode": r.ContainerTypeCode, "currencyCode": r.CurrencyCode, "rate": r.Rate})
	}
	v["rates"] = rows
	return v
}

func New(db *database.DB) *master.Service[Charge, Input] {
	return &master.Service[Charge, Input]{
		Spec: master.Spec[Charge, Input]{
			Menu: "pm.charge", Table: "charge_code", Noun: "Kode biaya",
			Base: func(x *Charge) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Charge, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func check(ctx context.Context, db *database.DB, in *Input, stored *Charge, p record.Problems) (record.Problems, error) {
	kept := map[string]bool{}
	if stored != nil {
		for _, id := range []*string{stored.CurrencyID, stored.PpnTaxTypeID, stored.PphTaxTypeID} {
			if id != nil {
				kept[*id] = true
			}
		}
		for _, r := range stored.Rates {
			kept[r.ContainerTypeID], kept[r.CurrencyID] = true, true
		}
	}
	var err error
	pick := func(table, field, label string, code *string) *refs.Ref {
		if err != nil || code == nil || p.Has(field) {
			return nil
		}
		var keep *string
		if r, e := refs.Lookup(ctx, db, table, *code); e == nil && kept[r.ID] {
			keep = &r.ID
		}
		var r *refs.Ref
		r, err = refs.Pick(ctx, db, table, field, label, code, keep, &p)
		return r
	}
	if c := pick("currency", "currencyCode", "Mata uang", in.CurrencyCode); c != nil {
		in.CurrencyID = &c.ID
	}
	for _, t := range []struct {
		code  *string
		field string
		dst   **string
		ppn   bool
	}{{in.PpnCode, "ppnTaxTypeCode", &in.PpnID, true}, {in.PphCode, "pphTaxTypeCode", &in.PphID, false}} {
		r := pick("tax_type", t.field, "Jenis pajak", t.code)
		if r == nil || err != nil {
			continue
		}
		var cat, method string
		if err = db.Q(ctx).QueryRow(ctx, `SELECT category, method FROM pm.tax_type WHERE id = $1`, r.ID).Scan(&cat, &method); err != nil {
			break
		}
		switch {
		case t.ppn && !((cat == "PPN" || cat == "PPNBM") && method == "added"):
			p.Add(t.field, fmt.Sprintf("%s bukan PPN yang ditambahkan ke tagihan.", r.Name))
		case !t.ppn && !(strings.HasPrefix(cat, "PPH") && method == "withheld"):
			p.Add(t.field, fmt.Sprintf("%s bukan PPh yang dipotong.", r.Name))
		default:
			*t.dst = &r.ID
		}
	}
	for i := range in.Rates {
		x := &in.Rates[i]
		if c := pick("container_type", record.RowField("rates", i, "containerTypeCode"), "Jenis kontainer", nonEmpty(x.ContainerTypeCode)); c != nil {
			x.ContainerTypeID = c.ID
		}
		if c := pick("currency", record.RowField("rates", i, "currencyCode"), "Mata uang", nonEmpty(x.CurrencyCode)); c != nil {
			x.CurrencyID = c.ID
		}
	}
	return p, err
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.charge_group, x.is_receivable, x.is_payable, x.basis, x.currency_id, c.code, x.default_rate::float8,
	x.ppn_tax_type_id, n.code, x.pph_tax_type_id, h.code, x.account_code
	FROM pm.charge_code x LEFT JOIN pm.currency c ON c.id = x.currency_id
	LEFT JOIN pm.tax_type n ON n.id = x.ppn_tax_type_id LEFT JOIN pm.tax_type h ON h.id = x.pph_tax_type_id`

func scan(row pgx.Row) (Charge, error) {
	var x Charge
	return x, x.Scan(row, &x.Group, &x.Receivable, &x.Payable, &x.Basis, &x.CurrencyID, &x.CurrencyCode, &x.DefaultRate,
		&x.PpnTaxTypeID, &x.PpnTaxTypeCode, &x.PphTaxTypeID, &x.PphTaxTypeCode, &x.AccountCode)
}

func (r repo) withRates(ctx context.Context, list []Charge) ([]Charge, error) {
	if len(list) == 0 {
		return list, nil
	}
	ids := make([]string, len(list))
	at := map[string]int{}
	for i := range list {
		ids[i], at[list[i].ID] = list[i].ID, i
		list[i].Rates = []ContainerRate{}
	}
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT x.charge_code_id, x.container_type_id, t.code, x.currency_id, c.code, x.rate::float8
		FROM pm.charge_code_rate x JOIN pm.container_type t ON t.id = x.container_type_id JOIN pm.currency c ON c.id = x.currency_id
		WHERE x.charge_code_id = ANY($1::uuid[]) ORDER BY x.charge_code_id, x.seq`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var x ContainerRate
		if err := rows.Scan(&id, &x.ContainerTypeID, &x.ContainerTypeCode, &x.CurrencyID, &x.CurrencyCode, &x.Rate); err != nil {
			return nil, err
		}
		list[at[id]].Rates = append(list[at[id]].Rates, x)
	}
	return list, rows.Err()
}

func (r repo) one(ctx context.Context, q string, arg any) (Charge, error) {
	x, err := master.One(ctx, r.db, scan, q, arg)
	if err != nil {
		return x, err
	}
	list, err := r.withRates(ctx, []Charge{x})
	if err != nil {
		return x, err
	}
	return list[0], nil
}

func (r repo) List(ctx context.Context) ([]Charge, error) {
	list, err := master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.code`)
	if err != nil {
		return nil, err
	}
	return r.withRates(ctx, list)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Charge, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return r.one(ctx, q, code)
}

func (r repo) saveRates(ctx context.Context, id string, rates []ContainerRate) error {
	if _, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM pm.charge_code_rate WHERE charge_code_id = $1`, id); err != nil {
		return err
	}
	for i, x := range rates {
		if _, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO pm.charge_code_rate (charge_code_id, seq, container_type_id, currency_id, rate) VALUES ($1, $2, $3, $4, $5)`,
			id, i+1, x.ContainerTypeID, x.CurrencyID, x.Rate); err != nil {
			return err
		}
	}
	return nil
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Charge, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.charge_code (code, name, description, is_active, valid_from, valid_to, created_by,
		charge_group, is_receivable, is_payable, basis, currency_id, default_rate, ppn_tax_type_id, pph_tax_type_id, account_code)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor,
		in.Group, in.Receivable, in.Payable, in.Basis, in.CurrencyID, in.DefaultRate, in.PpnID, in.PphID, in.Account).Scan(&id); err != nil {
		return Charge{}, err
	}
	if err := r.saveRates(ctx, id, in.Rates); err != nil {
		return Charge{}, err
	}
	return r.one(ctx, sel+` WHERE x.id = $1`, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Charge, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.charge_code SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		charge_group = $7, is_receivable = $8, is_payable = $9, basis = $10, currency_id = $11, default_rate = $12, ppn_tax_type_id = $13,
		pph_tax_type_id = $14, account_code = $15, updated_at = now(), updated_by = $16, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo,
		in.Group, in.Receivable, in.Payable, in.Basis, in.CurrencyID, in.DefaultRate, in.PpnID, in.PphID, in.Account, actor); err != nil {
		return Charge{}, err
	}
	if err := r.saveRates(ctx, id, in.Rates); err != nil {
		return Charge{}, err
	}
	return r.one(ctx, sel+` WHERE x.id = $1`, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "charge_code", id, actor)
}
