package response

import (
	"net/http"
	"strings"
)

type JSONEngine struct {
	appURL         string
	appPort        int
	isLocalhost    bool
	allowedOrigins []string
}

func NewJSONEngine(appURL string, appPort int, isLocalhost bool, allowedOrigins []string) *JSONEngine {
	schema := func() string {
		if isLocalhost {
			return "http"
		}
		return "https"
	}()

	// Add schema to all allowed origins
	schemaOrigins := make([]string, len(allowedOrigins))
	for i, origin := range allowedOrigins {
		schemaOrigins[i] = schema + "://" + origin
	}

	return &JSONEngine{
		appURL:         appURL,
		appPort:        appPort,
		isLocalhost:    isLocalhost,
		allowedOrigins: schemaOrigins,
	}
}

func (je *JSONEngine) addDefaultHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")

	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Access-Control-Allow-Origin", strings.Join(je.allowedOrigins, ", "))
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	if !je.isLocalhost {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload") // HSTS
	}
}

func (je *JSONEngine) RenderResponse(w http.ResponseWriter, r *http.Request, data Jsonable) error {
	je.addDefaultHeaders(w)

	jsonString, err := data.ToJSONString()
	if err != nil {
		return err
	}
	w.Write([]byte(jsonString))

	return nil
}
