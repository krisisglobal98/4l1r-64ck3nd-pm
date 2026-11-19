package orglevel

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
)

const adminID = "00000000-0000-7000-8000-000000000001"

// memRepo keeps levels by id, deleted ones included, as the table does.
type memRepo struct {
	rows   map[string]*memRow
	nextID int
	using  map[string]int // units per level id
}

func (m *memRepo) UnitsUsing(_ context.Context, id string) (int, error) { return m.using[id], nil }
func (m *memRepo) UsedBy(_ context.Context, id string) ([]string, error) {
	if n := m.using[id]; n > 0 {
		return []string{fmt.Sprintf("%d unit", n)}, nil
	}
	return nil, nil
}

type memRow struct {
	l       Level
	deleted bool
}

func (m *memRepo) live(code string) *memRow {
	for _, r := range m.rows {
		if strings.EqualFold(r.l.Code, code) && !r.deleted {
			return r
		}
	}
	return nil
}

func (m *memRepo) List(context.Context) ([]Level, error) {
	out := []Level{}
	for _, r := range m.rows {
		if !r.deleted {
			out = append(out, r.l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out, nil
}
func (m *memRepo) Get(_ context.Context, code string, _ bool) (Level, error) {
	if r := m.live(code); r != nil {
		return r.l, nil
	}
	return Level{}, ErrNotFound
}
func (m *memRepo) RankHolder(_ context.Context, rank int, except string) (string, error) {
	for id, r := range m.rows {
		if r.l.Rank == rank && !r.deleted && id != except {
			return r.l.Name, nil
		}
	}
	return "", nil
}
func (m *memRepo) CodeTaken(_ context.Context, code string) (Taken, error) {
	for _, r := range m.rows {
		if strings.EqualFold(r.l.Code, code) {
			if r.deleted {
				return TakenDeleted, nil
			}
			return TakenLive, nil
		}
	}
	return Free, nil
}
func (m *memRepo) Insert(_ context.Context, in Input, actor string) (Level, error) {
	m.nextID++
	l := Level{Base: record.Base{ID: fmt.Sprintf("id-%d", m.nextID), Code: in.Code, Name: in.Name, Description: in.Description,
		Active: in.Active, ValidFrom: in.ValidFrom, ValidTo: in.ValidTo, CreatedBy: actor, Version: 1}, Rank: in.Rank}
	m.rows[l.ID] = &memRow{l: l}
	return l, nil
}
func (m *memRepo) Update(_ context.Context, id string, in Input, actor string) (Level, error) {
	r, ok := m.rows[id]
	if !ok || r.deleted {
		return Level{}, ErrNotFound
	}
	b := r.l.Base
	b.Name, b.Description, b.Active, b.ValidFrom, b.ValidTo, b.UpdatedBy, b.Version = in.Name, in.Description, in.Active, in.ValidFrom, in.ValidTo, &actor, b.Version+1
	r.l = Level{Base: b, Rank: in.Rank}
	return r.l, nil
}
func (m *memRepo) SoftDelete(_ context.Context, id, _ string) error {
	r, ok := m.rows[id]
	if !ok || r.deleted {
		return ErrNotFound
	}
	r.deleted = true
	return nil
}

// put stores a level as the seed or an older version would have, without the service.
func (m *memRepo) put(code string, rank int, deleted bool) {
	m.nextID++
	id := fmt.Sprintf("id-%d", m.nextID)
	m.rows[id] = &memRow{l: Level{Base: record.Base{ID: id, Code: code, Name: code, Active: true, Version: 1}, Rank: rank}, deleted: deleted}
}

// codes is pm.code_prefix for org_level: prefix ML, three digits.
type codes struct {
	last  int64
	unset bool
}

func (c *codes) Next(_ context.Context, master string) (string, error) {
	if c.unset || master != Table {
		return "", codeprefix.NotSetUp(master)
	}
	c.last++
	return codeprefix.Format("ML", 3, c.last), nil
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
	repo.put("unit", 5, false)
	ev := &events{}
	return &Service{Repo: repo, Tx: noTx{}, Codes: &codes{}, Events: ev}, repo, ev
}

var admin = access.WithUser(context.Background(), access.User{ID: adminID, Username: "admin", Full: true})

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

func firstProblem(t *testing.T, err error) apperr.Problem {
	t.Helper()
	e := appErr(err)
	if e == nil || e.Status != 422 {
		t.Fatalf("want 422, got %v", err)
	}
	return e.Details.(map[string]any)["problems"].([]apperr.Problem)[0]
}

func TestCreateGeneratesTheCodeStoresAndEmits(t *testing.T) {
	s, _, ev := newService(t)
	l, err := s.Create(admin, V{"code": "typed-by-hand", "name": "Divisi", "rank": 6, "description": "Di bawah perusahaan"})
	if err != nil {
		t.Fatal(err)
	}
	if l.Code != "ML001" || l.CreatedBy != adminID || !l.Active || *l.Description != "Di bawah perusahaan" {
		t.Fatalf("not stored as sent, or the code not generated: %+v", l)
	}
	if len(ev.got) != 1 || ev.got[0].Type != "pm.org_level.created" || ev.got[0].AggregateID != l.ID || ev.got[0].AggregateCode != "ML001" || ev.got[0].ActorID != adminID {
		t.Fatalf("events %+v", ev.got)
	}
	if l, _ = s.Create(admin, V{"name": "Seksi", "rank": 7}); l.Code != "ML002" {
		t.Fatalf("next code: %s", l.Code)
	}
}

func TestCreateSkipsCodesAlreadyTaken(t *testing.T) {
	s, repo, _ := newService(t)
	repo.put("ml001", 6, false) // entered by hand before codes were generated (any case)
	repo.put("ML002", 7, true)  // deleted
	l, err := s.Create(admin, V{"name": "Divisi", "rank": 8})
	if err != nil || l.Code != "ML003" {
		t.Fatalf("want ML003: %v %+v", err, l)
	}
}

func TestADeletedCodeIsNeverReused(t *testing.T) {
	s, _, _ := newService(t)
	first, _ := s.Create(admin, V{"name": "Divisi", "rank": 6})
	if err := s.Delete(admin, first.Code); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(admin, first.Code); status(err) != 404 {
		t.Fatalf("deleted level still answered: %v", err)
	}
	list, _ := s.List(admin)
	if len(list) != 1 {
		t.Fatalf("deleted level still listed: %+v", list)
	}
	again, _ := s.Create(admin, V{"name": "Divisi", "rank": 6})
	if again.Code == first.Code {
		t.Fatalf("code %s reused", again.Code)
	}
}

func TestCreateProblemsTakeNoNumber(t *testing.T) {
	s, _, ev := newService(t)
	if _, err := s.Create(admin, V{"name": " ", "rank": 6}); firstProblem(t, err).Field != "name" {
		t.Fatalf("problem %v", err)
	}
	if c := s.Codes.(*codes); c.last != 0 || len(ev.got) != 0 {
		t.Fatalf("a refused save took number %d", c.last)
	}
}

func TestCreateWithoutPrefixIs409(t *testing.T) {
	s, _, _ := newService(t)
	s.Codes.(*codes).unset = true
	_, err := s.Create(admin, V{"name": "Divisi", "rank": 6})
	if e := appErr(err); e == nil || e.Status != 409 || e.Code != "NO_PREFIX" {
		t.Fatalf("want 409 NO_PREFIX, got %v", err)
	}
}

func TestUpdateMergesKeepsTheCodeAndIgnoresWorkflow(t *testing.T) {
	s, repo, ev := newService(t)
	l, err := s.Update(admin, "unit", V{"code": "changed", "name": "Unit", "active": false, "state": "APPROVED", "status": "POSTED"})
	if err != nil {
		t.Fatal(err)
	}
	if l.Code != "unit" || l.Name != "Unit" || l.Rank != 5 || l.Active || l.Version != 2 || l.State != nil || l.Status != nil {
		t.Fatalf("got %+v", l)
	}
	if repo.live("changed") != nil {
		t.Fatal("code must not change")
	}
	if ev.got[0].Type != "pm.org_level.updated" {
		t.Fatalf("events %+v", ev.got)
	}
}

func TestUpdateAndDeleteUnknownAre404(t *testing.T) {
	s, _, _ := newService(t)
	if _, err := s.Update(admin, "nope", V{"name": "X"}); status(err) != 404 {
		t.Fatalf("update: %v", err)
	}
	if err := s.Delete(admin, "nope"); status(err) != 404 {
		t.Fatalf("delete: %v", err)
	}
}

func TestDeleteKeepsBuiltInLevels(t *testing.T) {
	s, repo, _ := newService(t)
	if err := s.Delete(admin, "unit"); status(err) != 409 {
		t.Fatalf("want 409, got %v", err)
	}
	if repo.live("unit") == nil {
		t.Fatal("built-in level was deleted")
	}
}

func TestEveryActionNeedsItsGrant(t *testing.T) {
	s, _, _ := newService(t)
	reader := access.WithUser(context.Background(), access.User{ID: "u", Grants: map[string][]string{Menu: {"read"}}})
	if _, err := s.List(reader); err != nil {
		t.Fatalf("read allowed: %v", err)
	}
	if _, err := s.Create(reader, V{"code": "x", "name": "X", "rank": 9}); status(err) != 403 {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Update(reader, "unit", V{"name": "X"}); status(err) != 403 {
		t.Fatalf("update: %v", err)
	}
	if err := s.Delete(reader, "unit"); status(err) != 403 {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.List(context.Background()); status(err) != 401 {
		t.Fatalf("no user: %v", err)
	}
}

func TestLookupIgnoresCase(t *testing.T) {
	s, _, _ := newService(t)
	if l, err := s.Get(admin, "UNIT"); err != nil || l.Code != "unit" {
		t.Fatalf("get UNIT: %v %+v", err, l)
	}
	if err := s.Delete(admin, "Unit"); status(err) != 409 {
		t.Fatalf("built-in in another case must stay: %v", err)
	}
}

func TestOddKeysAreNotFound(t *testing.T) {
	s, _, _ := newService(t)
	for _, k := range []string{"", "a\x00b", "\xff", strings.Repeat("x", 161)} {
		if _, err := s.Get(admin, k); status(err) != 404 {
			t.Errorf("get %q: %v", k, err)
		}
		if _, err := s.Update(admin, k, V{"name": "x"}); status(err) != 404 {
			t.Errorf("update %q: %v", k, err)
		}
		if err := s.Delete(admin, k); status(err) != 404 {
			t.Errorf("delete %q: %v", k, err)
		}
	}
}

func TestOneLiveLevelPerRank(t *testing.T) {
	s, _, _ := newService(t)
	_, err := s.Create(admin, V{"name": "Divisi", "rank": 5})
	if p := firstProblem(t, err); p.Field != "rank" || p.Message != "Urutan 5 sudah dipakai level unit." {
		t.Fatalf("problem %+v", p)
	}
	d, _ := s.Create(admin, V{"name": "Divisi", "rank": 6})
	if _, err := s.Update(admin, d.Code, V{"rank": 5}); status(err) != 422 {
		t.Fatalf("update onto a taken rank: %v", err)
	}
	if _, err := s.Update(admin, d.Code, V{"rank": 6, "name": "Divisi Usaha"}); err != nil {
		t.Fatalf("keeping its own rank: %v", err)
	}
	_ = s.Delete(admin, d.Code)
	if _, err := s.Create(admin, V{"name": "Lagi", "rank": 6}); err != nil {
		t.Fatalf("a deleted level frees its rank: %v", err)
	}
}

func TestALevelInUseKeepsItsRankAndStays(t *testing.T) {
	s, repo, _ := newService(t)
	d, _ := s.Create(admin, V{"name": "Divisi", "rank": 6})
	repo.using = map[string]int{d.ID: 2}
	_, err := s.Update(admin, d.Code, V{"rank": 7})
	if p := firstProblem(t, err); p.Field != "rank" || p.Message != "Urutan tidak dapat diubah selama level ini dipakai 2 unit." {
		t.Fatalf("problem %+v", p)
	}
	if _, err := s.Update(admin, d.Code, V{"name": "Divisi Usaha", "active": false}); err != nil {
		t.Fatalf("name and status may change: %v", err)
	}
	if err := s.Delete(admin, d.Code); appErr(err) == nil || appErr(err).Code != "IN_USE" {
		t.Fatalf("delete in use: %v", err)
	}
	repo.using = nil
	if err := s.Delete(admin, d.Code); err != nil {
		t.Fatalf("delete unused: %v", err)
	}
}

func TestStaleVersionIsRefused(t *testing.T) {
	s, _, ev := newService(t)
	if _, err := s.Update(admin, "unit", V{"name": "A", "version": 1}); err != nil {
		t.Fatalf("current version: %v", err)
	}
	ev.got = nil
	_, err := s.Update(admin, "unit", V{"name": "B", "version": 1})
	if e := appErr(err); e == nil || e.Status != 409 || e.Code != "STALE" {
		t.Fatalf("want 409 STALE, got %v", err)
	}
	if l, _ := s.Get(admin, "unit"); l.Name != "A" || len(ev.got) != 0 {
		t.Fatalf("stale change applied: %+v", l)
	}
	if _, err := s.Update(admin, "unit", V{"name": "C", "version": "x"}); status(err) != 422 {
		t.Fatalf("bad version: %v", err)
	}
}
