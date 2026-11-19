//go:build e2e

// Kurs, Kode Biaya, Rekening Bank Perusahaan (Keuangan & Pajak), Jenis Dokumen (Dokumen) and Moda & Layanan.
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
	fxBase  = "/pm/masters/exchange-rates"
	chBase  = "/pm/masters/charges"
	cbaBase = "/pm/masters/company-bank-accounts"
	docBase = "/pm/masters/document-types"
	svcBase = "/pm/masters/service-types"
)

type finRefs struct{ idr, usd, sgd, ppn, pph, gp20, hc40, bank string }

func newFinRefs(t testing.TB) finRefs {
	t.Helper()
	cur := must(t, call(t, "GET", curBase, nil), 200).List
	ctn := must(t, call(t, "GET", ctnBase, nil), 200).List
	return finRefs{
		idr: byKey(cur, "isoCode", "IDR")["code"].(string), usd: byKey(cur, "isoCode", "USD")["code"].(string), sgd: byKey(cur, "isoCode", "SGD")["code"].(string),
		ppn:  must(t, call(t, "POST", taxBase, map[string]any{"name": "PPN uji", "category": "PPN", "ratePct": 12, "basePct": 91.6667}), 201).str("code"),
		pph:  must(t, call(t, "POST", taxBase, map[string]any{"name": "PPh 23 uji", "category": "PPH23", "ratePct": 2}), 201).str("code"),
		gp20: byKey(ctn, "isoCode", "22G1")["code"].(string), hc40: byKey(ctn, "isoCode", "45G1")["code"].(string),
		bank: must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank Perusahaan Uji"}), 201).str("code"),
	}
}

var fxDay atomic.Int64

// nextDay is a date no other rate test uses (from 2030-01-01 on).
func nextDay() string {
	n := fxDay.Add(1)
	return fmt.Sprintf("2030-%02d-%02d", 1+(n/28)%12, 1+n%28)
}

