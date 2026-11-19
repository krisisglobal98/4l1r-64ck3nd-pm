package codeprefix

import "testing"

func TestFormat(t *testing.T) {
	for _, c := range []struct {
		prefix string
		digits int
		n      int64
		want   string
	}{{"ML", 3, 1, "ML001"}, {"ML", 3, 42, "ML042"}, {"ML", 3, 1234, "ML1234"}, {"MG", 2, 1, "MG01"}} {
		if got := Format(c.prefix, c.digits, c.n); got != c.want {
			t.Errorf("Format(%q, %d, %d) = %q, want %q", c.prefix, c.digits, c.n, got, c.want)
		}
	}
}
