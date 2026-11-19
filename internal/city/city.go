// Package city is the master "Kota" (Geografi & Lokasi, menu pm.city): cities and regencies of a country, with the
// province and an optional official region code. One live city per country, name and province.
package city

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type City struct {
	record.Base
	CountryID   string  `json:"countryId"`
	CountryCode string  `json:"countryCode"`
	Province    *string `json:"province"`
	Kind        string  `json:"kind"` // city | regency | other
	AreaCode    *string `json:"areaCode"`
}

type Input struct {
	record.Input
	CountryCode string
	CountryID   string
	Province    *string
	Kind        string
	AreaCode    *string
}

var area = regexp.MustCompile(`^[0-9A-Za-z.]{1,10}$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.CountryCode = record.RequiredText(v, "countryCode", "Negara", record.MaxCode, &p)
	in.Province = record.OptionalText(v, "province", "Provinsi", 100, &p)
	in.Kind = record.ChoiceOr(v, "kind", "Jenis", []string{"city", "regency", "other"}, "city", &p)
	if in.AreaCode = record.OptionalText(v, "areaCode", "Kode wilayah", 10, &p); in.AreaCode != nil && !p.Has("areaCode") && !area.MatchString(*in.AreaCode) {
		p.Add("areaCode", "Kode wilayah berisi angka, huruf atau titik, mis. 31.71.")
	}
	return in, p
}

func values(x City) record.Values {
	v := record.ValuesOf(x.Base)
	v["countryCode"], v["kind"], v["province"], v["areaCode"] = x.CountryCode, x.Kind, nil, nil
	if x.Province != nil {
		v["province"] = *x.Province
	}
	if x.AreaCode != nil {
		v["areaCode"] = *x.AreaCode
	}
	return v
}

func New(db *database.DB) *master.Service[City, Input] {
	return &master.Service[City, Input]{
		Spec: master.Spec[City, Input]{
			Menu: "pm.city", Table: "city", Noun: "Kota",
			Base: func(x *City) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *City, p record.Problems) (record.Problems, error) {
				self := ""
				var kept *string
				if stored != nil {
					self, kept = stored.ID, &stored.CountryID
				}
				var code *string
				if in.CountryCode != "" {
					code = &in.CountryCode
				}
				c, err := refs.Pick(ctx, db, "country", "countryCode", "Negara", code, kept, &p)
				if err != nil || c == nil {
					return p, err
				}
				in.CountryID = c.ID
				if in.Name == "" || p.Has("name") {
					return p, nil
				}
				var other string
				err = db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.city WHERE country_id = $1 AND lower(name) = lower($2)
					AND lower(coalesce(province, '')) = lower(coalesce($3, '')) AND NOT is_deleted AND id::text <> $4`, c.ID, in.Name, in.Province, self).Scan(&other)
				switch {
				case err == nil:
					p.Add("name", fmt.Sprintf("Kota %s di %s sudah ada (%s).", in.Name, c.Name, other))
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

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.country_id, c.code, x.province, x.kind, x.area_code FROM pm.city x JOIN pm.country c ON c.id = x.country_id`

func scan(row pgx.Row) (City, error) {
	var x City
	return x, x.Scan(row, &x.CountryID, &x.CountryCode, &x.Province, &x.Kind, &x.AreaCode)
}

func (r repo) List(ctx context.Context) ([]City, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY lower(c.name), lower(coalesce(x.province, '')), lower(x.name)`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (City, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (City, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (City, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.city (code, name, description, is_active, valid_from, valid_to, created_by, country_id, province, kind, area_code)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.CountryID, in.Province, in.Kind, in.AreaCode).Scan(&id); err != nil {
		return City{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (City, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.city SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		country_id = $7, province = $8, kind = $9, area_code = $10, updated_at = now(), updated_by = $11, version = version + 1
		WHERE id = $1 AND NOT is_deleted`, id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.CountryID, in.Province, in.Kind, in.AreaCode, actor); err != nil {
		return City{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "city", id, actor)
}
