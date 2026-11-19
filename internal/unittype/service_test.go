package unittype

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

type V = record.Values

const adminID = "00000000-0000-7000-8000-000000000001"

var admin = access.WithUser(context.Background(), access.User{ID: adminID, Username: "admin", Full: true})

// memRepo keeps unit types by id, deleted ones included, as the table does.
type memRepo struct {
	rows   map[string]*memRow
	nextID int
	using  map[string]int // units per unit type id
}

func (m *memRepo) UsedBy(_ context.Context, id string) ([]string, error) {
	if n := m.using[id]; n > 0 {
		return []string{fmt.Sprintf("%d unit", n)}, nil
	}
	return nil, nil
}

// jabatan: EM is live and active, OLD inactive.
func (m *memRepo) Jabatan(_ context.Context, code string) (refs.Ref, error) {
	switch strings.ToUpper(code) {
	case "EM":
		return refs.Ref{ID: "j-em", Code: "EM", Name: "Estate Manager", Active: true}, nil
	case "OLD":
		return refs.Ref{ID: "j-old", Code: "OLD", Name: "Lama", Active: false}, nil
	}
	return refs.Ref{}, refs.ErrNotFound
}

type memRow struct {
	u       UnitType
	deleted bool
}

func (m *memRepo) put(code string, deleted bool) {
	m.nextID++
	id := fmt.Sprintf("id-%d", m.nextID)
	m.rows[id] = &memRow{u: UnitType{Base: record.Base{ID: id, Code: code, Name: code, Active: true, Version: 1}}, deleted: deleted}
}

func (m *memRepo) live(code string) *memRow {
	for _, r := range m.rows {
		if strings.EqualFold(r.u.Code, code) && !r.deleted {
			return r
		}
	}
	return nil
}

