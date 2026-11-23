//go:build e2e

// Bagan Akun (COA, /pm/masters/accounts), the revenue and cost accounts of Kode Biaya, and the Excel import of Kode
// Biaya (template + upload: preview, then all-or-nothing save). Positive, negative.
package e2e

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

const accBase = "/pm/masters/accounts"

func account(t testing.TB, b map[string]any) resp {
	t.Helper()
	return must(t, call(t, "POST", accBase, b), 201)
}

// workbook is an .xlsx with a Data sheet of the rows (the first row is the header).
func workbook(t testing.TB, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Data")
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		row := r
		_ = f.SetSheetRow("Data", cell, &row)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestChartOfAccounts(t *testing.T) {
	t.Run("COA01 header and detail accounts of a type; normal balance follows the type", func(t *testing.T) {
		h := account(t, map[string]any{"name": "Pendapatan Jasa", "accountNo": "4-1000", "accountType": "revenue", "header": true})
		d := account(t, map[string]any{"name": "Pendapatan Freight", "accountNo": "4-1100", "accountType": "revenue", "parentCode": h.str("code")})
		if !genCodeOf("MCA").MatchString(d.str("code")) || d.str("normalBalance") != "credit" || d.str("parentCode") != h.str("code") {
			t.Errorf("detail %s", trunc(d.Raw))
		}
		if e := account(t, map[string]any{"name": "Beban Freight", "accountNo": "5-1100", "accountType": "cogs"}); e.str("normalBalance") != "debit" {
			t.Errorf("cogs %s", trunc(e.Raw))
		}
		for field, b := range map[string]map[string]any{
			"accountNo":   {"name": "x", "accountNo": "4-1100", "accountType": "revenue"},
			"parentCode":  {"name": "x", "accountNo": "5-9999", "accountType": "expense", "parentCode": h.str("code")},
			"accountType": {"name": "x", "accountNo": "9-1", "accountType": "laba"},
		} {
			if x := call(t, "POST", accBase, b); !want(t, x, 422) || x.problem(field) == "" {
				t.Errorf("%s: %s", field, trunc(x.Raw))
			}
		}
		if x := call(t, "POST", accBase, map[string]any{"name": "x", "accountNo": "4-1200", "accountType": "revenue", "parentCode": d.str("code")}); !want(t, x, 422) || !strings.Contains(x.problem("parentCode"), "bukan akun induk") {
			t.Errorf("parent not a header %s", trunc(x.Raw))
		}
		if x := call(t, "DELETE", accBase+"/"+h.str("code"), nil); !want(t, x, 409) {
			t.Errorf("header in use %s", trunc(x.Raw))
		}
	})

	t.Run("COA02 a charge's revenue and cost accounts: a detail account of the right type", func(t *testing.T) {
		rev := account(t, map[string]any{"name": "Pendapatan Trucking", "accountNo": "4-1300", "accountType": "revenue"})
		cost := account(t, map[string]any{"name": "Beban Trucking", "accountNo": "5-1300", "accountType": "cogs"})
		c := must(t, call(t, "POST", chBase, map[string]any{"name": "Trucking", "group": "trucking", "basis": "container", "revenueAccountCode": rev.str("code"), "costAccountCode": cost.str("code")}), 201)
		if c.str("revenueAccountCode") != rev.str("code") || c.str("costAccountCode") != cost.str("code") {
			t.Errorf("accounts %s", trunc(c.Raw))
		}
		if x := call(t, "POST", chBase, map[string]any{"name": "x", "group": "trucking", "basis": "container", "revenueAccountCode": cost.str("code")}); !want(t, x, 422) || x.problem("revenueAccountCode") == "" {
			t.Errorf("cost account as revenue %s", trunc(x.Raw))
		}
		u := must(t, call(t, "PUT", chBase+"/"+c.str("code"), map[string]any{"payable": false}), 200)
		if u.Obj["costAccountCode"] != nil || u.str("revenueAccountCode") != rev.str("code") {
			t.Errorf("no cost account once not payable %s", trunc(u.Raw))
		}
		if x := call(t, "DELETE", accBase+"/"+rev.str("code"), nil); !want(t, x, 409) || !strings.Contains(x.str("message"), "kode biaya") {
			t.Errorf("account in use %s", trunc(x.Raw))
		}
	})
}

func TestChargeImport(t *testing.T) {
	head := []any{"Kode", "Nama Biaya *", "Kelompok *", "Ditagihkan ke Customer *", "Dibayar ke Vendor *", "Dasar Tarif *", "Mata Uang", "Tarif Dasar", "Kode PPN", "Kode PPh", "No. Akun Pendapatan", "No. Akun Beban", "Deskripsi", "Aktif"}

	t.Run("CI01 the template: Data, Petunjuk and the reference sheets", func(t *testing.T) {
		x := must(t, call(t, "GET", chBase+"/template.xlsx", nil), 200)
		f, err := excelize.OpenReader(bytes.NewReader(x.Raw))
		if err != nil {
			t.Fatalf("not a workbook: %v", err)
		}
		if got := strings.Join(f.GetSheetList(), ","); got != "Data,Petunjuk,Mata Uang,Pajak,Akun" {
			t.Errorf("sheets %s", got)
		}
		row, _ := f.GetRows("Data")
		if len(row) == 0 || row[0][1] != "Nama Biaya *" || row[0][2] != "Kelompok *" || len(row[0]) != len(head) {
			t.Errorf("header %v", row)
		}
	})

	t.Run("CI02 preview saves nothing; one bad row stops the save; a clean file is saved", func(t *testing.T) {
		account(t, map[string]any{"name": "Pendapatan Dokumen", "accountNo": "4-1400", "accountType": "revenue"})
		before := len(must(t, call(t, "GET", chBase, nil), 200).List)
		bad := workbook(t, [][]any{head,
			{"", "Biaya B/L", "Dokumen", "Ya", "Tidak", "Per dokumen", "IDR", "750000", "", "", "4-1400", "", "Dokumen B/L", "Ya"},
			{"", "Tanpa Kelompok", "", "Ya", "Ya", "Per pengiriman"},
			{"", "Akun Salah", "Lainnya", "Ya", "Tidak", "Per pengiriman", "", "", "", "", "9-9999"},
		})
		p := must(t, call(t, "POST", chBase+"/upload", bad), 200)
		rows := p.Obj["rows"].([]any)
		if p.Obj["committed"] != false || p.Obj["added"] != 1.0 || p.Obj["failed"] != 2.0 || len(rows) != 3 {
			t.Fatalf("preview %s", trunc(p.Raw))
		}
		r2 := rows[1].(map[string]any)["problems"].([]any)
		r3 := rows[2].(map[string]any)["problems"].([]any)
		if len(r2) == 0 || r2[0].(map[string]any)["field"] != "Kelompok" || len(r3) == 0 || r3[0].(map[string]any)["field"] != "No. Akun Pendapatan" {
			t.Errorf("row problems %v / %v", r2, r3)
		}
		if c := must(t, call(t, "POST", chBase+"/upload?commit=true", bad), 200); c.Obj["committed"] != false {
			t.Errorf("a file with errors is not saved: %s", trunc(c.Raw))
		}
		if n := len(must(t, call(t, "GET", chBase, nil), 200).List); n != before {
			t.Errorf("nothing saved: %d → %d", before, n)
		}
		good := workbook(t, [][]any{head,
			{"", "Biaya B/L", "Dokumen", "Ya", "Tidak", "Per dokumen", "IDR", "750000", "", "", "4-1400", "", "Dokumen B/L", "Ya"},
			{"", "Lift On", "origin", "y", "n", "container"},
		})
		c := must(t, call(t, "POST", chBase+"/upload?commit=true", good), 200)
		if c.Obj["committed"] != true || c.Obj["added"] != 2.0 {
			t.Fatalf("commit %s", trunc(c.Raw))
		}
		code := c.Obj["rows"].([]any)[0].(map[string]any)["code"].(string)
		upd := workbook(t, [][]any{head, {code, "", "", "", "", "", "", "800000"}})
		if u := must(t, call(t, "POST", chBase+"/upload?commit=true", upd), 200); u.Obj["updated"] != 1.0 || u.Obj["committed"] != true {
			t.Errorf("update by code %s", trunc(u.Raw))
		}
		if g := must(t, call(t, "GET", chBase+"/"+code, nil), 200); g.Obj["defaultRate"] != 800000.0 || g.str("name") != "Biaya B/L" {
			t.Errorf("only the filled cell changes %s", trunc(g.Raw))
		}
	})

	t.Run("CI04 a name already there is not added: the preview names its code", func(t *testing.T) {
		list := must(t, call(t, "GET", chBase, nil), 200).List
		codeOf := map[string]string{}
		for _, x := range list {
			codeOf[x["name"].(string)] = x["code"].(string)
		}
		lift, bl := codeOf["Lift On"], codeOf["Biaya B/L"]
		if lift == "" || bl == "" {
			t.Fatalf("CI02 data missing: %v", codeOf)
		}
		problem := func(r map[string]any) string {
			ps := r["problems"].([]any)
			if len(ps) == 0 {
				return ""
			}
			p := ps[0].(map[string]any)
			return p["field"].(string) + ": " + p["message"].(string)
		}
		f := workbook(t, [][]any{head,
			{"", " lift on ", "origin", "Ya", "Ya", "container"}, // stored name, other case → not added
			{"", "Biaya Baru", "Lainnya", "Ya", "Tidak", "shipment"},
			{"", "BIAYA BARU", "Lainnya", "Ya", "Tidak", "shipment"}, // same as the row above
			{bl, "Lift On"}, // renaming onto another record
			{lift, "Lift On", "", "", "", "", "IDR", "1250"}, // its own name: fine
		})
		x := must(t, call(t, "POST", chBase+"/upload?commit=true", f), 200)
		rows := x.Obj["rows"].([]any)
		if x.Obj["committed"] != false || x.Obj["failed"] != 3.0 || len(rows) != 5 {
			t.Fatalf("result %s", trunc(x.Raw))
		}
		r := func(i int) map[string]any { return rows[i].(map[string]any) }
		if p := problem(r(0)); !strings.HasPrefix(p, "Nama Biaya:") || !strings.Contains(p, lift) || !strings.Contains(p, "isi kolom Kode") {
			t.Errorf("stored name: %q", p)
		}
		if p := problem(r(1)); p != "" {
			t.Errorf("new name: %q", p)
		}
		if p := problem(r(2)); !strings.Contains(p, "baris 3") {
			t.Errorf("same name in the file: %q", p)
		}
		if p := problem(r(3)); !strings.Contains(p, "sudah dipakai") || !strings.Contains(p, lift) {
			t.Errorf("rename onto another: %q", p)
		}
		if p := problem(r(4)); p != "" {
			t.Errorf("own name: %q", p)
		}
		if n := len(must(t, call(t, "GET", chBase, nil), 200).List); n != len(list) {
			t.Errorf("nothing saved: %d → %d", len(list), n)
		}
		// The user's choice: fill Kode to change the stored record with the file's row.
		fix := workbook(t, [][]any{head, {lift, "Lift On", "origin", "Ya", "Ya", "container", "IDR", "1300"}})
		if c := must(t, call(t, "POST", chBase+"/upload?commit=true", fix), 200); c.Obj["committed"] != true || c.Obj["updated"] != 1.0 {
			t.Errorf("update by code %s", trunc(c.Raw))
		}
		if g := must(t, call(t, "GET", chBase+"/"+lift, nil), 200); g.Obj["defaultRate"] != 1300.0 {
			t.Errorf("rate %s", trunc(g.Raw))
		}
	})

	t.Run("CI05 a Kode that does not exist is a row problem, not a server error", func(t *testing.T) {
		f := workbook(t, [][]any{head, {"MKB999", "Biaya Tidak Ada"}, {"", "Biaya Lain", "Lainnya", "Ya", "Tidak", "shipment"}})
		x := must(t, call(t, "POST", chBase+"/upload", f), 200)
		rows := x.Obj["rows"].([]any)
		if x.Obj["failed"] != 1.0 || x.Obj["added"] != 1.0 || len(rows) != 2 {
			t.Fatalf("result %s", trunc(x.Raw))
		}
		p := rows[0].(map[string]any)["problems"].([]any)[0].(map[string]any)
		if p["field"] != "Kode" || !strings.Contains(p["message"].(string), "MKB999 tidak ditemukan") {
			t.Errorf("problem %v", p)
		}
	})

	t.Run("CI03 not a workbook, a missing required column", func(t *testing.T) {
		if x := call(t, "POST", chBase+"/upload", "nama,kelompok\nx,y"); !want(t, x, 422) || x.problem("file") == "" {
			t.Errorf("csv %s", trunc(x.Raw))
		}
		if x := call(t, "POST", chBase+"/upload", workbook(t, [][]any{{"Nama Biaya"}, {"x"}})); !want(t, x, 422) || !strings.Contains(x.problem("file"), "Kelompok") {
			t.Errorf("missing column %s", trunc(x.Raw))
		}
	})
}
