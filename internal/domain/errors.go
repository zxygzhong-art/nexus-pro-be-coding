package domain

import (
	"errors"
	"fmt"
)

// AppError 定義 app 錯誤的資料結構。
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

// Error 處理錯誤。
func (e *AppError) Error() string {

	return e.Message
}

// E 處理 e。
func E(status int, code, message string) *AppError {
	return &AppError{Status: status, Code: code, PublicCode: appErrorCode(code), Message: message}
}

// WithPublicCode 附加 public 碼。
func (e *AppError) WithPublicCode(code ErrorCode) *AppError {
	e.PublicCode = code
	return e
}

// WithReasonCode attaches a semantic reason and its public numeric mapping when one exists.
func (e *AppError) WithReasonCode(reasonCode string) *AppError {
	e.ReasonCode = reasonCode
	if code, ok := reasonErrorCode(reasonCode); ok {
		e.PublicCode = code
	}
	return e
}

// NumericCode 處理 numeric 碼。
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

// FieldError 定義欄位錯誤的資料結構。
type FieldError struct {
	Tab     string `json:"tab,omitempty"`
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RowError 定義列錯誤的資料結構。
type RowError struct {
	Row     int    `json:"row_number"`
	Field   string `json:"field,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NotFound 處理 not found。
func NotFound(entity, id string) *AppError {
	return E(404, "not_found", fmt.Sprintf("%s %s not found", entity, id))
}

// BadRequest 處理 bad 請求。
func BadRequest(message string) *AppError {
	return E(400, "bad_request", message)
}

// BadRequestCode 處理 bad 請求碼。
func BadRequestCode(code ErrorCode, message string) *AppError {
	return BadRequest(message).WithPublicCode(code)
}

// ValidationFailed 處理驗證 failed。
func ValidationFailed(message string, fields []FieldError) *AppError {
	return &AppError{
		Status:      400,
		Code:        "validation_failed",
		PublicCode:  firstFieldErrorCode(fields, ErrorCodeValidationFailed),
		ReasonCode:  firstFieldReasonCode(fields),
		Message:     message,
		FieldErrors: fields,
	}
}

// firstFieldReasonCode 取得第一個欄位 reason 碼。
func firstFieldReasonCode(fields []FieldError) string {
	for _, field := range fields {
		if field.Code == "field_denied" {
			return "field_denied"
		}
	}
	return ""
}

// ImportValidationFailed 匯入驗證 failed。
func ImportValidationFailed(message string, rows []RowError) *AppError {
	return &AppError{
		Status:     400,
		Code:       "import_validation_failed",
		PublicCode: firstRowErrorCode(rows, ErrorCodeImportValidation),
		Message:    message,
		RowErrors:  rows,
	}
}

// Forbidden 處理禁止。
func Forbidden(message string) *AppError {
	return E(403, "forbidden", message)
}

// ForbiddenReason 處理禁止 reason。
func ForbiddenReason(reasonCode, message string) *AppError {
	err := Forbidden(message)
	err.ReasonCode = reasonCode
	if code, ok := reasonErrorCode(reasonCode); ok {
		err.PublicCode = code
	}
	return err
}

// Unauthorized 處理未授權。
func Unauthorized(message string) *AppError {
	return E(401, "unauthorized", message)
}

// UnauthorizedReason 處理未授權 reason。
func UnauthorizedReason(reasonCode, message string) *AppError {
	err := Unauthorized(message)
	err.ReasonCode = reasonCode
	if code, ok := reasonErrorCode(reasonCode); ok {
		err.PublicCode = code
	}
	return err
}

// Conflict 處理衝突。
func Conflict(message string) *AppError {
	return E(409, "conflict", message)
}

// TooManyRequests 處理 too many 請求。
func TooManyRequests(message string) *AppError {
	return E(429, "too_many_requests", message)
}

// AsAppError 處理 as app 錯誤。
func AsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
