package master

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/httpx"
	"alir.local/pm-service/internal/platform/record"
)

// Import is how a master is filled from an Excel workbook, as accounting applications do it (Accurate, Zahir): a
// template with a "Data" sheet (one row per record under a header row), a "Petunjuk" sheet explaining each column,
// and reference sheets listing the values a column takes. Every row goes through the same Create/Update as the form,
// so the rules are the form's own. A row with a code changes that record (only the filled cells); without one it is a
// new record (the code is generated).
type Import struct {
	Columns []ImportColumn
	// Lists are the reference sheets of the template (kelompok, mata uang, akun, …).
	Lists func(ctx context.Context) ([]ImportList, error)
	// UniqueName rejects a row whose name another record already has (or another row of the same file): nothing is
	// added; the problem tells the user to fill Kode with the existing code to change that record, or to rename.
	UniqueName bool
}

type ImportColumn struct {
	Header   string // as in the template's header row ("*" is added when Required)
	Key      string // the form value; "code" is the record's code (update when filled)
	Required bool   // required for a new record
	Hint     string // the Petunjuk sheet's explanation
	Example  string
	Width    float64
	// Choices make the column a drop-down in the template.
	Choices []string
	// Convert turns the cell text into the form value; a non-empty problem rejects the cell. Nil keeps the text.
	Convert func(ctx context.Context, cell string) (value any, problem string, err error)
}

type ImportList struct {
	Sheet  string
	Header []string
	Rows   [][]string
}

// ImportRow is the outcome of one row of the workbook.
type ImportRow struct {
	Row      int              `json:"row"` // the sheet's row number
	Action   string           `json:"action"`
	Code     string           `json:"code,omitempty"`
	Name     string           `json:"name,omitempty"`
	Problems []apperr.Problem `json:"problems"`
}

type ImportResult struct {
	Rows      []ImportRow `json:"rows"`
	Added     int         `json:"added"`
	Updated   int         `json:"updated"`
	Failed    int         `json:"failed"`
	Committed bool        `json:"committed"`
}

const (
	dataSheet   = "Data"
	helpSheet   = "Petunjuk"
	maxRows     = 2000
	maxFileSize = 5 << 20
)

// YesNo reads Ya/Tidak (also Y/N, 1/0, true/false) as a boolean.
func YesNo(_ context.Context, s string) (any, string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ya", "y", "yes", "1", "true", "benar":
		return true, "", nil
	case "tidak", "t", "n", "no", "0", "false", "salah":
		return false, "", nil
	}
	return nil, "Isi Ya atau Tidak.", nil
}

// ByLabel reads a choice by its code or its label (any case).
func ByLabel(labels map[string]string) func(context.Context, string) (any, string, error) {
	return func(_ context.Context, s string) (any, string, error) {
		t := strings.TrimSpace(s)
		for code, label := range labels {
			if strings.EqualFold(t, code) || strings.EqualFold(t, label) {
				return code, "", nil
			}
		}
		return nil, fmt.Sprintf("\"%s\" tidak dikenal; lihat daftar di sheet Petunjuk.", t), nil
	}
}

func header(c ImportColumn) string {
	if c.Required {
		return c.Header + " *"
	}
	return c.Header
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "*")))
}

