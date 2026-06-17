package v1

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"nexus-pro-be/internal/domain"
)

type validatedInput interface {
	Validate() error
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.BadRequest("invalid JSON body: " + err.Error())
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return domain.BadRequest("request body must contain a single JSON value")
	}
	if err := validateInput(target); err != nil {
		return err
	}
	return nil
}

func readOptionalJSON(w http.ResponseWriter, r *http.Request, target any) (bool, error) {
	if r.Body == nil || r.ContentLength == 0 {
		return false, nil
	}
	return true, readJSON(w, r, target)
}

func validateInput(target any) error {
	input, ok := target.(validatedInput)
	if !ok {
		return nil
	}
	return input.Validate()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status >= 200 && status < 400 {
		payload = map[string]any{"data": payload}
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func (a *API) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		traceID := appErr.TraceID
		if traceID == "" {
			traceID, _ = traceContextIDs(r)
		}
		if traceID == "" {
			traceID = requestIDFrom(r)
		}
		body := map[string]any{
			"code":    appErr.Code,
			"message": appErr.Message,
		}
		if appErr.ReasonCode != "" {
			body["reason_code"] = appErr.ReasonCode
		}
		if len(appErr.FieldErrors) > 0 {
			body["field_errors"] = appErr.FieldErrors
		}
		if len(appErr.RowErrors) > 0 {
			body["row_errors"] = rowErrorPayloads(appErr.RowErrors)
		}
		if traceID != "" {
			body["trace_id"] = traceID
		}
		writeJSON(w, appErr.Status, map[string]any{
			"error": body,
		})
		return
	}
	traceID, spanID := traceContextIDs(r)
	requestID := requestIDFrom(r)
	if traceID == "" {
		traceID = requestID
	}
	a.logger.Error("request failed", "path", r.URL.Path, "trace_id", traceID, "span_id", spanID, "request_id", requestID, "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"error": map[string]any{
			"code":     "internal_error",
			"message":  "internal server error",
			"trace_id": traceID,
		},
	})
}

type rowErrorPayload struct {
	RowNumber   int                 `json:"row_number"`
	FieldErrors []domain.FieldError `json:"field_errors"`
}

func rowErrorPayloads(rowErrors []domain.RowError) []rowErrorPayload {
	grouped := make(map[int][]domain.FieldError)
	order := make([]int, 0)
	for _, rowError := range rowErrors {
		if _, ok := grouped[rowError.Row]; !ok {
			order = append(order, rowError.Row)
		}
		grouped[rowError.Row] = append(grouped[rowError.Row], domain.FieldError{
			Field:   rowError.Field,
			Code:    rowError.Code,
			Message: rowError.Message,
		})
	}
	payloads := make([]rowErrorPayload, 0, len(order))
	for _, rowNumber := range order {
		payloads = append(payloads, rowErrorPayload{
			RowNumber:   rowNumber,
			FieldErrors: grouped[rowNumber],
		})
	}
	return payloads
}
