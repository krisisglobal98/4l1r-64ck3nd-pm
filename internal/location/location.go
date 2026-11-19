// Package location is the master "Pelabuhan & Lokasi" (Geografi & Lokasi, menu pm.port): the places goods are loaded,
// discharged, transhipped or cleared: seaports, river ports, airports, dry ports (ICD), rail terminals and border
// crossings. A shipment names its port of loading, of discharge and its place of delivery from here; the role is the
// shipment's, the place is one record.
//
// Rules: the UN/LOCODE starts with the country's ISO code and is unique; an IATA code only for an airport, unique;
// the city lies in the same country; coordinates both or neither.
package location

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

var Kinds = []string{"seaport", "river_port", "airport", "dry_port", "rail_terminal", "border", "other"}

type Location struct {
	record.Base
	Unlocode      *string  `json:"unlocode"`
	CountryID     string   `json:"countryId"`
	CountryCode   string   `json:"countryCode"`
	CityID        *string  `json:"cityId"`
	CityCode      *string  `json:"cityCode"`
	Kind          string   `json:"kind"`
	IataCode      *string  `json:"iataCode"`
	CustomsOffice *string  `json:"customsOffice"`
	Latitude      *float64 `json:"latitude"`
	Longitude     *float64 `json:"longitude"`
}

type Input struct {
	record.Input
	Unlocode, CityCode, IataCode, CustomsOffice *string
	CountryCode, Kind                           string
	Latitude, Longitude                         *float64
	CountryID                                   string
	CityID                                      *string
}

var (
	unlocode = regexp.MustCompile(`^[A-Z]{2}[A-Z2-9]{3}$`)
	iata     = regexp.MustCompile(`^[A-Z]{3}$`)
	customs  = regexp.MustCompile(`^[0-9]{6}$`)
)

func upper(s *string) *string {
	if s == nil {
		return nil
	}
	u := strings.ToUpper(strings.ReplaceAll(*s, " ", ""))
	return &u
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	if in.Unlocode = upper(record.OptionalText(v, "unlocode", "UN/LOCODE", 6, &p)); in.Unlocode != nil && !unlocode.MatchString(*in.Unlocode) {
		p.Add("unlocode", "UN/LOCODE berisi 5 karakter: kode negara 2 huruf lalu 3 huruf/angka, mis. IDTPP.")
	}
	in.CountryCode = record.RequiredText(v, "countryCode", "Negara", record.MaxCode, &p)
	in.CityCode = record.OptionalText(v, "cityCode", "Kota", record.MaxCode, &p)
	in.Kind = record.Choice(v, "kind", "Jenis lokasi", Kinds, &p)
	if in.IataCode = upper(record.OptionalText(v, "iataCode", "Kode IATA", 3, &p)); in.IataCode != nil && !p.Has("iataCode") {
		switch {
		case !iata.MatchString(*in.IataCode):
			p.Add("iataCode", "Kode IATA berisi 3 huruf, mis. CGK.")
		case in.Kind != "airport" && !p.Has("kind"):
			p.Add("iataCode", "Kode IATA hanya untuk bandara.")
		}
	}
	if in.CustomsOffice = record.OptionalText(v, "customsOffice", "Kode kantor pabean", 6, &p); in.CustomsOffice != nil && !p.Has("customsOffice") && !customs.MatchString(*in.CustomsOffice) {
		p.Add("customsOffice", "Kode kantor pabean berisi 6 angka, mis. 040300.")
	}
	in.Latitude = record.Decimal(v, "latitude", "Lintang", -90, 90, 6, false, &p)
	in.Longitude = record.Decimal(v, "longitude", "Bujur", -180, 180, 6, false, &p)
	if (in.Latitude == nil) != (in.Longitude == nil) && !p.Has("latitude") && !p.Has("longitude") {
		p.Add("longitude", "Isi lintang dan bujur sekaligus, atau kosongkan keduanya.")
	}
	return in, p
}

func values(x Location) record.Values {
	v := record.ValuesOf(x.Base)
	v["unlocode"], v["countryCode"], v["cityCode"], v["kind"] = record.Opt(x.Unlocode), x.CountryCode, record.Opt(x.CityCode), x.Kind
	v["iataCode"], v["customsOffice"], v["latitude"], v["longitude"] = record.Opt(x.IataCode), record.Opt(x.CustomsOffice), record.Opt(x.Latitude), record.Opt(x.Longitude)
	return v
}

