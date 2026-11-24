//go:build e2e

// Logistics masters: Pelabuhan & Lokasi, Incoterms, Jenis Kontainer, Kemasan, Satuan and Komoditas.
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
	locBase = "/pm/masters/ports"
	incBase = "/pm/masters/incoterms"
	ctnBase = "/pm/masters/containers"
	pkgBase = "/pm/masters/packagings"
	uomBase = "/pm/masters/uoms"
	cmdBase = "/pm/masters/commodities"
)

var locSeq atomic.Int64

// nextLocode is a UN/LOCODE of country iso2 no other test uses.
func nextLocode(iso2 string) string { return iso2 + letters(locSeq.Add(1), 3) }

func TestLogisticsPositive(t *testing.T) {
	t.Run("LP01 a company starts with Incoterms 2020, the common container types, packagings and units", func(t *testing.T) {
		inc := must(t, call(t, "GET", incBase, nil), 200).List
		fob, cif, ddp := byKey(inc, "term", "FOB"), byKey(inc, "term", "CIF"), byKey(inc, "term", "DDP")
		if len(inc) < 11 || fob == nil || fob["anyMode"] != false || fob["sellerPaysCarriage"] != false || cif["sellerInsures"] != true ||
			ddp["sellerClearsImport"] != true || fob["edition"] != 2020.0 || !genCodeOf("MIC").MatchString(fob["code"].(string)) {
			t.Fatalf("incoterms %d fob %v cif %v", len(inc), fob, cif)
		}
		ctn := must(t, call(t, "GET", ctnBase, nil), 200).List
		hc := byKey(ctn, "isoCode", "45G1")
		if len(ctn) < 10 || hc == nil || hc["height"] != "high_cube" || hc["teu"] != 2.0 || hc["maxGrossKg"] != 30480.0 || hc["payloadKg"] != nil {
			t.Fatalf("containers %d hc %v", len(ctn), hc)
		}
		if ct := byKey(must(t, call(t, "GET", pkgBase, nil), 200).List, "uneceCode", "CT"); ct == nil || ct["name"] != "Karton" {
			t.Errorf("packaging CT %v", ct)
		}
		uoms := must(t, call(t, "GET", uomBase, nil), 200).List
		if tne := byKey(uoms, "uneceCode", "TNE"); tne == nil || tne["factor"] != 1000.0 || tne["dimension"] != "mass" {
			t.Errorf("uom TNE %v", tne)
		}
	})

	t.Run("LP02 a port and an airport: codes upper-cased, city in the country, coordinates", func(t *testing.T) {
		c := testCountry(t)
		city := must(t, call(t, "POST", cityBase, map[string]any{"name": "Kota Pelabuhan", "countryCode": c["code"]}), 201).str("code")
		code := nextLocode(c["iso2"].(string))
		p := must(t, call(t, "POST", locBase, map[string]any{"name": "Pelabuhan Uji", "unlocode": strings.ToLower(code), "countryCode": c["code"], "cityCode": city,
			"kind": "seaport", "customsOffice": "040300", "latitude": -6.104, "longitude": 106.88}), 201)
		if p.str("unlocode") != code || p.str("cityCode") != city || p.Obj["latitude"] != -6.104 || !genCodeOf("MLK").MatchString(p.str("code")) {
			t.Errorf("port %s", trunc(p.Raw))
		}
		a := must(t, call(t, "POST", locBase, map[string]any{"name": "Bandara Uji", "countryCode": c["code"], "kind": "airport", "iataCode": "x" + letters(locSeq.Add(1), 2)}), 201)
		if len(a.str("iataCode")) != 3 || a.str("iataCode") != strings.ToUpper(a.str("iataCode")) {
			t.Errorf("airport %s", trunc(a.Raw))
		}
		u := must(t, call(t, "PUT", locBase+"/"+p.str("code"), map[string]any{"latitude": nil, "longitude": nil, "version": p.Obj["version"]}), 200)
		if u.Obj["latitude"] != nil || u.str("unlocode") != code {
			t.Errorf("update %s", trunc(u.Raw))
		}
		if d := call(t, "DELETE", cityBase+"/"+city, nil); !want(t, d, 409) || !strings.Contains(d.str("message"), "lokasi") {
			t.Errorf("city in use %s", trunc(d.Raw))
		} // Singkatan: upper-cased, one live location per nickname.
		nick := "J" + letters(locSeq.Add(1), 2)
		n := must(t, call(t, "PUT", locBase+"/"+p.str("code"), map[string]any{"nickname": strings.ToLower(nick)}), 200)
		if n.str("nickname") != nick {
			t.Errorf("nickname %s", trunc(n.Raw))
		}
		if x := call(t, "PUT", locBase+"/"+a.str("code"), map[string]any{"nickname": nick}); !want(t, x, 422) || x.problem("nickname") == "" {
			t.Errorf("taken nickname %s", trunc(x.Raw))
		}
		if x := call(t, "PUT", locBase+"/"+a.str("code"), map[string]any{"nickname": "J*K"}); !want(t, x, 422) || x.problem("nickname") == "" {
			t.Errorf("bad nickname %s", trunc(x.Raw))
		}
	})

	t.Run("LP03 a container type: TEU from the length, payload from gross and tare", func(t *testing.T) {
		x := must(t, call(t, "POST", ctnBase, map[string]any{"name": "45' Pallet Wide", "lengthFt": 45, "height": "high_cube", "group": "general",
			"maxGrossKg": 32500, "tareKg": 4800, "innerVolumeCbm": 86.5}), 201)
		if x.Obj["teu"] != 2.25 || x.Obj["payloadKg"] != 27700.0 || x.Obj["innerVolumeCbm"] != 86.5 || !genCodeOf("MKN").MatchString(x.str("code")) {
			t.Errorf("container %s", trunc(x.Raw))
		}
		u := must(t, call(t, "PUT", ctnBase+"/"+x.str("code"), map[string]any{"teu": 2}), 200)
		if u.Obj["teu"] != 2.0 || u.Obj["payloadKg"] != 27700.0 {
			t.Errorf("update %s", trunc(u.Raw))
		}
	})

	t.Run("LP04 a commodity: HS without dots, dangerous goods data, default unit and packaging in use", func(t *testing.T) {
		uom := must(t, call(t, "POST", uomBase, map[string]any{"name": "Satuan Uji", "dimension": "count"}), 201).str("code")
		pkg := must(t, call(t, "POST", pkgBase, map[string]any{"name": "Kemasan Uji", "maxWeightKg": 25}), 201).str("code")
		x := must(t, call(t, "POST", cmdBase, map[string]any{"name": "Pupuk Amonium Nitrat", "hsCode": "3102.30.00", "dangerous": true, "unNumber": "UN 1942", "dgClass": "5.1",
			"uomCode": uom, "packagingCode": pkg}), 201)
		if x.str("hsCode") != "31023000" || x.str("unNumber") != "1942" || x.str("dgClass") != "5.1" || x.str("uomCode") != uom || !genCodeOf("MKD").MatchString(x.str("code")) {
			t.Errorf("commodity %s", trunc(x.Raw))
		}
		u := must(t, call(t, "PUT", cmdBase+"/"+x.str("code"), map[string]any{"dangerous": false}), 200)
		if u.Obj["unNumber"] != nil || u.Obj["dgClass"] != nil || u.str("hsCode") != "31023000" {
			t.Errorf("not dangerous any more %s", trunc(u.Raw))
		}
		for _, path := range []string{uomBase + "/" + uom, pkgBase + "/" + pkg} {
			if d := call(t, "DELETE", path, nil); !want(t, d, 409) || !strings.Contains(d.str("message"), "komoditas") {
				t.Errorf("DELETE %s %s", path, trunc(d.Raw))
			}
		}
		must(t, call(t, "DELETE", cmdBase+"/"+x.str("code"), nil), 204)
		must(t, call(t, "DELETE", uomBase+"/"+uom, nil), 204)
	})
}

