//go:build e2e

// Excel import of the Keuangan & Pajak masters (Mata Uang, Bank, Syarat Pembayaran, Jenis Pajak, Kurs, Rekening Bank
// Perusahaan, Bagan Akun): the template of each, the master's own rules per row, values already there not added,
// dates from Excel date cells and text, text columns keeping leading zeros. Positive, negative.
package e2e

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// upload sends the rows (header first) to a master's upload; commit keeps them when no row fails.
func upload(t testing.TB, base string, commit bool, rows ...[]any) resp {
	t.Helper()
	q := ""
	if commit {
		q = "?commit=true"
	}
	return must(t, call(t, "POST", base+"/upload"+q, workbook(t, rows)), 200)
}

// problems of the n-th row of an upload result, as "field: message" lines.
func rowProblems(r resp, n int) string {
	rows, _ := r.Obj["rows"].([]any)
	if n >= len(rows) {
		return "<no row>"
	}
	var out []string
	for _, p := range rows[n].(map[string]any)["problems"].([]any) {
		m := p.(map[string]any)
		out = append(out, fmt.Sprint(m["field"], ": ", m["message"]))
	}
	return strings.Join(out, " | ")
}

func rowCode(r resp, n int) string {
	s, _ := r.Obj["rows"].([]any)[n].(map[string]any)["code"].(string)
	return s
}

func counts(r resp) string {
	return fmt.Sprintf("added=%v updated=%v failed=%v committed=%v", r.Obj["added"], r.Obj["updated"], r.Obj["failed"], r.Obj["committed"])
}

