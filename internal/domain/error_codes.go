package domain

// ErrorCode 表示錯誤碼。
type ErrorCode int

const (
	// 公開錯誤碼前綴分配：
	// 1xxxx 表示通用平台、請求與認證錯誤。
	// 2xxxx 表示 IAM 與授權錯誤。
	// 3xxxx 表示 people-domain 與 HR 錯誤。
	// 4xxxx 表示考勤錯誤。
	// 5xxxx 表示流程錯誤。
	// 6xxxx 表示 agent 錯誤。

	// ErrorCodeInternal 說明此處的錯誤處理語義。
	ErrorCodeInternal ErrorCode = 10000

	// ErrorCodeBadRequest 說明此處的錯誤處理語義。
	ErrorCodeBadRequest ErrorCode = 10001
	// ErrorCodeInvalidJSONBody 說明此處的錯誤處理語義。
	ErrorCodeInvalidJSONBody ErrorCode = 10002
	// ErrorCodeMultipleJSONValues 說明此處的錯誤處理語義。
	ErrorCodeMultipleJSONValues ErrorCode = 10003
	// ErrorCodeInvalidQueryInteger 說明此處的錯誤處理語義。
	ErrorCodeInvalidQueryInteger ErrorCode = 10004
	// ErrorCodeQueryBelowMinimum 說明此處的錯誤處理語義。
	ErrorCodeQueryBelowMinimum ErrorCode = 10005
	// ErrorCodeQueryAboveMaximum 說明此處的錯誤處理語義。
	ErrorCodeQueryAboveMaximum ErrorCode = 10006
	// ErrorCodeInvalidMultipartForm 說明此處的錯誤處理語義。
	ErrorCodeInvalidMultipartForm ErrorCode = 10007
	// ErrorCodeRequiredMultipartFile 說明此處的錯誤處理語義。
	ErrorCodeRequiredMultipartFile ErrorCode = 10008
	// ErrorCodeMultipartFileReadFailed 說明此處的錯誤處理語義。
	ErrorCodeMultipartFileReadFailed ErrorCode = 10009

	// ErrorCodeValidationFailed 說明此處的錯誤處理語義。
	ErrorCodeValidationFailed ErrorCode = 30010
	// ErrorCodeFieldRequired 說明此處的錯誤處理語義。
	ErrorCodeFieldRequired ErrorCode = 30011
	// ErrorCodeFieldInvalid 說明此處的錯誤處理語義。
	ErrorCodeFieldInvalid ErrorCode = 30012
	// ErrorCodeFieldNotFound 說明此處的錯誤處理語義。
	ErrorCodeFieldNotFound ErrorCode = 30013
	// ErrorCodeFieldUnique 說明此處的錯誤處理語義。
	ErrorCodeFieldUnique ErrorCode = 30014
	// ErrorCodeFieldDenied 說明此處的錯誤處理語義。
	ErrorCodeFieldDenied ErrorCode = 30015
	// ErrorCodeDuplicateInImport 說明此處的錯誤處理語義。
	ErrorCodeDuplicateInImport ErrorCode = 30016
	// ErrorCodeDuplicateInFile 說明此處的錯誤處理語義。
	ErrorCodeDuplicateInFile ErrorCode = 30017
	// ErrorCodeImportValidation 說明此處的錯誤處理語義。
	ErrorCodeImportValidation ErrorCode = 30018
	// ErrorCodePositionNotFound 說明此處的錯誤處理語義。
	ErrorCodePositionNotFound ErrorCode = 30030
	// ErrorCodePositionConflict 說明此處的錯誤處理語義。
	ErrorCodePositionConflict ErrorCode = 30031
	// ErrorCodeEmploymentContractNotFound 說明此處的錯誤處理語義。
	ErrorCodeEmploymentContractNotFound ErrorCode = 30040
	// ErrorCodeEmploymentContractInvalidStatus 說明此處的錯誤處理語義。
	ErrorCodeEmploymentContractInvalidStatus ErrorCode = 30041
	// ErrorCodeEmploymentContractInvalidTransition 說明此處的錯誤處理語義。
	ErrorCodeEmploymentContractInvalidTransition ErrorCode = 30042
	// ErrorCodeUnauthorized 說明此處的錯誤處理語義。
	ErrorCodeUnauthorized ErrorCode = 10030
	// ErrorCodeAccountInactive 說明此處的錯誤處理語義。
	ErrorCodeAccountInactive ErrorCode = 10031
	// ErrorCodeSSOEmailNotAuthorized 說明此處的錯誤處理語義。
	ErrorCodeSSOEmailNotAuthorized ErrorCode = 10032
	// ErrorCodeSSOEmailUnverified 說明此處的錯誤處理語義。
	ErrorCodeSSOEmailUnverified ErrorCode = 10033
	// ErrorCodeCompanyInactive 說明此處的錯誤處理語義。
	ErrorCodeCompanyInactive ErrorCode = 10034
	// ErrorCodeGoogleLoginFailed 說明此處的錯誤處理語義。
	ErrorCodeGoogleLoginFailed ErrorCode = 10035
	// ErrorCodeSSOIdentityConflict 說明此處的錯誤處理語義。
	ErrorCodeSSOIdentityConflict ErrorCode = 10036
	// ErrorCodeSSOEmailAmbiguous 說明此處的錯誤處理語義。
	ErrorCodeSSOEmailAmbiguous ErrorCode = 10037
	// ErrorCodeForbidden 說明此處的錯誤處理語義。
	ErrorCodeForbidden ErrorCode = 20040
	// ErrorCodePermissionMissing 說明此處的錯誤處理語義。
	ErrorCodePermissionMissing ErrorCode = 20041
	// ErrorCodeMenuDenied 說明此處的錯誤處理語義。
	ErrorCodeMenuDenied ErrorCode = 20042
	// ErrorCodeButtonDenied 說明此處的錯誤處理語義。
	ErrorCodeButtonDenied ErrorCode = 20043
	// ErrorCodeDataScopeDenied 說明此處的錯誤處理語義。
	ErrorCodeDataScopeDenied ErrorCode = 20044
	// ErrorCodeApprovalRequired 說明此處的錯誤處理語義。
	ErrorCodeApprovalRequired ErrorCode = 20045
	// ErrorCodeCrossTenantDenied 說明此處的錯誤處理語義。
	ErrorCodeCrossTenantDenied ErrorCode = 20046
	// ErrorCodePermissionPackageInvalid 說明此處的錯誤處理語義。
	ErrorCodePermissionPackageInvalid ErrorCode = 20047
	// ErrorCodePermissionPackageVersionConflict 說明此處的錯誤處理語義。
	ErrorCodePermissionPackageVersionConflict ErrorCode = 20048
	// ErrorCodePermissionPackageStateConflict 說明此處的錯誤處理語義。
	ErrorCodePermissionPackageStateConflict ErrorCode = 20049
	// ErrorCodeNotFound 說明此處的錯誤處理語義。
	ErrorCodeNotFound ErrorCode = 10050
	// ErrorCodeConflict 說明此處的錯誤處理語義。
	ErrorCodeConflict ErrorCode = 10060
	// ErrorCodeTooManyRequests 說明此處的錯誤處理語義。
	ErrorCodeTooManyRequests ErrorCode = 10070
)

