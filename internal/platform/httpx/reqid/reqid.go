// Package reqid carries the request id (X-Request-ID) through the context, into logs, events and the response.
package reqid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
)

const Header = "X-Request-ID"

type ctxKey struct{}

var valid = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func From(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Middleware keeps a caller's id (the gateway's, the frontend's) or makes one, and echoes it in the response.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(Header)
		if !valid.MatchString(id) {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
		}
		w.Header().Set(Header, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}
