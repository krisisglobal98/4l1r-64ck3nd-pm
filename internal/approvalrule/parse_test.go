package approvalrule

import (
	"encoding/json"
	"testing"

	"alir.local/pm-service/internal/platform/record"
)

func problem(v record.Values, field string) string {
	_, ps := Parse(v)
	for _, p := range ps {
		if p.Field == field {
			return p.Message
		}
	}
	return ""
}

func base(reach any) record.Values {
	return record.Values{"name": "Pembelian", "sop": "SOP-1", "module": "procurement", "minSteps": json.Number("1"), "reach": reach}
}

func TestReach(t *testing.T) {
	in, ps := Parse(base(map[string]any{"kind": "steps", "steps": json.Number("2")}))
	if len(ps) != 0 || in.Reach.Kind != "steps" || *in.Reach.Steps != 2 {
		t.Fatalf("steps: %+v %v", in.Reach, ps)
	}
	in, ps = Parse(base(map[string]any{"kind": "amount", "bands": []any{
		map[string]any{"max": json.Number("50000000"), "level": "company"}, map[string]any{"max": nil, "level": "group"}}}))
	if len(ps) != 0 || len(in.Reach.Bands) != 2 || *in.Reach.Bands[0].Max != 5e7 || in.Reach.Bands[1].Max != nil {
		t.Fatalf("amount: %+v %v", in.Reach, ps)
	}
	cases := map[string]struct {
		reach      any
		field, msg string
	}{
		"missing":          {nil, "reach.kind", "Jangkauan wajib diisi."},
		"unknown kind":     {map[string]any{"kind": "x"}, "reach.kind", "Jangkauan harus salah satu dari: steps, level, amount."},
		"steps 0":          {map[string]any{"kind": "steps", "steps": json.Number("0")}, "reach.steps", "Jumlah atasan minimal 1."},
		"level missing":    {map[string]any{"kind": "level"}, "reach.level", "Sampai level wajib diisi."},
		"no bands":         {map[string]any{"kind": "amount", "bands": []any{}}, "reach.bands", "Pita nilai wajib diisi: satu baris per pita."},
		"open band first":  {map[string]any{"kind": "amount", "bands": []any{map[string]any{"max": nil, "level": "group"}, map[string]any{"max": json.Number("5"), "level": "unit"}}}, "reach.bands", "Pita tanpa nilai maksimal (di atasnya) hanya boleh di baris terakhir."},
		"not rising":       {map[string]any{"kind": "amount", "bands": []any{map[string]any{"max": json.Number("9"), "level": "unit"}, map[string]any{"max": json.Number("5"), "level": "group"}}}, "reach.bands", "Nilai maksimal pita harus naik dari baris ke baris."},
		"band without lvl": {map[string]any{"kind": "amount", "bands": []any{map[string]any{"max": json.Number("9")}}}, "reach.bands", "Pita ke-1: level wajib diisi."},
		"negative max":     {map[string]any{"kind": "amount", "bands": []any{map[string]any{"max": json.Number("-1"), "level": "unit"}}}, "reach.bands", "Pita ke-1: nilai maksimal harus angka lebih dari 0."},
	}
	for name, c := range cases {
		if got := problem(base(c.reach), c.field); got != c.msg {
			t.Errorf("%s: %s = %q, want %q", name, c.field, got, c.msg)
		}
	}
	if got := problem(record.Values{"name": "x", "reach": map[string]any{"kind": "steps", "steps": 1}}, "sop"); got != "Dasar SOP wajib diisi." {
		t.Errorf("sop: %q", got)
	}
	if got := problem(record.Values{"name": "x", "sop": "s", "module": "m", "minSteps": json.Number("-1"), "reach": map[string]any{"kind": "steps", "steps": 1}}, "minSteps"); got != "Minimal penyetuju minimal 0." {
		t.Errorf("minSteps: %q", got)
	}
}
