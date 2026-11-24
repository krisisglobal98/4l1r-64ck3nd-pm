package salesperson

import (
	"context"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/master"
)

// importSpec is the Excel import of Tenaga Penjualan: each row points to a Karyawan (one sales person per employee).
func importSpec(db *database.DB) *master.Import {
	return &master.Import{
		Unique: []master.ImportUnique{{Key: "name", Column: "name"}},
		Columns: master.Columns([]master.ImportColumn{
			master.CodeColumn("MTJ001"),
			master.NameColumn("Nama Tenaga Penjualan", "Budi Santoso"),
			{Header: "Karyawan", Key: "employeeCode", Required: true, Convert: master.Lookup(db, "employee", "Karyawan", "code", "nik"), Hint: "Kode karyawan atau NIK (sheet Karyawan). Satu karyawan satu tenaga penjualan.", Example: "MKY0001", Width: 14},
			{Header: "Unit Penjualan", Key: "unitCode", Convert: master.Lookup(db, "org_unit", "Unit", "code", "name"), Hint: "Kode atau nama unit organisasi (sheet Unit).", Width: 16},
			{Header: "Email Kerja", Key: "email", Hint: "Untuk customer; kosong berarti email karyawan.", Width: 22},
			{Header: "Telepon", Key: "phone", Hint: "Angka, spasi, ( ) atau -, boleh diawali +.", Width: 16},
			{Header: "Komisi (%)", Key: "commissionPct", Convert: master.Number, Hint: "0–100.", Width: 10},
		}, master.TailColumns()),
		Lists: func(ctx context.Context) ([]master.ImportList, error) {
			e, err := master.ListRows(ctx, db, `SELECT code, nik, name FROM pm.employee WHERE is_active AND NOT is_deleted ORDER BY name`, 3)
			if err != nil {
				return nil, err
			}
			u, err := master.ListRows(ctx, db, `SELECT code, name FROM pm.org_unit WHERE is_active AND NOT is_deleted ORDER BY code`, 2)
			if err != nil {
				return nil, err
			}
			return []master.ImportList{{Sheet: "Karyawan", Header: []string{"Kode", "NIK", "Nama"}, Rows: e}, {Sheet: "Unit", Header: []string{"Kode", "Nama"}, Rows: u}}, nil
		},
	}
}
