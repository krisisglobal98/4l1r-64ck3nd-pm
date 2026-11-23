//go:build e2e

// Profil Perusahaan (/pm/masters/company-profiles) and the profile documents print (/pm/company-profile/print).
// Positive, negative, stress.
package e2e

import (
	"strings"
	"sync/atomic"
	"testing"
)

const cpBase = "/pm/masters/company-profiles"

// A 1×1 PNG.
const tinyPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

func TestCompanyProfile(t *testing.T) {
	t.Run("CP01 a profile with every field; the document print reads the one marked for documents", func(t *testing.T) {
		if x := call(t, "GET", "/pm/company-profile/print", nil); x.Status != 404 {
			t.Fatalf("no profile yet: %d", x.Status)
		}
		p := must(t, call(t, "POST", cpBase, map[string]any{"name": "PT Gex Logistik Indonesia", "tradeName": "GEX LOGISTICS", "npwp": "01.234.567.8-901.000",
			"address": "Jl. Pangeran Jayakarta No. 8", "city": "Jakarta Barat", "countryCode": byKey(must(t, call(t, "GET", "/pm/masters/countries", nil), 200).List, "iso2", "ID")["code"],
			"phone": "+62 21 6591011", "fax": "+62 21 6591218", "email": "ops@contoh.test", "website": "https://contoh.test", "logo": tinyPNG, "printDefault": true}), 201)
		if !genCodeOf("MPF").MatchString(p.str("code")) || p.str("npwp") != "0012345678901000" || p.str("countryName") == "" || p.Obj["printDefault"] != true || p.str("logo") != tinyPNG {
			t.Errorf("created %s", trunc(p.Raw))
		}
		pr := must(t, call(t, "GET", "/pm/company-profile/print", nil), 200)
		if pr.str("code") != p.str("code") || pr.str("tradeName") != "GEX LOGISTICS" {
			t.Errorf("print %s", trunc(pr.Raw))
		}
	})

	t.Run("CP02 bad fields, a second default, a second profile for the same company", func(t *testing.T) {
		cases := map[string]map[string]any{
			"npwp":  {"name": "X", "npwp": "123"},
			"logo":  {"name": "X", "logo": "data:text/html;base64,PGI+"},
			"email": {"name": "X", "email": "bukan-email"},
		}
		for field, body := range cases {
			if x := call(t, "POST", cpBase, body); !want(t, x, 422) || x.problem(field) == "" {
				t.Errorf("%s: %s", field, trunc(x.Raw))
			}
		}
		if x := call(t, "POST", cpBase, map[string]any{"name": "PT Lain", "logo": "data:image/png;base64," + strings.Repeat("A", 600000)}); !want(t, x, 422) || x.problem("logo") == "" {
			t.Errorf("logo too large: %d", x.Status)
		}
		// The tenant-level profile (no company unit) exists from CP01: another one for no unit is refused.
		if x := call(t, "POST", cpBase, map[string]any{"name": "PT Kedua"}); !want(t, x, 422) || x.problem("unitCode") == "" {
			t.Errorf("second tenant profile: %s", trunc(x.Raw))
		}
	})

	t.Run("CP03 concurrent reads of the print profile", func(t *testing.T) {
		var bad atomic.Int32
		parallel(30, 10, func(int) {
			if x := call(t, "GET", "/pm/company-profile/print", nil); x.Status != 200 {
				bad.Add(1)
			}
		})
		if bad.Load() != 0 {
			t.Errorf("%d failed reads", bad.Load())
		}
	})
}
