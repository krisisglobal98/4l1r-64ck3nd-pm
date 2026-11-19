package partner

import (
	"strings"
	"testing"

	"alir.local/pm-service/internal/platform/record"
)

func base(extra record.Values) record.Values {
	v := record.Values{"name": "PT Uji", "entityType": "pt", "roles": []any{"shipper"}, "countryCode": "MNG001"}
	for k, x := range extra {
		v[k] = x
	}
	return v
}

func problem(p record.Problems, field string) string {
	for _, x := range p {
		if x.Field == field {
			return x.Message
		}
	}
	return ""
}

func TestParseTaxIdentity(t *testing.T) {
	in, p := Parse(base(record.Values{"npwp": "01.234.567.8-701.000", "nitku": "0012345678701000000001", "pkp": true}))
	if len(p) != 0 || *in.Npwp != "0012345678701000" || *in.Nitku != "0012345678701000000001" || !in.Pkp {
		t.Fatalf("15-digit NPWP converted, NITKU kept: %+v %v", in, p)
	}
	for _, c := range []struct {
		v     record.Values
		field string
		want  string
	}{
		{record.Values{"npwp": "1234"}, "npwp", "16 angka"},
		{record.Values{"pkp": true}, "npwp", "PKP"},
		{record.Values{"nitku": "0012345678701000000001"}, "nitku", "perlu NPWP"},
		{record.Values{"npwp": "0012345678701000", "nitku": "9912345678701000000001"}, "nitku", "diawali NPWP"},
		{record.Values{"npwp": "0012345678701000", "nitku": "00123"}, "nitku", "22 angka"},
		{record.Values{"nib": "12"}, "nib", "13 angka"},
		{record.Values{"roles": []any{}}, "roles", "minimal satu"},
		{record.Values{"roles": []any{"buyer"}}, "roles", "tidak dikenal"},
		{record.Values{"entityType": "llc"}, "entityType", ""},
		{record.Values{"email": "bukan-email"}, "email", "bukan alamat"},
		{record.Values{"website": "perusahaan.co.id"}, "website", "http"},
	} {
		_, p := Parse(base(c.v))
		if m := problem(p, c.field); m == "" || !strings.Contains(m, c.want) {
			t.Errorf("%v: %s = %q (all %v)", c.v, c.field, m, p)
		}
	}
}

func TestParseSides(t *testing.T) {
	// A customer needs its term, currency and an address; the vendor side of a pure customer is dropped.
	_, p := Parse(base(record.Values{"roles": []any{"customer"}}))
	for _, f := range []string{"customerPaymentTermCode", "customerCurrencyCode", "addresses"} {
		if problem(p, f) == "" {
			t.Errorf("customer without %s: %v", f, p)
		}
	}
	in, p := Parse(base(record.Values{"roles": []any{"customer"}, "customerPaymentTermCode": "MTP001", "customerCurrencyCode": "MMU001",
		"creditLimit": 1500000.5, "vendorPaymentTermCode": "MTP002", "vendorTaxTypeCode": "MPJ002",
		"addresses": []any{map[string]any{"address": "Jl. Uji 1"}}}))
	if len(p) != 0 || in.VendorTermCode != nil || in.VendorTaxCode != nil || *in.CreditLimit != 1500000.5 {
		t.Fatalf("customer only: %+v %v", in, p)
	}
	// A carrier keeps vendor terms and drops customer terms.
	in, p = Parse(base(record.Values{"roles": []any{"carrier"}, "customerPaymentTermCode": "MTP001", "vendorPaymentTermCode": "MTP002"}))
	if len(p) != 0 || in.CustomerTermCode != nil || in.VendorTermCode == nil {
		t.Fatalf("carrier: %+v %v", in, p)
	}
	// Roles come back in form order, once.
	in, _ = Parse(base(record.Values{"roles": []any{"warehouse", "Customer", "customer", "vendor"}, "customerPaymentTermCode": "A", "customerCurrencyCode": "B",
		"addresses": []any{map[string]any{"address": "x"}}}))
	if strings.Join(in.Roles, ",") != "customer,vendor,warehouse" {
		t.Errorf("roles %v", in.Roles)
	}
}

