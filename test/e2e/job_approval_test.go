//go:build e2e

// Jabatan & pejabat (grades, jabatan, positions) and Alur approval (approval rules, delegations, custom flows):
// the masters on the shared shell, their relations, the usage checks across masters and the full sample seed.
package e2e

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
)

const (
	grBase  = "/pm/masters/grades"
	jbBase  = "/pm/masters/jabatan"
	psBase  = "/pm/masters/positions"
	arBase  = "/pm/masters/approval-rules"
	dlBase  = "/pm/masters/delegations"
	cfBase  = "/pm/masters/custom-flows"
	graded  = 1000 // grade ranks of this suite start here
	ensured = 201
)

var gradeRank struct {
	sync.Mutex
	n int
}

func nextGradeRank() int {
	gradeRank.Lock()
	defer gradeRank.Unlock()
	gradeRank.n++
	return graded + gradeRank.n
}

func must(t testing.TB, r resp, status int) resp {
	t.Helper()
	if r.Status != status {
		t.Fatalf("status %d, want %d: %s", r.Status, status, trunc(r.Raw))
	}
	return r
}

// org is a small organization: one branch, a grade, job titles at company and unit level, and positions.
type org struct {
	b                       branch
	grade                   string
	jCompany, jUnit, jUnit2 string
	pDirector, pHead, pAsst string
	emp                     string // an employee (Karyawan) to delegate to
}

func newOrg(t testing.TB) org {
	t.Helper()
	var o org
	o.b = newBranch(t)
	o.grade = must(t, call(t, "POST", grBase, map[string]any{"name": "Grade uji", "rank": nextGradeRank()}), 201).str("code")
	o.jCompany = must(t, call(t, "POST", jbBase, map[string]any{"name": "Direktur", "gradeCode": o.grade, "kind": "struktural", "levelCode": "company"}), 201).str("code")
	o.jUnit = must(t, call(t, "POST", jbBase, map[string]any{"name": "Manajer Kebun", "gradeCode": o.grade, "kind": "struktural", "levelCode": "unit"}), 201).str("code")
	o.jUnit2 = must(t, call(t, "POST", jbBase, map[string]any{"name": "Asisten", "gradeCode": o.grade, "kind": "fungsional", "levelCode": "unit"}), 201).str("code")
	o.pDirector = must(t, call(t, "POST", psBase, map[string]any{"unitCode": o.b.c, "jabatanCode": o.jCompany, "head": true}), 201).str("code")
	o.pHead = must(t, call(t, "POST", psBase, map[string]any{"unitCode": o.b.u, "jabatanCode": o.jUnit, "head": true, "reportsToCode": o.pDirector}), 201).str("code")
	o.pAsst = must(t, call(t, "POST", psBase, map[string]any{"unitCode": o.b.u, "jabatanCode": o.jUnit2, "reportsToCode": o.pHead}), 201).str("code")
	o.emp = newEmployee(t)
	return o
}

var genCodeOf = func(prefix string) *regexp.Regexp { return regexp.MustCompile(`^` + prefix + `\d{3,}$`) }

