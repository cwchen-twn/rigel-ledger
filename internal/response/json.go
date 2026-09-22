// Package response writes HTTP responses: JSON for the API and the HTML shell
// that the SolidJS app mounts into.
package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrorBody is the shape of every API error:
//
//	{"error": {"code": "unbalanced", "message": "...", "fields": {"lines": "unbalanced"}}}
//
// code is stable and translated by the frontend as error.<code>; message is
// English for logs and curl; fields maps input paths to per-field codes.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string            `json:"code"`
	Message string            `json:"message,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func setAPIHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func JSON(w http.ResponseWriter, status int, v any) {
	setAPIHeaders(w)
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	JSON(w, status, ErrorBody{Error: ErrorDetail{Code: code, Message: message, Fields: fields}})
}

func NoContent(w http.ResponseWriter) {
	setAPIHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

const maxBodyBytes = 1 << 20

// ErrBadJSON wraps any request-body decoding failure.
var ErrBadJSON = errors.New("bad json")

// Decode reads a JSON body into v, rejecting unknown fields and bodies over
// 1 MiB so a typo in a field name fails loudly instead of being ignored.
func Decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	if dec.More() {
		return fmt.Errorf("%w: trailing data", ErrBadJSON)
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("%w: trailing data", ErrBadJSON)
	}
	return nil
}
