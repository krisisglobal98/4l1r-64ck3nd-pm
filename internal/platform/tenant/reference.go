package tenant

import (
	"context"

	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
)

// The standard reference data, global (one set for every tenant, kept by the platform administrator): the currencies
// (ISO 4217, with their minor units) most used in Indonesian trade, every country of ISO 3166-1 (countries.go), and
// the Incoterms 2020.

type currencyRef struct {
	iso, name, symbol string
	decimals          int
}

var Currencies = []currencyRef{
	{"IDR", "Rupiah Indonesia", "Rp", 2}, {"USD", "Dolar Amerika Serikat", "$", 2}, {"EUR", "Euro", "€", 2},
	{"SGD", "Dolar Singapura", "S$", 2}, {"MYR", "Ringgit Malaysia", "RM", 2}, {"THB", "Baht Thailand", "฿", 2},
	{"VND", "Dong Vietnam", "₫", 0}, {"PHP", "Peso Filipina", "₱", 2}, {"BND", "Dolar Brunei", "B$", 2},
	{"KHR", "Riel Kamboja", "៛", 2}, {"MMK", "Kyat Myanmar", "K", 2}, {"LAK", "Kip Laos", "₭", 2},
	{"CNY", "Yuan Tiongkok", "¥", 2}, {"HKD", "Dolar Hong Kong", "HK$", 2}, {"TWD", "Dolar Taiwan Baru", "NT$", 2},
	{"JPY", "Yen Jepang", "¥", 0}, {"KRW", "Won Korea Selatan", "₩", 0}, {"INR", "Rupee India", "₹", 2},
	{"BDT", "Taka Bangladesh", "৳", 2}, {"PKR", "Rupee Pakistan", "Rs", 2}, {"LKR", "Rupee Sri Lanka", "Rs", 2},
	{"AUD", "Dolar Australia", "A$", 2}, {"NZD", "Dolar Selandia Baru", "NZ$", 2}, {"CAD", "Dolar Kanada", "C$", 2},
	{"MXN", "Peso Meksiko", "Mex$", 2}, {"BRL", "Real Brasil", "R$", 2}, {"GBP", "Pound Sterling", "£", 2},
	{"CHF", "Franc Swiss", "CHF", 2}, {"TRY", "Lira Turki", "₺", 2}, {"RUB", "Rubel Rusia", "₽", 2},
	{"AED", "Dirham Uni Emirat Arab", "AED", 2}, {"SAR", "Riyal Arab Saudi", "SAR", 2}, {"EGP", "Pound Mesir", "E£", 2},
	{"ZAR", "Rand Afrika Selatan", "R", 2}, {"NGN", "Naira Nigeria", "₦", 2}, {"KES", "Shilling Kenya", "KSh", 2},
}

type countryRef struct {
	iso2, iso3, phone, name, currency string
}

// seedGlobal makes sure the global reference masters hold the standard rows: the currencies, every country of ISO
// 3166-1 and the Incoterms 2020. They are one set for all tenants; a row is added only when no row (live or deleted)
// has its ISO code / term, so what the platform administrator removed or changed stays so. Idempotent and cheap: it
// runs as a tenant is prepared, one at a time (a global lock).
func seedGlobal(ctx context.Context, db *database.DB) error {
	if _, err := db.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('pm.global.seed'))`); err != nil {
		return err
	}
	have := func(sql string) (map[string]bool, error) {
		rows, err := db.Q(ctx).Query(ctx, sql)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := map[string]bool{}
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return nil, err
			}
			out[k] = true
		}
		return out, rows.Err()
	}
	cur, err := have(`SELECT upper(iso_code) FROM pm.currency`)
	if err != nil {
		return err
	}
	curCodes := codeprefix.Global{DB: db, Prefix: "MMU", Digits: 3}
	for _, c := range Currencies {
		if cur[c.iso] {
			continue
		}
		code, err := curCodes.Next(ctx, "currency")
		if err != nil {
			return err
		}
		if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.currency (code, name, iso_code, symbol, decimals, created_by) VALUES ($1, $2, $3, $4, $5, $6)`,
			code, c.name, c.iso, c.symbol, c.decimals, systemUser); err != nil {
			return err
		}
	}
	cty, err := have(`SELECT upper(iso2) FROM pm.country`)
	if err != nil {
		return err
	}
	ctyCodes := codeprefix.Global{DB: db, Prefix: "MNG", Digits: 3}
	for _, c := range Countries {
		if cty[c.iso2] {
			continue
		}
		code, err := ctyCodes.Next(ctx, "country")
		if err != nil {
			return err
		}
		if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.country (code, name, iso2, iso3, phone_code, currency_id, created_by)
			VALUES ($1, $2, $3, $4, $5, (SELECT id FROM pm.currency WHERE iso_code = nullif($6, '') AND NOT is_deleted), $7)`,
			code, c.name, c.iso2, c.iso3, c.phone, c.currency, systemUser); err != nil {
			return err
		}
	}
	inc, err := have(`SELECT term FROM pm.incoterm WHERE edition = 2020`)
	if err != nil {
		return err
	}
	incCodes := codeprefix.Global{DB: db, Prefix: "MIC", Digits: 3}
	for _, x := range Incoterms {
		if inc[x.Term] {
			continue
		}
		code, err := incCodes.Next(ctx, "incoterm")
		if err != nil {
			return err
		}
		if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.incoterm (code, name, term, edition, any_mode, seller_pays_carriage, seller_insures, seller_clears_import, risk_point, created_by)
			VALUES ($1, $2, $3, 2020, $4, $5, $6, $7, $8, $9)`, code, x.Name, x.Term, x.AnyMode, x.Carriage, x.Insures, x.Clear, x.Risk, systemUser); err != nil {
			return err
		}
	}
	return nil
}

