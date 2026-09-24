package sealing

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestSealOpenAndBinding(t *testing.T) {
	priv, pub, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := PublicKey(priv); !bytes.Equal(p, pub) || !ValidPublicKey(pub) {
		t.Fatal("public key does not derive")
	}
	msg := []byte(`{"id_number":"A123456789","password":"hunter2"}`)
	aad := AAD("credentials", "42", "cathay")
	blob, err := Seal(pub, msg, aad)
	if err != nil {
		t.Fatal(err)
	}
	if !WellFormed(blob) || bytes.Contains(blob, []byte("hunter2")) {
		t.Fatal("blob is malformed or carries the plaintext")
	}
	got, err := Open(priv, blob, aad)
	if err != nil || !bytes.Equal(got, msg) {
		t.Fatalf("open = %q %v", got, err)
	}
	// Moved to another user or connector, or tampered with: it does not open.
	for _, bad := range [][]byte{AAD("credentials", "43", "cathay"), AAD("credentials", "42", "sinopac"), AAD("answer", "42", "cathay")} {
		if _, err := Open(priv, blob, bad); err == nil {
			t.Errorf("opened with aad %q", bad)
		}
	}
	flipped := append([]byte{}, blob...)
	flipped[len(flipped)-1] ^= 1
	if _, err := Open(priv, flipped, aad); err == nil {
		t.Error("opened a tampered blob")
	}
	other, _, _ := NewKey()
	if _, err := Open(other, blob, aad); err == nil {
		t.Error("opened with another runner's key")
	}
}

// A blob made by web/src/lib/seal.ts (WebCrypto) with a fixed key pair:
// the browser and Go agree on the format. Regenerate with
// web/scripts/seal-fixture.ts if the format ever changes.
func TestOpensABrowserBlob(t *testing.T) {
	priv, _ := base64.StdEncoding.DecodeString(browserFixturePrivate)
	blob, _ := base64.StdEncoding.DecodeString(browserFixtureBlob)
	got, err := Open(priv, blob, AAD("credentials", "7", "fake"))
	if err != nil || string(got) != `{"username":"alice","password":"otp"}` {
		t.Fatalf("open = %q %v", got, err)
	}
}

// A throwaway test key pair; it protects nothing.
const (
	browserFixturePrivate = "mW4ImU8Om/3tQlDhvBD9CbEOCjsEM/ZT7MeKah2G6wk=" // gitleaks:allow
	browserFixtureBlob    = "AbIl0v0XsqLPx5K+WSo7T3NUeZm4chBdsPjxF4GYxM8iVv0EFV6ZZyXhRievVh0DktfwuQsTLVPMECdWRl/vv4+y+bWq7LbSN1SNksediZa7yY682ns0OChMlgBuUBQOCPQ="
)
