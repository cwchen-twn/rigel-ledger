package routes

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/identity"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

var (
	codePattern = regexp.MustCompile(`\b\d{6}\b`)
	linkPattern = regexp.MustCompile(`https://ledger\.test(/\S+)`)
)

func mailCode(t *testing.T, f *apiFixture, to string) string {
	t.Helper()
	m := f.outbox.last(t, to)
	c := codePattern.FindString(m.Text)
	if c == "" {
		t.Fatalf("no code in mail to %s: %s", to, m.Text)
	}
	return c
}

func mailLink(t *testing.T, f *apiFixture, to string) string {
	t.Helper()
	m := f.outbox.last(t, to)
	l := linkPattern.FindStringSubmatch(m.Text)
	if l == nil {
		t.Fatalf("no link in mail to %s: %s", to, m.Text)
	}
	return l[1]
}

// signIn logs in with a password and returns the cookie client, without the
// fixture's expectations about who the user is.
func (f *apiFixture) signIn(user, password string) (*client, *http.Response, []byte) {
	c := &client{f: f, csrf: true}
	res, body := c.do("POST", "/api/auth/login", map[string]string{"username": user, "password": password})
	c.takeCookie(res)
	return c, res, body
}

func (c *client) takeCookie(res *http.Response) {
	for _, ck := range res.Cookies() {
		if ck.Name == auth.CookieName && ck.Value != "" {
			c.cookie = ck
		}
	}
}

var profile = map[string]any{
	"display_name": "Dora", "language": "zh", "display_currency": "TWD",
	"timezone": "Asia/Taipei", "date_format": "YYYY/MM/DD", "theme": "dark",
}

func with(m map[string]any, kv ...any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	for i := 0; i < len(kv); i += 2 {
		out[kv[i].(string)] = kv[i+1]
	}
	return out
}

func TestFirstLoginWizardGatesTheAPI(t *testing.T) {
	f := newAPI(t)
	if _, err := f.svc.CreateUser(context.Background(), ledger.NewUser{Username: "dora", Email: "dora@example.com", Password: "correct horse"}); err != nil {
		t.Fatal(err)
	}
	dora, res, b := f.signIn("dora", "correct horse")
	if res.StatusCode != 200 {
		t.Fatalf("login: %d %s", res.StatusCode, b)
	}
	var me UserDTO
	dora.json("GET", "/api/me", nil, 200, &me)
	if me.Initialized || me.EmailVerified {
		t.Fatalf("me = %+v", me)
	}
	res, b = dora.do("GET", "/api/books", nil)
	if res.StatusCode != 403 || errorCode(t, b) != "onboarding_required" {
		t.Fatalf("books before the wizard: %d %s", res.StatusCode, b)
	}

	// The address must be verified before the wizard can finish.
	res, b = dora.do("POST", "/api/me/onboarding", with(profile, "username", "dora"))
	if res.StatusCode != 422 || !strings.Contains(string(b), `"email":"unverified"`) {
		t.Fatalf("onboarding unverified: %d %s", res.StatusCode, b)
	}
	// No current password during the wizard.
	dora.json("POST", "/api/me/email", map[string]string{"email": "dora@example.com"}, 204, nil)
	code := mailCode(t, f, "dora@example.com")
	res, b = dora.do("POST", "/api/me/email/confirm", map[string]string{"code": "000000"})
	if res.StatusCode != 422 || !strings.Contains(string(b), `"code":"wrong"`) {
		if code != "000000" {
			t.Fatalf("wrong code: %d %s", res.StatusCode, b)
		}
	}
	dora.json("POST", "/api/me/email/confirm", map[string]string{"code": code}, 200, &me)
	if !me.EmailVerified {
		t.Fatalf("not verified: %+v", me)
	}

	res, b = dora.do("POST", "/api/me/onboarding", with(profile, "username", "bob"))
	if res.StatusCode != 409 || !strings.Contains(string(b), `"username":"taken"`) {
		t.Fatalf("taken username: %d %s", res.StatusCode, b)
	}
	dora.json("POST", "/api/me/onboarding", with(profile, "username", "dora.chen"), 200, &me)
	if !me.Initialized || me.Username != "dora.chen" || me.Language != "zh" || me.DisplayCurrency != "TWD" || me.Theme != "dark" {
		t.Fatalf("after wizard: %+v", me)
	}
	dora.json("GET", "/api/books", nil, 200, nil)
	res, _ = dora.do("POST", "/api/me/onboarding", with(profile, "username", "dora.chen"))
	if res.StatusCode != 409 {
		t.Fatalf("second onboarding = %d", res.StatusCode)
	}
}