// Logistics reference data (Stage FF-1C-1), seeded the same way: once, while the table is empty.

// Incoterms are the eleven rules of Incoterms 2020 (ICC): term, name, any mode (false: sea/inland waterway only), seller
// pays the main carriage, seller insures, seller clears import, where the risk passes.
var Incoterms = []struct {
	Term, Name                        string
	AnyMode, Carriage, Insures, Clear bool
	Risk                              string
}{
	{"EXW", "Ex Works", true, false, false, false, "Saat barang disediakan di tempat penjual"},
	{"FCA", "Free Carrier", true, false, false, false, "Saat barang diserahkan ke pengangkut yang ditunjuk pembeli"},
	{"CPT", "Carriage Paid To", true, true, false, false, "Saat barang diserahkan ke pengangkut pertama"},
	{"CIP", "Carriage and Insurance Paid To", true, true, true, false, "Saat barang diserahkan ke pengangkut pertama"},
	{"DAP", "Delivered at Place", true, true, false, false, "Saat barang siap dibongkar di tempat tujuan"},
	{"DPU", "Delivered at Place Unloaded", true, true, false, false, "Setelah barang dibongkar di tempat tujuan"},
	{"DDP", "Delivered Duty Paid", true, true, false, true, "Saat barang siap dibongkar di tempat tujuan, impor sudah diurus penjual"},
	{"FAS", "Free Alongside Ship", false, false, false, false, "Saat barang di samping kapal di pelabuhan muat"},
	{"FOB", "Free On Board", false, false, false, false, "Saat barang di atas kapal di pelabuhan muat"},
	{"CFR", "Cost and Freight", false, true, false, false, "Saat barang di atas kapal di pelabuhan muat"},
	{"CIF", "Cost, Insurance and Freight", false, true, true, false, "Saat barang di atas kapal di pelabuhan muat"},
}

// ContainerTypes are the common ISO 6346 size-types; 30 480 kg is the ISO maximum gross mass of 20' and 40' boxes.
var ContainerTypes = []struct {
	Iso, Name, Height, Group string
	Length                   int
	Teu                      float64
}{
	{"22G1", "20' General Purpose (20GP)", "standard", "general", 20, 1},
	{"42G1", "40' General Purpose (40GP)", "standard", "general", 40, 2},
	{"45G1", "40' High Cube (40HC)", "high_cube", "general", 40, 2},
	{"22R1", "20' Reefer (20RF)", "standard", "reefer", 20, 1},
	{"45R1", "40' High Cube Reefer (40RH)", "high_cube", "reefer", 40, 2},
	{"22U1", "20' Open Top (20OT)", "standard", "open_top", 20, 1},
	{"42U1", "40' Open Top (40OT)", "standard", "open_top", 40, 2},
	{"22P1", "20' Flat Rack (20FR)", "standard", "flat_rack", 20, 1},
	{"42P1", "40' Flat Rack (40FR)", "standard", "flat_rack", 40, 2},
	{"22T1", "20' Tank (20TK)", "standard", "tank", 20, 1},
}

// Packagings are common kinds of package with their UN/ECE Recommendation 21 code.
var Packagings = []struct{ Code, Name string }{
	{"BG", "Kantong (bag)"}, {"SA", "Karung (sack)"}, {"JB", "Jumbo bag"}, {"BX", "Kotak (box)"}, {"CT", "Karton"},
	{"CS", "Peti (case)"}, {"CR", "Krat (crate)"}, {"DR", "Drum"}, {"PX", "Palet"}, {"BE", "Bundel"}, {"PK", "Paket"},
	{"NE", "Tanpa kemasan"},
}

