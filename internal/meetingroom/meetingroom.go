// Package meetingroom is the master "Ruang Rapat" (Meeting Room, menu pm.meetingRoom): a bookable room of a building
// (Gedung & Kantor) with its seats, floor, amenities (Fasilitas Ruang), opening hours on the building's clock, the
// longest booking, whether a booking waits for approval, and the employee who looks after it. Bookings live in the
// Meeting Room service, which reads rooms here.
package meetingroom

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// Amenity is a facility of the room as the schedule shows it.
type Amenity struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type Room struct {
	record.Base
	BuildingID         string    `json:"buildingId"`
	BuildingCode       string    `json:"buildingCode"`
	BuildingName       string    `json:"buildingName"` // read only: the schedule groups rooms by building
	CityName           string    `json:"cityName"`     // read only
	TimeZone           string    `json:"timeZone"`     // read only: the building's clock
	Floor              *string   `json:"floor"`
	Capacity           int       `json:"capacity"`
	OpenTime           string    `json:"openTime"`  // HH:MM on the building's clock
	CloseTime          string    `json:"closeTime"` // HH:MM, after OpenTime
	MaxDurationMinutes *int      `json:"maxDurationMinutes"`
	ApprovalMode       string    `json:"approvalMode"`  // none: first come, first booked; required: waits for Persetujuan Booking
	NeedsApproval      bool      `json:"needsApproval"` // read only: approvalMode == required (the Meeting Room service reads it)
	WeekendBookable    bool      `json:"weekendBookable"`
	PicEmployeeID      *string   `json:"picEmployeeId"`
	PicEmployeeCode    *string   `json:"picEmployeeCode"`
	PicEmployeeName    *string   `json:"picEmployeeName"` // read only
	AmenityCodes       []string  `json:"amenityCodes"`
	Amenities          []Amenity `json:"amenities"` // read only, in the order of AmenityCodes
	amenityIDs         []string
}

type Input struct {
	record.Input
	BuildingCode       string
	BuildingID         string
	Floor              *string
	Capacity           int
	OpenTime           string
	CloseTime          string
	MaxDurationMinutes *int
	NeedsApproval      bool
	WeekendBookable    bool
	PicEmployeeCode    *string
	PicEmployeeID      *string
	AmenityCodes       []string
	AmenityIDs         []string
}

// ApprovalModes are the choices of Persetujuan: without approval (first come, first booked) or with approval.
var ApprovalModes = []string{"none", "required"}

var clock = regexp.MustCompile(`^([01][0-9]|2[0-3]):(00|15|30|45)$`)

// minutes is HH:MM as minutes after midnight.
func minutes(hhmm string) int {
	h, _ := strconv.Atoi(hhmm[:2])
	m, _ := strconv.Atoi(hhmm[3:])
	return h*60 + m
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.BuildingCode = record.RequiredText(v, "buildingCode", "Gedung", record.MaxCode, &p)
	in.Floor = record.OptionalText(v, "floor", "Lantai", 20, &p)
	if in.Capacity = record.RequiredInt(v, "capacity", "Kapasitas", 1, &p); !p.Has("capacity") && in.Capacity > 1000 {
		p.Add("capacity", "Kapasitas paling banyak 1000 kursi.")
	}
	for _, f := range []struct {
		key, label, def string
		to              *string
	}{{"openTime", "Jam buka", "08:00", &in.OpenTime}, {"closeTime", "Jam tutup", "18:00", &in.CloseTime}} {
		*f.to = f.def
		if s := record.OptionalText(v, f.key, f.label, 5, &p); s != nil && !p.Has(f.key) {
			*f.to = *s
			if !clock.MatchString(*s) {
				p.Add(f.key, fmt.Sprintf("%s berformat HH:MM dengan menit 00, 15, 30 atau 45, mis. 08:00.", f.label))
			}
		}
	}
	if !p.Has("openTime") && !p.Has("closeTime") && minutes(in.CloseTime) <= minutes(in.OpenTime) {
		p.Add("closeTime", "Jam tutup harus setelah jam buka.")
	}
	if in.MaxDurationMinutes = record.OptionalInt(v, "maxDurationMinutes", "Durasi maksimal", 15, &p); in.MaxDurationMinutes != nil && !p.Has("maxDurationMinutes") {
		switch d := *in.MaxDurationMinutes; {
		case d > 1440:
			p.Add("maxDurationMinutes", "Durasi maksimal paling lama 1440 menit (24 jam).")
		case d%15 != 0:
			p.Add("maxDurationMinutes", "Durasi maksimal kelipatan 15 menit, mis. 120.")
		case !p.Has("closeTime") && !p.Has("openTime") && d > minutes(in.CloseTime)-minutes(in.OpenTime):
			p.Add("maxDurationMinutes", "Durasi maksimal melebihi jam buka ruang; kosongkan bila boleh sampai tutup.")
		}
	}
	// Persetujuan: "none" (default) books at once, whoever is first; "required" waits for Persetujuan Booking.
	// needsApproval (true/false) is still read when approvalMode is not sent.
	def := "none"
	if b, ok := v["needsApproval"].(bool); ok && b {
		def = "required"
	}
	in.NeedsApproval = record.ChoiceOr(v, "approvalMode", "Persetujuan", ApprovalModes, def, &p) == "required"
	in.WeekendBookable = record.Bool(v, "weekendBookable", "Bisa dipesan akhir pekan", false, &p)
	in.PicEmployeeCode = record.OptionalText(v, "picEmployeeCode", "Penanggung jawab", record.MaxCode, &p)
	in.AmenityCodes = record.Codes(v, "amenityCodes", "Fasilitas", &p)
	if len(in.AmenityCodes) > 30 {
		p.Add("amenityCodes", "Fasilitas paling banyak 30.")
	}
	return in, p
}

