// Package partner is the master "Mitra Bisnis" (menu pm.businessPartner): ONE record per legal party the company deals
// with, whatever it is for the company: customer, vendor, shipper, consignee, notify party, overseas agent, carrier,
// trucking, PPJK (customs broker), warehouse/depot. Its roles say which; a party that is both customer and vendor stays
// one record, so its tax identity, addresses, contacts and bank accounts exist once and every module (Freight
// Forwarding, finance, procurement, distribution) points to it.
//
// Rules:
//   - at least one role; the customer terms are kept only while it has a receivable role (customer, agent) and the
//     vendor terms only while it has a payable role (vendor, agent, carrier, trucking, PPJK, warehouse): removing the
//     role clears them;
//   - a customer needs its payment term, invoice currency and at least one address;
//   - NPWP is 16 digits (the 15-digit format of before 2024 is converted: a leading 0); NITKU is the NPWP plus 6
//     digits of a place of business; PKP needs an NPWP; one live partner per NITKU, or per NPWP without NITKU;
//   - the vendor's tax type is a PPh (withheld); payment terms must apply to that side (customer/vendor/both);
//   - each list (addresses, contacts, bank accounts) has at most one primary row; with none marked the first is.
package partner

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// Roles a partner can play, in form order.
var Roles = []string{"customer", "vendor", "shipper", "consignee", "notify_party", "agent", "carrier", "trucking", "customs_broker", "warehouse"}

// EntityTypes are the legal forms.
var EntityTypes = []string{"pt", "cv", "firma", "ud", "koperasi", "yayasan", "bumn", "government", "individual", "foreign", "other"}

var (
	receivable = []string{"customer", "agent"}
	payable    = []string{"vendor", "agent", "carrier", "trucking", "customs_broker", "warehouse"}

	addressKinds    = []string{"office", "billing", "delivery", "pickup", "warehouse", "other"}
	contactPurposes = []string{"general", "operations", "finance", "sales", "management", "customs"}
)

// Limits of the lists.
const (
	MaxAddresses = 20
	MaxContacts  = 30
	MaxAccounts  = 10
	maxCredit    = 9999999999999999.99
)

type Address struct {
	Kind       string  `json:"kind"`
	Label      *string `json:"label"`
	Address    string  `json:"address"`
	CityID     *string `json:"cityId"`
	CityCode   *string `json:"cityCode"`
	PostalCode *string `json:"postalCode"`
	Primary    bool    `json:"primary"`
}

