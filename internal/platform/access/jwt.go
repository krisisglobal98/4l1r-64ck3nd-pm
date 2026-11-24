package access

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"alir.local/pm-service/internal/platform/apperr"
	"alir.local/pm-service/internal/platform/httpx"
)

// JWT reads the access token of the Authentication service: EdDSA (Ed25519) signature by a key of its JWKS,
// issuer, audience "alir" and expiry. The user's grants travel in the token (at most its lifetime old, 15 min).
// A request without a token goes on anonymous; Require refuses it with 401.
type JWT struct {
	JWKSURL string
	Issuer  string
	Client  *http.Client

	mu      sync.Mutex
	keys    map[string]ed25519.PublicKey
	fetched time.Time
}

type claims struct {
	Iss      string              `json:"iss"`
	Aud      string              `json:"aud"`
	Sub      string              `json:"sub"`
	Tid      string              `json:"tid"` // the tenant (company) the token acts in
	Exp      int64               `json:"exp"`
	Username string              `json:"username"`
	Full     bool                `json:"full"`
	Grants   map[string][]string `json:"grants"`
	Platform bool                `json:"pa"` // administers the installation (the global reference masters)
}

var b64 = base64.RawURLEncoding

// key is the public key kid, reading the JWKS again when it is unknown (a new key) at most every 30 s.
func (j *JWT) key(kid string) (ed25519.PublicKey, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if k, ok := j.keys[kid]; ok && time.Since(j.fetched) < time.Hour {
		return k, nil
	}
	if time.Since(j.fetched) < 30*time.Second && j.keys != nil {
		if k, ok := j.keys[kid]; ok {
			return k, nil
		}
		return nil, errors.New("unknown key")
	}
	res, err := j.Client.Get(j.JWKSURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var set struct {
		Keys []struct{ Kty, Crv, Kid, X string } `json:"keys"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&set) != nil {
		return nil, errors.New("jwks unreadable")
	}
	keys := map[string]ed25519.PublicKey{}
	for _, k := range set.Keys {
		if x, err := b64.DecodeString(k.X); err == nil && k.Kty == "OKP" && k.Crv == "Ed25519" && len(x) == ed25519.PublicKeySize {
			keys[k.Kid] = ed25519.PublicKey(x)
		}
	}
	j.keys, j.fetched = keys, time.Now()
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("unknown key")
}

func (j *JWT) verify(tok string) (claims, error) {
	var c claims
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || len(tok) > 16<<10 {
		return c, errInvalid
	}
	var head struct{ Alg, Kid string }
	hb, err := b64.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &head) != nil || head.Alg != "EdDSA" {
		return c, errInvalid
	}
	key, err := j.key(head.Kid)
	if err != nil {
		slog.Warn("token key", "kid", head.Kid, "err", err)
		return c, errInvalid
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil || !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), sig) {
		return c, errInvalid
	}
	body, err := b64.DecodeString(parts[1])
	if err != nil || json.Unmarshal(body, &c) != nil || c.Iss != j.Issuer || c.Aud != "alir" || !uuidLike(c.Sub) || !uuidLike(c.Tid) {
		return c, errInvalid
	}
	if time.Now().Unix() >= c.Exp {
		return c, errExpired
	}
	return c, nil
}

var (
	errInvalid = errors.New("invalid token")
	errExpired = errors.New("expired token")
)

// uuidLike is a canonical UUID (8-4-4-4-12 hex digits): the tenant goes into a uuid cast in every policy.
func uuidLike(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// Middleware puts the caller of a valid token in the request.
func (j *JWT) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}
		c, err := j.verify(strings.TrimPrefix(h, "Bearer "))
		switch {
		case errors.Is(err, errExpired):
			httpx.WriteError(w, r, &apperr.Error{Status: http.StatusUnauthorized, Code: "TOKEN_EXPIRED", Message: "Sesi perlu diperbarui."})
			return
		case err != nil:
			httpx.WriteError(w, r, &apperr.Error{Status: http.StatusUnauthorized, Code: "TOKEN_INVALID", Message: "Sesi tidak sah. Silakan masuk kembali."})
			return
		}
		u := User{ID: c.Sub, TenantID: c.Tid, Username: c.Username, Grants: c.Grants, Full: c.Full, Platform: c.Platform}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}
