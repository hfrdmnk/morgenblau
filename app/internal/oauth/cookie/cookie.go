package cookie

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"morgenblau/internal/secret"
)

const (
	cookieName = "mb_session"
	maxAge     = int(30 * 24 * time.Hour / time.Second)
)

type Sealer struct {
	keyset *secret.Keyset
}

func New(key []byte) (*Sealer, error) {
	ks, err := secret.NewKeyset(key)
	if err != nil {
		return nil, err
	}
	return &Sealer{keyset: ks}, nil
}

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
