package orgunit

import (
	"strings"
	"testing"

	"alir.local/pm-service/internal/platform/record"
)

type V = record.Values

func TestParse(t *testing.T) {
	in, ps := Parse(V{"code": "ignored", "name": " Kebun A ", "levelCode": "unit", "parentCode": "C11", "unitTypeCode": "EST", "location": "Sekadau"})
	if len(ps) != 0 || in.Code != "" || in.Name != "Kebun A" || in.LevelCode != "unit" || *in.ParentCode != "C11" || *in.UnitTypeCode != "EST" || *in.Location != "Sekadau" {
		t.Fatalf("got %+v %v", in, ps)
	}
	if in, _ := Parse(V{"name": "G", "levelCode": "group", "parentCode": "", "unitTypeCode": nil}); in.ParentCode != nil || in.UnitTypeCode != nil {
		t.Fatalf("empty relations are none: %+v", in)
	}
	cases := []struct {
		v          V
		field, msg string
	}{
		{V{"name": "x"}, "levelCode", "Level wajib diisi."},
		{V{"name": "x", "levelCode": 5}, "levelCode", "Level harus berupa teks."},
		{V{"name": "x", "levelCode": "unit", "parentCode": strings.Repeat("p", 41)}, "parentCode", "Induk paling banyak 40 karakter."},
		{V{"name": "x", "levelCode": "unit", "unitTypeCode": "a\x00"}, "unitTypeCode", "Jenis unit berisi karakter yang tidak diizinkan."},
		{V{"name": "x", "levelCode": "unit", "location": strings.Repeat("l", 81)}, "location", "Lokasi paling banyak 80 karakter."},
		{V{"levelCode": "unit"}, "name", "Nama wajib diisi."},
	}
	for _, c := range cases {
		_, ps := Parse(c.v)
		got := ""
		for _, p := range ps {
			if p.Field == c.field {
				got = p.Message
			}
		}
		if got != c.msg {
			t.Errorf("%v: %s = %q, want %q", c.v, c.field, got, c.msg)
		}
	}
}
