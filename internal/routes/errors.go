package routes

import (
	"errors"
	"net/http"

	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

// fail writes err as a JSON error. Ledger errors carry their own status and
// code; anything else is logged and reported as a bare 500 so internals never
// reach the client.
func (h *handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	var le *ledger.Error
	if errors.As(err, &le) {
		status := http.StatusUnprocessableEntity
		switch le.Kind {
		case ledger.KindNotFound:
			status = http.StatusNotFound
		case ledger.KindForbidden:
			status = http.StatusForbidden
		case ledger.KindConflict:
			status = http.StatusConflict
		}
		response.Error(w, status, le.Code, le.Message, le.Fields)
		return
	}
	if errors.Is(err, response.ErrBadJSON) {
		response.Error(w, http.StatusBadRequest, "bad_json", err.Error(), nil)
		return
	}
	h.logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	response.Error(w, http.StatusInternalServerError, "internal", "", nil)
}

func writeAuthError(w http.ResponseWriter, status int, code string) {
	response.Error(w, status, code, http.StatusText(status), nil)
}

func badParam(w http.ResponseWriter, name, code string) {
	response.Error(w, http.StatusBadRequest, "invalid_input", "bad parameter "+name, map[string]string{name: code})
}
