//go:build e2e

// Multi-tenant isolation: every company (tenant) sees, references and numbers only its own rows. The suite acts
// for other tenants with X-Tenant-ID (default-user mode); the database check connects as the application role.
package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
)

const (
	tenantB = "00000000-0000-7000-8000-0000000000b2"
	tenantC = "00000000-0000-7000-8000-0000000000c3"
)

func as(tid string) []string { return []string{"X-Tenant-ID", tid} }

func codesOf(r resp) map[string]bool {
	out := map[string]bool{}
	for _, x := range r.List {
		out[x["code"].(string)] = true
	}
	return out
}

func TestTenantsPositive(t *testing.T) {
	t.Run("TN01 a new tenant is prepared on first access: built-in levels and code prefixes, nothing else", func(t *testing.T) {
		lv := call(t, "GET", "/pm/masters/levels", nil, as(tenantB)...)
		if !want(t, lv, 200) {
			return
		}
		got := codesOf(lv)
		for _, c := range []string{"group", "region", "wilayah", "company", "unit", "org1", "org2", "org3"} {
			if !got[c] {
				t.Errorf("built-in level %s missing in a new tenant: %v", c, got)
			}
		}
		if len(lv.List) != 8 {
			t.Errorf("a new tenant has exactly the 8 built-in levels, got %d", len(lv.List))
		}
		if ut := call(t, "GET", utBase, nil, as(tenantB)...); want(t, ut, 200) && len(ut.List) != 0 {
			t.Errorf("unit types of the first company must not appear in a new tenant: %v", codesOf(ut))
		}
		if em := call(t, "GET", emBase, nil, as(tenantB)...); want(t, em, 200) && len(em.List) != 0 {
			t.Errorf("employees of another tenant visible: %d", len(em.List))
		}
	})

	t.Run("TN02 generated codes are numbered per tenant", func(t *testing.T) {
		b1 := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank B1"}, as(tenantC)...), 201).str("code")
		b2 := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank B2"}, as(tenantC)...), 201).str("code")
		if b1 != "MBK001" || b2 != "MBK002" {
			t.Errorf("a new tenant starts its own numbering: %s %s", b1, b2)
		}
		a := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank A"}), 201).str("code")
		if a == "" || !strings.HasPrefix(a, "MBK") {
			t.Errorf("first tenant code: %s", a)
		}
	})

	t.Run("TN03 the same code, NIK and email may exist once in every tenant", func(t *testing.T) {
		for _, tid := range []string{tenantA, tenantB} {
			st := must(t, call(t, "POST", esBase, map[string]any{"code": "x", "name": "Tetap", "permanent": true}, as(tid)...), 201).str("code")
			e := call(t, "POST", emBase, map[string]any{"name": "Sama Nama", "nik": "SAMA-001", "email": "sama@contoh.co.id", "employmentStatusCode": st}, as(tid)...)
			want(t, e, 201)
		}
		// Within one tenant the rule still holds.
		st := must(t, call(t, "POST", esBase, map[string]any{"name": "Tetap 2", "permanent": true}, as(tenantB)...), 201).str("code")
		dup := call(t, "POST", emBase, map[string]any{"name": "Lain", "nik": "SAMA-001", "employmentStatusCode": st}, as(tenantB)...)
		if want(t, dup, 422) && !strings.Contains(dup.problem("nik"), "sudah dipakai") {
			t.Errorf("duplicate NIK within a tenant: %s", dup.Raw)
		}
	})
}

