//go:build e2e

// Jenis Unit Operasional (/pm/masters/unit-types), built on the Level Organisasi pilot.
package e2e

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const utBase = "/pm/masters/unit-types"

var utCode = regexp.MustCompile(`^MJU\d{3,}$`)

func utCreate(t testing.TB, extra map[string]any) resp {
	t.Helper()
	body := map[string]any{"name": "Jenis uji"}
	for k, v := range extra {
		body[k] = v
	}
	return call(t, "POST", utBase, body)
}

func TestUnitTypesPositive(t *testing.T) {
	t.Run("UP01 list: seeded types by code, standard fields, head job title empty", func(t *testing.T) {
		r := call(t, "GET", utBase, nil)
		if !want(t, r, 200) {
			return
		}
		var codes []string
		for _, u := range r.List {
			codes = append(codes, u["code"].(string))
			if !uuidV7.MatchString(u["id"].(string)) || !utcTime.MatchString(u["createdAt"].(string)) {
				t.Errorf("id/createdAt: %v %v", u["id"], u["createdAt"])
			}
			if _, ok := u["headJabatanId"]; !ok {
				t.Errorf("headJabatanId missing: %v", u)
			}
			if _, ok := u["headJabatanCode"]; !ok {
				t.Errorf("headJabatanCode missing: %v", u)
			}
			for _, k := range []string{"name", "description", "active", "state", "status", "validFrom", "validTo", "createdBy", "version"} {
				if _, ok := u[k]; !ok {
					t.Errorf("field %s missing", k)
				}
			}
		}
		// The seed is there and the list is sorted by code; other suites may have added types.
		have := map[string]bool{}
		for _, c := range codes {
			have[c] = true
		}
		for _, c := range []string{"BLK", "EST", "KTR", "PKS", "PLS"} {
			if !have[c] {
				t.Errorf("seeded %s missing: %v", c, codes)
			}
		}
		if !sort.StringsAreSorted(codes) {
			t.Errorf("not sorted by code: %v", codes)
		}
	})

	t.Run("UP02 create: generated code, standard fields, head job title not taken", func(t *testing.T) {
		r := utCreate(t, map[string]any{"code": "KBN", "name": "  Kebun Bibit  ", "description": "Baris 1\nBaris 2", "validFrom": "2026-01-01", "validTo": "2026-12-31",
			"active": false, "headJabatan": "EM", "headJabatanId": "00000000-0000-7000-8000-000000000009", "status": "POSTED"})
		if !want(t, r, 201) {
			return
		}
		if !utCode.MatchString(r.str("code")) || r.str("name") != "Kebun Bibit" || r.str("description") != "Baris 1\nBaris 2" || r.Obj["active"] != false ||
			r.Obj["headJabatanId"] != nil || r.Obj["status"] != nil || r.str("createdBy") != userID || r.Obj["version"] != 1.0 {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("UP03 codes are consecutive and their own series (not the levels')", func(t *testing.T) {
		a, b := utCreate(t, nil).str("code"), utCreate(t, nil).str("code")
		var x, y int
		fmt.Sscanf(a, "MJU%d", &x)
		fmt.Sscanf(b, "MJU%d", &y)
		if y != x+1 {
			t.Errorf("%s then %s", a, b)
		}
	})

	t.Run("UP04 get in any case; partial update with version; seeded types may change", func(t *testing.T) {
		c := utCreate(t, map[string]any{"description": "awal"})
		code := c.str("code")
		want(t, call(t, "GET", utBase+"/"+strings.ToLower(code), nil), 200)
		r := call(t, "PUT", utBase+"/"+strings.ToLower(code), map[string]any{"name": "Baru", "version": 1})
		if want(t, r, 200) && (r.str("code") != code || r.str("description") != "awal" || r.Obj["version"] != 2.0 || r.str("updatedBy") != userID || r.str("id") != c.str("id")) {
			t.Errorf("got %s", r.Raw)
		}
		if r := call(t, "PUT", utBase+"/est", map[string]any{"name": "Kebun Inti Baru"}); want(t, r, 200) && r.str("code") != "EST" {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("UP05 the bodies the ALIR form sends, a second admin's stale save, the list toggle", func(t *testing.T) {
		c := call(t, "POST", utBase, map[string]any{"code": nil, "name": "Gudang", "description": "", "validFrom": "", "validTo": "", "active": true, "headJabatan": nil})
		if !want(t, c, 201) {
			return
		}
		code := c.str("code")
		form := map[string]any{"code": code, "name": "Gudang Besar", "description": "", "validFrom": "", "validTo": "", "active": true, "headJabatan": nil, "version": 1}
		want(t, call(t, "PUT", utBase+"/"+code, form), 200)
		form["name"] = "Versi admin B"
		if r := call(t, "PUT", utBase+"/"+code, form); want(t, r, 409) && r.str("code") != "STALE" {
			t.Errorf("got %s", r.Raw)
		}
		row := call(t, "GET", utBase+"/"+code, nil)
		toggle := map[string]any{"name": row.str("name"), "description": row.Obj["description"], "validFrom": row.Obj["validFrom"], "validTo": row.Obj["validTo"], "active": false, "version": row.Obj["version"]}
		if r := call(t, "PUT", utBase+"/"+code, toggle); want(t, r, 200) && (r.Obj["active"] != false || r.str("name") != "Gudang Besar") {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("UP06 soft delete: row kept with audit, three events, code never reused", func(t *testing.T) {
		c := utCreate(t, nil)
		code, id := c.str("code"), c.str("id")
		call(t, "PUT", utBase+"/"+code, map[string]any{"name": "x"})
		want(t, call(t, "DELETE", utBase+"/"+code, nil), 204)
		want(t, call(t, "GET", utBase+"/"+code, nil), 404)
		var deleted, active bool
		var by string
		if err := db.QueryRow(context.Background(), `SELECT is_deleted, is_active, deleted_by::text FROM pm.unit_type WHERE id = $1`, id).Scan(&deleted, &active, &by); err != nil || !deleted || active || by != userID {
			t.Errorf("row: %v %v %v %s", err, deleted, active, by)
		}
		var n int
		db.QueryRow(context.Background(), `SELECT count(*) FROM pm.outbox WHERE aggregate_id = $1 AND aggregate_type = 'unit_type' AND event_type LIKE 'pm.unit_type.%'`, id).Scan(&n)
		if n != 3 {
			t.Errorf("events %d", n)
		}
		if n := utCreate(t, nil).str("code"); n == code {
			t.Errorf("code %s reused", code)
		}
	})

	t.Run("UP07 levels and unit types keep separate counters", func(t *testing.T) {
		var lv, ut int64
		db.QueryRow(context.Background(), `SELECT last_number FROM pm.code_prefix WHERE code = 'org_level'`).Scan(&lv)
		utCreate(t, nil)
		var lv2 int64
		db.QueryRow(context.Background(), `SELECT last_number FROM pm.code_prefix WHERE code = 'org_level'`).Scan(&lv2)
		db.QueryRow(context.Background(), `SELECT last_number FROM pm.code_prefix WHERE code = 'unit_type'`).Scan(&ut)
		if lv != lv2 || ut == 0 {
			t.Errorf("org_level %d → %d, unit_type %d", lv, lv2, ut)
		}
	})
}

func TestUnitTypesNegative(t *testing.T) {
	cases := []struct {
		id, name string
		body     map[string]any
		field    string
		msg      string
	}{
		{"UN01", "name missing", map[string]any{}, "name", "Nama wajib diisi."},
		{"UN02", "name blank", map[string]any{"name": "  "}, "name", "Nama wajib diisi."},
		{"UN03", "name 151", map[string]any{"name": strings.Repeat("n", 151)}, "name", "Nama paling banyak 150 karakter."},
		{"UN04", "name NUL", map[string]any{"name": "a\x00"}, "name", "Nama berisi karakter yang tidak diizinkan."},
		{"UN05", "description 501", map[string]any{"name": "x", "description": strings.Repeat("d", 501)}, "description", "Deskripsi paling banyak 500 karakter."},
		{"UN06", "active text", map[string]any{"name": "x", "active": "ya"}, "active", "Status aktif harus ya atau tidak."},
		{"UN07", "bad date", map[string]any{"name": "x", "validTo": "2026-02-30"}, "validTo", "Berlaku sampai bukan tanggal yang sah (format YYYY-MM-DD)."},
		{"UN08", "end before start", map[string]any{"name": "x", "validFrom": "2026-06-02", "validTo": "2026-06-01"}, "validTo", "Tanggal selesai tidak boleh sebelum tanggal mulai."},
	}
	for _, c := range cases {
		t.Run(c.id+" "+c.name, func(t *testing.T) {
			r := call(t, "POST", utBase, c.body)
			if want(t, r, 422) && r.problem(c.field) != c.msg {
				t.Errorf("%s: got %q, want %q", c.field, r.problem(c.field), c.msg)
			}
		})
	}

	t.Run("UN09 a type no unit uses can be deleted, in any case (a type in use: OUX02)", func(t *testing.T) {
		code := utCreate(t, nil).str("code")
		want(t, call(t, "DELETE", utBase+"/"+strings.ToLower(code), nil), 204)
		want(t, call(t, "GET", utBase+"/"+code, nil), 404)
	})

	t.Run("UN10 unknown and odd codes are 404; malformed bodies 400; wrong method 405", func(t *testing.T) {
		for _, c := range []string{"MJU999999", "%00", "%FF", strings.Repeat("A", 300), "%27%20OR%201%3D1--"} {
			for _, m := range []string{"GET", "PUT", "DELETE"} {
				var body any
				if m == "PUT" {
					body = map[string]any{"name": "x"}
				}
				if r := call(t, m, utBase+"/"+c, body); r.Status != 404 || r.str("code") != "NOT_FOUND" {
					t.Errorf("%s %s: %d %s", m, c, r.Status, trunc(r.Raw))
				}
			}
		}
		for _, b := range []string{"", "{", "[]", `{"name":"x"} {}`} {
			if r := call(t, "POST", utBase, b); r.Status != 400 {
				t.Errorf("body %q: %d", b, r.Status)
			}
		}
		if r := call(t, "PATCH", utBase+"/EST", nil); r.Status != 405 || r.str("code") != "METHOD_NOT_ALLOWED" {
			t.Errorf("PATCH: %d %s", r.Status, r.Raw)
		}
	})

	t.Run("UN11 bad version is 422; stale is 409 and writes nothing", func(t *testing.T) {
		code := utCreate(t, nil).str("code")
		if r := call(t, "PUT", utBase+"/"+code, map[string]any{"name": "x", "version": "abc"}); want(t, r, 422) && r.problem("version") == "" {
			t.Errorf("got %s", r.Raw)
		}
		call(t, "PUT", utBase+"/"+code, map[string]any{"name": "Pertama", "version": 1})
		want(t, call(t, "PUT", utBase+"/"+code, map[string]any{"name": "Basi", "version": 1}), 409)
		if g := call(t, "GET", utBase+"/"+code, nil); g.str("name") != "Pertama" {
			t.Errorf("stale write applied: %s", g.Raw)
		}
	})

	t.Run("UN12 no active prefix: 409 NO_PREFIX, nothing stored, levels unaffected", func(t *testing.T) {
		ctx := context.Background()
		db.Exec(ctx, `UPDATE pm.code_prefix SET is_active = false WHERE code = 'unit_type'`)
		defer db.Exec(ctx, `UPDATE pm.code_prefix SET is_active = true WHERE code = 'unit_type'`)
		if r := utCreate(t, nil); r.Status != 409 || r.str("code") != "NO_PREFIX" {
			t.Errorf("got %d %s", r.Status, r.Raw)
		}
		want(t, call(t, "POST", base, map[string]any{"name": "Level tetap bisa", "rank": nextRank()}), 201)
	})

	t.Run("UN13 upload is not available yet (501)", func(t *testing.T) {
		if r := call(t, "POST", utBase+"/upload", "x"); r.Status != 501 || r.str("code") != "NO_IMPORT" {
			t.Errorf("got %d %s", r.Status, r.Raw)
		}
	})
}

func TestUnitTypesStress(t *testing.T) {
	t.Run("US01 200 parallel creates with levels at the same time: both series unique and gap-free", func(t *testing.T) {
		var mu sync.Mutex
		var ut, lv []int
		bad := 0
		parallel(200, 50, func(i int) {
			var r resp
			if i%2 == 0 {
				r = utCreate(t, nil)
			} else {
				r = call(t, "POST", base, map[string]any{"name": "Paralel", "rank": nextRank()})
			}
			mu.Lock()
			defer mu.Unlock()
			if r.Status != 201 {
				bad++
				return
			}
			var x int
			if fmt.Sscanf(r.str("code"), "MJU%d", &x); strings.HasPrefix(r.str("code"), "MJU") {
				ut = append(ut, x)
			} else if _, err := fmt.Sscanf(r.str("code"), "ML%d", &x); err == nil {
				lv = append(lv, x)
			}
		})
		if bad > 0 || len(ut) != 100 || len(lv) != 100 {
			t.Errorf("failed %d, unit types %d, levels %d", bad, len(ut), len(lv))
		}
		for name, s := range map[string][]int{"MJU": ut, "ML": lv} {
			sort.Ints(s)
			for i := 1; i < len(s); i++ {
				if s[i] != s[i-1]+1 {
					t.Errorf("%s gap or duplicate at %d → %d", name, s[i-1], s[i])
					break
				}
			}
		}
	})

	t.Run("US02 100 parallel updates of one type, deletes racing updates: nothing lost, no 500", func(t *testing.T) {
		code := utCreate(t, nil).str("code")
		parallel(100, 25, func(i int) { call(t, "PUT", utBase+"/"+code, map[string]any{"description": fmt.Sprint(i)}) })
		if g := call(t, "GET", utBase+"/"+code, nil); g.Obj["version"] != 101.0 {
			t.Errorf("version %v, want 101", g.Obj["version"])
		}
		for round := range 5 {
			c := utCreate(t, nil).str("code")
			var mu sync.Mutex
			st := map[int]int{}
			parallel(20, 20, func(i int) {
				var r resp
				if i%4 == 0 {
					r = call(t, "DELETE", utBase+"/"+c, nil)
				} else {
					r = call(t, "PUT", utBase+"/"+c, map[string]any{"name": "r"})
				}
				mu.Lock()
				st[r.Status]++
				mu.Unlock()
			})
			if st[204] != 1 || st[500] > 0 {
				t.Errorf("round %d %v", round, st)
			}
		}
	})

	t.Run("US03 2000 reads of both masters while writing", func(t *testing.T) {
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
					utCreate(t, nil)
				}
			}
		}()
		var mu sync.Mutex
		var lat []time.Duration
		errs := 0
		paths := []string{utBase, utBase + "/EST", base}
		parallel(2000, 100, func(i int) {
			start := time.Now()
			r := call(t, "GET", paths[i%len(paths)], nil)
			mu.Lock()
			lat = append(lat, time.Since(start))
			if r.Status != 200 {
				errs++
			}
			mu.Unlock()
		})
		close(stop)
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		p95 := lat[len(lat)*95/100]
		t.Logf("reads: p50 %v, p95 %v, max %v, errors %d", lat[len(lat)/2], p95, lat[len(lat)-1], errs)
		if errs > 0 || p95 > 500*time.Millisecond {
			t.Errorf("errors %d, p95 %v", errs, p95)
		}
		var dup int
		db.QueryRow(context.Background(), `SELECT count(*) FROM (SELECT lower(code) FROM pm.unit_type GROUP BY 1 HAVING count(*) > 1) d`).Scan(&dup)
		if dup > 0 {
			t.Errorf("%d duplicate codes", dup)
		}
	})
}
