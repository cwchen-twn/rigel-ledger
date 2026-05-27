package response

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/cwc1222/rigelledger/internal/auth"
	"github.com/go-chi/chi/v5"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var printer = message.NewPrinter(language.English)

var copyrightDeclaration = fmt.Sprintf(`
&copy; 2024 - %d Catopia de Chen Antúnez E.A.S. Paraguay —
<a href="https://github.com/cwc1222/rigelledger/blob/main/LICENSE">MIT Licensed</a>
`, time.Now().Year())

// generateNonce creates a cryptographically secure random nonce
func generateNonce() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}

var templateFuncs = template.FuncMap{
	// Time functions
	"now":            time.Now,
	"nowUnix":        time.Now().Unix,
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

	// Map/Dict functions for template data
	"map": createMap,
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

// createMap creates a map from key-value pairs for template use
func createMap(pairs ...any) map[string]any {
	result := make(map[string]any)

	// Process pairs in groups of 2 (key, value)
	for i := 0; i < len(pairs); i += 2 {
		if i+1 < len(pairs) {
			key, ok := pairs[i].(string)
			if ok {
				result[key] = pairs[i+1]
			}
		}
	}

	return result
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
	isLocalhost   bool
	appVersion    string
	templateFs    fs.FS
	templateFuncs template.FuncMap
}

type ColorScheme string

const (
	Light ColorScheme = "light"
	Dark  ColorScheme = "dark"
)

type TemplateData struct {
	Version              string
	CopyrightDeclaration template.HTML
	AccessTokenLeftTime  float64
	Username             string
	ColorScheme          ColorScheme
	Page                 string
	Data                 any
	Nonce                string
}

func NewTemplateEngine(appVersion string, templateFs fs.FS, isLocalhost bool) *TemplateEngine {
	return &TemplateEngine{
		appVersion:    appVersion,
		templateFs:    templateFs,
		templateFuncs: templateFuncs,
		isLocalhost:   isLocalhost,
	}
}

func (te *TemplateEngine) addDefaultHeaders(w http.ResponseWriter, nonce string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	w.Header().Set("Content-Security-Policy", fmt.Sprintf("default-src 'self'; script-src 'self' 'nonce-%s' 'strict-dynamic' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; base-uri 'none';", nonce))
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")

	// Allow bfcache for HTML pages while preventing stale content
	// no-cache requires revalidation but allows bfcache compatibility
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.Header().Set("X-Frame-Options", "deny")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("X-Powered-By", "RigelLedger")

	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

	w.Header().Set("Accept-CH", "Sec-CH-Prefers-Color-Scheme")
	w.Header().Set("Critical-CH", "Sec-CH-Prefers-Color-Scheme")

	if !te.isLocalhost {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload") // HSTS
	}
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

	username := chi.URLParam(r, "username")
	accessTokenLeftTime, ok := r.Context().Value(auth.AccessTokenLeftTimeKey).(time.Duration)
	if !ok {
		accessTokenLeftTime = 0
	}

	// Generate nonce for this request
	nonce := generateNonce()

	td := TemplateData{
		Version:              te.appVersion,
		CopyrightDeclaration: safeHTML(copyrightDeclaration),
		Username:             username,
		ColorScheme:          preferredColorScheme,
		AccessTokenLeftTime:  accessTokenLeftTime.Seconds(),
		Page:                 templateName,
		Data:                 data,
		Nonce:                nonce,
	}

	err = ts.ExecuteTemplate(buf, templateName, td)
	if err != nil {
		return err
	}

	// maps.Copy(w.Header(), headers)
	te.addDefaultHeaders(w, nonce)

	// w.WriteHeader(status)
	buf.WriteTo(w)

	return nil
}
