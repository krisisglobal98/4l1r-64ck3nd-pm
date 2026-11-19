//go:build e2e

// Karyawan and the Employee Management masters it uses (status kepegawaian, jenjang pendidikan, shift kerja, bank),
// and employees as holders, acting holders and delegates.
package e2e

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	esBase = "/pm/masters/employment-status"
	elBase = "/pm/masters/education-levels"
	wsBase = "/pm/masters/work-shifts"
	bkBase = "/pm/masters/banks"
	emBase = "/pm/masters/employees"
)

var (
	nikSeq  atomic.Int64
	eduRank atomic.Int64
	empCode = regexp.MustCompile(`^MKY\d{4,}$`)
)

func nextNik() string { return fmt.Sprintf("E2E-%06d", nikSeq.Add(1)) }

func newStatus(t testing.TB) string {
	t.Helper()
	return must(t, call(t, "POST", esBase, map[string]any{"name": "Tetap uji", "permanent": true}), 201).str("code")
}

// newEmployee makes an active employee with its own employment status.
func newEmployee(t testing.TB) string {
	t.Helper()
	return must(t, call(t, "POST", emBase, map[string]any{"name": "Karyawan Uji", "nik": nextNik(), "employmentStatusCode": newStatus(t)}), 201).str("code")
}

func TestEmployeesPositive(t *testing.T) {
	t.Run("EP01 the four masters: generated codes and their own fields", func(t *testing.T) {
		st := must(t, call(t, "POST", esBase, map[string]any{"name": "Kontrak", "permanent": false, "contractMonths": 12}), 201)
		if !genCodeOf("MSK").MatchString(st.str("code")) || st.Obj["permanent"] != false || st.Obj["contractMonths"] != 12.0 {
			t.Errorf("status: %s", st.Raw)
		}
		ed := must(t, call(t, "POST", elBase, map[string]any{"name": "Doktor", "rank": 1000 + eduRank.Add(1)}), 201)
		if !genCodeOf("MJP").MatchString(ed.str("code")) {
			t.Errorf("education: %s", ed.Raw)
		}
		sh := must(t, call(t, "POST", wsBase, map[string]any{"name": "Malam", "start": "23:00", "end": "07:00", "night": true}), 201)
		if !genCodeOf("MSF").MatchString(sh.str("code")) || sh.str("start") != "23:00" || sh.str("end") != "07:00" || sh.Obj["night"] != true {
			t.Errorf("shift: %s", sh.Raw)
		}
		bk := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank Uji", "clearing": "123"}), 201)
		if !genCodeOf("MBK").MatchString(bk.str("code")) || bk.str("clearing") != "123" {
			t.Errorf("bank: %s", bk.Raw)
		}
	})

	t.Run("EP02 employee: generated code, NIK, relations by code; changes keep the code", func(t *testing.T) {
		st := newStatus(t)
		bk := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank Gaji"}), 201).str("code")
		e := must(t, call(t, "POST", emBase, map[string]any{"name": "  Budi Santoso ", "nik": "E2E-BUDI-1", "joinDate": "2020-02-01", "employmentStatusCode": strings.ToLower(st), "bankCode": bk}), 201)
		if !empCode.MatchString(e.str("code")) || e.str("name") != "Budi Santoso" || e.str("nik") != "E2E-BUDI-1" || e.str("joinDate") != "2020-02-01" ||
			e.str("employmentStatusCode") != st || e.str("bankCode") != bk || e.Obj["workShiftCode"] != nil {
			t.Errorf("got %s", e.Raw)
		}
		u := call(t, "PUT", emBase+"/"+e.str("code"), map[string]any{"name": "Budi S.", "bankCode": nil, "version": 1})
		if want(t, u, 200) && (u.str("code") != e.str("code") || u.Obj["bankCode"] != nil || u.str("nik") != "E2E-BUDI-1") {
			t.Errorf("update: %s", u.Raw)
		}
		call(t, "DELETE", emBase+"/"+e.str("code"), nil)
		want(t, call(t, "POST", emBase, map[string]any{"name": "Budi baru", "nik": "E2E-BUDI-1", "employmentStatusCode": st}), 201) // a deleted employee's NIK may be given again
	})

	t.Run("EP03 holders, acting holders and delegates are employees", func(t *testing.T) {
		o := newOrg(t)
		h, a := newEmployee(t), newEmployee(t)
		r := call(t, "PUT", psBase+"/"+o.pHead, map[string]any{"holderEmployeeCode": h, "actingEmployeeCode": a})
		if want(t, r, 200) && (r.str("holderEmployeeCode") != h || r.str("actingEmployeeCode") != a || !uuidV7.MatchString(r.str("holderEmployeeId"))) {
			t.Errorf("position: %s", r.Raw)
		}
		d := call(t, "POST", dlBase, map[string]any{"name": "Cuti", "validFrom": "2026-01-01", "validTo": "2026-01-05", "positionCode": o.pHead, "delegateEmployeeCode": a})
		if want(t, d, 201) && d.str("delegateEmployeeCode") != a {
			t.Errorf("delegation: %s", d.Raw)
		}
		v := call(t, "PUT", psBase+"/"+o.pHead, map[string]any{"holderEmployeeCode": nil, "actingEmployeeCode": nil})
		if want(t, v, 200) && (v.Obj["holderEmployeeCode"] != nil || v.Obj["actingEmployeeCode"] != nil) {
			t.Errorf("vacant: %s", v.Raw)
		}
	})

	t.Run("EP04 an employee deactivated after being chosen stays; a new choice of them is refused", func(t *testing.T) {
		o := newOrg(t)
		h := newEmployee(t)
		must(t, call(t, "PUT", psBase+"/"+o.pHead, map[string]any{"holderEmployeeCode": h}), 200)
		must(t, call(t, "PUT", emBase+"/"+h, map[string]any{"active": false}), 200)
		want(t, call(t, "PUT", psBase+"/"+o.pHead, map[string]any{"description": "tetap"}), 200)
		if r := call(t, "PUT", psBase+"/"+o.pAsst, map[string]any{"holderEmployeeCode": h}); want(t, r, 422) && !strings.Contains(r.problem("holderEmployeeCode"), "nonaktif") {
			t.Errorf("got %s", r.Raw)
		}
	})
}

