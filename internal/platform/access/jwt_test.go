package access

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func sign(t *testing.T, key ed25519.PrivateKey, kid string, c map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": "EdDSA", "kid": kid, "typ": "JWT"})
	b, _ := json.Marshal(c)
	in := b64.EncodeToString(h) + "." + b64.EncodeToString(b)
	return in + "." + b64.EncodeToString(ed25519.Sign(key, []byte(in)))
}

func TestJWT(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// As Authentication publishes it: the RSA key of ID tokens beside the Ed25519 key of access tokens.
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": "krsa", "use": "sig", "alg": "RS256", "n": "1ruRBMX5lJtzBl0mu38o73HA4fIRNAhM45d", "e": "AQAB"},
			{"kty": "OKP", "crv": "Ed25519", "kid": "k1", "x": b64.EncodeToString(pub)}}})
	}))
	defer jwks.Close()
	j := &JWT{JWKSURL: jwks.URL, Issuer: "alir-auth", Client: jwks.Client()}
	var got *User
	h := j.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := UserFrom(r.Context()); ok {
			got = &u
		}
		w.WriteHeader(204)
	}))
	call := func(tok string) (int, string) {
		got = nil
		req := httptest.NewRequest("GET", "/x", nil)
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var body struct{ Code string }
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body.Code
	}
	good := map[string]any{"iss": "alir-auth", "aud": "alir", "sub": "01a1ef2d-339b-7352-8690-a1f17660799f", "exp": time.Now().Add(time.Minute).Unix(),
		"tid": "00000000-0000-7000-8000-0000000000a1", "username": "budi", "grants": map[string][]string{"pm.orgUnit": {"read"}}}
	if code, _ := call(sign(t, priv, "k1", good)); code != 204 || got == nil || got.Username != "budi" || got.TenantID != "00000000-0000-7000-8000-0000000000a1" || !Can(*got, "pm.orgUnit", "read") || Can(*got, "pm.orgUnit", "delete") {
		t.Fatalf("valid token: %d %+v", code, got)
	}
	with := func(k string, v any) map[string]any {
		c := map[string]any{}
		for a, b := range good {
			c[a] = b
		}
		c[k] = v
		return c
	}
	for name, c := range map[string]struct {
		tok  string
		code string
	}{
		"expired":      {sign(t, priv, "k1", with("exp", time.Now().Add(-time.Second).Unix())), "TOKEN_EXPIRED"},
		"other key":    {sign(t, other, "k1", good), "TOKEN_INVALID"},
		"unknown kid":  {sign(t, priv, "k9", good), "TOKEN_INVALID"},
		"wrong issuer": {sign(t, priv, "k1", with("iss", "evil")), "TOKEN_INVALID"},
		"wrong aud":    {sign(t, priv, "k1", with("aud", "other")), "TOKEN_INVALID"},
		"bad subject":  {sign(t, priv, "k1", with("sub", "admin")), "TOKEN_INVALID"},
		"garbage":      {"a.b.c", "TOKEN_INVALID"},
		"RS256 kid":    {signAlg(t, priv, "krsa", "RS256", good), "TOKEN_INVALID"},
		"no tenant":    {sign(t, priv, "k1", with("tid", "")), "TOKEN_INVALID"},
		"bad tenant":   {sign(t, priv, "k1", with("tid", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz")), "TOKEN_INVALID"},
	} {
		if code, body := call(c.tok); code != 401 || body != c.code || got != nil {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
	if code, _ := call(""); code != 204 || got != nil {
		t.Fatalf("no token goes on anonymous: %d", code)
	}
	if _, err := Require(context.Background(), "pm.orgUnit", "read"); err == nil {
		t.Fatal("anonymous must be refused")
	}
}

// signAlg signs with the Ed25519 key but names another algorithm and key (a token that is not an access token).
func signAlg(t *testing.T, key ed25519.PrivateKey, kid, alg string, c map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": alg, "kid": kid, "typ": "JWT"})
	b, _ := json.Marshal(c)
	in := b64.EncodeToString(h) + "." + b64.EncodeToString(b)
	return in + "." + b64.EncodeToString(ed25519.Sign(key, []byte(in)))
}
