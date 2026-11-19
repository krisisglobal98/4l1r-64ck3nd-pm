// Package jabatan is the Portal Master master "Jabatan" (menu pm.jabatan): job titles, apart from where they sit.
package jabatan

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// Kinds: struktural heads a unit; fungsional is staff.
var Kinds = []string{"struktural", "fungsional"}

type Jabatan struct {
	record.Base
	GradeID   string `json:"gradeId"`
	GradeCode string `json:"gradeCode"`
	Kind      string `json:"kind"`
	LevelID   string `json:"levelId"`
	LevelCode string `json:"levelCode"`
}

type Input struct {
	record.Input
	GradeCode, Kind, LevelCode string
	GradeID, LevelID           string // resolved
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.GradeCode = record.RequiredText(v, "gradeCode", "Grade", record.MaxCode, &p)
	in.Kind = record.Choice(v, "kind", "Jenis", Kinds, &p)
	in.LevelCode = record.RequiredText(v, "levelCode", "Level", record.MaxCode, &p)
	return in, p
}

func values(j Jabatan) record.Values {
	v := record.ValuesOf(j.Base)
	v["gradeCode"], v["kind"], v["levelCode"] = j.GradeCode, j.Kind, j.LevelCode
	return v
}

func New(db *database.DB) *master.Service[Jabatan, Input] {
	return &master.Service[Jabatan, Input]{
		Spec: master.Spec[Jabatan, Input]{
			Menu: "pm.jabatan", Table: "jabatan", Noun: "Jabatan",
			Base:    func(j *Jabatan) *record.Base { return &j.Base },
			Values:  values,
			Parse:   Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Jabatan, p record.Problems) (record.Problems, error) {
				var keptGrade, keptLevel *string
				if stored != nil {
					keptGrade, keptLevel = &stored.GradeID, &stored.LevelID
				}
				g, err := refs.Pick(ctx, db, "grade", "gradeCode", "Grade", nonEmpty(in.GradeCode), keptGrade, &p)
				if err != nil {
					return p, err
				}
				l, err := refs.Pick(ctx, db, "org_level", "levelCode", "Level", nonEmpty(in.LevelCode), keptLevel, &p)
				if err != nil {
					return p, err
				}
				if g != nil {
					in.GradeID = g.ID
				}
				if l != nil {
					in.LevelID = l.ID
					// Positions put a job title in a unit of its level: a title in use keeps its level.
					if stored != nil && l.ID != stored.LevelID {
						var n int
						if err := db.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM pm.position WHERE jabatan_id = $1 AND NOT is_deleted`, stored.ID).Scan(&n); err != nil {
							return p, err
						}
						if n > 0 {
							p.Add("levelCode", fmt.Sprintf("Level tidak dapat diubah selama jabatan ini dipakai %d posisi.", n))
						}
					}
				}
				return p, nil
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type repo struct{ db *database.DB }

var selectJabatan = `SELECT ` + record.ColumnsAs("j") + `, j.grade_id, g.code, j.kind, j.org_level_id, l.code
	FROM pm.jabatan j JOIN pm.grade g ON g.id = j.grade_id JOIN pm.org_level l ON l.id = j.org_level_id`

func scan(row pgx.Row) (Jabatan, error) {
	var j Jabatan
	return j, j.Scan(row, &j.GradeID, &j.GradeCode, &j.Kind, &j.LevelID, &j.LevelCode)
}

func (r repo) List(ctx context.Context) ([]Jabatan, error) {
	return master.Rows(ctx, r.db, scan, selectJabatan+` WHERE NOT j.is_deleted ORDER BY l.rank, g.rank DESC, j.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Jabatan, error) {
	q := selectJabatan + ` WHERE lower(j.code) = lower($1) AND NOT j.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF j`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Jabatan, error) {
	return master.One(ctx, r.db, scan, selectJabatan+` WHERE j.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Jabatan, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.jabatan (code, name, description, is_active, valid_from, valid_to, created_by, grade_id, kind, org_level_id)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.GradeID, in.Kind, in.LevelID).Scan(&id); err != nil {
		return Jabatan{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Jabatan, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.jabatan SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, grade_id = $7, kind = $8, org_level_id = $9, updated_at = now(), updated_by = $10, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.GradeID, in.Kind, in.LevelID, actor); err != nil {
		return Jabatan{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "jabatan", id, actor)
}