func TestInvitationFlow(t *testing.T) {
	f := newAPI(t)
	alice, bob := f.browser("alice"), f.browser("bob")

	res, _ := bob.do("GET", "/api/admin/users", nil)
	if res.StatusCode != 404 {
		t.Fatalf("non-admin sees the admin API: %d", res.StatusCode)
	}
	res, b := alice.do("POST", "/api/admin/invitations", map[string]string{"username": "bob", "email": "x@example.com"})
	if res.StatusCode != 409 || !strings.Contains(string(b), `"username":"taken"`) {
		t.Fatalf("taken: %d %s", res.StatusCode, b)
	}
	var invited AdminUserDTO
	alice.json("POST", "/api/admin/invitations", map[string]string{"username": "erin", "email": "Erin@Example.com"}, 201, &invited)
	if !invited.Invited || !invited.InvitePending {
		t.Fatalf("invited = %+v", invited)
	}
	// An invited user has no password yet.
	if _, res, _ := f.signIn("erin", ""); res.StatusCode != 401 {
		t.Fatalf("invited user signed in without a password: %d", res.StatusCode)
	}

	link := mailLink(t, f, "Erin@Example.com")
	if !strings.HasPrefix(link, "/invite/") {
		t.Fatalf("link = %s", link)
	}
	anon := &client{f: f, csrf: true}
	var inv InvitationDTO
	anon.json("GET", "/api"+strings.Replace(link, "/invite/", "/auth/invite/", 1), nil, 200, &inv)
	if inv.Username != "erin" || inv.Email != "Erin@Example.com" {
		t.Fatalf("preview = %+v", inv)
	}

	accept := "/api" + strings.Replace(link, "/invite/", "/auth/invite/", 1)
	res, b = anon.do("POST", accept, with(profile, "username", "erin", "password", "short"))
	if res.StatusCode != 422 || !strings.Contains(string(b), `"password":"too_short"`) {
		t.Fatalf("short password: %d %s", res.StatusCode, b)
	}
	res, b = anon.do("POST", accept, with(profile, "username", "erin", "password", "a long enough one"))
	if res.StatusCode != 200 {
		t.Fatalf("accept: %d %s", res.StatusCode, b)
	}
	anon.takeCookie(res)
	var me UserDTO
	anon.json("GET", "/api/me", nil, 200, &me)
	if !me.Initialized || !me.EmailVerified || me.Timezone != "Asia/Taipei" {
		t.Fatalf("after accepting: %+v", me)
	}
	anon.json("GET", "/api/books", nil, 200, nil)

	// The link worked once.
	res, _ = (&client{f: f, csrf: true}).do("POST", accept, with(profile, "username", "erin", "password", "a long enough one"))
	if res.StatusCode != 404 {
		t.Fatalf("reused invitation = %d", res.StatusCode)
	}
	if _, res, _ := f.signIn("erin", "a long enough one"); res.StatusCode != 200 {
		t.Fatalf("password sign-in after accepting = %d", res.StatusCode)
	}

	// Revoking works only while an invitation is open.
	var users []AdminUserDTO
	alice.json("GET", "/api/admin/users", nil, 200, &users)
	res, _ = alice.do("DELETE", fmt.Sprintf("/api/admin/invitations/%d", invited.ID), nil)
	if res.StatusCode != 409 {
		t.Fatalf("revoke accepted invitation = %d", res.StatusCode)
	}
	alice.json("POST", "/api/admin/invitations", map[string]string{"username": "gus", "email": "gus@example.com"}, 201, &invited)
	first := mailLink(t, f, "gus@example.com")
	alice.json("POST", fmt.Sprintf("/api/admin/invitations/%d/resend", invited.ID), nil, 204, nil)
	if mailLink(t, f, "gus@example.com") == first {
		t.Fatal("resend reused the link")
	}
	res, _ = anon.do("GET", "/api"+strings.Replace(first, "/invite/", "/auth/invite/", 1), nil)
	if res.StatusCode != 404 {
		t.Fatalf("old link after resend = %d", res.StatusCode)
	}
	alice.json("DELETE", fmt.Sprintf("/api/admin/invitations/%d", invited.ID), nil, 204, nil)
}

