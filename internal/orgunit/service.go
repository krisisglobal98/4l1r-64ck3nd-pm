package orgunit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// ErrNotFound is returned by a Repository when no live (not deleted) row has the code.
var ErrNotFound = errors.New("org unit not found")

// ErrDuplicate is returned by a Repository when another unit already has the code (any case).
var ErrDuplicate = errors.New("org unit code taken")

// Taken says who holds a code already.
type Taken int

const (
	Free Taken = iota
	TakenLive
	TakenDeleted
)

// LevelRef is a live level as a unit refers to it.
type LevelRef struct {
	ID, Code, Name string
	Rank           int
	Active         bool
}

// UnitRef is a live unit as a child refers to it.
type UnitRef struct {
	ID, Code, Name string
	LevelID        string
	Active         bool
}

// TypeRef is a live unit type.
type TypeRef struct {
	ID, Code, Name string
	Active         bool
}

// Repository is the storage port of this master. The *Ref lookups find live rows by code in any case and lock
// them against change until the transaction ends (a parent or a type cannot be deleted under a new child).
type Repository interface {
	List(ctx context.Context) ([]Unit, error)
	Get(ctx context.Context, code string, forUpdate bool) (Unit, error)
	CodeTaken(ctx context.Context, code string) (Taken, error)
	Insert(ctx context.Context, in Input, refs Refs, actorID string) (Unit, error)
	Update(ctx context.Context, id string, in Input, refs Refs, actorID string) (Unit, error)
	SoftDelete(ctx context.Context, id, actorID string) error

	Level(ctx context.Context, code string) (LevelRef, error)
	// LevelAbove is the live level with the highest rank below rank (false at the top of the tree).
	LevelAbove(ctx context.Context, rank int) (LevelRef, bool, error)
	Unit(ctx context.Context, code string) (UnitRef, error)
	UnitType(ctx context.Context, code string) (TypeRef, error)
	// Children counts the live units directly under id.
	Children(ctx context.Context, id string) (int, error)
	// IsUnder reports whether unit id is ancestorID or one of the units below it.
	IsUnder(ctx context.Context, id, ancestorID string) (bool, error)
	// UsedBy says who still uses the unit (units under it, positions, approval custom flows).
	UsedBy(ctx context.Context, id string) ([]string, error)
}

// TxRunner runs a function in one database transaction.
type TxRunner interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Codes generates the code of a new record (pm.code_prefix), inside the transaction that saves it.
type Codes interface {
	Next(ctx context.Context, master string) (string, error)
}

// Events records what changed, in the same transaction (outbox).
type Events interface {
	Write(ctx context.Context, e outbox.Event) error
}

type Service struct {
	Repo   Repository
	Tx     TxRunner
	Codes  Codes
	Events Events
}

const maxCodeTries = 20

func (s *Service) List(ctx context.Context) ([]Unit, error) {
	if _, err := access.Require(ctx, Menu, "read"); err != nil {
		return nil, err
	}
	return s.Repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, code string) (Unit, error) {
	if _, err := access.Require(ctx, Menu, "read"); err != nil {
		return Unit{}, err
	}
	if !record.LookupKey(code) {
		return Unit{}, apperr.NotFound()
	}
	u, err := s.Repo.Get(ctx, code, false)
	return u, notFound(err)
}

func (s *Service) Create(ctx context.Context, body record.Values) (Unit, error) {
	u, err := access.Require(ctx, Menu, "create")
	if err != nil {
		return Unit{}, err
	}
	var saved Unit
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		in, problems := Parse(body)
		refs, problems, err := s.resolve(ctx, in, nil, problems)
		if err != nil {
			return err
		}
		if len(problems) > 0 {
			return apperr.Validation(problems)
		}
		if in.Code, err = s.nextCode(ctx); err != nil {
			return err
		}
		if saved, err = s.Repo.Insert(ctx, in, refs, u.ID); err != nil {
			if errors.Is(err, ErrDuplicate) {
				return apperr.Conflict("CODE_TAKEN", fmt.Sprintf("Kode %s sudah dipakai. Simpan ulang untuk mendapat kode berikutnya.", in.Code))
			}
			return err
		}
		return s.emit(ctx, "created", saved, u)
	})
	return saved, err
}

