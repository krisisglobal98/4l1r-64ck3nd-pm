// Package record holds the standard columns every master table starts with (id, master data, status, workflow,
// validity, audit, version), their checks, and the SQL that reads them. A master embeds Base and adds the fields
// of its own page after it, in the table and in the JSON alike.
package record

import (
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Base is the standard part of a master record. Deleted rows are never answered, so the delete columns are not here.
type Base struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description"`

	Active bool `json:"active"`

	// Workflow, set by the approval flow, never by the form. State = where the record is in its approval;
	// Status = its outcome (approved, void, cancelled, posted, …).
	State                 *string    `json:"state"`
	Status                *string    `json:"status"`
	Remark                *string    `json:"remark"`
	ApprovalLevelRequired *int       `json:"approvalLevelRequired"`
	ApprovedAt            *time.Time `json:"approvedAt"`
	ApprovedBy            *string    `json:"approvedBy"`

	// Business dates YYYY-MM-DD (no time zone), inclusive.
	ValidFrom *string `json:"validFrom"`
	ValidTo   *string `json:"validTo"`

	// Instants in UTC; user ids from the Authentication service.
	CreatedAt time.Time  `json:"createdAt"`
	CreatedBy string     `json:"createdBy"`
	UpdatedAt *time.Time `json:"updatedAt"`
	UpdatedBy *string    `json:"updatedBy"`
	Version   int        `json:"version"`
}

// Columns lists the standard columns in the order Targets scans them; append the master's own columns after it.
var Columns = ColumnsAs("")

// ColumnsAs is Columns of the table with alias (for queries that join other tables), e.g. ColumnsAs("u").
func ColumnsAs(alias string) string {
	if alias != "" {
		alias += "."
	}
	cols := []string{"id", "code", "name", "description", "is_active", "state", "status", "remark",
		"approval_level_required", "approved_at", "approved_by", "to_char(%svalid_from, 'YYYY-MM-DD')",
		"to_char(%svalid_to, 'YYYY-MM-DD')", "created_at", "created_by", "updated_at", "updated_by", "version"}
	for i, c := range cols {
		if strings.Contains(c, "%s") {
			cols[i] = fmt.Sprintf(c, alias)
		} else {
			cols[i] = alias + c
		}
	}
	return strings.Join(cols, ", ")
}

// Targets are the scan destinations of Columns.
func (b *Base) Targets() []any {
	return []any{&b.ID, &b.Code, &b.Name, &b.Description, &b.Active, &b.State, &b.Status, &b.Remark,
		&b.ApprovalLevelRequired, &b.ApprovedAt, &b.ApprovedBy, &b.ValidFrom, &b.ValidTo, &b.CreatedAt, &b.CreatedBy,
		&b.UpdatedAt, &b.UpdatedBy, &b.Version}
}

// Scan reads Columns followed by the master's own columns, and puts every instant in UTC.
func (b *Base) Scan(row pgx.Row, own ...any) error {
	if err := row.Scan(append(b.Targets(), own...)...); err != nil {
		return err
	}
	b.CreatedAt = b.CreatedAt.UTC()
	for _, t := range []*time.Time{b.UpdatedAt, b.ApprovedAt} {
		if t != nil {
			*t = t.UTC()
		}
	}
	return nil
}

// Limits of the standard columns (the DDL of every master table).
const (
	MaxCode        = 40
	MaxName        = 150
	MaxDescription = 500
)

// LookupKey says whether code can name a record at all (a code in a URL); anything else is simply not found.
func LookupKey(code string) bool { return code != "" && len(code) <= 4*MaxCode && Clean(code) }

// Input is the part of the form every master shares: what a user may set of the standard columns, and the code
// the service generated.
type Input struct {
	Code        string
	Name        string
	Description *string
	Active      bool
	ValidFrom   *string
	ValidTo     *string
}

// ValuesOf is a stored record as form values, the base a partial update is merged onto.
func ValuesOf(b Base) Values {
	return Values{"code": b.Code, "name": b.Name, "description": deref(b.Description), "active": b.Active,
		"validFrom": deref(b.ValidFrom), "validTo": deref(b.ValidTo)}
}

// ParseInput checks the standard fields of v that a user may set and adds a problem per field that needs
// correcting. The code is not one of them: the service generates it (see codeprefix) and it never changes.
func ParseInput(v Values, p *Problems) Input {
	var in Input
	in.Name = RequiredText(v, "name", "Nama", MaxName, p)
	in.Description = OptionalText(v, "description", "Deskripsi", MaxDescription, p)

	switch a := v["active"].(type) {
	case nil:
		in.Active = true // a new record is active unless the form says otherwise
	case bool:
		in.Active = a
	default:
		p.Add("active", "Status aktif harus ya atau tidak.")
	}

	in.ValidFrom = OptionalDate(v, "validFrom", "Berlaku mulai", p)
	in.ValidTo = OptionalDate(v, "validTo", "Berlaku sampai", p)
	if in.ValidFrom != nil && in.ValidTo != nil && *in.ValidTo < *in.ValidFrom {
		p.Add("validTo", "Tanggal selesai tidak boleh sebelum tanggal mulai.")
	}
	return in
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
