// Package sealing is the one format for secrets meant for the sync runner
// alone: institution credentials and challenge answers. They are sealed
// (in the browser, web/src/lib/seal.ts) to the runner's X25519 public key;
// the app stores the blob but holds no key that opens it.
//
// Blob, version 1:
//
//	0x01 | ephemeral X25519 public key (32) | AES-GCM nonce (12) | ciphertext+tag
//
// key = HKDF-SHA256(ikm = X25519(ephemeral, runner), salt = ephemeral || runner,
// info = "rigel-ledger sealed v1"), 32 bytes, used for AES-256-GCM with the
// caller's additional data (see AAD), which binds a blob to where it belongs.
// Everything is in the Go standard library and in WebCrypto, and node:crypto
// has the same primitives for the Node runner.
package sealing

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"strings"
)

const (
	version  = 1
	keyLen   = 32
	nonceLen = 12
	info     = "rigel-ledger sealed v1"
)

var ErrInvalid = errors.New("sealing: not a valid sealed blob")

// NewKey makes a runner key pair (raw 32-byte X25519 keys).
func NewKey() (private, public []byte, err error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return k.Bytes(), k.PublicKey().Bytes(), nil
}

// PublicKey derives the public half of a raw private key.
func PublicKey(private []byte) ([]byte, error) {
	k, err := ecdh.X25519().NewPrivateKey(private)
	if err != nil {
		return nil, err
	}
	return k.PublicKey().Bytes(), nil
}

// ValidPublicKey reports whether b is a usable X25519 public key.
func ValidPublicKey(b []byte) bool {
	_, err := ecdh.X25519().NewPublicKey(b)
	return err == nil && len(b) == keyLen
}

// AAD is the additional data for a purpose and its context, joined with
// NUL: AAD("credentials", "42", "cathay") binds a blob to user 42's cathay
// connection, so a blob copied onto another row does not open.
func AAD(purpose string, context ...string) []byte {
	return []byte("rigel-ledger/" + purpose + "/v1\x00" + strings.Join(context, "\x00"))
}

func aead(shared, eph, runner []byte) (cipher.AEAD, error) {
	salt := append(append([]byte{}, eph...), runner...)
	key, err := hkdf.Key(sha256.New, shared, salt, info, keyLen)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal encrypts plaintext to the runner's public key. The app never calls it
// with real credentials -- the browser seals those -- only tests and the
// fake runner do.
func Seal(runnerPublic, plaintext, aad []byte) ([]byte, error) {
	rp, err := ecdh.X25519().NewPublicKey(runnerPublic)
	if err != nil {
		return nil, err
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := eph.ECDH(rp)
	if err != nil {
		return nil, err
	}
	g, err := aead(shared, eph.PublicKey().Bytes(), runnerPublic)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{version}, eph.PublicKey().Bytes()...)
	out = append(out, nonce...)
	return g.Seal(out, nonce, plaintext, aad), nil
}

// Open is the runner's side.
func Open(runnerPrivate, blob, aad []byte) ([]byte, error) {
	if !WellFormed(blob) {
		return nil, ErrInvalid
	}
	priv, err := ecdh.X25519().NewPrivateKey(runnerPrivate)
	if err != nil {
		return nil, err
	}
	eph := blob[1 : 1+keyLen]
	ep, err := ecdh.X25519().NewPublicKey(eph)
	if err != nil {
		return nil, ErrInvalid
	}
	shared, err := priv.ECDH(ep)
	if err != nil {
		return nil, ErrInvalid
	}
	g, err := aead(shared, eph, priv.PublicKey().Bytes())
	if err != nil {
		return nil, err
	}
	nonce := blob[1+keyLen : 1+keyLen+nonceLen]
	pt, err := g.Open(nil, nonce, blob[1+keyLen+nonceLen:], aad)
	if err != nil {
		return nil, ErrInvalid
	}
	return pt, nil
}

// WellFormed is all the app can check about a blob: the version and that it
// is long enough to hold a key, a nonce and a tag. It cannot see inside.
func WellFormed(blob []byte) bool {
	return len(blob) >= 1+keyLen+nonceLen+16 && blob[0] == version && len(blob) <= 16<<10
}
