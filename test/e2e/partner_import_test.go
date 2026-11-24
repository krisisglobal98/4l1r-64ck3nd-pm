//go:build e2e

// Excel import of Mitra Bisnis: one row per partner with its primary address, contact and bank account as columns;
// roles as several labels in a cell; the partner rules per row (customer terms, contact reach, postal code, NPWP);
// a change by Kode touches the primary rows only; a name already there is not added. Positive, negative.
package e2e

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestPartnerImport(t *testing.T) {
	r := newPartnerRefs(t)
	sfx := fmt.Sprint(time.Now().UnixNano() % 100000)
	head := []any{"Kode", "Nama Mitra", "Nama Dagang", "Bentuk Usaha", "Peran", "Negara", "Telepon", "NPWP", "Alamat", "Kode Pos",
		"Nama Kontak", "Email Kontak", "Telepon Kontak", "Syarat Pembayaran Customer", "Mata Uang Tagihan", "Bank", "No. Rekening",
		"Nama Pemilik Rekening", "Mata Uang Rekening", "Deskripsi", "Aktif"}

	t.Run("BP01 the template: the partner, its primary address, contact and bank account, the reference sheets", func(t *testing.T) {
		x := must(t, call(t, "GET", bpBase+"/template.xlsx", nil), 200)
		f, err := excelize.OpenReader(bytes.NewReader(x.Raw))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(f.GetSheetList(), ","); got != "Data,Petunjuk,Negara,Kota,Syarat Pembayaran,Mata Uang,Tenaga Penjualan,Pajak,Bank" {
			t.Errorf("sheets %s", got)
		}
		rows, _ := f.GetRows("Data")
		h := strings.Join(rows[0], "|")
		for _, w := range []string{"Nama Mitra *", "Bentuk Usaha *", "Peran *", "Negara *", "Alamat", "Nama Kontak", "No. Rekening", "PPh Dipotong"} {
			if !strings.Contains(h, w) {
				t.Errorf("header lacks %q: %s", w, h)
			}
		}
	})

	t.Run("BP02 rows checked by the partner rules; a clean file saved with its lists", func(t *testing.T) {
		x := upload(t, bpBase, true, head,
			[]any{"", "Consignee Uji " + sfx, "CU", "Badan usaha asing", "Consignee, Notify party", "SG", "+65 6555 0101", "", "1 Raffles Place, Singapore", "048616",
				"Ms. Tan", "tan@contoh.sg", "", "", "", "", "", "", "", "Kode lama: CS-00001", "Ya"},
			[]any{"", "Customer Tanpa Syarat " + sfx, "", "PT", "Customer", "ID", "", "", "Jl. Uji 1"},
			[]any{"", "Peran Salah " + sfx, "", "PT", "Pembeli", "ID"},
			[]any{"", "Kontak Tanpa Telepon " + sfx, "", "Lainnya", "Shipper", "ZZ", "", "", "Jl. Uji 2", "1", "Pak Budi"},
			[]any{"", "Rekening Uji " + sfx, "", "PT", "Vendor", "ID", "", "12345", "", "", "", "", "", "", "", r.bank, "0012-3456", "PT Rekening Uji", "IDR"},
		)
		if x.Obj["committed"] != false || x.Obj["failed"] != 4.0 {
			t.Fatalf("%s %s", counts(x), trunc(x.Raw))
		}
		if p := rowProblems(x, 1); !strings.Contains(p, "Syarat Pembayaran Customer") || !strings.Contains(p, "Mata Uang Tagihan") {
			t.Errorf("customer terms: %s", p)
		}
		if p := rowProblems(x, 2); !strings.HasPrefix(p, "Peran:") {
			t.Errorf("role: %s", p)
		}
		if p := rowProblems(x, 3); !strings.Contains(p, "Negara") {
			t.Errorf("country: %s", p)
		}
		if p := rowProblems(x, 4); !strings.Contains(p, "NPWP") {
			t.Errorf("npwp: %s", p)
		}
		// The form's own rules on the lists, once the references resolve.
		y := upload(t, bpBase, false, head, []any{"", "Kontak Tanpa Telepon " + sfx, "", "Lainnya", "Shipper", "ID", "", "", "Jl. Uji 2", "1", "Pak Budi"})
		if p := rowProblems(y, 0); !strings.Contains(p, "Kode Pos") || !strings.Contains(p, "Email Kontak") {
			t.Errorf("postal code and contact reach: %s", p)
		}
		ok := upload(t, bpBase, true, head,
			[]any{"", "Consignee Uji " + sfx, "CU", "Badan usaha asing", "Consignee, Notify party", "SG", "+65 6555 0101", "", "1 Raffles Place, Singapore", "048616",
				"Ms. Tan", "tan@contoh.sg", "", "", "", "", "", "", "", "Kode lama: CS-00001", "Ya"},
			[]any{"", "Customer Uji " + sfx, "", "PT", "customer; shipper", "Indonesia", "", nextNpwp(), "Jl. Uji 1, Jakarta", "", "", "", "", r.termC, "IDR",
				r.bank, "0012-3456", "PT Customer Uji", "IDR"},
		)
		if ok.Obj["committed"] != true {
			t.Fatalf("%s", trunc(ok.Raw))
		}
		g := must(t, call(t, "GET", bpBase+"/"+rowCode(ok, 0), nil), 200)
		adr, con := rowsOf(g, "addresses"), rowsOf(g, "contacts")
		if g.str("entityType") != "foreign" || strings.Join(strs(g.Obj["roles"]), ",") != "consignee,notify_party" || len(adr) != 1 || adr[0]["postalCode"] != "048616" ||
			adr[0]["kind"] != "office" || len(con) != 1 || con[0]["email"] != "tan@contoh.sg" || con[0]["primary"] != true {
			t.Errorf("consignee %s", trunc(g.Raw))
		}
		c := must(t, call(t, "GET", bpBase+"/"+rowCode(ok, 1), nil), 200)
		if acc := rowsOf(c, "bankAccounts"); len(acc) != 1 || acc[0]["accountNumber"] != "0012-3456" || c.str("customerPaymentTermCode") != r.termC {
			t.Errorf("customer %s", trunc(c.Raw))
		}
	})

	t.Run("BP03 a change by Kode replaces the primary rows' filled cells; the other rows stay", func(t *testing.T) {
		p := must(t, call(t, "POST", bpBase, r.customer(map[string]any{"name": "PT Ubah Lewat Upload " + sfx})), 201)
		x := upload(t, bpBase, true, head, []any{p.str("code"), "", "", "", "", "", "+62 21 777 0000", "", "Kawasan Industri Dumai Blok B"})
		if x.Obj["committed"] != true || x.Obj["updated"] != 1.0 {
			t.Fatalf("%s %s", counts(x), trunc(x.Raw))
		}
		g := must(t, call(t, "GET", bpBase+"/"+p.str("code"), nil), 200)
		adr := rowsOf(g, "addresses")
		if g.str("phone") != "+62 21 777 0000" || len(adr) != 2 || len(rowsOf(g, "contacts")) != 2 || len(rowsOf(g, "bankAccounts")) != 2 {
			t.Fatalf("kept %s", trunc(g.Raw))
		}
		for _, a := range adr {
			switch a["kind"] {
			case "warehouse": // the primary one: its address changed, its label kept
				if a["address"] != "Kawasan Industri Dumai Blok B" || a["label"] != "Gudang Dumai" || a["primary"] != true {
					t.Errorf("primary %v", a)
				}
			case "billing":
				if a["address"] != "Jl. Sudirman 1" {
					t.Errorf("other row changed %v", a)
				}
			}
		}
	})

	t.Run("BP04 a name already there is not added; the same name twice in the file neither", func(t *testing.T) {
		x := upload(t, bpBase, false, head,
			[]any{"", "consignee uji " + sfx, "", "Lainnya", "Consignee", "SG"},
			[]any{"", "Mitra Kembar " + sfx, "", "Lainnya", "Consignee", "SG"},
			[]any{"", "MITRA KEMBAR " + sfx, "", "Lainnya", "Shipper", "SG"},
		)
		if p := rowProblems(x, 0); !strings.HasPrefix(p, "Nama Mitra:") || !strings.Contains(p, "isi kolom Kode") {
			t.Errorf("stored: %s", p)
		}
		if p := rowProblems(x, 2); !strings.Contains(p, "baris 3") {
			t.Errorf("in the file: %s", p)
		}
	})
}

func strs(x any) []string {
	var out []string
	for _, v := range x.([]any) {
		out = append(out, fmt.Sprint(v))
	}
	return out
}
