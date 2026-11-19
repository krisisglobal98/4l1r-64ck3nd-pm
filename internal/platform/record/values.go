package record

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"alir.local/pm-service/internal/platform/apperr"
)

// Values are form values as the frontend sends them: text, numbers (or numeric text), booleans, null.
type Values map[string]any

// CodePattern is what a code may hold: letters, digits, - and _, starting with a letter or digit.
var CodePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Problems collects one message per field, the first one found.
type Problems []apperr.Problem

func (p *Problems) Add(field, message string) {
	if !p.Has(field) {
		*p = append(*p, apperr.Problem{Field: field, Message: message})
	}
}

func (p Problems) Has(field string) bool {
	for _, x := range p {
		if x.Field == field {
			return true
		}
	}
	return false
}

// Text reads a string field; null and absent read as "" (an empty field).
func Text(x any) (string, bool) {
	switch s := x.(type) {
	case nil:
		return "", true
	case string:
		return s, true
	default:
		return "", false
	}
}

// Clean says whether s can be stored as text: valid UTF-8 without NUL (PostgreSQL refuses both).
func Clean(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }

func badChars(label string) string {
	return fmt.Sprintf("%s berisi karakter yang tidak diizinkan.", label)
}

// RequiredText is the text of key, trimmed; a problem when it is missing, blank or too long.
func RequiredText(v Values, key, label string, max int, p *Problems) string {
	s, ok := Text(v[key])
	s = strings.TrimSpace(s)
	switch {
	case !ok:
		p.Add(key, fmt.Sprintf("%s harus berupa teks.", label))
	case !Clean(s):
		p.Add(key, badChars(label))
	case s == "":
		p.Add(key, fmt.Sprintf("%s wajib diisi.", label))
	case len([]rune(s)) > max:
		p.Add(key, fmt.Sprintf("%s paling banyak %d karakter.", label, max))
	}
	return s
}

// OptionalText is nil for an empty field. Outer spaces are kept (line breaks of a description matter) only
// when there is text.
func OptionalText(v Values, key, label string, max int, p *Problems) *string {
	s, ok := Text(v[key])
	switch {
	case !ok:
		p.Add(key, fmt.Sprintf("%s harus berupa teks.", label))
	case !Clean(s):
		p.Add(key, badChars(label))
	case len([]rune(s)) > max:
		p.Add(key, fmt.Sprintf("%s paling banyak %d karakter.", label, max))
	case strings.TrimSpace(s) != "":
		return &s
	}
	return nil
}

// OptionalDate reads YYYY-MM-DD; nil for an empty field.
func OptionalDate(v Values, key, label string, p *Problems) *string {
	s, ok := Text(v[key])
	if !ok {
		p.Add(key, fmt.Sprintf("%s harus berupa tanggal.", label))
		return nil
	}
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		p.Add(key, fmt.Sprintf("%s bukan tanggal yang sah (format YYYY-MM-DD).", label))
		return nil
	}
	return &s
}

// ExpectedVersion is the "version" a client sends with a change: the version it read. Absent = no check.
func ExpectedVersion(v Values, p *Problems) (int, bool) {
	if v["version"] == nil {
		return 0, false
	}
	n, state := number(v["version"])
	if state != numOK || n != math.Trunc(n) || n < 1 || n > math.MaxInt32 {
		p.Add("version", "Versi data tidak sah; muat ulang halaman.")
		return 0, false
	}
	return int(n), true
}

// RequiredInt reads a whole number of at least min; a numeric text is accepted.
func RequiredInt(v Values, key, label string, min int, p *Problems) int {
	n, state := number(v[key])
	switch {
	case state == numBlank:
		p.Add(key, fmt.Sprintf("%s wajib diisi.", label))
	case state == numInvalid || n < float64(min):
		p.Add(key, fmt.Sprintf("%s minimal %d.", label, min))
	case n != math.Trunc(n):
		p.Add(key, fmt.Sprintf("%s harus bilangan bulat.", label))
	case n > math.MaxInt32: // the integer column
		p.Add(key, fmt.Sprintf("%s paling besar %d.", label, math.MaxInt32))
	default:
		return int(n)
	}
	return 0
}

type numState int

const (
	numOK numState = iota
	numBlank
	numInvalid
)

func number(x any) (float64, numState) {
	switch n := x.(type) {
	case nil:
		return 0, numBlank
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, numInvalid
		}
		return f, numOK
	case float64:
		return n, numOK
	case int:
		return float64(n), numOK
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, numBlank
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, numInvalid
		}
		return f, numOK
	default:
		return 0, numInvalid
	}
}

// Choice is the text of key when it is one of allowed (required).
func Choice(v Values, key, label string, allowed []string, p *Problems) string {
	s := RequiredText(v, key, label, 40, p)
	if s == "" || p.Has(key) {
		return s
	}
	for _, a := range allowed {
		if strings.EqualFold(s, a) {
			return a
		}
	}
	p.Add(key, fmt.Sprintf("%s harus salah satu dari: %s.", label, strings.Join(allowed, ", ")))
	return s
}

// Bool reads key; absent is def.
func Bool(v Values, key, label string, def bool, p *Problems) bool {
	switch b := v[key].(type) {
	case nil:
		return def
	case bool:
		return b
	}
	p.Add(key, fmt.Sprintf("%s harus ya atau tidak.", label))
	return def
}

// Codes reads a list of codes (relations by code): trimmed, empty items dropped, duplicates (any case) kept once.
func Codes(v Values, key, label string, p *Problems) []string {
	var items []any
	switch x := v[key].(type) {
	case nil:
		return nil
	case []any:
		items = x
	case []string:
		for _, s := range x {
			items = append(items, s)
		}
	default:
		p.Add(key, fmt.Sprintf("%s harus berupa daftar kode.", label))
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		s, ok := it.(string)
		if !ok || !Clean(s) || len([]rune(s)) > MaxCode {
			p.Add(key, fmt.Sprintf("%s berisi kode yang tidak sah.", label))
			return nil
		}
		if s = strings.TrimSpace(s); s != "" && !seen[strings.ToLower(s)] {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	return out
}

// OptionalInt reads a whole number of at least min; nil for an empty field.
func OptionalInt(v Values, key, label string, min int, p *Problems) *int {
	if _, state := number(v[key]); state == numBlank {
		return nil
	}
	n := RequiredInt(v, key, label, min, p)
	if p.Has(key) {
		return nil
	}
	return &n
}
