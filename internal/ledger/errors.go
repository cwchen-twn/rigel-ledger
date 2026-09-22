package ledger

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Error is a failure the caller can act on. Code is stable and machine
// readable (the frontend translates it as error.<code>); Fields maps an
// input path such as "lines[1].amount" to a code for that field.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Fields  map[string]string
}

type Kind int

const (
	KindInvalid Kind = iota
	KindNotFound
	KindForbidden
	KindConflict
)

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

func invalid(code, format string, args ...any) *Error {
	return &Error{Kind: KindInvalid, Code: code, Message: fmt.Sprintf(format, args...)}
}

func fieldError(field, code, format string, args ...any) *Error {
	return &Error{Kind: KindInvalid, Code: "invalid_input", Message: fmt.Sprintf(format, args...),
		Fields: map[string]string{field: code}}
}

func notFound(what string) *Error {
	return &Error{Kind: KindNotFound, Code: "not_found", Message: what + " not found"}
}

func forbidden(code, msg string) *Error {
	return &Error{Kind: KindForbidden, Code: code, Message: msg}
}

func conflict(code, msg string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: msg}
}

// translate turns database errors into *Error. The schema's triggers name
// their failures through CONSTRAINT, and that name becomes the code, so a
// rule enforced only in SQL still reaches the user as something readable.
func translate(err error, what string) error {
	if err == nil {
		return nil
	}
	var le *Error
	if errors.As(err, &le) {
		return le
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(what)
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23514": // check_violation, including the trigger-raised ones
			code := pg.ConstraintName
			if code == "" {
				code = "check_violation"
			}
			return &Error{Kind: KindInvalid, Code: code, Message: pg.Message}
		case "23505":
			return conflict("duplicate", pg.Message)
		case "23503":
			return &Error{Kind: KindInvalid, Code: "invalid_reference", Message: pg.Message}
		}
	}
	return err
}
