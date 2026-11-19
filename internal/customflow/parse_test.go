package customflow

import (
	"testing"

	"alir.local/pm-service/internal/platform/record"
)

func TestParse(t *testing.T) {
	in, ps := Parse(record.Values{"name": "Capex", "sop": "SOP", "ruleCode": "CAPEX", "scopeUnitCode": "SUM", "stepCodes": []any{"A", "B", "a"}})
	if len(ps) != 0 || len(in.StepCodes) != 2 || in.StepCodes[0] != "A" {
		t.Fatalf("steps in order, once each: %+v %v", in.StepCodes, ps)
	}
	_, ps = Parse(record.Values{"name": "Capex", "sop": "SOP", "ruleCode": "CAPEX", "scopeUnitCode": "SUM", "stepCodes": []any{}})
	if len(ps) != 1 || ps[0].Message != "Langkah penyetuju wajib diisi: pilih minimal satu posisi." {
		t.Fatalf("no steps: %v", ps)
	}
}
