// Package cookie seals (DID, session_id) into an authenticated cookie.
//
// AEAD: AES-256-GCM. The cookie value is base64url(nonce || ciphertext || tag).
// Plaintext is "<did>\x00<sid>" — both are opaque strings to this package; no
// length limits beyond AEAD's 64-bit input cap.
package cookie

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

const (
	cookieName = "mb_session"
	maxAge     = 30 * 24 * 60 * 60 // 30 days
	keySize    = 32                // AES-256
)

// Sealer mints and reads sealed session cookies.
type Sealer struct {
	aead cipher.AEAD
}

// New builds a Sealer from a 32-byte key. Use SESSION_COOKIE_KEY from env,
// decoded with base64.
func New(key []byte) (*Sealer, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("cookie key must be %d bytes (got %d)", keySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// Set seals (did, sid) and writes the cookie on w.
func (s *Sealer) Set(w http.ResponseWriter, did, sid string) {
	plain := []byte(did + "\x00" + sid)
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return
	}
	sealed := s.aead.Seal(nonce, nonce, plain, nil)
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

// Get unseals the cookie. ok=false on any failure (missing, garbage,
// tampered, wrong key) — callers treat this as "no session".
func (s *Sealer) Get(r *http.Request) (did, sid string, ok bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", "", false
	}
	sealed, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return "", "", false
	}
	ns := s.aead.NonceSize()
	if len(sealed) < ns+s.aead.Overhead() {
		return "", "", false
	}
	nonce, ct := sealed[:ns], sealed[ns:]
	plain, err := s.aead.Open(nil, nonce, ct, nil)
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
