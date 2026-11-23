package bank

import (
	"context"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// importSpec is the Excel import of Bank.
func importSpec(db *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}, {Key: "swiftCode", Column: "swift_code"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MBK001"),
			master.NameColumn("Nama Bank", "Bank Central Asia"),
			{Header: "Kode Kliring", Key: "clearing", Text: true, Hint: "3–7 angka, mis. 014 (kolom ini berformat teks agar nol di depan tetap ada).", Example: "014", Width: 12},
			{Header: "Kode SWIFT/BIC", Key: "swiftCode", Hint: "8 atau 11 karakter.", Example: "CENAIDJA", Width: 14},
			{Header: "Negara", Key: "countryCode", Convert: master.Lookup(db, "country", "Negara", "iso2", "iso3", "code", "name"), Hint: "Kode ISO 2 huruf, mis. ID (lihat sheet Negara).", Example: "ID", Width: 10},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			c, err := master.ListRows(ctx, db, `SELECT iso2, name, code FROM pm.country WHERE is_active AND NOT is_deleted ORDER BY name`, 3)
			if err != nil {
				return nil, err
			}
			return []master.ImportList{{Sheet: "Negara", Header: []string{"Kode ISO", "Nama", "Kode Master"}, Rows: c}}, nil
		},
	}
}