func TestFinanceImport(t *testing.T) {
	sfx := fmt.Sprint(time.Now().UnixNano() % 100000)

	t.Run("FI01 every Keuangan & Pajak master has a template: Data, Petunjuk, the required columns", func(t *testing.T) {
		for base, want := range map[string][]string{
			curBase:  {"Kode Iso *", "Nama Mata Uang *"},
			bkBase:   {"Nama Bank *", "Kode Kliring"},
			termBase: {"Nama Syarat *", "Jumlah Hari *"},
			taxBase:  {"Nama Pajak *", "Kategori *", "Tarif (%) *"},
			fxBase:   {"Mata Uang Asal *", "Kurs *", "Berlaku Mulai *"},
			cbaBase:  {"Bank *", "No. Rekening *", "Mata Uang *"},
			accBase:  {"No. Akun *", "Nama Akun *", "Tipe Akun *", "No. Akun Induk"},
		} {
			x := must(t, call(t, "GET", base+"/template.xlsx", nil), 200)
			f, err := excelize.OpenReader(bytes.NewReader(x.Raw))
			if err != nil {
				t.Fatalf("%s: not a workbook: %v", base, err)
			}
			if s := f.GetSheetList(); len(s) < 2 || s[0] != "Data" || s[1] != "Petunjuk" {
				t.Errorf("%s sheets %v", base, s)
			}
			rows, _ := f.GetRows("Data")
			head := strings.ToLower(strings.Join(rows[0], "|"))
			for _, w := range want {
				if !strings.Contains(head, strings.ToLower(w)) {
					t.Errorf("%s header %q lacks %q", base, head, w)
				}
			}
		}
	})

	t.Run("FI02 Mata Uang: an ISO code or name already there is not added; a clean row is saved", func(t *testing.T) {
		head := []any{"Kode", "Kode ISO", "Nama Mata Uang", "Simbol", "Angka Desimal"}
		x := upload(t, curBase, true, head,
			[]any{"", "usd", "Dolar Lain " + sfx},
			[]any{"", "ZQA", "Mata Uang Uji " + sfx, "Q", 0},
			[]any{"", "ZQA", "Mata Uang Uji Lagi " + sfx},
			[]any{"", "ZQB", "Desimal Salah " + sfx, "", 7},
		)
		if x.Obj["committed"] != false || x.Obj["failed"] != 3.0 {
			t.Fatalf("%s %s", counts(x), trunc(x.Raw))
		}
		if p := rowProblems(x, 0); !strings.HasPrefix(p, "Kode ISO:") || !strings.Contains(p, "isi kolom Kode") {
			t.Errorf("stored ISO: %s", p)
		}
		if p := rowProblems(x, 2); !strings.Contains(p, "baris 3") {
			t.Errorf("same ISO in the file: %s", p)
		}
		if p := rowProblems(x, 3); !strings.Contains(p, "Angka Desimal") {
			t.Errorf("decimals: %s", p)
		}
		ok := upload(t, curBase, true, head, []any{"", "ZQA", "Mata Uang Uji " + sfx, "Q", 0})
		if ok.Obj["committed"] != true {
			t.Fatalf("%s", trunc(ok.Raw))
		}
		if g := must(t, call(t, "GET", curBase+"/"+rowCode(ok, 0), nil), 200); g.str("isoCode") != "ZQA" || g.Obj["decimals"] != 0.0 {
			t.Errorf("saved %s", trunc(g.Raw))
		}
	})

	t.Run("FI03 Bank: the clearing code keeps its leading zero; the country by ISO; SWIFT unique", func(t *testing.T) {
		head := []any{"Kode", "Nama Bank", "Kode Kliring", "Kode SWIFT/BIC", "Negara"}
		x := upload(t, bkBase, true, head,
			[]any{"", "Bank Uji " + sfx, "014", "ZQBKIDJA", "id"},
			[]any{"", "Bank Uji Dua " + sfx, "12", "zqbkidja", "Atlantis"},
			[]any{"", "Bank Uji Tiga " + sfx, "12"},
		)
		if p := rowProblems(x, 2); !strings.Contains(p, "Kode Kliring") {
			t.Errorf("clearing: %s", p)
		}
		if p := rowProblems(x, 1); !strings.Contains(p, "Negara") || !strings.Contains(p, "baris 2") {
			t.Errorf("bad row: %s", p)
		}
		ok := upload(t, bkBase, true, head, []any{"", "Bank Uji " + sfx, "014", "ZQBKIDJA", "ID"})
		if ok.Obj["committed"] != true {
			t.Fatalf("%s", trunc(ok.Raw))
		}
		if g := must(t, call(t, "GET", bkBase+"/"+rowCode(ok, 0), nil), 200); g.str("clearing") != "014" || g.str("swiftCode") != "ZQBKIDJA" || g.str("countryCode") == "" {
			t.Errorf("saved %s", trunc(g.Raw))
		}
	})

	t.Run("FI04 Syarat Pembayaran: choices by label; days over 365 rejected", func(t *testing.T) {
		head := []any{"Kode", "Nama Syarat", "Jumlah Hari", "Dihitung Dari", "Uang Muka (%)", "Berlaku Untuk"}
		x := upload(t, termBase, false, head,
			[]any{"", "Net 45 EOM " + sfx, 45, "Akhir bulan", 10, "Vendor"},
			[]any{"", "Terlalu Lama " + sfx, 400},
			[]any{"", "Dasar Salah " + sfx, 30, "Tanggal panen"},
		)
		if x.Obj["added"] != 1.0 || x.Obj["failed"] != 2.0 || !strings.Contains(rowProblems(x, 1), "Jumlah Hari") || !strings.Contains(rowProblems(x, 2), "Dihitung Dari") {
			t.Fatalf("%s %s", counts(x), trunc(x.Raw))
		}
		ok := upload(t, termBase, true, head, []any{"", "Net 45 EOM " + sfx, 45, "Akhir bulan", 10, "Vendor"})
		if g := must(t, call(t, "GET", termBase+"/"+rowCode(ok, 0), nil), 200); g.str("basis") != "end_of_month" || g.str("appliesTo") != "vendor" || g.Obj["days"] != 45.0 {
			t.Errorf("saved %s", trunc(g.Raw))
		}
	})

	t.Run("FI05 Jenis Pajak: method follows the category; PPN withheld is rejected; an Excel date is read", func(t *testing.T) {
		head := []any{"Kode", "Nama Pajak", "Kategori", "Tarif (%)", "Dasar Pengenaan (%)", "Cara", "Kode di Sistem Pajak", "Berlaku Mulai"}
		day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		x := upload(t, taxBase, false, head,
			[]any{"", "PPh 23 Uji " + sfx, "PPh 23", 2, "", "", "", day},
			[]any{"", "PPN Salah " + sfx, "PPN", 12, 91.6667, "Dipotong pembayar (PPh)"},
		)
		if x.Obj["added"] != 1.0 || !strings.Contains(rowProblems(x, 1), "Cara") {
			t.Fatalf("%s %s", counts(x), trunc(x.Raw))
		}
		ok := upload(t, taxBase, true, head, []any{"", "PPh 23 Uji " + sfx, "PPh 23", 2, "", "", "", day})
		if g := must(t, call(t, "GET", taxBase+"/"+rowCode(ok, 0), nil), 200); g.str("category") != "PPH23" || g.str("method") != "withheld" || g.str("validFrom") != "2026-01-01" {
			t.Errorf("saved %s", trunc(g.Raw))
		}
	})

	t.Run("FI06 Kurs: the date from a date cell or DD/MM/YYYY text, required; the same rate and date twice rejected", func(t *testing.T) {
		head := []any{"Kode", "Mata Uang Asal", "Mata Uang Tujuan", "Jenis Kurs", "Kurs", "Berlaku Mulai", "Berlaku Sampai", "Sumber"}
		x := upload(t, fxBase, true, head,
			[]any{"", "USD", "", "Kurs internal", 16123.5, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
			[]any{"", "USD", "IDR", "Kurs internal", "16200", "03/03/2026", "", "Uji"},
			[]any{"", "USD", "", "Kurs internal", 16300},
			[]any{"", "USD", "USD", "", 1, "2026-03-04"},
		)
		if n := x.Obj["rows"].([]any)[2].(map[string]any)["name"]; n != "USD/IDR · Kurs internal" {
			t.Errorf("a failed rate is named by its pair: %v", n)
		}
		if x.Obj["committed"] != false || x.Obj["failed"] != 2.0 || !strings.Contains(rowProblems(x, 2), "Berlaku Mulai") || !strings.Contains(rowProblems(x, 3), "Mata Uang Tujuan") {
			t.Fatalf("%s %s", counts(x), trunc(x.Raw))
		}
		ok := upload(t, fxBase, true, head,
			[]any{"", "USD", "", "Kurs internal", 16123.5, time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)},
			[]any{"", "USD", "IDR", "Kurs internal", "16200", "03/03/2026", "", "Uji"},
		)
		if ok.Obj["committed"] != true {
			t.Fatalf("%s", trunc(ok.Raw))
		}
		if g := must(t, call(t, "GET", fxBase+"/"+rowCode(ok, 1), nil), 200); g.str("validFrom") != "2026-03-03" || g.Obj["rate"] != 16200.0 || g.str("toIso") != "IDR" {
			t.Errorf("saved %s", trunc(g.Raw))
		}
		again := upload(t, fxBase, false, head, []any{"", "USD", "", "Kurs internal", 1, "2026-03-02"})
		if p := rowProblems(again, 0); !strings.Contains(p, "Berlaku Mulai") || !strings.Contains(p, "sudah ada") {
			t.Errorf("same rate and date: %s", p)
		}
	})

	t.Run("FI07 Rekening Bank Perusahaan: bank by name, the number keeps its leading zero, purpose by label", func(t *testing.T) {
		bank := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank Rekening " + sfx}), 201)
		head := []any{"Kode", "Nama Rekening", "Bank", "No. Rekening", "Nama Pemilik Rekening", "Mata Uang", "Cabang", "Perusahaan Pemilik", "Keperluan", "Rekening Utama Invoice"}
		x := upload(t, cbaBase, false, head,
			[]any{"", "Rek Uji " + sfx, "Bank Rekening " + sfx, "0098765432", "PT Uji", "IDR", "", "", "Penerimaan", "Tidak"},
			[]any{"", "Rek Gaji " + sfx, bank.str("code"), "1234567", "PT Uji", "IDR", "", "", "Gaji", "Ya"},
			[]any{"", "Rek Salah " + sfx, "Bank Tidak Ada", "12", "PT Uji", "XXX"},
		)
		if x.Obj["added"] != 1.0 || !strings.Contains(rowProblems(x, 1), "Rekening Utama Invoice") {
			t.Fatalf("%s %s", counts(x), trunc(x.Raw))
		}
		if p := rowProblems(x, 2); !strings.Contains(p, "Bank:") || !strings.Contains(p, "Mata Uang") {
			t.Errorf("bad refs: %s", p)
		}
		ok := upload(t, cbaBase, true, head, []any{"", "Rek Uji " + sfx, "Bank Rekening " + sfx, "0098765432", "PT Uji", "IDR", "", "", "Penerimaan", "Tidak"})
		if g := must(t, call(t, "GET", cbaBase+"/"+rowCode(ok, 0), nil), 200); g.str("accountNumber") != "0098765432" || g.str("purpose") != "receipt" || g.str("bankCode") != bank.str("code") {
			t.Errorf("saved %s", trunc(g.Raw))
		}
	})

	t.Run("FI08 Bagan Akun: a header and the accounts under it in one file; an account number already there not added", func(t *testing.T) {
		stored := account(t, map[string]any{"name": "Kas Kecil " + sfx, "accountNo": "1-" + sfx, "accountType": "cash_bank"})
		head := []any{"Kode", "No. Akun", "Nama Akun", "Tipe Akun", "Akun Induk", "No. Akun Induk", "Mata Uang"}
		h, d1, d2 := "6-"+sfx, "6-"+sfx+".1", "6-"+sfx+".2"
		x := upload(t, accBase, false, head,
			[]any{"", "1-" + sfx, "Kas Lain", "Kas & Bank"},
			[]any{"", h, "Beban Umum " + sfx, "Beban", "Ya"},
			[]any{"", d1, "Beban Listrik " + sfx, "Beban", "Tidak", h},
			[]any{"", d2, "Bank USD " + sfx, "Kas & Bank", "", h, "USD"},
		)
		if p := rowProblems(x, 0); !strings.HasPrefix(p, "No. Akun:") || !strings.Contains(p, stored.str("code")) {
			t.Errorf("stored number: %s", p)
		}
		if p := rowProblems(x, 2); p != "" {
			t.Errorf("child of a header from the file: %s", p)
		}
		if p := rowProblems(x, 3); !strings.Contains(p, "No. Akun Induk") {
			t.Errorf("parent of another type: %s", p)
		}
		ok := upload(t, accBase, true, head,
			[]any{"", h, "Beban Umum " + sfx, "Beban", "Ya"},
			[]any{"", d1, "Beban Listrik " + sfx, "Beban", "Tidak", h},
		)
		if ok.Obj["committed"] != true {
			t.Fatalf("%s", trunc(ok.Raw))
		}
		if g := must(t, call(t, "GET", accBase+"/"+rowCode(ok, 1), nil), 200); g.str("parentCode") != rowCode(ok, 0) || g.str("normalBalance") != "debit" {
			t.Errorf("saved %s", trunc(g.Raw))
		}
	})

	t.Run("FI09 stress: 2,000 rows (the limit) checked and saved in one go; 2,001 refused", func(t *testing.T) {
		head := []any{"Kode", "Nama Syarat", "Jumlah Hari", "Dihitung Dari"}
		rows := [][]any{head}
		for i := 0; i < 2000; i++ {
			rows = append(rows, []any{"", fmt.Sprintf("Beban %s-%04d", sfx, i), i % 366, "Tanggal invoice"})
		}
		start := time.Now()
		x := upload(t, termBase, true, rows...)
		took := time.Since(start)
		t.Logf("2000 rows saved in %v", took)
		if x.Obj["committed"] != true || x.Obj["added"] != 2000.0 {
			t.Fatalf("%s", counts(x))
		}
		if took > 30*time.Second {
			t.Errorf("too slow: %v", took)
		}
		more := append(rows, []any{"", "Satu Lagi " + sfx, 1})
		if r := call(t, "POST", termBase+"/upload", workbook(t, more)); !want(t, r, 422) || !strings.Contains(r.problem("file"), "2000") {
			t.Errorf("over the limit %s", trunc(r.Raw))
		}
	})
}
