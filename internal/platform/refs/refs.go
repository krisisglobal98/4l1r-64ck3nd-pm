// Package refs reads the rows one master refers to in another, and who still uses a row. Every master table has
// the standard columns, so one lookup serves them all; the table names are constants of this service, never input.
package refs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/record"
)

// ErrNotFound: no live (not deleted) row has the code.
var ErrNotFound = errors.New("not found")

// Ref is a live row of a master table as another one refers to it.
type Ref struct {
	ID, Code, Name string
	Active         bool
}

// Lookup finds the live row of table with code (any case) and locks it against change until the transaction ends,
// so it cannot be deleted under the record that is being saved.
func Lookup(ctx context.Context, db *database.DB, table, code string) (Ref, error) {
	var r Ref
	err := db.Q(ctx).QueryRow(ctx, `SELECT id, code, name, is_active FROM pm.`+table+
		` WHERE lower(code) = lower($1) AND NOT is_deleted FOR SHARE`, code).Scan(&r.ID, &r.Code, &r.Name, &r.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// CodeTaken reports whether any row of table, deleted ones included, has code (any case).
func CodeTaken(ctx context.Context, db *database.DB, table, code string) (bool, error) {
	var taken bool
	err := db.Q(ctx).QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pm.`+table+` WHERE lower(code) = lower($1))`, code).Scan(&taken)
	return taken, err
}

// user is one place a row can still be used: a count of live rows that point to it.
type user struct {
	what string // shown as "<n> <what>"
	sql  string // counts live users of $1
}

// usage lists, per master table, who points to its rows. A row in use is not deleted (deactivate it instead).
var usage = map[string][]user{
	"org_level": {
		{"unit", `SELECT count(*) FROM pm.org_unit WHERE org_level_id = $1 AND NOT is_deleted`},
		{"jabatan", `SELECT count(*) FROM pm.jabatan WHERE org_level_id = $1 AND NOT is_deleted`},
		{"aturan approval", `SELECT count(DISTINCT r.id) FROM pm.approval_rule r LEFT JOIN pm.approval_rule_band b ON b.approval_rule_id = r.id
			WHERE (r.reach_level_id = $1 OR b.org_level_id = $1) AND NOT r.is_deleted`},
	},
	"unit_type": {
		{"unit", `SELECT count(*) FROM pm.org_unit WHERE unit_type_id = $1 AND NOT is_deleted`},
	},
	"org_unit": {
		{"unit di bawahnya", `SELECT count(*) FROM pm.org_unit WHERE parent_id = $1 AND NOT is_deleted`},
		{"posisi", `SELECT count(*) FROM pm.position WHERE org_unit_id = $1 AND NOT is_deleted`},
		{"approval custom", `SELECT count(*) FROM pm.custom_flow WHERE scope_unit_id = $1 AND NOT is_deleted`},
		{"tenaga penjualan", `SELECT count(*) FROM pm.sales_person WHERE org_unit_id = $1 AND NOT is_deleted`},
	},
	"grade": {
		{"jabatan", `SELECT count(*) FROM pm.jabatan WHERE grade_id = $1 AND NOT is_deleted`},
	},
	"jabatan": {
		{"posisi", `SELECT count(*) FROM pm.position WHERE jabatan_id = $1 AND NOT is_deleted`},
		{"jenis unit", `SELECT count(*) FROM pm.unit_type WHERE head_jabatan_id = $1 AND NOT is_deleted`},
	},
	"position": {
		{"posisi yang melapor ke sini", `SELECT count(*) FROM pm.position WHERE reports_to_id = $1 AND NOT is_deleted`},
		{"delegasi", `SELECT count(*) FROM pm.delegation WHERE position_id = $1 AND NOT is_deleted`},
		{"approval custom", `SELECT count(DISTINCT f.id) FROM pm.custom_flow f JOIN pm.custom_flow_step s ON s.custom_flow_id = f.id
			WHERE s.position_id = $1 AND NOT f.is_deleted`},
	},
	"employment_status": {{"karyawan", `SELECT count(*) FROM pm.employee WHERE employment_status_id = $1 AND NOT is_deleted`}},
	"education_level":   {{"karyawan", `SELECT count(*) FROM pm.employee WHERE education_level_id = $1 AND NOT is_deleted`}},
	"work_shift":        {{"karyawan", `SELECT count(*) FROM pm.employee WHERE work_shift_id = $1 AND NOT is_deleted`}},
	"bank": {
		{"karyawan", `SELECT count(*) FROM pm.employee WHERE bank_id = $1 AND NOT is_deleted`},
		{"rekening mitra bisnis", `SELECT count(DISTINCT x.id) FROM pm.business_partner x JOIN pm.business_partner_bank_account a ON a.business_partner_id = x.id WHERE a.bank_id = $1 AND NOT x.is_deleted`},
	},
	"currency": {
		{"negara", `SELECT count(*) FROM pm.country WHERE currency_id = $1 AND NOT is_deleted`},
		{"mitra bisnis", `SELECT count(*) FROM pm.business_partner WHERE (customer_currency_id = $1 OR vendor_currency_id = $1) AND NOT is_deleted`},
		{"rekening mitra bisnis", `SELECT count(DISTINCT x.id) FROM pm.business_partner x JOIN pm.business_partner_bank_account a ON a.business_partner_id = x.id WHERE a.currency_id = $1 AND NOT x.is_deleted`},
	},
	"country": {
		{"kota", `SELECT count(*) FROM pm.city WHERE country_id = $1 AND NOT is_deleted`},
		{"bank", `SELECT count(*) FROM pm.bank WHERE country_id = $1 AND NOT is_deleted`},
		{"mitra bisnis", `SELECT count(*) FROM pm.business_partner WHERE country_id = $1 AND NOT is_deleted`},
	},
	"city":         {{"alamat mitra bisnis", `SELECT count(DISTINCT x.id) FROM pm.business_partner x JOIN pm.business_partner_address a ON a.business_partner_id = x.id WHERE a.city_id = $1 AND NOT x.is_deleted`}},
	"payment_term": {{"mitra bisnis", `SELECT count(*) FROM pm.business_partner WHERE (customer_payment_term_id = $1 OR vendor_payment_term_id = $1) AND NOT is_deleted`}},
	"tax_type":     {{"mitra bisnis", `SELECT count(*) FROM pm.business_partner WHERE vendor_tax_type_id = $1 AND NOT is_deleted`}},
	"sales_person": {{"mitra bisnis", `SELECT count(*) FROM pm.business_partner WHERE sales_person_id = $1 AND NOT is_deleted`}},
	"employee": {
		{"posisi (pemegang)", `SELECT count(*) FROM pm.position WHERE holder_employee_id = $1 AND NOT is_deleted`},
		{"posisi (Plt)", `SELECT count(*) FROM pm.position WHERE acting_employee_id = $1 AND NOT is_deleted`},
		{"delegasi", `SELECT count(*) FROM pm.delegation WHERE delegate_employee_id = $1 AND NOT is_deleted`},
		{"tenaga penjualan", `SELECT count(*) FROM pm.sales_person WHERE employee_id = $1 AND NOT is_deleted`},
	},
	"approval_rule": {
		{"delegasi", `SELECT count(*) FROM pm.delegation d JOIN pm.delegation_rule x ON x.delegation_id = d.id
			WHERE x.approval_rule_id = $1 AND NOT d.is_deleted`},
		{"approval custom", `SELECT count(*) FROM pm.custom_flow WHERE approval_rule_id = $1 AND NOT is_deleted`},
	},
}

// UsedBy says who still uses the row id of table, e.g. ["6 unit", "4 jabatan"]; empty when nothing does.
func UsedBy(ctx context.Context, db *database.DB, table, id string) ([]string, error) {
	var out []string
	for _, u := range usage[table] {
		var n int
		if err := db.Q(ctx).QueryRow(ctx, u.sql, id).Scan(&n); err != nil {
			return nil, fmt.Errorf("usage of %s by %s: %w", table, u.what, err)
		}
		if n > 0 {
			out = append(out, fmt.Sprintf("%d %s", n, u.what))
		}
	}
	return out, nil
}

// InUseMessage is the refusal of a delete: "<Noun> <name> masih dipakai 6 unit, 4 jabatan; …".
func InUseMessage(noun, name string, used []string) string {
	return fmt.Sprintf("%s %s masih dipakai %s; tidak dapat dihapus. Nonaktifkan saja.", noun, name, strings.Join(used, ", "))
}

// Pick resolves the relation in field to a live row of table by code. A missing row is a problem; so is an inactive
// one, unless it is the row the record already points to (keptID): old choices stay readable after deactivation.
// label names the master in messages ("Grade"); nil code is no relation. It answers nil when there is none.
func Pick(ctx context.Context, db *database.DB, table, field, label string, code, keptID *string, p *record.Problems) (*Ref, error) {
	if code == nil || p.Has(field) {
		return nil, nil
	}
	r, err := Lookup(ctx, db, table, *code)
	switch {
	case errors.Is(err, ErrNotFound):
		p.Add(field, fmt.Sprintf("%s \"%s\" tidak ditemukan.", label, *code))
		return nil, nil
	case err != nil:
		return nil, err
	case !r.Active && (keptID == nil || *keptID != r.ID):
		p.Add(field, fmt.Sprintf("%s %s nonaktif; pilih %s lain.", label, r.Name, strings.ToLower(label)))
		return nil, nil
	}
	return &r, nil
}

// Duplicate is the name of another live row of table whose column equals value (case-insensitive when ci), ""
// when none; selfID is the row being saved ("" on create). Table and column are constants of the caller.
func Duplicate(ctx context.Context, db *database.DB, table, column, value, selfID string, ci bool) (string, error) {
	cond := column + ` = $1`
	if ci {
		cond = `lower(` + column + `) = lower($1)`
	}
	var other string
	err := db.Q(ctx).QueryRow(ctx, `SELECT name FROM pm.`+table+` WHERE `+cond+` AND NOT is_deleted AND id::text <> $2`, value, selfID).Scan(&other)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return other, err
}
