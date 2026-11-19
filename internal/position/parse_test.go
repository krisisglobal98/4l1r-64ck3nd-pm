package position

import (
	"testing"

	"alir.local/pm-service/internal/platform/record"
)

func TestParse(t *testing.T) {
	in, ps := Parse(record.Values{"unitCode": "C11", "jabatanCode": "EM", "head": true})
	if len(ps) != 0 || !in.AutoName || !in.Head || in.ReportsToCode != nil {
		t.Fatalf("an empty name is made from jabatan and unit: %+v %v", in, ps)
	}
	if in, _ := Parse(record.Values{"name": " Kepala Kebun ", "unitCode": "C11", "jabatanCode": "EM"}); in.AutoName || in.Name != "Kepala Kebun" {
		t.Fatalf("a typed name is kept: %+v", in)
	}
	_, ps = Parse(record.Values{"head": "ya"})
	want := map[string]bool{"unitCode": true, "jabatanCode": true, "head": true}
	for _, p := range ps {
		delete(want, p.Field)
	}
	if len(want) != 0 {
		t.Fatalf("missing problems %v in %v", want, ps)
	}
}
