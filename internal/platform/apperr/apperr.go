// Package apperr is the error vocabulary of the use cases; the HTTP layer turns it into a status and the
// `{code, message, details}` body the frontend reads. Messages are Indonesian, as the frontend shows them.
package apperr

import "net/http"

type Error struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Problem is one field of a form that needs correcting.
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func Validation(problems []Problem) *Error {
	return &Error{http.StatusUnprocessableEntity, "VALIDATION", "Periksa isian yang ditandai.", map[string]any{"problems": problems}}
}
func NotFound() *Error {
	return &Error{http.StatusNotFound, "NOT_FOUND", "Data tidak ditemukan.", nil}
}
func Forbidden() *Error {
	return &Error{http.StatusForbidden, "FORBIDDEN", "Peran Anda tidak punya hak untuk tindakan ini.", nil}
}
func InUse(message string) *Error {
	return &Error{http.StatusConflict, "IN_USE", message, nil}
}
func BadRequest(message string) *Error {
	return &Error{http.StatusBadRequest, "BAD_REQUEST", message, nil}
}
func NotImplemented(code, message string) *Error {
	return &Error{http.StatusNotImplemented, code, message, nil}
}
func Conflict(code, message string) *Error {
	return &Error{http.StatusConflict, code, message, nil}
}