func TestFinanceDocsPositive(t *testing.T) {
	r := newFinRefs(t)

	t.Run("FP01 a company starts with the common document types and services", func(t *testing.T) {
		docs := must(t, call(t, "GET", docBase, nil), 200).List
		pib := byKey(docs, "customsCode", "BC 2.0")
		if len(docs) < 17 || pib == nil || pib["forImport"] != true || pib["forExport"] != false || pib["group"] != "customs" || !genCodeOf("MJD").MatchString(pib["code"].(string)) {
			t.Fatalf("documents %d pib %v", len(docs), pib)
		}
		svc := must(t, call(t, "GET", svcBase, nil), 200).List
		fcl := byKey(svc, "name", "Sea Freight FCL")
		if len(svc) < 10 || fcl == nil || fcl["needsContainer"] != true || fcl["mode"] != "sea" {
			t.Fatalf("services %d fcl %v", len(svc), fcl)
		}
	})

	t.Run("FP02 rates are history: the rate on a date is the latest valid then; rupiah by default; a made name", func(t *testing.T) {
		d1, d2 := "2029-11-12", "2029-11-19"
		a := must(t, call(t, "POST", fxBase, map[string]any{"fromCurrencyCode": r.usd, "rateType": "tax", "rate": 16012.5, "validFrom": d1, "source": "KMK uji"}), 201)
		b := must(t, call(t, "POST", fxBase, map[string]any{"fromCurrencyCode": r.usd, "rateType": "tax", "rate": 16100, "validFrom": d2}), 201)
		if a.str("toCurrencyCode") != r.idr || a.str("name") != "USD/IDR Kurs pajak "+d1 || a.Obj["rate"] != 16012.5 || !genCodeOf("MKR").MatchString(a.str("code")) || len(a.str("code")) != 9 {
			t.Errorf("rate %s", trunc(a.Raw))
		}
		if x := must(t, call(t, "GET", fxBase+"/lookup?from=USD&date=2029-11-15", nil), 200); x.str("code") != a.str("code") {
			t.Errorf("15 Nov gives %s, want %s", x.str("code"), a.str("code"))
		}
		if x := must(t, call(t, "GET", fxBase+"/lookup?from="+r.usd+"&to=IDR&type=tax&date=2029-12-01", nil), 200); x.str("code") != b.str("code") {
			t.Errorf("1 Dec gives %s", x.str("code"))
		}
		if x := call(t, "GET", fxBase+"/lookup?from=USD&type=bank_buy&date=2029-12-01", nil); x.Status != 404 {
			t.Errorf("no bank rate: %d", x.Status)
		}
		if x := call(t, "GET", fxBase+"/lookup?from=USD&date=2029-11-01", nil); x.Status != 404 {
			t.Errorf("before any rate: %d", x.Status)
		}
		must(t, call(t, "PUT", fxBase+"/"+b.str("code"), map[string]any{"active": false}), 200)
		if x := must(t, call(t, "GET", fxBase+"/lookup?from=USD&date=2029-12-01", nil), 200); x.str("code") != a.str("code") {
			t.Errorf("an inactive rate is skipped: %s", x.str("code"))
		}
		if d := call(t, "DELETE", curBase+"/"+r.sgd, nil); d.Status != 204 && d.Status != 409 {
			t.Errorf("currency delete %d", d.Status)
		}
	})

	t.Run("FP03 one charge code for both sides: rates per container type, PPN and PPh; sides and basis clear what no longer fits", func(t *testing.T) {
		c := must(t, call(t, "POST", chBase, map[string]any{"name": "Ocean Freight", "group": "freight", "basis": "container", "currencyCode": r.usd,
			"ppnTaxTypeCode": r.ppn, "pphTaxTypeCode": r.pph, "accountCode": "4101",
			"rates": []map[string]any{{"containerTypeCode": r.gp20, "currencyCode": r.usd, "rate": 650}, {"containerTypeCode": r.hc40, "currencyCode": r.usd, "rate": 1200.5}}}), 201)
		if c.Obj["receivable"] != true || c.Obj["payable"] != true || len(rowsOf(c, "rates")) != 2 || rowsOf(c, "rates")[1]["rate"] != 1200.5 ||
			c.str("ppnTaxTypeCode") != r.ppn || !genCodeOf("MKB").MatchString(c.str("code")) {
			t.Errorf("charge %s", trunc(c.Raw))
		}
		if d := call(t, "DELETE", ctnBase+"/"+r.gp20, nil); !want(t, d, 409) || !strings.Contains(d.str("message"), "kode biaya") {
			t.Errorf("container in use %s", trunc(d.Raw))
		}
		u := must(t, call(t, "PUT", chBase+"/"+c.str("code"), map[string]any{"payable": false, "basis": "shipment", "defaultRate": 100}), 200)
		if u.Obj["pphTaxTypeCode"] != nil || len(rowsOf(u, "rates")) != 0 || u.str("ppnTaxTypeCode") != r.ppn || u.Obj["defaultRate"] != 100.0 {
			t.Errorf("cleared %s", trunc(u.Raw))
		}
		p := must(t, call(t, "POST", chBase, map[string]any{"name": "Asuransi", "group": "insurance", "basis": "percent", "defaultRate": 0.15, "payable": false}), 201)
		if p.Obj["defaultRate"] != 0.15 || p.Obj["currencyCode"] != nil {
			t.Errorf("percent %s", trunc(p.Raw))
		}
	})

	t.Run("FP04 company accounts: one invoice default per currency and company; a bank in use stays", func(t *testing.T) {
		a := must(t, call(t, "POST", cbaBase, map[string]any{"name": "Operasional IDR", "bankCode": r.bank, "accountNumber": "123 456 7890", "accountName": "PT Uji",
			"currencyCode": r.idr, "purpose": "receipt", "invoiceDefault": true}), 201)
		if a.str("accountNumber") != "1234567890" || a.Obj["invoiceDefault"] != true || !genCodeOf("MRP").MatchString(a.str("code")) {
			t.Errorf("account %s", trunc(a.Raw))
		}
		must(t, call(t, "POST", cbaBase, map[string]any{"name": "Operasional USD", "bankCode": r.bank, "accountNumber": "1234567891", "accountName": "PT Uji",
			"currencyCode": r.usd, "invoiceDefault": true}), 201)
		if d := call(t, "DELETE", bkBase+"/"+r.bank, nil); !want(t, d, 409) || !strings.Contains(d.str("message"), "rekening perusahaan") {
			t.Errorf("bank in use %s", trunc(d.Raw))
		}
	})

	t.Run("FP05 a service: rail FCL needs containers", func(t *testing.T) {
		s := must(t, call(t, "POST", svcBase, map[string]any{"name": "Rail FCL", "mode": "rail", "loadType": "fcl", "needsContainer": false}), 201)
		if s.Obj["needsContainer"] != true || !genCodeOf("MML").MatchString(s.str("code")) {
			t.Errorf("service %s", trunc(s.Raw))
		}
	})
}

