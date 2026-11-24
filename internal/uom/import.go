package uom

import (
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// DimensionLabels name the dimensions as the form and the Excel template show them.
var DimensionLabels = map[string]string{"mass": "Massa", "volume": "Volume", "length": "Panjang", "area": "Luas", "count": "Jumlah", "time": "Waktu", "other": "Lainnya"}

// importSpec is the Excel import of Satuan.
func importSpec(_ *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}, {Key: "uneceCode", Column: "unece_code"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MST001"),
			master.NameColumn("Nama Satuan", "Meter kubik"),
			{Header: "Kode UN/ECE", Key: "uneceCode", Text: true, Hint: "Kode UN/ECE Rec. 20, 2–3 huruf/angka, mis. KGM (kilogram), MTQ (m³), SET, DAY.", Example: "MTQ", Width: 12},
			{Header: "Besaran", Key: "dimension", Required: true, Choices: master.Labels(Dimensions, DimensionLabels), Convert: master.ByLabel(DimensionLabels), Example: "Volume", Width: 12},
			{Header: "Faktor ke Satuan Dasar", Key: "factor", Convert: master.Number, Hint: "Mis. gram = 0.001 (dasar kg), hari = 24 (dasar jam). Kosongkan untuk besaran Lainnya.", Example: "1", Width: 14},
		}, master.TailColumns()),
	}
}
