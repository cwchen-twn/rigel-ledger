// Package secretbox seals the few secrets the database must hold (the SMTP
// password now, TOTP seeds later) with AES-256-GCM under APP_ENCRYPTION_KEY,
// so a dump or a backup alone does not reveal them.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// KeySize is the length of an AES-256 key.
const KeySize = 32

// devKey is used only in development when APP_ENCRYPTION_KEY is unset. It is
// public, so anything sealed with it is not secret.
var devKey = []byte("rigel-ledger-development-key-32b")

type Box struct {
	aead cipher.AEAD
	Dev  bool
}

// New parses a base64 key of 32 bytes. An empty key gives the development
// box when allowDev is set, and an error otherwise.
func New(b64 string, allowDev bool) (*Box, error) {
	var key []byte
	dev := false
	if b64 == "" {
		if !allowDev {
			return nil, errors.New("APP_ENCRYPTION_KEY is not set (generate one with: openssl rand -base64 32)")
		}
		key, dev = devKey, true
	} else {
		var err error
		key, err = base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("APP_ENCRYPTION_KEY is not base64: %w", err)
		}
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("APP_ENCRYPTION_KEY must decode to %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead, Dev: dev}, nil
}

// Seal returns nonce || ciphertext.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

var ErrOpen = errors.New("secretbox: cannot open (wrong APP_ENCRYPTION_KEY?)")

func (b *Box) Open(sealed []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, ErrOpen
	}
	out, err := b.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return nil, ErrOpen
	}
	return out, nil
}
