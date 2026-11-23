package exchangerate

import (
	"context"
	"strings"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// TypeLabels name the rate types as the form and the Excel template show them.
var TypeLabels = map[string]string{"tax": "Kurs pajak (KMK)", "bi_middle": "Kurs tengah BI", "bank_buy": "Kurs beli bank", "bank_sell": "Kurs jual bank", "internal": "Kurs internal"}

// importSpec is the Excel import of Kurs: one row per rate (pair, type, date); the name is made by the system.
func importSpec(db *database.DB) *master.Import {
	cur := master.Lookup(db, "currency", "Mata uang", "iso_code", "code")
	return &master.Import{
		Describe: func(c map[string]string) string {
			to := c["toCurrencyCode"]
			if to == "" {
				to = "IDR"
			}
			parts := []string{strings.ToUpper(c["fromCurrencyCode"]) + "/" + strings.ToUpper(to)}
			for _, k := range []string{"rateType", "validFrom"} {
				if c[k] != "" {
					parts = append(parts, c[k])
				}
			}
			return strings.Join(parts, " · ")
		},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MKR000001"),
			{Header: "Mata Uang Asal", Key: "fromCurrencyCode", Required: true, Convert: cur, Hint: "Kode ISO (lihat sheet Mata Uang).", Example: "USD", Width: 12},
			{Header: "Mata Uang Tujuan", Key: "toCurrencyCode", Convert: cur, Hint: "Kosong berarti IDR.", Example: "IDR", Width: 12},
			{Header: "Jenis Kurs", Key: "rateType", Choices: master.Labels(Types, TypeLabels), Convert: master.ByLabel(TypeLabels), Hint: "Kosong berarti Kurs pajak (KMK).", Example: "Kurs pajak (KMK)", Width: 18},
			{Header: "Kurs", Key: "rate", Required: true, Convert: master.Number, Hint: "Nilai 1 mata uang asal dalam mata uang tujuan, lebih dari 0.", Example: "16250.50", Width: 14},
		}, master.ValidityColumns(true), []master.ImportColumn{
			{Header: "Sumber", Key: "source", Hint: "Mis. KMK-12/KM.10/2026 atau BI, maks. 100 karakter.", Example: "KMK", Width: 22},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			c, err := master.ListRows(ctx, db, `SELECT iso_code, name, code FROM pm.currency WHERE is_active AND NOT is_deleted ORDER BY iso_code`, 3)
			if err != nil {
				return nil, err
			}
			return []master.ImportList{{Sheet: "Mata Uang", Header: []string{"Kode ISO", "Nama", "Kode Master"}, Rows: c}}, nil
		},
	}
}
