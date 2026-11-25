// Package roomamenity is the master "Fasilitas Ruang" (Meeting Room, menu pm.roomAmenity): what a room offers —
// projector, screen, video conference, whiteboard, … — shown on the schedule and used to filter rooms. The icon is
// one of a fixed set the pages can draw.
package roomamenity

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

// Icons the pages can draw, in form order.
var Icons = []string{"projector", "screen", "video", "audio", "whiteboard", "wifi", "phone", "ac", "pantry", "accessible", "other"}

type Amenity struct {
	record.Base
	Icon string `json:"icon"`
}

type Input struct {
	record.Input
	Icon string
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Icon = record.ChoiceOr(v, "icon", "Ikon", Icons, "other", &p)
	return in, p
}

func values(x Amenity) record.Values {
	v := record.ValuesOf(x.Base)
	v["icon"] = x.Icon
	return v
}

func New(db *database.DB) *master.Service[Amenity, Input] {
	return &master.Service[Amenity, Input]{
		Spec: master.Spec[Amenity, Input]{
			Menu: "pm.roomAmenity", Table: "room_amenity", Noun: "Fasilitas", Import: importSpec(db),
			Base: func(x *Amenity) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Amenity, p record.Problems) (record.Problems, error) {
				if in.Name == "" || p.Has("name") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				other, err := refs.Duplicate(ctx, db, "room_amenity", "name", in.Name, self, true)
				if other != "" {
					p.Add("name", fmt.Sprintf("Fasilitas %s sudah ada.", other))
				}
				return p, err
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var cols = record.Columns + `, icon`

func scan(row pgx.Row) (Amenity, error) {
	var x Amenity
	return x, x.Scan(row, &x.Icon)
}

func (r repo) List(ctx context.Context) ([]Amenity, error) {
	return master.Rows(ctx, r.db, scan, `SELECT `+cols+` FROM pm.room_amenity WHERE NOT is_deleted ORDER BY lower(name)`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Amenity, error) {
	q := `SELECT ` + cols + ` FROM pm.room_amenity WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Amenity, error) {
	return master.One(ctx, r.db, scan, `INSERT INTO pm.room_amenity (code, name, description, is_active, valid_from, valid_to, created_by, icon)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8) RETURNING `+cols,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Icon)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Amenity, error) {
	return master.One(ctx, r.db, scan, `UPDATE pm.room_amenity SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		icon = $7, updated_at = now(), updated_by = $8, version = version + 1 WHERE id = $1 AND NOT is_deleted RETURNING `+cols,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Icon, actor)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "room_amenity", id, actor)
}
