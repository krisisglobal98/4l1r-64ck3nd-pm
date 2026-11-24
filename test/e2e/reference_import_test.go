//go:build e2e

// Excel import of the reference masters a forwarder migrates with its partners: Jenis Kontainer, Satuan, Jenis
// Dokumen, Pelabuhan & Lokasi (a name unique within its country), and Tenaga Penjualan (pointing to a Karyawan).
// Positive, negative.
package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestReferenceImport(t *testing.T) {
	sfx := fmt.Sprint(time.Now().UnixNano() % 100000)

	t.Run("RI01 every one has a template", func(t *testing.T) {
		for _, base := range []string{ctnBase, uomBase, docBase, locBase, spBase} {
			must(t, call(t, "GET", base+"/template.xlsx", nil), 200)
		}
	})

	t.Run("RI02 Jenis Kontainer and Satuan: choices by label, the ISO / UN/ECE code unique", func(t *testing.T) {
		ch := []any{"Kode", "Nama Jenis Kontainer", "Kode ISO 6346", "Panjang (ft)", "Tinggi", "Kelompok"}
		x := upload(t, ctnBase, false, ch,
			[]any{"", "45' High Cube Uji " + sfx, "L5G1", 45, "High cube (9'6\")", "General purpose (dry)"},
			[]any{"", "Panjang Salah " + sfx, "", 30, "", "Reefer (berpendingin)"},
			[]any{"", "ISO Kembar " + sfx, "22g1", 20, "", "General purpose (dry)"},
		)
		if x.Obj["added"] != 1.0 || !strings.Contains(rowProblems(x, 1), "Panjang") || !strings.Contains(rowProblems(x, 2), "Kode ISO 6346") {
			t.Errorf("containers %s | %s | %s", counts(x), rowProblems(x, 1), rowProblems(x, 2))
		}
		uh := []any{"Kode", "Nama Satuan", "Kode UN/ECE", "Besaran", "Faktor ke Satuan Dasar"}
		y := upload(t, uomBase, true, uh, []any{"", "Set Uji " + sfx, "", "Jumlah", 1}, []any{"", "Malam Uji " + sfx, "", "Waktu", 24})
		if y.Obj["committed"] != true || y.Obj["added"] != 2.0 {
			t.Errorf("uoms %s %s", counts(y), trunc(y.Raw))
		}
		z := upload(t, uomBase, false, uh, []any{"", "Lainnya Berfaktor " + sfx, "", "Lainnya", 2})
		if !strings.Contains(rowProblems(z, 0), "Faktor") {
			t.Errorf("factor of other: %s", rowProblems(z, 0))
		}
	})

	t.Run("RI03 Jenis Dokumen: Ya/Tidak flags, at least one of export/import/domestic", func(t *testing.T) {
		h := []any{"Kode", "Nama Dokumen", "Kelompok", "Kode BC", "Penerbit", "Ekspor", "Impor", "Domestik", "Wajib Nomor"}
		x := upload(t, docBase, false, h,
			[]any{"", "Part Of B/L Uji " + sfx, "Pengiriman", "", "Forwarder", "Ya", "Tidak", "Tidak", "Ya"},
			[]any{"", "Tanpa Arah " + sfx, "Pengiriman", "", "", "Tidak", "Tidak", "Tidak"},
		)
		if x.Obj["added"] != 1.0 || !strings.Contains(rowProblems(x, 1), "Ekspor") {
			t.Errorf("%s %s", counts(x), rowProblems(x, 1))
		}
	})

	t.Run("RI04 Pelabuhan & Lokasi: a name unique within its country, stored or in the file", func(t *testing.T) {
		h := []any{"Kode", "Nama Lokasi", "UN/LOCODE", "Negara", "Kota", "Jenis Lokasi"}
		ok := upload(t, locBase, true, h, []any{"", "Pelabuhan Uji " + sfx, "", "ID", "", "Pelabuhan laut"})
		if ok.Obj["committed"] != true {
			t.Fatalf("%s", trunc(ok.Raw))
		}
		x := upload(t, locBase, false, h,
			[]any{"", "pelabuhan uji " + sfx, "", "ID", "", "Pelabuhan laut"},     // stored, same country
			[]any{"", "Pelabuhan Uji " + sfx, "", "SG", "", "Pelabuhan laut"},     // another country: fine
			[]any{"", "Kembar " + sfx, "", "MY", "", "Pelabuhan laut"},            //
			[]any{"", "KEMBAR " + sfx, "", "MY", "", "Pelabuhan laut"},            // same country in the file
			[]any{"", "Bandara Salah " + sfx, "", "ID", "", "Pelabuhan laut", ""}, //
		)
		if p := rowProblems(x, 0); !strings.Contains(p, rowCode(ok, 0)) {
			t.Errorf("stored in the country: %s", p)
		}
		if p := rowProblems(x, 1); p != "" {
			t.Errorf("another country: %s", p)
		}
		if p := rowProblems(x, 3); !strings.Contains(p, "baris 4") {
			t.Errorf("in the file: %s", p)
		}
	})

	t.Run("RI05 Tenaga Penjualan: the employee by code or NIK, one sales person per employee", func(t *testing.T) {
		emp := must(t, call(t, "GET", emBase+"/"+newEmployee(t), nil), 200)
		h := []any{"Kode", "Nama Tenaga Penjualan", "Karyawan", "Unit Penjualan", "Email Kerja", "Telepon", "Komisi (%)"}
		x := upload(t, spBase, false, h,
			[]any{"", "Sales Uji " + sfx, emp.str("nik"), "", "sales@contoh.co.id", "+62 21 555 0101", 2.5},
			[]any{"", "Sales Tanpa Karyawan " + sfx, "NIK-TIDAK-ADA"},
		)
		if x.Obj["added"] != 1.0 || !strings.Contains(rowProblems(x, 1), "Karyawan") {
			t.Fatalf("%s %s", counts(x), rowProblems(x, 1))
		}
		ok := upload(t, spBase, true, h, []any{"", "Sales Uji " + sfx, emp.str("code"), "", "", "", 2.5})
		if g := must(t, call(t, "GET", spBase+"/"+rowCode(ok, 0), nil), 200); g.str("employeeCode") != emp.str("code") || g.Obj["commissionPct"] != 2.5 {
			t.Errorf("saved %s", trunc(g.Raw))
		}
		y := upload(t, spBase, false, h, []any{"", "Sales Lain " + sfx, emp.str("code")})
		if p := rowProblems(y, 0); !strings.Contains(p, "sudah menjadi tenaga penjualan") {
			t.Errorf("one per employee: %s", p)
		}
		// An employee who left: an inactive sales person may point to them (history), an active one may not.
		gone := must(t, call(t, "GET", emBase+"/"+newEmployee(t), nil), 200)
		must(t, call(t, "PUT", emBase+"/"+gone.str("code"), map[string]any{"active": false}), 200)
		hh := []any{"Kode", "Nama Tenaga Penjualan", "Karyawan", "Aktif"}
		z := upload(t, spBase, false, hh, []any{"", "Sales Keluar " + sfx, gone.str("nik"), "Tidak"}, []any{"", "Sales Aktif Keluar " + sfx, gone.str("code"), "Ya"})
		if p := rowProblems(z, 0); p != "" {
			t.Errorf("inactive to inactive: %s", p)
		}
		if p := rowProblems(z, 1); !strings.Contains(p, "nonaktif") {
			t.Errorf("active to inactive: %s", p)
		}
	})
}