func TestJobAndApprovalPositive(t *testing.T) {
	o := newOrg(t)

	t.Run("JP01 grade: generated code, rank, sorted by seniority", func(t *testing.T) {
		g := call(t, "GET", grBase+"/"+strings.ToLower(o.grade), nil)
		if want(t, g, 200) && (!genCodeOf("MGR").MatchString(g.str("code")) || g.Obj["rank"] == nil) {
			t.Errorf("got %s", g.Raw)
		}
		list := call(t, "GET", grBase, nil)
		for i := 1; i < len(list.List); i++ {
			if list.List[i]["rank"].(float64) > list.List[i-1]["rank"].(float64) {
				t.Errorf("not by seniority: %v then %v", list.List[i-1]["rank"], list.List[i]["rank"])
			}
		}
	})

	t.Run("JP02 jabatan: grade and level by id and code", func(t *testing.T) {
		j := call(t, "GET", jbBase+"/"+o.jUnit, nil)
		if want(t, j, 200) && (j.str("gradeCode") != o.grade || !uuidV7.MatchString(j.str("gradeId")) || j.str("levelCode") != "unit" || j.str("kind") != "struktural") {
			t.Errorf("got %s", j.Raw)
		}
	})

	t.Run("JP03 the head job title of a unit type is now a real relation", func(t *testing.T) {
		r := call(t, "PUT", utBase+"/EST", map[string]any{"headJabatanCode": o.jUnit})
		if want(t, r, 200) && (r.str("headJabatanCode") != o.jUnit || !uuidV7.MatchString(r.str("headJabatanId"))) {
			t.Errorf("got %s", r.Raw)
		}
		if r := call(t, "DELETE", jbBase+"/"+o.jUnit, nil); want(t, r, 409) && !strings.Contains(r.str("message"), "1 jenis unit") {
			t.Errorf("a head job title in use: %s", r.Raw)
		}
		want(t, call(t, "PUT", utBase+"/EST", map[string]any{"headJabatanCode": nil}), 200)
	})

	t.Run("JP04 position: name made from jabatan and unit, reporting line, holders empty", func(t *testing.T) {
		p := call(t, "GET", psBase+"/"+o.pAsst, nil)
		if !want(t, p, 200) {
			return
		}
		if !genCodeOf("MPS").MatchString(p.str("code")) || p.str("name") != "Asisten · Kebun" || p.str("unitCode") != o.b.u || p.str("jabatanCode") != o.jUnit2 ||
			p.str("reportsToCode") != o.pHead || p.Obj["head"] != false || p.Obj["holderEmployeeId"] != nil || p.Obj["actingEmployeeId"] != nil {
			t.Errorf("got %s", p.Raw)
		}
		r := call(t, "PUT", psBase+"/"+o.pAsst, map[string]any{"name": "Asisten Tanaman", "unitCode": o.b.c, "jabatanCode": o.jCompany, "version": 1})
		if want(t, r, 200) && (r.str("name") != "Asisten Tanaman" || r.str("unitCode") != o.b.u || r.str("jabatanCode") != o.jUnit2) {
			t.Errorf("unit and job title stay: %s", r.Raw)
		}
	})

	t.Run("JP05 approval rules: steps, level, amount; switching kind drops the bands", func(t *testing.T) {
		steps := must(t, call(t, "POST", arBase, map[string]any{"name": "Cuti uji", "sop": "SOP-1", "module": "employee-self-service", "minSteps": 1,
			"reach": map[string]any{"kind": "steps", "steps": 2}}), 201)
		if steps.Obj["reach"].(map[string]any)["steps"] != 2.0 || !genCodeOf("MAR").MatchString(steps.str("code")) {
			t.Errorf("steps: %s", steps.Raw)
		}
		amt := must(t, call(t, "POST", arBase, map[string]any{"name": "Capex uji", "sop": "SOP-2", "module": "procurement", "minSteps": 1,
			"reach": map[string]any{"kind": "amount", "bands": []any{map[string]any{"max": 50000000, "level": "company"}, map[string]any{"max": 500000000, "level": "wilayah"}, map[string]any{"max": nil, "level": "GROUP"}}}}), 201)
		bands := amt.Obj["reach"].(map[string]any)["bands"].([]any)
		if len(bands) != 3 || bands[0].(map[string]any)["max"] != 5e7 || bands[2].(map[string]any)["level"] != "group" || bands[2].(map[string]any)["max"] != nil {
			t.Errorf("amount: %s", amt.Raw)
		}
		lvl := call(t, "PUT", arBase+"/"+amt.str("code"), map[string]any{"reach": map[string]any{"kind": "level", "level": "unit"}})
		if want(t, lvl, 200) && (lvl.Obj["reach"].(map[string]any)["level"] != "unit" || lvl.Obj["reach"].(map[string]any)["bands"] != nil) {
			t.Errorf("level: %s", lvl.Raw)
		}
		var n int
		db.QueryRow(context.Background(), `SELECT count(*) FROM pm.approval_rule_band b JOIN pm.approval_rule r ON r.id = b.approval_rule_id WHERE r.code = $1`, amt.str("code")).Scan(&n)
		if n != 0 {
			t.Errorf("bands left: %d", n)
		}
	})

	t.Run("JP06 delegation: period, document types, every type when empty; delegate empty", func(t *testing.T) {
		rule := must(t, call(t, "POST", arBase, map[string]any{"name": "Reimb uji", "sop": "S", "module": "reimbursement", "minSteps": 1, "reach": map[string]any{"kind": "steps", "steps": 1}}), 201).str("code")
		d := must(t, call(t, "POST", dlBase, map[string]any{"delegateEmployeeCode": o.emp, "name": "Dinas luar kota", "validFrom": "2026-11-10", "validTo": "2026-11-20", "positionCode": o.pHead, "ruleCodes": []any{rule, strings.ToLower(rule)}}), 201)
		if !genCodeOf("MDL").MatchString(d.str("code")) || d.str("positionCode") != o.pHead || fmt.Sprint(d.Obj["ruleCodes"]) != fmt.Sprintf("[%s]", rule) || d.str("delegateEmployeeCode") != o.emp {
			t.Errorf("got %s", d.Raw)
		}
		all := call(t, "PUT", dlBase+"/"+d.str("code"), map[string]any{"ruleCodes": []any{}})
		if want(t, all, 200) && len(all.Obj["ruleCodes"].([]any)) != 0 {
			t.Errorf("every type: %s", all.Raw)
		}
	})

	t.Run("JP07 custom flow: steps kept in order", func(t *testing.T) {
		rule := must(t, call(t, "POST", arBase, map[string]any{"name": "PR uji", "sop": "S", "module": "procurement", "minSteps": 1, "reach": map[string]any{"kind": "steps", "steps": 1}}), 201).str("code")
		f := must(t, call(t, "POST", cfBase, map[string]any{"name": "PR khusus", "sop": "SOP-3", "ruleCode": rule, "scopeUnitCode": o.b.c, "stepCodes": []any{o.pHead, o.pDirector}}), 201)
		if !genCodeOf("MAC").MatchString(f.str("code")) || fmt.Sprint(f.Obj["stepCodes"]) != fmt.Sprintf("[%s %s]", o.pHead, o.pDirector) || f.str("scopeUnitCode") != o.b.c {
			t.Errorf("got %s", f.Raw)
		}
		r := call(t, "PUT", cfBase+"/"+f.str("code"), map[string]any{"stepCodes": []any{o.pDirector, o.pHead, o.pAsst}})
		if want(t, r, 200) && fmt.Sprint(r.Obj["stepCodes"]) != fmt.Sprintf("[%s %s %s]", o.pDirector, o.pHead, o.pAsst) {
			t.Errorf("reordered: %s", r.Raw)
		}
	})

	t.Run("JP08 the full sample seed obeys the rules: an untouched save of each master passes", func(t *testing.T) {
		sql, err := os.ReadFile("../../seeds/sample_org.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("seed: %v", err)
		}
		for _, x := range []struct{ base, code string }{{grBase, "GM"}, {jbBase, "EM"}, {psBase, "C11-E1-EM"}, {psBase, "GAN-CEO"}, {arBase, "PR"}, {arBase, "CUTI"}, {dlBase, "DLG-002"}, {cfBase, "CF-CAPEX-SUM"}, {utBase, "PKS"}} {
			g := call(t, "GET", x.base+"/"+x.code, nil)
			if !want(t, g, 200) {
				continue
			}
			r := call(t, "PUT", x.base+"/"+x.code, map[string]any{"version": g.Obj["version"]})
			if r.Status != 200 {
				t.Errorf("%s %s: %d %s", x.base, x.code, r.Status, trunc(r.Raw))
			}
		}
		pr := call(t, "GET", arBase+"/PR", nil)
		if bands, _ := pr.Obj["reach"].(map[string]any)["bands"].([]any); pr.Obj["reach"].(map[string]any)["kind"] == "amount" && len(bands) == 0 {
			t.Errorf("PR bands: %s", pr.Raw)
		}
		if d := call(t, "GET", dlBase+"/DLG-002", nil); fmt.Sprint(d.Obj["ruleCodes"]) != "[CUTI REIMB]" || d.str("positionCode") != "C11-E1-EM" {
			t.Errorf("DLG-002: %s", d.Raw)
		}
		if f := call(t, "GET", cfBase+"/CF-CAPEX-SUM", nil); fmt.Sprint(f.Obj["stepCodes"]) != "[SUM-RC SUM-RD GAN-HFIN GAN-CEO]" {
			t.Errorf("CF-CAPEX-SUM: %s", f.Raw)
		}
		if u := call(t, "GET", utBase+"/EST", nil); u.str("headJabatanCode") != "EM" {
			t.Errorf("EST head: %s", u.Raw)
		}
	})
}

