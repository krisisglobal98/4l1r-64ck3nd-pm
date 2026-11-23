// Package coa is the master "Bagan Akun (COA)" (Keuangan & Pajak, menu pm.account): the accounts the Finance module
// journals to. As accounting applications (Accurate, Zahir) keep them: the company's own account number, a name, an
// account type that fixes the normal balance, header accounts grouping detail accounts of the same type, and an
// optional currency for a foreign-currency account. Kode Biaya points to a revenue and a cost account here.
package coa

import (
	"context"
	"errors"
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

// Types are the account types, in the order of a balance sheet then an income statement.
var Types = []string{"cash_bank", "receivable", "inventory", "other_current_asset", "fixed_asset", "accumulated_depreciation", "other_asset",
	"payable", "other_current_liability", "long_term_liability", "equity", "revenue", "cogs", "expense", "other_income", "other_expense"}

// TypeLabels name the types as the forms and the Excel template show them.
var TypeLabels = map[string]string{
	"cash_bank": "Kas & Bank", "receivable": "Piutang Usaha", "inventory": "Persediaan", "other_current_asset": "Aset Lancar Lainnya",
	"fixed_asset": "Aset Tetap", "accumulated_depreciation": "Akumulasi Penyusutan", "other_asset": "Aset Lainnya", "payable": "Utang Usaha",
	"other_current_liability": "Utang Lancar Lainnya", "long_term_liability": "Utang Jangka Panjang", "equity": "Ekuitas", "revenue": "Pendapatan",
	"cogs": "Harga Pokok Penjualan", "expense": "Beban", "other_income": "Pendapatan Lain-lain", "other_expense": "Beban Lain-lain",
}

// NormalBalance is debit for assets, cost of sales and expenses; credit otherwise (accumulated depreciation too).
func NormalBalance(t string) string {
	switch t {
	case "cash_bank", "receivable", "inventory", "other_current_asset", "fixed_asset", "other_asset", "cogs", "expense", "other_expense":
		return "debit"
	}
	return "credit"
}

// Revenue / Cost are the types a Kode Biaya's revenue and cost accounts may have.
var (
	Revenue = []string{"revenue", "other_income"}
	Cost    = []string{"cogs", "expense", "other_expense"}
)

type Account struct {
	record.Base
	AccountNo     string  `json:"accountNo"`
	AccountType   string  `json:"accountType"`
	NormalBalance string  `json:"normalBalance"`
	Header        bool    `json:"header"`
	ParentID      *string `json:"parentId"`
	ParentCode    *string `json:"parentCode"`
	CurrencyID    *string `json:"currencyId"`
	CurrencyCode  *string `json:"currencyCode"`
}

type Input struct {
	record.Input
	AccountNo, AccountType   string
	Header                   bool
	ParentCode, CurrencyCode *string
	ParentID, CurrencyID     *string
}

var accountNo = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.\-]*$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.AccountNo = strings.TrimSpace(record.RequiredText(v, "accountNo", "No. akun", 30, &p))
	if in.AccountNo != "" && !accountNo.MatchString(in.AccountNo) {
		p.Add("accountNo", "No. akun berisi huruf, angka, titik, atau tanda hubung, mis. 4-1100 atau 410100.")
	}
	in.AccountType = record.Choice(v, "accountType", "Tipe akun", Types, &p)
	in.Header = record.Bool(v, "header", "Akun induk", false, &p)
	in.ParentCode = record.OptionalText(v, "parentCode", "Akun induk", record.MaxCode, &p)
	in.CurrencyCode = record.OptionalText(v, "currencyCode", "Mata uang", record.MaxCode, &p)
	return in, p
}

func values(x Account) record.Values {
	v := record.ValuesOf(x.Base)
	v["accountNo"], v["accountType"], v["header"] = x.AccountNo, x.AccountType, x.Header
	v["parentCode"], v["currencyCode"] = record.Opt(x.ParentCode), record.Opt(x.CurrencyCode)
	return v
}

