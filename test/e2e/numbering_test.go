//go:build e2e

// Format Penomoran Dokumen: patterns by document type and date; the owning service keeps the counter.
package e2e

import (
	"strings"
	"testing"
)

const numBase = "/pm/masters/numbering"

func TestNumbering(t *testing.T) {
	t.Run("NB01 a company starts with the job pattern; a later pattern applies from its date", func(t *testing.T) {
		l := must(t, call(t, "GET", "/pm/numbering/ff.job?date=2026-11-20", nil), 200)
		if l.str("pattern") != "{SEQ:4}/{MM}/{YY}" || l.str("reset") != "monthly" || l.str("docType") != "FF.JOB" {
			t.Fatalf("seed %s", trunc(l.Raw))
		}
		for doc, pattern := range map[string]string{"FF.INV": "INV/{SEQ:4}/{MM}/{YY}", "FF.RMB": "RMB/{SEQ:4}/{MM}/{YY}"} {
			if x := must(t, call(t, "GET", "/pm/numbering/"+doc+"?date=2026-11-20", nil), 200); x.str("pattern") != pattern {
				t.Errorf("seed %s: %s", doc, trunc(x.Raw))
			}
		}
		n := must(t, call(t, "POST", numBase, map[string]any{"name": "Job baru", "docType": "ff.job", "pattern": "JOB/{YYYY}/{SEQ:5}", "reset": "yearly", "validFrom": "2031-01-01"}), 201)
		if n.str("docType") != "FF.JOB" || !genCodeOf("MPN").MatchString(n.str("code")) {
			t.Errorf("created %s", trunc(n.Raw))
		}
		if x := must(t, call(t, "GET", "/pm/numbering/FF.JOB?date=2031-03-01", nil), 200); x.str("pattern") != "JOB/{YYYY}/{SEQ:5}" {
			t.Errorf("2031 pattern %s", x.str("pattern"))
		}
		if x := must(t, call(t, "GET", "/pm/numbering/FF.JOB?date=2030-12-31", nil), 200); x.str("pattern") != "{SEQ:4}/{MM}/{YY}" {
			t.Errorf("2030 pattern %s", x.str("pattern"))
		}
	})

	t.Run("NB02 patterns that would repeat numbers, duplicates, unknown types and dates", func(t *testing.T) {
		for body, field := range map[string]string{
			`{"name":"x","docType":"X.A","pattern":"A-{YYYY}","validFrom":"2026-01-01"}`:                     "pattern",
			`{"name":"x","docType":"X.B","pattern":"A-{YYYY}-{SEQ:3}","reset":"monthly","validFrom":"2026-01-01"}`: "pattern",
			`{"name":"x","docType":"X.C","pattern":"A-{FOO}-{SEQ:3}","validFrom":"2026-01-01"}`:              "pattern",
			`{"name":"x","docType":"X.D","pattern":"A-{SEQ:3}","scope":"unit","validFrom":"2026-01-01"}`:     "pattern",
			`{"name":"x","docType":"x d","pattern":"A-{SEQ:3}","validFrom":"2026-01-01"}`:                    "docType",
			`{"name":"x","docType":"X.E","pattern":"A-{SEQ:3}"}`:                                             "validFrom",
			`{"name":"x","docType":"FF.JOB","pattern":"A-{SEQ:3}","validFrom":"2026-01-01"}`:                 "validFrom",
		} {
			if x := call(t, "POST", numBase, body); !want(t, x, 422) || x.problem(field) == "" {
				t.Errorf("%s: %s", body, trunc(x.Raw))
			}
		}
		if x := call(t, "GET", "/pm/numbering/NOPE.DOC", nil); x.Status != 404 {
			t.Errorf("unknown type %d", x.Status)
		}
		if x := call(t, "GET", "/pm/numbering/FF.JOB?date=20-11-2026", nil); !want(t, x, 422) || !strings.Contains(x.problem("date"), "YYYY") {
			t.Errorf("bad date %s", trunc(x.Raw))
		}
	})
}