// Uoms are common units with their UN/ECE Recommendation 20 code and factor to the dimension's base unit.
var Uoms = []struct {
	Code, Name, Dimension string
	Factor                float64
}{
	{"KGM", "Kilogram", "mass", 1}, {"GRM", "Gram", "mass", 0.001}, {"TNE", "Ton (metrik)", "mass", 1000},
	{"MTQ", "Meter kubik", "volume", 1}, {"LTR", "Liter", "volume", 0.001},
	{"MTR", "Meter", "length", 1}, {"CMT", "Sentimeter", "length", 0.01}, {"MTK", "Meter persegi", "area", 1},
	{"H87", "Buah", "count", 1}, {"HUR", "Jam", "time", 1}, {"DAY", "Hari", "time", 24},
}

// seedLogistics adds the logistics reference rows to a tenant whose tables are still empty.
func seedLogistics(ctx context.Context, db *database.DB) error {
	codes := codeprefix.Postgres{DB: db}
	seed := func(table string, n int, insert func(i int, code string) error) error {
		var any bool
		if err := db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pm.`+table+`)`).Scan(&any); err != nil || any {
			return err
		}
		for i := range n {
			code, err := codes.Next(ctx, table)
			if err != nil {
				return err
			}
			if err := insert(i, code); err != nil {
				return err
			}
		}
		return nil
	}
	exec := func(sql string, args ...any) error {
		_, err := db.Q(ctx).Exec(ctx, sql, args...)
		return err
	}
	if err := seed("container_type", len(ContainerTypes), func(i int, code string) error {
		x := ContainerTypes[i]
		return exec(`INSERT INTO pm.container_type (code, name, iso_code, length_ft, height, type_group, teu, max_gross_kg, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 30480, $8)`, code, x.Name, x.Iso, x.Length, x.Height, x.Group, x.Teu, systemUser)
	}); err != nil {
		return err
	}
	if err := seed("packaging", len(Packagings), func(i int, code string) error {
		return exec(`INSERT INTO pm.packaging (code, name, unece_code, created_by) VALUES ($1, $2, $3, $4)`, code, Packagings[i].Name, Packagings[i].Code, systemUser)
	}); err != nil {
		return err
	}
	return seed("uom", len(Uoms), func(i int, code string) error {
		x := Uoms[i]
		return exec(`INSERT INTO pm.uom (code, name, unece_code, dimension, factor, created_by) VALUES ($1, $2, $3, $4, $5, $6)`, code, x.Name, x.Code, x.Dimension, x.Factor, systemUser)
	})
}

// Document and service reference data (Stage FF-1C-2), seeded once while the table is empty.

// DocumentTypes are common shipping, customs, commercial and certificate documents. Flags: export, import, domestic,
// number required, has expiry, original required.
var DocumentTypes = []struct {
	Name, Group, Customs, Issuer        string
	Exp, Imp, Dom, Number, Expiry, Orig bool
}{
	{"Bill of Lading (B/L)", "shipping", "", "carrier", true, true, false, true, false, true},
	{"House Bill of Lading (HBL)", "shipping", "", "forwarder", true, true, false, true, false, true},
	{"Sea Waybill", "shipping", "", "carrier", true, true, false, true, false, false},
	{"Air Waybill (AWB)", "shipping", "", "carrier", true, true, true, true, false, false},
	{"Delivery Order (DO)", "shipping", "", "carrier", false, true, true, true, true, false},
	{"Shipping Instruction (SI)", "shipping", "", "shipper", true, false, true, false, false, false},
	{"Packing List", "commercial", "", "shipper", true, true, true, false, false, false},
	{"Commercial Invoice", "commercial", "", "shipper", true, true, false, true, false, false},
	{"Pemberitahuan Impor Barang (PIB)", "customs", "BC 2.0", "customs", false, true, false, true, false, false},
	{"Pemberitahuan Ekspor Barang (PEB)", "customs", "BC 3.0", "customs", true, false, false, true, false, false},
	{"Manifes (Inward/Outward)", "customs", "BC 1.1", "carrier", true, true, false, true, false, false},
	{"Surat Persetujuan Pengeluaran Barang (SPPB)", "customs", "", "customs", false, true, false, true, false, false},
	{"Nota Pelayanan Ekspor (NPE)", "customs", "", "customs", true, false, false, true, false, false},
	{"Surat Keterangan Asal (SKA / COO)", "certificate", "", "government", true, true, false, true, false, true},
	{"Sertifikat Fitosanitari", "certificate", "", "government", true, true, false, true, true, true},
	{"Sertifikat Fumigasi", "certificate", "", "other", true, true, false, true, false, false},
	{"Polis Asuransi Kargo", "insurance", "", "other", true, true, true, true, true, false},
}

// ServiceTypes are common forwarding services: mode, load type, needs containers, includes customs.
var ServiceTypes = []struct {
	Name, Mode, Load   string
	Container, Customs bool
}{
	{"Sea Freight FCL", "sea", "fcl", true, false}, {"Sea Freight LCL", "sea", "lcl", false, false},
	{"Sea Freight Bulk", "sea", "bulk", false, false}, {"Sea Freight Breakbulk", "sea", "breakbulk", false, false},
	{"Air Freight", "air", "general", false, false}, {"Trucking FTL", "land", "ftl", false, false},
	{"Trucking LTL", "land", "ltl", false, false}, {"Door to Door", "multimodal", "general", false, true},
	{"Customs Clearance (PPJK)", "none", "none", false, true}, {"Warehousing", "none", "none", false, false},
}

// seedDocuments adds the document and service types to a tenant whose tables are still empty.
func seedDocuments(ctx context.Context, db *database.DB) error {
	codes := codeprefix.Postgres{DB: db}
	empty := func(table string) (bool, error) {
		var any bool
		err := db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pm.`+table+`)`).Scan(&any)
		return !any, err
	}
	if ok, err := empty("document_type"); err != nil {
		return err
	} else if ok {
		for _, d := range DocumentTypes {
			code, err := codes.Next(ctx, "document_type")
			if err != nil {
				return err
			}
			if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.document_type (code, name, doc_group, customs_code, issuer, for_export, for_import, for_domestic,
				number_required, has_expiry, original_required, created_by) VALUES ($1, $2, $3, nullif($4, ''), $5, $6, $7, $8, $9, $10, $11, $12)`,
				code, d.Name, d.Group, d.Customs, d.Issuer, d.Exp, d.Imp, d.Dom, d.Number, d.Expiry, d.Orig, systemUser); err != nil {
				return err
			}
		}
	}
	if ok, err := empty("service_type"); err != nil || !ok {
		return err
	}
	for _, s := range ServiceTypes {
		code, err := codes.Next(ctx, "service_type")
		if err != nil {
			return err
		}
		if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.service_type (code, name, mode, load_type, needs_container, includes_customs, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, code, s.Name, s.Mode, s.Load, s.Container, s.Customs, systemUser); err != nil {
			return err
		}
	}
	return nil
}

// Numberings are the standard number patterns a tenant starts with (Format Penomoran Dokumen), valid from 2026-01-01;
// the owning service keeps the counter.
var Numberings = []struct{ DocType, Name, Module, Pattern, Reset string }{
	{"FF.JOB", "Job Freight Forwarding", "freight-forwarding", "{SEQ:4}/{MM}/{YY}", "monthly"},
	{"FF.INV", "Invoice Freight Forwarding", "freight-forwarding", "INV/{SEQ:4}/{MM}/{YY}", "monthly"},
	{"FF.RMB", "Invoice Reimbursement Freight Forwarding", "freight-forwarding", "RMB/{SEQ:4}/{MM}/{YY}", "monthly"},
	{"FF.VB", "Vendor Bill Freight Forwarding", "freight-forwarding", "VB/{SEQ:4}/{MM}/{YY}", "monthly"},
	{"FF.RCV", "Penerimaan Pembayaran Freight Forwarding", "freight-forwarding", "RCV/{SEQ:4}/{MM}/{YY}", "monthly"},
	{"FF.PAY", "Pembayaran Vendor Freight Forwarding", "freight-forwarding", "PAY/{SEQ:4}/{MM}/{YY}", "monthly"},
}

// seedNumbering adds the standard patterns whose document type the tenant has none of yet.
func seedNumbering(ctx context.Context, db *database.DB) error {
	codes := codeprefix.Postgres{DB: db}
	for _, n := range Numberings {
		var exists bool
		if err := db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pm.doc_numbering WHERE doc_type = $1)`, n.DocType).Scan(&exists); err != nil || exists {
			if err != nil {
				return err
			}
			continue
		}
		code, err := codes.Next(ctx, "doc_numbering")
		if err != nil {
			return err
		}
		if _, err := db.Q(ctx).Exec(ctx, `INSERT INTO pm.doc_numbering (code, name, doc_type, module, pattern, reset, scope, start_number, valid_from, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, 'global', 1, DATE '2026-01-01', $7)`, code, n.Name, n.DocType, n.Module, n.Pattern, n.Reset, systemUser); err != nil {
			return err
		}
	}
	return nil
}