func TestFinanceDocsNegative(t *testing.T) {
	r := newFinRefs(t)

	t.Run("FN01 rates: one per pair, type and date; a pair of two currencies; date and rate required; lookup checks its query", func(t *testing.T) {
		day := nextDay()
		a := must(t, call(t, "POST", fxBase, map[string]any{"fromCurrencyCode": r.sgd, "rateType": "bi_middle", "rate": 12000, "validFrom": day}), 201)
		if x := call(t, "POST", fxBase, map[string]any{"fromCurrencyCode": r.sgd, "rateType": "bi_middle", "rate": 12001, "validFrom": day}); !want(t, x, 422) || !strings.Contains(x.problem("validFrom"), a.str("code")) {
			t.Errorf("duplicate %s", trunc(x.Raw))
		}
		if x := call(t, "POST", fxBase, map[string]any{"fromCurrencyCode": r.idr, "toCurrencyCode": r.idr, "rate": 1, "validFrom": day}); !want(t, x, 422) || x.problem("toCurrencyCode") == "" {
			t.Errorf("same currency %s", trunc(x.Raw))
		}
		if x := call(t, "POST", fxBase, map[string]any{"fromCurrencyCode": r.usd, "rate": 0, "rateType": "spot"}); !want(t, x, 422) || x.problem("validFrom") == "" || x.problem("rate") == "" || x.problem("rateType") == "" {
			t.Errorf("required %s", trunc(x.Raw))
		}
		if x := call(t, "GET", fxBase+"/lookup?type=spot&date=19-11-2029", nil); !want(t, x, 422) || x.problem("from") == "" || x.problem("type") == "" || x.problem("date") == "" {
			t.Errorf("lookup query %s", trunc(x.Raw))
		}
	})

	t.Run("FN02 charge codes: a side, the right taxes, a currency for the rate, a percentage, rate rows", func(t *testing.T) {
		x := call(t, "POST", chBase, map[string]any{"name": "Salah", "group": "freight", "basis": "shipment", "receivable": false, "payable": false})
		if !want(t, x, 422) || x.problem("receivable") == "" {
			t.Errorf("no side %s", trunc(x.Raw))
		}
		x = call(t, "POST", chBase, map[string]any{"name": "Pajak tertukar", "group": "freight", "basis": "shipment", "ppnTaxTypeCode": r.pph, "pphTaxTypeCode": r.ppn, "defaultRate": 5})
		if !want(t, x, 422) || !strings.Contains(x.problem("ppnTaxTypeCode"), "PPN") || !strings.Contains(x.problem("pphTaxTypeCode"), "PPh") || x.problem("currencyCode") == "" {
			t.Errorf("taxes %s", trunc(x.Raw))
		}
		if x = call(t, "POST", chBase, map[string]any{"name": "Persen", "group": "insurance", "basis": "percent", "defaultRate": 150}); !want(t, x, 422) || x.problem("defaultRate") == "" {
			t.Errorf("percent %s", trunc(x.Raw))
		}
		x = call(t, "POST", chBase, map[string]any{"name": "Tarif", "group": "origin", "basis": "container", "rates": []map[string]any{
			{"containerTypeCode": r.gp20, "currencyCode": r.idr, "rate": 1}, {"containerTypeCode": r.gp20, "currencyCode": r.idr, "rate": 2}, {"containerTypeCode": "MKN999", "currencyCode": r.idr}}})
		if !want(t, x, 422) || !strings.Contains(x.problem("rates.1.containerTypeCode"), "baris 1") || x.problem("rates.2.rate") == "" || x.problem("rates.2.containerTypeCode") == "" {
			t.Errorf("rows %s", trunc(x.Raw))
		}
		if x = call(t, "POST", chBase, map[string]any{"name": "Tanpa dasar", "group": "freight"}); !want(t, x, 422) || x.problem("basis") == "" {
			t.Errorf("basis %s", trunc(x.Raw))
		}
	})

	t.Run("FN03 company accounts: once per bank and number; payroll accounts are not on invoices; a second invoice default", func(t *testing.T) {
		body := map[string]any{"name": "Rek A", "bankCode": r.bank, "accountNumber": "5550001", "accountName": "PT Uji", "currencyCode": r.sgd, "invoiceDefault": true}
		must(t, call(t, "POST", cbaBase, body), 201)
		if x := call(t, "POST", cbaBase, body); !want(t, x, 422) || x.problem("accountNumber") == "" || x.problem("invoiceDefault") == "" {
			t.Errorf("duplicate %s", trunc(x.Raw))
		}
		if x := call(t, "POST", cbaBase, map[string]any{"name": "Gaji", "bankCode": r.bank, "accountNumber": "5550002", "accountName": "PT Uji", "currencyCode": r.idr, "purpose": "payroll", "invoiceDefault": true}); !want(t, x, 422) || x.problem("invoiceDefault") == "" {
			t.Errorf("payroll %s", trunc(x.Raw))
		}
		if x := call(t, "POST", cbaBase, map[string]any{"name": "Kurang"}); !want(t, x, 422) || x.problem("bankCode") == "" || x.problem("accountNumber") == "" || x.problem("currencyCode") == "" {
			t.Errorf("required %s", trunc(x.Raw))
		}
	})

	t.Run("FN04 document types and services: BC codes once, a scope; loads that fit their mode", func(t *testing.T) {
		if x := call(t, "POST", docBase, map[string]any{"name": "PIB lagi", "group": "customs", "customsCode": "bc  2.0"}); !want(t, x, 422) || x.problem("customsCode") == "" {
			t.Errorf("BC 2.0 twice %s", trunc(x.Raw))
		}
		if x := call(t, "POST", docBase, map[string]any{"name": "Tanpa cakupan", "group": "other", "forExport": false, "forImport": false}); !want(t, x, 422) || x.problem("forExport") == "" {
			t.Errorf("scope %s", trunc(x.Raw))
		}
		if x := call(t, "POST", svcBase, map[string]any{"name": "LCL udara", "mode": "air", "loadType": "lcl"}); !want(t, x, 422) || x.problem("loadType") == "" {
			t.Errorf("load %s", trunc(x.Raw))
		}
		if x := call(t, "POST", svcBase, map[string]any{"name": "Roket", "mode": "space"}); !want(t, x, 422) || x.problem("mode") == "" {
			t.Errorf("mode %s", trunc(x.Raw))
		}
	})
}

