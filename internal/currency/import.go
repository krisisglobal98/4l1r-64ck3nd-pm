package currency

import (
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// importSpec is the Excel import of Mata Uang.
func importSpec(_ *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "isoCode", Column: "iso_code"}, {Key: "name", Column: "name"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MMU001"),
			{Header: "Kode ISO", Key: "isoCode", Required: true, Hint: "Kode ISO 4217, 3 huruf. Tidak boleh sama dengan mata uang lain.", Example: "SGD", Width: 10},
			master.NameColumn("Nama Mata Uang", "Dolar Singapura"),
			{Header: "Simbol", Key: "symbol", Hint: "Maks. 5 karakter.", Example: "S$", Width: 10},
			{Header: "Angka Desimal", Key: "decimals", Convert: master.Integer, Hint: "Digit di belakang koma pada nominal, 0–4. Kosong berarti 2 (IDR biasanya 0).", Example: "2", Width: 12},
		}, master.TailColumns()),
	}
}
