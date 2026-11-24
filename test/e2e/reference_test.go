//go:build e2e

// Reference masters every module shares: Mata Uang, Negara, Kota (Geografi & Lokasi), Syarat Pembayaran, Jenis Pajak
// (Keuangan & Pajak), and Bank with SWIFT and country. Positive, negative and stress.
package e2e

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	curBase  = "/pm/masters/currencies"
	ctyBase  = "/pm/masters/countries"
	cityBase = "/pm/masters/cities"
	termBase = "/pm/masters/payment-terms"
	taxBase  = "/pm/masters/tax-types"
)

// byKey is the list row whose field equals value.
func byKey(rows []map[string]any, field, value string) map[string]any {
	for _, r := range rows {
		if r[field] == value {
			return r
		}
	}
	return nil
}

var refSeq atomic.Int64

// uniq is a short unique tag of the run ("A1B2…"), for ISO-like test codes that must not clash.
func letters(n int64, size int) string {
	b := make([]byte, size)
	for i := size - 1; i >= 0; i-- {
		b[i] = byte('A' + n%26)
		n /= 26
	}
	return string(b)
}

// testCountry makes a country of its own (ISO codes no standard country uses: X?, X??).
func testCountry(t testing.TB) map[string]any {
	n := refSeq.Add(1)
	return must(t, call(t, "POST", ctyBase, map[string]any{"name": "Negara Uji " + fmt.Sprint(n), "iso2": "X" + letters(n, 1), "iso3": "X" + letters(n, 2)}), 201).Obj
}