func adminSettings(t *testing.T, c *client) map[string]any {
	t.Helper()
	var s map[string]any
	c.json("GET", "/api/admin/settings", nil, 200, &s)
	return s
}

// settingsPatch is the PATCH body for the settings: the GET minus the mail
// fields, which have their own endpoint.
func settingsPatch(t *testing.T, c *client) map[string]any {
	t.Helper()
	s := adminSettings(t, c)
	for k := range s {
		if strings.HasPrefix(k, "mail_") || strings.HasPrefix(k, "smtp_") || k == "updated_at" {
			delete(s, k)
		}
	}
	return s
}

func TestRegistrationModes(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	anon := &client{f: f, csrf: true}
	signup := map[string]string{"username": "frank", "email": "frank@example.com", "password": "correct horse", "language": "es"}

	res, b := anon.do("POST", "/api/auth/register", signup)
	if res.StatusCode != 403 || errorCode(t, b) != "registration_closed" {
		t.Fatalf("closed: %d %s", res.StatusCode, b)
	}

	s := settingsPatch(t, alice)
	s["registration"] = "open"
	alice.json("PATCH", "/api/admin/settings", s, 200, nil)
	var cfg AuthConfigDTO
	anon.json("GET", "/api/auth/config", nil, 200, &cfg)
	if cfg.Registration != "open" {
		t.Fatalf("config = %+v", cfg)
	}

	anon.json("POST", "/api/auth/register", signup, 202, nil)
	// The same name again while the first link is unused.
	res, b = anon.do("POST", "/api/auth/register", signup)
	if res.StatusCode != 409 || errorCode(t, b) != "pending_registration" {
		t.Fatalf("pending: %d %s", res.StatusCode, b)
	}
	// No user exists until the link is clicked.
	if _, res, _ := f.signIn("frank", "correct horse"); res.StatusCode != 401 {
		t.Fatalf("sign-in before confirming = %d", res.StatusCode)
	}
	link := mailLink(t, f, "frank@example.com")
	token := strings.TrimPrefix(link, "/verify?token=")
	res, b = anon.do("POST", "/api/auth/verify-link", map[string]string{"token": token})
	if res.StatusCode != 200 {
		t.Fatalf("verify: %d %s", res.StatusCode, b)
	}
	anon.takeCookie(res)
	var me UserDTO
	anon.json("GET", "/api/me", nil, 200, &me)
	if me.Initialized || !me.EmailVerified || me.Language != "es" {
		t.Fatalf("registered user = %+v", me)
	}
	res, _ = (&client{f: f, csrf: true}).do("POST", "/api/auth/verify-link", map[string]string{"token": token})
	if res.StatusCode != 404 {
		t.Fatalf("reused sign-up link = %d", res.StatusCode)
	}

	// Request mode: requests reach the admins, and approving one invites.
	s["registration"] = "request"
	alice.json("PATCH", "/api/admin/settings", s, 200, nil)
	visitor := &client{f: f, csrf: true}
	visitor.json("POST", "/api/auth/request-access", map[string]string{"username": "hana", "email": "hana@example.com", "message": "<script>hi</script>"}, 202, nil)
	if m := f.outbox.last(t, "alice@example.com"); !strings.Contains(m.Subject, "hana") || strings.Contains(m.HTML, "<script>") {
		t.Fatalf("admin notice: %+v", m)
	}
	// A taken username is accepted silently, and filed nowhere.
	visitor.json("POST", "/api/auth/request-access", map[string]string{"username": "bob", "email": "b2@example.com"}, 202, nil)
	var reqs []AccessRequestDTO
	alice.json("GET", "/api/admin/access-requests?status=pending", nil, 200, &reqs)
	if len(reqs) != 1 || reqs[0].Username != "hana" {
		t.Fatalf("requests = %+v", reqs)
	}
	alice.json("POST", fmt.Sprintf("/api/admin/access-requests/%d/approve", reqs[0].ID), nil, 204, nil)
	// The admin's log includes events that carry no address (invite_sent).
	var events []AuthEventDTO
	alice.json("GET", "/api/admin/events", nil, 200, &events)
	if len(events) == 0 || events[0].Event != "access_approved" {
		t.Fatalf("admin events = %+v", events)
	}
	if !strings.HasPrefix(mailLink(t, f, "hana@example.com"), "/invite/") {
		t.Fatal("approval did not send an invitation")
	}
	res, _ = alice.do("POST", fmt.Sprintf("/api/admin/access-requests/%d/reject", reqs[0].ID), nil)
	if res.StatusCode != 409 {
		t.Fatalf("second decision = %d", res.StatusCode)
	}
}