func New(db *database.DB) *master.Service[Account, Input] {
	return &master.Service[Account, Input]{
		Spec: master.Spec[Account, Input]{
			Menu: "pm.account", Table: "account", Noun: "Akun", Import: importSpec(db),
			Base: func(x *Account) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Account, p record.Problems) (record.Problems, error) {
				self := ""
				var keptParent, keptCur *string
				if stored != nil {
					self, keptParent, keptCur = stored.ID, stored.ParentID, stored.CurrencyID
				}
				if in.AccountNo != "" && !p.Has("accountNo") {
					var other string
					switch err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.account WHERE lower(account_no) = lower($1) AND NOT is_deleted AND id::text <> $2`, in.AccountNo, self).Scan(&other); {
					case err == nil:
						p.Add("accountNo", fmt.Sprintf("No. akun %s sudah dipakai akun %s.", in.AccountNo, other))
					case !errors.Is(err, pgx.ErrNoRows):
						return p, err
					}
				}
				par, err := refs.Pick(ctx, db, "account", "parentCode", "Akun induk", in.ParentCode, keptParent, &p)
				if err != nil {
					return p, err
				}
				if par != nil {
					var ptype string
					var header bool
					if err := db.Q(ctx).QueryRow(ctx, `SELECT account_type, is_header FROM pm.account WHERE id = $1`, par.ID).Scan(&ptype, &header); err != nil {
						return p, err
					}
					switch {
					case par.ID == self:
						p.Add("parentCode", "Akun tidak dapat menjadi induk dirinya sendiri.")
					case !header:
						p.Add("parentCode", fmt.Sprintf("%s bukan akun induk (header).", par.Name))
					case ptype != in.AccountType && !p.Has("accountType"):
						p.Add("parentCode", fmt.Sprintf("Akun induk bertipe %s; akun di bawahnya harus bertipe sama.", TypeLabels[ptype]))
					default:
						in.ParentID = &par.ID
					}
				}
				if stored != nil && stored.Header && !in.Header {
					var n int
					if err := db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM pm.account WHERE parent_id = $1 AND NOT is_deleted`, stored.ID).Scan(&n); err != nil {
						return p, err
					}
					if n > 0 {
						p.Add("header", fmt.Sprintf("Masih ada %d akun di bawahnya; akun ini tetap akun induk.", n))
					}
				}
				if stored != nil && stored.AccountType != in.AccountType && !p.Has("accountType") {
					var n int
					if err := db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM pm.account WHERE parent_id = $1 AND NOT is_deleted`, stored.ID).Scan(&n); err != nil {
						return p, err
					}
					if n > 0 {
						p.Add("accountType", "Tipe akun induk yang sudah punya akun di bawahnya tidak dapat diubah.")
					}
				}
				cur, err := refs.Pick(ctx, db, "currency", "currencyCode", "Mata uang", in.CurrencyCode, keptCur, &p)
				if err != nil {
					return p, err
				}
				if cur != nil {
					in.CurrencyID = &cur.ID
				}
				return p, nil
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.account_no, x.account_type, x.normal_balance, x.is_header, x.parent_id, pa.code, x.currency_id, c.code
	FROM pm.account x LEFT JOIN pm.account pa ON pa.id = x.parent_id LEFT JOIN pm.currency c ON c.id = x.currency_id`

func scan(row pgx.Row) (Account, error) {
	var x Account
	return x, x.Scan(row, &x.AccountNo, &x.AccountType, &x.NormalBalance, &x.Header, &x.ParentID, &x.ParentCode, &x.CurrencyID, &x.CurrencyCode)
}

func (r repo) List(ctx context.Context) ([]Account, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.account_no`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Account, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Account, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Account, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.account (code, name, description, is_active, valid_from, valid_to, created_by,
		account_no, account_type, normal_balance, is_header, parent_id, currency_id) VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.AccountNo, in.AccountType, NormalBalance(in.AccountType), in.Header,
		in.ParentID, in.CurrencyID).Scan(&id); err != nil {
		return Account{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Account, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.account SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		account_no = $7, account_type = $8, normal_balance = $9, is_header = $10, parent_id = $11, currency_id = $12, updated_at = now(), updated_by = $13,
		version = version + 1 WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.AccountNo, in.AccountType,
		NormalBalance(in.AccountType), in.Header, in.ParentID, in.CurrencyID, actor); err != nil {
		return Account{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "account", id, actor)
}

// Pick resolves an account reference of another master (a Kode Biaya's revenue or cost account): it must exist, be a
// detail account (not a header), and be of one of the types.
func Pick(ctx context.Context, db *database.DB, field, label string, code, kept *string, types []string, p *record.Problems) (*refs.Ref, error) {
	r, err := refs.Pick(ctx, db, "account", field, label, code, kept, p)
	if err != nil || r == nil {
		return r, err
	}
	var t string
	var header bool
	if err := db.Q(ctx).QueryRow(ctx, `SELECT account_type, is_header FROM pm.account WHERE id = $1`, r.ID).Scan(&t, &header); err != nil {
		return nil, err
	}
	ok := false
	for _, x := range types {
		ok = ok || x == t
	}
	names := make([]string, len(types))
	for i, x := range types {
		names[i] = TypeLabels[x]
	}
	switch {
	case header:
		p.Add(field, fmt.Sprintf("%s adalah akun induk; pilih akun di bawahnya.", r.Name))
		return nil, nil
	case !ok:
		p.Add(field, fmt.Sprintf("%s bertipe %s; %s harus bertipe %s.", r.Name, TypeLabels[t], strings.ToLower(label), strings.Join(names, " atau ")))
		return nil, nil
	}
	return r, nil
}
