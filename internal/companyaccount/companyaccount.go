// Package companyaccount is the master "Rekening Bank Perusahaan" (Keuangan & Pajak, menu pm.companyBankAccount): the
// company's own bank accounts, per owning company unit, with their purpose. One account per currency and unit is the
// invoice default, printed on customer invoices in that currency. One live record per bank and account number.
package companyaccount

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

var Purposes = []string{"operational", "receipt", "payment", "payroll", "tax"}

type Account struct {
	record.Base
	BankID         string  `json:"bankId"`
	BankCode       string  `json:"bankCode"`
	AccountNumber  string  `json:"accountNumber"`
	AccountName    string  `json:"accountName"`
	CurrencyID     string  `json:"currencyId"`
	CurrencyCode   string  `json:"currencyCode"`
	Branch         *string `json:"branch"`
	UnitID         *string `json:"unitId"`
	UnitCode       *string `json:"unitCode"`
	Purpose        string  `json:"purpose"`
	InvoiceDefault bool    `json:"invoiceDefault"`
	AccountCode    *string `json:"accountCode"`
}

type Input struct {
	record.Input
	BankCode, AccountNumber, AccountName, CurrencyCode, Purpose string
	Branch, UnitCode, AccountCode                               *string
	InvoiceDefault                                              bool
	BankID, CurrencyID                                          string
	UnitID                                                      *string
}