// Update merges body onto the stored unit. The code is fixed after creation and the workflow fields belong to the
// approval flow; the relations are checked again, as on create.
func (s *Service) Update(ctx context.Context, code string, body record.Values) (Unit, error) {
	u, err := access.Require(ctx, Menu, "update")
	if err != nil {
		return Unit{}, err
	}
	if !record.LookupKey(code) {
		return Unit{}, apperr.NotFound()
	}
	var saved Unit
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		stored, err := s.Repo.Get(ctx, code, true)
		if err != nil {
			return notFound(err)
		}
		var problems record.Problems
		if want, ok := record.ExpectedVersion(body, &problems); ok && want != stored.Version {
			return apperr.Conflict("STALE", fmt.Sprintf("Unit %s sudah diubah pengguna lain sejak Anda membukanya. Muat ulang, lalu ulangi perubahan Anda.", stored.Name))
		}
		v := valuesOf(stored)
		for k, x := range body {
			v[k] = x
		}
		v["code"] = stored.Code
		in, more := Parse(v)
		refs, problems, err := s.resolve(ctx, in, &stored, append(problems, more...))
		if err != nil {
			return err
		}
		if len(problems) > 0 {
			return apperr.Validation(problems)
		}
		if saved, err = s.Repo.Update(ctx, stored.ID, in, refs, u.ID); err != nil {
			return notFound(err)
		}
		return s.emit(ctx, "updated", saved, u)
	})
	return saved, err
}

// Delete is a soft delete. A unit still used (units under it, positions, approval custom flows) is kept.
func (s *Service) Delete(ctx context.Context, code string) error {
	u, err := access.Require(ctx, Menu, "delete")
	if err != nil {
		return err
	}
	if !record.LookupKey(code) {
		return apperr.NotFound()
	}
	return s.Tx.InTx(ctx, func(ctx context.Context) error {
		stored, err := s.Repo.Get(ctx, code, true)
		if err != nil {
			return notFound(err)
		}
		used, err := s.Repo.UsedBy(ctx, stored.ID)
		if err != nil {
			return err
		}
		if len(used) > 0 {
			return apperr.InUse(refs.InUseMessage("Unit", stored.Name, used))
		}
		if err := s.Repo.SoftDelete(ctx, stored.ID, u.ID); err != nil {
			return notFound(err)
		}
		return s.emit(ctx, "deleted", stored, u)
	})
}

