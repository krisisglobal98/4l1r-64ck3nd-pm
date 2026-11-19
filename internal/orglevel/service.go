package orglevel

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

// ErrNotFound is returned by a Repository when no live (not deleted) level has the code.
var ErrNotFound = errors.New("org level not found")

// ErrDuplicate is returned by a Repository when another level already has the code (any case).
var ErrDuplicate = errors.New("org level code taken")

// ErrRankTaken is returned by a Repository when another live level already has the rank.
var ErrRankTaken = errors.New("org level rank taken")

// Taken says who holds a code already.
type Taken int

const (
	Free Taken = iota
	TakenLive
	TakenDeleted
)

// Repository is the storage port of this master. Deleted levels are invisible except to CodeTaken.
type Repository interface {
	List(ctx context.Context) ([]Level, error)
	// Get finds a live level by code in any case; it locks the row for the rest of the transaction when forUpdate.
	Get(ctx context.Context, code string, forUpdate bool) (Level, error)
	// RankHolder is the name of the live level with rank other than exceptID ("" = free).
	RankHolder(ctx context.Context, rank int, exceptID string) (string, error)
	// CodeTaken reports whether any level, deleted ones included, has code (case-insensitive).
	CodeTaken(ctx context.Context, code string) (Taken, error)
	Insert(ctx context.Context, in Input, actorID string) (Level, error)
	// Update writes in onto the level with id; the code is not changed.
	Update(ctx context.Context, id string, in Input, actorID string) (Level, error)
	// SoftDelete marks the level deleted (and inactive); the row stays for history.
	SoftDelete(ctx context.Context, id, actorID string) error
	// UnitsUsing counts the live organization units at this level.
	UnitsUsing(ctx context.Context, id string) (int, error)
	// UsedBy says who still uses the level (units, jabatan, approval rules), e.g. ["6 unit"].
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

// maxCodeTries bounds the numbers skipped because a code was already taken (e.g. entered before codes were
// generated, or a prefix whose counter was set back).
const maxCodeTries = 20

func (s *Service) List(ctx context.Context) ([]Level, error) {
	if _, err := access.Require(ctx, Menu, "read"); err != nil {
		return nil, err
	}
	return s.Repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, code string) (Level, error) {
	if _, err := access.Require(ctx, Menu, "read"); err != nil {
		return Level{}, err
	}
	if !record.LookupKey(code) {
		return Level{}, apperr.NotFound()
	}
	l, err := s.Repo.Get(ctx, code, false)
	return l, notFound(err)
}

func (s *Service) Create(ctx context.Context, body record.Values) (Level, error) {
	u, err := access.Require(ctx, Menu, "create")
	if err != nil {
		return Level{}, err
	}
	var saved Level
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		in, problems := Parse(body)
		if problems, err = s.checkRank(ctx, in, "", problems); err != nil {
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
			return rankRace(err, in.Rank)
		}
		return s.emit(ctx, "created", saved, u)
	})
	return saved, err
}

// nextCode is the first generated code no level holds, deleted levels included: a code is never reused.
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

// Update merges body onto the stored level. The code is fixed after creation, and the workflow fields belong to
// the approval flow: neither is taken from the body.
func (s *Service) Update(ctx context.Context, code string, body record.Values) (Level, error) {
	u, err := access.Require(ctx, Menu, "update")
	if err != nil {
		return Level{}, err
	}
	var saved Level
	if !record.LookupKey(code) {
		return Level{}, apperr.NotFound()
	}
	err = s.Tx.InTx(ctx, func(ctx context.Context) error {
		stored, err := s.Repo.Get(ctx, code, true)
		if err != nil {
			return notFound(err)
		}
		var problems record.Problems
		if want, ok := record.ExpectedVersion(body, &problems); ok && want != stored.Version {
			return apperr.Conflict("STALE", fmt.Sprintf("Level %s sudah diubah pengguna lain sejak Anda membukanya. Muat ulang, lalu ulangi perubahan Anda.", stored.Name))
		}
		v := valuesOf(stored)
		for k, x := range body {
			v[k] = x
		}
		v["code"] = stored.Code
		in, more := Parse(v)
		problems = append(problems, more...)
		if problems, err = s.checkRank(ctx, in, stored.ID, problems); err != nil {
			return err
		}
		// Units hang one level below their parent by rank: a level in use keeps its place in the tree.
		if in.Rank != stored.Rank && !problems.Has("rank") {
			n, err := s.Repo.UnitsUsing(ctx, stored.ID)
			if err != nil {
				return err
			}
			if n > 0 {
				problems.Add("rank", fmt.Sprintf("Urutan tidak dapat diubah selama level ini dipakai %d unit.", n))
			}
		}
		if len(problems) > 0 {
			return apperr.Validation(problems)
		}
		if saved, err = s.Repo.Update(ctx, stored.ID, in, u.ID); err != nil {
			return rankRace(notFound(err), in.Rank)
		}
		return s.emit(ctx, "updated", saved, u)
	})
	return saved, err
}

// Delete is a soft delete: the level leaves every list but stays in the table, and its code is never reused.
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
		// Units and job titles still live outside this service; the built-in levels they use stay.
		if IsBuiltIn(stored.Code) {
			return apperr.InUse(fmt.Sprintf("Level %s adalah level bawaan yang dipakai unit dan jabatan; tidak dapat dihapus. Nonaktifkan saja.", stored.Name))
		}
		used, err := s.Repo.UsedBy(ctx, stored.ID)
		if err != nil {
			return err
		}
		if len(used) > 0 {
			return apperr.InUse(refs.InUseMessage("Level", stored.Name, used))
		}
		if err := s.Repo.SoftDelete(ctx, stored.ID, u.ID); err != nil {
			return notFound(err)
		}
		return s.emit(ctx, "deleted", stored, u)
	})
}

func (s *Service) emit(ctx context.Context, what string, l Level, u access.User) error {
	return s.Events.Write(ctx, outbox.Event{AggregateType: "org_level", AggregateID: l.ID, AggregateCode: l.Code,
		Type: "pm.org_level." + what, Payload: l, ActorID: u.ID})
}

// checkRank adds a problem when another live level holds the rank of in (skipped when rank has a problem already).
func (s *Service) checkRank(ctx context.Context, in Input, selfID string, problems record.Problems) (record.Problems, error) {
	if problems.Has("rank") {
		return problems, nil
	}
	holder, err := s.Repo.RankHolder(ctx, in.Rank, selfID)
	if err != nil || holder == "" {
		return problems, err
	}
	return append(problems, apperr.Problem{Field: "rank", Message: fmt.Sprintf("Urutan %d sudah dipakai level %s.", in.Rank, holder)}), nil
}

// rankRace is the answer when two saves took one rank at the same moment (the unique index refused the second).
func rankRace(err error, rank int) error {
	if errors.Is(err, ErrRankTaken) {
		return apperr.Validation([]apperr.Problem{{Field: "rank", Message: fmt.Sprintf("Urutan %d baru saja dipakai level lain.", rank)}})
	}
	return err
}

func notFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apperr.NotFound()
	}
	return err
}
