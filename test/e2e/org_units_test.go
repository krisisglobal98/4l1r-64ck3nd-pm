//go:build e2e

// Unit Organisasi (/pm/masters/org-units): the tree, its relations to levels and unit types, and the sample seed.
package e2e

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const ouBase = "/pm/masters/org-units"

var ouCode = regexp.MustCompile(`^MUO\d{3,}$`)

func ouCreate(t testing.TB, body map[string]any) resp {
	t.Helper()
	return call(t, "POST", ouBase, body)
}

// branch is one path of the tree: group → region → wilayah → company → operational unit.
type branch struct{ g, r, w, c, u string }

func newBranch(t testing.TB) branch {
	t.Helper()
	code := func(r resp) string {
		t.Helper()
		if r.Status != 201 {
			t.Fatalf("building the tree: %d %s", r.Status, r.Raw)
		}
		return r.str("code")
	}
	var b branch
	b.g = code(ouCreate(t, map[string]any{"name": "Grup", "levelCode": "group"}))
	b.r = code(ouCreate(t, map[string]any{"name": "Region", "levelCode": "region", "parentCode": b.g}))
	b.w = code(ouCreate(t, map[string]any{"name": "Wilayah", "levelCode": "wilayah", "parentCode": b.r}))
	b.c = code(ouCreate(t, map[string]any{"name": "PT", "levelCode": "company", "parentCode": b.w}))
	b.u = code(ouCreate(t, map[string]any{"name": "Kebun", "levelCode": "unit", "parentCode": b.c, "unitTypeCode": "EST", "location": "Sekadau"}))
	return b
}

