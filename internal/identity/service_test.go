package identity

import (
	"testing"
	"time"
)

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		lang string
		d    time.Duration
		want string
	}{
		{"en", 7 * 24 * time.Hour, "7 days"},
		{"en", 24 * time.Hour, "1 day"},
		{"en", 10 * time.Minute, "10 minutes"},
		{"zh", 10 * time.Minute, "10 分鐘"},
		{"es", 24 * time.Hour, "1 día"},
		{"es", 36 * time.Hour, "36 horas"},
		{"en", 90 * time.Second, "1 minute"},
	}
	for _, c := range cases {
		if got := humanDuration(c.lang, c.d); got != c.want {
			t.Errorf("humanDuration(%s, %s) = %q, want %q", c.lang, c.d, got, c.want)
		}
	}
}

func TestCodes(t *testing.T) {
	code, hash, err := newCode()
	if err != nil || len(code) != 6 {
		t.Fatalf("code %q err %v", code, err)
	}
	if !codeMatches(hash, code) || codeMatches(hash, "000000x") || codeMatches(nil, code) {
		t.Fatal("codeMatches is wrong")
	}
}
