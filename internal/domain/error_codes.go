package domain

// ErrorCode is the numeric public code returned in API error envelopes.
type ErrorCode int

const (
	// Public error code prefix allocation:
	// 1xxxx common platform/request/authentication errors.
	// 2xxxx IAM and authorization errors.
	// 3xxxx people-domain and HR errors.
	// 4xxxx attendance errors.
	// 5xxxx workflow errors.
	// 6xxxx agent errors.

	// ErrorCodeInternal is the fallback for unclassified server-side failures.
	ErrorCodeInternal ErrorCode = 10000

	// ErrorCodeBadRequest is the generic malformed request fallback.
	ErrorCodeBadRequest ErrorCode = 10001
	// ErrorCodeInvalidJSONBody indicates the request body could not be decoded as valid JSON.
	ErrorCodeInvalidJSONBody ErrorCode = 10002
	// ErrorCodeMultipleJSONValues indicates the request body contained more than one JSON value.
	ErrorCodeMultipleJSONValues ErrorCode = 10003
	// ErrorCodeInvalidQueryInteger indicates a query parameter expected an integer value.
	ErrorCodeInvalidQueryInteger ErrorCode = 10004
	// ErrorCodeQueryBelowMinimum indicates a numeric query parameter is below its allowed minimum.
	ErrorCodeQueryBelowMinimum ErrorCode = 10005
	// ErrorCodeQueryAboveMaximum indicates a numeric query parameter is above its allowed maximum.
	ErrorCodeQueryAboveMaximum ErrorCode = 10006
	// ErrorCodeInvalidMultipartForm indicates a multipart form could not be parsed.
	ErrorCodeInvalidMultipartForm ErrorCode = 10007
	// ErrorCodeRequiredMultipartFile indicates a required multipart file field is missing.
	ErrorCodeRequiredMultipartFile ErrorCode = 10008
	// ErrorCodeMultipartFileReadFailed indicates an uploaded multipart file could not be read.
	ErrorCodeMultipartFileReadFailed ErrorCode = 10009

	// ErrorCodeValidationFailed is the generic field validation fallback.
	ErrorCodeValidationFailed ErrorCode = 30010
	// ErrorCodeFieldRequired indicates a required field is missing or blank.
	ErrorCodeFieldRequired ErrorCode = 30011
	// ErrorCodeFieldInvalid indicates a field value is malformed or unsupported.
	ErrorCodeFieldInvalid ErrorCode = 30012
	// ErrorCodeFieldNotFound indicates a referenced field value points to a missing resource.
	ErrorCodeFieldNotFound ErrorCode = 30013
	// ErrorCodeFieldUnique indicates a field value violates a uniqueness rule.
	ErrorCodeFieldUnique ErrorCode = 30014
	// ErrorCodeFieldDenied indicates field-level policy denies access or update.
	ErrorCodeFieldDenied ErrorCode = 30015
	// ErrorCodeDuplicateInImport indicates a value duplicates an existing imported row or entity.
	ErrorCodeDuplicateInImport ErrorCode = 30016
	// ErrorCodeDuplicateInFile indicates duplicate values within the same import file.
	ErrorCodeDuplicateInFile ErrorCode = 30017
	// ErrorCodeImportValidation is the generic import row validation fallback.
	ErrorCodeImportValidation ErrorCode = 30018
	// ErrorCodeUnauthorized indicates authenticated tenant or account context is missing or invalid.
	ErrorCodeUnauthorized ErrorCode = 10030
	// ErrorCodeAccountInactive indicates the authenticated account is disabled or not active.
	ErrorCodeAccountInactive ErrorCode = 10031
	// ErrorCodeForbidden is the generic authorization denial fallback.
	ErrorCodeForbidden ErrorCode = 20040
	// ErrorCodePermissionMissing indicates no matching permission allowed the operation.
	ErrorCodePermissionMissing ErrorCode = 20041
	// ErrorCodeMenuDenied indicates a read/menu-level permission is missing.
	ErrorCodeMenuDenied ErrorCode = 20042
	// ErrorCodeButtonDenied indicates a write/button-level permission is missing.
	ErrorCodeButtonDenied ErrorCode = 20043
	// ErrorCodeDataScopeDenied indicates the target resource is outside the allowed data scope.
	ErrorCodeDataScopeDenied ErrorCode = 20044
	// ErrorCodeApprovalRequired indicates a high-risk action needs approval confirmation.
	ErrorCodeApprovalRequired ErrorCode = 20045
	// ErrorCodeNotFound indicates the requested resource does not exist in the current tenant.
	ErrorCodeNotFound ErrorCode = 10050
	// ErrorCodeConflict indicates the request conflicts with current resource state.
	ErrorCodeConflict ErrorCode = 10060
)

func appErrorCode(kind string) ErrorCode {
	switch kind {
	case "bad_request":
		return ErrorCodeBadRequest
	case "validation_failed":
		return ErrorCodeValidationFailed
	case "import_validation_failed":
		return ErrorCodeImportValidation
	case "unauthorized":
		return ErrorCodeUnauthorized
	case "forbidden":
		return ErrorCodeForbidden
	case "not_found":
		return ErrorCodeNotFound
	case "conflict":
		return ErrorCodeConflict
	default:
		return ErrorCodeInternal
	}
}

func reasonErrorCode(reason string) (ErrorCode, bool) {
	switch reason {
	case "account_inactive":
		return ErrorCodeAccountInactive, true
	case "permission_missing":
		return ErrorCodePermissionMissing, true
	case "menu_denied":
		return ErrorCodeMenuDenied, true
	case "button_denied":
		return ErrorCodeButtonDenied, true
	case "field_denied":
		return ErrorCodeFieldDenied, true
	case "data_scope_denied":
		return ErrorCodeDataScopeDenied, true
	case "approval_required":
		return ErrorCodeApprovalRequired, true
	default:
		return 0, false
	}
}

func fieldErrorCode(kind string) (ErrorCode, bool) {
	switch kind {
	case "required":
		return ErrorCodeFieldRequired, true
	case "invalid":
		return ErrorCodeFieldInvalid, true
	case "not_found":
		return ErrorCodeFieldNotFound, true
	case "unique":
		return ErrorCodeFieldUnique, true
	case "field_denied":
		return ErrorCodeFieldDenied, true
	case "duplicate_in_import":
		return ErrorCodeDuplicateInImport, true
	case "duplicate_in_file":
		return ErrorCodeDuplicateInFile, true
	default:
		return 0, false
	}
}

func firstFieldErrorCode(fields []FieldError, fallback ErrorCode) ErrorCode {
	for _, field := range fields {
		if code, ok := fieldErrorCode(field.Code); ok {
			return code
		}
	}
	return fallback
}

func firstRowErrorCode(rows []RowError, fallback ErrorCode) ErrorCode {
	for _, row := range rows {
		if code, ok := fieldErrorCode(row.Code); ok {
			return code
		}
		if code := appErrorCode(row.Code); code != ErrorCodeInternal {
			return code
		}
	}
	return fallback
}