func TestReferencePositive(t *testing.T) {
	t.Run("RF01 a company starts with the standard currencies and countries", func(t *testing.T) {
		cur := must(t, call(t, "GET", curBase, nil), 200).List
		idr, jpy := byKey(cur, "isoCode", "IDR"), byKey(cur, "isoCode", "JPY")
		if len(cur) < 36 || idr == nil || jpy == nil || idr["decimals"] != 2.0 || jpy["decimals"] != 0.0 || !genCodeOf("MMU").MatchString(idr["code"].(string)) {
			t.Fatalf("currencies %d idr %v jpy %v", len(cur), idr, jpy)
		}
		cty := must(t, call(t, "GET", ctyBase, nil), 200).List
		id := byKey(cty, "iso2", "ID")
		if len(cty) < 249 || id == nil || id["iso3"] != "IDN" || id["phoneCode"] != "+62" || id["currencyCode"] != idr["code"] || id["name"] != "Indonesia" {
			t.Fatalf("countries %d id %v", len(cty), id)
		}
		// The whole of ISO 3166-1: a forwarder's consignees are anywhere; a country without the currency in the
		// master has none linked.
		for iso, name := range map[string]string{"SN": "Senegal", "PG": "Papua Nugini", "HT": "Haiti", "AF": "Afganistan", "ZW": "Zimbabwe"} {
			if c := byKey(cty, "iso2", iso); c == nil || c["name"] != name || c["currencyCode"] != nil {
				t.Errorf("%s: %v", iso, c)
			}
		}
	})

	t.Run("RF02 a currency: ISO code upper-cased, decimals default 2, a generated code", func(t *testing.T) {
		r := must(t, call(t, "POST", curBase, map[string]any{"code": "typed", "name": "Mata Uang Uji", "isoCode": "xts", "symbol": "¤"}), 201)
		if r.str("isoCode") != "XTS" || r.Obj["decimals"] != 2.0 || !genCodeOf("MMU").MatchString(r.str("code")) || r.str("code") == "typed" {
			t.Errorf("created %s", r.Raw)
		}
		u := must(t, call(t, "PUT", curBase+"/"+r.str("code"), map[string]any{"decimals": 0, "version": r.Obj["version"]}), 200)
		if u.Obj["decimals"] != 0.0 || u.str("isoCode") != "XTS" || u.Obj["version"] != 2.0 {
			t.Errorf("updated %s", u.Raw)
		}
	})

	t.Run("RF03 countries and their cities: same name in another province is another city", func(t *testing.T) {
		c := testCountry(t)
		r := must(t, call(t, "POST", cityBase, map[string]any{"name": "Kota Baru", "countryCode": c["code"], "province": "Provinsi Satu", "kind": "regency", "areaCode": "63.02"}), 201)
		if r.str("countryCode") != c["code"] || r.str("kind") != "regency" || r.str("province") != "Provinsi Satu" || !genCodeOf("MKT").MatchString(r.str("code")) {
			t.Errorf("city %s", r.Raw)
		}
		must(t, call(t, "POST", cityBase, map[string]any{"name": "Kota Baru", "countryCode": c["code"], "province": "Provinsi Dua"}), 201)
		if d := call(t, "POST", cityBase, map[string]any{"name": "kota baru", "countryCode": c["code"], "province": "provinsi satu"}); d.Status != 422 || !strings.Contains(d.problem("name"), "sudah ada") {
			t.Errorf("same city twice: %d %s", d.Status, d.Raw)
		}
	})

	t.Run("RF04 payment terms: days from a basis, a down payment, for whom", func(t *testing.T) {
		a := must(t, call(t, "POST", termBase, map[string]any{"name": "NET 30", "days": 30}), 201)
		if a.Obj["days"] != 30.0 || a.str("basis") != "invoice_date" || a.Obj["downPaymentPct"] != 0.0 || a.str("appliesTo") != "both" || !genCodeOf("MTP").MatchString(a.str("code")) {
			t.Errorf("defaults %s", a.Raw)
		}
		b := must(t, call(t, "POST", termBase, map[string]any{"name": "DP 30% sisa 14 hari setelah B/L", "days": 14, "basis": "bl_date", "downPaymentPct": 30, "appliesTo": "customer"}), 201)
		if b.str("basis") != "bl_date" || b.Obj["downPaymentPct"] != 30.0 || b.str("appliesTo") != "customer" {
			t.Errorf("with a down payment %s", b.Raw)
		}
		must(t, call(t, "POST", termBase, map[string]any{"name": "Tunai", "days": 0}), 201)
	})

	t.Run("RF05 tax types: PPN added with a tax base share, PPh withheld; the effective rate is answered", func(t *testing.T) {
		ppn := must(t, call(t, "POST", taxBase, map[string]any{"name": "PPN 12% DPP 11/12", "category": "ppn", "ratePct": 12, "basePct": 91.6667, "validFrom": "2025-01-01"}), 201)
		if ppn.str("category") != "PPN" || ppn.str("method") != "added" || ppn.Obj["effectivePct"] != 11.0 || ppn.str("validFrom") != "2025-01-01" || !genCodeOf("MPJ").MatchString(ppn.str("code")) {
			t.Errorf("PPN %s", ppn.Raw)
		}
		pph := must(t, call(t, "POST", taxBase, map[string]any{"name": "PPh 23 jasa", "category": "PPH23", "ratePct": 2}), 201)
		if pph.str("method") != "withheld" || pph.Obj["basePct"] != 100.0 || pph.Obj["effectivePct"] != 2.0 {
			t.Errorf("PPh 23 %s", pph.Raw)
		}
	})

	t.Run("RF06 a bank with SWIFT/BIC (upper-cased) and its country", func(t *testing.T) {
		id := byKey(must(t, call(t, "GET", ctyBase, nil), 200).List, "iso2", "ID")
		r := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank Uji Internasional", "clearing": "999", "swiftCode": "brinidja", "countryCode": id["code"]}), 201)
		if r.str("swiftCode") != "BRINIDJA" || r.str("countryCode") != id["code"] || r.str("clearing") != "999" {
			t.Errorf("bank %s", r.Raw)
		}
	})
}

