package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"math/big"
)

// newCode is a 6-digit code for in-app verification. Its hash is stored;
// guessing is bounded by maxCodeAttempts per code and the sign-in throttle.
func newCode() (code string, hash []byte, err error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", nil, err
	}
	code = n.String()
	for len(code) < 6 {
		code = "0" + code
	}
	return code, hashCode(code), nil
}

func hashCode(code string) []byte {
	h := sha256.Sum256([]byte("rigel-email-code:" + code))
	return h[:]
}

func codeMatches(hash []byte, code string) bool {
	return len(hash) > 0 && subtle.ConstantTimeCompare(hash, hashCode(code)) == 1
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