// resolve checks the relations of in against the tree and returns their ids. stored is the unit being changed
// (nil on create): a relation it already has may stay even when that record was deactivated since.
func (s *Service) resolve(ctx context.Context, in Input, stored *Unit, problems record.Problems) (Refs, record.Problems, error) {
	var refs Refs
	add := func(field, msg string) { problems.Add(field, msg) }
	same := func(had *string, code *string) bool {
		return stored != nil && had != nil && code != nil && strings.EqualFold(*had, *code)
	}
	if problems.Has("levelCode") {
		return refs, problems, nil
	}

	level, err := s.Repo.Level(ctx, in.LevelCode)
	switch {
	case errors.Is(err, ErrNotFound):
		add("levelCode", fmt.Sprintf("Level \"%s\" tidak ditemukan.", in.LevelCode))
		return refs, problems, nil
	case err != nil:
		return refs, problems, err
	}
	refs.LevelID = level.ID
	levelChanged := stored == nil || stored.LevelID != level.ID
	if !level.Active && levelChanged {
		add("levelCode", fmt.Sprintf("Level %s nonaktif; pilih level lain.", level.Name))
	}
	if stored != nil && levelChanged {
		n, err := s.Repo.Children(ctx, stored.ID)
		if err != nil {
			return refs, problems, err
		}
		if n > 0 {
			add("levelCode", fmt.Sprintf("Level tidak dapat diubah selama unit ini punya %d unit di bawahnya.", n))
		}
	}

	// Parent: exactly one level above; none at the top of the tree.
	above, hasAbove, err := s.Repo.LevelAbove(ctx, level.Rank)
	if err != nil {
		return refs, problems, err
	}
	switch {
	case !hasAbove && in.ParentCode != nil:
		add("parentCode", fmt.Sprintf("Unit di level %s (teratas) tidak punya induk.", level.Name))
	case hasAbove && in.ParentCode == nil && !problems.Has("parentCode"):
		add("parentCode", fmt.Sprintf("Induk wajib diisi: pilih unit di level %s.", above.Name))
	case hasAbove && in.ParentCode != nil:
		parent, err := s.Repo.Unit(ctx, *in.ParentCode)
		switch {
		case errors.Is(err, ErrNotFound):
			add("parentCode", fmt.Sprintf("Induk \"%s\" tidak ditemukan.", *in.ParentCode))
		case err != nil:
			return refs, problems, err
		case stored != nil && parent.ID == stored.ID:
			add("parentCode", "Induk tidak boleh unit ini sendiri atau unit di bawahnya.")
		case parent.LevelID != above.ID:
			add("parentCode", fmt.Sprintf("Induk harus unit di level %s, tepat satu level di atas %s.", above.Name, level.Name))
		case !parent.Active && !same(storedParent(stored), in.ParentCode):
			add("parentCode", fmt.Sprintf("Induk %s nonaktif; pilih unit lain.", parent.Name))
		default:
			if stored != nil {
				under, err := s.Repo.IsUnder(ctx, parent.ID, stored.ID)
				if err != nil {
					return refs, problems, err
				}
				if under {
					add("parentCode", "Induk tidak boleh unit ini sendiri atau unit di bawahnya.")
					break
				}
			}
			refs.ParentID = &parent.ID
		}
	}

	// Unit type: always at the operational level, never elsewhere.
	operational := strings.EqualFold(level.Code, OperationalLevel)
	switch {
	case !operational && in.UnitTypeCode != nil:
		add("unitTypeCode", fmt.Sprintf("Jenis unit hanya untuk level Unit Operasional, bukan %s.", level.Name))
	case operational && in.UnitTypeCode == nil && !problems.Has("unitTypeCode"):
		add("unitTypeCode", "Jenis unit wajib diisi untuk level Unit Operasional.")
	case operational && in.UnitTypeCode != nil:
		t, err := s.Repo.UnitType(ctx, *in.UnitTypeCode)
		switch {
		case errors.Is(err, ErrNotFound):
			add("unitTypeCode", fmt.Sprintf("Jenis unit \"%s\" tidak ditemukan.", *in.UnitTypeCode))
		case err != nil:
			return refs, problems, err
		case !t.Active && !same(storedType(stored), in.UnitTypeCode):
			add("unitTypeCode", fmt.Sprintf("Jenis unit %s nonaktif; pilih jenis lain.", t.Name))
		default:
			refs.UnitTypeID = &t.ID
		}
	}
	return refs, problems, nil
}

func storedParent(u *Unit) *string {
	if u == nil {
		return nil
	}
	return u.ParentCode
}

func storedType(u *Unit) *string {
	if u == nil {
		return nil
	}
	return u.UnitTypeCode
}

// nextCode is the first generated code no unit holds, deleted units included: a code is never reused.
func (s *Service) nextCode(ctx context.Context) (string, error) {
	for range maxCodeTries {
		code, err := s.Codes.Next(ctx, Table)
		if err != nil {
			return "", err
		}
		taken, err := s.Repo.CodeTaken(ctx, code)
		if err != nil {
			return "", err
		}
		if taken == Free {
			return code, nil
		}
	}
	return "", apperr.Conflict("CODE_EXHAUSTED", "Kode otomatis berikutnya sudah dipakai. Periksa nomor terakhir di Prefix Kode Master.")
}

func (s *Service) emit(ctx context.Context, what string, u Unit, by access.User) error {
	return s.Events.Write(ctx, outbox.Event{AggregateType: "org_unit", AggregateID: u.ID, AggregateCode: u.Code,
		Type: "pm.org_unit." + what, Payload: u, ActorID: by.ID})
}

func notFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apperr.NotFound()
	}
	return err
}
