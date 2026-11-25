package building

import (
	"context"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// importSpec is the Excel import of Gedung & Kantor.
func importSpec(db *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name", Scope: "cityCode", ScopeColumn: "city_id", ScopeTable: "city"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MGD001"),
			{Header: "Nama Gedung", Key: "name", Required: true, Hint: "Maks. 150 karakter. Tidak boleh sama dalam satu kota.", Example: "Kantor Pusat", Width: 26},
			{Header: "Kota", Key: "cityCode", Required: true, Convert: master.Lookup(db, "city", "Kota", "code", "name"), Hint: "Kode atau nama di master Kota (sheet Kota).", Example: "Jakarta Selatan", Width: 18},
			{Header: "Unit Organisasi", Key: "unitCode", Convert: master.Lookup(db, "org_unit", "Unit organisasi", "code", "name"), Hint: "Unit yang mengelola gedung (opsional).", Width: 18},
			{Header: "Zona Waktu", Key: "timeZone", Hint: "Nama IANA, mis. Asia/Jakarta (WIB), Asia/Makassar (WITA), Asia/Jayapura (WIT). Kosong berarti Asia/Jakarta.", Example: "Asia/Jakarta", Width: 16},
			{Header: "Jumlah Lantai", Key: "floors", Convert: master.Integer, Hint: "1–200.", Example: "12", Width: 12},
			{Header: "Alamat", Key: "address", Hint: "Maks. 500 karakter.", Width: 36},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			k, err := master.ListRows(ctx, db, `SELECT c.code, c.name, coalesce(c.province, '') FROM pm.city c WHERE c.is_active AND NOT c.is_deleted ORDER BY c.name`, 3)
			if err != nil {
				return nil, err
			}
			return []master.ImportList{{Sheet: "Kota", Header: []string{"Kode", "Nama", "Provinsi"}, Rows: k}}, nil
		},
	}
}
