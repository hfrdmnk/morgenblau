package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const KeySize = 32

var ErrOpen = errors.New("secret: no key could open the ciphertext")

type Keyset struct {
	aeads []cipher.AEAD
}

func NewKeyset(keys ...[]byte) (*Keyset, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("secret: keyset requires at least one key")
	}
	aeads := make([]cipher.AEAD, 0, len(keys))
	for i, key := range keys {
		if len(key) != KeySize {
			return nil, fmt.Errorf("secret: key %d must be %d bytes (got %d)", i, KeySize, len(key))
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, fmt.Errorf("secret: new cipher: %w", err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("secret: new gcm: %w", err)
		}
		aeads = append(aeads, aead)
	}
	return &Keyset{aeads: aeads}, nil
}

func (k *Keyset) Seal(plaintext []byte) ([]byte, error) {
	aead := k.aeads[0]
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secret: nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (k *Keyset) Open(sealed []byte) ([]byte, error) {
	for _, aead := range k.aeads {
		ns := aead.NonceSize()
		if len(sealed) < ns+aead.Overhead() {
			continue
		}
		nonce, ct := sealed[:ns], sealed[ns:]
		if pt, err := aead.Open(nil, nonce, ct, nil); err == nil {
			return pt, nil
		}
	}
	return nil, ErrOpen
}