func TestOrgUnitsPositive(t *testing.T) {
	t.Run("OUP01 a whole branch: generated codes, relations by id and code, standard fields", func(t *testing.T) {
		b := newBranch(t)
		r := call(t, "GET", ouBase+"/"+b.u, nil)
		if !want(t, r, 200) {
			return
		}
		if !ouCode.MatchString(r.str("code")) || r.str("levelCode") != "unit" || !uuidV7.MatchString(r.str("levelId")) || r.str("parentCode") != b.c ||
			!uuidV7.MatchString(r.str("parentId")) || r.str("unitTypeCode") != "EST" || !uuidV7.MatchString(r.str("unitTypeId")) || r.str("location") != "Sekadau" ||
			r.Obj["active"] != true || r.Obj["version"] != 1.0 || r.str("createdBy") != userID || !utcTime.MatchString(r.str("createdAt")) {
			t.Errorf("got %s", r.Raw)
		}
		top := call(t, "GET", ouBase+"/"+strings.ToLower(b.g), nil)
		if want(t, top, 200) && (top.Obj["parentId"] != nil || top.Obj["parentCode"] != nil || top.Obj["unitTypeCode"] != nil) {
			t.Errorf("top: %s", top.Raw)
		}
		list := call(t, "GET", ouBase, nil)
		if want(t, list, 200) && len(list.List) < 5 {
			t.Errorf("list %d", len(list.List))
		}
	})

	t.Run("OUP02 the bodies the ALIR form sends (adapter: level, parent and type by code; empty strings)", func(t *testing.T) {
		b := newBranch(t)
		r := ouCreate(t, map[string]any{"name": "Pabrik", "description": "", "validFrom": "", "validTo": "", "active": true, "location": "",
			"levelCode": "unit", "parentCode": b.c, "unitTypeCode": "PKS"})
		if !want(t, r, 201) {
			return
		}
		code := r.str("code")
		edit := map[string]any{"name": "Pabrik Satu", "description": "Kapasitas 60 t/jam", "validFrom": "2026-01-01", "validTo": "", "active": true, "location": "Sanggau",
			"levelCode": "unit", "parentCode": b.c, "unitTypeCode": "PKS", "version": 1}
		if r := call(t, "PUT", ouBase+"/"+code, edit); want(t, r, 200) && (r.str("location") != "Sanggau" || r.Obj["version"] != 2.0 || r.str("code") != code) {
			t.Errorf("got %s", r.Raw)
		}
		edit["name"] = "Versi admin B"
		if r := call(t, "PUT", ouBase+"/"+code, edit); want(t, r, 409) && r.str("code") != "STALE" {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("OUP03 change the unit type; move a unit to another company; rename the top", func(t *testing.T) {
		b := newBranch(t)
		c2 := ouCreate(t, map[string]any{"name": "PT Dua", "levelCode": "company", "parentCode": b.w}).str("code")
		r := call(t, "PUT", ouBase+"/"+b.u, map[string]any{"unitTypeCode": "PLS", "parentCode": c2})
		if want(t, r, 200) && (r.str("unitTypeCode") != "PLS" || r.str("parentCode") != c2) {
			t.Errorf("got %s", r.Raw)
		}
		want(t, call(t, "PUT", ouBase+"/"+b.g, map[string]any{"name": "Grup Baru"}), 200)
	})

	t.Run("OUP04 a level change of a leaf with its new parent and type", func(t *testing.T) {
		b := newBranch(t)
		// a company with nothing under it becomes an operational unit under another company
		c2 := ouCreate(t, map[string]any{"name": "PT Kosong", "levelCode": "company", "parentCode": b.w}).str("code")
		r := call(t, "PUT", ouBase+"/"+c2, map[string]any{"levelCode": "unit", "parentCode": b.c, "unitTypeCode": "KTR"})
		if want(t, r, 200) && (r.str("levelCode") != "unit" || r.str("unitTypeCode") != "KTR") {
			t.Errorf("got %s", r.Raw)
		}
		// and back: the type must go with it
		r = call(t, "PUT", ouBase+"/"+c2, map[string]any{"levelCode": "company", "parentCode": b.w, "unitTypeCode": nil})
		if want(t, r, 200) && r.Obj["unitTypeCode"] != nil {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("OUP05 soft delete of a leaf, then of its emptied parent; codes never reused", func(t *testing.T) {
		b := newBranch(t)
		want(t, call(t, "DELETE", ouBase+"/"+b.u, nil), 204)
		want(t, call(t, "DELETE", ouBase+"/"+b.c, nil), 204)
		want(t, call(t, "GET", ouBase+"/"+b.c, nil), 404)
		var deleted bool
		db.QueryRow(context.Background(), `SELECT is_deleted FROM pm.org_unit WHERE code = $1`, b.u).Scan(&deleted)
		if !deleted {
			t.Error("row not kept as deleted")
		}
		n := ouCreate(t, map[string]any{"name": "Baru", "levelCode": "company", "parentCode": b.w}).str("code")
		if n == b.c || n == b.u {
			t.Errorf("code %s reused", n)
		}
	})

	t.Run("OUP06 the sample seed loads the 27 units with their tree, and again changes nothing", func(t *testing.T) {
		sql, err := os.ReadFile("../../seeds/sample_org.sql")
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		var before int
		_ = before
		for range 2 {
			if _, err := db.Exec(ctx, string(sql)); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
		var after, orphans, typed int
		// the 27 sample units are there (another suite may have seeded them first)
		db.QueryRow(ctx, `SELECT count(*) + $1 FROM pm.org_unit WHERE created_by = '00000000-0000-7000-8000-000000000000' AND NOT is_deleted`, before).Scan(&after)
		db.QueryRow(ctx, `SELECT count(*) FROM pm.org_unit u JOIN pm.org_level l ON l.id = u.org_level_id WHERE l.code <> 'group' AND u.parent_id IS NULL AND u.code IN ('KAL','SUM','KALBAR','C11','C11-E1')`).Scan(&orphans)
		db.QueryRow(ctx, `SELECT count(*) FROM pm.org_unit u JOIN pm.unit_type t ON t.id = u.unit_type_id WHERE u.code = 'C11-E1' AND t.code = 'EST'`).Scan(&typed)
		var deep int // a block (org3) whose chain reaches an operational unit: org3 → org2 → org1 → unit
		db.QueryRow(ctx, `SELECT count(*) FROM pm.org_unit b JOIN pm.org_level lb ON lb.id = b.org_level_id AND lb.code = 'org3'
			JOIN pm.org_unit a ON a.id = b.parent_id JOIN pm.org_unit r ON r.id = a.parent_id JOIN pm.org_unit u ON u.id = r.parent_id
			JOIN pm.org_level lu ON lu.id = u.org_level_id AND lu.code = 'unit'`).Scan(&deep)
		if after-before < 27 || orphans != 0 || typed != 1 || deep == 0 {
			t.Errorf("seed: %d units, %d without parent, C11-E1 typed %d, blocks under a unit %d", after-before, orphans, typed, deep)
		}
		r := call(t, "GET", ouBase+"/c11-e1", nil)
		if want(t, r, 200) && (r.str("parentCode") != "C11" || r.str("levelCode") != "unit" || r.str("unitTypeCode") != "EST" || r.str("location") != "Sekadau") {
			t.Errorf("got %s", r.Raw)
		}
		// the seeded tree obeys the service's rules: an untouched save passes
		want(t, call(t, "PUT", ouBase+"/C11-E1", map[string]any{"name": "Kebun Sungai Ayak"}), 200)
	})
}

func TestOrgUnitsNegative(t *testing.T) {
	b := newBranch(t)
	cases := []struct {
		id, name string
		body     map[string]any
		field    string
		msg      string
	}{
		{"OUN01", "level missing", map[string]any{"name": "x"}, "levelCode", "Level wajib diisi."},
		{"OUN02", "unknown level", map[string]any{"name": "x", "levelCode": "nope"}, "levelCode", `Level "nope" tidak ditemukan.`},
		{"OUN03", "top with a parent", map[string]any{"name": "x", "levelCode": "group", "parentCode": b.g}, "parentCode", "Unit di level Group (teratas) tidak punya induk."},
		{"OUN04", "parent missing", map[string]any{"name": "x", "levelCode": "wilayah"}, "parentCode", "Induk wajib diisi: pilih unit di level Region."},
		{"OUN05", "parent two levels up", map[string]any{"name": "x", "levelCode": "wilayah", "parentCode": b.g}, "parentCode", "Induk harus unit di level Region, tepat satu level di atas Wilayah."},
		{"OUN06", "parent on the same level", map[string]any{"name": "x", "levelCode": "company", "parentCode": b.c}, "parentCode", "Induk harus unit di level Wilayah, tepat satu level di atas Perusahaan."},
		{"OUN07", "unknown parent", map[string]any{"name": "x", "levelCode": "region", "parentCode": "NOPE"}, "parentCode", `Induk "NOPE" tidak ditemukan.`},
		{"OUN08", "unit without type", map[string]any{"name": "x", "levelCode": "unit", "parentCode": b.c}, "unitTypeCode", "Jenis unit wajib diisi untuk level Unit Operasional."},
		{"OUN09", "unknown type", map[string]any{"name": "x", "levelCode": "unit", "parentCode": b.c, "unitTypeCode": "ZZZ"}, "unitTypeCode", `Jenis unit "ZZZ" tidak ditemukan.`},
		{"OUN10", "type on a company", map[string]any{"name": "x", "levelCode": "company", "parentCode": b.w, "unitTypeCode": "EST"}, "unitTypeCode", "Jenis unit hanya untuk level Unit Operasional, bukan Perusahaan."},
		{"OUN11", "name blank", map[string]any{"name": " ", "levelCode": "group"}, "name", "Nama wajib diisi."},
		{"OUN12", "location 81", map[string]any{"name": "x", "levelCode": "group", "location": strings.Repeat("l", 81)}, "location", "Lokasi paling banyak 80 karakter."},
		{"OUN13", "NUL in a relation", map[string]any{"name": "x", "levelCode": "uni\x00t"}, "levelCode", "Level berisi karakter yang tidak diizinkan."},
		{"OUN14", "end before start", map[string]any{"name": "x", "levelCode": "group", "validFrom": "2026-06-02", "validTo": "2026-06-01"}, "validTo", "Tanggal selesai tidak boleh sebelum tanggal mulai."},
	}
	for _, c := range cases {
		t.Run(c.id+" "+c.name, func(t *testing.T) {
			r := ouCreate(t, c.body)
			if want(t, r, 422) && r.problem(c.field) != c.msg {
				t.Errorf("%s: got %q, want %q (%s)", c.field, r.problem(c.field), c.msg, trunc(r.Raw))
			}
		})
	}

	t.Run("OUN15 inactive level, parent or type: refused for a new choice, kept when unchanged", func(t *testing.T) {
		ctx := context.Background()
		x := newBranch(t)
		call(t, "PUT", ouBase+"/"+x.c, map[string]any{"active": false})
		r := ouCreate(t, map[string]any{"name": "x", "levelCode": "unit", "parentCode": x.c, "unitTypeCode": "EST"})
		if want(t, r, 422) && r.problem("parentCode") != "Induk PT nonaktif; pilih unit lain." {
			t.Errorf("got %s", r.Raw)
		}
		want(t, call(t, "PUT", ouBase+"/"+x.u, map[string]any{"name": "Tetap di induknya"}), 200)
		db.Exec(ctx, `UPDATE pm.unit_type SET is_active = false WHERE code = 'PLS'`)
		defer db.Exec(ctx, `UPDATE pm.unit_type SET is_active = true WHERE code = 'PLS'`)
		if r := call(t, "PUT", ouBase+"/"+x.u, map[string]any{"unitTypeCode": "PLS"}); want(t, r, 422) && !strings.Contains(r.problem("unitTypeCode"), "nonaktif") {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("OUN16 tree changes that would break it", func(t *testing.T) {
		x := newBranch(t)
		if r := call(t, "PUT", ouBase+"/"+x.c, map[string]any{"levelCode": "unit", "parentCode": b.c, "unitTypeCode": "EST"}); want(t, r, 422) &&
			r.problem("levelCode") != "Level tidak dapat diubah selama unit ini punya 1 unit di bawahnya." {
			t.Errorf("level with children: %s", r.Raw)
		}
		if r := call(t, "PUT", ouBase+"/"+x.r, map[string]any{"parentCode": x.r}); want(t, r, 422) && r.problem("parentCode") == "" {
			t.Errorf("own parent: %s", r.Raw)
		}
		if r := call(t, "DELETE", ouBase+"/"+x.w, nil); want(t, r, 409) && (r.str("code") != "IN_USE" || !strings.Contains(r.str("message"), "1 unit di bawahnya")) {
			t.Errorf("delete a parent: %s", r.Raw)
		}
	})

	t.Run("OUN17 unknown and odd codes 404; malformed 400; wrong method 405; bad version 422", func(t *testing.T) {
		for _, c := range []string{"MUO999999", "%00", "%FF", strings.Repeat("A", 300)} {
			if r := call(t, "GET", ouBase+"/"+c, nil); r.Status != 404 {
				t.Errorf("GET %s: %d", c, r.Status)
			}
		}
		if r := ouCreate(t, nil); r.Status != 400 {
			t.Errorf("empty body: %d", r.Status)
		}
		if r := call(t, "PATCH", ouBase+"/"+b.g, nil); r.Status != 405 {
			t.Errorf("PATCH: %d", r.Status)
		}
		if r := call(t, "PUT", ouBase+"/"+b.g, map[string]any{"version": "x"}); want(t, r, 422) && r.problem("version") == "" {
			t.Errorf("version: %s", r.Raw)
		}
	})

	t.Run("OUN18 typed code, ids, audit and workflow are ignored", func(t *testing.T) {
		r := ouCreate(t, map[string]any{"code": "C99", "id": "x", "name": "Grup X", "levelCode": "group", "status": "POSTED", "createdBy": systemID, "levelId": "x", "parentId": "y"})
		if want(t, r, 201) && (!ouCode.MatchString(r.str("code")) || r.Obj["status"] != nil || r.str("createdBy") != userID || r.str("levelCode") != "group") {
			t.Errorf("got %s", r.Raw)
		}
	})
}

// Rules that cross masters: levels and unit types in use by units.
func TestOrgUnitsAcrossMasters(t *testing.T) {
	t.Run("OUX01 organisation level 1 under an operational unit; a level in use keeps its rank; built-in levels stay", func(t *testing.T) {
		b := newBranch(t)
		r := ouCreate(t, map[string]any{"name": "Rayon A", "levelCode": "org1", "parentCode": b.u})
		if want(t, r, 201) && (r.str("levelCode") != "org1" || r.str("parentCode") != b.u || r.Obj["unitTypeCode"] != nil) {
			t.Errorf("org1: %s", r.Raw)
		}
		a := ouCreate(t, map[string]any{"name": "Afdeling 01", "levelCode": "org2", "parentCode": r.str("code")})
		want(t, a, 201)
		if x := ouCreate(t, map[string]any{"name": "x", "levelCode": "org2", "parentCode": b.u}); want(t, x, 422) && !strings.Contains(x.problem("parentCode"), "Organisasi Level 1") {
			t.Errorf("org2 straight under a unit: %s", x.Raw)
		}
		if x := ouCreate(t, map[string]any{"name": "x", "levelCode": "org1", "parentCode": b.u, "unitTypeCode": "EST"}); want(t, x, 422) && x.problem("unitTypeCode") == "" {
			t.Errorf("unit type on org1: %s", x.Raw)
		}
		if x := call(t, "PUT", base+"/org1", map[string]any{"rank": 20}); want(t, x, 422) && !strings.Contains(x.problem("rank"), "dipakai") {
			t.Errorf("rank of org1 in use: %s", x.Raw)
		}
		want(t, call(t, "PUT", base+"/org1", map[string]any{"name": "Rayon"}), 200)
		want(t, call(t, "PUT", base+"/org1", map[string]any{"name": "Organisasi Level 1"}), 200)
		want(t, call(t, "DELETE", base+"/org1", nil), 409)
	})

	t.Run("OUX02 a unit type in use cannot be deleted; deactivated it still reads", func(t *testing.T) {
		b := newBranch(t)
		_ = b
		if r := call(t, "DELETE", utBase+"/EST", nil); want(t, r, 409) && !strings.Contains(r.str("message"), "masih dipakai") {
			t.Errorf("got %s", r.Raw)
		}
		ut := call(t, "POST", utBase, map[string]any{"name": "Jenis sekali pakai"}).str("code")
		want(t, call(t, "DELETE", utBase+"/"+ut, nil), 204)
	})

	t.Run("OUX03 a level with nothing between: the next lower rank is the level above", func(t *testing.T) {
		// rank 6 may be taken by OUX01; the new level sits below every other one
		lv := call(t, "POST", base, map[string]any{"name": "Petak", "rank": nextRank() + 1000})
		if !want(t, lv, 201) {
			return
		}
		ctx := context.Background()
		var aboveName string
		db.QueryRow(ctx, `SELECT name FROM pm.org_level WHERE rank < $1 AND NOT is_deleted ORDER BY rank DESC LIMIT 1`, lv.Obj["rank"]).Scan(&aboveName)
		r := ouCreate(t, map[string]any{"name": "x", "levelCode": lv.str("code"), "parentCode": newBranch(t).u})
		if r.Status != 201 && r.problem("parentCode") != fmt.Sprintf("Induk harus unit di level %s, tepat satu level di atas Petak.", aboveName) {
			t.Errorf("got %d %s", r.Status, r.Raw)
		}
	})
}

func TestOrgUnitsStress(t *testing.T) {
	t.Run("OUS01 200 parallel creates under one company: all stored, codes unique and gap-free", func(t *testing.T) {
		b := newBranch(t)
		var mu sync.Mutex
		var codes []int
		st := map[int]int{}
		parallel(200, 50, func(i int) {
			r := ouCreate(t, map[string]any{"name": fmt.Sprint("Kebun ", i), "levelCode": "unit", "parentCode": b.c, "unitTypeCode": []string{"EST", "PKS", "PLS", "KTR"}[i%4]})
			mu.Lock()
			defer mu.Unlock()
			st[r.Status]++
			var x int
			if _, err := fmt.Sscanf(r.str("code"), "MUO%d", &x); err == nil {
				codes = append(codes, x)
			}
		})
		if st[201] != 200 {
			t.Errorf("statuses %v", st)
		}
		sort.Ints(codes)
		for i := 1; i < len(codes); i++ {
			if codes[i] != codes[i-1]+1 {
				t.Errorf("gap or duplicate %d → %d", codes[i-1], codes[i])
				break
			}
		}
	})

	t.Run("OUS02 deleting a parent while children are added: never an orphan, no 500", func(t *testing.T) {
		for round := range 10 {
			b := newBranch(t)
			call(t, "DELETE", ouBase+"/"+b.u, nil) // the company is empty now
			var mu sync.Mutex
			st := map[int]int{}
			parallel(20, 20, func(i int) {
				var r resp
				if i%5 == 0 {
					r = call(t, "DELETE", ouBase+"/"+b.c, nil)
				} else {
					r = ouCreate(t, map[string]any{"name": "x", "levelCode": "unit", "parentCode": b.c, "unitTypeCode": "EST"})
				}
				mu.Lock()
				st[r.Status]++
				mu.Unlock()
			})
			if st[500] > 0 {
				t.Errorf("round %d: %v", round, st)
			}
		}
		var orphans int
		db.QueryRow(context.Background(), `SELECT count(*) FROM pm.org_unit c JOIN pm.org_unit p ON p.id = c.parent_id WHERE NOT c.is_deleted AND p.is_deleted`).Scan(&orphans)
		if orphans > 0 {
			t.Errorf("%d live units under a deleted parent", orphans)
		}
	})

	t.Run("OUS03 deleting a unit type while units take it: never a live unit of a deleted type", func(t *testing.T) {
		b := newBranch(t)
		for round := range 10 {
			ut := call(t, "POST", utBase, map[string]any{"name": fmt.Sprint("Jenis ", round)}).str("code")
			var mu sync.Mutex
			st := map[int]int{}
			parallel(20, 20, func(i int) {
				var r resp
				if i%5 == 0 {
					r = call(t, "DELETE", utBase+"/"+ut, nil)
				} else {
					r = ouCreate(t, map[string]any{"name": "x", "levelCode": "unit", "parentCode": b.c, "unitTypeCode": ut})
				}
				mu.Lock()
				st[r.Status]++
				mu.Unlock()
			})
			if st[500] > 0 {
				t.Errorf("round %d: %v", round, st)
			}
		}
		var bad int
		db.QueryRow(context.Background(), `SELECT count(*) FROM pm.org_unit u JOIN pm.unit_type t ON t.id = u.unit_type_id WHERE NOT u.is_deleted AND t.is_deleted`).Scan(&bad)
		if bad > 0 {
			t.Errorf("%d live units of a deleted type", bad)
		}
	})

	t.Run("OUS04 children and parents changed at once (lock order races): retried, no 500, nothing lost", func(t *testing.T) {
		b := newBranch(t)
		c2 := ouCreate(t, map[string]any{"name": "PT Dua", "levelCode": "company", "parentCode": b.w}).str("code")
		var kids []string
		for i := range 10 {
			kids = append(kids, ouCreate(t, map[string]any{"name": fmt.Sprint("k", i), "levelCode": "unit", "parentCode": b.c, "unitTypeCode": "EST"}).str("code"))
		}
		var mu sync.Mutex
		st := map[int]int{}
		parallel(200, 40, func(i int) {
			var r resp
			switch i % 4 {
			case 0:
				r = call(t, "PUT", ouBase+"/"+b.c, map[string]any{"description": fmt.Sprint(i)})
			case 1:
				r = call(t, "PUT", ouBase+"/"+c2, map[string]any{"description": fmt.Sprint(i)})
			default:
				to := []string{b.c, c2}[i%2]
				r = call(t, "PUT", ouBase+"/"+kids[i%len(kids)], map[string]any{"parentCode": to})
			}
			mu.Lock()
			st[r.Status]++
			mu.Unlock()
		})
		if st[200] != 200 {
			t.Errorf("statuses %v", st)
		}
		var n int
		db.QueryRow(context.Background(), `SELECT count(*) FROM pm.org_unit p JOIN pm.org_unit c ON c.parent_id = p.id WHERE p.code IN ($1, $2) AND NOT c.is_deleted`, b.c, c2).Scan(&n)
		if n != 11 { // the 10 moved ones and the branch's own unit
			t.Errorf("children %d, want 11", n)
		}
	})

	t.Run("OUS05 2000 reads of the tree while writing", func(t *testing.T) {
		b := newBranch(t)
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
					ouCreate(t, map[string]any{"name": "x", "levelCode": "unit", "parentCode": b.c, "unitTypeCode": "EST"})
				}
			}
		}()
		var mu sync.Mutex
		var lat []time.Duration
		errs := 0
		paths := []string{ouBase, ouBase + "/" + b.u}
		parallel(2000, 100, func(i int) {
			start := time.Now()
			r := call(t, "GET", paths[i%2], nil)
			mu.Lock()
			lat = append(lat, time.Since(start))
			if r.Status != 200 {
				errs++
			}
			mu.Unlock()
		})
		close(stop)
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		var total int
		db.QueryRow(context.Background(), `SELECT count(*) FROM pm.org_unit WHERE NOT is_deleted`).Scan(&total)
		p95 := lat[len(lat)*95/100]
		t.Logf("reads over %d units: p50 %v, p95 %v, max %v, errors %d", total, lat[len(lat)/2], p95, lat[len(lat)-1], errs)
		if errs > 0 || p95 > 500*time.Millisecond {
			t.Errorf("errors %d, p95 %v", errs, p95)
		}
	})
}
