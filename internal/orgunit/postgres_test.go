package orgunit

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/outbox"
)

const adminID = "00000000-0000-7000-8000-000000000001"

var admin = access.WithUser(context.Background(), access.User{ID: adminID, Username: "admin", Full: true})

func appErr(err error) *apperr.Error {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// problem is the message of field in a 422, "" when the save was not refused for it.
func problem(err error, field string) string {
	e := appErr(err)
	if e == nil || e.Status != 422 {
		return ""
	}
	for _, p := range e.Details.(map[string]any)["problems"].([]apperr.Problem) {
		if p.Field == field {
			return p.Message
		}
	}
	return ""
}

// The tree rules against the real tables (TEST_DATABASE_URL, migrated), in one transaction rolled back at the end.
func TestPostgresTree(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := database.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Pool.Close()
	tx, err := db.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The rows belong to the migrated tenant; inserts take tenant_id from this setting (rolled back with tx).
	if _, err := tx.Exec(context.Background(), `SELECT set_config('app.tenant_id', '00000000-0000-7000-8000-0000000000a1', true)`); err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	ctx := database.WithTx(admin, tx)
	s := &Service{Repo: PostgresRepo{DB: db}, Tx: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db}}

	mk := func(v V) Unit {
		t.Helper()
		u, err := s.Create(ctx, v)
		if err != nil {
			t.Fatalf("create %v: %v", v, err)
		}
		return u
	}
	g := mk(V{"name": "Grup", "levelCode": "group", "location": "Jakarta"})
	if len(g.Code) != 6 || g.Code[:3] != "MUO" || g.LevelCode != "group" || g.ParentID != nil || g.ID[14] != '7' || g.CreatedAt.Location() != time.UTC {
		t.Fatalf("group: %+v", g)
	}
	r := mk(V{"name": "Region", "levelCode": "REGION", "parentCode": g.Code})
	w := mk(V{"name": "Wilayah", "levelCode": "wilayah", "parentCode": r.Code})
	c1 := mk(V{"name": "PT Satu", "levelCode": "company", "parentCode": w.Code})
	c2 := mk(V{"name": "PT Dua", "levelCode": "company", "parentCode": w.Code})
	e := mk(V{"name": "Kebun", "levelCode": "unit", "parentCode": c1.Code, "unitTypeCode": "est"})
	if e.UnitTypeCode == nil || *e.UnitTypeCode != "EST" || e.UnitTypeID == nil || *e.ParentCode != c1.Code || *e.ParentID != c1.ID || e.LevelCode != "unit" {
		t.Fatalf("relations answered by id and code: %+v", e)
	}

	// Refused relations, by field and message.
	for _, c := range []struct {
		v          V
		field, msg string
	}{
		{V{"name": "x", "levelCode": "group", "parentCode": g.Code}, "parentCode", "Unit di level Group (teratas) tidak punya induk."},
		{V{"name": "x", "levelCode": "region"}, "parentCode", "Induk wajib diisi: pilih unit di level Group."},
		{V{"name": "x", "levelCode": "region", "parentCode": r.Code}, "parentCode", "Induk harus unit di level Group, tepat satu level di atas Region."},
		{V{"name": "x", "levelCode": "unit", "parentCode": w.Code, "unitTypeCode": "EST"}, "parentCode", "Induk harus unit di level Perusahaan, tepat satu level di atas Unit Operasional."},
		{V{"name": "x", "levelCode": "region", "parentCode": "NOPE"}, "parentCode", `Induk "NOPE" tidak ditemukan.`},
		{V{"name": "x", "levelCode": "nope"}, "levelCode", `Level "nope" tidak ditemukan.`},
		{V{"name": "x", "levelCode": "unit", "parentCode": c1.Code}, "unitTypeCode", "Jenis unit wajib diisi untuk level Unit Operasional."},
		{V{"name": "x", "levelCode": "unit", "parentCode": c1.Code, "unitTypeCode": "XXX"}, "unitTypeCode", `Jenis unit "XXX" tidak ditemukan.`},
		{V{"name": "x", "levelCode": "company", "parentCode": w.Code, "unitTypeCode": "EST"}, "unitTypeCode", "Jenis unit hanya untuk level Unit Operasional, bukan Perusahaan."},
	} {
		if _, err := s.Create(ctx, c.v); problem(err, c.field) != c.msg {
			t.Errorf("%v: %s = %q, want %q (%v)", c.v, c.field, problem(err, c.field), c.msg, err)
		}
	}

	// Inactive references: refused for a new choice, kept when unchanged.
	db.Q(ctx).Exec(ctx, `UPDATE pm.unit_type SET is_active = false WHERE code = 'EST'`)
	if _, err := s.Create(ctx, V{"name": "x", "levelCode": "unit", "parentCode": c1.Code, "unitTypeCode": "EST"}); problem(err, "unitTypeCode") != "Jenis unit Kebun Inti nonaktif; pilih jenis lain." {
		t.Errorf("inactive type for a new unit: %v", err)
	}
	if _, err := s.Update(ctx, e.Code, V{"name": "Kebun Satu"}); err != nil {
		t.Errorf("a unit keeps its type when it is deactivated: %v", err)
	}
	db.Q(ctx).Exec(ctx, `UPDATE pm.unit_type SET is_active = true WHERE code = 'EST'`)

	// Moves: to another company is fine; under itself, a level change with children, a deleted parent are not.
	moved, err := s.Update(ctx, e.Code, V{"parentCode": c2.Code, "version": 2})
	if err != nil || *moved.ParentCode != c2.Code || moved.Version != 3 {
		t.Fatalf("move: %v %+v", err, moved)
	}
	if _, err := s.Update(ctx, c2.Code, V{"levelCode": "unit", "parentCode": c1.Code, "unitTypeCode": "EST"}); problem(err, "levelCode") != "Level tidak dapat diubah selama unit ini punya 1 unit di bawahnya." {
		t.Errorf("level change with children: %v", err)
	}
	if _, err := s.Update(ctx, g.Code, V{"name": "Grup Baru"}); err != nil {
		t.Errorf("rename the top: %v", err)
	}
	if _, err := s.Update(ctx, e.Code, V{"version": 1}); appErr(err) == nil || appErr(err).Code != "STALE" {
		t.Errorf("stale: %v", err)
	}

	// Deletes: a parent with children stays; leaves go; then the parent.
	if err := s.Delete(ctx, c2.Code); appErr(err) == nil || appErr(err).Code != "IN_USE" {
		t.Errorf("delete a parent: %v", err)
	}
	if err := s.Delete(ctx, e.Code); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, c2.Code); err != nil {
		t.Errorf("delete the emptied parent: %v", err)
	}
	if _, err := s.Create(ctx, V{"name": "x", "levelCode": "unit", "parentCode": c2.Code, "unitTypeCode": "EST"}); problem(err, "parentCode") == "" {
		t.Errorf("a deleted parent: %v", err)
	}
	list, _ := s.List(ctx)
	if len(list) != 4 { // group, region, wilayah, PT Satu
		t.Errorf("live units %d, want 4", len(list))
	}
	var n int
	db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM pm.outbox WHERE aggregate_type = 'org_unit' AND aggregate_id = $1`, e.ID).Scan(&n)
	if n != 4 { // created, updated, moved, deleted
		t.Errorf("events %d", n)
	}
}
