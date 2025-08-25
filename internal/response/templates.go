package response

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var printer = message.NewPrinter(language.English)

var templateFuncs = template.FuncMap{
	// Time functions
	"now":            time.Now,
	"timeSince":      time.Since,
	"timeUntil":      time.Until,
	"formatTime":     formatTime,
	"approxDuration": approxDuration,

	// String functions
	"uppercase": strings.ToUpper,
	"lowercase": strings.ToLower,
	"pluralize": pluralize,
	"slugify":   slugify,
	"safeHTML":  safeHTML,

	// Slice functions
	"join": strings.Join,

	// Number functions
	"incr":        incr,
	"decr":        decr,
	"formatInt":   formatInt,
	"formatFloat": formatFloat,

	// Boolean functions
	"yesno": yesno,

	// URL functions
	"urlSetParam": urlSetParam,
	"urlDelParam": urlDelParam,
}

func formatTime(format string, t time.Time) string {
	return t.Format(format)
}

func approxDuration(d time.Duration) string {
	const (
		day  = 24 * time.Hour
		year = 365 * day
	)

	formatUnit := func(count int, singular, plural string) string {
		if count == 1 {
			return fmt.Sprintf("1 %s", singular)
		}
		return fmt.Sprintf("%d %s", count, plural)
	}

	switch {
	case d >= year:
		return formatUnit(int(math.Round(float64(d)/float64(year))), "year", "years")
	case d >= day:
		return formatUnit(int(math.Round(float64(d)/float64(day))), "day", "days")
	case d >= time.Hour:
		return formatUnit(int(math.Round(d.Hours())), "hour", "hours")
	case d >= time.Minute:
		return formatUnit(int(math.Round(d.Minutes())), "minute", "minutes")
	case d >= time.Second:
		return formatUnit(int(math.Round(d.Seconds())), "second", "seconds")
	default:
		return "less than 1 second"
	}
}

func pluralize(count any, singular string, plural string) (string, error) {
	n, err := toInt64(count)
	if err != nil {
		return "", err
	}

	if n == 1 {
		return singular, nil
	}

	return plural, nil
}

func slugify(s string) string {
	var buf bytes.Buffer

	for _, r := range s {
		switch {
		case r > unicode.MaxASCII:
			continue
		case unicode.IsLetter(r):
			buf.WriteRune(unicode.ToLower(r))
		case unicode.IsDigit(r), r == '_', r == '-':
			buf.WriteRune(r)
		case unicode.IsSpace(r):
			buf.WriteRune('-')
		}
	}

	return buf.String()
}

func safeHTML(s string) template.HTML {
	return template.HTML(s)
}

func incr(i any) (int64, error) {
	n, err := toInt64(i)
	if err != nil {
		return 0, err
	}

	n++
	return n, nil
}

func decr(i any) (int64, error) {
	n, err := toInt64(i)
	if err != nil {
		return 0, err
	}

	n--
	return n, nil
}

func formatInt(i any) (string, error) {
	n, err := toInt64(i)
	if err != nil {
		return "", err
	}

	return printer.Sprintf("%d", n), nil
}

func formatFloat(f float64, dp int) string {
	format := "%." + strconv.Itoa(dp) + "f"
	return printer.Sprintf(format, f)
}

func yesno(b bool) string {
	if b {
		return "Yes"
	}

	return "No"
}

func urlSetParam(u *url.URL, key string, value any) *url.URL {
	nu := *u
	values := nu.Query()

	values.Set(key, fmt.Sprintf("%v", value))

	nu.RawQuery = values.Encode()
	return &nu
}

func urlDelParam(u *url.URL, key string) *url.URL {
	nu := *u
	values := nu.Query()

	values.Del(key)

	nu.RawQuery = values.Encode()
	return &nu
}

func toInt64(i any) (int64, error) {
	switch v := i.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	// Note: uint64 not supported due to risk of truncation.
	case string:
		return strconv.ParseInt(v, 10, 64)
	}

	return 0, fmt.Errorf("unable to convert type %T to int", i)
}

type TemplateEngine struct {
	appVersion    string
	templateFs    embed.FS
	templateFuncs template.FuncMap
}

type ColorScheme string

const (
	Light ColorScheme = "light"
	Dark  ColorScheme = "dark"
)

type TemplateData struct {
	Version     string
	ColorScheme ColorScheme
	Data        any
}

func NewTemplateEngine(appVersion string, templateFs embed.FS) *TemplateEngine {
	return &TemplateEngine{
		appVersion:    appVersion,
		templateFs:    templateFs,
		templateFuncs: templateFuncs,
	}
}

func (te *TemplateEngine) addDefaultHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")

	w.Header().Set("Cache-Control", "no-store")

	w.Header().Set("X-Frame-Options", "deny")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("X-Powered-By", "RigelLedger")

	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

	w.Header().Set("Accept-CH", "Sec-CH-Prefers-Color-Scheme")
	w.Header().Set("Critical-CH", "Sec-CH-Prefers-Color-Scheme")
}

func (te *TemplateEngine) RenderResponse(w http.ResponseWriter, r *http.Request, data any, templateName string) error {
	patterns := []string{"templates/partials/*.tmpl", "templates/" + templateName + ".tmpl"}

	// patterns := []string{"templates/" + templateName}

	ts, err := template.New("").Funcs(te.templateFuncs).ParseFS(te.templateFs, patterns...)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)

	// Extract just the filename from the template path for execution
	// e.g., "pages/home.tmpl" becomes "home.tmpl"
	// templateFileName := templateName
	// if idx := strings.LastIndex(templateName, "/"); idx != -1 {
	// 	templateFileName = templateName[idx+1:]
	// }

	preferredColorScheme := Dark
	if strings.Contains(r.Header.Get("Sec-CH-Prefers-Color-Scheme"), "light") {
		preferredColorScheme = Light
	}

	td := TemplateData{
		Version:     te.appVersion,
		ColorScheme: preferredColorScheme,
		Data:        data,
	}

	err = ts.ExecuteTemplate(buf, templateName, td)
	if err != nil {
		return err
	}

	// maps.Copy(w.Header(), headers)
	te.addDefaultHeaders(w)

	// w.WriteHeader(status)
	buf.WriteTo(w)

	return nil
}
