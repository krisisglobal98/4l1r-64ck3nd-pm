//go:build e2e

// Mitra Bisnis (one party, many roles, with its tax identity, lists and terms) and Tenaga Penjualan.
// Positive, negative and stress.
package e2e

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	bpBase = "/pm/masters/business-partners"
	spBase = "/pm/masters/sales-persons"
)

var npwpSeq atomic.Int64

// nextNpwp is a 16-digit NPWP no other test uses.
func nextNpwp() string { return fmt.Sprintf("%016d", 9100000000000000+npwpSeq.Add(1)) }

// refsFor are the masters a partner points to, made fresh for one test.
type refsFor struct {
	idr, usd, country, city, bank, termC, termV, termAll, pph, ppn, sales string
}

func newPartnerRefs(t testing.TB) refsFor {
	t.Helper()
	cur := must(t, call(t, "GET", curBase, nil), 200).List
	cty := must(t, call(t, "GET", ctyBase, nil), 200).List
	id := byKey(cty, "iso2", "ID")["code"].(string)
	return refsFor{
		idr: byKey(cur, "isoCode", "IDR")["code"].(string), usd: byKey(cur, "isoCode", "USD")["code"].(string), country: id,
		city:    must(t, call(t, "POST", cityBase, map[string]any{"name": "Kota Mitra", "countryCode": id, "province": fmt.Sprint("Prov ", npwpSeq.Add(1))}), 201).str("code"),
		bank:    must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank Mitra"}), 201).str("code"),
		termC:   must(t, call(t, "POST", termBase, map[string]any{"name": "NET 30 customer", "days": 30, "appliesTo": "customer"}), 201).str("code"),
		termV:   must(t, call(t, "POST", termBase, map[string]any{"name": "NET 14 vendor", "days": 14, "appliesTo": "vendor"}), 201).str("code"),
		termAll: must(t, call(t, "POST", termBase, map[string]any{"name": "Tunai", "days": 0}), 201).str("code"),
		pph:     must(t, call(t, "POST", taxBase, map[string]any{"name": "PPh 23", "category": "PPH23", "ratePct": 2}), 201).str("code"),
		ppn:     must(t, call(t, "POST", taxBase, map[string]any{"name": "PPN", "category": "PPN", "ratePct": 12, "basePct": 91.6667}), 201).str("code"),
		sales:   must(t, call(t, "POST", spBase, map[string]any{"name": "Sales Uji", "employeeCode": newEmployee(t)}), 201).str("code"),
	}
}

