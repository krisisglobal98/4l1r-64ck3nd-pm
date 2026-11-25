//go:build e2e

// Meeting Room masters: Gedung & Kantor (/pm/masters/buildings), Fasilitas Ruang (/pm/masters/room-amenities) and
// Ruang Rapat (/pm/masters/meeting-rooms), positive, negative and stress.
package e2e

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	bldBase  = "/pm/masters/buildings"
	amnBase  = "/pm/masters/room-amenities"
	roomBase = "/pm/masters/meeting-rooms"
)

var mrSeq atomic.Int64

func testCity(t testing.TB) string {
	t.Helper()
	c := testCountry(t)
	return must(t, call(t, "POST", cityBase, map[string]any{"name": fmt.Sprintf("Kota Rapat %d", mrSeq.Add(1)), "countryCode": c["code"]}), 201).str("code")
}

func testBuilding(t testing.TB, extra map[string]any) resp {
	t.Helper()
	b := map[string]any{"name": fmt.Sprintf("Gedung %d", mrSeq.Add(1)), "cityCode": testCity(t)}
	for k, v := range extra {
		b[k] = v
	}
	return must(t, call(t, "POST", bldBase, b), 201)
}

func testAmenity(t testing.TB, icon string) string {
	t.Helper()
	return must(t, call(t, "POST", amnBase, map[string]any{"name": fmt.Sprintf("Fasilitas %d", mrSeq.Add(1)), "icon": icon}), 201).str("code")
}

func TestMeetingRoomPositive(t *testing.T) {
	t.Run("MR01 a building in a city with its zone; the default zone is Asia/Jakarta", func(t *testing.T) {
		b := testBuilding(t, map[string]any{"address": "Jl. Sudirman 1", "floors": 12, "timeZone": "Europe/Berlin"})
		if !genCodeOf("MGD").MatchString(b.str("code")) || b.str("timeZone") != "Europe/Berlin" || b.Obj["floors"] != 12.0 || b.str("cityName") == "" {
			t.Errorf("building %s", trunc(b.Raw))
		}
		d := testBuilding(t, nil)
		if d.str("timeZone") != "Asia/Jakarta" || d.Obj["floors"] != nil {
			t.Errorf("defaults %s", trunc(d.Raw))
		}
	})
	t.Run("MR02 a room with seats, hours, amenities in order, PIC; read back with building, zone and amenity names", func(t *testing.T) {
		b := testBuilding(t, map[string]any{"floors": 5, "timeZone": "Asia/Makassar"})
		a1, a2 := testAmenity(t, "projector"), testAmenity(t, "video")
		emp := newEmployee(t)
		r := must(t, call(t, "POST", roomBase, map[string]any{"name": "Dragonstone", "buildingCode": b.str("code"), "floor": "3", "capacity": 12,
			"openTime": "07:30", "closeTime": "20:00", "maxDurationMinutes": 180, "needsApproval": true, "picEmployeeCode": emp,
			"amenityCodes": []string{a2, a1}}), 201)
		if !genCodeOf("MRR").MatchString(r.str("code")) || r.Obj["capacity"] != 12.0 || r.str("openTime") != "07:30" || r.str("closeTime") != "20:00" ||
			r.Obj["needsApproval"] != true || r.Obj["weekendBookable"] != false || r.str("timeZone") != "Asia/Makassar" || r.str("buildingName") != b.str("name") ||
			r.str("picEmployeeCode") != emp || r.str("picEmployeeName") == "" {
			t.Fatalf("room %s", trunc(r.Raw))
		}
		codes, _ := r.Obj["amenityCodes"].([]any)
		ams, _ := r.Obj["amenities"].([]any)
		if len(codes) != 2 || codes[0] != a2 || len(ams) != 2 || ams[1].(map[string]any)["icon"] != "projector" {
			t.Errorf("amenities %s", trunc(r.Raw))
		}
		// Defaults: 08:00–18:00, no limit; a change keeps the amenities it does not send.
		d := must(t, call(t, "POST", roomBase, map[string]any{"name": "Skynet", "buildingCode": b.str("code"), "capacity": 5}), 201)
		if d.str("openTime") != "08:00" || d.str("closeTime") != "18:00" || d.Obj["maxDurationMinutes"] != nil || d.str("approvalMode") != "none" || d.Obj["needsApproval"] != false {
			t.Errorf("defaults %s", trunc(d.Raw))
		}
		u := must(t, call(t, "PUT", roomBase+"/"+r.str("code"), map[string]any{"capacity": 14, "version": r.Obj["version"]}), 200)
		if u.Obj["capacity"] != 14.0 || len(u.Obj["amenityCodes"].([]any)) != 2 || u.str("approvalMode") != "required" {
			t.Errorf("kept %s", trunc(u.Raw))
		}
		// Persetujuan as a choice: without approval books at once.
		n := must(t, call(t, "PUT", roomBase+"/"+r.str("code"), map[string]any{"approvalMode": "none"}), 200)
		if n.Obj["needsApproval"] != false || n.str("approvalMode") != "none" {
			t.Errorf("no approval %s", trunc(n.Raw))
		}
		if x := call(t, "PUT", roomBase+"/"+r.str("code"), map[string]any{"approvalMode": "kadang"}); !want(t, x, 422) || x.problem("approvalMode") == "" {
			t.Errorf("bad mode %s", trunc(x.Raw))
		}
		if l := must(t, call(t, "GET", roomBase, nil), 200); len(l.List) < 2 {
			t.Errorf("list %d", len(l.List))
		}
	})
	t.Run("MR03 in use: a building with rooms, an amenity of a room and the PIC cannot be deleted", func(t *testing.T) {
		b := testBuilding(t, nil)
		a := testAmenity(t, "whiteboard")
		emp := newEmployee(t)
		r := must(t, call(t, "POST", roomBase, map[string]any{"name": "Room 404", "buildingCode": b.str("code"), "capacity": 12, "amenityCodes": []string{a}, "picEmployeeCode": emp}), 201)
		for _, path := range []string{bldBase + "/" + b.str("code"), amnBase + "/" + a, emBase + "/" + emp} {
			if x := call(t, "DELETE", path, nil); !want(t, x, 409) || !strings.Contains(x.str("message"), "ruang rapat") {
				t.Errorf("%s: %s", path, trunc(x.Raw))
			}
		}
		must(t, call(t, "DELETE", roomBase+"/"+r.str("code"), nil), 204)
		must(t, call(t, "DELETE", bldBase+"/"+b.str("code"), nil), 204)
		must(t, call(t, "DELETE", amnBase+"/"+a, nil), 204)
	})
	t.Run("MR04 the standard number pattern of bookings is there", func(t *testing.T) {
		if x := must(t, call(t, "GET", "/pm/numbering/MR.BOOK?date=2026-11-24", nil), 200); x.str("pattern") != "BKR/{SEQ:4}/{MM}/{YY}" {
			t.Errorf("pattern %s", trunc(x.Raw))
		}
	})
}

