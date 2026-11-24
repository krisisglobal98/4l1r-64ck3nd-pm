package partner

import (
	"context"
	"strings"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/record"
)

// RoleLabels / EntityLabels name the choices as the form and the Excel template show them.
var (
	RoleLabels = map[string]string{"customer": "Customer", "vendor": "Vendor", "shipper": "Shipper", "consignee": "Consignee", "notify_party": "Notify party",
		"agent": "Agen luar negeri", "carrier": "Pelayaran / maskapai", "trucking": "Trucking", "customs_broker": "PPJK", "warehouse": "Gudang / depo"}
	EntityLabels = map[string]string{"pt": "PT", "cv": "CV", "firma": "Firma", "ud": "UD", "koperasi": "Koperasi", "yayasan": "Yayasan", "bumn": "BUMN / BUMD",
		"government": "Instansi pemerintah", "individual": "Perorangan", "foreign": "Badan usaha asing", "other": "Lainnya"}
)

// The template holds one row per partner: its own fields, and the PRIMARY address, contact and bank account as flat
// columns ("<list>.<field>"); Assemble puts them in the lists. A change by Kode replaces the filled cells of the primary
// row only; the other rows of a list stay.
var lists = []struct {
	key, prefix string
	fields      []string
	defaults    map[string]any
}{
	{"addresses", "address", []string{"address", "cityCode", "postalCode"}, map[string]any{"kind": "office"}},
	{"contacts", "contact", []string{"name", "jobTitle", "email", "phone"}, map[string]any{"purpose": "general"}},
	{"bankAccounts", "bank", []string{"bankCode", "accountNumber", "accountName", "currencyCode", "branch"}, nil},
}

func assemble(v, stored record.Values) record.Values {
	for _, l := range lists {
		cells := map[string]any{}
		for _, f := range l.fields {
			if x, ok := v[l.prefix+"."+f]; ok {
				cells[f] = x
				delete(v, l.prefix+"."+f)
			}
		}
		if len(cells) == 0 {
			continue
		}
		var rows []any
		if stored != nil {
			rows, _ = stored[l.key].([]any)
		}
		at := -1
		for i, r := range rows {
			if m, ok := r.(map[string]any); ok && m["primary"] == true {
				at = i
			}
		}
		if at < 0 && len(rows) > 0 {
			at = 0
		}
		row := map[string]any{"primary": true}
		for k, x := range l.defaults {
			row[k] = x
		}
		if at >= 0 {
			for k, x := range rows[at].(map[string]any) {
				row[k] = x
			}
		}
		for k, x := range cells {
			row[k] = x
		}
		out := append([]any{}, rows...)
		if at >= 0 {
			out[at] = row
		} else {
			out = append(out, row)
		}
		v[l.key] = out
	}
	return v
}