func values(x Room) record.Values {
	v := record.ValuesOf(x.Base)
	codes := []any{}
	for _, c := range x.AmenityCodes {
		codes = append(codes, c)
	}
	v["buildingCode"], v["floor"], v["capacity"], v["openTime"], v["closeTime"] = x.BuildingCode, record.Opt(x.Floor), x.Capacity, x.OpenTime, x.CloseTime
	v["approvalMode"] = x.ApprovalMode
	v["maxDurationMinutes"], v["needsApproval"], v["weekendBookable"], v["picEmployeeCode"], v["amenityCodes"] =
		record.Opt(x.MaxDurationMinutes), nil, x.WeekendBookable, record.Opt(x.PicEmployeeCode), codes
	return v
}

func New(db *database.DB) *master.Service[Room, Input] {
	return &master.Service[Room, Input]{
		Spec: master.Spec[Room, Input]{
			Menu: "pm.meetingRoom", Table: "meeting_room", Noun: "Ruang rapat", Import: importSpec(db),
			Base: func(x *Room) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Room, p record.Problems) (record.Problems, error) {
				return check(ctx, db, in, stored, p)
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

func check(ctx context.Context, db *database.DB, in *Input, stored *Room, p record.Problems) (record.Problems, error) {
	self := ""
	var keptBuilding, keptPic *string
	var keptAmenities []string
	if stored != nil {
		self, keptBuilding, keptPic, keptAmenities = stored.ID, &stored.BuildingID, stored.PicEmployeeID, stored.amenityIDs
	}
	var code *string
	if in.BuildingCode != "" {
		code = &in.BuildingCode
	}
	b, err := refs.Pick(ctx, db, "building", "buildingCode", "Gedung", code, keptBuilding, &p)
	if err != nil {
		return p, err
	}
	pic, err := refs.Pick(ctx, db, "employee", "picEmployeeCode", "Karyawan", in.PicEmployeeCode, keptPic, &p)
	if err != nil {
		return p, err
	}
	if pic != nil {
		in.PicEmployeeID = &pic.ID
	}
	for _, c := range in.AmenityCodes {
		var kept *string
		if r, err := refs.Lookup(ctx, db, "room_amenity", c); err == nil && slices.Contains(keptAmenities, r.ID) {
			kept = &r.ID
		}
		a, err := refs.Pick(ctx, db, "room_amenity", "amenityCodes", "Fasilitas", &c, kept, &p)
		if err != nil {
			return p, err
		}
		if a != nil {
			in.AmenityIDs = append(in.AmenityIDs, a.ID)
		}
	}
	if b == nil {
		return p, nil
	}
	in.BuildingID = b.ID
	if in.Floor != nil && !p.Has("floor") {
		var floors *int16
		if err := db.Q(ctx).QueryRow(ctx, `SELECT floors FROM pm.building WHERE id = $1`, b.ID).Scan(&floors); err != nil {
			return p, err
		}
		if n, err := strconv.Atoi(*in.Floor); err == nil && floors != nil && (n < 0 || n > int(*floors)) {
			p.Add("floor", fmt.Sprintf("Gedung %s hanya %d lantai.", b.Name, *floors))
		}
	}
	if in.Name == "" || p.Has("name") {
		return p, nil
	}
	var other string
	err = db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.meeting_room WHERE building_id = $1 AND lower(name) = lower($2) AND NOT is_deleted
		AND id::text <> $3`, b.ID, in.Name, self).Scan(&other)
	switch {
	case err == nil:
		p.Add("name", fmt.Sprintf("Ruang %s di %s sudah ada (%s).", in.Name, b.Name, other))
	case !errors.Is(err, pgx.ErrNoRows):
		return p, err
	}
	return p, nil
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.building_id, b.code, b.name, c.name, b.time_zone, x.floor, x.capacity,
	to_char(x.open_time, 'HH24:MI'), to_char(x.close_time, 'HH24:MI'), x.max_duration_minutes, x.needs_approval, x.weekend_bookable,
	x.pic_employee_id, e.code, e.name,
	coalesce(array_agg(a.code ORDER BY ma.seq) FILTER (WHERE a.id IS NOT NULL), '{}'),
	coalesce(array_agg(a.name ORDER BY ma.seq) FILTER (WHERE a.id IS NOT NULL), '{}'),
	coalesce(array_agg(a.icon ORDER BY ma.seq) FILTER (WHERE a.id IS NOT NULL), '{}'),
	coalesce(array_agg(a.id::text ORDER BY ma.seq) FILTER (WHERE a.id IS NOT NULL), '{}')
	FROM pm.meeting_room x JOIN pm.building b ON b.id = x.building_id JOIN pm.city c ON c.id = b.city_id
	LEFT JOIN pm.employee e ON e.id = x.pic_employee_id
	LEFT JOIN pm.meeting_room_amenity ma ON ma.meeting_room_id = x.id LEFT JOIN pm.room_amenity a ON a.id = ma.room_amenity_id`

const groupBy = ` GROUP BY x.id, b.code, b.name, c.name, b.time_zone, e.code, e.name`

func scan(row pgx.Row) (Room, error) {
	var x Room
	var capacity int16
	var names, icons []string
	if err := x.Scan(row, &x.BuildingID, &x.BuildingCode, &x.BuildingName, &x.CityName, &x.TimeZone, &x.Floor, &capacity,
		&x.OpenTime, &x.CloseTime, &x.MaxDurationMinutes, &x.NeedsApproval, &x.WeekendBookable,
		&x.PicEmployeeID, &x.PicEmployeeCode, &x.PicEmployeeName, &x.AmenityCodes, &names, &icons, &x.amenityIDs); err != nil {
		return x, err
	}
	x.Capacity = int(capacity)
	x.ApprovalMode = "none"
	if x.NeedsApproval {
		x.ApprovalMode = "required"
	}
	x.Amenities = make([]Amenity, len(x.AmenityCodes))
	for i, c := range x.AmenityCodes {
		x.Amenities[i] = Amenity{Code: c, Name: names[i], Icon: icons[i]}
	}
	return x, nil
}

func (r repo) List(ctx context.Context) ([]Room, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted`+groupBy+` ORDER BY lower(b.name), lower(x.name)`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Room, error) {
	if forUpdate { // lock the row first: FOR UPDATE does not go with GROUP BY
		var id string
		if err := r.db.Q(ctx).QueryRow(ctx, `SELECT id FROM pm.meeting_room WHERE lower(code) = lower($1) AND NOT is_deleted FOR UPDATE`, code).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Room{}, master.ErrNotFound
			}
			return Room{}, err
		}
	}
	return master.One(ctx, r.db, scan, sel+` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`+groupBy, code)
}

func (r repo) byID(ctx context.Context, id string) (Room, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`+groupBy, id)
}

func (r repo) saveAmenities(ctx context.Context, id string, ids []string) error {
	if _, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM pm.meeting_room_amenity WHERE meeting_room_id = $1`, id); err != nil {
		return err
	}
	for i, a := range ids {
		if _, err := r.db.Q(ctx).Exec(ctx, `INSERT INTO pm.meeting_room_amenity (meeting_room_id, room_amenity_id, seq) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, id, a, i+1); err != nil {
			return err
		}
	}
	return nil
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Room, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.meeting_room (code, name, description, is_active, valid_from, valid_to, created_by,
		building_id, floor, capacity, open_time, close_time, max_duration_minutes, needs_approval, weekend_bookable, pic_employee_id)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10, $11::time, $12::time, $13, $14, $15, $16) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.BuildingID, in.Floor, in.Capacity, in.OpenTime, in.CloseTime,
		in.MaxDurationMinutes, in.NeedsApproval, in.WeekendBookable, in.PicEmployeeID).Scan(&id); err != nil {
		return Room{}, err
	}
	if err := r.saveAmenities(ctx, id, in.AmenityIDs); err != nil {
		return Room{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Room, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.meeting_room SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		building_id = $7, floor = $8, capacity = $9, open_time = $10::time, close_time = $11::time, max_duration_minutes = $12, needs_approval = $13,
		weekend_bookable = $14, pic_employee_id = $15, updated_at = now(), updated_by = $16, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.BuildingID, in.Floor, in.Capacity, in.OpenTime, in.CloseTime,
		in.MaxDurationMinutes, in.NeedsApproval, in.WeekendBookable, in.PicEmployeeID, actor); err != nil {
		return Room{}, err
	}
	if err := r.saveAmenities(ctx, id, in.AmenityIDs); err != nil {
		return Room{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "meeting_room", id, actor)
}
