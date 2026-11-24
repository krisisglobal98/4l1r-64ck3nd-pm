package location

import (
	"context"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// KindLabels name the kinds as the form and the Excel template show them.
var KindLabels = map[string]string{"seaport": "Pelabuhan laut", "river_port": "Pelabuhan sungai", "airport": "Bandara", "dry_port": "Dry port / ICD",
	"rail_terminal": "Terminal kereta", "border": "Pos lintas batas", "other": "Lainnya"}

// importSpec is the Excel import of Pelabuhan & Lokasi.
func importSpec(db *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "unlocode", Column: "unlocode"}, {Key: "name", Column: "name", Scope: "countryCode", ScopeColumn: "country_id", ScopeTable: "country"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MLK001"),
			{Header: "Nama Lokasi", Key: "name", Required: true, Hint: "Nama pelabuhan / bandara / kota, maks. 150 karakter. Tidak boleh sama dalam satu negara.", Example: "Tanjung Priok", Width: 26},
			{Header: "UN/LOCODE", Key: "unlocode", Text: true, Hint: "5 karakter: kode negara lalu 3 huruf/angka, mis. IDTPP. Tidak boleh sama.", Example: "IDTPP", Width: 10},
			{Header: "Negara", Key: "countryCode", Required: true, Convert: master.Lookup(db, "country", "Negara", "iso2", "iso3", "code", "name"), Hint: "Kode ISO 2 huruf (sheet Negara).", Example: "ID", Width: 10},
			{Header: "Kota", Key: "cityCode", Convert: master.Lookup(db, "city", "Kota", "code", "name"), Hint: "Kode atau nama di master Kota; kosongkan bila belum ada.", Width: 14},
			{Header: "Jenis Lokasi", Key: "kind", Required: true, Choices: master.Labels(Kinds, KindLabels), Convert: master.ByLabel(KindLabels), Example: "Pelabuhan laut", Width: 16},
			{Header: "Kode IATA", Key: "iataCode", Hint: "3 huruf, hanya untuk bandara.", Width: 10},
			{Header: "Kode Kantor Pabean", Key: "customsOffice", Text: true, Hint: "6 angka, mis. 040300.", Width: 14},
			{Header: "Lintang", Key: "latitude", Convert: master.Number, Width: 10},
			{Header: "Bujur", Key: "longitude", Convert: master.Number, Width: 10},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			c, err := master.ListRows(ctx, db, `SELECT iso2, name, code FROM pm.country WHERE is_active AND NOT is_deleted ORDER BY name`, 3)
			if err != nil {
				return nil, err
			}
			k, err := master.ListRows(ctx, db, `SELECT c.code, c.name, n.iso2 FROM pm.city c LEFT JOIN pm.country n ON n.id = c.country_id WHERE c.is_active AND NOT c.is_deleted ORDER BY c.name`, 3)
			if err != nil {
				return nil, err
			}
			return []master.ImportList{{Sheet: "Negara", Header: []string{"Kode ISO", "Nama", "Kode Master"}, Rows: c}, {Sheet: "Kota", Header: []string{"Kode", "Nama", "Negara"}, Rows: k}}, nil
		},
	}
}
