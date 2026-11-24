package containertype

import (
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// GroupLabels / HeightLabels name the choices as the form and the Excel template show them.
var (
	GroupLabels = map[string]string{"general": "General purpose (dry)", "reefer": "Reefer (berpendingin)", "open_top": "Open top", "flat_rack": "Flat rack",
		"tank": "Tank", "platform": "Platform", "ventilated": "Ventilated", "bulk": "Bulk", "other": "Lainnya"}
	HeightLabels = map[string]string{"standard": "Standar (8'6\")", "high_cube": "High cube (9'6\")"}
)

// importSpec is the Excel import of Jenis Kontainer.
func importSpec(_ *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}, {Key: "isoCode", Column: "iso_code"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MJK001"),
			master.NameColumn("Nama Jenis Kontainer", "40' High Cube (40HC)"),
			{Header: "Kode ISO 6346", Key: "isoCode", Text: true, Hint: "4 karakter ukuran-tipe, mis. 22G1 (20GP), 45G1 (40HC), 22R1 (20RF).", Example: "45G1", Width: 12},
			{Header: "Panjang (ft)", Key: "lengthFt", Required: true, Convert: master.Integer, Hint: "10, 20, 40, 45, 48 atau 53.", Example: "40", Width: 10},
			{Header: "Tinggi", Key: "height", Choices: master.Labels([]string{"standard", "high_cube"}, HeightLabels), Convert: master.ByLabel(HeightLabels), Hint: "Kosong berarti Standar.", Example: "High cube (9'6\")", Width: 16},
			{Header: "Kelompok", Key: "group", Required: true, Choices: master.Labels(Groups, GroupLabels), Convert: master.ByLabel(GroupLabels), Example: "General purpose (dry)", Width: 20},
			{Header: "TEU", Key: "teu", Convert: master.Number, Hint: "Kosong mengikuti panjang (20 ft = 1, 40/45 ft = 2).", Width: 8},
			{Header: "Berat Kotor Maks. (kg)", Key: "maxGrossKg", Convert: master.Integer, Width: 14},
			{Header: "Berat Kosong (kg)", Key: "tareKg", Convert: master.Integer, Width: 12},
			{Header: "Volume Dalam (m³)", Key: "innerVolumeCbm", Convert: master.Number, Width: 12},
		}, master.TailColumns()),
	}
}
