package companyaccount

import (
	"context"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// PurposeLabels name the purposes as the form and the Excel template show them.
var PurposeLabels = map[string]string{"operational": "Operasional", "receipt": "Penerimaan", "payment": "Pembayaran", "payroll": "Gaji", "tax": "Pajak"}

// importSpec is the Excel import of Rekening Bank Perusahaan.
func importSpec(db *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MRP001"),
			master.NameColumn("Nama Rekening", "BCA Operasional IDR"),
			{Header: "Bank", Key: "bankCode", Required: true, Convert: master.Lookup(db, "bank", "Bank", "code", "name", "swift_code"), Hint: "Kode, nama, atau SWIFT bank (lihat sheet Bank).", Example: "MBK001", Width: 14},
			{Header: "No. Rekening", Key: "accountNumber", Text: true, Required: true, Hint: "4–34 angka/huruf, boleh dengan - (kolom berformat teks agar nol di depan tetap ada).", Example: "0123456789", Width: 18},
			{Header: "Nama Pemilik Rekening", Key: "accountName", Required: true, Hint: "Sesuai buku tabungan / rekening koran.", Example: "PT Contoh Logistik", Width: 26},
			{Header: "Mata Uang", Key: "currencyCode", Required: true, Convert: master.Lookup(db, "currency", "Mata uang", "iso_code", "code"), Hint: "Kode ISO (lihat sheet Mata Uang).", Example: "IDR", Width: 10},
			{Header: "Cabang", Key: "branch", Hint: "Maks. 80 karakter.", Example: "KCU Thamrin", Width: 18},
			{Header: "Perusahaan Pemilik", Key: "unitCode", Convert: master.Lookup(db, "org_unit", "Unit", "code", "name"), Hint: "Kode atau nama unit perusahaan (lihat sheet Perusahaan). Kosong berarti semua perusahaan.", Width: 18},
			{Header: "Keperluan", Key: "purpose", Choices: master.Labels(Purposes, PurposeLabels), Convert: master.ByLabel(PurposeLabels), Hint: "Kosong berarti Operasional.", Example: "Penerimaan", Width: 14},
			{Header: "Rekening Utama Invoice", Key: "invoiceDefault", Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: "Ya bila dicetak di invoice; satu per mata uang dan perusahaan. Tidak untuk rekening Gaji/Pajak.", Example: "Tidak", Width: 14},
			{Header: "Kode Akun", Key: "accountCode", Hint: "No. akun buku besar rekening ini (opsional), maks. 30 karakter.", Example: "1-1100", Width: 12},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			b, err := master.ListRows(ctx, db, `SELECT code, name, coalesce(swift_code, '') FROM pm.bank WHERE is_active AND NOT is_deleted ORDER BY name`, 3)
			if err != nil {
				return nil, err
			}
			c, err := master.ListRows(ctx, db, `SELECT iso_code, name, code FROM pm.currency WHERE is_active AND NOT is_deleted ORDER BY iso_code`, 3)
			if err != nil {
				return nil, err
			}
			u, err := master.ListRows(ctx, db, `SELECT code, name FROM pm.org_unit WHERE is_active AND NOT is_deleted ORDER BY code`, 2)
			if err != nil {
				return nil, err
			}
			return []master.ImportList{
				{Sheet: "Bank", Header: []string{"Kode", "Nama", "SWIFT"}, Rows: b},
				{Sheet: "Mata Uang", Header: []string{"Kode ISO", "Nama", "Kode Master"}, Rows: c},
				{Sheet: "Perusahaan", Header: []string{"Kode", "Nama"}, Rows: u},
			}, nil
		},
	}
}