var number = regexp.MustCompile(`^[A-Z0-9-]{4,34}$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.BankCode = record.RequiredText(v, "bankCode", "Bank", record.MaxCode, &p)
	if n := record.RequiredText(v, "accountNumber", "Nomor rekening", 40, &p); n != "" {
		in.AccountNumber = strings.ToUpper(strings.ReplaceAll(n, " ", ""))
		if !number.MatchString(in.AccountNumber) {
			p.Add("accountNumber", "Nomor rekening berisi 4–34 angka/huruf (IBAN), boleh dengan -.")
		}
	}
	in.AccountName = record.RequiredText(v, "accountName", "Nama pemilik rekening", record.MaxName, &p)
	in.CurrencyCode = record.RequiredText(v, "currencyCode", "Mata uang", record.MaxCode, &p)
	in.Branch = record.OptionalText(v, "branch", "Cabang", 80, &p)
	in.UnitCode = record.OptionalText(v, "unitCode", "Perusahaan pemilik", record.MaxCode, &p)
	in.Purpose = record.ChoiceOr(v, "purpose", "Keperluan", Purposes, "operational", &p)
	in.InvoiceDefault = record.Bool(v, "invoiceDefault", "Rekening utama di invoice", false, &p)
	in.AccountCode = record.OptionalText(v, "accountCode", "Kode akun", 30, &p)
	if in.InvoiceDefault && (in.Purpose == "payroll" || in.Purpose == "tax") {
		p.Add("invoiceDefault", "Rekening gaji atau pajak tidak dicetak di invoice.")
	}
	return in, p
}

func values(x Account) record.Values {
	v := record.ValuesOf(x.Base)
	v["bankCode"], v["accountNumber"], v["accountName"], v["currencyCode"], v["purpose"], v["invoiceDefault"] = x.BankCode, x.AccountNumber, x.AccountName, x.CurrencyCode, x.Purpose, x.InvoiceDefault
	v["branch"], v["unitCode"], v["accountCode"] = record.Opt(x.Branch), record.Opt(x.UnitCode), record.Opt(x.AccountCode)
	return v
}

func New(db *database.DB) *master.Service[Account, Input] {
	return &master.Service[Account, Input]{
		Spec: master.Spec[Account, Input]{
			Menu: "pm.companyBankAccount", Table: "company_bank_account", Noun: "Rekening perusahaan", Import: importSpec(db),
			Base: func(x *Account) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Account, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func check(ctx context.Context, db *database.DB, in *Input, stored *Account, p record.Problems) (record.Problems, error) {
	self := ""
	var keptBank, keptCur, keptUnit *string
	if stored != nil {
		self, keptBank, keptCur, keptUnit = stored.ID, &stored.BankID, &stored.CurrencyID, stored.UnitID
	}
	bank, err := refs.Pick(ctx, db, "bank", "bankCode", "Bank", nonEmpty(in.BankCode), keptBank, &p)
	if err != nil {
		return p, err
	}
	cur, err := refs.Pick(ctx, db, "currency", "currencyCode", "Mata uang", nonEmpty(in.CurrencyCode), keptCur, &p)
	if err != nil {
		return p, err
	}
	unit, err := refs.Pick(ctx, db, "org_unit", "unitCode", "Unit", in.UnitCode, keptUnit, &p)
	if err != nil {
		return p, err
	}
	if unit != nil {
		in.UnitID = &unit.ID
	}
	if bank != nil {
		in.BankID = bank.ID
		if in.AccountNumber != "" && !p.Has("accountNumber") {
			var other string
			switch err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.company_bank_account WHERE bank_id = $1 AND account_number = $2 AND NOT is_deleted AND id::text <> $3`,
				bank.ID, in.AccountNumber, self).Scan(&other); {
			case err == nil:
				p.Add("accountNumber", fmt.Sprintf("Rekening %s di %s sudah ada (%s).", in.AccountNumber, bank.Name, other))
			case !errors.Is(err, pgx.ErrNoRows):
				return p, err
			}
		}
	}
	if cur != nil {
		in.CurrencyID = cur.ID
		if in.InvoiceDefault && !p.Has("invoiceDefault") {
			var other string
			switch err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.company_bank_account WHERE currency_id = $1 AND org_unit_id IS NOT DISTINCT FROM $2::uuid
				AND is_invoice_default AND NOT is_deleted AND id::text <> $3`, cur.ID, in.UnitID, self).Scan(&other); {
			case err == nil:
				p.Add("invoiceDefault", fmt.Sprintf("Rekening utama invoice %s untuk perusahaan ini sudah %s; matikan yang itu dulu.", cur.Name, other))
			case !errors.Is(err, pgx.ErrNoRows):
				return p, err
			}
		}
	}
	return p, nil
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.bank_id, b.code, x.account_number, x.account_name, x.currency_id, c.code, x.branch,
	x.org_unit_id, u.code, x.purpose, x.is_invoice_default, x.account_code
	FROM pm.company_bank_account x JOIN pm.bank b ON b.id = x.bank_id JOIN pm.currency c ON c.id = x.currency_id LEFT JOIN pm.org_unit u ON u.id = x.org_unit_id`

func scan(row pgx.Row) (Account, error) {
	var x Account
	return x, x.Scan(row, &x.BankID, &x.BankCode, &x.AccountNumber, &x.AccountName, &x.CurrencyID, &x.CurrencyCode, &x.Branch,
		&x.UnitID, &x.UnitCode, &x.Purpose, &x.InvoiceDefault, &x.AccountCode)
}

func (r repo) List(ctx context.Context) ([]Account, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.code`)
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
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.company_bank_account (code, name, description, is_active, valid_from, valid_to, created_by,
		bank_id, account_number, account_name, currency_id, branch, org_unit_id, purpose, is_invoice_default, account_code)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor,
		in.BankID, in.AccountNumber, in.AccountName, in.CurrencyID, in.Branch, in.UnitID, in.Purpose, in.InvoiceDefault, in.AccountCode).Scan(&id); err != nil {
		return Account{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Account, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.company_bank_account SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		bank_id = $7, account_number = $8, account_name = $9, currency_id = $10, branch = $11, org_unit_id = $12, purpose = $13,
		is_invoice_default = $14, account_code = $15, updated_at = now(), updated_by = $16, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo,
		in.BankID, in.AccountNumber, in.AccountName, in.CurrencyID, in.Branch, in.UnitID, in.Purpose, in.InvoiceDefault, in.AccountCode, actor); err != nil {
		return Account{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "company_bank_account", id, actor)
}