func (m *memRepo) List(context.Context) ([]UnitType, error) {
	out := []UnitType{}
	for _, r := range m.rows {
		if !r.deleted {
			out = append(out, r.u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out, nil
}
func (m *memRepo) Get(_ context.Context, code string, _ bool) (UnitType, error) {
	if r := m.live(code); r != nil {
		return r.u, nil
	}
	return UnitType{}, ErrNotFound
}
func (m *memRepo) CodeTaken(_ context.Context, code string) (Taken, error) {
	for _, r := range m.rows {
		if strings.EqualFold(r.u.Code, code) {
			if r.deleted {
				return TakenDeleted, nil
			}
			return TakenLive, nil
		}
	}
	return Free, nil
}
func (m *memRepo) Insert(_ context.Context, in Input, actor string) (UnitType, error) {
	m.nextID++
	u := UnitType{Base: record.Base{ID: fmt.Sprintf("id-%d", m.nextID), Code: in.Code, Name: in.Name, Description: in.Description,
		Active: in.Active, ValidFrom: in.ValidFrom, ValidTo: in.ValidTo, CreatedBy: actor, Version: 1}, HeadJabatanID: in.HeadJabatanID, HeadJabatanCode: in.HeadJabatanCode}
	m.rows[u.ID] = &memRow{u: u}
	return u, nil
}
func (m *memRepo) Update(_ context.Context, id string, in Input, actor string) (UnitType, error) {
	r, ok := m.rows[id]
	if !ok || r.deleted {
		return UnitType{}, ErrNotFound
	}
	b := r.u.Base
	b.Name, b.Description, b.Active, b.ValidFrom, b.ValidTo, b.UpdatedBy, b.Version = in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, &actor, b.Version+1
	r.u = UnitType{Base: b, HeadJabatanID: in.HeadJabatanID, HeadJabatanCode: in.HeadJabatanCode}
	return r.u, nil
}
func (m *memRepo) SoftDelete(_ context.Context, id, _ string) error {
	r, ok := m.rows[id]
	if !ok || r.deleted {
		return ErrNotFound
	}
	r.deleted = true
	return nil
}

type codes struct {
	last  int64
	unset bool
}

func (c *codes) Next(_ context.Context, master string) (string, error) {
	if c.unset || master != Table {
		return "", codeprefix.NotSetUp(master)
	}
	c.last++
	return codeprefix.Format("MJU", 3, c.last), nil
}

type noTx struct{}

func (noTx) InTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type events struct{ got []outbox.Event }

func (e *events) Write(_ context.Context, ev outbox.Event) error {
	e.got = append(e.got, ev)
	return nil
}

func newService(t *testing.T) (*Service, *memRepo, *events) {
	t.Helper()
	repo := &memRepo{rows: map[string]*memRow{}}
	repo.put("EST", false)
	ev := &events{}
	return &Service{Repo: repo, Tx: noTx{}, Codes: &codes{}, Events: ev}, repo, ev
}

func appErr(err error) *apperr.Error {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

func status(err error) int {
	if e := appErr(err); e != nil {
		return e.Status
	}
	return 0
}

func TestHeadJabatanByCode(t *testing.T) {
	s, _, _ := newService(t)
	u, err := s.Create(admin, V{"name": "Kebun Bibit", "headJabatanCode": "em"})
	if err != nil || u.HeadJabatanID == nil || *u.HeadJabatanID != "j-em" {
		t.Fatalf("head: %v %+v", err, u)
	}
	for code, msg := range map[string]string{"NOPE": `Jabatan "NOPE" tidak ditemukan.`, "OLD": "Jabatan Lama nonaktif; pilih jabatan lain."} {
		_, err := s.Create(admin, V{"name": "x", "headJabatanCode": code})
		if e := appErr(err); e == nil || e.Status != 422 || e.Details.(map[string]any)["problems"].([]apperr.Problem)[0].Message != msg {
			t.Errorf("%s: %v", code, err)
		}
	}
	if u, err := s.Update(admin, u.Code, V{"headJabatanCode": nil}); err != nil || u.HeadJabatanID != nil {
		t.Fatalf("clear the head: %v %+v", err, u)
	}
}

func TestCreateGeneratesTheCode(t *testing.T) {
	s, _, ev := newService(t)
	u, err := s.Create(admin, V{"code": "KBN", "name": "Kebun Bibit", "headJabatanId": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if u.Code != "MJU001" || u.Name != "Kebun Bibit" || !u.Active || u.HeadJabatanID != nil || u.CreatedBy != adminID {
		t.Fatalf("got %+v", u)
	}
	if len(ev.got) != 1 || ev.got[0].Type != "pm.unit_type.created" || ev.got[0].AggregateCode != "MJU001" {
		t.Fatalf("events %+v", ev.got)
	}
}

func TestCreateSkipsTakenCodesAndRefusesProblems(t *testing.T) {
	s, repo, _ := newService(t)
	repo.put("mju001", true)
	if u, err := s.Create(admin, V{"name": "A"}); err != nil || u.Code != "MJU002" {
		t.Fatalf("want MJU002: %v %+v", err, u)
	}
	if _, err := s.Create(admin, V{"name": " "}); status(err) != 422 {
		t.Fatalf("blank name: %v", err)
	}
	if s.Codes.(*codes).last != 2 {
		t.Fatal("a refused save took a number")
	}
	s.Codes.(*codes).unset = true
	if _, err := s.Create(admin, V{"name": "A"}); appErr(err) == nil || appErr(err).Code != "NO_PREFIX" {
		t.Fatalf("no prefix: %v", err)
	}
}

func TestUpdateKeepsCodeIgnoresWorkflowAndChecksVersion(t *testing.T) {
	s, _, _ := newService(t)
	u, err := s.Update(admin, "est", V{"code": "X", "name": "Kebun Inti 2", "status": "POSTED", "headJabatanId": "x", "version": 1}) // ids are never taken
	if err != nil || u.Code != "EST" || u.Name != "Kebun Inti 2" || u.Status != nil || u.HeadJabatanID != nil || u.Version != 2 {
		t.Fatalf("update: %v %+v", err, u)
	}
	if _, err := s.Update(admin, "EST", V{"name": "Basi", "version": 1}); appErr(err) == nil || appErr(err).Code != "STALE" {
		t.Fatalf("stale: %v", err)
	}
}

func TestDeleteSoftKeepsTypesInUseAndNeverReusesCodes(t *testing.T) {
	s, repo, _ := newService(t)
	repo.using = map[string]int{repo.live("EST").u.ID: 3}
	if err := s.Delete(admin, "est"); appErr(err) == nil || appErr(err).Status != 409 || appErr(err).Message != "Jenis unit EST masih dipakai 3 unit; tidak dapat dihapus. Nonaktifkan saja." {
		t.Fatalf("in use: %v", err)
	}
	a, _ := s.Create(admin, V{"name": "A"})
	if err := s.Delete(admin, a.Code); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(admin, a.Code); status(err) != 404 {
		t.Fatalf("deleted still answered: %v", err)
	}
	if b, _ := s.Create(admin, V{"name": "B"}); b.Code == a.Code {
		t.Fatalf("code %s reused", a.Code)
	}
	for _, k := range []string{"nope", "a\x00", "\xff"} {
		if err := s.Delete(admin, k); status(err) != 404 {
			t.Errorf("delete %q: %v", k, err)
		}
	}
}

func TestEveryActionNeedsItsGrant(t *testing.T) {
	s, _, _ := newService(t)
	reader := access.WithUser(context.Background(), access.User{ID: "u", Grants: map[string][]string{Menu: {"read"}}})
	if _, err := s.List(reader); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := s.Create(reader, V{"name": "X"}); status(err) != 403 {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Update(reader, "EST", V{"name": "X"}); status(err) != 403 {
		t.Fatalf("update: %v", err)
	}
	if err := s.Delete(reader, "EST"); status(err) != 403 {
		t.Fatalf("delete: %v", err)
	}
	other := access.WithUser(context.Background(), access.User{ID: "u", Grants: map[string][]string{"pm.orgLevel": {"read"}}})
	if _, err := s.List(other); status(err) != 403 {
		t.Fatalf("a grant on another master: %v", err)
	}
}