// appErrorCode 處理 app 錯誤碼。
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
	case "too_many_requests":
		return ErrorCodeTooManyRequests
	default:
		return ErrorCodeInternal
	}
}

// reasonErrorCode 處理 reason 錯誤碼。
func reasonErrorCode(reason string) (ErrorCode, bool) {
	switch reason {
	case "account_inactive":
		return ErrorCodeAccountInactive, true
	case "sso_email_not_authorized":
		return ErrorCodeSSOEmailNotAuthorized, true
	case "sso_email_unverified":
		return ErrorCodeSSOEmailUnverified, true
	case "company_inactive":
		return ErrorCodeCompanyInactive, true
	case "google_login_failed":
		return ErrorCodeGoogleLoginFailed, true
	case "sso_identity_conflict":
		return ErrorCodeSSOIdentityConflict, true
	case "sso_email_ambiguous":
		return ErrorCodeSSOEmailAmbiguous, true
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
	case "cross_tenant_denied":
		return ErrorCodeCrossTenantDenied, true
	default:
		return 0, false
	}
}

// fieldErrorCode 處理欄位錯誤碼。
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

// firstFieldErrorCode 取得第一個欄位錯誤碼。
func firstFieldErrorCode(fields []FieldError, fallback ErrorCode) ErrorCode {
	for _, field := range fields {
		if code, ok := fieldErrorCode(field.Code); ok {
			return code
		}
	}
	return fallback
}

// firstRowErrorCode 取得第一個列錯誤碼。
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
