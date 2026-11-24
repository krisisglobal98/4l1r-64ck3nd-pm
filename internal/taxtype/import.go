package taxtype

import (
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// CategoryLabels / MethodLabels name the choices as the form and the Excel template show them.
var (
	CategoryLabels = map[string]string{"PPN": "PPN", "PPNBM": "PPnBM", "PPH21": "PPh 21", "PPH22": "PPh 22", "PPH23": "PPh 23", "PPH4_2": "PPh 4(2)",
		"PPH15": "PPh 15", "PPH26": "PPh 26", "OTHER": "Lainnya"}
	MethodLabels = map[string]string{"added": "Ditambahkan ke tagihan (PPN)", "withheld": "Dipotong pembayar (PPh)"}
)

// importSpec is the Excel import of Jenis Pajak.
func importSpec(_ *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MPJ001"),
			master.NameColumn("Nama Pajak", "PPN 12% (DPP 11/12)"),
			{Header: "Kategori", Key: "category", Required: true, Choices: master.Labels(Categories, CategoryLabels), Convert: master.ByLabel(CategoryLabels), Example: "PPN", Width: 12},
			{Header: "Tarif (%)", Key: "ratePct", Required: true, Convert: master.Number, Hint: "0–100, angka biasa: isi 12 untuk 12% (bukan format persen Excel).", Example: "12", Width: 10},
			{Header: "Dasar Pengenaan (%)", Key: "basePct", Convert: master.Number, Hint: "Bagian nilai yang dikenai pajak. Kosong berarti 100. DPP 11/12 = 91.6667; jasa freight 10% = 10.", Example: "91.6667", Width: 16},
			{Header: "Cara", Key: "method", Choices: master.Labels([]string{"added", "withheld"}, MethodLabels), Convert: master.ByLabel(MethodLabels), Hint: "Kosong mengikuti kategori: PPN/PPnBM ditambahkan, PPh dipotong.", Example: "Ditambahkan ke tagihan (PPN)", Width: 26},
			{Header: "Harga Termasuk Pajak", Key: "inclusive", Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: "Ya bila harga sudah termasuk PPN (PPN diambil dari harga, total tidak bertambah). Kosong berarti Tidak (PPN ditambahkan di atas harga).", Example: "Tidak", Width: 12},
			{Header: "Kode di Sistem Pajak", Key: "taxCode", Hint: "Kode objek / jenis pajak di sistem pajak (opsional), maks. 20 karakter.", Example: "", Width: 16},
		}, master.ValidityColumns(false), master.TailColumns()),
	}
}
