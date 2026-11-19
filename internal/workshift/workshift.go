// Package workshift is the master "Shift Kerja" (Employee Management, menu pm.workShift): working hours per shift
// for attendance and overtime. Times are the wall clock of the work place (HH:MM), not instants.
package workshift

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
)

type Shift struct {
	record.Base
	Start string `json:"start"` // HH:MM
	End   string `json:"end"`   // HH:MM; not after Start = the next day
	Night bool   `json:"night"`
}

type Input struct {
	record.Input
	Start, End string
	Night      bool
}

var clock = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	for _, f := range []struct {
		key, label string
		to         *string
	}{{"start", "Jam masuk", &in.Start}, {"end", "Jam pulang", &in.End}} {
		*f.to = record.RequiredText(v, f.key, f.label, 5, &p)
		if *f.to != "" && !p.Has(f.key) && !clock.MatchString(*f.to) {
			p.Add(f.key, fmt.Sprintf("%s harus jam 24 jam HH:MM, mis. 07:00.", f.label))
		}
	}
	in.Night = record.Bool(v, "night", "Shift malam", false, &p)
	if !p.Has("start") && !p.Has("end") && in.End <= in.Start && !in.Night {
		p.Add("night", "Shift yang berakhir di hari berikutnya harus ditandai shift malam.")
	}
	return in, p
}

func values(x Shift) record.Values {
	v := record.ValuesOf(x.Base)
	v["start"], v["end"], v["night"] = x.Start, x.End, x.Night
	return v
}

func New(db *database.DB) *master.Service[Shift, Input] {
	return &master.Service[Shift, Input]{
		Spec: master.Spec[Shift, Input]{
			Menu: "pm.workShift", Table: "work_shift", Noun: "Shift",
			Base: func(x *Shift) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var cols = record.Columns + `, to_char(start_time, 'HH24:MI'), to_char(end_time, 'HH24:MI'), is_night`

func scan(row pgx.Row) (Shift, error) {
	var x Shift
	return x, x.Scan(row, &x.Start, &x.End, &x.Night)
}

func (r repo) List(ctx context.Context) ([]Shift, error) {
	return master.Rows(ctx, r.db, scan, `SELECT `+cols+` FROM pm.work_shift WHERE NOT is_deleted ORDER BY start_time, code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Shift, error) {
	q := `SELECT ` + cols + ` FROM pm.work_shift WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Shift, error) {
	return master.One(ctx, r.db, scan, `INSERT INTO pm.work_shift (code, name, description, is_active, valid_from, valid_to, created_by,
		start_time, end_time, is_night) VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8::time, $9::time, $10) RETURNING `+cols,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Start, in.End, in.Night)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Shift, error) {
	return master.One(ctx, r.db, scan, `UPDATE pm.work_shift SET name = $2, description = $3, is_active = $4, valid_from = $5::date,
		valid_to = $6::date, start_time = $7::time, end_time = $8::time, is_night = $9, updated_at = now(), updated_by = $10, version = version + 1
		WHERE id = $1 AND NOT is_deleted RETURNING `+cols, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Start, in.End, in.Night, actor)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "work_shift", id, actor)
}
