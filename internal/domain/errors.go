package domain

import (
	"errors"
	"fmt"
)

// AppError is the stable error shape carried from services to HTTP responses.
type AppError struct {
	Status      int
	Code        string
	PublicCode  ErrorCode
	ReasonCode  string
	Message     string
	FieldErrors []FieldError
	RowErrors   []RowError
	TraceID     string
}

// Error returns the public application error message.
func (e *AppError) Error() string {

	return e.Message
}

// E constructs an application error with the given HTTP status and public code.
func E(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, PublicCode: appErrorCode(code), Message: message}
}

// WithPublicCode overrides the default public numeric code for a specific error.
func (e *AppError) WithPublicCode(code ErrorCode) *AppError {
	e.PublicCode = code
	return e
}

// NumericCode returns the public numeric code for API responses.
func (e *AppError) NumericCode() ErrorCode {
	if e.PublicCode != 0 {
		return e.PublicCode
	}
	if e.ReasonCode != "" {
		if code, ok := reasonErrorCode(e.ReasonCode); ok {
			return code
		}
	}
	return appErrorCode(e.Code)
}

// FieldError describes a validation failure for one request field.
type FieldError struct {
	Tab     string `json:"tab,omitempty"`
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RowError describes a validation failure for one imported spreadsheet row.
type RowError struct {
	Row     int    `json:"row_number"`
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NotFound returns a 404 application error for a missing entity.
func NotFound(entity, id string) *AppError {
	return E(404, "not_found", fmt.Sprintf("%s %s not found", entity, id))
}

// BadRequest returns a 400 application error for malformed input.
func BadRequest(message string) *AppError {
	return E(400, "bad_request", message)
}

// BadRequestCode returns a 400 error with a more specific public numeric code.
func BadRequestCode(code ErrorCode, message string) *AppError {
	return BadRequest(message).WithPublicCode(code)
}

// ValidationFailed returns a 400 error with field-level validation details.
func ValidationFailed(message string, fields []FieldError) *AppError {
	return &AppError{
		Status:      400,
		Code:        "validation_failed",
		PublicCode:  firstFieldErrorCode(fields, ErrorCodeValidationFailed),
		Message:     message,
		FieldErrors: fields,
	}
}

// ImportValidationFailed returns a 400 error with row-level import failures.
func ImportValidationFailed(message string, rows []RowError) *AppError {
	return &AppError{
		Status:     400,
		Code:       "import_validation_failed",
		PublicCode: firstRowErrorCode(rows, ErrorCodeImportValidation),
		Message:    message,
		RowErrors:  rows,
	}
}

// Forbidden returns a 403 application error.
func Forbidden(message string) *AppError {
	return E(403, "forbidden", message)
}

// ForbiddenReason returns a 403 application error with a machine-readable reason.
func ForbiddenReason(reasonCode, message string) *AppError {
	err := Forbidden(message)
	err.ReasonCode = reasonCode
	if code, ok := reasonErrorCode(reasonCode); ok {
		err.PublicCode = code
	}
	return err
}

// Unauthorized returns a 401 application error.
func Unauthorized(message string) *AppError {
	return E(401, "unauthorized", message)
}

// Conflict returns a 409 application error.
func Conflict(message string) *AppError {
	return E(409, "conflict", message)
}

// AsAppError unwraps an error chain into an AppError when possible.
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
