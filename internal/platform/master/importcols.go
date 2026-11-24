package master

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"alir.local/pm-service/internal/platform/database"
)

// Building blocks of an Import: the columns every master shares and the cell converters.

// CodeColumn is the "Kode" column: empty for a new record, a stored code to change that record.
func CodeColumn(example string) ImportColumn {
	return ImportColumn{Header: "Kode", Key: "code", Example: example, Width: 12,
		Hint: "Kosongkan untuk data baru (kode dibuat sistem). Isi kode yang ada untuk mengubah data itu; hanya kolom yang diisi yang berubah."}
}

// NameColumn is the record's name.
func NameColumn(header, example string) ImportColumn {
	return ImportColumn{Header: header, Key: "name", Required: true, Hint: "Maks. 120 karakter. Nama yang sudah ada tidak ditambahkan lagi.", Example: example, Width: 28}
}

// ValidityColumns are "Berlaku Mulai" and "Berlaku Sampai".
func ValidityColumns(fromRequired bool) []ImportColumn {
	return []ImportColumn{
		{Header: "Berlaku Mulai", Key: "validFrom", Required: fromRequired, Date: true, Hint: "Tanggal (format tanggal Excel, atau teks YYYY-MM-DD / DD/MM/YYYY).", Example: "2026-01-01", Width: 14},
		{Header: "Berlaku Sampai", Key: "validTo", Date: true, Hint: "Kosongkan bila tanpa batas.", Width: 14},
	}
}

// TailColumns are "Deskripsi" and "Aktif", last in every template.
func TailColumns() []ImportColumn {
	return []ImportColumn{
		{Header: "Deskripsi", Key: "description", Hint: "Keterangan, maks. 500 karakter.", Width: 30},
		{Header: "Aktif", Key: "active", Choices: []string{"Ya", "Tidak"}, Convert: YesNo, Hint: "Kosong berarti Ya untuk data baru.", Example: "Ya", Width: 8},
	}
}

// Columns joins column groups in order.
func Columns(groups ...[]ImportColumn) []ImportColumn {
	var out []ImportColumn
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// Lookup resolves a cell to a master's code by one of its columns (any case), active records first; a problem when
// nothing matches. It runs inside the import's transaction, so it also finds records added by earlier rows.
func Lookup(db *database.DB, table, label string, cols ...string) func(context.Context, string) (any, string, error) {
	return func(ctx context.Context, s string) (any, string, error) {
		conds := make([]string, len(cols))
		for i, c := range cols {
			conds[i] = "lower(" + c + ") = lower($1)"
		}
		var code string
		err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.`+table+` WHERE NOT is_deleted AND (`+strings.Join(conds, " OR ")+`) ORDER BY is_active DESC, code LIMIT 1`, s).Scan(&code)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Sprintf("%s \"%s\" tidak ditemukan.", label, s), nil
		}
		return code, "", err
	}
}

// Number reads a number: no thousands separator needed ("1,500,000" and "1500000" both read), a point for decimals.
func Number(_ context.Context, s string) (any, string, error) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	if err != nil {
		return nil, "Isi angka, mis. 1500000 atau 125.5.", nil
	}
	return f, "", nil
}

// Integer reads a whole number.
func Integer(_ context.Context, s string) (any, string, error) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	if err != nil || f != float64(int64(f)) {
		return nil, "Isi bilangan bulat, mis. 30.", nil
	}
	return f, "", nil
}

var dateLayouts = []string{"2006-01-02", "02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "2006/01/02"}

// Date reads an Excel date (the cell's serial number) or a text date, YYYY-MM-DD or DD/MM/YYYY.
func Date(_ context.Context, s string) (any, string, error) {
	t := strings.TrimSpace(s)
	if f, err := strconv.ParseFloat(t, 64); err == nil && f > 0 && f < 2958466 {
		d, err := excelize.ExcelDateToTime(f, false)
		if err == nil {
			return d.Format("2006-01-02"), "", nil
		}
	}
	for _, l := range dateLayouts {
		if d, err := time.Parse(l, t); err == nil {
			return d.Format("2006-01-02"), "", nil
		}
	}
	return nil, "Isi tanggal, mis. 2026-01-31 atau 31/01/2026.", nil
}

// ListRows runs a query for a reference sheet: each row's n columns as text.
func ListRows(ctx context.Context, db *database.DB, sql string, n int) ([][]string, error) {
	rows, err := db.Q(ctx).Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := [][]string{}
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

// Labels lists the labels of codes, in the codes' order (the choices of a drop-down).
func Labels(codes []string, labels map[string]string) []string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = labels[c]
		if out[i] == "" {
			out[i] = c
		}
	}
	return out
}

// Multi reads several choices in one cell, separated by a comma, semicolon or slash, each by its code or label.
func Multi(labels map[string]string) func(context.Context, string) (any, string, error) {
	one := ByLabel(labels)
	return func(ctx context.Context, s string) (any, string, error) {
		var out []any
		for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '/' || r == '\n' }) {
			if strings.TrimSpace(part) == "" {
				continue
			}
			x, problem, err := one(ctx, part)
			if problem != "" || err != nil {
				return nil, problem, err
			}
			out = append(out, x)
		}
		if len(out) == 0 {
			return nil, "Isi minimal satu pilihan.", nil
		}
		return out, "", nil
	}
}
