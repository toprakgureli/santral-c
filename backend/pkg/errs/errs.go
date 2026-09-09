// Package errs provides typed application errors with HTTP status codes.
package errs

import (
	"errors"
	"fmt"
	"net/http"
)

// Code is a stable machine-readable error code.
type Code string

// Error codes.
const (
	CodeInvalid      Code = "INVALID"
	CodeUnauthorized Code = "UNAUTHORIZED"
	CodeForbidden    Code = "FORBIDDEN"
	CodeNotFound     Code = "NOT_FOUND"
	CodeConflict     Code = "CONFLICT"
	CodeLocked       Code = "LOCKED"
	CodeTooMany      Code = "TOO_MANY_REQUESTS"
	CodeInternal     Code = "INTERNAL"
)

// Error is an application error carrying a code, message and HTTP status.
type Error struct {
	Code    Code
	Message string
	Status  int
	Err     error
}

// Error returns the message, with the wrapped cause when present.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s", e.Message, e.Err.Error())
	}
	return e.Message
}

// Unwrap returns the wrapped cause.
func (e *Error) Unwrap() error { return e.Err }

// New builds an Error.
func New(c Code, status int, message string, err error) *Error {
	return &Error{Code: c, Message: message, Status: status, Err: err}
}

// Invalid is a 400 error.
func Invalid(message string, err error) *Error {
	return New(CodeInvalid, http.StatusBadRequest, message, err)
}

// Unauthorized is a 401 error.
func Unauthorized(message string) *Error {
	return New(CodeUnauthorized, http.StatusUnauthorized, message, nil)
}

// Forbidden is a 403 error.
func Forbidden(message string) *Error {
	return New(CodeForbidden, http.StatusForbidden, message, nil)
}

// NotFound is a 404 error.
func NotFound(message string) *Error {
	return New(CodeNotFound, http.StatusNotFound, message, nil)
}

// Conflict is a 409 error.
func Conflict(message string, err error) *Error {
	return New(CodeConflict, http.StatusConflict, message, err)
}

// Locked is a 423 error.
func Locked(message string) *Error {
	return New(CodeLocked, http.StatusLocked, message, nil)
}

// TooMany is a 429 error.
func TooMany(message string) *Error {
	return New(CodeTooMany, http.StatusTooManyRequests, message, nil)
}

// Internal is a 500 error with a safe generic message.
func Internal(err error) *Error {
	return New(CodeInternal, http.StatusInternalServerError, "Beklenmeyen bir hata oluştu.", err)
}

// From coerces any error to an Error, defaulting to Internal.
func From(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Internal(err)
}