func TestEmailChangeNeedsTheCode(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")

	res, b := alice.do("POST", "/api/me/email", map[string]string{"email": "new@example.com", "current_password": "nope"})
	if res.StatusCode != 422 || !strings.Contains(string(b), `"current_password":"wrong"`) {
		t.Fatalf("wrong password: %d %s", res.StatusCode, b)
	}
	res, b = alice.do("POST", "/api/me/email", map[string]string{"email": "BOB@example.com", "current_password": "correct horse"})
	if res.StatusCode != 409 || !strings.Contains(string(b), `"email":"taken"`) {
		t.Fatalf("taken (case-insensitive): %d %s", res.StatusCode, b)
	}
	alice.json("POST", "/api/me/email", map[string]string{"email": "new@example.com", "current_password": "correct horse"}, 204, nil)
	var me UserDTO
	alice.json("GET", "/api/me", nil, 200, &me)
	if me.Email != "alice@example.com" || me.PendingEmail != "new@example.com" {
		t.Fatalf("pending: %+v", me)
	}
	alice.json("POST", "/api/me/email/confirm", map[string]string{"code": mailCode(t, f, "new@example.com")}, 200, &me)
	if me.Email != "new@example.com" || !me.EmailVerified {
		t.Fatalf("changed: %+v", me)
	}
	if m := f.outbox.last(t, "alice@example.com"); !strings.Contains(m.Text, "new@example.com") {
		t.Fatalf("old address not told: %+v", m)
	}

	// Cancel drops a pending change.
	alice.json("POST", "/api/me/email", map[string]string{"email": "other@example.com", "current_password": "correct horse"}, 204, nil)
	alice.json("DELETE", "/api/me/email/pending", nil, 204, nil)
	me = UserDTO{}
	alice.json("GET", "/api/me", nil, 200, &me)
	if me.PendingEmail != "" {
		t.Fatalf("still pending: %+v", me)
	}
}

func TestLoginThrottle(t *testing.T) {
	f := newAPI(t)
	for i := 0; i < 5; i++ {
		if _, res, _ := f.signIn("bob", "wrong"); res.StatusCode != 401 {
			t.Fatalf("attempt %d = %d", i+1, res.StatusCode)
		}
	}
	// Even the right password is refused now, with a Retry-After.
	_, res, b := f.signIn("bob", "correct horse")
	if res.StatusCode != 429 || errorCode(t, b) != "too_many_attempts" || res.Header.Get("Retry-After") == "" {
		t.Fatalf("6th attempt: %d %s %v", res.StatusCode, b, res.Header)
	}
	// An unknown username is throttled the same way, so 429 reveals nothing.
	for i := 0; i < 5; i++ {
		f.signIn("ghost", "x")
	}
	if _, res, _ := f.signIn("ghost", "x"); res.StatusCode != 429 {
		t.Fatalf("unknown user = %d", res.StatusCode)
	}

	// The failures are in bob's audit, visible to bob once he can sign in.
	if _, err := f.store.Pool.Exec(context.Background(), "DELETE FROM auth_events WHERE event = 'password_bad'"); err != nil {
		t.Fatal(err)
	}
	bob := f.browser("bob")
	var events []AuthEventDTO
	bob.json("GET", "/api/me/events", nil, 200, &events)
	if len(events) < 2 || events[0].Event != "signed_in" || events[1].Event != "password_ok" || events[0].IP != "127.0.0.1" {
		t.Fatalf("events = %+v", events)
	}
}

