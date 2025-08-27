package auth

import (
	"fmt"
	"testing"
)

func TestJWT(t *testing.T) {
	privKey := []byte("secret")
	jwt := New("https://ledger.chenantunez.com", []string{"rigelledger.web", "rigelledger.ios"}, privKey)
	claims := map[string]any{
		"name": "John Doe",
		"role": "admin",
	}
	token, err := jwt.Sign("cwc1222", claims, AccessTokenLifetime)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("token: ", string(token))

	parsed, err := jwt.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	json, err := parsed.ToJSONString()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("parsed: ", json)
}