func TestLogisticsNegative(t *testing.T) {
	c := testCountry(t)
	other := testCountry(t)
	iso2 := c["iso2"].(string)

	t.Run("LN01 locations: UN/LOCODE of another country, IATA on a seaport, city elsewhere, half coordinates, duplicates", func(t *testing.T) {
		farCity := must(t, call(t, "POST", cityBase, map[string]any{"name": "Kota Jauh", "countryCode": other["code"]}), 201).str("code")
		x := call(t, "POST", locBase, map[string]any{"name": "Salah", "unlocode": other["iso2"].(string) + "AAA", "countryCode": c["code"], "cityCode": farCity, "kind": "seaport", "latitude": 1})
		if !want(t, x, 422) || x.problem("unlocode") == "" || x.problem("cityCode") == "" || x.problem("longitude") == "" {
			t.Errorf("mismatch %s", trunc(x.Raw))
		}
		x = call(t, "POST", locBase, map[string]any{"name": "Salah", "unlocode": "1", "countryCode": c["code"], "kind": "seaport", "iataCode": "ABC", "customsOffice": "12"})
		if !want(t, x, 422) || x.problem("unlocode") == "" || !strings.Contains(x.problem("iataCode"), "bandara") || x.problem("customsOffice") == "" {
			t.Errorf("formats %s", trunc(x.Raw))
		}
		if x = call(t, "POST", locBase, map[string]any{"name": "Tanpa jenis", "countryCode": c["code"]}); !want(t, x, 422) || x.problem("kind") == "" {
			t.Errorf("kind %s", trunc(x.Raw))
		}
		code := nextLocode(iso2)
		must(t, call(t, "POST", locBase, map[string]any{"name": "Satu", "unlocode": code, "countryCode": c["code"], "kind": "seaport"}), 201)
		if x = call(t, "POST", locBase, map[string]any{"name": "Dua", "unlocode": code, "countryCode": c["code"], "kind": "river_port"}); !want(t, x, 422) || !strings.Contains(x.problem("unlocode"), "Satu") {
			t.Errorf("duplicate %s", trunc(x.Raw))
		}
	})

	t.Run("LN02 incoterms: one per term and edition, known editions, insurance needs carriage", func(t *testing.T) {
		if x := call(t, "POST", incBase, map[string]any{"name": "FOB lagi", "term": "fob"}); !want(t, x, 422) || x.problem("term") == "" {
			t.Errorf("duplicate %s", trunc(x.Raw))
		}
		must(t, call(t, "POST", incBase, map[string]any{"name": "Free On Board 2010", "term": "FOB", "edition": 2010, "anyMode": false}), 201)
		x := call(t, "POST", incBase, map[string]any{"name": "Aneh", "term": "ABCD", "edition": 2015, "sellerInsures": true})
		if !want(t, x, 422) || x.problem("term") == "" || x.problem("edition") == "" || x.problem("sellerInsures") == "" {
			t.Errorf("rules %s", trunc(x.Raw))
		}
	})

	t.Run("LN03 container types: known lengths, tare below gross, ISO code format and once", func(t *testing.T) {
		x := call(t, "POST", ctnBase, map[string]any{"name": "Aneh", "lengthFt": 30, "group": "general", "maxGrossKg": 1000, "tareKg": 1000, "isoCode": "G1"})
		if !want(t, x, 422) || x.problem("lengthFt") == "" || x.problem("tareKg") == "" || x.problem("isoCode") == "" {
			t.Errorf("rules %s", trunc(x.Raw))
		}
		if x = call(t, "POST", ctnBase, map[string]any{"name": "Kembar", "lengthFt": 20, "group": "general", "isoCode": "22g1"}); !want(t, x, 422) || x.problem("isoCode") == "" {
			t.Errorf("duplicate ISO %s", trunc(x.Raw))
		}
		if x = call(t, "POST", ctnBase, map[string]any{"name": "Tanpa kelompok", "lengthFt": 20}); !want(t, x, 422) || x.problem("group") == "" {
			t.Errorf("group %s", trunc(x.Raw))
		}
	})

	t.Run("LN04 units and packagings: UN/ECE codes once, no factor for 'other'", func(t *testing.T) {
		if x := call(t, "POST", uomBase, map[string]any{"name": "Kilo lagi", "uneceCode": "kgm", "dimension": "mass"}); !want(t, x, 422) || x.problem("uneceCode") == "" {
			t.Errorf("duplicate KGM %s", trunc(x.Raw))
		}
		if x := call(t, "POST", uomBase, map[string]any{"name": "Lain", "dimension": "other", "factor": 2, "uneceCode": "!"}); !want(t, x, 422) || x.problem("factor") == "" || x.problem("uneceCode") == "" {
			t.Errorf("other %s", trunc(x.Raw))
		}
		if x := call(t, "POST", pkgBase, map[string]any{"name": "Karton lagi", "uneceCode": "ct"}); !want(t, x, 422) || x.problem("uneceCode") == "" {
			t.Errorf("duplicate CT %s", trunc(x.Raw))
		}
	})

	t.Run("LN05 commodities: dangerous goods need UN number and class; HS length; inactive unit", func(t *testing.T) {
		if x := call(t, "POST", cmdBase, map[string]any{"name": "Bahaya", "dangerous": true, "hsCode": "12345"}); !want(t, x, 422) || x.problem("unNumber") == "" || x.problem("dgClass") == "" || x.problem("hsCode") == "" {
			t.Errorf("dg %s", trunc(x.Raw))
		}
		if x := call(t, "POST", cmdBase, map[string]any{"name": "Bahaya", "dangerous": true, "unNumber": "12", "dgClass": "10"}); !want(t, x, 422) || x.problem("unNumber") == "" || x.problem("dgClass") == "" {
			t.Errorf("formats %s", trunc(x.Raw))
		}
		off := must(t, call(t, "POST", uomBase, map[string]any{"name": "Satuan Mati", "dimension": "count", "active": false}), 201).str("code")
		if x := call(t, "POST", cmdBase, map[string]any{"name": "Biasa", "uomCode": off}); !want(t, x, 422) || !strings.Contains(x.problem("uomCode"), "nonaktif") {
			t.Errorf("inactive unit %s", trunc(x.Raw))
		}
	})
}

