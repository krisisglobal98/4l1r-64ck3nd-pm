package orglevel

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// PostgresRepo stores levels in pm.org_level. Every time comes from the database's now().
type PostgresRepo struct{ DB *database.DB }

var columns = record.Columns + `, rank`

func scan(row pgx.Row) (Level, error) {
	var l Level
	err := l.Scan(row, &l.Rank)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

func (r PostgresRepo) List(ctx context.Context) ([]Level, error) {
	rows, err := r.DB.Q(ctx).Query(ctx, `SELECT `+columns+` FROM pm.org_level WHERE NOT is_deleted ORDER BY rank, code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Level{}
	for rows.Next() {
		l, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r PostgresRepo) Get(ctx context.Context, code string, forUpdate bool) (Level, error) {
	q := `SELECT ` + columns + ` FROM pm.org_level WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return scan(r.DB.Q(ctx).QueryRow(ctx, q, code))
}

func (r PostgresRepo) CodeTaken(ctx context.Context, code string) (Taken, error) {
	var deleted *bool
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT is_deleted FROM pm.org_level WHERE lower(code) = lower($1)`, code).Scan(&deleted)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Free, nil
	case err != nil:
		return Free, err
	case *deleted:
		return TakenDeleted, nil
	default:
		return TakenLive, nil
	}
}

func (r PostgresRepo) RankHolder(ctx context.Context, rank int, exceptID string) (string, error) {
	var name string
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT name FROM pm.org_level WHERE rank = $1 AND NOT is_deleted AND id::text <> $2`, rank, exceptID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// unique maps a unique-index violation to the error of that index.
func unique(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		if pg.ConstraintName == "org_level_rank_live" {
			return ErrRankTaken
		}
		return ErrDuplicate
	}
	return err
}

func (r PostgresRepo) Insert(ctx context.Context, in Input, actorID string) (Level, error) {
	saved, err := scan(r.DB.Q(ctx).QueryRow(ctx,
		`INSERT INTO pm.org_level (code, name, description, is_active, valid_from, valid_to, created_by, rank)
		 VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8) RETURNING `+columns,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actorID, in.Rank))
	return saved, unique(err)
}

func (r PostgresRepo) Update(ctx context.Context, id string, in Input, actorID string) (Level, error) {
	saved, err := scan(r.DB.Q(ctx).QueryRow(ctx,
		`UPDATE pm.org_level
		 SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		     rank = $7, updated_at = now(), updated_by = $8, version = version + 1
		 WHERE id = $1 AND NOT is_deleted RETURNING `+columns,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Rank, actorID))
	return saved, unique(err)
}

func (r PostgresRepo) UnitsUsing(ctx context.Context, id string) (int, error) {
	var n int
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM pm.org_unit WHERE org_level_id = $1 AND NOT is_deleted`, id).Scan(&n)
	return n, err
}

func (r PostgresRepo) UsedBy(ctx context.Context, id string) ([]string, error) {
	return refs.UsedBy(ctx, r.DB, "org_level", id)
}

func (r PostgresRepo) SoftDelete(ctx context.Context, id, actorID string) error {
	tag, err := r.DB.Q(ctx).Exec(ctx,
		`UPDATE pm.org_level
		 SET is_deleted = true, is_active = false, deleted_at = now(), deleted_by = $2,
		     updated_at = now(), updated_by = $2, version = version + 1
		 WHERE id = $1 AND NOT is_deleted`, id, actorID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
