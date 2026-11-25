// Package building is the master "Gedung & Kantor" (Geografi & Lokasi, menu pm.building): an office or building of
// the company in a city, where meeting rooms (and later assets, reception desks, …) are. Its time zone is the wall
// clock of the place: the opening hours of its rooms are read on that clock, while every instant stays UTC.
package building

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
	_ "time/tzdata" // zones also on hosts without a zoneinfo database

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type Building struct {
	record.Base
	CityID   string  `json:"cityId"`
	CityCode string  `json:"cityCode"`
	CityName string  `json:"cityName"` // read only, for lists and headers
	UnitID   *string `json:"unitId"`
	UnitCode *string `json:"unitCode"`
	Address  *string `json:"address"`
	TimeZone string  `json:"timeZone"`
	Floors   *int    `json:"floors"`
}

type Input struct {
	record.Input
	CityCode string
	CityID   string
	UnitCode *string
	UnitID   *string
	Address  *string
	TimeZone string
	Floors   *int
}

var zonePattern = regexp.MustCompile(`^[A-Za-z_]+(/[A-Za-z0-9_+-]+){0,2}$`)

// DefaultZone is the zone of a building whose form leaves it empty.
const DefaultZone = "Asia/Jakarta"

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.CityCode = record.RequiredText(v, "cityCode", "Kota", record.MaxCode, &p)
	in.UnitCode = record.OptionalText(v, "unitCode", "Unit organisasi", record.MaxCode, &p)
	in.Address = record.OptionalText(v, "address", "Alamat", 500, &p)
	in.TimeZone = DefaultZone
	if z := record.OptionalText(v, "timeZone", "Zona waktu", 40, &p); z != nil && !p.Has("timeZone") {
		in.TimeZone = *z
		if _, err := time.LoadLocation(*z); err != nil || !zonePattern.MatchString(*z) {
			p.Add("timeZone", fmt.Sprintf("Zona waktu \"%s\" tidak dikenal; pakai nama IANA, mis. Asia/Jakarta.", *z))
		}
	}
	if in.Floors = record.OptionalInt(v, "floors", "Jumlah lantai", 1, &p); in.Floors != nil && *in.Floors > 200 {
		p.Add("floors", "Jumlah lantai paling banyak 200.")
	}
	return in, p
}

func values(x Building) record.Values {
	v := record.ValuesOf(x.Base)
	v["cityCode"], v["unitCode"], v["address"], v["timeZone"], v["floors"] = x.CityCode, record.Opt(x.UnitCode), record.Opt(x.Address), x.TimeZone, record.Opt(x.Floors)
	return v
}

func New(db *database.DB) *master.Service[Building, Input] {
	return &master.Service[Building, Input]{
		Spec: master.Spec[Building, Input]{
			Menu: "pm.building", Table: "building", Noun: "Gedung", Import: importSpec(db),
			Base: func(x *Building) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Building, p record.Problems) (record.Problems, error) {
				self := ""
				var keptCity, keptUnit *string
				if stored != nil {
					self, keptCity, keptUnit = stored.ID, &stored.CityID, stored.UnitID
				}
				var code *string
				if in.CityCode != "" {
					code = &in.CityCode
				}
				c, err := refs.Pick(ctx, db, "city", "cityCode", "Kota", code, keptCity, &p)
				if err != nil {
					return p, err
				}
				u, err := refs.Pick(ctx, db, "org_unit", "unitCode", "Unit organisasi", in.UnitCode, keptUnit, &p)
				if err != nil {
					return p, err
				}
				if u != nil {
					in.UnitID = &u.ID
				}
				if c == nil {
					return p, nil
				}
				in.CityID = c.ID
				if in.Name == "" || p.Has("name") {
					return p, nil
				}
				var other string
				err = db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.building WHERE city_id = $1 AND lower(name) = lower($2) AND NOT is_deleted
					AND id::text <> $3`, c.ID, in.Name, self).Scan(&other)
				switch {
				case err == nil:
					p.Add("name", fmt.Sprintf("Gedung %s di %s sudah ada (%s).", in.Name, c.Name, other))
				case !errors.Is(err, pgx.ErrNoRows):
					return p, err
				}
				return p, nil
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.city_id, c.code, c.name, x.org_unit_id, u.code, x.address, x.time_zone, x.floors
	FROM pm.building x JOIN pm.city c ON c.id = x.city_id LEFT JOIN pm.org_unit u ON u.id = x.org_unit_id`

func scan(row pgx.Row) (Building, error) {
	var x Building
	var floors *int16
	err := x.Scan(row, &x.CityID, &x.CityCode, &x.CityName, &x.UnitID, &x.UnitCode, &x.Address, &x.TimeZone, &floors)
	if floors != nil {
		n := int(*floors)
		x.Floors = &n
	}
	return x, err
}

func (r repo) List(ctx context.Context) ([]Building, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY lower(c.name), lower(x.name)`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Building, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Building, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Building, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.building (code, name, description, is_active, valid_from, valid_to, created_by,
		city_id, org_unit_id, address, time_zone, floors) VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.CityID, in.UnitID, in.Address, in.TimeZone, in.Floors).Scan(&id); err != nil {
		return Building{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Building, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.building SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		city_id = $7, org_unit_id = $8, address = $9, time_zone = $10, floors = $11, updated_at = now(), updated_by = $12, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.CityID, in.UnitID, in.Address,
		in.TimeZone, in.Floors, actor); err != nil {
		return Building{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "building", id, actor)
}
