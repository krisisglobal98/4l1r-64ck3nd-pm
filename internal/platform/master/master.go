// Package master is the use-case shell every master shares: access, generated codes, the version check, merging a
// change onto the stored record, soft delete with the usage check, and events, all in one transaction. A master
// gives its Spec (fields, relation checks) and its Repo (SQL); see internal/grade for the smallest one.
//
// Level Organisasi, Jenis Unit and Unit Organisasi were written before this shell and keep their own services
// with the same behaviour.
package master

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/codeprefix"
	"alir.local/pm-service/internal/platform/database"
	"alir.local/pm-service/internal/platform/httpx"
	"alir.local/pm-service/internal/platform/outbox"
	"alir.local/pm-service/internal/platform/record"
	"alir.local/pm-service/internal/platform/refs"
)

// ErrNotFound is returned by a Repo when no live row has the code or id.
var ErrNotFound = errors.New("not found")

// Repo stores the records of one master table. Get finds a live record by code in any case and, with forUpdate,
// locks it until the transaction ends.
type Repo[T, I any] interface {
	List(ctx context.Context) ([]T, error)
	Get(ctx context.Context, code string, forUpdate bool) (T, error)
	Insert(ctx context.Context, in I, actorID string) (T, error)
	Update(ctx context.Context, id string, in I, actorID string) (T, error)
	SoftDelete(ctx context.Context, id, actorID string) error
}

// Spec is what makes a master itself.
type Spec[T, I any] struct {
	Menu  string // permission catalog menu, e.g. pm.grade
	Table string // table in schema pm, also its row in pm.code_prefix and its usage entry in refs
	Noun  string // "Grade", in messages
	Base  func(*T) *record.Base
	// Values is a stored record as form values (relations by code), the base a change is merged onto.
	Values func(T) record.Values
	// Parse reads the form values (never the code).
	Parse   func(record.Values) (I, record.Problems)
	SetCode func(*I, string)
	// Check resolves the relations of in and adds problems; stored is nil on create. Optional.
	Check func(ctx context.Context, in *I, stored *T, p record.Problems) (record.Problems, error)
	// BeforeDelete refuses a delete for a reason of the master's own (usage is checked by Table). Optional.
	BeforeDelete func(ctx context.Context, stored T) error
	// Serialize puts every write of the master in one line (create, update, delete), taken first in the
	// transaction. For masters whose rows lock each other (a reporting line: A under B while B goes under A),
	// one fixed lock order instead of row locks taken in opposite orders, which deadlock. Optional.
	Serialize bool
	// Import fills the master from an Excel workbook (template + upload). Optional: without it the upload is refused.
	Import *Import
}

type Service[T, I any] struct {
	Spec   Spec[T, I]
	Repo   Repo[T, I]
	DB     *database.DB
	Codes  codeprefix.Generator
	Events outbox.Writer
}

func (s *Service[T, I]) List(ctx context.Context) ([]T, error) {
	if _, err := access.Require(ctx, s.Spec.Menu, "read"); err != nil {
		return nil, err
	}
	return s.Repo.List(ctx)
}

func (s *Service[T, I]) Get(ctx context.Context, code string) (T, error) {
	var zero T
	if _, err := access.Require(ctx, s.Spec.Menu, "read"); err != nil {
		return zero, err
	}
	if !record.LookupKey(code) {
		return zero, apperr.NotFound()
	}
	t, err := s.Repo.Get(ctx, code, false)
	return t, notFound(err)
}

func (s *Service[T, I]) Create(ctx context.Context, body record.Values) (T, error) {
	var saved T
	u, err := access.Require(ctx, s.Spec.Menu, "create")
	if err != nil {
		return saved, err
	}
	err = s.DB.InTx(ctx, func(ctx context.Context) error {
		if err := s.serialize(ctx); err != nil {
			return err
		}
		in, problems := s.Spec.Parse(body)
		if problems, err = s.check(ctx, &in, nil, problems); err != nil {
			return err
		}
		if len(problems) > 0 {
			return apperr.Validation(problems)
		}
		code, err := codeprefix.NextFree(ctx, s.Codes, s.Spec.Table, func(ctx context.Context, c string) (bool, error) {
			return refs.CodeTaken(ctx, s.DB, s.Spec.Table, c)
		})
		if err != nil {
			return err
		}
		s.Spec.SetCode(&in, code)
		if saved, err = s.Repo.Insert(ctx, in, u.ID); err != nil {
			return err
		}
		return s.emit(ctx, "created", saved, u)
	})
	return saved, err
}