func TestMeetingRoomNegative(t *testing.T) {
	b := testBuilding(t, map[string]any{"floors": 4})
	t.Run("MN01 building: unknown city and zone, too many floors, same name in the same city", func(t *testing.T) {
		for field, body := range map[string]map[string]any{
			"cityCode": {"name": "X", "cityCode": "TIDAKADA"},
			"timeZone": {"name": "X", "cityCode": b.str("cityCode"), "timeZone": "Mars/Olympus"},
			"floors":   {"name": "X", "cityCode": b.str("cityCode"), "floors": 201},
			"name":     {"name": strings.ToUpper(b.str("name")), "cityCode": b.str("cityCode")},
		} {
			if x := call(t, "POST", bldBase, body); !want(t, x, 422) || x.problem(field) == "" {
				t.Errorf("%s: %s", field, trunc(x.Raw))
			}
		}
	})
	t.Run("MN02 room: seats, hours, longest booking, floor above the building, unknown amenity, same name twice", func(t *testing.T) {
		ok := func(extra map[string]any) map[string]any {
			m := map[string]any{"name": fmt.Sprintf("Ruang %d", mrSeq.Add(1)), "buildingCode": b.str("code"), "capacity": 6}
			for k, v := range extra {
				m[k] = v
			}
			return m
		}
		for field, body := range map[string]map[string]any{
			"buildingCode":       {"name": "X", "capacity": 4},
			"capacity":           ok(map[string]any{"capacity": 0}),
			"openTime":           ok(map[string]any{"openTime": "08:10"}),
			"closeTime":          ok(map[string]any{"openTime": "10:00", "closeTime": "09:00"}),
			"maxDurationMinutes": ok(map[string]any{"openTime": "08:00", "closeTime": "10:00", "maxDurationMinutes": 180}),
			"floor":              ok(map[string]any{"floor": "9"}),
			"amenityCodes":       ok(map[string]any{"amenityCodes": []string{"MFR999"}}),
		} {
			if x := call(t, "POST", roomBase, body); !want(t, x, 422) || x.problem(field) == "" {
				t.Errorf("%s: %s", field, trunc(x.Raw))
			}
		}
		if x := call(t, "POST", roomBase, ok(map[string]any{"maxDurationMinutes": 50})); !want(t, x, 422) || x.problem("maxDurationMinutes") == "" {
			t.Errorf("not a quarter: %s", trunc(x.Raw))
		}
		must(t, call(t, "POST", roomBase, ok(map[string]any{"name": "Bazinga"})), 201)
		if x := call(t, "POST", roomBase, ok(map[string]any{"name": "bazinga"})); !want(t, x, 422) || !strings.Contains(x.problem("name"), "sudah ada") {
			t.Errorf("twice: %s", trunc(x.Raw))
		}
	})
	t.Run("MN03 an inactive building or amenity cannot be chosen for a new room", func(t *testing.T) {
		off := testBuilding(t, map[string]any{"active": false})
		a := must(t, call(t, "POST", amnBase, map[string]any{"name": "Fasilitas mati", "active": false}), 201).str("code")
		if x := call(t, "POST", roomBase, map[string]any{"name": "X", "buildingCode": off.str("code"), "capacity": 2}); !want(t, x, 422) || x.problem("buildingCode") == "" {
			t.Errorf("building: %s", trunc(x.Raw))
		}
		if x := call(t, "POST", roomBase, map[string]any{"name": "X", "buildingCode": b.str("code"), "capacity": 2, "amenityCodes": []string{a}}); !want(t, x, 422) || x.problem("amenityCodes") == "" {
			t.Errorf("amenity: %s", trunc(x.Raw))
		}
	})
}