type Contact struct {
	Name     string  `json:"name"`
	JobTitle *string `json:"jobTitle"`
	Purpose  string  `json:"purpose"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Primary  bool    `json:"primary"`
}

type BankAccount struct {
	BankID        string  `json:"bankId"`
	BankCode      string  `json:"bankCode"`
	AccountNumber string  `json:"accountNumber"`
	AccountName   string  `json:"accountName"`
	CurrencyID    string  `json:"currencyId"`
	CurrencyCode  string  `json:"currencyCode"`
	Branch        *string `json:"branch"`
	Primary       bool    `json:"primary"`
}

type Partner struct {
	record.Base
	TradeName   *string  `json:"tradeName"`
	EntityType  string   `json:"entityType"`
	Roles       []string `json:"roles"`
	CountryID   string   `json:"countryId"`
	CountryCode string   `json:"countryCode"`
	Email       *string  `json:"email"`
	Phone       *string  `json:"phone"`
	Website     *string  `json:"website"`
	Nib         *string  `json:"nib"`

	Npwp         *string `json:"npwp"`
	Nitku        *string `json:"nitku"`
	Pkp          bool    `json:"pkp"`
	TaxName      *string `json:"taxName"`
	TaxAddress   *string `json:"taxAddress"`
	ForeignTaxID *string `json:"foreignTaxId"`

	CustomerPaymentTermID   *string  `json:"customerPaymentTermId"`
	CustomerPaymentTermCode *string  `json:"customerPaymentTermCode"`
	CustomerCurrencyID      *string  `json:"customerCurrencyId"`
	CustomerCurrencyCode    *string  `json:"customerCurrencyCode"`
	CreditLimit             *float64 `json:"creditLimit"`
	CreditHold              bool     `json:"creditHold"`
	SalesPersonID           *string  `json:"salesPersonId"`
	SalesPersonCode         *string  `json:"salesPersonCode"`

	VendorPaymentTermID   *string `json:"vendorPaymentTermId"`
	VendorPaymentTermCode *string `json:"vendorPaymentTermCode"`
	VendorCurrencyID      *string `json:"vendorCurrencyId"`
	VendorCurrencyCode    *string `json:"vendorCurrencyCode"`
	VendorTaxTypeID       *string `json:"vendorTaxTypeId"`
	VendorTaxTypeCode     *string `json:"vendorTaxTypeCode"`

	Addresses    []Address     `json:"addresses"`
	Contacts     []Contact     `json:"contacts"`
	BankAccounts []BankAccount `json:"bankAccounts"`
}

// Input is the form as parsed; the ids are filled by the relation check.
type Input struct {
	record.Input
	TradeName, Email, Phone, Website, Nib             *string
	EntityType, CountryCode                           string
	Roles                                             []string
	Npwp, Nitku, TaxName, TaxAddress, ForeignTaxID    *string
	Pkp, CreditHold                                   bool
	CustomerTermCode, CustomerCurrencyCode            *string
	CreditLimit                                       *float64
	SalesPersonCode                                   *string
	VendorTermCode, VendorCurrencyCode, VendorTaxCode *string
	Addresses                                         []Address
	Contacts                                          []Contact
	BankAccounts                                      []BankAccount

	CountryID                                         string
	CustomerTermID, CustomerCurrencyID, SalesPersonID *string
	VendorTermID, VendorCurrencyID, VendorTaxID       *string
}

var (
	digits16 = regexp.MustCompile(`^[0-9]{16}$`)
	digits22 = regexp.MustCompile(`^[0-9]{22}$`)
	digits13 = regexp.MustCompile(`^[0-9]{13}$`)
	postal   = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z -]{2,9}$`)
	account  = regexp.MustCompile(`^[A-Z0-9-]{4,34}$`)
	taxSep   = strings.NewReplacer(".", "", "-", "", " ", "")
)

