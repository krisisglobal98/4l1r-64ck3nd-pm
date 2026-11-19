package orglevel

import (
	"net/http"

	"alir.local/pm-service/internal/platform/access"
	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/httpx"
	"alir.local/pm-service/internal/platform/record"
)

// Routes registers the master under /pm/masters/levels, the path ALIR's masterService calls.
func Routes(mux *http.ServeMux, s *Service) {
	const base = "/pm/masters/levels"
	mux.Handle("GET "+base, httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		list, err := s.List(r.Context())
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, list)
	}))
	mux.Handle("GET "+base+"/{code}", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		l, err := s.Get(r.Context(), r.PathValue("code"))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, l)
	}))
	mux.Handle("POST "+base, httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		body, err := httpx.DecodeObject(r)
		if err != nil {
			return err
		}
		l, err := s.Create(r.Context(), record.Values(body))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, l)
	}))
	mux.Handle("POST "+base+"/upload", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if _, err := access.Require(r.Context(), Menu, "upload"); err != nil {
			return err
		}
		return apperr.NotImplemented("NO_IMPORT", "Impor file Level Organisasi belum tersedia.")
	}))
	mux.Handle("PUT "+base+"/{code}", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		body, err := httpx.DecodeObject(r)
		if err != nil {
			return err
		}
		l, err := s.Update(r.Context(), r.PathValue("code"), record.Values(body))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, l)
	}))
	mux.Handle("DELETE "+base+"/{code}", httpx.Handler(func(w http.ResponseWriter, r *http.Request) error {
		if err := s.Delete(r.Context(), r.PathValue("code")); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
}