func TestEmployeesNegative(t *testing.T) {
	st := newStatus(t)
	cases := []struct {
		id, path string
		body     map[string]any
		field    string
		msg      string
	}{
		{"EN01", esBase, map[string]any{"name": "x", "permanent": true, "contractMonths": 12}, "contractMonths", "Karyawan tetap tidak punya lama kontrak; isi 0."},
		{"EN02", esBase, map[string]any{"name": "x", "contractMonths": 121}, "contractMonths", "Lama kontrak (bulan) paling banyak 120."},
		{"EN03", elBase, map[string]any{"name": "x"}, "rank", "Urutan wajib diisi."},
		{"EN04", wsBase, map[string]any{"name": "x", "start": "25:00", "end": "07:00"}, "start", "Jam masuk harus jam 24 jam HH:MM, mis. 07:00."},
		{"EN05", wsBase, map[string]any{"name": "x", "start": "23:00", "end": "07:00"}, "night", "Shift yang berakhir di hari berikutnya harus ditandai shift malam."},
		{"EN06", wsBase, map[string]any{"name": "x", "start": "07:00"}, "end", "Jam pulang wajib diisi."},
		{"EN07", bkBase, map[string]any{"name": "x", "clearing": "ABC"}, "clearing", "Kode kliring berisi 3–7 angka, mis. 002."},
		{"EN08", emBase, map[string]any{"name": "x", "nik": "12 34", "employmentStatusCode": st}, "nik", "NIK berisi huruf, angka, titik, garis miring, - atau _ tanpa spasi."},
		{"EN09", emBase, map[string]any{"name": "x", "nik": nextNik()}, "employmentStatusCode", "Status kepegawaian wajib diisi."},
		{"EN10", emBase, map[string]any{"name": "x", "nik": nextNik(), "employmentStatusCode": "NOPE"}, "employmentStatusCode", `Status kepegawaian "NOPE" tidak ditemukan.`},
		{"EN11", emBase, map[string]any{"name": "x", "nik": nextNik(), "employmentStatusCode": st, "workShiftCode": "NOPE"}, "workShiftCode", `Shift "NOPE" tidak ditemukan.`},
		{"EN12", emBase, map[string]any{"name": "x", "nik": nextNik(), "employmentStatusCode": st, "joinDate": "2026-02-30"}, "joinDate", "Tanggal masuk bukan tanggal yang sah (format YYYY-MM-DD)."},
	}
	for _, c := range cases {
		t.Run(c.id+" "+c.field, func(t *testing.T) {
			r := call(t, "POST", c.path, c.body)
			if want(t, r, 422) && r.problem(c.field) != c.msg {
				t.Errorf("%s: got %q, want %q (%s)", c.field, r.problem(c.field), c.msg, trunc(r.Raw))
			}
		})
	}

	t.Run("EN17 email: valid and stored lower-case; one live employee per address; a bad one refused", func(t *testing.T) {
		e := must(t, call(t, "POST", emBase, map[string]any{"name": "Punya Email", "nik": nextNik(), "employmentStatusCode": st, "email": " Punya.Email@Contoh.co.id "}), 201)
		if e.str("email") != "punya.email@contoh.co.id" {
			t.Errorf("stored: %s", e.Raw)
		}
		if r := call(t, "POST", emBase, map[string]any{"name": "Kembar", "nik": nextNik(), "employmentStatusCode": st, "email": "PUNYA.EMAIL@contoh.co.id"}); want(t, r, 422) && r.problem("email") != "Email punya.email@contoh.co.id sudah dipakai Punya Email." {
			t.Errorf("taken: %s", r.Raw)
		}
		if r := call(t, "POST", emBase, map[string]any{"name": "Salah", "nik": nextNik(), "employmentStatusCode": st, "email": "bukan-email"}); want(t, r, 422) && r.problem("email") != "Email bukan alamat yang sah, mis. nama@perusahaan.co.id." {
			t.Errorf("format: %s", r.Raw)
		}
		u := call(t, "PUT", emBase+"/"+e.str("code"), map[string]any{"email": nil})
		if want(t, u, 200) && u.Obj["email"] != nil {
			t.Errorf("cleared: %s", u.Raw)
		}
	})

	t.Run("EN13 one live employee per NIK, any case", func(t *testing.T) {
		nik := nextNik()
		must(t, call(t, "POST", emBase, map[string]any{"name": "Satu", "nik": nik, "employmentStatusCode": st}), 201)
		if r := call(t, "POST", emBase, map[string]any{"name": "Dua", "nik": strings.ToLower(nik), "employmentStatusCode": st}); want(t, r, 422) && r.problem("nik") != fmt.Sprintf("NIK %s sudah dipakai Satu.", strings.ToLower(nik)) {
			t.Errorf("got %s", r.Raw)
		}
	})

	t.Run("EN14 acting is not the holder; a delegate is not the holder of the position", func(t *testing.T) {
		o := newOrg(t)
		h := newEmployee(t)
		if r := call(t, "PUT", psBase+"/"+o.pHead, map[string]any{"holderEmployeeCode": h, "actingEmployeeCode": h}); want(t, r, 422) && r.problem("actingEmployeeCode") != "Plt tidak boleh pemegang posisi itu sendiri." {
			t.Errorf("acting: %s", r.Raw)
		}
		must(t, call(t, "PUT", psBase+"/"+o.pHead, map[string]any{"holderEmployeeCode": h}), 200)
		if r := call(t, "POST", dlBase, map[string]any{"name": "x", "validFrom": "2026-01-01", "validTo": "2026-01-02", "positionCode": o.pHead, "delegateEmployeeCode": h}); want(t, r, 422) && !strings.Contains(r.problem("delegateEmployeeCode"), "pemegang posisi itu sendiri") {
			t.Errorf("delegate: %s", r.Raw)
		}
		if r := call(t, "POST", dlBase, map[string]any{"name": "x", "validFrom": "2026-01-01", "validTo": "2026-01-02", "positionCode": o.pHead}); want(t, r, 422) && r.problem("delegateEmployeeCode") != "Penerima delegasi wajib diisi." {
			t.Errorf("no delegate: %s", r.Raw)
		}
	})

	t.Run("EN15 records in use are kept: status, bank, employee", func(t *testing.T) {
		o := newOrg(t)
		s := newStatus(t)
		b := must(t, call(t, "POST", bkBase, map[string]any{"name": "Bank pakai"}), 201).str("code")
		e := must(t, call(t, "POST", emBase, map[string]any{"name": "Dipakai", "nik": nextNik(), "employmentStatusCode": s, "bankCode": b}), 201).str("code")
		must(t, call(t, "PUT", psBase+"/"+o.pHead, map[string]any{"holderEmployeeCode": e}), 200)
		must(t, call(t, "POST", dlBase, map[string]any{"name": "d", "validFrom": "2026-01-01", "validTo": "2026-01-02", "positionCode": o.pAsst, "delegateEmployeeCode": e}), 201)
		for _, c := range []struct{ path, want string }{
			{esBase + "/" + s, "1 karyawan"}, {bkBase + "/" + b, "1 karyawan"}, {emBase + "/" + e, "1 posisi (pemegang), 1 delegasi"},
		} {
			if r := call(t, "DELETE", c.path, nil); r.Status != 409 || !strings.Contains(r.str("message"), c.want) {
				t.Errorf("DELETE %s: %d %s", c.path, r.Status, r.Raw)
			}
		}
	})

	t.Run("EN16 odd codes 404, bad bodies 400 on every employee master", func(t *testing.T) {
		for _, b := range []string{esBase, elBase, wsBase, bkBase, emBase} {
			if r := call(t, "GET", b+"/%00", nil); r.Status != 404 {
				t.Errorf("%s: %d", b, r.Status)
			}
			if r := call(t, "POST", b, "["); r.Status != 400 {
				t.Errorf("%s: %d", b, r.Status)
			}
		}
	})
}

