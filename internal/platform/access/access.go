// Package access answers who is calling and whether they may act: may(user, menu, action), the rule of
// ALIR docs/auth-api.md. The caller comes from the access token of the Authentication service (jwt.go); in
// development, a default user with full access may stand in (DefaultUser).
package access

import (
	"context"
	"net/http"

	"alir.local/pm-service/internal/platform/apperr"
)

type User struct {
	// ID is the user's id in the Authentication service (UUID); audit columns store it.
	ID string
	// TenantID is the company the request acts in (claim tid); Row-Level Security keeps it to that tenant.
	TenantID string
	Username string
	// Grants by menu (e.g. "pm.orgLevel" → read, create, …); ignored when Full.
	Grants map[string][]string
	Full   bool
	// Platform: the installation's administrator, who alone changes the global reference masters (countries,
	// currencies, Incoterms) that every company reads.
	Platform bool
}

type ctxKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

// Can reports whether u may do action on menu.
func Can(u User, menu, action string) bool {
	if u.Full {
		return true
	}
	for _, a := range u.Grants[menu] {
		if a == action {
			return true
		}
	}
	return false
}

// Require returns the signed-in user: 401 when there is none, 403 when they lack the action.
func Require(ctx context.Context, menu, action string) (User, error) {
	u, ok := UserFrom(ctx)
	if !ok {
		return User{}, &apperr.Error{Status: http.StatusUnauthorized, Code: "SIGNED_OUT", Message: "Sesi tidak ditemukan. Silakan masuk kembali."}
	}
	if !Can(u, menu, action) {
		return User{}, apperr.Forbidden()
	}
	return u, nil
}

// DefaultUser makes every request act as user id/username with full access (PM_AUTH_MODE=default-user:
// development and the e2e suite only, never production).
// The tenant is tenantID, or the X-Tenant-ID header of the request (the e2e suite checks the isolation with it).
func DefaultUser(id, username, tenantID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := User{ID: id, Username: username, Full: true, TenantID: tenantID, Platform: true}
			if h := r.Header.Get("X-Tenant-ID"); uuidLike(h) {
				u.TenantID = h
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
		})
	}
}

// TenantOf is the tenant of the request's caller ("" when there is none).
func TenantOf(r *http.Request) string {
	u, _ := UserFrom(r.Context())
	return u.TenantID
}