func TestLogisticsStress(t *testing.T) {
	c := testCountry(t)
	iso2 := c["iso2"].(string)

	t.Run("LS01 30 ports saved at once get 30 codes; one UN/LOCODE saved 10 times at once is stored once", func(t *testing.T) {
		var fails, created atomic.Int64
		codes := sync.Map{}
		parallel(30, 10, func(i int) {
			x := call(t, "POST", locBase, map[string]any{"name": fmt.Sprintf("Pelabuhan Serbu %d", i), "unlocode": nextLocode(iso2), "countryCode": c["code"], "kind": "seaport"})
			if x.Status != 201 {
				fails.Add(1)
				return
			}
			if _, dup := codes.LoadOrStore(x.str("code"), true); dup {
				fails.Add(1)
			}
		})
		code := nextLocode(iso2)
		parallel(10, 10, func(int) {
			switch x := call(t, "POST", locBase, map[string]any{"name": "Rebutan", "unlocode": code, "countryCode": c["code"], "kind": "seaport"}); x.Status {
			case 201:
				created.Add(1)
			case 409, 422:
			default:
				fails.Add(1)
			}
		})
		if fails.Load() != 0 || created.Load() != 1 {
			t.Errorf("failures %d, the same UN/LOCODE created %d times", fails.Load(), created.Load())
		}
	})

	t.Run("LS02 one Incoterm saved 10 times at once is stored once; 20 parallel updates of one container lose none", func(t *testing.T) {
		var fails, created atomic.Int64
		parallel(10, 10, func(int) {
			switch x := call(t, "POST", incBase, map[string]any{"name": "Rebutan", "term": "EXW", "edition": 2010}); x.Status {
			case 201:
				created.Add(1)
			case 409, 422:
			default:
				fails.Add(1)
			}
		})
		x := must(t, call(t, "POST", ctnBase, map[string]any{"name": "Kontainer Serbu", "lengthFt": 20, "group": "general"}), 201)
		parallel(20, 10, func(i int) {
			if r := call(t, "PUT", ctnBase+"/"+x.str("code"), map[string]any{"description": fmt.Sprint("ubah ", i)}); r.Status != 200 {
				fails.Add(1)
			}
		})
		if g := must(t, call(t, "GET", ctnBase+"/"+x.str("code"), nil), 200); g.Obj["version"] != 21.0 {
			t.Errorf("version %v", g.Obj["version"])
		}
		if fails.Load() != 0 || created.Load() != 1 {
			t.Errorf("failures %d, EXW 2010 created %d times", fails.Load(), created.Load())
		}
	})
}
