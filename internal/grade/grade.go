// Package grade is the Portal Master master "Grade Jabatan" (menu pm.grade): job classes ordered by seniority.
// It is the smallest master on the shared shell (internal/platform/master): read it first.
package grade

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
)

// Grade is the standard record followed by the fields of the Grade Jabatan page.
type Grade struct {
	record.Base
	Rank int `json:"rank"` // higher = more senior; one live grade per rank
}

type Input struct {
	record.Input
	Rank int
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Rank = record.RequiredInt(v, "rank", "Urutan", 1, &p)
	return in, p
}

func values(g Grade) record.Values {
	v := record.ValuesOf(g.Base)
	v["rank"] = g.Rank
	return v
}

// New wires the master on db.
func New(db *database.DB) *master.Service[Grade, Input] {
	return &master.Service[Grade, Input]{
		Spec: master.Spec[Grade, Input]{
			Menu: "pm.grade", Table: "grade", Noun: "Grade",
			Base:    func(g *Grade) *record.Base { return &g.Base },
			Values:  values,
			Parse:   Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Grade, p record.Problems) (record.Problems, error) {
				if p.Has("rank") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				var holder string
				err := db.Q(ctx).QueryRow(ctx, `SELECT coalesce(max(name), '') FROM pm.grade WHERE rank = $1 AND NOT is_deleted AND id::text <> $2`, in.Rank, self).Scan(&holder)
				if holder != "" {
					p.Add("rank", fmt.Sprintf("Urutan %d sudah dipakai grade %s.", in.Rank, holder))
				}
				return p, err
			},
		},
		Repo:   repo{db},
		DB:     db,
		Codes:  codeprefix.Postgres{DB: db},
		Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var selectGrade = `SELECT ` + record.Columns + `, rank FROM pm.grade`

func scan(row pgx.Row) (Grade, error) {
	var g Grade
	return g, g.Scan(row, &g.Rank)
}

func (r repo) List(ctx context.Context) ([]Grade, error) {
	return master.Rows(ctx, r.db, scan, selectGrade+` WHERE NOT is_deleted ORDER BY rank DESC`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Grade, error) {
	q := selectGrade + ` WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Grade, error) {
	return master.One(ctx, r.db, scan, `INSERT INTO pm.grade (code, name, description, is_active, valid_from, valid_to, created_by, rank)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8) RETURNING `+record.Columns+`, rank`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Rank)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Grade, error) {
	return master.One(ctx, r.db, scan, `UPDATE pm.grade SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, rank = $7, updated_at = now(), updated_by = $8, version = version + 1
		WHERE id = $1 AND NOT is_deleted RETURNING `+record.Columns+`, rank`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Rank, actor)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "grade", id, actor)
}
