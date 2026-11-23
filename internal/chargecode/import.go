package chargecode

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/coa"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// GroupLabels / BasisLabels name the choices as the form and the Excel template show them.
var (
	GroupLabels = map[string]string{"freight": "Freight", "origin": "Biaya asal (origin)", "destination": "Biaya tujuan (destination)", "customs": "Kepabeanan",
		"trucking": "Trucking", "warehouse": "Gudang", "document": "Dokumen", "insurance": "Asuransi", "surcharge": "Surcharge", "other": "Lainnya"}
	BasisLabels = map[string]string{"shipment": "Per pengiriman", "container": "Per kontainer", "teu": "Per TEU", "kg": "Per kg", "ton": "Per ton",
		"cbm": "Per CBM", "wm": "Per W/M", "document": "Per dokumen", "day": "Per hari", "percent": "Persen (%)"}
)

func labels(codes []string, m map[string]string) []string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = m[c]
	}
	return out
}

// lookup resolves a cell to a master code by one of its columns (any case); a problem when nothing matches.
func lookup(db *database.DB, table, label string, cols ...string) func(context.Context, string) (any, string, error) {
	return func(ctx context.Context, s string) (any, string, error) {
		conds := make([]string, len(cols))
		for i, c := range cols {
			conds[i] = "lower(" + c + ") = lower($1)"
		}
		var code string
		err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.`+table+` WHERE NOT is_deleted AND (`+strings.Join(conds, " OR ")+`) ORDER BY is_active DESC LIMIT 1`, s).Scan(&code)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Sprintf("%s \"%s\" tidak ditemukan.", label, s), nil
		}
		return code, "", err
	}
}

func number(_ context.Context, s string) (any, string, error) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	if err != nil {
		return nil, "Isi angka, mis. 1500000 atau 125.5.", nil
	}
	return f, "", nil
}

// importSpec is the Excel import of Kode Biaya: the columns of the template and the reference sheets.
func importSpec(db *database.DB) *master.Import {
	return &master.Import{
		Columns: []master.ImportColumn{
			{Header: "Kode", Key: "code", Hint: "Kosongkan untuk kode biaya baru (kode dibuat sistem). Isi kode yang ada untuk mengubahnya.", Example: "MKB001", Width: 12},
			{Header: "Nama Biaya", Key: "name", Required: true, Hint: "Nama jenis biaya, maks. 80 karakter.", Example: "Ocean Freight", Width: 28},
			{Header: "Kelompok", Key: "group", Required: true, Choices: labels(Groups, GroupLabels), Convert: master.ByLabel(GroupLabels), Example: "Freight", Width: 22},
			{Header: "Ditagihkan ke Customer", Key: "receivable", Required: true, Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: "Ya bila biaya ini dijual / ditagih ke customer (receivable).", Example: "Ya", Width: 14},
			{Header: "Dibayar ke Vendor", Key: "payable", Required: true, Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: "Ya bila biaya ini dibeli / dibayar ke vendor (payable). Minimal satu dari dua kolom ini Ya.", Example: "Ya", Width: 14},
			{Header: "Dasar Tarif", Key: "basis", Required: true, Choices: labels(Bases, BasisLabels), Convert: master.ByLabel(BasisLabels), Example: "Per kontainer", Width: 18},
			{Header: "Mata Uang", Key: "currencyCode", Convert: lookup(db, "currency", "Mata uang", "iso_code", "code"), Hint: "Kode ISO mata uang tarif dasar (lihat sheet Mata Uang).", Example: "USD", Width: 10},
			{Header: "Tarif Dasar", Key: "defaultRate", Convert: number, Hint: "Angka tanpa pemisah ribuan; titik untuk desimal. Untuk dasar Persen: 0–100.", Example: "1450", Width: 14},
			{Header: "Kode PPN", Key: "ppnTaxTypeCode", Convert: lookup(db, "tax_type", "Jenis pajak", "code", "name"), Hint: "PPN bawaan saat ditagih ke customer (sheet Pajak). Hanya untuk biaya yang ditagihkan.", Example: "MPJ001", Width: 12},
			{Header: "Kode PPh", Key: "pphTaxTypeCode", Convert: lookup(db, "tax_type", "Jenis pajak", "code", "name"), Hint: "PPh yang dipotong saat dibayar ke vendor (sheet Pajak). Hanya untuk biaya yang dibayar ke vendor.", Example: "MPJ003", Width: 12},
			{Header: "No. Akun Pendapatan", Key: "revenueAccountCode", Convert: lookup(db, "account", "Akun", "account_no", "code"), Hint: "No. akun pendapatan di Bagan Akun (sheet Akun), tipe Pendapatan atau Pendapatan Lain-lain, bukan akun induk.", Example: "4-1100", Width: 16},
			{Header: "No. Akun Beban", Key: "costAccountCode", Convert: lookup(db, "account", "Akun", "account_no", "code"), Hint: "No. akun beban / HPP di Bagan Akun (sheet Akun), tipe HPP, Beban, atau Beban Lain-lain, bukan akun induk.", Example: "5-1100", Width: 16},
			{Header: "Deskripsi", Key: "description", Hint: "Keterangan, maks. 500 karakter.", Width: 30},
			{Header: "Aktif", Key: "active", Choices: []string{"Ya", "Tidak"}, Convert: master.YesNo, Hint: "Kosong berarti Ya untuk data baru.", Example: "Ya", Width: 8},
		},
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			q := func(sql string, n int) ([][]string, error) {
				rows, err := db.Q(ctx).Query(ctx, sql)
				if err != nil {
					return nil, err
				}
				defer rows.Close()
				var out [][]string
				for rows.Next() {
					vals := make([]any, n)
					ptrs := make([]any, n)
					for i := range vals {
						ptrs[i] = &vals[i]
					}
					if err := rows.Scan(ptrs...); err != nil {
						return nil, err
					}
					row := make([]string, n)
					for i, v := range vals {
						if v != nil {
							row[i] = fmt.Sprint(v)
						}
					}
					out = append(out, row)
				}
				return out, rows.Err()
			}
			cur, err := q(`SELECT iso_code, name, code FROM pm.currency WHERE is_active AND NOT is_deleted ORDER BY iso_code`, 3)
			if err != nil {
				return nil, err
			}
			tax, err := q(`SELECT code, name, category, CASE method WHEN 'added' THEN 'Ditambahkan (PPN)' ELSE 'Dipotong (PPh)' END FROM pm.tax_type WHERE is_active AND NOT is_deleted ORDER BY category, code`, 4)
			if err != nil {
				return nil, err
			}
			acc, err := q(`SELECT account_no, name, account_type FROM pm.account WHERE is_active AND NOT is_deleted AND NOT is_header
				AND account_type IN ('revenue', 'other_income', 'cogs', 'expense', 'other_expense') ORDER BY account_no`, 3)
			if err != nil {
				return nil, err
			}
			for _, r := range acc {
				r[2] = coa.TypeLabels[r[2]]
			}
			return []master.ImportList{
				{Sheet: "Mata Uang", Header: []string{"Kode ISO", "Nama", "Kode Master"}, Rows: cur},
				{Sheet: "Pajak", Header: []string{"Kode", "Nama", "Kategori", "Cara"}, Rows: tax},
				{Sheet: "Akun", Header: []string{"No. Akun", "Nama Akun", "Tipe"}, Rows: acc},
			}, nil
		},
	}
}