func TestReferenceNegative(t *testing.T) {
	t.Run("RN01 currency: ISO code form and uniqueness, decimals", func(t *testing.T) {
		for name, x := range map[string]map[string]any{
			"two letters": {"isoCode": "US"}, "four letters": {"isoCode": "USDX"}, "digits": {"isoCode": "123"}, "taken": {"isoCode": "idr"},
			"no iso": {}, "decimals 5": {"isoCode": "XTA", "decimals": 5}, "decimals text": {"isoCode": "XTB", "decimals": "dua"},
		} {
			body := map[string]any{"name": "Mata Uang Salah"}
			for k, v := range x {
				body[k] = v
			}
			if r := call(t, "POST", curBase, body); r.Status != 422 {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	t.Run("RN02 country: ISO codes, dialling code, currency", func(t *testing.T) {
		for name, x := range map[string]map[string]any{
			"iso2 one letter": {"iso2": "I"}, "iso2 digits": {"iso2": "12"}, "iso2 taken": {"iso2": "id"}, "iso3 taken": {"iso2": "QQ", "iso3": "idn"},
			"iso3 bad": {"iso2": "QR", "iso3": "I1N"}, "phone without +": {"iso2": "QS", "phoneCode": "62"}, "phone too long": {"iso2": "QT", "phoneCode": "+12345"},
			"unknown currency": {"iso2": "QU", "currencyCode": "MMU999"},
		} {
			body := map[string]any{"name": "Negara Salah"}
			for k, v := range x {
				body[k] = v
			}
			if r := call(t, "POST", ctyBase, body); r.Status != 422 {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	t.Run("RN03 city: country required and known, kind, region code", func(t *testing.T) {
		c := testCountry(t)
		for name, x := range map[string]map[string]any{
			"no country": {}, "unknown country": {"countryCode": "MNG999"}, "bad kind": {"countryCode": c["code"], "kind": "desa"},
			"bad region code": {"countryCode": c["code"], "areaCode": "63 02"},
		} {
			body := map[string]any{"name": "Kota Salah"}
			for k, v := range x {
				body[k] = v
			}
			if r := call(t, "POST", cityBase, body); r.Status != 422 {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	t.Run("RN04 payment term: days 0–365, a known basis, down payment 0–100 with 2 decimals", func(t *testing.T) {
		for name, x := range map[string]map[string]any{
			"366 days": {"days": 366}, "negative": {"days": -1}, "fraction": {"days": 1.5}, "no days": {}, "bad basis": {"days": 7, "basis": "kapan_saja"},
			"dp 101": {"days": 7, "downPaymentPct": 101}, "dp 3 decimals": {"days": 7, "downPaymentPct": 2.555}, "bad applies": {"days": 7, "appliesTo": "semua"},
		} {
			body := map[string]any{"name": "Termin Salah"}
			for k, v := range x {
				body[k] = v
			}
			if r := call(t, "POST", termBase, body); r.Status != 422 {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	t.Run("RN05 tax type: the method follows the kind, rates in range with 4 decimals", func(t *testing.T) {
		for name, x := range map[string]map[string]any{
			"PPN withheld": {"category": "PPN", "ratePct": 12, "method": "withheld"}, "PPh added": {"category": "PPH23", "ratePct": 2, "method": "added"},
			"rate 101": {"category": "PPN", "ratePct": 101}, "rate 5 decimals": {"category": "PPN", "ratePct": 1.23456}, "no rate": {"category": "PPN"},
			"base 0": {"category": "PPN", "ratePct": 12, "basePct": 0}, "unknown kind": {"category": "VAT", "ratePct": 10},
		} {
			body := map[string]any{"name": "Pajak Salah"}
			for k, v := range x {
				body[k] = v
			}
			if r := call(t, "POST", taxBase, body); r.Status != 422 {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	t.Run("RN06 a currency or country is global: never deleted (deactivated instead)", func(t *testing.T) {
		cur := must(t, call(t, "POST", curBase, map[string]any{"name": "Mata Uang Global", "isoCode": "XTC"}), 201)
		if r := call(t, "DELETE", curBase+"/"+cur.str("code"), nil); r.Status != 409 || !strings.Contains(r.str("message"), "nonaktifkan") {
			t.Errorf("currency delete: %d %s", r.Status, r.Raw)
		}
		c := testCountry(t)
		if r := call(t, "DELETE", ctyBase+"/"+c["code"].(string), nil); r.Status != 409 || !strings.Contains(r.str("message"), "global") {
			t.Errorf("country delete: %d %s", r.Status, r.Raw)
		}
		if u := must(t, call(t, "PUT", curBase+"/"+cur.str("code"), map[string]any{"active": false}), 200); u.Obj["active"] != false {
			t.Errorf("deactivate %s", trunc(u.Raw))
		}
	})

	t.Run("RN07 bank: SWIFT form and uniqueness, a known country", func(t *testing.T) {
		must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank SWIFT Satu", "swiftCode": "TESTIDJA"}), 201)
		for name, x := range map[string]map[string]any{
			"short": {"swiftCode": "TEST"}, "bad chars": {"swiftCode": "TEST-DJA"}, "taken": {"swiftCode": "testidja"}, "unknown country": {"countryCode": "MNG999"},
		} {
			body := map[string]any{"name": "Bank Salah"}
			for k, v := range x {
				body[k] = v
			}
			if r := call(t, "POST", bkBase, body); r.Status != 422 {
				t.Errorf("%s: %d %s", name, r.Status, r.Raw)
			}
		}
	})

	t.Run("RN08 global: every company reads the same rows; an ISO code is one for all companies", func(t *testing.T) {
		mine := must(t, call(t, "POST", curBase, map[string]any{"name": "Mata Uang Bersama", "isoCode": "XTD"}), 201)
		other := must(t, call(t, "GET", curBase, nil, as(tenantB)...), 200).List
		if x := byKey(other, "isoCode", "XTD"); x == nil || x["code"] != mine.str("code") {
			t.Errorf("tenant B sees the global row: %v", x)
		}
		if a, b := len(must(t, call(t, "GET", ctyBase, nil), 200).List), len(must(t, call(t, "GET", ctyBase, nil, as(tenantB)...), 200).List); a != b || a < 249 {
			t.Errorf("countries: A %d, B %d", a, b)
		}
		if x := call(t, "POST", curBase, map[string]any{"name": "Mata Uang B", "isoCode": "XTD"}, as(tenantB)...); !want(t, x, 422) || x.problem("isoCode") == "" {
			t.Errorf("ISO code taken for all: %s", trunc(x.Raw))
		}
		// A company's own record points to the global row.
		id := byKey(must(t, call(t, "GET", ctyBase, nil, as(tenantB)...), 200).List, "iso2", "ID")
		if bk := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank Tenant B", "countryCode": id["code"]}, as(tenantB)...), 201); bk.str("countryCode") != id["code"] {
			t.Errorf("bank of B %s", trunc(bk.Raw))
		}
	})
}

func TestReferenceStress(t *testing.T) {
	c := testCountry(t)
	t.Run("RS01 30 cities saved at once get 30 codes; one city saved 10 times at once is stored once", func(t *testing.T) {
		var fails atomic.Int64
		codes := sync.Map{}
		parallel(30, 10, func(i int) {
			r := call(t, "POST", cityBase, map[string]any{"name": fmt.Sprintf("Kota Serbu %d", i), "countryCode": c["code"]})
			if r.Status != 201 {
				fails.Add(1)
				return
			}
			if _, dup := codes.LoadOrStore(r.str("code"), true); dup {
				fails.Add(1)
			}
		})
		var created atomic.Int64
		parallel(10, 10, func(int) {
			switch r := call(t, "POST", cityBase, map[string]any{"name": "Kota Kembar", "countryCode": c["code"]}); r.Status {
			case 201:
				created.Add(1)
			case 409, 422:
			default:
				fails.Add(1)
			}
		})
		if fails.Load() != 0 || created.Load() != 1 {
			t.Errorf("failures %d, the same city created %d times", fails.Load(), created.Load())
		}
	})
}