// HasAny says whether roles holds one of want.
func HasAny(roles, want []string) bool {
	return slices.ContainsFunc(roles, func(r string) bool { return slices.Contains(want, r) })
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.TradeName = record.OptionalText(v, "tradeName", "Nama dagang", record.MaxName, &p)
	in.EntityType = record.Choice(v, "entityType", "Bentuk usaha", EntityTypes, &p)
	in.Roles = parseRoles(v, &p)
	in.CountryCode = record.RequiredText(v, "countryCode", "Negara", record.MaxCode, &p)
	in.Email = record.OptionalEmail(v, "email", "Email", &p)
	in.Phone = record.OptionalPhone(v, "phone", "Telepon", &p)
	in.Website = record.OptionalURL(v, "website", "Situs web", 200, &p)
	if in.Nib = taxDigits(v, "nib", "NIB", &p); in.Nib != nil && !digits13.MatchString(*in.Nib) {
		p.Add("nib", "NIB berisi 13 angka.")
	}

	if in.Npwp = taxDigits(v, "npwp", "NPWP", &p); in.Npwp != nil {
		if len(*in.Npwp) == 15 { // the format of before 2024: the 16-digit NPWP is a leading 0
			n := "0" + *in.Npwp
			in.Npwp = &n
		}
		if !digits16.MatchString(*in.Npwp) {
			p.Add("npwp", "NPWP berisi 16 angka (NPWP 15 angka lama diterima dan diubah otomatis).")
		}
	}
	if in.Nitku = taxDigits(v, "nitku", "NITKU", &p); in.Nitku != nil && !p.Has("nitku") {
		switch {
		case !digits22.MatchString(*in.Nitku):
			p.Add("nitku", "NITKU berisi 22 angka: NPWP 16 angka lalu 6 angka tempat usaha.")
		case in.Npwp == nil:
			p.Add("nitku", "NITKU perlu NPWP; isi NPWP-nya dulu.")
		case !p.Has("npwp") && !strings.HasPrefix(*in.Nitku, *in.Npwp):
			p.Add("nitku", "NITKU harus diawali NPWP mitra ini.")
		}
	}
	in.Pkp = record.Bool(v, "pkp", "PKP", false, &p)
	if in.Pkp && in.Npwp == nil && !p.Has("npwp") {
		p.Add("npwp", "Mitra PKP wajib punya NPWP.")
	}
	in.TaxName = record.OptionalText(v, "taxName", "Nama sesuai NPWP", record.MaxName, &p)
	in.TaxAddress = record.OptionalText(v, "taxAddress", "Alamat sesuai NPWP", 500, &p)
	in.ForeignTaxID = record.OptionalText(v, "foreignTaxId", "Nomor pajak luar negeri", 30, &p)

	// The terms of a side the partner does not play are cleared, whatever the form sent.
	if HasAny(in.Roles, receivable) {
		in.CustomerTermCode = record.OptionalText(v, "customerPaymentTermCode", "Syarat pembayaran customer", record.MaxCode, &p)
		in.CustomerCurrencyCode = record.OptionalText(v, "customerCurrencyCode", "Mata uang tagihan", record.MaxCode, &p)
		in.CreditLimit = record.Decimal(v, "creditLimit", "Plafon kredit", 0, maxCredit, 2, false, &p)
		in.CreditHold = record.Bool(v, "creditHold", "Tahan kredit", false, &p)
		in.SalesPersonCode = record.OptionalText(v, "salesPersonCode", "Tenaga penjualan", record.MaxCode, &p)
		if slices.Contains(in.Roles, "customer") {
			if in.CustomerTermCode == nil && !p.Has("customerPaymentTermCode") {
				p.Add("customerPaymentTermCode", "Syarat pembayaran customer wajib diisi untuk mitra berperan Customer.")
			}
			if in.CustomerCurrencyCode == nil && !p.Has("customerCurrencyCode") {
				p.Add("customerCurrencyCode", "Mata uang tagihan wajib diisi untuk mitra berperan Customer.")
			}
		}
		if in.CreditLimit != nil && in.CustomerCurrencyCode == nil && !p.Has("customerCurrencyCode") {
			p.Add("customerCurrencyCode", "Plafon kredit perlu mata uang tagihan.")
		}
	}
	if HasAny(in.Roles, payable) {
		in.VendorTermCode = record.OptionalText(v, "vendorPaymentTermCode", "Syarat pembayaran vendor", record.MaxCode, &p)
		in.VendorCurrencyCode = record.OptionalText(v, "vendorCurrencyCode", "Mata uang pembayaran", record.MaxCode, &p)
		in.VendorTaxCode = record.OptionalText(v, "vendorTaxTypeCode", "PPh yang dipotong", record.MaxCode, &p)
	}

	in.Addresses = parseAddresses(v, &p)
	in.Contacts = parseContacts(v, &p)
	in.BankAccounts = parseAccounts(v, &p)
	if slices.Contains(in.Roles, "customer") && len(in.Addresses) == 0 && !p.Has("addresses") {
		p.Add("addresses", "Customer wajib punya minimal satu alamat (untuk tagihan).")
	}
	return in, p
}

// taxDigits is a tax or business number with its separators (. - space) removed; nil when blank.
func taxDigits(v record.Values, key, label string, p *record.Problems) *string {
	s := record.OptionalText(v, key, label, 40, p)
	if s == nil || p.Has(key) {
		return nil
	}
	d := taxSep.Replace(*s)
	return &d
}

