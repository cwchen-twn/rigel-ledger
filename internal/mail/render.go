package mail

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"strings"
	"text/template"
)

//go:embed templates
var templateFS embed.FS

// Data is what every template may use. Link and Code are shown prominently
// in the HTML part when set.
type Data struct {
	AppName  string
	Name     string // the recipient's display name or username
	Username string
	Email    string
	Link     string
	Code     string
	Expires  string // "7 days", "10 minutes"
	Inviter  string
	Message  string
}

var languages = []string{"en", "zh", "es"}

var parsed = func() map[string]*template.Template {
	m := map[string]*template.Template{}
	for _, lang := range languages {
		entries, err := templateFS.ReadDir("templates/" + lang)
		if err != nil {
			panic(err)
		}
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".tmpl")
			t := template.Must(template.New(name).ParseFS(templateFS, "templates/"+lang+"/"+e.Name()))
			m[lang+"/"+name] = t
		}
	}
	return m
}()

var layout = htmltemplate.Must(htmltemplate.New("layout").Parse(`<!doctype html>
<html><body style="font-family:system-ui,-apple-system,Segoe UI,sans-serif;color:#18181b;background:#f4f4f5;margin:0;padding:24px">
<div style="max-width:520px;margin:0 auto;background:#fff;border:1px solid #e4e4e7;border-radius:12px;padding:24px">
<p style="font-weight:600;margin:0 0 16px">{{.AppName}}</p>
{{range .Paragraphs}}<p style="line-height:1.5;margin:0 0 12px">{{.}}</p>{{end}}
{{if .Code}}<p style="font-size:28px;letter-spacing:6px;font-weight:700;margin:16px 0">{{.Code}}</p>{{end}}
{{if .Link}}<p style="margin:20px 0"><a href="{{.Link}}" style="background:#18181b;color:#fff;text-decoration:none;padding:10px 16px;border-radius:8px;display:inline-block">{{.Action}}</a></p>
<p style="font-size:12px;color:#71717a;word-break:break-all">{{.Link}}</p>{{end}}
</div></body></html>`))

// Render builds the subject, text and HTML of template name in lang (falling
// back to English). The To field is left for the caller.
func Render(lang, name string, d Data) (Message, error) {
	t, ok := parsed[lang+"/"+name]
	if !ok {
		t, ok = parsed["en/"+name]
	}
	if !ok {
		return Message{}, fmt.Errorf("no mail template %q", name)
	}
	part := func(block string) (string, error) {
		var b bytes.Buffer
		if t.Lookup(block) == nil {
			return "", nil
		}
		if err := t.ExecuteTemplate(&b, block, d); err != nil {
			return "", err
		}
		return strings.TrimSpace(b.String()), nil
	}
	subject, err := part("subject")
	if err != nil {
		return Message{}, err
	}
	body, err := part("body")
	if err != nil {
		return Message{}, err
	}
	action, err := part("action")
	if err != nil {
		return Message{}, err
	}

	text := body
	if d.Code != "" {
		text += "\n\n" + d.Code
	}
	if d.Link != "" {
		text += "\n\n" + d.Link
	}

	var paragraphs []string
	for _, p := range strings.Split(body, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			paragraphs = append(paragraphs, p)
		}
	}
	var h bytes.Buffer
	if err := layout.Execute(&h, map[string]any{
		"AppName": d.AppName, "Paragraphs": paragraphs, "Code": d.Code, "Link": d.Link, "Action": action,
	}); err != nil {
		return Message{}, err
	}
	return Message{Subject: subject, Text: text, HTML: h.String()}, nil
}