func New(db *database.DB) *master.Service[Location, Input] {
	return &master.Service[Location, Input]{
		Spec: master.Spec[Location, Input]{
			Menu: "pm.port", Table: "location", Noun: "Lokasi",
			Base: func(x *Location) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Location, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func check(ctx context.Context, db *database.DB, in *Input, stored *Location, p record.Problems) (record.Problems, error) {
	self := ""
	var keptCountry, keptCity *string
	if stored != nil {
		self, keptCountry, keptCity = stored.ID, &stored.CountryID, stored.CityID
	}
	for _, u := range []struct {
		field, column string
		value         *string
		what          string
	}{{"unlocode", "unlocode", in.Unlocode, "UN/LOCODE"}, {"iataCode", "iata_code", in.IataCode, "Kode IATA"}} {
		if u.value == nil || p.Has(u.field) {
			continue
		}
		other, err := refs.Duplicate(ctx, db, "location", u.column, *u.value, self, false)
		if err != nil {
			return p, err
		}
		if other != "" {
			p.Add(u.field, fmt.Sprintf("%s %s sudah dipakai lokasi %s.", u.what, *u.value, other))
		}
	}
	var code *string
	if in.CountryCode != "" {
		code = &in.CountryCode
	}
	c, err := refs.Pick(ctx, db, "country", "countryCode", "Negara", code, keptCountry, &p)
	if err != nil {
		return p, err
	}
	city, err := refs.Pick(ctx, db, "city", "cityCode", "Kota", in.CityCode, keptCity, &p)
	if err != nil || c == nil {
		return p, err
	}
	in.CountryID = c.ID
	var iso2 string
	if err := db.Q(ctx).QueryRow(ctx, `SELECT iso2 FROM pm.country WHERE id = $1`, c.ID).Scan(&iso2); err != nil {
		return p, err
	}
	if in.Unlocode != nil && !p.Has("unlocode") && !strings.HasPrefix(*in.Unlocode, iso2) {
		p.Add("unlocode", fmt.Sprintf("UN/LOCODE lokasi di %s diawali %s.", c.Name, iso2))
	}
	if city != nil {
		var cityCountry string
		if err := db.Q(ctx).QueryRow(ctx, `SELECT country_id FROM pm.city WHERE id = $1`, city.ID).Scan(&cityCountry); err != nil {
			return p, err
		}
		if cityCountry != c.ID {
			p.Add("cityCode", fmt.Sprintf("Kota %s tidak berada di %s.", city.Name, c.Name))
		} else {
			in.CityID = &city.ID
		}
	}
	return p, nil
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.unlocode, x.country_id, c.code, x.city_id, ci.code, x.kind, x.iata_code, x.customs_office,
	x.latitude::float8, x.longitude::float8 FROM pm.location x JOIN pm.country c ON c.id = x.country_id LEFT JOIN pm.city ci ON ci.id = x.city_id`

func scan(row pgx.Row) (Location, error) {
	var x Location
	return x, x.Scan(row, &x.Unlocode, &x.CountryID, &x.CountryCode, &x.CityID, &x.CityCode, &x.Kind, &x.IataCode, &x.CustomsOffice, &x.Latitude, &x.Longitude)
}

func (r repo) List(ctx context.Context) ([]Location, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.code`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Location, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Location, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Location, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.location (code, name, description, is_active, valid_from, valid_to, created_by,
		unlocode, country_id, city_id, kind, iata_code, customs_office, latitude, longitude)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor,
		in.Unlocode, in.CountryID, in.CityID, in.Kind, in.IataCode, in.CustomsOffice, in.Latitude, in.Longitude).Scan(&id); err != nil {
		return Location{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Location, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.location SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		unlocode = $7, country_id = $8, city_id = $9, kind = $10, iata_code = $11, customs_office = $12, latitude = $13, longitude = $14,
		updated_at = now(), updated_by = $15, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo,
		in.Unlocode, in.CountryID, in.CityID, in.Kind, in.IataCode, in.CustomsOffice, in.Latitude, in.Longitude, actor); err != nil {
		return Location{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "location", id, actor)
}