func TestSessionsCanBeRevoked(t *testing.T) {
	f := newAPI(t)
	laptop, phone := f.browser("bob"), f.browser("bob")
	var sessions []SessionDTO
	laptop.json("GET", "/api/me/sessions", nil, 200, &sessions)
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	var other int64
	for _, s := range sessions {
		if !s.Current {
			other = s.ID
		}
	}
	laptop.json("DELETE", fmt.Sprintf("/api/me/sessions/%d", other), nil, 204, nil)
	if res, _ := phone.do("GET", "/api/me", nil); res.StatusCode != 401 {
		t.Fatalf("revoked session still works: %d", res.StatusCode)
	}
	// Someone else's session is not yours to revoke.
	carol := f.browser("carol")
	var cs []SessionDTO
	carol.json("GET", "/api/me/sessions", nil, 200, &cs)
	if res, _ := laptop.do("DELETE", fmt.Sprintf("/api/me/sessions/%d", cs[0].ID), nil); res.StatusCode != 404 {
		t.Fatalf("revoking another user's session = %d", res.StatusCode)
	}
}

func TestAdminUsersAndMailSettings(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	bob := f.browser("bob")

	var users []AdminUserDTO
	alice.json("GET", "/api/admin/users", nil, 200, &users)
	ids := map[string]int64{}
	for _, u := range users {
		ids[u.Username] = u.ID
	}
	res, b := alice.do("PATCH", fmt.Sprintf("/api/admin/users/%d", ids["alice"]), map[string]bool{"is_admin": false})
	if res.StatusCode != 403 || errorCode(t, b) != "self" {
		t.Fatalf("self-demotion: %d %s", res.StatusCode, b)
	}
	alice.json("PATCH", fmt.Sprintf("/api/admin/users/%d", ids["bob"]), map[string]bool{"is_active": false}, 200, nil)
	if res, _ := bob.do("GET", "/api/me", nil); res.StatusCode != 401 {
		t.Fatalf("deactivated user's session = %d", res.StatusCode)
	}

	// Until mail is saved the driver is the default "log": the test says so
	// instead of claiming it was sent.
	var sent MailTestDTO
	alice.json("POST", "/api/admin/mail/test", nil, 200, &sent)
	if sent.Driver != "log" || sent.To != "alice@example.com" {
		t.Fatalf("test before saving = %+v", sent)
	}

	// The SMTP password goes in sealed and never comes back out.
	mailBody := map[string]any{"mail_driver": "smtp", "smtp_host": "smtp.example.com", "smtp_port": 465,
		"smtp_security": "tls", "smtp_user": "ledger@example.com", "smtp_password": "hunter2-hunter2",
		"mail_from": "ledger@example.com"}
	res, b = alice.do("PATCH", "/api/admin/mail", mailBody)
	if res.StatusCode != 200 || bytes.Contains(b, []byte("hunter2")) || !bytes.Contains(b, []byte(`"smtp_password_set":true`)) {
		t.Fatalf("mail settings: %d %s", res.StatusCode, b)
	}
	// The test uses what was saved.
	alice.json("POST", "/api/admin/mail/test", nil, 200, &sent)
	if sent.Driver != "smtp" {
		t.Fatalf("test after saving = %+v", sent)
	}
	var sealed []byte
	if err := f.store.Pool.QueryRow(context.Background(), "SELECT smtp_pass_enc FROM system_settings").Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if len(sealed) == 0 || bytes.Contains(sealed, []byte("hunter2")) {
		t.Fatalf("stored password = %q", sealed)
	}
	var logged int
	if err := f.store.Pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_log WHERE table_name = 'system_settings' AND (old_values ? 'smtp_pass_enc' OR new_values ? 'smtp_pass_enc')").Scan(&logged); err != nil {
		t.Fatal(err)
	}
	if logged != 0 {
		t.Fatal("the sealed password reached the audit log")
	}
	// An untouched form keeps it; clear removes it.
	delete(mailBody, "smtp_password")
	alice.json("PATCH", "/api/admin/mail", mailBody, 200, nil)
	if s := adminSettings(t, alice); s["smtp_password_set"] != true {
		t.Fatalf("password dropped by an untouched form: %v", s)
	}
	alice.json("PATCH", "/api/admin/mail", with(mailBody, "clear_smtp_password", true), 200, nil)
	if s := adminSettings(t, alice); s["smtp_password_set"] != false {
		t.Fatalf("password not cleared: %v", s)
	}

	// Validation reports the admin form's own field names.
	s := settingsPatch(t, alice)
	s["default_display_currency"] = "XXX"
	res, b = alice.do("PATCH", "/api/admin/settings", s)
	if res.StatusCode != 422 || !strings.Contains(string(b), `"default_display_currency":"unknown"`) {
		t.Fatalf("bad default: %d %s", res.StatusCode, b)
	}
	s["default_display_currency"] = "PYG"
	s["mfa_required"] = true
	s["mfa_methods"] = []string{}
	res, b = alice.do("PATCH", "/api/admin/settings", s)
	if res.StatusCode != 422 || !strings.Contains(string(b), `"mfa_methods":"required"`) {
		t.Fatalf("no methods while required: %d %s", res.StatusCode, b)
	}
}

