package routes

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
)

func (f *apiFixture) requireMFA(on bool) {
	f.t.Helper()
	if _, err := f.store.Pool.Exec(context.Background(), "UPDATE system_settings SET mfa_required = $1", on); err != nil {
		f.t.Fatal(err)
	}
	f.ids.InvalidateSettings()
}

// code for a TOTP secret at an offset in steps (0 = now, 1 = the next 30 s).
func totpCode(t *testing.T, secret string, step int) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, time.Now().Add(time.Duration(step)*30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// enrolTOTP sets up an authenticator for a signed-in client and returns the
// secret and the recovery codes.
func enrolTOTP(t *testing.T, c *client) (string, []string) {
	t.Helper()
	var setup TOTPSetupDTO
	c.json("POST", "/api/me/mfa/totp", nil, 200, &setup)
	if setup.Secret == "" || !strings.HasPrefix(setup.QR, "data:image/png;base64,") || !strings.HasPrefix(setup.URI, "otpauth://totp/") {
		t.Fatalf("setup = %+v", setup)
	}
	res, b := c.do("POST", "/api/me/mfa/totp/confirm", map[string]string{"code": "000000"})
	if res.StatusCode != 422 && totpCode(t, setup.Secret, 0) != "000000" {
		t.Fatalf("wrong code accepted: %d %s", res.StatusCode, b)
	}
	var out EnrolledDTO
	c.json("POST", "/api/me/mfa/totp/confirm", map[string]string{"code": totpCode(t, setup.Secret, 0)}, 200, &out)
	return setup.Secret, out.RecoveryCodes
}

func TestMFAEnforcementAndTOTP(t *testing.T) {
	f := newAPI(t)
	f.requireMFA(true)
	alice := f.browser("alice")

	// A password-only session reaches enrolment and nothing else.
	res, b := alice.do("GET", "/api/books", nil)
	if res.StatusCode != 403 || errorCode(t, b) != "mfa_enrollment_required" {
		t.Fatalf("books at aal 1 = %d %s", res.StatusCode, b)
	}
	var me UserDTO
	alice.json("GET", "/api/me", nil, 200, &me)
	if !me.MFARequired || me.MFAEnrolled || me.SessionAAL != 1 {
		t.Fatalf("me = %+v", me)
	}

	secret, codes := enrolTOTP(t, alice)
	if len(codes) != 10 {
		t.Fatalf("recovery codes = %v", codes)
	}
	// Enrolling raised this very session.
	alice.json("GET", "/api/books", nil, 200, nil)

	// A new sign-in needs the second step.
	anon := &client{f: f, csrf: true}
	var step loginResponse
	anon.json("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "correct horse"}, 200, &step)
	if !step.MFARequired || step.User != nil || step.Challenge == "" || strings.Join(step.Methods, ",") != "totp,recovery" {
		t.Fatalf("password step = %+v", step)
	}
	// The code used to confirm (this 30 s step) is spent: no replay.
	res, _ = anon.do("POST", "/api/auth/mfa", map[string]string{"challenge": step.Challenge, "method": "totp", "code": totpCode(t, secret, 0)})
	if res.StatusCode != 422 {
		t.Fatalf("replayed TOTP = %d", res.StatusCode)
	}
	res, b = anon.do("POST", "/api/auth/mfa", map[string]string{"challenge": step.Challenge, "method": "totp", "code": totpCode(t, secret, 1)})
	if res.StatusCode != 200 {
		t.Fatalf("next-step TOTP = %d %s", res.StatusCode, b)
	}
	anon.takeCookie(res)
	anon.json("GET", "/api/me", nil, 200, &me)
	if me.SessionAAL != 2 || !me.MFAEnrolled {
		t.Fatalf("after second step: %+v", me)
	}
	// The challenge worked once.
	res, _ = (&client{f: f, csrf: true}).do("POST", "/api/auth/mfa", map[string]string{"challenge": step.Challenge, "method": "totp", "code": totpCode(t, secret, 1)})
	if res.StatusCode != 404 {
		t.Fatalf("reused challenge = %d", res.StatusCode)
	}

	// A recovery code works once, with or without its dash.
	signin := func() string {
		var s loginResponse
		(&client{f: f, csrf: true}).json("POST", "/api/auth/login", map[string]string{"username": "alice", "password": "correct horse"}, 200, &s)
		return s.Challenge
	}
	rc := strings.ToUpper(strings.ReplaceAll(codes[0], "-", ""))
	(&client{f: f, csrf: true}).json("POST", "/api/auth/mfa", map[string]string{"challenge": signin(), "method": "recovery", "code": rc}, 200, nil)
	res, _ = (&client{f: f, csrf: true}).do("POST", "/api/auth/mfa", map[string]string{"challenge": signin(), "method": "recovery", "code": codes[0]})
	if res.StatusCode != 422 {
		t.Fatalf("recovery code reused = %d", res.StatusCode)
	}

	// Five wrong codes end the challenge -- or the sign-in throttle (5 per
	// username and address, counting the misses above) stops them first.
	ch := signin()
	for i := 0; i < 5; i++ {
		(&client{f: f, csrf: true}).do("POST", "/api/auth/mfa", map[string]string{"challenge": ch, "method": "totp", "code": "123456x"})
	}
	res, _ = (&client{f: f, csrf: true}).do("POST", "/api/auth/mfa", map[string]string{"challenge": ch, "method": "totp", "code": totpCode(t, secret, 1)})
	if res.StatusCode != 404 && res.StatusCode != 429 {
		t.Fatalf("challenge after 5 misses = %d", res.StatusCode)
	}

	// The last factor stays while it is required; the password is asked for.
	res, b = anon.do("DELETE", "/api/me/mfa/totp", map[string]string{"current_password": "nope"})
	if res.StatusCode != 422 || !strings.Contains(string(b), `"current_password":"wrong"`) {
		t.Fatalf("remove with wrong password = %d %s", res.StatusCode, b)
	}
	res, b = anon.do("DELETE", "/api/me/mfa/totp", map[string]string{"current_password": "correct horse"})
	if res.StatusCode != 409 || errorCode(t, b) != "last_factor" {
		t.Fatalf("remove last factor = %d %s", res.StatusCode, b)
	}
	// Regenerating the codes needs the two-factor session and the password.
	var again EnrolledDTO
	anon.json("POST", "/api/me/mfa/recovery-codes", map[string]string{"current_password": "correct horse"}, 200, &again)
	if len(again.RecoveryCodes) != 10 || again.RecoveryCodes[1] == codes[1] {
		t.Fatalf("regenerated = %v", again.RecoveryCodes)
	}
}

func TestEmailFactorAndAdminReset(t *testing.T) {
	f := newAPI(t)
	f.requireMFA(true)
	bob := f.browser("bob")

	var ch challengeRequest
	bob.json("POST", "/api/me/mfa/email", nil, 200, &ch)
	var enrolled EnrolledDTO
	bob.json("POST", "/api/me/mfa/email/confirm", map[string]string{"challenge": ch.Challenge, "code": mailCode(t, f, "bob@example.com")}, 200, &enrolled)
	if len(enrolled.RecoveryCodes) != 10 {
		t.Fatalf("first factor gave no recovery codes: %+v", enrolled)
	}

	anon := &client{f: f, csrf: true}
	var step loginResponse
	anon.json("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "correct horse"}, 200, &step)
	if strings.Join(step.Methods, ",") != "email,recovery" {
		t.Fatalf("methods = %v", step.Methods)
	}
	anon.json("POST", "/api/auth/mfa/email", map[string]string{"challenge": step.Challenge}, 204, nil)
	// A second send within a minute is throttled.
	if res, _ := anon.do("POST", "/api/auth/mfa/email", map[string]string{"challenge": step.Challenge}); res.StatusCode != 429 {
		t.Fatalf("resend at once = %d", res.StatusCode)
	}
	res, b := anon.do("POST", "/api/auth/mfa", map[string]string{"challenge": step.Challenge, "method": "email", "code": mailCode(t, f, "bob@example.com")})
	if res.StatusCode != 200 {
		t.Fatalf("email code = %d %s", res.StatusCode, b)
	}
	anon.takeCookie(res)
	anon.json("GET", "/api/books", nil, 200, nil)

	// Alice (admin) resets bob: his factors and sessions are gone.
	alice := f.browser("alice")
	enrolTOTP(t, alice) // the admin API needs her own two-factor session
	var users []AdminUserDTO
	alice.json("GET", "/api/admin/users", nil, 200, &users)
	var bobID int64
	for _, u := range users {
		if u.Username == "bob" {
			bobID = u.ID
		}
	}
	alice.json("POST", fmt.Sprintf("/api/admin/users/%d/reset-mfa", bobID), nil, 204, nil)
	if res, _ := anon.do("GET", "/api/me", nil); res.StatusCode != 401 {
		t.Fatalf("bob's session after reset = %d", res.StatusCode)
	}
	var fresh loginResponse
	(&client{f: f, csrf: true}).json("POST", "/api/auth/login", map[string]string{"username": "bob", "password": "correct horse"}, 200, &fresh)
	if fresh.MFARequired || fresh.User == nil {
		t.Fatalf("after reset the login still asks for a factor: %+v", fresh)
	}
}

// A stranger guessing bob's password from many addresses must not lock bob
// out of the device he signs in from.
func TestOwnerDeviceIsExemptFromUsernameLockout(t *testing.T) {
	f := newAPI(t)
	ctx := context.Background()
	home := netip.MustParseAddr("100.99.1.2")
	for i := 0; i < 25; i++ {
		ip := netip.AddrFrom4([4]byte{203, 0, 113, byte(i)})
		if err := f.mgr.Record(ctx, auth.Event{Username: "bob", IP: &ip, Name: "password_bad", Failure: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := auth.IsThrottled(f.mgr.Check(ctx, "bob", &home)); !ok {
		t.Fatal("25 failures elsewhere did not trip the username-wide limit")
	}
	u, err := f.store.GetUserByUsername(ctx, "bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.mgr.OpenSession(ctx, u, "web", auth.Client{IP: &home}, auth.AALPassword); err != nil {
		t.Fatal(err)
	}
	if err := f.mgr.Check(ctx, "bob", &home); err != nil {
		t.Fatalf("bob's own device is still locked out: %v", err)
	}
	stranger := netip.MustParseAddr("198.51.100.7")
	if _, ok := auth.IsThrottled(f.mgr.Check(ctx, "bob", &stranger)); !ok {
		t.Fatal("the exemption leaked to other addresses")
	}
}
