// Package cookie seals (DID, session_id) pairs into an authenticated cookie
// using AES-256-GCM: base64url(nonce || ciphertext || tag) over "<did>\x00<sid>".
package cookie

import (
	"encoding/base64"
	"net/http"
	"strings"

	"morgenblau/internal/secret"
)

const (
	cookieName = "mb_session"
	maxAge     = 30 * 24 * 60 * 60 // 30 days
)

// Sealer mints and reads sealed session cookies over a secret.Keyset (supports key rotation).
type Sealer struct {
	keyset *secret.Keyset
}

// New builds a Sealer from a 32-byte key (SESSION_COOKIE_KEY from env, base64-decoded).
func New(key []byte) (*Sealer, error) {
	ks, err := secret.NewKeyset(key)
	if err != nil {
		return nil, err
	}
	return &Sealer{keyset: ks}, nil
}

// Set seals (did, sid) and writes the cookie on w.
func (s *Sealer) Set(w http.ResponseWriter, did, sid string) {
	sealed, err := s.keyset.Seal([]byte(did + "\x00" + sid))
	if err != nil {
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(sealed)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    encoded,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// Get unseals the cookie; ok=false on any failure (missing, garbage, tampered, wrong key) means no session.
func (s *Sealer) Get(r *http.Request) (did, sid string, ok bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return "", "", false
	}
	plain, err := s.keyset.Open(sealed)
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(plain), "\x00", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Clear writes a cookie that overrides the previous one (MaxAge<0).
func (s *Sealer) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