func TestParseLists(t *testing.T) {
	in, p := Parse(base(record.Values{
		"addresses": []any{map[string]any{"address": "Kantor"}, map[string]any{"address": "Gudang", "kind": "warehouse"}},
		"contacts":  []any{map[string]any{"name": "Budi", "phone": "+62 811 555 0101"}, map[string]any{"name": "Sari", "email": "SARI@X.CO.ID", "primary": true}},
		"bankAccounts": []any{map[string]any{"bankCode": "MBK001", "accountNumber": "1234 5678 90", "accountName": "PT Uji", "currencyCode": "MMU001"}},
	}))
	if len(p) != 0 {
		t.Fatalf("problems %v", p)
	}
	if !in.Addresses[0].Primary || in.Addresses[1].Primary || in.Addresses[1].Kind != "warehouse" || in.Addresses[0].Kind != "office" {
		t.Errorf("addresses %+v", in.Addresses)
	}
	if in.Contacts[0].Primary || !in.Contacts[1].Primary || *in.Contacts[1].Email != "sari@x.co.id" || in.Contacts[0].Purpose != "general" {
		t.Errorf("contacts %+v", in.Contacts)
	}
	if a := in.BankAccounts[0]; a.AccountNumber != "1234567890" || !a.Primary {
		t.Errorf("account %+v", a)
	}

	_, p = Parse(base(record.Values{
		"addresses":    []any{map[string]any{"address": " ", "postalCode": "!"}, map[string]any{"address": "a", "primary": true}, map[string]any{"address": "b", "primary": true}},
		"contacts":     []any{map[string]any{"name": "Tanpa kontak"}},
		"bankAccounts": []any{map[string]any{"bankCode": "B", "accountNumber": "111", "accountName": "x", "currencyCode": "C"}, map[string]any{"bankCode": "B", "accountNumber": "1111", "accountName": "x", "currencyCode": "C"}, map[string]any{"bankCode": "b", "accountNumber": "1111", "accountName": "x", "currencyCode": "C"}},
	}))
	for field, want := range map[string]string{
		"addresses.0.address":          "Alamat baris 1",
		"addresses.0.postalCode":       "Kode pos",
		"addresses":                    "hanya satu",
		"contacts.0.email":             "email atau telepon",
		"bankAccounts.0.accountNumber": "4–34",
		"bankAccounts.2.accountNumber": "sama dengan baris 2",
	} {
		if m := problem(p, field); !strings.Contains(m, want) {
			t.Errorf("%s = %q, want %q (all %v)", field, m, want, p)
		}
	}

	rows := make([]any, MaxAccounts+1)
	for i := range rows {
		rows[i] = map[string]any{}
	}
	if _, p = Parse(base(record.Values{"bankAccounts": rows})); !strings.Contains(problem(p, "bankAccounts"), "paling banyak") {
		t.Errorf("too many rows: %v", p)
	}
	if _, p = Parse(base(record.Values{"contacts": "Budi"})); problem(p, "contacts") == "" {
		t.Errorf("not a list: %v", p)
	}
}

// values → Parse is the identity: a partial update merged onto a stored partner keeps everything.
func TestValuesRoundTrip(t *testing.T) {
	code := func(s string) *string { return &s }
	x := Partner{EntityType: "cv", Roles: []string{"customer", "vendor"}, CountryCode: "MNG001", Npwp: code("0012345678701000"),
		CustomerPaymentTermCode: code("MTP001"), CustomerCurrencyCode: code("MMU001"), VendorTaxTypeCode: code("MPJ002"),
		Addresses:    []Address{{Kind: "billing", Address: "Jl. A", CityCode: code("MKT001"), Primary: true}},
		Contacts:     []Contact{{Name: "Budi", Purpose: "finance", Phone: code("0811 5550"), Primary: true}},
		BankAccounts: []BankAccount{{BankCode: "MBK001", AccountNumber: "123456", AccountName: "CV Uji", CurrencyCode: "MMU001", Primary: true}}}
	x.Name = "CV Uji"
	in, p := Parse(values(x))
	if len(p) != 0 || in.EntityType != "cv" || len(in.Addresses) != 1 || *in.Addresses[0].CityCode != "MKT001" || in.Contacts[0].Purpose != "finance" ||
		in.BankAccounts[0].AccountNumber != "123456" || *in.VendorTaxCode != "MPJ002" || *in.Npwp != "0012345678701000" {
		t.Fatalf("round trip %+v %v", in, p)
	}
}
