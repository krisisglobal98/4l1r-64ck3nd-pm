package unittype

import (
	"context"
	"errors"
	"fmt"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// ErrNotFound is returned by a Repository when no live (not deleted) unit type has the code.
var ErrNotFound = errors.New("unit type not found")

// ErrDuplicate is returned by a Repository when another unit type already has the code (any case).
var ErrDuplicate = errors.New("unit type code taken")

// Taken says who holds a code already.
type Taken int

const (
	Free Taken = iota
	TakenLive
	TakenDeleted
)

// Repository is the storage port of this master. Deleted unit types are invisible except to CodeTaken.
type Repository interface {
	List(ctx context.Context) ([]UnitType, error)
	// Get finds a live unit type by code in any case; it locks the row for the rest of the transaction when forUpdate.
	Get(ctx context.Context, code string, forUpdate bool) (UnitType, error)
	// CodeTaken reports whether any unit type, deleted ones included, has code (case-insensitive).
	CodeTaken(ctx context.Context, code string) (Taken, error)
	Insert(ctx context.Context, in Input, actorID string) (UnitType, error)
	// Update writes in onto the unit type with id; the code is not changed.
	Update(ctx context.Context, id string, in Input, actorID string) (UnitType, error)
	// SoftDelete marks the unit type deleted (and inactive); the row stays for history.
	SoftDelete(ctx context.Context, id, actorID string) error
	// UsedBy says who still uses the unit type (units).
	UsedBy(ctx context.Context, id string) ([]string, error)
	// Jabatan finds a live job title by code (any case) and locks it until the save ends.
	Jabatan(ctx context.Context, code string) (refs.Ref, error)
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

// maxCodeTries bounds the numbers skipped because a code was already taken.
const maxCodeTries = 20

func (s *Service) List(ctx context.Context) ([]UnitType, error) {
	if _, err := access.Require(ctx, Menu, "read"); err != nil {
		return nil, err
	}
	return s.Repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, code string) (UnitType, error) {
	if _, err := access.Require(ctx, Menu, "read"); err != nil {
		return UnitType{}, err
	}
	if !record.LookupKey(code) {
		return UnitType{}, apperr.NotFound()
	}
	u, err := s.Repo.Get(ctx, code, false)
	return u, notFound(err)
}

func (s *Service) Create(ctx context.Context, body record.Values) (UnitType, error) {
	u, err := access.Require(ctx, Menu, "create")
	if err != nil {
		return UnitType{}, err
	}
	var saved UnitType
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		in, problems := Parse(body)
		if problems, err = s.resolve(ctx, &in, nil, problems); err != nil {
			return err
		}
		if len(problems) > 0 {
			return apperr.Validation(problems)
		}
		if in.Code, err = s.nextCode(ctx); err != nil {
			return err
		}
		if saved, err = s.Repo.Insert(ctx, in, u.ID); err != nil {
			if errors.Is(err, ErrDuplicate) { // only if the table was changed outside the service
				return apperr.Conflict("CODE_TAKEN", fmt.Sprintf("Kode %s sudah dipakai. Simpan ulang untuk mendapat kode berikutnya.", in.Code))
			}
			return err
		}
		return s.emit(ctx, "created", saved, u)
	})
	return saved, err
}

// nextCode is the first generated code no unit type holds, deleted ones included: a code is never reused.
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

// Update merges body onto the stored unit type. The code is fixed after creation and the workflow fields belong to
// the approval flow: neither is taken from the body.
func (s *Service) Update(ctx context.Context, code string, body record.Values) (UnitType, error) {
	u, err := access.Require(ctx, Menu, "update")
	if err != nil {
		return UnitType{}, err
	}
	if !record.LookupKey(code) {
		return UnitType{}, apperr.NotFound()
	}
	var saved UnitType
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		stored, err := s.Repo.Get(ctx, code, true)
		if err != nil {
			return notFound(err)
		}
		var problems record.Problems
		if want, ok := record.ExpectedVersion(body, &problems); ok && want != stored.Version {
			return apperr.Conflict("STALE", fmt.Sprintf("Jenis unit %s sudah diubah pengguna lain sejak Anda membukanya. Muat ulang, lalu ulangi perubahan Anda.", stored.Name))
		}
		v := valuesOf(stored)
		for k, x := range body {
			v[k] = x
		}
		v["code"] = stored.Code
		in, more := Parse(v)
		if problems, err = s.resolve(ctx, &in, &stored, append(problems, more...)); err != nil {
			return err
		}
		if len(problems) > 0 {
			return apperr.Validation(problems)
		}
		if saved, err = s.Repo.Update(ctx, stored.ID, in, u.ID); err != nil {
			return notFound(err)
		}
		return s.emit(ctx, "updated", saved, u)
	})
	return saved, err
}

// Delete is a soft delete: the unit type leaves every list but stays in the table, and its code is never reused.
// A type still used by a unit is kept (deactivate it instead).
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
			return apperr.InUse(refs.InUseMessage("Jenis unit", stored.Name, used))
		}
		if err := s.Repo.SoftDelete(ctx, stored.ID, u.ID); err != nil {
			return notFound(err)
		}
		return s.emit(ctx, "deleted", stored, u)
	})
}

// resolve finds the head job title (optional); a new choice must be active, a kept one may have been deactivated.
func (s *Service) resolve(ctx context.Context, in *Input, stored *UnitType, problems record.Problems) (record.Problems, error) {
	if in.HeadJabatanCode == nil || problems.Has("headJabatanCode") {
		return problems, nil
	}
	j, err := s.Repo.Jabatan(ctx, *in.HeadJabatanCode)
	switch {
	case errors.Is(err, refs.ErrNotFound):
		problems.Add("headJabatanCode", fmt.Sprintf("Jabatan \"%s\" tidak ditemukan.", *in.HeadJabatanCode))
	case err != nil:
		return problems, err
	case !j.Active && (stored == nil || stored.HeadJabatanID == nil || *stored.HeadJabatanID != j.ID):
		problems.Add("headJabatanCode", fmt.Sprintf("Jabatan %s nonaktif; pilih jabatan lain.", j.Name))
	default:
		in.HeadJabatanID = &j.ID
	}
	return problems, nil
}

func (s *Service) emit(ctx context.Context, what string, t UnitType, u access.User) error {
	return s.Events.Write(ctx, outbox.Event{AggregateType: "unit_type", AggregateID: t.ID, AggregateCode: t.Code,
		Type: "pm.unit_type." + what, Payload: t, ActorID: u.ID})
}

func notFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apperr.NotFound()
	}
	return err
}
