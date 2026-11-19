package master

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/record"
)

// TableRepo is the Repo of a master whose own columns all sit in its table (no relation to show by code): the
// standard columns, then Cols. Scan reads record.Columns followed by Cols (Cols may cast, e.g. "teu::float8"; the
// column written is the name before "::"); Args gives the values to write, in Cols order.
type TableRepo[T, I any] struct {
	DB    *database.DB
	Table string
	Cols  []string
	Scan  func(pgx.Row) (T, error)
	Input func(*I) *record.Input
	Args  func(I) []any
	Order string // ORDER BY of the list; default the code
}

func (r TableRepo[T, I]) sel() string {
	return `SELECT ` + record.Columns + `, ` + strings.Join(r.Cols, ", ") + ` FROM pm.` + r.Table
}

func (r TableRepo[T, I]) names() []string {
	out := make([]string, len(r.Cols))
	for i, c := range r.Cols {
		out[i], _, _ = strings.Cut(c, "::")
	}
	return out
}

func (r TableRepo[T, I]) List(ctx context.Context) ([]T, error) {
	order := r.Order
	if order == "" {
		order = "code"
	}
	return Rows(ctx, r.DB, r.Scan, r.sel()+` WHERE NOT is_deleted ORDER BY `+order)
}

func (r TableRepo[T, I]) Get(ctx context.Context, code string, forUpdate bool) (T, error) {
	q := r.sel() + ` WHERE lower(code) = lower($1) AND NOT is_deleted`
	if forUpdate {
		q += ` FOR UPDATE`
	}
	return One(ctx, r.DB, r.Scan, q, code)
}

func (r TableRepo[T, I]) Insert(ctx context.Context, in I, actor string) (T, error) {
	b := r.Input(&in)
	cols := append([]string{"code", "name", "description", "is_active", "valid_from", "valid_to", "created_by"}, r.names()...)
	args := append([]any{b.Code, b.Name, b.Description, b.Active, b.ValidFrom, b.ValidTo, actor}, r.Args(in)...)
	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = fmt.Sprintf("$%d", i+1)
		if cols[i] == "valid_from" || cols[i] == "valid_to" {
			ph[i] += "::date"
		}
	}
	return One(ctx, r.DB, r.Scan, `WITH x AS (INSERT INTO pm.`+r.Table+` (`+strings.Join(cols, ", ")+`) VALUES (`+strings.Join(ph, ", ")+`) RETURNING *) `+
		strings.Replace(r.sel(), `FROM pm.`+r.Table, `FROM x`, 1), args...)
}

func (r TableRepo[T, I]) Update(ctx context.Context, id string, in I, actor string) (T, error) {
	b := r.Input(&in)
	sets := []string{"name = $2", "description = $3", "is_active = $4", "valid_from = $5::date", "valid_to = $6::date"}
	args := []any{id, b.Name, b.Description, b.Active, b.ValidFrom, b.ValidTo}
	for i, c := range r.names() {
		sets = append(sets, fmt.Sprintf("%s = $%d", c, len(args)+1))
		args = append(args, r.Args(in)[i])
	}
	args = append(args, actor)
	q := `WITH x AS (UPDATE pm.` + r.Table + ` SET ` + strings.Join(sets, ", ") + fmt.Sprintf(`, updated_at = now(), updated_by = $%d, version = version + 1
		WHERE id = $1 AND NOT is_deleted RETURNING *) `, len(args)) + strings.Replace(r.sel(), `FROM pm.`+r.Table, `FROM x`, 1)
	return One(ctx, r.DB, r.Scan, q, args...)
}

func (r TableRepo[T, I]) SoftDelete(ctx context.Context, id, actor string) error {
	return SoftDelete(ctx, r.DB, r.Table, id, actor)
}
