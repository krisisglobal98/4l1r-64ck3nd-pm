package documenttype

import (
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// GroupLabels / IssuerLabels name the choices as the form and the Excel template show them.
var (
	GroupLabels  = map[string]string{"shipping": "Pengiriman", "customs": "Kepabeanan", "commercial": "Komersial", "certificate": "Sertifikat", "insurance": "Asuransi", "other": "Lainnya"}
	IssuerLabels = map[string]string{"carrier": "Pelayaran / maskapai", "forwarder": "Forwarder", "customs": "Bea Cukai", "shipper": "Shipper", "government": "Instansi pemerintah",
		"company": "Perusahaan sendiri", "other": "Lainnya"}
)

// importSpec is the Excel import of Jenis Dokumen.
func importSpec(_ *database.DB) *master.Import {
	yn := func(header, key, hint, example string) master.ImportColumn {
		return master.ImportColumn{Header: header, Key: key, Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: hint, Example: example, Width: 10}
	}
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MJD001"),
			master.NameColumn("Nama Dokumen", "Master Bill of Lading (MB/L)"),
			{Header: "Kelompok", Key: "group", Required: true, Choices: master.Labels(Groups, GroupLabels), Convert: master.ByLabel(GroupLabels), Example: "Pengiriman", Width: 14},
			{Header: "Kode BC", Key: "customsCode", Hint: "Kode dokumen pabean, mis. BC 3.0 untuk PEB.", Width: 10},
			{Header: "Penerbit", Key: "issuer", Choices: master.Labels(Issuers, IssuerLabels), Convert: master.ByLabel(IssuerLabels), Hint: "Kosong berarti Lainnya.", Example: "Pelayaran / maskapai", Width: 20},
			yn("Ekspor", "forExport", "Kosong berarti Ya. Minimal satu dari Ekspor/Impor/Domestik Ya.", "Ya"),
			yn("Impor", "forImport", "Kosong berarti Ya.", "Ya"),
			yn("Domestik", "forDomestic", "Kosong berarti Tidak.", "Tidak"),
			yn("Wajib Nomor", "numberRequired", "Kosong berarti Ya: nomor dokumen diisi di job.", "Ya"),
			yn("Ada Masa Berlaku", "hasExpiry", "Kosong berarti Tidak.", "Tidak"),
			yn("Wajib Asli", "originalRequired", "Kosong berarti Tidak.", "Tidak"),
		}, master.TailColumns()),
	}
}