func TestMeetingRoomStress(t *testing.T) {
	t.Run("MS01 20 rooms saved at once get 20 codes", func(t *testing.T) {
		b := testBuilding(t, nil)
		codes := make([]string, 20)
		parallel(20, 10, func(i int) {
			codes[i] = must(t, call(t, "POST", roomBase, map[string]any{"name": fmt.Sprintf("Paralel %d", i), "buildingCode": b.str("code"), "capacity": 4 + i}), 201).str("code")
		})
		seen := map[string]bool{}
		for _, c := range codes {
			if seen[c] {
				t.Errorf("code %s twice", c)
			}
			seen[c] = true
		}
	})
}

func TestMeetingRoomImport(t *testing.T) {
	t.Run("MI01 templates; buildings, amenities and rooms from Excel (names, labels, several amenities)", func(t *testing.T) {
		for _, base := range []string{bldBase, amnBase, roomBase} {
			must(t, call(t, "GET", base+"/template.xlsx", nil), 200)
		}
		city := testCity(t)
		cityName := must(t, call(t, "GET", cityBase+"/"+city, nil), 200).str("name")
		b := upload(t, bldBase, true, []any{"Kode", "Nama Gedung", "Kota", "Unit Organisasi", "Zona Waktu", "Jumlah Lantai"},
			[]any{"", "Gedung Impor", cityName, "", "Asia/Makassar", 6})
		if b.Obj["committed"] != true || b.Obj["added"] != 1.0 {
			t.Fatalf("building %s", trunc(b.Raw))
		}
		a := upload(t, amnBase, true, []any{"Kode", "Nama Fasilitas", "Ikon"}, []any{"", "Proyektor Impor", "Proyektor"}, []any{"", "Papan Impor", "Papan tulis"})
		if a.Obj["committed"] != true || a.Obj["added"] != 2.0 {
			t.Fatalf("amenities %s", trunc(a.Raw))
		}
		h := []any{"Kode", "Nama Ruang", "Gedung", "Lantai", "Kapasitas (Kursi)", "Fasilitas", "Jam Buka", "Jam Tutup", "Durasi Terlama (Menit)", "Persetujuan", "Akhir Pekan"}
		x := upload(t, roomBase, false, h,
			[]any{"", "Ruang Impor", "Gedung Impor", "2", 10, "Proyektor Impor, papan impor", "07:30", "17:00", 120, "Dengan persetujuan", "Tidak"},
			[]any{"", "Ruang Salah", "Gedung Impor", "9", 0, "Tidak Ada", "08:10", "", "", "Kadang", ""},
		)
		if x.Obj["added"] != 1.0 || !strings.Contains(rowProblems(x, 1), "Tidak Ada") || !strings.Contains(rowProblems(x, 1), "Kadang") {
			t.Errorf("check %s | %s", counts(x), rowProblems(x, 1))
		}
		y := upload(t, roomBase, true, h, []any{"", "Ruang Impor", "Gedung Impor", "2", 10, "Proyektor Impor, papan impor", "07:30", "17:00", 120, "Dengan persetujuan", "Tidak"})
		if y.Obj["committed"] != true {
			t.Fatalf("rooms %s", trunc(y.Raw))
		}
		r := must(t, call(t, "GET", roomBase+"/"+rowCode(y, 0), nil), 200)
		if r.str("approvalMode") != "required" || r.str("openTime") != "07:30" || len(r.Obj["amenityCodes"].([]any)) != 2 || r.str("timeZone") != "Asia/Makassar" {
			t.Errorf("room %s", trunc(r.Raw))
		}
	})
}
