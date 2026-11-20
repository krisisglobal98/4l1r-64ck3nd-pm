// Package numbering is the master "Format Penomoran Dokumen" (Pengaturan Sistem, menu pm.numbering): the number
// pattern of each document type of every module, valid from a date. The owning service asks for the pattern valid on
// a document's date (GET /pm/numbering/{docType}?date=) and keeps the counter itself, so the number is made inside the
// transaction that saves the document. The pattern rules are the frontend's (services/masters/numbering.ts).
package numbering

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

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

var (
	Resets  = []string{"never", "yearly", "monthly", "daily"}
	Scopes  = []string{"global", "company", "unit"}
	token   = regexp.MustCompile(`\{([A-Z]+)(?::(\d+))?\}`)
	docType = regexp.MustCompile(`^[A-Z][A-Z0-9_.-]{1,39}$`)
	known   = map[string]bool{"PT": true, "UNIT": true, "YYYY": true, "YY": true, "MM": true, "DD": true, "SEQ": true}
)

// Problem says why a pattern cannot be used with this reset and scope ("" when it can): numbers must never repeat.
func Problem(pattern, reset, scope string) string {
	var seq []string
	has := map[string]bool{}
	for _, m := range token.FindAllStringSubmatch(pattern, -1) {
		if !known[m[1]] {
			return fmt.Sprintf("Token {%s} tidak dikenal. Gunakan {PT}, {UNIT}, {YYYY}, {YY}, {MM}, {DD}, atau {SEQ:n}.", m[1])
		}
		has[m[1]] = true
		if m[1] == "SEQ" {
			seq = append(seq, m[2])
		}
	}
	if len(seq) != 1 {
		return "Pola harus berisi tepat satu {SEQ:n}, mis. {SEQ:5}."
	}
	if d, err := strconv.Atoi(seq[0]); err != nil || d < 1 || d > 10 {
		return "Panjang {SEQ:n} harus 1 sampai 10 digit."
	}
	year := has["YYYY"] || has["YY"]
	switch {
	case reset == "yearly" && !year:
		return "Reset tahunan butuh {YYYY} atau {YY} agar nomor tidak berulang."
	case reset == "monthly" && !(year && has["MM"]):
		return "Reset bulanan butuh tahun dan {MM} agar nomor tidak berulang."
	case reset == "daily" && !(year && has["MM"] && has["DD"]):
		return "Reset harian butuh tahun, {MM}, dan {DD} agar nomor tidak berulang."
	case scope == "company" && !has["PT"]:
		return "Urutan per perusahaan butuh {PT} agar nomor antar-perusahaan tidak sama."
	case scope == "unit" && !has["UNIT"]:
		return "Urutan per unit butuh {UNIT} agar nomor antar-unit tidak sama."
	}
	return ""
}

type Format struct {
	record.Base
	DocType     string  `json:"docType"`
	Module      *string `json:"module"`
	Pattern     string  `json:"pattern"`
	Reset       string  `json:"reset"`
	Scope       string  `json:"scope"`
	StartNumber int     `json:"start"`
}

type Input struct {
	record.Input
	DocType, Pattern, Reset, Scope string
	Module                         *string
	Start                          int
}

func Parse(v record.Values) (Input, record.Problems) {
	var p record.Problems
	in := Input{Input: record.ParseInput(v, &p)}
	if in.DocType = strings.ToUpper(record.RequiredText(v, "docType", "Jenis dokumen", 40, &p)); in.DocType != "" && !p.Has("docType") && !docType.MatchString(in.DocType) {
		p.Add("docType", "Jenis dokumen berisi huruf besar, angka, titik, - atau _, mis. FF.JOB.")
	}
	in.Module = record.OptionalText(v, "module", "Modul pemakai", 60, &p)
	in.Pattern = record.RequiredText(v, "pattern", "Pola nomor", 60, &p)
	in.Reset = record.ChoiceOr(v, "reset", "Reset urutan", Resets, "never", &p)
	in.Scope = record.ChoiceOr(v, "scope", "Urutan dihitung", Scopes, "global", &p)
	in.Start = 1
	if s := record.OptionalInt(v, "start", "Mulai dari", 1, &p); s != nil {
		in.Start = *s
	}
	if in.Pattern != "" && !p.Has("pattern") {
		if why := Problem(in.Pattern, in.Reset, in.Scope); why != "" {
			p.Add("pattern", why)
		}
	}
	if in.ValidFrom == nil && !p.Has("validFrom") {
		p.Add("validFrom", "Tanggal berlaku pola wajib diisi.")
	}
	return in, p
}

