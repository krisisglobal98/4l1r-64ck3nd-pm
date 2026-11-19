package partner

import (
	"context"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/record"
)

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.trade_name, x.entity_type, x.roles, x.country_id, c.code, x.email, x.phone, x.website, x.nib,
	x.npwp, x.nitku, x.is_pkp, x.tax_name, x.tax_address, x.foreign_tax_id,
	x.customer_payment_term_id, ct.code, x.customer_currency_id, cc.code, x.credit_limit::float8, x.credit_hold, x.sales_person_id, sp.code,
	x.vendor_payment_term_id, vt.code, x.vendor_currency_id, vc.code, x.vendor_tax_type_id, tx.code
	FROM pm.business_partner x JOIN pm.country c ON c.id = x.country_id
	LEFT JOIN pm.payment_term ct ON ct.id = x.customer_payment_term_id LEFT JOIN pm.currency cc ON cc.id = x.customer_currency_id
	LEFT JOIN pm.sales_person sp ON sp.id = x.sales_person_id
	LEFT JOIN pm.payment_term vt ON vt.id = x.vendor_payment_term_id LEFT JOIN pm.currency vc ON vc.id = x.vendor_currency_id
	LEFT JOIN pm.tax_type tx ON tx.id = x.vendor_tax_type_id`

func scan(row pgx.Row) (Partner, error) {
	var x Partner
	return x, x.Scan(row, &x.TradeName, &x.EntityType, &x.Roles, &x.CountryID, &x.CountryCode, &x.Email, &x.Phone, &x.Website, &x.Nib,
		&x.Npwp, &x.Nitku, &x.Pkp, &x.TaxName, &x.TaxAddress, &x.ForeignTaxID,
		&x.CustomerPaymentTermID, &x.CustomerPaymentTermCode, &x.CustomerCurrencyID, &x.CustomerCurrencyCode, &x.CreditLimit, &x.CreditHold,
		&x.SalesPersonID, &x.SalesPersonCode,
		&x.VendorPaymentTermID, &x.VendorPaymentTermCode, &x.VendorCurrencyID, &x.VendorCurrencyCode, &x.VendorTaxTypeID, &x.VendorTaxTypeCode)
}

func (r repo) List(ctx context.Context) ([]Partner, error) {
	list, err := master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.code`)
	if err != nil {
		return nil, err
	}
	return list, r.lists(ctx, list)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Partner, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return r.one(ctx, q, code)
}

func (r repo) one(ctx context.Context, q string, arg any) (Partner, error) {
	x, err := master.One(ctx, r.db, scan, q, arg)
	if err != nil {
		return x, err
	}
	list := []Partner{x}
	err = r.lists(ctx, list)
	return list[0], err
}

// lists reads the addresses, contacts and bank accounts of partners in three queries, in form order.
func (r repo) lists(ctx context.Context, ps []Partner) error {
	if len(ps) == 0 {
		return nil
	}
	ids := make([]string, len(ps))
	at := map[string]int{}
	for i := range ps {
		ids[i], at[ps[i].ID] = ps[i].ID, i
		ps[i].Addresses, ps[i].Contacts, ps[i].BankAccounts = []Address{}, []Contact{}, []BankAccount{}
	}
	rows, err := r.db.Q(ctx).Query(ctx, `SELECT a.business_partner_id, a.kind, a.label, a.address, a.city_id, c.code, a.postal_code, a.is_primary
		FROM pm.business_partner_address a LEFT JOIN pm.city c ON c.id = a.city_id
		WHERE a.business_partner_id = ANY($1::uuid[]) ORDER BY a.business_partner_id, a.seq`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var a Address
		if err := rows.Scan(&id, &a.Kind, &a.Label, &a.Address, &a.CityID, &a.CityCode, &a.PostalCode, &a.Primary); err != nil {
			rows.Close()
			return err
		}
		ps[at[id]].Addresses = append(ps[at[id]].Addresses, a)
	}
	if rows.Close(); rows.Err() != nil {
		return rows.Err()
	}
	rows, err = r.db.Q(ctx).Query(ctx, `SELECT business_partner_id, name, job_title, purpose, email, phone, is_primary
		FROM pm.business_partner_contact WHERE business_partner_id = ANY($1::uuid[]) ORDER BY business_partner_id, seq`, ids)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var c Contact
		if err := rows.Scan(&id, &c.Name, &c.JobTitle, &c.Purpose, &c.Email, &c.Phone, &c.Primary); err != nil {
			rows.Close()
			return err
		}
		ps[at[id]].Contacts = append(ps[at[id]].Contacts, c)
	}
	if rows.Close(); rows.Err() != nil {
		return rows.Err()
	}
	rows, err = r.db.Q(ctx).Query(ctx, `SELECT a.business_partner_id, a.bank_id, b.code, a.account_number, a.account_name, a.currency_id, c.code, a.branch, a.is_primary
		FROM pm.business_partner_bank_account a JOIN pm.bank b ON b.id = a.bank_id JOIN pm.currency c ON c.id = a.currency_id
		WHERE a.business_partner_id = ANY($1::uuid[]) ORDER BY a.business_partner_id, a.seq`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var a BankAccount
		if err := rows.Scan(&id, &a.BankID, &a.BankCode, &a.AccountNumber, &a.AccountName, &a.CurrencyID, &a.CurrencyCode, &a.Branch, &a.Primary); err != nil {
			return err
		}
		ps[at[id]].BankAccounts = append(ps[at[id]].BankAccounts, a)
	}
	return rows.Err()
}