func parseRoles(v record.Values, p *record.Problems) []string {
	codes := record.Codes(v, "roles", "Peran", p)
	if p.Has("roles") {
		return nil
	}
	var out []string
	for _, c := range codes {
		c = strings.ToLower(c)
		if !slices.Contains(Roles, c) {
			p.Add("roles", fmt.Sprintf("Peran \"%s\" tidak dikenal.", c))
			return nil
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		p.Add("roles", "Peran wajib diisi: pilih minimal satu, mis. Customer.")
	}
	// Form order, whatever order was sent.
	slices.SortFunc(out, func(a, b string) int { return slices.Index(Roles, a) - slices.Index(Roles, b) })
	return out
}

func parseAddresses(v record.Values, p *record.Problems) []Address {
	rows := record.Rows(v, "addresses", "Alamat", MaxAddresses, p)
	out := make([]Address, 0, len(rows))
	flags := make([]bool, 0, len(rows))
	for i, r := range rows {
		var q record.Problems
		a := Address{
			Kind:     record.ChoiceOr(r, "kind", "Jenis alamat", addressKinds, "office", &q),
			Label:    record.OptionalText(r, "label", "Label", 60, &q),
			Address:  record.RequiredText(r, "address", "Alamat", 500, &q),
			CityCode: record.OptionalText(r, "cityCode", "Kota", record.MaxCode, &q),
			Primary:  record.Bool(r, "primary", "Utama", false, &q),
		}
		if a.PostalCode = record.OptionalText(r, "postalCode", "Kode pos", 10, &q); a.PostalCode != nil && !q.Has("postalCode") && !postal.MatchString(*a.PostalCode) {
			q.Add("postalCode", "Kode pos berisi 3–10 huruf atau angka, mis. 78121.")
		}
		record.AddRow(p, "addresses", "Alamat", i, q)
		out, flags = append(out, a), append(flags, a.Primary)
	}
	record.Primary(flags, "addresses", "Alamat", p)
	for i := range out {
		out[i].Primary = flags[i]
	}
	return out
}

func parseContacts(v record.Values, p *record.Problems) []Contact {
	rows := record.Rows(v, "contacts", "Kontak", MaxContacts, p)
	out := make([]Contact, 0, len(rows))
	flags := make([]bool, 0, len(rows))
	for i, r := range rows {
		var q record.Problems
		c := Contact{
			Name:     record.RequiredText(r, "name", "Nama kontak", 100, &q),
			JobTitle: record.OptionalText(r, "jobTitle", "Jabatan", 80, &q),
			Purpose:  record.ChoiceOr(r, "purpose", "Keperluan", contactPurposes, "general", &q),
			Email:    record.OptionalEmail(r, "email", "Email", &q),
			Phone:    record.OptionalPhone(r, "phone", "Telepon", &q),
			Primary:  record.Bool(r, "primary", "Utama", false, &q),
		}
		if c.Email == nil && c.Phone == nil && !q.Has("email") && !q.Has("phone") {
			q.Add("email", "Isi email atau telepon kontak.")
		}
		record.AddRow(p, "contacts", "Kontak", i, q)
		out, flags = append(out, c), append(flags, c.Primary)
	}
	record.Primary(flags, "contacts", "Kontak", p)
	for i := range out {
		out[i].Primary = flags[i]
	}
	return out
}

func parseAccounts(v record.Values, p *record.Problems) []BankAccount {
	rows := record.Rows(v, "bankAccounts", "Rekening", MaxAccounts, p)
	out := make([]BankAccount, 0, len(rows))
	flags := make([]bool, 0, len(rows))
	seen := map[string]int{}
	for i, r := range rows {
		var q record.Problems
		a := BankAccount{
			BankCode:     record.RequiredText(r, "bankCode", "Bank", record.MaxCode, &q),
			AccountName:  record.RequiredText(r, "accountName", "Nama pemilik rekening", record.MaxName, &q),
			CurrencyCode: record.RequiredText(r, "currencyCode", "Mata uang rekening", record.MaxCode, &q),
			Branch:       record.OptionalText(r, "branch", "Cabang", 80, &q),
			Primary:      record.Bool(r, "primary", "Utama", false, &q),
		}
		if n := record.RequiredText(r, "accountNumber", "Nomor rekening", 40, &q); n != "" {
			a.AccountNumber = strings.ToUpper(strings.ReplaceAll(n, " ", ""))
			if !account.MatchString(a.AccountNumber) {
				q.Add("accountNumber", "Nomor rekening berisi 4–34 angka/huruf (IBAN), boleh dengan -.")
			}
		}
		if key := strings.ToLower(a.BankCode) + "/" + a.AccountNumber; a.AccountNumber != "" && a.BankCode != "" {
			if j, dup := seen[key]; dup {
				q.Add("accountNumber", fmt.Sprintf("Rekening ini sama dengan baris %d.", j+1))
			}
			seen[key] = i
		}
		record.AddRow(p, "bankAccounts", "Rekening", i, q)
		out, flags = append(out, a), append(flags, a.Primary)
	}
	record.Primary(flags, "bankAccounts", "Rekening", p)
	for i := range out {
		out[i].Primary = flags[i]
	}
	return out
}

// values is a stored partner as form values (relations by code), the base a partial change is merged onto.
func values(x Partner) record.Values {
	v := record.ValuesOf(x.Base)
	roles := make([]any, len(x.Roles))
	for i, r := range x.Roles {
		roles[i] = r
	}
	v["roles"] = roles
	v["entityType"], v["countryCode"] = x.EntityType, x.CountryCode
	for k, s := range map[string]*string{"tradeName": x.TradeName, "email": x.Email, "phone": x.Phone, "website": x.Website, "nib": x.Nib,
		"npwp": x.Npwp, "nitku": x.Nitku, "taxName": x.TaxName, "taxAddress": x.TaxAddress, "foreignTaxId": x.ForeignTaxID,
		"customerPaymentTermCode": x.CustomerPaymentTermCode, "customerCurrencyCode": x.CustomerCurrencyCode, "salesPersonCode": x.SalesPersonCode,
		"vendorPaymentTermCode": x.VendorPaymentTermCode, "vendorCurrencyCode": x.VendorCurrencyCode, "vendorTaxTypeCode": x.VendorTaxTypeCode} {
		v[k] = record.Opt(s)
	}
	v["pkp"], v["creditHold"], v["creditLimit"] = x.Pkp, x.CreditHold, record.Opt(x.CreditLimit)
	var adr, con, acc []any
	for _, a := range x.Addresses {
		adr = append(adr, map[string]any{"kind": a.Kind, "label": record.Opt(a.Label), "address": a.Address, "cityCode": record.Opt(a.CityCode), "postalCode": record.Opt(a.PostalCode), "primary": a.Primary})
	}
	for _, c := range x.Contacts {
		con = append(con, map[string]any{"name": c.Name, "jobTitle": record.Opt(c.JobTitle), "purpose": c.Purpose, "email": record.Opt(c.Email), "phone": record.Opt(c.Phone), "primary": c.Primary})
	}
	for _, a := range x.BankAccounts {
		acc = append(acc, map[string]any{"bankCode": a.BankCode, "accountNumber": a.AccountNumber, "accountName": a.AccountName, "currencyCode": a.CurrencyCode, "branch": record.Opt(a.Branch), "primary": a.Primary})
	}
	v["addresses"], v["contacts"], v["bankAccounts"] = adr, con, acc
	return v
}

func New(db *database.DB) *master.Service[Partner, Input] {
	return &master.Service[Partner, Input]{
		Spec: master.Spec[Partner, Input]{
			Menu: "pm.businessPartner", Table: "business_partner", Noun: "Mitra bisnis", Import: importSpec(db),
			Base: func(x *Partner) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Partner, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

// picker resolves relations by code; a row the stored partner already points to stays valid when inactive.
type picker struct {
	ctx  context.Context
	db   *database.DB
	kept map[string]bool // ids the stored partner points to
	p    *record.Problems
}

func (k picker) pick(table, field, label string, code *string) (*string, error) {
	if code == nil || k.p.Has(field) {
		return nil, nil
	}
	var kept *string
	if r, err := refs.Lookup(k.ctx, k.db, table, *code); err == nil && k.kept[r.ID] {
		kept = &r.ID
	}
	r, err := refs.Pick(k.ctx, k.db, table, field, label, code, kept, k.p)
	if r == nil || err != nil {
		return nil, err
	}
	return &r.ID, nil
}

func check(ctx context.Context, db *database.DB, in *Input, stored *Partner, p record.Problems) (record.Problems, error) {
	self := ""
	kept := map[string]bool{}
	if stored != nil {
		self = stored.ID
		for _, id := range []*string{&stored.CountryID, stored.CustomerPaymentTermID, stored.CustomerCurrencyID, stored.SalesPersonID,
			stored.VendorPaymentTermID, stored.VendorCurrencyID, stored.VendorTaxTypeID} {
			if id != nil {
				kept[*id] = true
			}
		}
		for _, a := range stored.Addresses {
			if a.CityID != nil {
				kept[*a.CityID] = true
			}
		}
		for _, a := range stored.BankAccounts {
			kept[a.BankID], kept[a.CurrencyID] = true, true
		}
	}
	k := picker{ctx, db, kept, &p}
	var err error
	one := func(dst **string, table, field, label string, code *string) {
		if err == nil {
			*dst, err = k.pick(table, field, label, code)
		}
	}
	var country *string
	one(&country, "country", "countryCode", "Negara", nonEmpty(in.CountryCode))
	one(&in.CustomerTermID, "payment_term", "customerPaymentTermCode", "Syarat pembayaran", in.CustomerTermCode)
	one(&in.CustomerCurrencyID, "currency", "customerCurrencyCode", "Mata uang", in.CustomerCurrencyCode)
	one(&in.SalesPersonID, "sales_person", "salesPersonCode", "Tenaga penjualan", in.SalesPersonCode)
	one(&in.VendorTermID, "payment_term", "vendorPaymentTermCode", "Syarat pembayaran", in.VendorTermCode)
	one(&in.VendorCurrencyID, "currency", "vendorCurrencyCode", "Mata uang", in.VendorCurrencyCode)
	one(&in.VendorTaxID, "tax_type", "vendorTaxTypeCode", "Jenis pajak", in.VendorTaxCode)
	for i := range in.Addresses {
		one(&in.Addresses[i].CityID, "city", record.RowField("addresses", i, "cityCode"), "Kota", in.Addresses[i].CityCode)
	}
	for i := range in.BankAccounts {
		a := &in.BankAccounts[i]
		var bank, cur *string
		one(&bank, "bank", record.RowField("bankAccounts", i, "bankCode"), "Bank", nonEmpty(a.BankCode))
		one(&cur, "currency", record.RowField("bankAccounts", i, "currencyCode"), "Mata uang", nonEmpty(a.CurrencyCode))
		if bank != nil {
			a.BankID = *bank
		}
		if cur != nil {
			a.CurrencyID = *cur
		}
	}
	if err != nil {
		return p, err
	}
	if country != nil {
		in.CountryID = *country
	}

	if err := sides(ctx, db, in, &p); err != nil {
		return p, err
	}
	return p, taxIdentity(ctx, db, in, self, &p)
}

// sides checks that each payment term applies to its side and the vendor's tax is a PPh.
func sides(ctx context.Context, db *database.DB, in *Input, p *record.Problems) error {
	for _, t := range []struct {
		id    *string
		field string
		side  string
	}{{in.CustomerTermID, "customerPaymentTermCode", "customer"}, {in.VendorTermID, "vendorPaymentTermCode", "vendor"}} {
		if t.id == nil {
			continue
		}
		var applies, name string
		if err := db.Q(ctx).QueryRow(ctx, `SELECT applies_to, name FROM pm.payment_term WHERE id = $1`, *t.id).Scan(&applies, &name); err != nil {
			return err
		}
		if applies != "both" && applies != t.side {
			p.Add(t.field, fmt.Sprintf("Syarat pembayaran %s hanya untuk %s.", name, applies))
		}
	}
	if in.VendorTaxID != nil {
		var cat, method, name string
		if err := db.Q(ctx).QueryRow(ctx, `SELECT category, method, name FROM pm.tax_type WHERE id = $1`, *in.VendorTaxID).Scan(&cat, &method, &name); err != nil {
			return err
		}
		if !strings.HasPrefix(cat, "PPH") || method != "withheld" {
			p.Add("vendorTaxTypeCode", fmt.Sprintf("%s bukan PPh yang dipotong; pilih jenis pajak PPh.", name))
		}
	}
	return nil
}

// taxIdentity keeps one live partner per place of business: per NITKU, or per NPWP when there is no NITKU.
func taxIdentity(ctx context.Context, db *database.DB, in *Input, self string, p *record.Problems) error {
	var q, field, val string
	switch {
	case in.Nitku != nil && !p.Has("nitku"):
		q, field, val = `SELECT code, name FROM pm.business_partner WHERE nitku = $1 AND NOT is_deleted AND id::text <> $2`, "nitku", *in.Nitku
	case in.Npwp != nil && !p.Has("npwp") && in.Nitku == nil:
		q, field, val = `SELECT code, name FROM pm.business_partner WHERE npwp = $1 AND nitku IS NULL AND NOT is_deleted AND id::text <> $2`, "npwp", *in.Npwp
	default:
		return nil
	}
	var code, name string
	err := db.Q(ctx).QueryRow(ctx, q, val, self).Scan(&code, &name)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return err
	case field == "nitku":
		p.Add("nitku", fmt.Sprintf("NITKU ini sudah dipakai mitra %s (%s).", name, code))
	default:
		p.Add("npwp", fmt.Sprintf("NPWP ini sudah dipakai mitra %s (%s). Untuk cabang, isi NITKU cabangnya.", name, code))
	}
	return nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
