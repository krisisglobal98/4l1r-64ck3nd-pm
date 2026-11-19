package master

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/database"
)

// Rows reads every row of a query with scan (ErrNotFound-free).
func Rows[T any](ctx context.Context, db *database.DB, scan func(pgx.Row) (T, error), sql string, args ...any) ([]T, error) {
	rows, err := db.Q(ctx).Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// One reads one row; no row is ErrNotFound.
func One[T any](ctx context.Context, db *database.DB, scan func(pgx.Row) (T, error), sql string, args ...any) (T, error) {
	t, err := scan(db.Q(ctx).QueryRow(ctx, sql, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// SoftDelete marks the live row id of table deleted (and inactive); the row stays for history.
func SoftDelete(ctx context.Context, db *database.DB, table, id, actorID string) error {
	tag, err := db.Q(ctx).Exec(ctx, `UPDATE pm.`+table+`
		SET is_deleted = true, is_active = false, deleted_at = now(), deleted_by = $2,
		    updated_at = now(), updated_by = $2, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, actorID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Exec runs one statement and reports ErrNotFound when it changed no row (an update of a deleted record).
func Exec(ctx context.Context, db *database.DB, sql string, args ...any) error {
	tag, err := db.Q(ctx).Exec(ctx, sql, args...)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
