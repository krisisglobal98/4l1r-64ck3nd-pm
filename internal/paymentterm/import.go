package paymentterm

import (
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// BasisLabels / AppliesLabels name the choices as the form and the Excel template show them.
var (
	BasisLabels = map[string]string{"invoice_date": "Tanggal invoice", "receipt_date": "Tanggal invoice diterima", "bl_date": "Tanggal B/L",
		"etd": "ETD (berangkat)", "eta": "ETA (tiba)", "end_of_month": "Akhir bulan", "delivery": "Tanggal serah terima"}
	AppliesLabels = map[string]string{"both": "Customer dan vendor", "customer": "Customer", "vendor": "Vendor"}
)

// importSpec is the Excel import of Syarat Pembayaran.
func importSpec(_ *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MTP001"),
			master.NameColumn("Nama Syarat", "Net 30"),
			{Header: "Jumlah Hari", Key: "days", Required: true, Convert: master.Integer, Hint: "0–365. 0 berarti tunai / memakai hari bawaan System Parameter.", Example: "30", Width: 12},
			{Header: "Dihitung Dari", Key: "basis", Choices: master.Labels(Bases, BasisLabels), Convert: master.ByLabel(BasisLabels), Hint: "Kosong berarti Tanggal invoice.", Example: "Tanggal invoice", Width: 22},
			{Header: "Uang Muka (%)", Key: "downPaymentPct", Convert: master.Number, Hint: "0–100; isi 30 untuk 30%.", Example: "0", Width: 12},
			{Header: "Berlaku Untuk", Key: "appliesTo", Choices: master.Labels([]string{"both", "customer", "vendor"}, AppliesLabels), Convert: master.ByLabel(AppliesLabels), Hint: "Kosong berarti Customer dan vendor.", Example: "Customer dan vendor", Width: 20},
		}, master.TailColumns()),
	}
}