func values(x Format) record.Values {
	v := record.ValuesOf(x.Base)
	v["docType"], v["module"], v["pattern"], v["reset"], v["scope"], v["start"] = x.DocType, record.Opt(x.Module), x.Pattern, x.Reset, x.Scope, x.StartNumber
	return v
}

func scan(row pgx.Row) (Format, error) {
	var x Format
	return x, x.Scan(row, &x.DocType, &x.Module, &x.Pattern, &x.Reset, &x.Scope, &x.StartNumber)
}

func New(db *database.DB) *master.Service[Format, Input] {
	return &master.Service[Format, Input]{
		Spec: master.Spec[Format, Input]{
			Menu: "pm.numbering", Table: "doc_numbering", Noun: "Format nomor",
			Base: func(x *Format) *record.Base { return &x.Base }, Values: values, Parse: Parse,
			SetCode: func(in *Input, c string) { in.Code = c },
			Check: func(ctx context.Context, in *Input, stored *Format, p record.Problems) (record.Problems, error) {
				if in.DocType == "" || in.ValidFrom == nil || p.Has("docType") || p.Has("validFrom") {
					return p, nil
				}
				self := ""
				if stored != nil {
					self = stored.ID
				}
				var other string
				err := db.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.doc_numbering WHERE doc_type = $1 AND valid_from = $2::date AND NOT is_deleted AND id::text <> $3`,
					in.DocType, *in.ValidFrom, self).Scan(&other)
				switch {
				case err == nil:
					p.Add("validFrom", fmt.Sprintf("Pola %s mulai %s sudah ada (%s); ubah yang itu.", in.DocType, *in.ValidFrom, other))
				case !errors.Is(err, pgx.ErrNoRows):
					return p, err
				}
				return p, nil
			},
		},
		Repo: master.TableRepo[Format, Input]{DB: db, Table: "doc_numbering",
			Cols:  []string{"doc_type", "module", "pattern", "reset", "scope", "start_number"},
			Scan:  scan,
			Input: func(in *Input) *record.Input { return &in.Input },
			Args:  func(in Input) []any { return []any{in.DocType, in.Module, in.Pattern, in.Reset, in.Scope, in.Start} },
			Order: "doc_type, valid_from DESC"},
		DB: db, Codes: codeprefix.Postgres{DB: db}, Events: outbox.Writer{DB: db},
	}
}

// On is the active pattern of a document type valid on a date: the latest valid_from on or before it, not ended.
func On(ctx context.Context, db *database.DB, doc, date string) (Format, error) {
	return master.One(ctx, db, scan, `SELECT `+record.Columns+`, doc_type, module, pattern, reset, scope, start_number FROM pm.doc_numbering
		WHERE doc_type = upper($1) AND is_active AND NOT is_deleted AND valid_from <= $2::date AND (valid_to IS NULL OR valid_to >= $2::date)
		ORDER BY valid_from DESC LIMIT 1`, doc, date)
}

// Routes registers the master and GET /pm/numbering/{docType}?date=YYYY-MM-DD (default today, UTC). Any signed-in
// user may read a pattern: a service issuing a document asks with the user's token.
func Routes(mux *http.ServeMux, base string, db *database.DB) {
	master.Routes(mux, base, New(db))
	mux.Handle("GET /pm/numbering/{docType}", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if _, ok := access.UserFrom(r.Context()); !ok {
			return &apperr.Error{Status: http.StatusUnauthorized, Code: "SIGNED_OUT", Message: "Sesi tidak ditemukan. Silakan masuk kembali."}
		}
		doc, date := r.PathValue("docType"), r.URL.Query().Get("date")
		if date == "" {
			date = time.Now().UTC().Format(time.DateOnly)
		}
		if !docType.MatchString(strings.ToUpper(doc)) {
			return apperr.NotFound()
		}
		if _, err := time.Parse(time.DateOnly, date); err != nil {
			return apperr.Validation(record.Problems{{Field: "date", Message: "Tanggal berformat YYYY-MM-DD."}})
		}
		f, err := On(r.Context(), db, doc, date)
		if errors.Is(err, master.ErrNotFound) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, f)
	}))
}
