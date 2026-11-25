package roomamenity

import (
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// IconLabels name the icons as the form and the Excel template show them.
var IconLabels = map[string]string{"projector": "Proyektor", "screen": "Layar / TV", "video": "Konferensi video", "audio": "Audio / mikrofon",
	"whiteboard": "Papan tulis", "wifi": "Wi-Fi", "phone": "Telepon", "ac": "AC", "pantry": "Pantry / konsumsi", "accessible": "Akses kursi roda", "other": "Lainnya"}

// importSpec is the Excel import of Fasilitas Ruang.
func importSpec(_ *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MFR001"),
			master.NameColumn("Nama Fasilitas", "Proyektor"),
			{Header: "Ikon", Key: "icon", Choices: master.Labels(Icons, IconLabels), Convert: master.ByLabel(IconLabels), Hint: "Gambar yang tampil di jadwal. Kosong berarti Lainnya.", Example: "Proyektor", Width: 18},
		}, master.TailColumns()),
	}
}
