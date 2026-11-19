//go:build e2e

// Level Organisasi (/pm/masters/levels): the pilot suite every master copies.
package e2e

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const base = "/pm/masters/levels"

var rankSeq struct {
	sync.Mutex
	n int
}

// nextRank gives every created level its own rank above the built-in ones.
func nextRank() int {
	rankSeq.Lock()
	defer rankSeq.Unlock()
	rankSeq.n++
	return 100 + rankSeq.n
}

func create(t testing.TB, extra map[string]any) resp {
	t.Helper()
	body := map[string]any{"name": "Level uji", "rank": nextRank()}
	for k, v := range extra {
		body[k] = v
	}
	return call(t, "POST", base, body)
}

/* ---------------- Positive ---------------- */

func TestPositive(t *testing.T) {
	t.Run("P01 health and readiness", func(t *testing.T) {
		want(t, call(t, "GET", "/healthz", nil), 200)
		want(t, call(t, "GET", "/readyz", nil), 200)
	})

	t.Run("P02 list: seeded levels by rank, standard fields, UUIDv7, UTC", func(t *testing.T) {
		r := call(t, "GET", base, nil)
		if !want(t, r, 200) {
			return
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("content type %q", ct)
		}
		var codes []string
		for _, l := range r.List {
			codes = append(codes, l["code"].(string))
			if !uuidV7.MatchString(l["id"].(string)) {
				t.Errorf("id %v is not UUIDv7", l["id"])
			}
			if !utcTime.MatchString(l["createdAt"].(string)) {
				t.Errorf("createdAt %v is not UTC", l["createdAt"])
			}
			for _, k := range []string{"name", "description", "active", "state", "status", "remark", "approvalLevelRequired", "approvedAt", "approvedBy", "validFrom", "validTo", "createdBy", "updatedAt", "updatedBy", "version", "rank"} {
				if _, ok := l[k]; !ok {
					t.Errorf("field %s missing", k)
				}
			}
			for _, k := range []string{"scopeLevel", "isDeleted", "deletedAt", "deletedBy"} {
				if _, ok := l[k]; ok {
					t.Errorf("field %s must not be answered", k)
				}
			}
		}
		if strings.Join(codes[:5], ",") != "group,region,wilayah,company,unit" {
			t.Errorf("order %v", codes)
		}
	})

	t.Run("P03 create minimal: generated code, defaults, audit", func(t *testing.T) {
		before := time.Now().Add(-time.Minute)
		r := create(t, nil)
		if !want(t, r, 201) {
			return
		}
		if !genCode.MatchString(r.str("code")) || r.Obj["active"] != true || r.Obj["version"] != 1.0 || r.str("createdBy") != userID ||
			r.Obj["updatedAt"] != nil || r.Obj["description"] != nil || r.Obj["validFrom"] != nil {
			t.Errorf("got %s", r.Raw)
		}
		at, err := time.Parse(time.RFC3339Nano, r.str("createdAt"))
		if err != nil || !utcTime.MatchString(r.str("createdAt")) || at.Before(before) || at.After(time.Now().Add(time.Minute)) {
			t.Errorf("createdAt %q must be now, in UTC", r.str("createdAt"))
		}
	})

	t.Run("P04 create full: description, validity, inactive", func(t *testing.T) {
		r := create(t, map[string]any{"description": "Baris 1\nBaris 2 — <b>ä</b> 😀", "validFrom": "2026-01-01", "validTo": "2026-12-31", "active": false})
		if want(t, r, 201) && (r.str("description") != "Baris 1\nBaris 2 — <b>ä</b> 😀" || r.str("validFrom") != "2026-01-01" || r.str("validTo") != "2026-12-31" || r.Obj["active"] != false) {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("P05 boundaries accepted: name 150, description 500, rank as numeric string, same-day validity", func(t *testing.T) {
		r := create(t, map[string]any{"name": strings.Repeat("N", 150), "description": strings.Repeat("é", 500), "validFrom": "2026-05-05", "validTo": "2026-05-05"})
		want(t, r, 201)
		r = call(t, "POST", base, map[string]any{"name": "Rank teks", "rank": fmt.Sprint(nextRank())})
		want(t, r, 201)
	})

	t.Run("P05b largest rank accepted; name trimmed", func(t *testing.T) {
		r := call(t, "POST", base, map[string]any{"name": "  Paling bawah  ", "rank": 2147483647})
		if want(t, r, 201) && (r.Obj["rank"] != 2147483647.0 || r.str("name") != "Paling bawah") {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("P06 codes are consecutive", func(t *testing.T) {
		a, b := create(t, nil), create(t, nil)
		var x, y int
		fmt.Sscanf(a.str("code"), "ML%d", &x)
		fmt.Sscanf(b.str("code"), "ML%d", &y)
		if y != x+1 {
			t.Errorf("%s then %s", a.str("code"), b.str("code"))
		}
	})

	t.Run("P07 get by code, also in another case", func(t *testing.T) {
		c := create(t, nil).str("code")
		r := call(t, "GET", base+"/"+c, nil)
		if want(t, r, 200) && r.str("code") != c {
			t.Errorf("got %s", r.Raw)
		}
		want(t, call(t, "GET", base+"/"+strings.ToLower(c), nil), 200)
		want(t, call(t, "GET", base+"/UNIT", nil), 200)
	})

	t.Run("P08 partial update keeps the rest, raises version, sets updated audit", func(t *testing.T) {
		c := create(t, map[string]any{"description": "awal", "validFrom": "2026-01-01"})
		r := call(t, "PUT", base+"/"+c.str("code"), map[string]any{"name": "Nama baru"})
		if !want(t, r, 200) {
			return
		}
		if r.str("name") != "Nama baru" || r.str("description") != "awal" || r.str("validFrom") != "2026-01-01" || r.Obj["rank"] != c.Obj["rank"] ||
			r.Obj["version"] != 2.0 || r.str("updatedBy") != userID || !utcTime.MatchString(r.str("updatedAt")) || r.str("id") != c.str("id") || r.str("createdAt") != c.str("createdAt") {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("P09 update clears optional fields with null and empty text", func(t *testing.T) {
		c := create(t, map[string]any{"description": "x", "validFrom": "2026-01-01", "validTo": "2026-02-01"})
		r := call(t, "PUT", base+"/"+c.str("code"), map[string]any{"description": "", "validFrom": nil, "validTo": ""})
		if want(t, r, 200) && (r.Obj["description"] != nil || r.Obj["validFrom"] != nil || r.Obj["validTo"] != nil) {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("P10 deactivate and activate", func(t *testing.T) {
		c := create(t, nil).str("code")
		if r := call(t, "PUT", base+"/"+c, map[string]any{"active": false}); want(t, r, 200) && r.Obj["active"] != false {
			t.Errorf("got %s", r.Raw)
		}
		if r := call(t, "PUT", base+"/"+c, map[string]any{"active": true}); want(t, r, 200) && r.Obj["active"] != true {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("P11 built-in levels may change name, rank and status", func(t *testing.T) {
		r := call(t, "PUT", base+"/unit", map[string]any{"name": "Unit Ops"})
		if want(t, r, 200) && r.str("name") != "Unit Ops" {
			t.Errorf("got %s", r.Raw)
		}
		call(t, "PUT", base+"/unit", map[string]any{"name": "Unit Operasional"})
	})

	t.Run("P12 soft delete: 204, gone from answers, row kept with audit, events", func(t *testing.T) {
		c := create(t, nil)
		code, id := c.str("code"), c.str("id")
		call(t, "PUT", base+"/"+code, map[string]any{"name": "Sebelum hapus"})
		r := call(t, "DELETE", base+"/"+code, nil)
		if want(t, r, 204) && len(r.Raw) != 0 {
			t.Errorf("204 with a body: %s", r.Raw)
		}
		want(t, call(t, "GET", base+"/"+code, nil), 404)
		for _, l := range call(t, "GET", base, nil).List {
			if l["code"] == code {
				t.Error("deleted level still listed")
			}
		}
		var deleted, active bool
		var by string
		var at time.Time
		if err := db.QueryRow(context.Background(), `SELECT is_deleted, is_active, deleted_by::text, deleted_at FROM pm.org_level WHERE id = $1`, id).Scan(&deleted, &active, &by, &at); err != nil ||
			!deleted || active || by != userID || time.Since(at) > time.Minute {
			t.Errorf("row: %v deleted=%v active=%v by=%s at=%v", err, deleted, active, by, at)
		}
		rows, _ := db.Query(context.Background(), `SELECT event_type FROM pm.outbox WHERE aggregate_id = $1 ORDER BY occurred_at, id`, id)
		var types []string
		for rows.Next() {
			var s string
			rows.Scan(&s)
			types = append(types, s)
		}
		if strings.Join(types, ",") != "pm.org_level.created,pm.org_level.updated,pm.org_level.deleted" {
			t.Errorf("events %v", types)
		}
	})

	t.Run("P13 a deleted code is never handed out again", func(t *testing.T) {
		c := create(t, nil).str("code")
		call(t, "DELETE", base+"/"+c, nil)
		if n := create(t, nil).str("code"); n == c {
			t.Errorf("code %s reused", c)
		}
	})

	t.Run("P14 request id: kept when valid, made when absent, stored with the event", func(t *testing.T) {
		r := call(t, "POST", base, map[string]any{"name": "Req", "rank": nextRank()}, "X-Request-ID", "uji-e2e-12345")
		if r.Header.Get("X-Request-ID") != "uji-e2e-12345" {
			t.Errorf("request id %q", r.Header.Get("X-Request-ID"))
		}
		var rid string
		db.QueryRow(context.Background(), `SELECT coalesce(request_id,'') FROM pm.outbox WHERE aggregate_id = $1`, r.str("id")).Scan(&rid)
		if rid != "uji-e2e-12345" {
			t.Errorf("outbox request_id %q", rid)
		}
		if g := call(t, "GET", base, nil).Header.Get("X-Request-ID"); len(g) < 8 {
			t.Errorf("generated request id %q", g)
		}
		if g := call(t, "GET", base, nil, "X-Request-ID", "bad id <script>"+strings.Repeat("x", 200)).Header.Get("X-Request-ID"); strings.Contains(g, "<") || len(g) > 64 {
			t.Errorf("unsafe request id echoed: %q", g)
		}
	})

	t.Run("P15 the stored instant is UTC whatever the session zone", func(t *testing.T) {
		r := create(t, nil)
		var dbNow time.Time
		db.QueryRow(context.Background(), `SELECT created_at FROM pm.org_level WHERE id = $1`, r.str("id")).Scan(&dbNow)
		api, _ := time.Parse(time.RFC3339Nano, r.str("createdAt"))
		if !api.Equal(dbNow) {
			t.Errorf("api %v != db %v", api, dbNow)
		}
	})

	t.Run("P16 text is stored as data (markup and SQL are not interpreted)", func(t *testing.T) {
		evil := `'); DROP TABLE pm.org_level; -- <script>alert(1)</script>`
		r := create(t, map[string]any{"name": evil})
		if want(t, r, 201) && r.str("name") != evil {
			t.Errorf("got %s", r.Raw)
		}
		want(t, call(t, "GET", base, nil), 200)
	})

	t.Run("P18 the bodies the ALIR form sends: create, edit with version, a second admin's stale save", func(t *testing.T) {
		form := map[string]any{"code": nil, "name": "Divisi", "rank": fmt.Sprint(nextRank()), "description": "", "validFrom": "", "validTo": "", "active": true}
		c := call(t, "POST", base, form)
		if !want(t, c, 201) {
			return
		}
		code := c.str("code")
		edit := map[string]any{"code": code, "name": "Divisi Usaha", "rank": fmt.Sprint(c.Obj["rank"]), "description": "Baris 1\nBaris 2", "validFrom": "2026-01-01", "validTo": "", "active": true}
		adminA, adminB := map[string]any{"version": 1}, map[string]any{"version": 1, "name": "Versi admin B"}
		for k, v := range edit {
			adminA[k] = v
			if k != "name" {
				adminB[k] = v
			}
		}
		if r := call(t, "PUT", base+"/"+code, adminA); want(t, r, 200) && (r.Obj["version"] != 2.0 || r.str("description") != "Baris 1\nBaris 2" || r.Obj["validTo"] != nil) {
			t.Errorf("got %s", r.Raw)
		}
		if r := call(t, "PUT", base+"/"+code, adminB); want(t, r, 409) && (r.str("code") != "STALE" || !strings.Contains(r.str("message"), "Muat ulang")) {
			t.Errorf("got %s", r.Raw)
		}
		// The list's activate/deactivate sends the whole form of the row with its version.
		row := call(t, "GET", base+"/"+code, nil)
		toggle := map[string]any{"name": row.str("name"), "rank": row.Obj["rank"], "description": row.Obj["description"], "validFrom": row.Obj["validFrom"], "validTo": row.Obj["validTo"], "active": false, "version": row.Obj["version"]}
		if r := call(t, "PUT", base+"/"+code, toggle); want(t, r, 200) && (r.Obj["active"] != false || r.str("name") != "Divisi Usaha") {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("P17 code skips numbers already taken; longer numbers are kept whole", func(t *testing.T) {
		ctx := context.Background()
		var last int64
		db.QueryRow(ctx, `SELECT last_number FROM pm.code_prefix WHERE code = 'org_level'`).Scan(&last)
		taken := fmt.Sprintf("ML%03d", last+1)
		db.Exec(ctx, `INSERT INTO pm.org_level (code, name, rank, created_by) VALUES ($1, 'Manual', $2, $3)`, strings.ToLower(taken), nextRank(), systemID)
		if r := create(t, nil); r.str("code") != fmt.Sprintf("ML%03d", last+2) {
			t.Errorf("want ML%03d, got %s", last+2, r.Raw)
		}
		db.Exec(ctx, `UPDATE pm.code_prefix SET last_number = 999 WHERE code = 'org_level'`)
		if r := create(t, nil); r.str("code") != "ML1000" {
			t.Errorf("want ML1000, got %s", r.Raw)
		}
	})
}

/* ---------------- Negative ---------------- */

func TestNegative(t *testing.T) {
	ok := func(extra map[string]any) map[string]any {
		b := map[string]any{"name": "Level uji", "rank": nextRank()}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	cases := []struct {
		id, name string
		body     map[string]any
		field    string
		msg      string
	}{
		{"N01", "name missing", map[string]any{"rank": 7}, "name", "Nama wajib diisi."},
		{"N02", "name blank", ok(map[string]any{"name": "   "}), "name", "Nama wajib diisi."},
		{"N03", "name 151", ok(map[string]any{"name": strings.Repeat("n", 151)}), "name", "Nama paling banyak 150 karakter."},
		{"N04", "name not text", ok(map[string]any{"name": 12}), "name", "Nama harus berupa teks."},
		{"N05", "name with NUL", ok(map[string]any{"name": "a\x00b"}), "name", "Nama berisi karakter yang tidak diizinkan."},
		{"N06", "description 501", ok(map[string]any{"description": strings.Repeat("d", 501)}), "description", "Deskripsi paling banyak 500 karakter."},
		{"N07", "description not text", ok(map[string]any{"description": []int{1}}), "description", "Deskripsi harus berupa teks."},
		{"N08", "rank missing", map[string]any{"name": "X"}, "rank", "Urutan wajib diisi."},
		{"N09", "rank 0", ok(map[string]any{"rank": 0}), "rank", "Urutan minimal 1."},
		{"N10", "rank negative", ok(map[string]any{"rank": -3}), "rank", "Urutan minimal 1."},
		{"N11", "rank fraction", ok(map[string]any{"rank": 1.5}), "rank", "Urutan harus bilangan bulat."},
		{"N12", "rank text", ok(map[string]any{"rank": "abc"}), "rank", "Urutan minimal 1."},
		{"N13", "rank beyond int32", ok(map[string]any{"rank": 3000000000}), "rank", "Urutan paling besar 2147483647."},
		{"N14", "rank boolean", ok(map[string]any{"rank": true}), "rank", "Urutan minimal 1."},
		{"N15", "active text", ok(map[string]any{"active": "ya"}), "active", "Status aktif harus ya atau tidak."},
		{"N16", "date impossible", ok(map[string]any{"validFrom": "2026-02-30"}), "validFrom", "Berlaku mulai bukan tanggal yang sah (format YYYY-MM-DD)."},
		{"N17", "date with time", ok(map[string]any{"validFrom": "2026-01-01T00:00:00Z"}), "validFrom", "Berlaku mulai bukan tanggal yang sah (format YYYY-MM-DD)."},
		{"N18", "date as number", ok(map[string]any{"validTo": 20260101}), "validTo", "Berlaku sampai harus berupa tanggal."},
		{"N19", "end before start", ok(map[string]any{"validFrom": "2026-06-02", "validTo": "2026-06-01"}), "validTo", "Tanggal selesai tidak boleh sebelum tanggal mulai."},
		{"N20", "rank of another live level", ok(map[string]any{"rank": 5}), "rank", "Urutan 5 sudah dipakai level Unit Operasional."},
	}
	for _, c := range cases {
		t.Run(c.id+" "+c.name, func(t *testing.T) {
			r := call(t, "POST", base, c.body)
			if !want(t, r, 422) {
				return
			}
			if r.str("code") != "VALIDATION" || r.problem(c.field) != c.msg {
				t.Errorf("%s: got %q, want %q (%s)", c.field, r.problem(c.field), c.msg, trunc(r.Raw))
			}
		})
	}

	t.Run("N21 every problem of a form at once", func(t *testing.T) {
		r := call(t, "POST", base, map[string]any{"name": "", "rank": 0, "validFrom": "x"})
		if want(t, r, 422) && (r.problem("name") == "" || r.problem("rank") == "" || r.problem("validFrom") == "") {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("N22 refused saves take no code number", func(t *testing.T) {
		var a, b int64
		db.QueryRow(context.Background(), `SELECT last_number FROM pm.code_prefix WHERE code='org_level'`).Scan(&a)
		call(t, "POST", base, map[string]any{"name": ""})
		db.QueryRow(context.Background(), `SELECT last_number FROM pm.code_prefix WHERE code='org_level'`).Scan(&b)
		if a != b {
			t.Errorf("counter %d → %d", a, b)
		}
	})

	t.Run("N23 typed code, id, audit, version and workflow are ignored on create", func(t *testing.T) {
		r := create(t, map[string]any{"code": "custom", "id": "00000000-0000-7000-8000-00000000ffff", "createdBy": systemID, "version": 99,
			"createdAt": "2000-01-01T00:00:00Z", "state": "APPROVED", "status": "POSTED", "remark": "x", "approvalLevelRequired": 3, "approvedBy": systemID, "isDeleted": true})
		if !want(t, r, 201) {
			return
		}
		if r.str("code") == "custom" || r.str("id") == "00000000-0000-7000-8000-00000000ffff" || r.str("createdBy") != userID || r.Obj["version"] != 1.0 ||
			strings.HasPrefix(r.str("createdAt"), "2000") || r.Obj["state"] != nil || r.Obj["status"] != nil || r.Obj["remark"] != nil || r.Obj["approvalLevelRequired"] != nil || r.Obj["approvedBy"] != nil {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("N24 update cannot change code, id or workflow", func(t *testing.T) {
		c := create(t, nil)
		r := call(t, "PUT", base+"/"+c.str("code"), map[string]any{"code": "lain", "id": "x", "status": "VOID", "createdBy": systemID})
		if want(t, r, 200) && (r.str("code") != c.str("code") || r.str("id") != c.str("id") || r.Obj["status"] != nil || r.str("createdBy") != userID) {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("N25 update with a stale version is refused", func(t *testing.T) {
		c := create(t, nil).str("code")
		call(t, "PUT", base+"/"+c, map[string]any{"name": "Pertama", "version": 1})
		r := call(t, "PUT", base+"/"+c, map[string]any{"name": "Basi", "version": 1})
		if want(t, r, 409) && r.str("code") != "STALE" {
			t.Errorf("got %s", r.Raw)
		}
		if g := call(t, "GET", base+"/"+c, nil); g.str("name") != "Pertama" {
			t.Errorf("stale write applied: %s", g.Raw)
		}
	})

	t.Run("N26 update problems", func(t *testing.T) {
		c := create(t, nil).str("code")
		r := call(t, "PUT", base+"/"+c, map[string]any{"name": nil, "rank": "x"})
		if want(t, r, 422) && (r.problem("name") != "Nama wajib diisi." || r.problem("rank") == "") {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("N27 unknown code: get, update, delete are 404 with the error body", func(t *testing.T) {
		for _, m := range []string{"GET", "PUT", "DELETE"} {
			var body any
			if m == "PUT" {
				body = map[string]any{"name": "x"}
			}
			r := call(t, m, base+"/ML999999", body)
			if want(t, r, 404) && (r.str("code") != "NOT_FOUND" || r.str("message") != "Data tidak ditemukan.") {
				t.Errorf("%s: %s", m, r.Raw)
			}
		}
	})

	t.Run("N28 delete twice: second is 404", func(t *testing.T) {
		c := create(t, nil).str("code")
		want(t, call(t, "DELETE", base+"/"+c, nil), 204)
		want(t, call(t, "DELETE", base+"/"+c, nil), 404)
		want(t, call(t, "PUT", base+"/"+c, map[string]any{"name": "x"}), 404)
	})

	t.Run("N29 built-in levels cannot be deleted, any case", func(t *testing.T) {
		for _, c := range []string{"group", "region", "wilayah", "company", "unit", "Company"} {
			r := call(t, "DELETE", base+"/"+c, nil)
			if want(t, r, 409) && (r.str("code") != "IN_USE" || !strings.Contains(r.str("message"), "Nonaktifkan saja")) {
				t.Errorf("%s: %s", c, r.Raw)
			}
		}
	})

	t.Run("N30 malformed bodies are 400 with the error body", func(t *testing.T) {
		for name, b := range map[string]string{"empty": "", "not json": "{name:", "array": `[{"name":"x"}]`, "null": "null", "string": `"x"`, "trailing data": `{"name":"x","rank":7} {"x":1}`} {
			r := call(t, "POST", base, b)
			if want(t, r, 400) && r.str("code") == "" {
				t.Errorf("%s: %s", name, r.Raw)
			}
		}
	})

	t.Run("N31 body over 1 MiB is refused, not stored", func(t *testing.T) {
		big := `{"name":"` + strings.Repeat("a", 2<<20) + `","rank":7}`
		r := call(t, "POST", base, big)
		if r.Status != 413 || r.str("code") != "TOO_LARGE" {
			t.Errorf("no error body: %s", trunc(r.Raw))
		}
	})

	t.Run("N32 unknown route and method answer the error body", func(t *testing.T) {
		r := call(t, "GET", "/pm/masters/nothing", nil)
		if want(t, r, 404) && r.str("code") != "NOT_FOUND" {
			t.Errorf("unknown path: %s", r.Raw)
		}
		r = call(t, "PATCH", base+"/unit", map[string]any{"name": "x"})
		if want(t, r, 405) && r.str("code") != "METHOD_NOT_ALLOWED" {
			t.Errorf("PATCH: %s", r.Raw)
		}
	})

	t.Run("N33 odd codes in the path are not found, never an error", func(t *testing.T) {
		for _, c := range []string{"%27%20OR%201%3D1--", strings.Repeat("A", 500), "%00", "%FF", "a%00b", "..%2F..%2Fetc", "%E2%9C%93"} {
			r := call(t, "GET", base+"/"+c, nil)
			if r.Status != 404 {
				t.Errorf("%s: %d %s", c, r.Status, trunc(r.Raw))
			}
		}
	})

	t.Run("N34 upload is not available yet (501)", func(t *testing.T) {
		r := call(t, "POST", base+"/upload", "x")
		if want(t, r, 501) && r.str("code") != "NO_IMPORT" {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("N35 no active prefix: 409 NO_PREFIX and nothing stored", func(t *testing.T) {
		ctx := context.Background()
		var before, after int
		db.QueryRow(ctx, `SELECT count(*) FROM pm.org_level`).Scan(&before)
		db.Exec(ctx, `UPDATE pm.code_prefix SET is_active = false WHERE code = 'org_level'`)
		defer db.Exec(ctx, `UPDATE pm.code_prefix SET is_active = true WHERE code = 'org_level'`)
		r := create(t, nil)
		if want(t, r, 409) && r.str("code") != "NO_PREFIX" {
			t.Errorf("got %s", r.Raw)
		}
		db.QueryRow(ctx, `SELECT count(*) FROM pm.org_level`).Scan(&after)
		if before != after {
			t.Errorf("rows %d → %d", before, after)
		}
	})

	t.Run("N36 a deleted level's rank is free again", func(t *testing.T) {
		rank := nextRank()
		c := call(t, "POST", base, map[string]any{"name": "A", "rank": rank}).str("code")
		call(t, "DELETE", base+"/"+c, nil)
		want(t, call(t, "POST", base, map[string]any{"name": "B", "rank": rank}), 201)
	})

	t.Run("N38 update onto another live level's rank is refused; own rank is fine", func(t *testing.T) {
		c := create(t, nil)
		r := call(t, "PUT", base+"/"+c.str("code"), map[string]any{"rank": 4})
		if want(t, r, 422) && r.problem("rank") != "Urutan 4 sudah dipakai level Perusahaan." {
			t.Errorf("got %s", r.Raw)
		}
		want(t, call(t, "PUT", base+"/"+c.str("code"), map[string]any{"rank": c.Obj["rank"], "name": "Tetap"}), 200)
	})

	t.Run("N39 a version that is not a number is refused", func(t *testing.T) {
		c := create(t, nil).str("code")
		for _, v := range []any{"x", 0, 1.5, true} {
			r := call(t, "PUT", base+"/"+c, map[string]any{"name": "x", "version": v})
			if want(t, r, 422) && r.problem("version") == "" {
				t.Errorf("version %v: %s", v, r.Raw)
			}
		}
	})

	t.Run("N37 errors never leak internals", func(t *testing.T) {
		r := call(t, "POST", base, map[string]any{"name": "a\x00b", "rank": nextRank()})
		if r.Status >= 500 || strings.Contains(strings.ToLower(string(r.Raw)), "sqlstate") || strings.Contains(string(r.Raw), "pgx") {
			t.Errorf("status %d: %s", r.Status, r.Raw)
		}
	})
}

/* ---------------- Stress and concurrency ---------------- */

func TestStress(t *testing.T) {
	t.Run("S01 300 parallel creates: all stored, codes unique and gap-free", func(t *testing.T) {
		const n = 300
		var mu sync.Mutex
		statuses := map[int]int{}
		var codes []int
		parallel(n, 50, func(int) {
			r := create(t, nil)
			mu.Lock()
			defer mu.Unlock()
			statuses[r.Status]++
			var x int
			if _, err := fmt.Sscanf(r.str("code"), "ML%d", &x); err == nil {
				codes = append(codes, x)
			}
		})
		if statuses[201] != n {
			t.Errorf("statuses %v", statuses)
		}
		sort.Ints(codes)
		for i := 1; i < len(codes); i++ {
			if codes[i] != codes[i-1]+1 {
				t.Errorf("gap or duplicate between ML%03d and ML%03d", codes[i-1], codes[i])
				break
			}
		}
	})

	t.Run("S02 100 parallel updates of one level: none lost", func(t *testing.T) {
		c := create(t, nil).str("code")
		var mu sync.Mutex
		statuses := map[int]int{}
		parallel(100, 25, func(i int) {
			r := call(t, "PUT", base+"/"+c, map[string]any{"description": fmt.Sprint("ubah ", i)})
			mu.Lock()
			statuses[r.Status]++
			mu.Unlock()
		})
		g := call(t, "GET", base+"/"+c, nil)
		if statuses[200] != 100 || g.Obj["version"] != 101.0 {
			t.Errorf("statuses %v, version %v", statuses, g.Obj["version"])
		}
		var n int
		db.QueryRow(context.Background(), `SELECT count(*) FROM pm.outbox o JOIN pm.org_level l ON l.id = o.aggregate_id WHERE l.code = $1 AND o.event_type = 'pm.org_level.updated'`, c).Scan(&n)
		if n != 100 {
			t.Errorf("updated events %d, want 100", n)
		}
	})

	t.Run("S03 delete racing updates: one delete wins, no server errors", func(t *testing.T) {
		for round := range 10 {
			c := create(t, nil).str("code")
			var mu sync.Mutex
			statuses := map[string]int{}
			parallel(20, 20, func(i int) {
				var r resp
				if i%5 == 0 {
					r = call(t, "DELETE", base+"/"+c, nil)
				} else {
					r = call(t, "PUT", base+"/"+c, map[string]any{"name": fmt.Sprint("r", i)})
				}
				mu.Lock()
				statuses[fmt.Sprint(r.Status)]++
				mu.Unlock()
			})
			if statuses["204"] != 1 || statuses["500"] > 0 || statuses["204"]+statuses["200"]+statuses["404"] != 20 {
				t.Errorf("round %d statuses %v", round, statuses)
			}
		}
	})

	t.Run("S04 parallel creates on one rank: exactly one wins", func(t *testing.T) {
		rank := nextRank()
		var mu sync.Mutex
		statuses := map[int]int{}
		parallel(30, 30, func(int) {
			r := call(t, "POST", base, map[string]any{"name": "Sama", "rank": rank})
			mu.Lock()
			statuses[r.Status]++
			mu.Unlock()
		})
		if statuses[201] != 1 || statuses[422] != 29 {
			t.Errorf("statuses %v", statuses)
		}
	})

	t.Run("S05 read load: 3000 reads, 100 at a time, while writing", func(t *testing.T) {
		stop := make(chan struct{})
		go func() {
			for {
				select {
				case <-stop:
					return
				default:
					create(t, nil)
				}
			}
		}()
		var mu sync.Mutex
		var lat []time.Duration
		errs := 0
		parallel(3000, 100, func(i int) {
			start := time.Now()
			path := base
			if i%2 == 1 {
				path = base + "/unit"
			}
			r := call(t, "GET", path, nil)
			d := time.Since(start)
			mu.Lock()
			lat = append(lat, d)
			if r.Status != 200 {
				errs++
			}
			mu.Unlock()
		})
		close(stop)
		sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
		p50, p95, p99 := lat[len(lat)/2], lat[len(lat)*95/100], lat[len(lat)*99/100]
		t.Logf("reads: p50 %v, p95 %v, p99 %v, max %v, errors %d", p50, p95, p99, lat[len(lat)-1], errs)
		if errs > 0 || p95 > 500*time.Millisecond {
			t.Errorf("errors %d, p95 %v (limit 500ms)", errs, p95)
		}
	})

	t.Run("S06 the service is healthy after the load", func(t *testing.T) {
		want(t, call(t, "GET", "/readyz", nil), 200)
		var dup int
		db.QueryRow(context.Background(), `SELECT count(*) FROM (SELECT lower(code) FROM pm.org_level GROUP BY 1 HAVING count(*) > 1) d`).Scan(&dup)
		if dup > 0 {
			t.Errorf("%d duplicate codes", dup)
		}
	})
}