func TestTenantsNegative(t *testing.T) {
	aBank := must(t, call(t, "POST", bkBase, map[string]any{"name": "Rahasia A"}), 201).str("code")
	aStatus := must(t, call(t, "POST", esBase, map[string]any{"name": "Status A", "permanent": true}), 201).str("code")

	t.Run("TN04 another tenant cannot list, read, change or delete a row", func(t *testing.T) {
		if l := call(t, "GET", bkBase, nil, as(tenantB)...); want(t, l, 200) && codesOf(l)[aBank] {
			// B may have its own MBK00x; the name tells A's row apart.
			for _, x := range l.List {
				if x["name"] == "Rahasia A" {
					t.Errorf("tenant B lists tenant A's bank")
				}
			}
		}
		get := call(t, "GET", bkBase+"/"+aBank, nil, as(tenantB)...)
		if get.Status == 200 && get.str("name") == "Rahasia A" {
			t.Errorf("tenant B reads tenant A's bank")
		}
		upd := call(t, "PUT", bkBase+"/"+aBank, map[string]any{"name": "Dibajak", "version": 1}, as(tenantB)...)
		del := call(t, "DELETE", bkBase+"/"+aBank, nil, as(tenantB)...)
		_, _ = upd, del
		if a := call(t, "GET", bkBase+"/"+aBank, nil); want(t, a, 200) && a.str("name") != "Rahasia A" {
			t.Errorf("tenant A's bank changed by tenant B: %s", a.Raw)
		}
	})

	t.Run("TN05 a row cannot reference another tenant's row", func(t *testing.T) {
		r := call(t, "POST", emBase, map[string]any{"name": "Penyusup", "nik": nextNik(), "employmentStatusCode": aStatus}, as(tenantC)...)
		if r.Status != 422 || r.problem("employmentStatusCode") == "" {
			t.Errorf("reference across tenants must be refused as not found: %d %s", r.Status, r.Raw)
		}
	})

	t.Run("TN06 a malformed tenant is not used", func(t *testing.T) {
		for _, h := range []string{"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz", "00000000-0000-7000-8000-0000000000b2' OR 1=1--", "x"} {
			r := call(t, "GET", bkBase, nil, "X-Tenant-ID", h)
			if r.Status != 200 {
				t.Errorf("header %q: status %d %s", h, r.Status, r.Raw)
				continue
			}
			// It falls back to the default tenant: A's row is there.
			found := false
			for _, x := range r.List {
				found = found || x["name"] == "Rahasia A"
			}
			if !found {
				t.Errorf("header %q did not fall back to the default tenant", h)
			}
		}
	})

	t.Run("TN07 the database itself keeps tenants apart for the application role", func(t *testing.T) {
		dsn := os.Getenv("TEST_APP_DATABASE_URL")
		if dsn == "" {
			t.Skip("TEST_APP_DATABASE_URL not set")
		}
		ctx := context.Background()
		c, err := pgx.Connect(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close(ctx)
		var n int
		if err := c.QueryRow(ctx, `SELECT count(*) FROM pm.bank`).Scan(&n); err != nil || n != 0 {
			t.Errorf("no tenant set: must see no rows: %v %d", err, n)
		}
		if _, err := c.Exec(ctx, `INSERT INTO pm.bank (code, name, created_by) VALUES ('ZZ1', 'Tanpa tenant', '00000000-0000-7000-8000-000000000001')`); err == nil {
			t.Errorf("insert without a tenant must fail")
		}
		if _, err := c.Exec(ctx, `SELECT set_config('app.tenant_id', $1, false)`, tenantB); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Exec(ctx, `INSERT INTO pm.bank (code, name, tenant_id, created_by) VALUES ('ZZ2', 'Menyamar', $1, '00000000-0000-7000-8000-000000000001')`, tenantA); err == nil {
			t.Errorf("insert into another tenant must fail (policy WITH CHECK)")
		}
		if err := c.QueryRow(ctx, `SELECT count(*) FROM pm.bank WHERE name = 'Rahasia A'`).Scan(&n); err != nil || n != 0 {
			t.Errorf("tenant B sees tenant A's rows in the database: %v %d", err, n)
		}
		if tag, err := c.Exec(ctx, `UPDATE pm.bank SET name = 'x' WHERE tenant_id = $1`, tenantA); err != nil || tag.RowsAffected() != 0 {
			t.Errorf("tenant B updates tenant A's rows: %v %d", err, tag.RowsAffected())
		}
	})
}

func TestTenantsStress(t *testing.T) {
	t.Run("TN08 300 interleaved requests of three tenants: no answer ever carries another tenant's rows", func(t *testing.T) {
		names := map[string]string{tenantA: "Hanya A", tenantB: "Hanya B", tenantC: "Hanya C"}
		for tid, n := range names {
			must(t, call(t, "POST", bkBase, map[string]any{"name": n}, as(tid)...), 201)
		}
		tids := []string{tenantA, tenantB, tenantC}
		var leaks, fails atomic.Int64
		parallel(300, 24, func(i int) {
			tid := tids[i%3]
			r := call(t, "GET", bkBase, nil, as(tid)...)
			if r.Status != 200 {
				fails.Add(1)
				return
			}
			mine := false
			for _, x := range r.List {
				n, _ := x["name"].(string)
				for other, on := range names {
					if n == on && other != tid {
						leaks.Add(1)
					}
				}
				mine = mine || n == names[tid]
			}
			if !mine {
				leaks.Add(1)
			}
		})
		if leaks.Load() != 0 || fails.Load() != 0 {
			t.Errorf("leaks %d, failures %d", leaks.Load(), fails.Load())
		}
	})

	t.Run("TN09 30 tenants prepared at once: each gets its levels exactly once", func(t *testing.T) {
		var bad atomic.Int64
		parallel(60, 30, func(i int) {
			tid := fmt.Sprintf("00000000-0000-7000-8000-%012x", 0xd00+i%30)
			r := call(t, "GET", "/pm/masters/levels", nil, as(tid)...)
			if r.Status != 200 || len(r.List) != 8 {
				bad.Add(1)
			}
		})
		if bad.Load() != 0 {
			t.Errorf("%d tenants not prepared exactly once", bad.Load())
		}
	})
}
