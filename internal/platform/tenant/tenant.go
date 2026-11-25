// Package tenant prepares a company (tenant) on its first request: the built-in organisation levels, the code prefixes
// of every master, and the standard reference data (currencies, countries). Everything else a tenant fills itself.
package tenant

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/httpx"
)

const systemUser = "00000000-0000-7000-8000-000000000000"

// Levels are the built-in levels of every tenant (renamed per tenant, never deleted).
var Levels = []struct {
	Code, Name, Description string
	Rank                    int
}{
	{"group", "Group", "", 1}, {"region", "Region", "", 2}, {"wilayah", "Wilayah", "", 3}, {"company", "Perusahaan", "", 4},
	{"unit", "Unit Operasional", "", 5},
	{"org1", "Organisasi Level 1", "Di bawah Unit Operasional. Beri nama sesuai perusahaan, mis. Rayon.", 6},
	{"org2", "Organisasi Level 2", "Di bawah Organisasi Level 1. Mis. Afdeling.", 7},
	{"org3", "Organisasi Level 3", "Di bawah Organisasi Level 2. Mis. Blok.", 8},
}

// Prefixes are the generated-code prefixes of every master table.
var Prefixes = []struct {
	Table, Name, Prefix string
	Digits              int
}{
	{"org_level", "Level Organisasi", "ML", 3}, {"unit_type", "Jenis Unit Operasional", "MJU", 3}, {"org_unit", "Unit Organisasi", "MUO", 3},
	{"grade", "Grade Jabatan", "MGR", 3}, {"jabatan", "Jabatan", "MJB", 3}, {"position", "Posisi", "MPS", 3},
	{"approval_rule", "Aturan Approval", "MAR", 3}, {"delegation", "Delegasi", "MDL", 3}, {"custom_flow", "Approval Custom", "MAC", 3},
	{"employment_status", "Status Kepegawaian", "MSK", 3}, {"education_level", "Jenjang Pendidikan", "MJP", 3},
	{"work_shift", "Shift Kerja", "MSF", 3}, {"bank", "Bank", "MBK", 3}, {"employee", "Karyawan", "MKY", 4},
	{"city", "Kota", "MKT", 3},
	{"payment_term", "Syarat Pembayaran", "MTP", 3}, {"tax_type", "Jenis Pajak", "MPJ", 3},
	{"business_partner", "Mitra Bisnis", "MMB", 4}, {"sales_person", "Tenaga Penjualan", "MTJ", 3},
	{"location", "Pelabuhan & Lokasi", "MLK", 3}, {"container_type", "Jenis Kontainer", "MKN", 3},
	{"packaging", "Kemasan", "MKM", 3}, {"uom", "Satuan", "MST", 3}, {"commodity", "Komoditas", "MKD", 3},
	{"exchange_rate", "Kurs", "MKR", 6}, {"charge_code", "Kode Biaya", "MKB", 3}, {"company_bank_account", "Rekening Bank Perusahaan", "MRP", 3},
	{"document_type", "Jenis Dokumen", "MJD", 3}, {"service_type", "Moda & Layanan", "MML", 3},
	{"doc_numbering", "Format Penomoran Dokumen", "MPN", 3}, {"company_profile", "Profil Perusahaan", "MPF", 3},
	{"system_parameter", "System Parameter", "MSP", 3}, {"account", "Bagan Akun (COA)", "MCA", 4},
	{"building", "Gedung & Kantor", "MGD", 3}, {"room_amenity", "Fasilitas Ruang", "MFR", 3}, {"meeting_room", "Ruang Rapat", "MRR", 3},
}

// Provisioner prepares each tenant once per process.
type Provisioner struct {
	DB   *database.DB
	done sync.Map
}

// Ensure prepares the tenant of ctx's connection when it has no levels yet (idempotent, one at a time).
func (p *Provisioner) Ensure(ctx context.Context, tid string) error {
	if _, ok := p.done.Load(tid); ok {
		return nil
	}
	err := p.DB.InTx(ctx, func(ctx context.Context) error {
		if _, err := p.DB.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('pm.tenant.' || $1::text))`, tid); err != nil {
			return err
		}
		for _, l := range Levels {
			if _, err := p.DB.Q(ctx).Exec(ctx, `INSERT INTO pm.org_level (code, name, description, rank, created_by)
				SELECT $1::text, $2::text, nullif($3::text, ''), $4::int, $5::uuid WHERE NOT EXISTS (SELECT 1 FROM pm.org_level WHERE lower(code) = $1::text)`, l.Code, l.Name, l.Description, l.Rank, systemUser); err != nil {
				return err
			}
		}
		for _, x := range Prefixes {
			if _, err := p.DB.Q(ctx).Exec(ctx, `INSERT INTO pm.code_prefix (code, name, prefix, digits, created_by)
				SELECT $1::text, $2::text, $3::text, $4::int, $5::uuid WHERE NOT EXISTS (SELECT 1 FROM pm.code_prefix WHERE lower(code) = $1::text)`, x.Table, x.Name, x.Prefix, x.Digits, systemUser); err != nil {
				return err
			}
		}
		if err := seedGlobal(ctx, p.DB); err != nil {
			return err
		}
		if err := seedLogistics(ctx, p.DB); err != nil {
			return err
		}
		if err := seedDocuments(ctx, p.DB); err != nil {
			return err
		}
		return seedNumbering(ctx, p.DB)
	})
	if err == nil {
		p.done.Store(tid, true)
	}
	return err
}

// Middleware prepares the caller's tenant before the request (after the tenant connection is set).
func (p *Provisioner) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tid := access.TenantOf(r); tid != "" {
			if err := p.Ensure(r.Context(), tid); err != nil {
				slog.Error("tenant not prepared", "tenant", tid, "err", err)
				httpx.WriteError(w, r, err)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