func TestFinanceDocsStress(t *testing.T) {
	r := newFinRefs(t)

	t.Run("FS01 30 rates on 30 dates at once get 30 codes; one rate saved 10 times at once is stored once", func(t *testing.T) {
		var fails, created atomic.Int64
		codes := sync.Map{}
		parallel(30, 10, func(i int) {
			x := call(t, "POST", fxBase, map[string]any{"fromCurrencyCode": r.usd, "rateType": "internal", "rate": 16000 + i, "validFrom": nextDay()})
			if x.Status != 201 {
				fails.Add(1)
				return
			}
			if _, dup := codes.LoadOrStore(x.str("code"), true); dup {
				fails.Add(1)
			}
		})
		day := nextDay()
		parallel(10, 10, func(int) {
			switch x := call(t, "POST", fxBase, map[string]any{"fromCurrencyCode": r.usd, "rateType": "bank_sell", "rate": 16200, "validFrom": day}); x.Status {
			case 201:
				created.Add(1)
			case 409, 422:
			default:
				fails.Add(1)
			}
		})
		if fails.Load() != 0 || created.Load() != 1 {
			t.Errorf("failures %d, the same rate created %d times", fails.Load(), created.Load())
		}
	})

	t.Run("FS02 20 rate-list replacements of one charge at once leave one whole list", func(t *testing.T) {
		c := must(t, call(t, "POST", chBase, map[string]any{"name": "THC", "group": "origin", "basis": "container"}), 201)
		var fails atomic.Int64
		parallel(20, 10, func(i int) {
			rows := []map[string]any{{"containerTypeCode": r.gp20, "currencyCode": r.idr, "rate": 1000 + i}, {"containerTypeCode": r.hc40, "currencyCode": r.idr, "rate": 2000 + i}}
			if x := call(t, "PUT", chBase+"/"+c.str("code"), map[string]any{"rates": rows}); x.Status != 200 {
				fails.Add(1)
			}
		})
		g := must(t, call(t, "GET", chBase+"/"+c.str("code"), nil), 200)
		rates := rowsOf(g, "rates")
		if fails.Load() != 0 || len(rates) != 2 || rates[1]["rate"].(float64)-rates[0]["rate"].(float64) != 1000 || g.Obj["version"] != 21.0 {
			t.Errorf("failures %d, rates %v, version %v", fails.Load(), rates, g.Obj["version"])
		}
	})
}