// customer is the body of a full customer-and-vendor partner.
func (r refsFor) customer(extra map[string]any) map[string]any {
	b := map[string]any{
		"name": "PT Mitra Uji", "tradeName": "Mitra", "entityType": "pt", "roles": []string{"vendor", "customer", "shipper"}, "countryCode": r.country,
		"email": "Info@Mitra.co.id", "phone": "+62 21 555 0101", "website": "https://mitra.co.id", "nib": "1234567890123",
		"npwp": nextNpwp(), "pkp": true, "taxName": "PT MITRA UJI", "taxAddress": "Jl. Pajak 1, Jakarta",
		"customerPaymentTermCode": r.termC, "customerCurrencyCode": r.idr, "creditLimit": 2500000000.5, "salesPersonCode": r.sales,
		"vendorPaymentTermCode": r.termV, "vendorCurrencyCode": r.idr, "vendorTaxTypeCode": r.pph,
		"addresses": []map[string]any{
			{"kind": "billing", "label": "Kantor pusat", "address": "Jl. Sudirman 1", "cityCode": r.city, "postalCode": "10210"},
			{"kind": "warehouse", "label": "Gudang Dumai", "address": "Kawasan Industri Dumai", "primary": true},
		},
		"contacts": []map[string]any{
			{"name": "Budi", "jobTitle": "Finance Manager", "purpose": "finance", "email": "budi@mitra.co.id", "primary": true},
			{"name": "Sari", "purpose": "operations", "phone": "0811-555-0101"},
		},
		"bankAccounts": []map[string]any{
			{"bankCode": r.bank, "accountNumber": "123 456 7890", "accountName": "PT Mitra Uji", "currencyCode": r.idr, "branch": "Sudirman"},
			{"bankCode": r.bank, "accountNumber": "999-888", "accountName": "PT Mitra Uji", "currencyCode": r.usd},
		},
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func rowsOf(r resp, key string) []map[string]any {
	list, _ := r.Obj[key].([]any)
	out := make([]map[string]any, len(list))
	for i, x := range list {
		out[i] = x.(map[string]any)
	}
	return out
}

func TestPartnersPositive(t *testing.T) {
	r := newPartnerRefs(t)

	t.Run("BP01 a full partner: roles in form order, tax identity, three lists, both sides' terms", func(t *testing.T) {
		p := must(t, call(t, "POST", bpBase, r.customer(nil)), 201)
		if !genCodeOf("MMB").MatchString(p.str("code")) || len(p.str("code")) != 7 {
			t.Errorf("code %s", p.str("code"))
		}
		roles, _ := p.Obj["roles"].([]any)
		if fmt.Sprint(roles) != "[customer vendor shipper]" || p.str("email") != "info@mitra.co.id" || p.Obj["pkp"] != true || p.str("countryCode") != r.country {
			t.Errorf("identity %s", trunc(p.Raw))
		}
		if p.str("customerPaymentTermCode") != r.termC || p.str("salesPersonCode") != r.sales || p.Obj["creditLimit"] != 2500000000.5 ||
			p.str("vendorTaxTypeCode") != r.pph || p.str("vendorPaymentTermCode") != r.termV {
			t.Errorf("terms %s", trunc(p.Raw))
		}
		adr, con, acc := rowsOf(p, "addresses"), rowsOf(p, "contacts"), rowsOf(p, "bankAccounts")
		if len(adr) != 2 || adr[0]["cityCode"] != r.city || adr[0]["primary"] != false || adr[1]["primary"] != true || adr[1]["kind"] != "warehouse" {
			t.Errorf("addresses %v", adr)
		}
		if len(con) != 2 || con[0]["primary"] != true || con[1]["purpose"] != "operations" {
			t.Errorf("contacts %v", con)
		}
		if len(acc) != 2 || acc[0]["accountNumber"] != "1234567890" || acc[0]["primary"] != true || acc[1]["currencyCode"] != r.usd || acc[0]["bankCode"] != r.bank {
			t.Errorf("accounts %v", acc)
		}
		g := must(t, call(t, "GET", bpBase+"/"+strings.ToLower(p.str("code")), nil), 200)
		if len(rowsOf(g, "addresses")) != 2 || len(rowsOf(g, "bankAccounts")) != 2 {
			t.Errorf("read back %s", trunc(g.Raw))
		}
		if l := byKey(must(t, call(t, "GET", bpBase, nil), 200).List, "code", p.str("code")); l == nil || len(l["contacts"].([]any)) != 2 {
			t.Errorf("list row %v", l)
		}
	})

	t.Run("BP02 a partial change keeps the lists and terms; sending a list replaces it", func(t *testing.T) {
		p := must(t, call(t, "POST", bpBase, r.customer(nil)), 201)
		u := must(t, call(t, "PUT", bpBase+"/"+p.str("code"), map[string]any{"name": "PT Mitra Uji Baru", "version": p.Obj["version"]}), 200)
		if u.str("name") != "PT Mitra Uji Baru" || len(rowsOf(u, "addresses")) != 2 || len(rowsOf(u, "contacts")) != 2 || u.str("customerPaymentTermCode") != r.termC || u.Obj["version"] != 2.0 {
			t.Errorf("partial %s", trunc(u.Raw))
		}
		u = must(t, call(t, "PUT", bpBase+"/"+p.str("code"), map[string]any{"contacts": []map[string]any{{"name": "Dewi", "email": "dewi@mitra.co.id"}}}), 200)
		if c := rowsOf(u, "contacts"); len(c) != 1 || c[0]["name"] != "Dewi" || c[0]["primary"] != true || len(rowsOf(u, "addresses")) != 2 {
			t.Errorf("replaced %s", trunc(u.Raw))
		}
	})

	t.Run("BP03 removing a role clears that side's terms; a carrier keeps only vendor terms", func(t *testing.T) {
		p := must(t, call(t, "POST", bpBase, r.customer(nil)), 201)
		u := must(t, call(t, "PUT", bpBase+"/"+p.str("code"), map[string]any{"roles": []string{"vendor"}}), 200)
		if u.Obj["customerPaymentTermCode"] != nil || u.Obj["salesPersonCode"] != nil || u.Obj["creditLimit"] != nil || u.str("vendorTaxTypeCode") != r.pph {
			t.Errorf("customer side kept %s", trunc(u.Raw))
		}
		c := must(t, call(t, "POST", bpBase, map[string]any{"name": "Pelayaran Uji", "entityType": "foreign", "roles": []string{"carrier"}, "countryCode": r.country,
			"customerPaymentTermCode": r.termC, "vendorPaymentTermCode": r.termAll, "foreignTaxId": "SG-201912345K"}), 201)
		if c.Obj["customerPaymentTermCode"] != nil || c.str("vendorPaymentTermCode") != r.termAll || len(rowsOf(c, "addresses")) != 0 {
			t.Errorf("carrier %s", trunc(c.Raw))
		}
	})

	t.Run("BP04 head office and branch share an NPWP; each branch has its NITKU; the old 15-digit NPWP is converted", func(t *testing.T) {
		n := nextNpwp()
		ho := must(t, call(t, "POST", bpBase, map[string]any{"name": "PT Pusat", "entityType": "pt", "roles": []string{"consignee"}, "countryCode": r.country, "npwp": n}), 201)
		br := must(t, call(t, "POST", bpBase, map[string]any{"name": "PT Pusat Cabang Dumai", "entityType": "pt", "roles": []string{"consignee"}, "countryCode": r.country,
			"npwp": n, "nitku": n + "000002"}), 201)
		if ho.str("npwp") != n || br.str("nitku") != n+"000002" {
			t.Errorf("ho %s br %s", trunc(ho.Raw), trunc(br.Raw))
		}
		old := "02.456.789.0-211.000"
		o := must(t, call(t, "POST", bpBase, map[string]any{"name": "PT Lama", "entityType": "pt", "roles": []string{"notify_party"}, "countryCode": r.country, "npwp": old}), 201)
		if o.str("npwp") != "0024567890211000" {
			t.Errorf("converted %q", o.str("npwp"))
		}
	})

	t.Run("BP05 a sales person per employee; masters used by a partner cannot be deleted", func(t *testing.T) {
		p := must(t, call(t, "POST", bpBase, r.customer(nil)), 201)
		for _, path := range []string{spBase + "/" + r.sales, cityBase + "/" + r.city, bkBase + "/" + r.bank, termBase + "/" + r.termC, taxBase + "/" + r.pph} {
			d := call(t, "DELETE", path, nil)
			if !want(t, d, 409) || !strings.Contains(d.str("message"), "mitra bisnis") {
				t.Errorf("DELETE %s: %s", path, trunc(d.Raw))
			}
		}
		sp := must(t, call(t, "GET", spBase+"/"+r.sales, nil), 200)
		if d := call(t, "DELETE", emBase+"/"+sp.str("employeeCode"), nil); !want(t, d, 409) || !strings.Contains(d.str("message"), "tenaga penjualan") {
			t.Errorf("employee delete %s", trunc(d.Raw))
		}
		// A deactivated sales person stays on the partner it already serves, but new partners cannot pick it.
		must(t, call(t, "PUT", spBase+"/"+r.sales, map[string]any{"active": false}), 200)
		must(t, call(t, "PUT", bpBase+"/"+p.str("code"), map[string]any{"name": "PT Masih Dilayani"}), 200)
		n := call(t, "POST", bpBase, r.customer(nil))
		if !want(t, n, 422) || !strings.Contains(n.problem("salesPersonCode"), "nonaktif") {
			t.Errorf("inactive sales person picked: %s", trunc(n.Raw))
		}
		must(t, call(t, "PUT", spBase+"/"+r.sales, map[string]any{"active": true}), 200)
		must(t, call(t, "DELETE", bpBase+"/"+p.str("code"), nil), 204)
		if g := call(t, "GET", bpBase+"/"+p.str("code"), nil); g.Status != 404 {
			t.Errorf("deleted partner still readable: %d", g.Status)
		}
	})
}

func TestPartnersNegative(t *testing.T) {
	r := newPartnerRefs(t)

	t.Run("BN01 the form's problems, row cells included", func(t *testing.T) {
		b := r.customer(map[string]any{"roles": []string{"customer", "buyer"}})
		if x := call(t, "POST", bpBase, b); !want(t, x, 422) || !strings.Contains(x.problem("roles"), "buyer") {
			t.Errorf("unknown role %s", trunc(x.Raw))
		}
		x := call(t, "POST", bpBase, map[string]any{"name": "", "entityType": "llc", "roles": []string{"customer"}, "npwp": "123", "pkp": true,
			"addresses": []map[string]any{{"address": ""}}, "contacts": []map[string]any{{"name": "Tanpa nomor"}},
			"bankAccounts": []map[string]any{{"bankCode": r.bank, "accountNumber": "12", "accountName": "x", "currencyCode": r.idr}}})
		if !want(t, x, 422) {
			return
		}
		for _, f := range []string{"name", "entityType", "countryCode", "npwp", "customerPaymentTermCode", "customerCurrencyCode", "addresses.0.address", "contacts.0.email", "bankAccounts.0.accountNumber"} {
			if x.problem(f) == "" {
				t.Errorf("no problem on %s: %s", f, trunc(x.Raw))
			}
		}
		if !strings.Contains(x.problem("addresses.0.address"), "baris 1") {
			t.Errorf("row message %q", x.problem("addresses.0.address"))
		}
	})

	t.Run("BN02 one live partner per NPWP (head office) and per NITKU", func(t *testing.T) {
		n := nextNpwp()
		body := func(nitku any) map[string]any {
			return map[string]any{"name": "PT Kembar", "entityType": "pt", "roles": []string{"shipper"}, "countryCode": r.country, "npwp": n, "nitku": nitku}
		}
		first := must(t, call(t, "POST", bpBase, body(nil)), 201)
		if x := call(t, "POST", bpBase, body(nil)); !want(t, x, 422) || !strings.Contains(x.problem("npwp"), first.str("code")) {
			t.Errorf("same NPWP %s", trunc(x.Raw))
		}
		must(t, call(t, "POST", bpBase, body(n+"000001")), 201)
		if x := call(t, "POST", bpBase, body(n+"000001")); !want(t, x, 422) || x.problem("nitku") == "" {
			t.Errorf("same NITKU %s", trunc(x.Raw))
		}
		// A deleted partner frees its NPWP.
		must(t, call(t, "DELETE", bpBase+"/"+first.str("code"), nil), 204)
		must(t, call(t, "POST", bpBase, body(nil)), 201)
	})

	t.Run("BN03 terms on the wrong side, PPN as the vendor's withholding, unknown and inactive masters", func(t *testing.T) {
		x := call(t, "POST", bpBase, r.customer(map[string]any{"customerPaymentTermCode": r.termV, "vendorPaymentTermCode": r.termC, "vendorTaxTypeCode": r.ppn}))
		if !want(t, x, 422) || x.problem("customerPaymentTermCode") == "" || x.problem("vendorPaymentTermCode") == "" || !strings.Contains(x.problem("vendorTaxTypeCode"), "PPh") {
			t.Errorf("sides %s", trunc(x.Raw))
		}
		b := r.customer(nil)
		b["addresses"] = []map[string]any{{"address": "x", "cityCode": "MKT999"}}
		b["bankAccounts"] = []map[string]any{{"bankCode": "NOPE", "accountNumber": "123456", "accountName": "x", "currencyCode": r.idr}}
		x = call(t, "POST", bpBase, b)
		if !want(t, x, 422) || !strings.Contains(x.problem("addresses.0.cityCode"), "tidak ditemukan") || x.problem("bankAccounts.0.bankCode") == "" {
			t.Errorf("unknown refs %s", trunc(x.Raw))
		}
		bank := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank Tutup", "active": false}), 201).str("code")
		b = r.customer(nil)
		b["bankAccounts"] = []map[string]any{{"bankCode": bank, "accountNumber": "123456", "accountName": "x", "currencyCode": r.idr}}
		if x = call(t, "POST", bpBase, b); !want(t, x, 422) || !strings.Contains(x.problem("bankAccounts.0.bankCode"), "nonaktif") {
			t.Errorf("inactive bank %s", trunc(x.Raw))
		}
		if x = call(t, "POST", spBase, map[string]any{"name": "Dobel", "employeeCode": must(t, call(t, "GET", spBase+"/"+r.sales, nil), 200).str("employeeCode")}); !want(t, x, 422) || x.problem("employeeCode") == "" {
			t.Errorf("second sales person for one employee %s", trunc(x.Raw))
		}
	})

	t.Run("BN04 stale version, unknown code, wrong JSON, another tenant", func(t *testing.T) {
		p := must(t, call(t, "POST", bpBase, r.customer(nil)), 201)
		must(t, call(t, "PUT", bpBase+"/"+p.str("code"), map[string]any{"name": "Satu"}), 200)
		if x := call(t, "PUT", bpBase+"/"+p.str("code"), map[string]any{"name": "Dua", "version": p.Obj["version"]}); !want(t, x, 409) || x.str("code") != "STALE" {
			t.Errorf("stale %s", trunc(x.Raw))
		}
		if x := call(t, "GET", bpBase+"/MMB9999", nil); x.Status != 404 {
			t.Errorf("unknown %d", x.Status)
		}
		if x := call(t, "POST", bpBase, "{bad"); x.Status != 400 {
			t.Errorf("bad JSON %d", x.Status)
		}
		if x := call(t, "PUT", bpBase+"/"+p.str("code"), map[string]any{"addresses": "Jl. A"}); !want(t, x, 422) || x.problem("addresses") == "" {
			t.Errorf("list as text %s", trunc(x.Raw))
		}
		if x := call(t, "GET", bpBase+"/"+p.str("code"), nil, as(tenantB)...); x.Status != 404 {
			t.Errorf("visible to another tenant: %d", x.Status)
		}
		if x := call(t, "GET", bpBase, nil, as(tenantB)...); want(t, x, 200) && len(x.List) != 0 {
			t.Errorf("another tenant lists %d partners", len(x.List))
		}
	})
}

func TestPartnersStress(t *testing.T) {
	r := newPartnerRefs(t)

	t.Run("BS01 40 partners saved at once get 40 codes with their own lists", func(t *testing.T) {
		var fails atomic.Int64
		codes := sync.Map{}
		parallel(40, 10, func(i int) {
			x := call(t, "POST", bpBase, r.customer(map[string]any{"name": fmt.Sprintf("PT Serbu %d", i)}))
			if x.Status != 201 || len(rowsOf(x, "bankAccounts")) != 2 {
				fails.Add(1)
				return
			}
			if _, dup := codes.LoadOrStore(x.str("code"), true); dup {
				fails.Add(1)
			}
		})
		if fails.Load() != 0 {
			t.Errorf("%d failures", fails.Load())
		}
	})

	t.Run("BS02 one NPWP saved 10 times at once is stored once", func(t *testing.T) {
		n := nextNpwp()
		var created, fails atomic.Int64
		parallel(10, 10, func(int) {
			switch x := call(t, "POST", bpBase, map[string]any{"name": "PT Rebutan", "entityType": "pt", "roles": []string{"shipper"}, "countryCode": r.country, "npwp": n}); x.Status {
			case 201:
				created.Add(1)
			case 409, 422:
			default:
				fails.Add(1)
			}
		})
		if created.Load() != 1 || fails.Load() != 0 {
			t.Errorf("created %d, failures %d", created.Load(), fails.Load())
		}
	})

	t.Run("BS03 20 list replacements at once leave exactly one whole list", func(t *testing.T) {
		p := must(t, call(t, "POST", bpBase, r.customer(nil)), 201)
		var fails atomic.Int64
		parallel(20, 10, func(i int) {
			rows := make([]map[string]any, i%5+1)
			for j := range rows {
				rows[j] = map[string]any{"name": fmt.Sprintf("K%d-%d", i, j), "phone": "0811555"}
			}
			if x := call(t, "PUT", bpBase+"/"+p.str("code"), map[string]any{"contacts": rows}); x.Status != 200 {
				fails.Add(1)
			}
		})
		g := must(t, call(t, "GET", bpBase+"/"+p.str("code"), nil), 200)
		con := rowsOf(g, "contacts")
		prefix := strings.SplitN(con[0]["name"].(string), "-", 2)[0]
		primaries := 0
		for j, c := range con {
			if c["name"] != fmt.Sprintf("%s-%d", prefix, j) {
				t.Errorf("mixed lists: %v", con)
				break
			}
			if c["primary"] == true {
				primaries++
			}
		}
		if fails.Load() != 0 || primaries != 1 || g.Obj["version"] != 21.0 {
			t.Errorf("failures %d, primaries %d, version %v", fails.Load(), primaries, g.Obj["version"])
		}
	})
}
