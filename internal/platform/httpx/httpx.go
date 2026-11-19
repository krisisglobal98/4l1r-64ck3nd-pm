// Package httpx is the HTTP side shared by every feature: JSON in and out, the error body, and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/httpx/reqid"
)

const maxBody = 1 << 20

// Handler is an http handler that returns its error instead of writing it.
type Handler func(w http.ResponseWriter, r *http.Request) error

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteError(w, r, err)
	}
}

func JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// DecodeObject reads a JSON object body of at most 1 MiB.
func DecodeObject(r *http.Request) (map[string]any, error) {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return nil, apperr.BadRequest("Isi permintaan harus JSON.")
	}
	var body map[string]any
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil || body == nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, &apperr.Error{Status: http.StatusRequestEntityTooLarge, Code: "TOO_LARGE", Message: "Isi permintaan terlalu besar (maksimal 1 MiB)."}
		}
		return nil, apperr.BadRequest("Isi permintaan bukan objek JSON yang sah.")
	}
	if _, err := dec.Token(); err != io.EOF { // one object, nothing after it
		return nil, apperr.BadRequest("Isi permintaan bukan objek JSON yang sah.")
	}
	return body, nil
}

// WriteError answers `{code, message, details}`; anything that is not an apperr is logged and hidden as 500.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var e *apperr.Error
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &e):
	case errors.As(err, &pg) && pg.Code == "23505": // a unique index refused a save that raced another one
		slog.WarnContext(r.Context(), "unique conflict", "constraint", pg.ConstraintName, "request_id", reqid.From(r.Context()))
		e = apperr.Conflict("CONFLICT", "Data bentrok dengan data lain yang baru saja disimpan. Muat ulang, lalu coba lagi.")
	case errors.As(err, &pg) && strings.HasPrefix(pg.Code, "22"): // data exception: a value the database refuses
		slog.WarnContext(r.Context(), "data refused by the database", "err", err, "request_id", reqid.From(r.Context()))
		e = apperr.BadRequest("Data yang dikirim tidak dapat disimpan.")
	default:
		slog.ErrorContext(r.Context(), "unhandled error", "err", err, "request_id", reqid.From(r.Context()))
		e = &apperr.Error{Status: http.StatusInternalServerError, Code: "INTERNAL", Message: "Terjadi kesalahan pada server."}
	}
	body := map[string]any{"code": e.Code, "message": e.Message}
	if e.Details != nil {
		body["details"] = e.Details
	}
	_ = JSON(w, e.Status, body)
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(s int) { r.status = s; r.ResponseWriter.WriteHeader(s) }

// Logging writes one line per request: method, path, status, duration, request id.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.InfoContext(r.Context(), "request", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"ms", time.Since(start).Milliseconds(), "request_id", reqid.From(r.Context()))
	})
}

// Recover turns a panic into a 500 answer instead of a dropped connection.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(r.Context(), "panic", "panic", p, "stack", string(debug.Stack()), "request_id", reqid.From(r.Context()))
				WriteError(w, r, errors.New("panic"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// JSONErrors gives the router's own answers (no route: 404, wrong method: 405) the `{code, message}` body of
// every other error, instead of plain text.
func JSONErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&routerErrors{ResponseWriter: w}, r)
	})
}

type routerErrors struct {
	http.ResponseWriter
	swallow bool
}

func (w *routerErrors) WriteHeader(status int) {
	plain := strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain")
	if plain && (status == http.StatusNotFound || status == http.StatusMethodNotAllowed) {
		w.swallow = true
		body := map[string]string{"code": "NOT_FOUND", "message": "Alamat tidak ditemukan."}
		if status == http.StatusMethodNotAllowed {
			body = map[string]string{"code": "METHOD_NOT_ALLOWED", "message": "Metode tidak didukung di alamat ini."}
		}
		_ = JSON(w.ResponseWriter, status, body)
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *routerErrors) Write(b []byte) (int, error) {
	if w.swallow {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

// Chain applies middleware so that the first one listed runs first.
func Chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}
