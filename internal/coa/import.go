package coa

import (
	"context"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// importSpec is the Excel import of Bagan Akun: identified by the account number. Header accounts go above the
// accounts under them in the file, so a later row can name an earlier one as its parent.
func importSpec(db *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "accountNo", Column: "account_no"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MCA0001"),
			{Header: "No. Akun", Key: "accountNo", Text: true, Required: true, Hint: "Nomor akun perusahaan: huruf, angka, titik, atau tanda hubung. Tidak boleh sama dengan akun lain.", Example: "4-1100", Width: 12},
			{Header: "Nama Akun", Key: "name", Required: true, Hint: "Maks. 120 karakter.", Example: "Pendapatan Freight", Width: 28},
			{Header: "Tipe Akun", Key: "accountType", Required: true, Choices: master.Labels(Types, TypeLabels), Convert: master.ByLabel(TypeLabels), Hint: "Menentukan saldo normal (debit/kredit).", Example: "Pendapatan", Width: 22},
			{Header: "Akun Induk", Key: "header", Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: "Ya untuk akun pengelompok (header), tidak dipakai jurnal. Kosong berarti Tidak. Letakkan di atas akun-akun di bawahnya.", Example: "Tidak", Width: 10},
			{Header: "No. Akun Induk", Key: "parentCode", Convert: master.Lookup(db, "account", "Akun induk", "account_no", "code"), Hint: "No. akun header di atasnya, dengan tipe yang sama (sheet Akun Induk, atau baris di atasnya dalam file ini).", Example: "4-1000", Width: 14},
			{Header: "Mata Uang", Key: "currencyCode", Convert: master.Lookup(db, "currency", "Mata uang", "iso_code", "code"), Hint: "Hanya untuk akun valas, mis. Bank USD. Kosongkan untuk akun rupiah.", Example: "", Width: 10},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			h, err := master.ListRows(ctx, db, `SELECT account_no, name, account_type FROM pm.account WHERE is_active AND NOT is_deleted AND is_header ORDER BY account_no`, 3)
			if err != nil {
				return nil, err
			}
			for _, r := range h {
				r[2] = TypeLabels[r[2]]
			}
			c, err := master.ListRows(ctx, db, `SELECT iso_code, name, code FROM pm.currency WHERE is_active AND NOT is_deleted ORDER BY iso_code`, 3)
			if err != nil {
				return nil, err
			}
			return []master.ImportList{
				{Sheet: "Akun Induk", Header: []string{"No. Akun", "Nama Akun", "Tipe"}, Rows: h},
				{Sheet: "Mata Uang", Header: []string{"Kode ISO", "Nama", "Kode Master"}, Rows: c},
			}, nil
		},
	}
}