func TestBootstrapAdmin(t *testing.T) {
	f := newAPI(t)
	ctx := context.Background()
	// alice is an admin already: the environment is ignored.
	created, err := f.ids.EnsureAdmin(ctx, identity.BootstrapAdmin{Username: "root", Password: "initial password"})
	if err != nil || created {
		t.Fatalf("with an admin present: created=%v err=%v", created, err)
	}
	if _, err := f.store.Pool.Exec(ctx, "UPDATE users SET is_admin = false"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ids.EnsureAdmin(ctx, identity.BootstrapAdmin{Username: "bob", Password: "initial password"}); err == nil {
		t.Fatal("an existing non-admin was taken over")
	}
	created, err = f.ids.EnsureAdmin(ctx, identity.BootstrapAdmin{Username: "root", Password: "initial password"})
	if err != nil || !created {
		t.Fatalf("bootstrap: created=%v err=%v", created, err)
	}
	if created, _ := f.ids.EnsureAdmin(ctx, identity.BootstrapAdmin{Username: "root", Password: "initial password"}); created {
		t.Fatal("created twice")
	}

	root, res, _ := f.signIn("root", "initial password")
	if res.StatusCode != 200 {
		t.Fatalf("bootstrap login = %d", res.StatusCode)
	}
	var me UserDTO
	root.json("GET", "/api/me", nil, 200, &me)
	if !me.PasswordMustChange || !me.IsAdmin || me.Email != "" {
		t.Fatalf("root = %+v", me)
	}
	if res, _ := root.do("GET", "/api/admin/settings", nil); res.StatusCode != 403 {
		t.Fatalf("admin API before the wizard = %d", res.StatusCode)
	}
	root.json("POST", "/api/me/email", map[string]string{"email": "root@example.com"}, 204, nil)
	root.json("POST", "/api/me/email/confirm", map[string]string{"code": mailCode(t, f, "root@example.com")}, 200, nil)
	res, b := root.do("POST", "/api/me/onboarding", with(profile, "username", "root", "new_password", "initial password"))
	if res.StatusCode != 422 || !strings.Contains(string(b), `"new_password":"same"`) {
		t.Fatalf("same password: %d %s", res.StatusCode, b)
	}
	root.json("POST", "/api/me/onboarding", with(profile, "username", "root", "new_password", "a brand new one"), 200, &me)
	if !me.Initialized || me.PasswordMustChange {
		t.Fatalf("after wizard: %+v", me)
	}
	root.json("GET", "/api/admin/settings", nil, 200, nil)
	if _, res, _ := f.signIn("root", "initial password"); res.StatusCode != 401 {
		t.Fatalf("initial password still works: %d", res.StatusCode)
	}
}
