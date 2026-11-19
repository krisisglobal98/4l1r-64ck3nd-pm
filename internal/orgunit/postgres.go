package orgunit

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// PostgresRepo stores units in pm.org_unit. Every time comes from the database's now().
type PostgresRepo struct{ DB *database.DB }

// selectUnit reads a unit with the codes of its relations.
var selectUnit = `SELECT ` + record.ColumnsAs("u") + `, u.org_level_id, l.code, u.parent_id, p.code, u.unit_type_id, t.code, u.location
	FROM pm.org_unit u
	JOIN pm.org_level l ON l.id = u.org_level_id
	LEFT JOIN pm.org_unit p ON p.id = u.parent_id
	LEFT JOIN pm.unit_type t ON t.id = u.unit_type_id`

func scan(row pgx.Row) (Unit, error) {
	var u Unit
	err := u.Scan(row, &u.LevelID, &u.LevelCode, &u.ParentID, &u.ParentCode, &u.UnitTypeID, &u.UnitTypeCode, &u.Location)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r PostgresRepo) List(ctx context.Context) ([]Unit, error) {
	rows, err := r.DB.Q(ctx).Query(ctx, selectUnit+` WHERE NOT u.is_deleted ORDER BY u.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Unit{}
	for rows.Next() {
		u, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (r PostgresRepo) Get(ctx context.Context, code string, forUpdate bool) (Unit, error) {
	q := selectUnit + ` WHERE lower(u.code) = lower($1) AND NOT u.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF u`
	}
	return scan(r.DB.Q(ctx).QueryRow(ctx, q, code))
}

func (r PostgresRepo) byID(ctx context.Context, id string) (Unit, error) {
	return scan(r.DB.Q(ctx).QueryRow(ctx, selectUnit+` WHERE u.id = $1`, id))
}

func (r PostgresRepo) CodeTaken(ctx context.Context, code string) (Taken, error) {
	var deleted *bool
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT is_deleted FROM pm.org_unit WHERE lower(code) = lower($1)`, code).Scan(&deleted)
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

func (r PostgresRepo) Insert(ctx context.Context, in Input, refs Refs, actorID string) (Unit, error) {
	var id string
	err := r.DB.Q(ctx).QueryRow(ctx,
		`INSERT INTO pm.org_unit (code, name, description, is_active, valid_from, valid_to, created_by, org_level_id, parent_id, unit_type_id, location)
		 VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actorID, refs.LevelID, refs.ParentID, refs.UnitTypeID, in.Location).Scan(&id)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return Unit{}, ErrDuplicate
	}
	if err != nil {
		return Unit{}, err
	}
	return r.byID(ctx, id)
}

func (r PostgresRepo) Update(ctx context.Context, id string, in Input, refs Refs, actorID string) (Unit, error) {
	tag, err := r.DB.Q(ctx).Exec(ctx,
		`UPDATE pm.org_unit
		 SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		     org_level_id = $7, parent_id = $8, unit_type_id = $9, location = $10,
		     updated_at = now(), updated_by = $11, version = version + 1
		 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, refs.LevelID, refs.ParentID, refs.UnitTypeID, in.Location, actorID)
	if err != nil {
		return Unit{}, err
	}
	if tag.RowsAffected() == 0 {
		return Unit{}, ErrNotFound
	}
	return r.byID(ctx, id)
}

func (r PostgresRepo) SoftDelete(ctx context.Context, id, actorID string) error {
	tag, err := r.DB.Q(ctx).Exec(ctx,
		`UPDATE pm.org_unit
		 SET is_deleted = true, is_active = false, deleted_at = now(), deleted_by = $2,
		     updated_at = now(), updated_by = $2, version = version + 1
		 WHERE id = $1 AND NOT is_deleted`, id, actorID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (r PostgresRepo) Level(ctx context.Context, code string) (LevelRef, error) {
	var l LevelRef
	err := r.DB.Q(ctx).QueryRow(ctx,
		`SELECT id, code, name, rank, is_active FROM pm.org_level WHERE lower(code) = lower($1) AND NOT is_deleted FOR SHARE`, code).
		Scan(&l.ID, &l.Code, &l.Name, &l.Rank, &l.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

func (r PostgresRepo) LevelAbove(ctx context.Context, rank int) (LevelRef, bool, error) {
	var l LevelRef
	err := r.DB.Q(ctx).QueryRow(ctx,
		`SELECT id, code, name, rank, is_active FROM pm.org_level WHERE rank < $1 AND NOT is_deleted ORDER BY rank DESC LIMIT 1`, rank).
		Scan(&l.ID, &l.Code, &l.Name, &l.Rank, &l.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, false, nil
	}
	return l, err == nil, err
}

func (r PostgresRepo) Unit(ctx context.Context, code string) (UnitRef, error) {
	var u UnitRef
	err := r.DB.Q(ctx).QueryRow(ctx,
		`SELECT id, code, name, org_level_id, is_active FROM pm.org_unit WHERE lower(code) = lower($1) AND NOT is_deleted FOR SHARE`, code).
		Scan(&u.ID, &u.Code, &u.Name, &u.LevelID, &u.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (r PostgresRepo) UnitType(ctx context.Context, code string) (TypeRef, error) {
	var t TypeRef
	err := r.DB.Q(ctx).QueryRow(ctx,
		`SELECT id, code, name, is_active FROM pm.unit_type WHERE lower(code) = lower($1) AND NOT is_deleted FOR SHARE`, code).
		Scan(&t.ID, &t.Code, &t.Name, &t.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (r PostgresRepo) Children(ctx context.Context, id string) (int, error) {
	var n int
	err := r.DB.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM pm.org_unit WHERE parent_id = $1 AND NOT is_deleted`, id).Scan(&n)
	return n, err
}

func (r PostgresRepo) UsedBy(ctx context.Context, id string) ([]string, error) {
	return refs.UsedBy(ctx, r.DB, "org_unit", id)
}

func (r PostgresRepo) IsUnder(ctx context.Context, id, ancestorID string) (bool, error) {
	var under bool
	err := r.DB.Q(ctx).QueryRow(ctx,
		`WITH RECURSIVE up AS (
		     SELECT id, parent_id, 1 AS depth FROM pm.org_unit WHERE id = $1
		     UNION ALL
		     SELECT u.id, u.parent_id, up.depth + 1 FROM pm.org_unit u JOIN up ON u.id = up.parent_id WHERE up.depth < 64
		 )
		 SELECT EXISTS (SELECT 1 FROM up WHERE id = $2)`, id, ancestorID).Scan(&under)
	return under, err
}
