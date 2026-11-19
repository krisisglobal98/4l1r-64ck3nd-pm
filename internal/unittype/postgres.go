package unittype

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// PostgresRepo stores unit types in pm.unit_type. Every time comes from the database's now().
type PostgresRepo struct{ DB *database.DB }

// selectType reads a unit type with the code of its head job title.
var selectType = `SELECT ` + record.ColumnsAs("t") + `, t.head_jabatan_id, j.code
	FROM pm.unit_type t LEFT JOIN pm.jabatan j ON j.id = t.head_jabatan_id`

func scan(row pgx.Row) (UnitType, error) {
	var u UnitType
	err := u.Scan(row, &u.HeadJabatanID, &u.HeadJabatanCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r PostgresRepo) List(ctx context.Context) ([]UnitType, error) {
	rows, err := r.DB.Q(ctx).Query(ctx, selectType+` WHERE NOT t.is_deleted ORDER BY t.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UnitType{}
	for rows.Next() {
		u, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r PostgresRepo) Get(ctx context.Context, code string, forUpdate bool) (UnitType, error) {
	q := selectType + ` WHERE lower(t.code) = lower($1) AND NOT t.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF t`
	}
	return scan(r.DB.Q(ctx).QueryRow(ctx, q, code))
}

func (r PostgresRepo) CodeTaken(ctx context.Context, code string) (Taken, error) {
	var deleted *bool
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT is_deleted FROM pm.unit_type WHERE lower(code) = lower($1)`, code).Scan(&deleted)
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

func (r PostgresRepo) Insert(ctx context.Context, in Input, actorID string) (UnitType, error) {
	var id string
	err := r.DB.Q(ctx).QueryRow(ctx,
		`INSERT INTO pm.unit_type (code, name, description, is_active, valid_from, valid_to, created_by, head_jabatan_id)
		 VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actorID, in.HeadJabatanID).Scan(&id)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return UnitType{}, ErrDuplicate
	}
	if err != nil {
		return UnitType{}, err
	}
	return scan(r.DB.Q(ctx).QueryRow(ctx, selectType+` WHERE t.id = $1`, id))
}

func (r PostgresRepo) Update(ctx context.Context, id string, in Input, actorID string) (UnitType, error) {
	tag, err := r.DB.Q(ctx).Exec(ctx,
		`UPDATE pm.unit_type
		 SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		     head_jabatan_id = $8, updated_at = now(), updated_by = $7, version = version + 1
		 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actorID, in.HeadJabatanID)
	if err != nil {
		return UnitType{}, err
	}
	if tag.RowsAffected() == 0 {
		return UnitType{}, ErrNotFound
	}
	return scan(r.DB.Q(ctx).QueryRow(ctx, selectType+` WHERE t.id = $1`, id))
}

func (r PostgresRepo) UsedBy(ctx context.Context, id string) ([]string, error) {
	return refs.UsedBy(ctx, r.DB, "unit_type", id)
}

func (r PostgresRepo) Jabatan(ctx context.Context, code string) (refs.Ref, error) {
	return refs.Lookup(ctx, r.DB, "jabatan", code)
}

func (r PostgresRepo) SoftDelete(ctx context.Context, id, actorID string) error {
	tag, err := r.DB.Q(ctx).Exec(ctx,
		`UPDATE pm.unit_type
		 SET is_deleted = true, is_active = false, deleted_at = now(), deleted_by = $2,
		     updated_at = now(), updated_by = $2, version = version + 1
		 WHERE id = $1 AND NOT is_deleted`, id, actorID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