func TestJobAndApprovalNegative(t *testing.T) {
	o := newOrg(t)
	rule := must(t, call(t, "POST", arBase, map[string]any{"name": "Neg", "sop": "S", "module": "m", "minSteps": 0, "reach": map[string]any{"kind": "steps", "steps": 1}}), 201).str("code")

	cases := []struct {
		id, path string
		body     map[string]any
		field    string
		msg      string
	}{
		{"JN01", grBase, map[string]any{"name": "x"}, "rank", "Urutan wajib diisi."},
		{"JN02", jbBase, map[string]any{"name": "x", "gradeCode": "NOPE", "kind": "struktural", "levelCode": "unit"}, "gradeCode", `Grade "NOPE" tidak ditemukan.`},
		{"JN03", jbBase, map[string]any{"name": "x", "gradeCode": o.grade, "kind": "lain", "levelCode": "unit"}, "kind", "Jenis harus salah satu dari: struktural, fungsional."},
		{"JN04", jbBase, map[string]any{"name": "x", "gradeCode": o.grade, "kind": "struktural", "levelCode": "nope"}, "levelCode", `Level "nope" tidak ditemukan.`},
		{"JN05", psBase, map[string]any{"unitCode": o.b.u, "jabatanCode": o.jCompany}, "jabatanCode", "Jabatan Direktur dipakai di level Perusahaan; pilih jabatan untuk level unit Kebun."},
		{"JN06", psBase, map[string]any{"unitCode": o.b.u, "jabatanCode": o.jUnit2, "head": true}, "head", "Unit Kebun sudah punya kepala: Manajer Kebun · Kebun."},
		{"JN07", psBase, map[string]any{"unitCode": "NOPE", "jabatanCode": o.jUnit}, "unitCode", `Unit "NOPE" tidak ditemukan.`},
		{"JN08", psBase, map[string]any{"unitCode": o.b.u, "jabatanCode": o.jUnit2, "reportsToCode": "NOPE"}, "reportsToCode", `Posisi atasan "NOPE" tidak ditemukan.`},
		{"JN09", arBase, map[string]any{"name": "x", "sop": "S", "module": "m", "minSteps": 0, "reach": map[string]any{"kind": "level", "level": "nope"}}, "reach.level", `Level "nope" tidak ditemukan.`},
		{"JN10", arBase, map[string]any{"name": "x", "sop": "S", "module": "m", "minSteps": 0, "reach": map[string]any{"kind": "amount", "bands": []any{map[string]any{"max": 5, "level": "group"}, map[string]any{"max": nil, "level": "unit"}}}}, "reach.bands", "Pita dengan nilai lebih besar harus sampai level yang sama atau lebih tinggi."},
		{"JN11", dlBase, map[string]any{"delegateEmployeeCode": o.emp, "name": "x", "positionCode": o.pHead}, "validTo", "Berlaku sampai wajib diisi: delegasi tidak boleh tanpa batas waktu."},
		{"JN12", dlBase, map[string]any{"delegateEmployeeCode": o.emp, "name": "x", "validFrom": "2026-01-02", "validTo": "2026-01-01", "positionCode": o.pHead}, "validTo", "Tanggal selesai tidak boleh sebelum tanggal mulai."},
		{"JN13", dlBase, map[string]any{"delegateEmployeeCode": o.emp, "name": "x", "validFrom": "2026-01-01", "validTo": "2026-01-02", "positionCode": o.pHead, "ruleCodes": []any{"NOPE"}}, "ruleCodes", `Jenis dokumen "NOPE" tidak ditemukan.`},
		{"JN14", cfBase, map[string]any{"name": "x", "sop": "S", "ruleCode": rule, "scopeUnitCode": o.b.c, "stepCodes": []any{"NOPE"}}, "stepCodes", `Posisi "NOPE" tidak ditemukan.`},
		{"JN15", cfBase, map[string]any{"name": "x", "sop": "S", "ruleCode": rule, "scopeUnitCode": o.b.c}, "stepCodes", "Langkah penyetuju wajib diisi: pilih minimal satu posisi."},
	}
	for _, c := range cases {
		t.Run(c.id+" "+c.field, func(t *testing.T) {
			r := call(t, "POST", c.path, c.body)
			if want(t, r, 422) && r.problem(c.field) != c.msg {
				t.Errorf("%s: got %q, want %q (%s)", c.field, r.problem(c.field), c.msg, trunc(r.Raw))
			}
		})
	}

	t.Run("JN16 grade rank taken; a reporting loop", func(t *testing.T) {
		g := call(t, "GET", grBase+"/"+o.grade, nil)
		if r := call(t, "POST", grBase, map[string]any{"name": "x", "rank": g.Obj["rank"]}); want(t, r, 422) && !strings.Contains(r.problem("rank"), "sudah dipakai grade") {
			t.Errorf("rank: %s", r.Raw)
		}
		if r := call(t, "PUT", psBase+"/"+o.pDirector, map[string]any{"reportsToCode": o.pAsst}); want(t, r, 422) && r.problem("reportsToCode") != "Garis lapor tidak boleh membentuk lingkaran." {
			t.Errorf("loop: %s", r.Raw)
		}
		if r := call(t, "PUT", psBase+"/"+o.pHead, map[string]any{"reportsToCode": o.pHead}); want(t, r, 422) && r.problem("reportsToCode") == "" {
			t.Errorf("own superior: %s", r.Raw)
		}
	})

	t.Run("JN17 one flow per document type and branch", func(t *testing.T) {
		must(t, call(t, "POST", cfBase, map[string]any{"name": "a", "sop": "S", "ruleCode": rule, "scopeUnitCode": o.b.w, "stepCodes": []any{o.pDirector}}), 201)
		r := call(t, "POST", cfBase, map[string]any{"name": "b", "sop": "S", "ruleCode": rule, "scopeUnitCode": o.b.w, "stepCodes": []any{o.pHead}})
		if want(t, r, 422) && !strings.Contains(r.problem("scopeUnitCode"), "Sudah ada approval custom") {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("JN18 records in use are kept: grade, jabatan, position, rule, unit, level", func(t *testing.T) {
		x := newOrg(t)
		r := must(t, call(t, "POST", arBase, map[string]any{"name": "Pakai", "sop": "S", "module": "m", "minSteps": 0, "reach": map[string]any{"kind": "steps", "steps": 1}}), 201).str("code")
		must(t, call(t, "POST", dlBase, map[string]any{"delegateEmployeeCode": x.emp, "name": "Cuti", "validFrom": "2026-01-01", "validTo": "2026-01-05", "positionCode": x.pHead, "ruleCodes": []any{r}}), 201)
		must(t, call(t, "POST", cfBase, map[string]any{"name": "f", "sop": "S", "ruleCode": r, "scopeUnitCode": x.b.u, "stepCodes": []any{x.pHead}}), 201)
		for _, c := range []struct{ path, want string }{
			{grBase + "/" + x.grade, "3 jabatan"},
			{jbBase + "/" + x.jUnit, "1 posisi"},
			{psBase + "/" + x.pHead, "1 posisi yang melapor ke sini, 1 delegasi, 1 approval custom"},
			{arBase + "/" + r, "1 delegasi, 1 approval custom"},
			{ouBase + "/" + x.b.u, "2 posisi, 1 approval custom"},
		} {
			res := call(t, "DELETE", c.path, nil)
			if res.Status != 409 || !strings.Contains(res.str("message"), c.want) {
				t.Errorf("DELETE %s: %d %s (want %q)", c.path, res.Status, res.Raw, c.want)
			}
		}
		lv := must(t, call(t, "POST", base, map[string]any{"name": "Level pakai", "rank": nextRank() + 5000}), 201).str("code")
		must(t, call(t, "POST", jbBase, map[string]any{"name": "j", "gradeCode": x.grade, "kind": "fungsional", "levelCode": lv}), 201)
		if res := call(t, "DELETE", base+"/"+lv, nil); res.Status != 409 || !strings.Contains(res.str("message"), "1 jabatan") {
			t.Errorf("level used by a jabatan: %d %s", res.Status, res.Raw)
		}
		if res := call(t, "PUT", jbBase+"/"+x.jUnit, map[string]any{"levelCode": "company"}); res.Status != 422 || !strings.Contains(res.problem("levelCode"), "dipakai 1 posisi") {
			t.Errorf("level of a jabatan in use: %d %s", res.Status, res.Raw)
		}
	})

	t.Run("JN19 inactive references: refused for a new choice, kept when unchanged", func(t *testing.T) {
		x := newOrg(t)
		must(t, call(t, "PUT", jbBase+"/"+x.jUnit2, map[string]any{"active": false}), 200)
		if r := call(t, "POST", psBase, map[string]any{"unitCode": x.b.u, "jabatanCode": x.jUnit2}); want(t, r, 422) && !strings.Contains(r.problem("jabatanCode"), "nonaktif") {
			t.Errorf("new: %s", r.Raw)
		}
		want(t, call(t, "PUT", psBase+"/"+x.pAsst, map[string]any{"description": "tetap"}), 200)
	})

	t.Run("JN20 unknown codes 404, odd codes 404, bad body 400, stale 409 on every new master", func(t *testing.T) {
		for _, b := range []string{grBase, jbBase, psBase, arBase, dlBase, cfBase} {
			for _, c := range []string{"NOPE999", "%00", "%FF"} {
				if r := call(t, "GET", b+"/"+c, nil); r.Status != 404 {
					t.Errorf("GET %s/%s: %d", b, c, r.Status)
				}
			}
			if r := call(t, "POST", b, "{"); r.Status != 400 {
				t.Errorf("POST %s bad body: %d", b, r.Status)
			}
		}
		if r := call(t, "PUT", psBase+"/"+o.pAsst, map[string]any{"version": 99}); r.Status != 409 || r.str("code") != "STALE" {
			t.Errorf("stale: %d %s", r.Status, r.Raw)
		}
	})
}

func TestJobAndApprovalStress(t *testing.T) {
	t.Run("JS01 parallel heads of one unit and parallel grades on one rank: exactly one wins each, no 500", func(t *testing.T) {
		o := newOrg(t)
		x := must(t, call(t, "POST", ouBase, map[string]any{"name": "Kebun baru", "levelCode": "unit", "parentCode": o.b.c, "unitTypeCode": "EST"}), 201).str("code")
		rank := nextGradeRank()
		var mu sync.Mutex
		heads, grades := map[int]int{}, map[int]int{}
		parallel(60, 60, func(i int) {
			if i%2 == 0 {
				r := call(t, "POST", psBase, map[string]any{"unitCode": x, "jabatanCode": o.jUnit, "head": true})
				mu.Lock()
				heads[r.Status]++
				mu.Unlock()
			} else {
				r := call(t, "POST", grBase, map[string]any{"name": "Sama", "rank": rank})
				mu.Lock()
				grades[r.Status]++
				mu.Unlock()
			}
		})
		if heads[201] != 1 || heads[500] > 0 || grades[201] != 1 || grades[500] > 0 {
			t.Errorf("heads %v, grades %v", heads, grades)
		}
	})

	t.Run("JS02 parallel flows on one document type and branch: exactly one", func(t *testing.T) {
		o := newOrg(t)
		rule := must(t, call(t, "POST", arBase, map[string]any{"name": "Race", "sop": "S", "module": "m", "minSteps": 0, "reach": map[string]any{"kind": "steps", "steps": 1}}), 201).str("code")
		var mu sync.Mutex
		st := map[int]int{}
		parallel(30, 30, func(int) {
			r := call(t, "POST", cfBase, map[string]any{"name": "f", "sop": "S", "ruleCode": rule, "scopeUnitCode": o.b.g, "stepCodes": []any{o.pDirector}})
			mu.Lock()
			st[r.Status]++
			mu.Unlock()
		})
		if st[201] != 1 || st[500] > 0 {
			t.Errorf("statuses %v", st)
		}
	})

	t.Run("JS03 crossing reporting lines at once: never a loop, no 500", func(t *testing.T) {
		for round := range 10 {
			o := newOrg(t)
			a := must(t, call(t, "POST", psBase, map[string]any{"unitCode": o.b.u, "jabatanCode": o.jUnit2}), 201).str("code")
			b := must(t, call(t, "POST", psBase, map[string]any{"unitCode": o.b.u, "jabatanCode": o.jUnit2}), 201).str("code")
			var mu sync.Mutex
			st := map[int]int{}
			parallel(20, 20, func(i int) {
				from, to := a, b
				if i%2 == 1 {
					from, to = b, a
				}
				r := call(t, "PUT", psBase+"/"+from, map[string]any{"reportsToCode": to})
				mu.Lock()
				st[r.Status]++
				mu.Unlock()
			})
			var loops int
			db.QueryRow(context.Background(), `SELECT count(*) FROM pm.position x JOIN pm.position y ON y.id = x.reports_to_id WHERE y.reports_to_id = x.id`).Scan(&loops)
			if loops > 0 || st[500] > 0 {
				t.Errorf("round %d: loops %d, statuses %v", round, loops, st)
			}
		}
	})

	t.Run("JS04 deleting a position while delegations and flows take it: never a live reference to a deleted one", func(t *testing.T) {
		for round := range 8 {
			o := newOrg(t)
			p := must(t, call(t, "POST", psBase, map[string]any{"unitCode": o.b.u, "jabatanCode": o.jUnit2}), 201).str("code")
			rule := must(t, call(t, "POST", arBase, map[string]any{"name": "r", "sop": "S", "module": "m", "minSteps": 0, "reach": map[string]any{"kind": "steps", "steps": 1}}), 201).str("code")
			var mu sync.Mutex
			st := map[int]int{}
			parallel(20, 20, func(i int) {
				var r resp
				switch i % 4 {
				case 0:
					r = call(t, "DELETE", psBase+"/"+p, nil)
				case 1:
					r = call(t, "POST", cfBase, map[string]any{"name": "f", "sop": "S", "ruleCode": rule, "scopeUnitCode": fmt.Sprint([]string{o.b.g, o.b.r, o.b.w, o.b.c, o.b.u}[i%5]), "stepCodes": []any{p}})
				default:
					r = call(t, "POST", dlBase, map[string]any{"delegateEmployeeCode": o.emp, "name": "d", "validFrom": "2026-01-01", "validTo": "2026-01-02", "positionCode": p})
				}
				mu.Lock()
				st[r.Status]++
				mu.Unlock()
			})
			if st[500] > 0 {
				t.Errorf("round %d: %v", round, st)
			}
		}
		var bad int
		db.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM pm.delegation d JOIN pm.position x ON x.id = d.position_id WHERE NOT d.is_deleted AND x.is_deleted)
			+ (SELECT count(*) FROM pm.custom_flow f JOIN pm.custom_flow_step s ON s.custom_flow_id = f.id JOIN pm.position x ON x.id = s.position_id WHERE NOT f.is_deleted AND x.is_deleted)`).Scan(&bad)
		if bad > 0 {
			t.Errorf("%d live references to deleted positions", bad)
		}
	})
}
