package domain

import (
	"errors"
	"fmt"
)

type AppError struct {
	Status      int
	Code        string
	Message     string
	FieldErrors []FieldError
	RowErrors   []RowError
	TraceID     string
}

func (e *AppError) Error() string {

	return e.Message
}

func E(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, Message: message}
}

type FieldError struct {
	Tab     string `json:"tab,omitempty"`
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type RowError struct {
	Row     int    `json:"row"`
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NotFound(entity, id string) *AppError {
	return E(404, "not_found", fmt.Sprintf("%s %s not found", entity, id))
}

func BadRequest(message string) *AppError {
	return E(400, "bad_request", message)
}

func ValidationFailed(message string, fields []FieldError) *AppError {
	return &AppError{Status: 400, Code: "validation_failed", Message: message, FieldErrors: fields}
}

func ImportValidationFailed(message string, rows []RowError) *AppError {
	return &AppError{Status: 400, Code: "import_validation_failed", Message: message, RowErrors: rows}
}

func Forbidden(message string) *AppError {
	return E(403, "forbidden", message)
}

func Unauthorized(message string) *AppError {
	return E(401, "unauthorized", message)
}

func Conflict(message string) *AppError {
	return E(409, "conflict", message)
}

func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
