package secret

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func key(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, KeySize)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestKeyset_SealOpen_RoundTrip(t *testing.T) {
	ks, err := NewKeyset(key(t))
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"token":"secret-value"}`)
	sealed, err := ks.Seal(plain)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, plain) {
		t.Error("ciphertext contains the plaintext")
	}
	got, err := ks.Open(sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip mismatch: got %q", got)
	}
}

// Data sealed under the old key must still open once the new key is prepended.
func TestKeyset_Rotation_OpensWithSecondKey(t *testing.T) {
	oldKey := key(t)
	newKey := key(t)

	oldSet, _ := NewKeyset(oldKey)
	sealed, err := oldSet.Seal([]byte("payload"))
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := NewKeyset(newKey, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := rotated.Open(sealed)
	if err != nil {
		t.Fatalf("Open after rotation: %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("got %q, want payload", got)
	}

	fresh, _ := rotated.Seal([]byte("payload"))
	if _, err := oldSet.Open(fresh); err == nil {
		t.Error("old-only keyset opened data sealed under the new key")
	}
}

func TestKeyset_TamperedCiphertext_Rejected(t *testing.T) {
	ks, _ := NewKeyset(key(t))
	sealed, _ := ks.Seal([]byte("payload"))
	sealed[len(sealed)-1] ^= 0xff // flip a tag byte
	if _, err := ks.Open(sealed); err == nil {
		t.Error("tampered ciphertext opened successfully")
	}
}

func TestKeyset_WrongKey_Rejected(t *testing.T) {
	a, _ := NewKeyset(key(t))
	b, _ := NewKeyset(key(t))
	sealed, _ := a.Seal([]byte("payload"))
	if _, err := b.Open(sealed); err == nil {
		t.Error("data sealed under key A opened under key B")
	}
}

func TestNewKeyset_Rejects(t *testing.T) {
	if _, err := NewKeyset(); err == nil {
		t.Error("empty keyset should error")
	}
	if _, err := NewKeyset(make([]byte, 16)); err == nil {
		t.Error("16-byte key should error (need 32 for AES-256)")
	}
}