// template builds the workbook a user fills.
func (s *Service[T, I]) template(ctx context.Context) (*excelize.File, error) {
	im := s.Spec.Import
	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", dataSheet); err != nil {
		return nil, err
	}
	head, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"0B7A3B"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	for i, c := range im.Columns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		col, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetCellValue(dataSheet, cell, header(c))
		w := c.Width
		if w == 0 {
			w = 18
		}
		_ = f.SetColWidth(dataSheet, col, col, w)
		if len(c.Choices) > 0 {
			dv := excelize.NewDataValidation(true)
			dv.Sqref = fmt.Sprintf("%s2:%s%d", col, col, maxRows+1)
			if err := dv.SetDropList(c.Choices); err == nil {
				_ = f.AddDataValidation(dataSheet, dv)
			}
		}
	}
	last, _ := excelize.CoordinatesToCellName(len(im.Columns), 1)
	_ = f.SetCellStyle(dataSheet, "A1", last, head)
	_ = f.SetRowHeight(dataSheet, 1, 30)
	_ = f.SetPanes(dataSheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	// Petunjuk: one line per column.
	if _, err := f.NewSheet(helpSheet); err != nil {
		return nil, err
	}
	bold, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	_ = f.SetSheetRow(helpSheet, "A1", &[]any{fmt.Sprintf("Template impor %s", s.Spec.Noun)})
	_ = f.SetCellStyle(helpSheet, "A1", "A1", bold)
	_ = f.SetSheetRow(helpSheet, "A2", &[]any{"Isi sheet Data mulai baris 2, satu baris satu data. Kolom bertanda * wajib untuk data baru. Kosongkan kolom Kode untuk data baru; isi Kode untuk mengubah data yang sudah ada (hanya kolom yang diisi yang diubah)."})
	_ = f.SetSheetRow(helpSheet, "A4", &[]any{"Kolom", "Wajib", "Keterangan", "Contoh"})
	_ = f.SetCellStyle(helpSheet, "A4", "D4", head)
	for i, c := range im.Columns {
		req := ""
		if c.Required {
			req = "Ya"
		}
		hint := c.Hint
		if len(c.Choices) > 0 {
			hint = strings.TrimSpace(hint + " Pilihan: " + strings.Join(c.Choices, ", ") + ".")
		}
		_ = f.SetSheetRow(helpSheet, fmt.Sprintf("A%d", i+5), &[]any{c.Header, req, hint, c.Example})
	}
	_ = f.SetColWidth(helpSheet, "A", "A", 28)
	_ = f.SetColWidth(helpSheet, "B", "B", 8)
	_ = f.SetColWidth(helpSheet, "C", "C", 90)
	_ = f.SetColWidth(helpSheet, "D", "D", 24)

	if im.Lists != nil {
		lists, err := im.Lists(ctx)
		if err != nil {
			return nil, err
		}
		for _, l := range lists {
			if _, err := f.NewSheet(l.Sheet); err != nil {
				return nil, err
			}
			row := make([]any, len(l.Header))
			for i, h := range l.Header {
				row[i] = h
			}
			_ = f.SetSheetRow(l.Sheet, "A1", &row)
			end, _ := excelize.CoordinatesToCellName(len(l.Header), 1)
			_ = f.SetCellStyle(l.Sheet, "A1", end, head)
			for r, vals := range l.Rows {
				x := make([]any, len(vals))
				for i, v := range vals {
					x[i] = v
				}
				_ = f.SetSheetRow(l.Sheet, fmt.Sprintf("A%d", r+2), &x)
			}
			endCol, _ := excelize.ColumnNumberToName(len(l.Header))
			_ = f.SetColWidth(l.Sheet, "A", endCol, 26)
		}
	}
	f.SetActiveSheet(0)
	return f, nil
}

var errPreview = errors.New("import preview: roll back")

// importFile reads the workbook and runs every row through Create/Update in one transaction: kept only when commit
// and no row failed, rolled back otherwise (a preview).
func (s *Service[T, I]) importFile(ctx context.Context, data []byte, commit bool) (ImportResult, error) {
	im := s.Spec.Import
	res := ImportResult{Rows: []ImportRow{}}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return res, apperr.Validation([]apperr.Problem{{Field: "file", Message: "File bukan workbook Excel (.xlsx) yang dapat dibaca."}})
	}
	defer f.Close()
	sheet := dataSheet
	if idx, _ := f.GetSheetIndex(dataSheet); idx < 0 {
		sheet = f.GetSheetName(0)
	}
	rows, err := f.GetRows(sheet)
	if err != nil || len(rows) == 0 {
		return res, apperr.Validation([]apperr.Problem{{Field: "file", Message: "Sheet Data kosong; isi di bawah baris judul kolom."}})
	}
	// Columns by their header text, in whatever order the sheet has them.
	at := map[int]ImportColumn{}
	found := map[string]bool{}
	for i, h := range rows[0] {
		for _, c := range im.Columns {
			if normalize(h) == normalize(c.Header) {
				at[i] = c
				found[c.Key] = true
			}
		}
	}
	var missing []string
	for _, c := range im.Columns {
		if c.Required && !found[c.Key] {
			missing = append(missing, c.Header)
		}
	}
	if len(missing) > 0 {
		return res, apperr.Validation([]apperr.Problem{{Field: "file", Message: "Kolom wajib tidak ada di baris judul: " + strings.Join(missing, ", ") + ". Gunakan template terbaru."}})
	}
	if len(rows)-1 > maxRows {
		return res, apperr.Validation([]apperr.Problem{{Field: "file", Message: fmt.Sprintf("Paling banyak %d baris per file.", maxRows)}})
	}
	headerOf := map[string]string{}
	for _, c := range im.Columns {
		headerOf[c.Key] = c.Header
	}
	seen := map[string]int{} // name (lower) → the file row that has it
	err = s.DB.InTx(ctx, func(ctx context.Context) error {
		for n, cells := range rows[1:] {
			empty := true
			for _, c := range cells {
				empty = empty && strings.TrimSpace(c) == ""
			}
			if empty {
				continue
			}
			out := ImportRow{Row: n + 2, Problems: []apperr.Problem{}}
			v := record.Values{}
			code := ""
			for i, cell := range cells {
				c, ok := at[i]
				cell = strings.TrimSpace(cell)
				if !ok || cell == "" {
					continue
				}
				if c.Key == "code" {
					code = cell
					continue
				}
				var val any = cell
				if c.Convert != nil {
					x, problem, err := c.Convert(ctx, cell)
					if err != nil {
						return err
					}
					if problem != "" {
						out.Problems = append(out.Problems, apperr.Problem{Field: c.Header, Message: problem})
						continue
					}
					val = x
				}
				v[c.Key] = val
			}
			out.Action = "create"
			if code != "" {
				out.Action, out.Code = "update", code
			}
			if name, ok := v["name"].(string); ok && im.UniqueName {
				problem, err := s.duplicateName(ctx, name, code, out.Row, seen)
				if err != nil {
					return err
				}
				if problem != "" {
					out.Problems = append(out.Problems, apperr.Problem{Field: headerOf["name"], Message: problem})
				}
			}
			if len(out.Problems) == 0 {
				err := s.DB.Savepoint(ctx, func(ctx context.Context) error {
					var t T
					var err error
					if code != "" {
						t, err = s.Update(ctx, code, v)
					} else {
						t, err = s.Create(ctx, v)
					}
					if err == nil {
						b := s.Spec.Base(&t)
						out.Code, out.Name = b.Code, b.Name
					}
					return err
				})
				var ae *apperr.Error
				switch {
				case err == nil:
				case errors.As(err, &ae):
					if ps, ok := ae.Details.(map[string]any)["problems"].([]apperr.Problem); ok {
						for _, p := range ps {
							field := headerOf[strings.SplitN(p.Field, ".", 2)[0]]
							if field == "" {
								field = p.Field
							}
							out.Problems = append(out.Problems, apperr.Problem{Field: field, Message: p.Message})
						}
					} else {
						out.Problems = append(out.Problems, apperr.Problem{Field: "Kode", Message: ae.Message})
					}
				default:
					return err
				}
			}
			if out.Name == "" {
				if s, ok := v["name"].(string); ok {
					out.Name = s
				}
			}
			if len(out.Problems) > 0 {
				res.Failed++
			} else if out.Action == "create" {
				res.Added++
			} else {
				res.Updated++
			}
			res.Rows = append(res.Rows, out)
		}
		if !commit || res.Failed > 0 {
			return errPreview
		}
		return nil
	})
	if errors.Is(err, errPreview) {
		// Nothing was kept: a new record's code would be taken again on the real save, so it is not shown.
		for i := range res.Rows {
			if res.Rows[i].Action == "create" {
				res.Rows[i].Code = ""
			}
		}
		return res, nil
	}
	if err != nil {
		return res, err
	}
	res.Committed = true
	return res, nil
}

