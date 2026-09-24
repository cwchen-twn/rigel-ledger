package mail

import (
	"strings"
	"testing"
)

func TestEveryTemplateRendersInEveryLanguage(t *testing.T) {
	names := []string{"invite", "register", "verify", "email_changed", "access_request", "test", "signin_code", "signin_alert"}
	for _, lang := range languages {
		for _, name := range names {
			if _, ok := parsed[lang+"/"+name]; !ok {
				t.Errorf("missing template %s/%s", lang, name)
				continue
			}
			m, err := Render(lang, name, Data{AppName: "RigelLedger", Name: "Alice", Username: "alice",
				Email: "a@example.com", Link: "https://x/y?t=1", Code: "123456", Expires: "7 days", Message: "<b>hi</b>"})
			if err != nil {
				t.Fatalf("%s/%s: %v", lang, name, err)
			}
			if m.Subject == "" || m.Text == "" || !strings.Contains(m.HTML, "RigelLedger") {
				t.Errorf("%s/%s rendered empty parts: %+v", lang, name, m)
			}
			if strings.Contains(m.HTML, "<b>hi</b>") {
				t.Errorf("%s/%s: user text reached the HTML unescaped", lang, name)
			}
		}
	}
}

func TestRenderFallsBackToEnglish(t *testing.T) {
	m, err := Render("fr", "test", Data{AppName: "R", Name: "A"})
	if err != nil || !strings.Contains(m.Subject, "test email") {
		t.Fatalf("fallback: %q %v", m.Subject, err)
	}
}
