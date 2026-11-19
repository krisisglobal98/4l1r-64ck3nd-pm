package orglevel

import (
	"context"
	"os"
	"testing"
	"time"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/outbox"
)

// Runs against TEST_DATABASE_URL (migrated); every test works in a transaction that is rolled back.
func withTestDB(t *testing.T) (context.Context, *Service, *database.DB) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Pool.Close)
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The rows belong to the migrated tenant; inserts take tenant_id from this setting (rolled back with tx).
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', '00000000-0000-7000-8000-0000000000a1', true)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return database.WithTx(admin, tx), &Service{Repo: PostgresRepo{DB: db}, Tx: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db}}, db
}

func TestPostgresRoundTrip(t *testing.T) {
	ctx, s, db := withTestDB(t)

	list, err := s.List(ctx)
	if err != nil || len(list) != 8 || list[0].Code != "group" || list[7].Code != "org3" || len(list[0].ID) != 36 || list[0].ID[14] != '7' {
		t.Fatalf("seeded levels with UUIDv7 ids: %v %+v", err, list)
	}

	var last int64
	if err := db.Q(ctx).QueryRow(ctx, `SELECT last_number FROM pm.code_prefix WHERE code = 'org_level'`).Scan(&last); err != nil {
		t.Fatal(err)
	}
	created, err := s.Create(ctx, V{"code": "divisi", "name": "Divisi", "rank": 9, "validFrom": "2026-01-01", "validTo": "2026-12-31"})
	if err != nil || created.Code != codeprefix.Format("ML", 3, last+1) || created.Version != 1 || created.CreatedBy != adminID || !created.Active ||
		*created.ValidTo != "2026-12-31" || created.UpdatedAt != nil {
		t.Fatalf("create (code generated, the typed one ignored): %v %+v", err, created)
	}
	if created.CreatedAt.Location() != time.UTC || time.Since(created.CreatedAt) > time.Minute {
		t.Fatalf("created_at must be the database's now() in UTC: %v", created.CreatedAt)
	}
	if _, err := s.Create(ctx, V{"name": " ", "rank": 10}); status(err) != 422 {
		t.Fatalf("blank name: %v", err)
	}
	second, err := s.Create(ctx, V{"name": "Seksi", "rank": 10})
	if err != nil || second.Code != codeprefix.Format("ML", 3, last+2) {
		t.Fatalf("a refused save gives its number back: %v %s", err, second.Code)
	}

	updated, err := s.Update(ctx, created.Code, V{"code": "x", "name": "Divisi Usaha", "active": false, "validTo": nil})
	if err != nil || updated.Code != created.Code || updated.Name != "Divisi Usaha" || updated.Rank != 9 || updated.Active || updated.ValidTo != nil ||
		updated.Version != 2 || updated.UpdatedAt == nil || *updated.UpdatedBy != adminID || updated.ID != created.ID {
		t.Fatalf("update: %v %+v", err, updated)
	}

	if err := s.Delete(ctx, "company"); status(err) != 409 {
		t.Fatalf("built-in delete: %v", err)
	}
	if err := s.Delete(ctx, created.Code); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, created.Code); status(err) != 404 {
		t.Fatalf("after delete: %v", err)
	}
	var deleted, active bool
	var deletedBy string
	if err := db.Q(ctx).QueryRow(ctx, `SELECT is_deleted, is_active, deleted_by FROM pm.org_level WHERE id = $1`, created.ID).Scan(&deleted, &active, &deletedBy); err != nil ||
		!deleted || active || deletedBy != adminID {
		t.Fatalf("soft delete keeps the row: %v deleted=%v active=%v by=%s", err, deleted, active, deletedBy)
	}
	if again, err := s.Create(ctx, V{"name": "Lagi", "rank": 9}); err != nil || again.Code == created.Code {
		t.Fatalf("deleted code reused: %v %s", err, again.Code)
	}

	var n int
	if err := db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM pm.outbox WHERE aggregate_id = $1 AND actor_id = $2`, created.ID, adminID).Scan(&n); err != nil || n != 3 {
		t.Fatalf("outbox rows: %v %d (want created, updated, deleted)", err, n)
	}
}
