// Package apperror provides typed application errors mapped to HTTP statuses.
package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

// Error is an application error carrying an HTTP status and a stable code.
type Error struct {
	Status  int
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// New builds an Error.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Wrap attaches an underlying error.
func Wrap(status int, code, message string, err error) *Error {
	return &Error{Status: status, Code: code, Message: message, Err: err}
}

// Common constructors.
func BadRequest(msg string) *Error   { return New(http.StatusBadRequest, "bad_request", msg) }
func Unauthorized(msg string) *Error { return New(http.StatusUnauthorized, "unauthorized", msg) }
func Forbidden(msg string) *Error    { return New(http.StatusForbidden, "forbidden", msg) }
func NotFound(msg string) *Error     { return New(http.StatusNotFound, "not_found", msg) }
func Internal(err error) *Error {
	return Wrap(http.StatusInternalServerError, "internal", "internal server error", err)
}
func NotImplemented(msg string) *Error {
	return New(http.StatusNotImplemented, "not_implemented", msg)
}

// As extracts an *Error from err, if present.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
