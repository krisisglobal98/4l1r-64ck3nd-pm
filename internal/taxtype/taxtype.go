// Package taxtype is the master "Jenis Pajak" (Keuangan & Pajak, menu pm.taxType): PPN and PPh kinds with their
// tariff and the share of the amount that is taxed (DPP), valid for a period (valid_from–valid_to, as tariffs change;
// a transaction uses the one valid on its date). PPN/PPnBM are added on top; PPh is withheld by the payer. ALIR keeps
// the rates the company's tax team sets; it does not decide them.
package taxtype

import (
	"context"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
)

// Categories of tax.
var Categories = []string{"PPN", "PPNBM", "PPH21", "PPH22", "PPH23", "PPH4_2", "PPH15", "PPH26", "OTHER"}

type TaxType struct {
	record.Base
	Category     string  `json:"category"`
	RatePct      float64 `json:"ratePct"`
	BasePct      float64 `json:"basePct"`
	EffectivePct float64 `json:"effectivePct"` // rate × base share: what is charged on the full amount
	Method       string  `json:"method"`       // added | withheld
	TaxCode      *string `json:"taxCode"`
}

type Input struct {
	record.Input
	Category string
	RatePct  float64
	BasePct  float64
	Method   string
	TaxCode  *string
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Category = strings.ToUpper(record.Choice(v, "category", "Kategori pajak", Categories, &p))
	if r := record.Decimal(v, "ratePct", "Tarif (%)", 0, 100, 4, true, &p); r != nil {
		in.RatePct = *r
	}
	in.BasePct = 100
	if b := record.Decimal(v, "basePct", "Dasar pengenaan (%)", 0.0001, 100, 4, false, &p); b != nil {
		in.BasePct = *b
	}
	def := "withheld"
	if in.Category == "PPN" || in.Category == "PPNBM" {
		def = "added"
	}
	in.Method = record.ChoiceOr(v, "method", "Cara", []string{"added", "withheld"}, def, &p)
	if !p.Has("method") && !p.Has("category") {
		switch {
		case (in.Category == "PPN" || in.Category == "PPNBM") && in.Method != "added":
			p.Add("method", "PPN dan PPnBM ditambahkan ke tagihan, bukan dipotong.")
		case strings.HasPrefix(in.Category, "PPH") && in.Method != "withheld":
			p.Add("method", "PPh dipotong oleh pembayar, bukan ditambahkan.")
		}
	}
	in.TaxCode = record.OptionalText(v, "taxCode", "Kode pajak", 20, &p)
	return in, p
}

func values(x TaxType) record.Values {
	v := record.ValuesOf(x.Base)
	v["category"], v["ratePct"], v["basePct"], v["method"], v["taxCode"] = x.Category, x.RatePct, x.BasePct, x.Method, nil
	if x.TaxCode != nil {
		v["taxCode"] = *x.TaxCode
	}
	return v
}

func New(db *database.DB) *master.Service[TaxType, Input] {
	return &master.Service[TaxType, Input]{
		Spec: master.Spec[TaxType, Input]{
			Menu: "pm.taxType", Table: "tax_type", Noun: "Jenis pajak", Import: importSpec(db),
			Base: func(x *TaxType) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var cols = record.Columns + `, category, rate_pct, base_pct, method, tax_code`

func scan(row pgx.Row) (TaxType, error) {
	var x TaxType
	err := x.Scan(row, &x.Category, &x.RatePct, &x.BasePct, &x.Method, &x.TaxCode)
	x.EffectivePct = math.Round(x.RatePct*x.BasePct/100*10000) / 10000
	return x, err
}

func (r repo) List(ctx context.Context) ([]TaxType, error) {
	return master.Rows(ctx, r.db, scan, `SELECT `+cols+` FROM pm.tax_type WHERE NOT is_deleted ORDER BY category, valid_from NULLS FIRST, code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (TaxType, error) {
	q := `SELECT ` + cols + ` FROM pm.tax_type WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (TaxType, error) {
	return master.One(ctx, r.db, scan, `INSERT INTO pm.tax_type (code, name, description, is_active, valid_from, valid_to, created_by, category, rate_pct, base_pct, method, tax_code)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12) RETURNING `+cols,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Category, in.RatePct, in.BasePct, in.Method, in.TaxCode)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (TaxType, error) {
	return master.One(ctx, r.db, scan, `UPDATE pm.tax_type SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		category = $7, rate_pct = $8, base_pct = $9, method = $10, tax_code = $11, updated_at = now(), updated_by = $12, version = version + 1
		WHERE id = $1 AND NOT is_deleted RETURNING `+cols, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Category, in.RatePct, in.BasePct, in.Method, in.TaxCode, actor)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "tax_type", id, actor)
}
