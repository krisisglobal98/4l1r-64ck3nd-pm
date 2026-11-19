package record

import "fmt"

// Rows reads a list of sub-records (one object per row: the addresses of a partner, …). Absent or null is an empty
// list; anything else than a list of objects, or more than max rows, is a problem on key.
func Rows(v Values, key, label string, max int, p *Problems) []Values {
	var items []any
	switch x := v[key].(type) {
	case nil:
		return nil
	case []any:
		items = x
	case []Values:
		return x
	case []map[string]any:
		out := make([]Values, len(x))
		for i, m := range x {
			out[i] = m
		}
		return out
	default:
		p.Add(key, fmt.Sprintf("%s harus berupa daftar baris.", label))
		return nil
	}
	if len(items) > max {
		p.Add(key, fmt.Sprintf("%s paling banyak %d baris.", label, max))
		return nil
	}
	out := make([]Values, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			p.Add(key, fmt.Sprintf("%s harus berupa daftar baris.", label))
			return nil
		}
		out = append(out, m)
	}
	return out
}

// RowField is the problem field of one cell: "<key>.<row>.<field>", rows counted from 0 (the form marks the cell).
func RowField(key string, i int, field string) string { return fmt.Sprintf("%s.%d.%s", key, i, field) }

// AddRow moves the problems of row i onto its cells, each message prefixed "<label> baris <n>: ".
func AddRow(p *Problems, key, label string, i int, sub Problems) {
	for _, x := range sub {
		p.Add(RowField(key, i, x.Field), fmt.Sprintf("%s baris %d: %s", label, i+1, x.Message))
	}
}

// Primary makes at most one row primary: a second one marked is a problem on key; with none marked, the first
// row is. flags are the rows' primary marks, changed in place.
func Primary(flags []bool, key, label string, p *Problems) {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	switch {
	case n > 1:
		p.Add(key, fmt.Sprintf("%s: hanya satu baris yang boleh utama.", label))
	case n == 0 && len(flags) > 0:
		flags[0] = true
	}
}
