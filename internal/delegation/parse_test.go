package delegation

import (
	"testing"

	"alir.local/pm-service/internal/platform/record"
)

func TestParse(t *testing.T) {
	in, ps := Parse(record.Values{"name": "Cuti", "validFrom": "2026-11-01", "validTo": "2026-11-05", "positionCode": "C11-EM", "delegateEmployeeCode": "E011", "ruleCodes": []any{"CUTI", "cuti", " ", "REIMB"}})
	if len(ps) != 0 || len(in.RuleCodes) != 2 {
		t.Fatalf("rules trimmed and once each: %+v %v", in.RuleCodes, ps)
	}
	msgs := map[string]string{}
	_, ps = Parse(record.Values{"name": "Cuti", "ruleCodes": "CUTI"})
	for _, p := range ps {
		msgs[p.Field] = p.Message
	}
	if msgs["validFrom"] != "Berlaku mulai wajib diisi." || msgs["validTo"] != "Berlaku sampai wajib diisi: delegasi tidak boleh tanpa batas waktu." ||
		msgs["positionCode"] != "Posisi pemberi wajib diisi." || msgs["delegateEmployeeCode"] != "Penerima delegasi wajib diisi." || msgs["ruleCodes"] != "Jenis dokumen harus berupa daftar kode." {
		t.Fatalf("problems %v", msgs)
	}
}
