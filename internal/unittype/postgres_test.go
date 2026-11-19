package unittype

import (
	"context"
	"os"
	"testing"
	"time"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/outbox"
)

// Runs against TEST_DATABASE_URL (migrated); the test works in a transaction that is rolled back.
func TestPostgresRoundTrip(t *testing.T) {
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

	list, err := s.List(ctx)
	if err != nil || len(list) != 5 || list[0].Code != "BLK" || list[0].ID[14] != '7' || list[0].HeadJabatanID != nil {
		t.Fatalf("seed: %v %+v", err, list)
	}
	var last int64
	db.Q(ctx).QueryRow(ctx, `SELECT last_number FROM pm.code_prefix WHERE code = 'unit_type'`).Scan(&last)
	u, err := s.Create(ctx, V{"name": "Kebun Bibit", "description": "Pembibitan", "validFrom": "2026-01-01", "headJabatanId": "00000000-0000-7000-8000-000000000009"})
	if err != nil || u.Code != codeprefix.Format("MJU", 3, last+1) || u.HeadJabatanID != nil || *u.ValidFrom != "2026-01-01" ||
		u.CreatedAt.Location() != time.UTC || time.Since(u.CreatedAt) > time.Minute {
		t.Fatalf("create: %v %+v", err, u)
	}
	if u, err = s.Update(ctx, "mju"+u.Code[3:], V{"name": "Kebun Bibit 2", "active": false, "version": 1}); err != nil || u.Name != "Kebun Bibit 2" || u.Active || u.Version != 2 {
		t.Fatalf("update: %v %+v", err, u)
	}
	if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.org_unit (code, name, org_level_id, unit_type_id, created_by)
		SELECT 'UJI-PKS', 'Pabrik uji', l.id, t.id, $1 FROM pm.org_level l, pm.unit_type t WHERE l.code = 'unit' AND t.code = 'PKS'`, adminID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "pks"); status(err) != 409 {
		t.Fatalf("a type used by a unit: %v", err)
	}
	if err := s.Delete(ctx, "ktr"); err != nil {
		t.Fatalf("a type no unit uses: %v", err)
	}
	if err := s.Delete(ctx, u.Code); err != nil {
		t.Fatal(err)
	}
	var deleted bool
	var by string
	if err := db.Q(ctx).QueryRow(ctx, `SELECT is_deleted, deleted_by FROM pm.unit_type WHERE id = $1`, u.ID).Scan(&deleted, &by); err != nil || !deleted || by != adminID {
		t.Fatalf("soft delete: %v %v %s", err, deleted, by)
	}
	var n int
	db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM pm.outbox WHERE aggregate_id = $1 AND aggregate_type = 'unit_type'`, u.ID).Scan(&n)
	if n != 3 {
		t.Fatalf("outbox %d", n)
	}
}
