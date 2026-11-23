// Package sysparam is the master "System Parameter" (Pengaturan Sistem, menu pm.systemParameter): named values the
// application modules read by key, e.g. INVOICE_DEFAULT_DUE_DAYS (Freight Forwarding). A module reads one through GET
// /pm/parameters/{key}, open to every signed-in user of the tenant.
package sysparam

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/httpx"
	"alir.local/pm-service/internal/platform/master"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
)

type Parameter struct {
	record.Base
	Key    string `json:"key"`
	Value  string `json:"value"`
	Module string `json:"module"`
}

type Input struct {
	record.Input
	Key, Value, Module string
}

// Modules a parameter belongs to.
var Modules = []string{"umum", "freight-forwarding", "finance", "hr"}

// Known are the parameters the modules read, with what their value must be.
var Known = map[string]struct {
	Module string
	Check  func(string) string // a problem, or ""
}{
	"INVOICE_DEFAULT_DUE_DAYS": {"freight-forwarding", func(v string) string {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err != nil || n < 0 || n > 365 {
			return "Jumlah hari 0–365."
		}
		return ""
	}},
}

var key = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	in.Key = strings.ToUpper(strings.TrimSpace(record.RequiredText(v, "key", "Kunci", 60, &p)))
	if in.Key != "" && !key.MatchString(in.Key) {
		p.Add("key", "Kunci berisi huruf besar, angka, dan garis bawah, diawali huruf, mis. INVOICE_DEFAULT_DUE_DAYS.")
	}
	in.Value = record.RequiredText(v, "value", "Nilai", 500, &p)
	in.Module = record.ChoiceOr(v, "module", "Modul", Modules, "umum", &p)
	if k, ok := Known[in.Key]; ok && !p.Has("value") {
		if msg := k.Check(in.Value); msg != "" {
			p.Add("value", msg)
		}
	}
	return in, p
}

func values(x Parameter) record.Values {
	v := record.ValuesOf(x.Base)
	v["key"], v["value"], v["module"] = x.Key, x.Value, x.Module
	return v
}

func New(db *database.DB) *master.Service[Parameter, Input] {
	return &master.Service[Parameter, Input]{
		Spec: master.Spec[Parameter, Input]{
			Menu: "pm.systemParameter", Table: "system_parameter", Noun: "System parameter",
			Base: func(x *Parameter) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Parameter, p record.Problems) (record.Problems, error) {
				if stored != nil && stored.Key != in.Key {
					p.Add("key", "Kunci tidak dapat diubah; modul membacanya dengan kunci ini.")
					return p, nil
				}
				if p.Has("key") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				var other string
				switch err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.system_parameter WHERE param_key = $1 AND NOT is_deleted AND id::text <> $2`, in.Key, self).Scan(&other); {
				case err == nil:
					p.Add("key", fmt.Sprintf("Kunci %s sudah dipakai parameter %s.", in.Key, other))
				case !errors.Is(err, pgx.ErrNoRows):
					return p, err
				}
				return p, nil
			},
		},
		Repo: repo{db}, DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

type repo struct{ db *database.DB }

var sel = `SELECT ` + record.ColumnsAs("x") + `, x.param_key, x.param_value, x.module FROM pm.system_parameter x`

func scan(row pgx.Row) (Parameter, error) {
	var x Parameter
	return x, x.Scan(row, &x.Key, &x.Value, &x.Module)
}

func (r repo) List(ctx context.Context) ([]Parameter, error) {
	return master.Rows(ctx, r.db, scan, sel+` WHERE NOT x.is_deleted ORDER BY x.module, x.param_key`)
}

func (r repo) Get(ctx context.Context, code string, forUpdate bool) (Parameter, error) {
	q := sel + ` WHERE lower(x.code) = lower($1) AND NOT x.is_deleted`
	if forUpdate {
		q += ` FOR UPDATE OF x`
	}
	return master.One(ctx, r.db, scan, q, code)
}

func (r repo) byID(ctx context.Context, id string) (Parameter, error) {
	return master.One(ctx, r.db, scan, sel+` WHERE x.id = $1`, id)
}

func (r repo) Insert(ctx context.Context, in Input, actor string) (Parameter, error) {
	var id string
	if err := r.db.Q(ctx).QueryRow(ctx, `INSERT INTO pm.system_parameter (code, name, description, is_active, valid_from, valid_to, created_by, param_key, param_value, module)
		VALUES ($1, $2, $3, $4, $5::date, $6::date, $7, $8, $9, $10) RETURNING id`,
		in.Code, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, actor, in.Key, in.Value, in.Module).Scan(&id); err != nil {
		return Parameter{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) Update(ctx context.Context, id string, in Input, actor string) (Parameter, error) {
	if err := master.Exec(ctx, r.db, `UPDATE pm.system_parameter SET name = $2, description = $3, is_active = $4, valid_from = $5::date, valid_to = $6::date,
		param_value = $7, module = $8, updated_at = now(), updated_by = $9, version = version + 1 WHERE id = $1 AND NOT is_deleted`,
		id, in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, in.Value, in.Module, actor); err != nil {
		return Parameter{}, err
	}
	return r.byID(ctx, id)
}

func (r repo) SoftDelete(ctx context.Context, id, actor string) error {
	return master.SoftDelete(ctx, r.db, "system_parameter", id, actor)
}

// ReadRoutes registers GET /pm/parameters/{key}: {key, value} of the active parameter (404 when none), for the modules.
func ReadRoutes(mux *http.ServeMux, db *database.DB) {
	mux.Handle("GET /pm/parameters/{key}", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if _, ok := access.UserFrom(r.Context()); !ok {
			return &apperr.Error{Status: http.StatusUnauthorized, Code: "SIGNED_OUT", Message: "Sesi tidak ditemukan. Silakan masuk kembali."}
		}
		k := strings.ToUpper(r.PathValue("key"))
		var v string
		err := db.Q(r.Context()).QueryRow(r.Context(), `SELECT param_value FROM pm.system_parameter WHERE param_key = $1 AND is_active AND NOT is_deleted
			AND (valid_from IS NULL OR valid_from <= current_date) AND (valid_to IS NULL OR valid_to >= current_date)`, k).Scan(&v)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]string{"key": k, "value": v})
	}))
}