// importSpec is the Excel import of Mitra Bisnis.
func importSpec(db *database.DB) *master.Import {
	cur := master.Lookup(db, "currency", "Mata uang", "iso_code", "code")
	term := master.Lookup(db, "payment_term", "Syarat pembayaran", "code", "name")
	return &master.Import{
		Unique:   []master.ImportUnique{{Key: "name", Column: "name"}},
		Assemble: assemble,
		Alias: map[string]string{
			"addresses": "Alamat", "addresses.*.address": "Alamat", "addresses.*.cityCode": "Kota", "addresses.*.postalCode": "Kode Pos",
			"addresses.*.kind": "Alamat",
			"contacts":         "Nama Kontak", "contacts.*.name": "Nama Kontak", "contacts.*.jobTitle": "Jabatan Kontak", "contacts.*.email": "Email Kontak",
			"contacts.*.phone": "Telepon Kontak", "contacts.*.purpose": "Nama Kontak",
			"bankAccounts": "Bank", "bankAccounts.*.bankCode": "Bank", "bankAccounts.*.accountNumber": "No. Rekening",
			"bankAccounts.*.accountName": "Nama Pemilik Rekening", "bankAccounts.*.currencyCode": "Mata Uang Rekening", "bankAccounts.*.branch": "Cabang Bank",
		},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MMB0001"),
			master.NameColumn("Nama Mitra", "PT Contoh Ekspor Indonesia"),
			{Header: "Nama Dagang", Key: "tradeName", Hint: "Nama yang dikenal sehari-hari / singkatan, bila beda dari nama resmi.", Example: "CONTOH", Width: 18},
			{Header: "Bentuk Usaha", Key: "entityType", Required: true, Choices: master.Labels(EntityTypes, EntityLabels), Convert: master.ByLabel(EntityLabels), Hint: "Mitra luar negeri: Badan usaha asing.", Example: "PT", Width: 18},
			{Header: "Peran", Key: "roles", Required: true, Convert: master.Multi(RoleLabels), Hint: "Satu atau beberapa, dipisah koma: " + strings.Join(master.Labels(Roles, RoleLabels), ", ") + ". Customer wajib punya Syarat Pembayaran Customer, Mata Uang Tagihan, dan Alamat.", Example: "Customer, Shipper", Width: 24},
			{Header: "Negara", Key: "countryCode", Required: true, Convert: master.Lookup(db, "country", "Negara", "iso2", "iso3", "code", "name"), Hint: "Negara domisili: kode ISO 2 huruf (sheet Negara), mis. ID.", Example: "ID", Width: 10},
			{Header: "Email", Key: "email", Hint: "Email perusahaan.", Example: "info@contoh.co.id", Width: 22},
			{Header: "Telepon", Key: "phone", Hint: "Angka, spasi, ( ) atau -, boleh diawali +, maks. 30 karakter.", Example: "+62 21 555 0101", Width: 18},
			{Header: "Situs Web", Key: "website", Hint: "Diawali http:// atau https://.", Width: 20},
			{Header: "NIB", Key: "nib", Text: true, Hint: "13 angka (OSS).", Width: 16},
			{Header: "NPWP", Key: "npwp", Text: true, Hint: "16 angka; NPWP 15 angka lama diubah otomatis. Satu mitra per NPWP (cabang: isi NITKU).", Example: "0012345678701000", Width: 20},
			{Header: "NITKU", Key: "nitku", Text: true, Hint: "22 angka: NPWP lalu 6 angka tempat usaha.", Width: 24},
			{Header: "PKP", Key: "pkp", Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: "Ya bila Pengusaha Kena Pajak (wajib NPWP). Kosong berarti Tidak.", Example: "Tidak", Width: 8},
			{Header: "Nama Sesuai NPWP", Key: "taxName", Width: 24},
			{Header: "Alamat Sesuai NPWP", Key: "taxAddress", Width: 30},
			{Header: "No. Pajak Luar Negeri", Key: "foreignTaxId", Text: true, Hint: "TIN/VAT mitra luar negeri.", Width: 18},
			{Header: "Alamat", Key: "address.address", Hint: "Alamat utama (kantor), maks. 500 karakter. Wajib untuk Customer.", Example: "Jl. Contoh No. 1, Jakarta Utara", Width: 40},
			{Header: "Kota", Key: "address.cityCode", Convert: master.Lookup(db, "city", "Kota", "code", "name"), Hint: "Kode atau nama di master Kota (sheet Kota). Kosongkan bila belum ada; tulis kotanya di Alamat.", Width: 14},
			{Header: "Kode Pos", Key: "address.postalCode", Text: true, Hint: "3–10 huruf atau angka.", Example: "14240", Width: 10},
			{Header: "Nama Kontak", Key: "contact.name", Hint: "Kontak utama; isi juga email atau telepon kontak.", Example: "Budi Santoso", Width: 20},
			{Header: "Jabatan Kontak", Key: "contact.jobTitle", Width: 16},
			{Header: "Email Kontak", Key: "contact.email", Width: 22},
			{Header: "Telepon Kontak", Key: "contact.phone", Width: 18},
			{Header: "Syarat Pembayaran Customer", Key: "customerPaymentTermCode", Convert: term, Hint: "Kode atau nama (sheet Syarat Pembayaran). Untuk peran Customer / Agen.", Width: 18},
			{Header: "Mata Uang Tagihan", Key: "customerCurrencyCode", Convert: cur, Hint: "Kode ISO. Untuk peran Customer / Agen.", Example: "IDR", Width: 12},
			{Header: "Plafon Kredit", Key: "creditLimit", Convert: master.Number, Hint: "Dalam mata uang tagihan; kosong = tanpa plafon.", Width: 14},
			{Header: "Tenaga Penjualan", Key: "salesPersonCode", Convert: master.Lookup(db, "sales_person", "Tenaga penjualan", "code", "name"), Hint: "Kode atau nama (sheet Tenaga Penjualan).", Width: 16},
			{Header: "Syarat Pembayaran Vendor", Key: "vendorPaymentTermCode", Convert: term, Hint: "Untuk peran vendor, agen, pelayaran, trucking, PPJK, gudang.", Width: 18},
			{Header: "Mata Uang Pembayaran", Key: "vendorCurrencyCode", Convert: cur, Width: 12},
			{Header: "PPh Dipotong", Key: "vendorTaxTypeCode", Convert: master.Lookup(db, "tax_type", "Jenis pajak", "code", "name"), Hint: "Jenis pajak PPh (sheet Pajak), mis. PPh 23 atas jasa.", Width: 14},
			{Header: "Bank", Key: "bank.bankCode", Convert: master.Lookup(db, "bank", "Bank", "code", "name", "swift_code"), Hint: "Rekening utama mitra: kode, nama, atau SWIFT bank (sheet Bank).", Width: 16},
			{Header: "No. Rekening", Key: "bank.accountNumber", Text: true, Width: 18},
			{Header: "Nama Pemilik Rekening", Key: "bank.accountName", Width: 22},
			{Header: "Mata Uang Rekening", Key: "bank.currencyCode", Convert: cur, Width: 12},
			{Header: "Cabang Bank", Key: "bank.branch", Width: 16},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			out := []master.ImportList{}
			for _, l := range []struct {
				sheet  string
				header []string
				sql    string
			}{
				{"Negara", []string{"Kode ISO", "Nama", "Kode Master"}, `SELECT iso2, name, code FROM pm.country WHERE is_active AND NOT is_deleted ORDER BY name`},
				{"Kota", []string{"Kode", "Nama", "Negara"}, `SELECT c.code, c.name, n.iso2 FROM pm.city c LEFT JOIN pm.country n ON n.id = c.country_id WHERE c.is_active AND NOT c.is_deleted ORDER BY c.name`},
				{"Syarat Pembayaran", []string{"Kode", "Nama", "Berlaku Untuk"}, `SELECT code, name, applies_to FROM pm.payment_term WHERE is_active AND NOT is_deleted ORDER BY code`},
				{"Mata Uang", []string{"Kode ISO", "Nama", "Kode Master"}, `SELECT iso_code, name, code FROM pm.currency WHERE is_active AND NOT is_deleted ORDER BY iso_code`},
				{"Tenaga Penjualan", []string{"Kode", "Nama"}, `SELECT code, name FROM pm.sales_person WHERE is_active AND NOT is_deleted ORDER BY name`},
				{"Pajak", []string{"Kode", "Nama", "Kategori"}, `SELECT code, name, category FROM pm.tax_type WHERE is_active AND NOT is_deleted AND method = 'withheld' ORDER BY code`},
				{"Bank", []string{"Kode", "Nama", "SWIFT"}, `SELECT code, name, coalesce(swift_code, '') FROM pm.bank WHERE is_active AND NOT is_deleted ORDER BY name`},
			} {
				rows, err := master.ListRows(ctx, db, l.sql, len(l.header))
				if err != nil {
					return nil, err
				}
				out = append(out, master.ImportList{Sheet: l.sheet, Header: l.header, Rows: rows})
			}
			return out, nil
		},
	}
}
