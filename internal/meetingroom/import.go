package meetingroom

import (
	"context"
	"strings"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// ApprovalLabels name the Persetujuan choices as the form and the Excel template show them.
var ApprovalLabels = map[string]string{"none": "Tanpa persetujuan", "required": "Dengan persetujuan"}

// amenities reads several Fasilitas Ruang in one cell (codes or names, separated by a comma or semicolon).
func amenities(db *database.DB) func(context.Context, string) (any, string, error) {
	one := master.Lookup(db, "room_amenity", "Fasilitas", "code", "name")
	return func(ctx context.Context, s string) (any, string, error) {
		out := []any{}
		for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
			if strings.TrimSpace(part) == "" {
				continue
			}
			x, problem, err := one(ctx, strings.TrimSpace(part))
			if problem != "" || err != nil {
				return nil, problem, err
			}
			out = append(out, x)
		}
		return out, "", nil
	}
}

// importSpec is the Excel import of Ruang Rapat.
func importSpec(db *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name", Scope: "buildingCode", ScopeColumn: "building_id", ScopeTable: "building"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MRR001"),
			{Header: "Nama Ruang", Key: "name", Required: true, Hint: "Maks. 150 karakter. Tidak boleh sama dalam satu gedung.", Example: "Merapi", Width: 22},
			{Header: "Gedung", Key: "buildingCode", Required: true, Convert: master.Lookup(db, "building", "Gedung", "code", "name"), Hint: "Kode atau nama di Gedung & Kantor (sheet Gedung).", Example: "Kantor Pusat", Width: 20},
			{Header: "Lantai", Key: "floor", Text: true, Hint: "Mis. 3, LG; maks. 20 karakter.", Example: "3", Width: 8},
			{Header: "Kapasitas (Kursi)", Key: "capacity", Required: true, Convert: master.Integer, Hint: "1–1000.", Example: "12", Width: 12},
			{Header: "Fasilitas", Key: "amenityCodes", Convert: amenities(db), Hint: "Kode atau nama Fasilitas Ruang, pisahkan dengan koma (sheet Fasilitas).", Example: "Proyektor, Papan tulis", Width: 26},
			{Header: "Jam Buka", Key: "openTime", Text: true, Hint: "HH:MM jam gedung, menit 00/15/30/45. Kosong berarti 08:00.", Example: "08:00", Width: 10},
			{Header: "Jam Tutup", Key: "closeTime", Text: true, Hint: "HH:MM, setelah jam buka. Kosong berarti 18:00.", Example: "18:00", Width: 10},
			{Header: "Durasi Terlama (Menit)", Key: "maxDurationMinutes", Convert: master.Integer, Hint: "Kelipatan 15; kosong berarti sampai jam tutup.", Width: 14},
			{Header: "Persetujuan", Key: "approvalMode", Choices: master.Labels(ApprovalModes, ApprovalLabels), Convert: master.ByLabel(ApprovalLabels), Hint: "Kosong berarti Tanpa persetujuan (siapa duluan booking, dia yang dapat).", Example: "Tanpa persetujuan", Width: 18},
			{Header: "Akhir Pekan", Key: "weekendBookable", Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: "Ya bila bisa dipesan Sabtu/Minggu. Kosong berarti Tidak.", Example: "Tidak", Width: 10},
			{Header: "Penanggung Jawab", Key: "picEmployeeCode", Convert: master.Lookup(db, "employee", "Karyawan", "code", "nik", "name"), Hint: "Kode, NIK atau nama Karyawan (opsional).", Width: 20},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			b, err := master.ListRows(ctx, db, `SELECT b.code, b.name, c.name FROM pm.building b JOIN pm.city c ON c.id = b.city_id WHERE b.is_active AND NOT b.is_deleted ORDER BY b.name`, 3)
			if err != nil {
				return nil, err
			}
			a, err := master.ListRows(ctx, db, `SELECT code, name FROM pm.room_amenity WHERE is_active AND NOT is_deleted ORDER BY name`, 2)
			if err != nil {
				return nil, err
			}
			return []master.ImportList{{Sheet: "Gedung", Header: []string{"Kode", "Nama", "Kota"}, Rows: b}, {Sheet: "Fasilitas", Header: []string{"Kode", "Nama"}, Rows: a}}, nil
		},
	}
}