// saveLists replaces the partner's lists with in's.
func (r repo) saveLists(ctx context.Context, id string, in Input) error {
	q := r.db.Q(ctx)
	for _, t := range []string{"business_partner_address", "business_partner_contact", "business_partner_bank_account"} {
		if _, err := q.Exec(ctx, `DELETE FROM pm.`+t+` WHERE business_partner_id = $1`, id); err != nil {
			return err
		}
	}
	for i, a := range in.Addresses {
		if _, err := q.Exec(ctx, `INSERT INTO pm.business_partner_address (business_partner_id, seq, kind, label, address, city_id, postal_code, is_primary)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, i+1, a.Kind, a.Label, a.Address, a.CityID, a.PostalCode, a.Primary); err != nil {
			return err
		}
	}
	for i, c := range in.Contacts {
		if _, err := q.Exec(ctx, `INSERT INTO pm.business_partner_contact (business_partner_id, seq, name, job_title, purpose, email, phone, is_primary)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, i+1, c.Name, c.JobTitle, c.Purpose, c.Email, c.Phone, c.Primary); err != nil {
			return err
		}
	}
	for i, a := range in.BankAccounts {
		if _, err := q.Exec(ctx, `INSERT INTO pm.business_partner_bank_account (business_partner_id, seq, bank_id, account_number, account_name, currency_id, branch, is_primary)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, id, i+1, a.BankID, a.AccountNumber, a.AccountName, a.CurrencyID, a.Branch, a.Primary); err != nil {
			return err
		}
	}
	return nil
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Partner, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.business_partner (code, name, description, is_active, valid_from, valid_to, created_by,
		trade_name, entity_type, roles, country_id, email, phone, website, nib, npwp, nitku, is_pkp, tax_name, tax_address, foreign_tax_id,
		customer_payment_term_id, customer_currency_id, credit_limit, credit_hold, sales_person_id,
		vendor_payment_term_id, vendor_currency_id, vendor_tax_type_id)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
		$22, $23, $24, $25, $26, $27, $28, $29) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor,
		in.TradeName, in.EntityType, in.Roles, in.CountryID, in.Email, in.Phone, in.Website, in.Nib, in.Npwp, in.Nitku, in.Pkp, in.TaxName, in.TaxAddress, in.ForeignTaxID,
		in.CustomerTermID, in.CustomerCurrencyID, in.CreditLimit, in.CreditHold, in.SalesPersonID,
		in.VendorTermID, in.VendorCurrencyID, in.VendorTaxID).Scan(&id); err != nil {
		return Partner{}, err
	}
	if err := r.saveLists(ctx, id, in); err != nil {
		return Partner{}, err
	}
	return r.one(ctx, sel+` WHERE x.id = $1`, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Partner, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.business_partner SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		trade_name = $7, entity_type = $8, roles = $9, country_id = $10, email = $11, phone = $12, website = $13, nib = $14,
		npwp = $15, nitku = $16, is_pkp = $17, tax_name = $18, tax_address = $19, foreign_tax_id = $20,
		customer_payment_term_id = $21, customer_currency_id = $22, credit_limit = $23, credit_hold = $24, sales_person_id = $25,
		vendor_payment_term_id = $26, vendor_currency_id = $27, vendor_tax_type_id = $28,
		updated_at = now(), updated_by = $29, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo,
		in.TradeName, in.EntityType, in.Roles, in.CountryID, in.Email, in.Phone, in.Website, in.Nib,
		in.Npwp, in.Nitku, in.Pkp, in.TaxName, in.TaxAddress, in.ForeignTaxID,
		in.CustomerTermID, in.CustomerCurrencyID, in.CreditLimit, in.CreditHold, in.SalesPersonID,
		in.VendorTermID, in.VendorCurrencyID, in.VendorTaxID, actor); err != nil {
		return Partner{}, err
	}
	if err := r.saveLists(ctx, id, in); err != nil {
		return Partner{}, err
	}
	return r.one(ctx, sel+` WHERE x.id = $1`, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "business_partner", id, actor)
}