// duplicateName is the problem of a row whose name is taken: by a stored record (other than the row's own, when it
// has a code), or by an earlier row of the file. A row joins seen only when its name passes, so a rejected row does
// not block the rows after it; rows saved earlier in the transaction are caught by seen, so a stored code it names
// always exists.
func (s *Service[T, I]) duplicateName(ctx context.Context, name, code string, row int, seen map[string]int) (string, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return "", nil
	}
	if r, ok := seen[key]; ok {
		return fmt.Sprintf("Nama \"%s\" sama dengan baris %d di file ini. Satukan kedua baris atau ganti salah satu nama.", name, r), nil
	}
	var other string
	err := s.DB.Q(ctx).QueryRow(ctx, `SELECT code FROM pm.`+s.Spec.Table+` WHERE lower(btrim(name)) = $1 AND NOT is_deleted AND lower(code) <> lower($2)
		ORDER BY code LIMIT 1`, key, code).Scan(&other)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		seen[key] = row
		return "", nil
	case err != nil:
		return "", err
	case code == "":
		return fmt.Sprintf("Nama \"%s\" sudah ada pada %s %s; baris ini tidak ditambahkan. Untuk mengubah data itu dengan isi file, isi kolom Kode dengan %s. Bila ini data lain, ganti namanya lalu upload ulang.",
			name, strings.ToLower(s.Spec.Noun), other, other), nil
	}
	return fmt.Sprintf("Nama \"%s\" sudah dipakai %s %s. Ganti namanya lalu upload ulang.", name, strings.ToLower(s.Spec.Noun), other), nil
}

// importRoutes registers GET {base}/template.xlsx and POST {base}/upload?commit=true (the workbook as the body).
func importRoutes[T, I any](mux *http.ServeMux, base string, s *Service[T, I]) {
	mux.Handle("GET "+base+"/template.xlsx", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if _, err := access.Require(r.Context(), s.Spec.Menu, "upload"); err != nil {
			return err
		}
		f, err := s.template(r.Context())
		if err != nil {
			return err
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="template-%s.xlsx"`, strings.ReplaceAll(strings.ToLower(s.Spec.Noun), " ", "-")))
		return f.Write(w)
	}))
	mux.Handle("POST "+base+"/upload", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if _, err := access.Require(r.Context(), s.Spec.Menu, "upload"); err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, maxFileSize+1))
		if err != nil {
			return err
		}
		if len(data) > maxFileSize {
			return apperr.Validation([]apperr.Problem{{Field: "file", Message: "File paling besar 5 MB."}})
		}
		res, err := s.importFile(r.Context(), data, r.URL.Query().Get("commit") == "true")
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, res)
	}))
}