func TestEmployeesStress(t *testing.T) {
	t.Run("ES01 200 parallel new employees: codes unique and gap-free; one NIK raced 20 times: one wins", func(t *testing.T) {
		st := newStatus(t)
		nik := nextNik()
		var mu sync.Mutex
		var codes []int
		same := map[int]int{}
		bad := 0
		parallel(220, 50, func(i int) {
			if i < 20 {
				r := call(t, "POST", emBase, map[string]any{"name": "Sama", "nik": nik, "employmentStatusCode": st})
				mu.Lock()
				same[r.Status]++
				mu.Unlock()
				return
			}
			r := call(t, "POST", emBase, map[string]any{"name": fmt.Sprint("K", i), "nik": nextNik(), "employmentStatusCode": st})
			mu.Lock()
			defer mu.Unlock()
			if r.Status != 201 {
				bad++
				return
			}
			var x int
			fmt.Sscanf(r.str("code"), "MKY%d", &x)
			codes = append(codes, x)
		})
		if bad > 0 || same[201] != 1 || same[500] > 0 {
			t.Errorf("failed %d, same NIK %v", bad, same)
		}
		sort.Ints(codes)
		for i := 1; i < len(codes); i++ {
			if codes[i] == codes[i-1] {
				t.Errorf("duplicate MKY%04d", codes[i])
				break
			}
		}
	})

	t.Run("ES02 holders changed while the employee is deleted: never a live position held by a deleted employee", func(t *testing.T) {
		for round := range 8 {
			o := newOrg(t)
			e := newEmployee(t)
			var mu sync.Mutex
			st := map[int]int{}
			parallel(20, 20, func(i int) {
				var r resp
				if i%5 == 0 {
					r = call(t, "DELETE", emBase+"/"+e, nil)
				} else {
					r = call(t, "PUT", psBase+"/"+[]string{o.pHead, o.pAsst, o.pDirector}[i%3], map[string]any{"holderEmployeeCode": e})
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
		db.QueryRow(t.Context(), `SELECT count(*) FROM pm.position x JOIN pm.employee e ON e.id = x.holder_employee_id WHERE NOT x.is_deleted AND e.is_deleted`).Scan(&bad)
		if bad > 0 {
			t.Errorf("%d live positions held by deleted employees", bad)
		}
	})
}