// Update merges body onto the stored record; the code, ids, audit and workflow fields are never taken from it.
func (s *Service[T, I]) Update(ctx context.Context, code string, body record.Values) (T, error) {
	var saved T
	u, err := access.Require(ctx, s.Spec.Menu, "update")
	if err != nil {
		return saved, err
	}
	if !record.LookupKey(code) {
		return saved, apperr.NotFound()
	}
	err = s.DB.InTx(ctx, func(ctx context.Context) error {
		if err := s.serialize(ctx); err != nil {
			return err
		}
		stored, err := s.Repo.Get(ctx, code, true)
		if err != nil {
			return notFound(err)
		}
		b := s.Spec.Base(&stored)
		var problems record.Problems
		if want, ok := record.ExpectedVersion(body, &problems); ok && want != b.Version {
			return apperr.Conflict("STALE", fmt.Sprintf("%s %s sudah diubah pengguna lain sejak Anda membukanya. Muat ulang, lalu ulangi perubahan Anda.", s.Spec.Noun, b.Name))
		}
		v := s.Spec.Values(stored)
		for k, x := range body {
			v[k] = x
		}
		v["code"] = b.Code
		in, more := s.Spec.Parse(v)
		if problems, err = s.check(ctx, &in, &stored, append(problems, more...)); err != nil {
			return err
		}
		if len(problems) > 0 {
			return apperr.Validation(problems)
		}
		s.Spec.SetCode(&in, b.Code)
		if saved, err = s.Repo.Update(ctx, b.ID, in, u.ID); err != nil {
			return notFound(err)
		}
		return s.emit(ctx, "updated", saved, u)
	})
	return saved, err
}

// Delete is a soft delete; a record still used elsewhere is kept (deactivate it instead).
func (s *Service[T, I]) Delete(ctx context.Context, code string) error {
	u, err := access.Require(ctx, s.Spec.Menu, "delete")
	if err != nil {
		return err
	}
	if !record.LookupKey(code) {
		return apperr.NotFound()
	}
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		if err := s.serialize(ctx); err != nil {
			return err
		}
		stored, err := s.Repo.Get(ctx, code, true)
		if err != nil {
			return notFound(err)
		}
		b := s.Spec.Base(&stored)
		used, err := refs.UsedBy(ctx, s.DB, s.Spec.Table, b.ID)
		if err != nil {
			return err
		}
		if len(used) > 0 {
			return apperr.InUse(refs.InUseMessage(s.Spec.Noun, b.Name, used))
		}
		if s.Spec.BeforeDelete != nil {
			if err := s.Spec.BeforeDelete(ctx, stored); err != nil {
				return err
			}
		}
		if err := s.Repo.SoftDelete(ctx, b.ID, u.ID); err != nil {
			return notFound(err)
		}
		return s.emit(ctx, "deleted", stored, u)
	})
}

// serialize takes the master's transaction-scoped advisory lock (released at commit or rollback).
func (s *Service[T, I]) serialize(ctx context.Context) error {
	if !s.Spec.Serialize {
		return nil
	}
	_, err := s.DB.Q(ctx).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "pm."+s.Spec.Table)
	return err
}

func (s *Service[T, I]) check(ctx context.Context, in *I, stored *T, p record.Problems) (record.Problems, error) {
	if s.Spec.Check == nil {
		return p, nil
	}
	return s.Spec.Check(ctx, in, stored, p)
}

func (s *Service[T, I]) emit(ctx context.Context, what string, t T, u access.User) error {
	b := s.Spec.Base(&t)
	return s.Events.Write(ctx, outbox.Event{AggregateType: s.Spec.Table, AggregateID: b.ID, AggregateCode: b.Code,
		Type: "pm." + s.Spec.Table + "." + what, Payload: t, ActorID: u.ID})
}

func notFound(err error) error {
	if errors.Is(err, ErrNotFound) || errors.Is(err, refs.ErrNotFound) {
		return apperr.NotFound()
	}
	return err
}

// Routes registers the master under base (e.g. /pm/masters/grades), the path ALIR's masterService calls.
func Routes[T, I any](mux *http.ServeMux, base string, s *Service[T, I]) {
	mux.Handle("GET "+base, httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		list, err := s.List(r.Context())
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, list)
	}))
	mux.Handle("GET "+base+"/{code}", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		t, err := s.Get(r.Context(), r.PathValue("code"))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, t)
	}))
	mux.Handle("POST "+base, httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		body, err := httpx.DecodeObject(r)
		if err != nil {
			return err
		}
		t, err := s.Create(r.Context(), record.Values(body))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, t)
	}))
	if s.Spec.Import != nil {
		importRoutes(mux, base, s)
	} else {
		mux.Handle("POST "+base+"/upload", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
			if _, err := access.Require(r.Context(), s.Spec.Menu, "upload"); err != nil {
				return err
			}
			return apperr.NotImplemented("NO_IMPORT", fmt.Sprintf("Impor file %s belum tersedia.", s.Spec.Noun))
		}))
	}
	mux.Handle("PUT "+base+"/{code}", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		body, err := httpx.DecodeObject(r)
		if err != nil {
			return err
		}
		t, err := s.Update(r.Context(), r.PathValue("code"), record.Values(body))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, t)
	}))
	mux.Handle("DELETE "+base+"/{code}", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if err := s.Delete(r.Context(), r.PathValue("code")); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
}
