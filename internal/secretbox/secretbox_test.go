package secretbox

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestSealOpen(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, KeySize))
	b, err := New(key, false)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := b.Seal([]byte("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("hunter2")) {
		t.Fatal("plaintext visible in sealed bytes")
	}
	got, err := b.Open(sealed)
	if err != nil || string(got) != "hunter2" {
		t.Fatalf("open: %q %v", got, err)
	}

	other, _ := New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, KeySize)), false)
	if _, err := other.Open(sealed); err == nil {
		t.Fatal("a different key opened the box")
	}
}

func TestKeyRules(t *testing.T) {
	if _, err := New("", false); err == nil {
		t.Fatal("empty key accepted outside development")
	}
	if b, err := New("", true); err != nil || !b.Dev {
		t.Fatalf("development fallback: %v", err)
	}
	if _, err := New(base64.StdEncoding.EncodeToString([]byte("short")), false); err == nil {
		t.Fatal("short key accepted")
	}
}
