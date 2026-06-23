package v1

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"

	"nexus-pro-be/internal/domain"
)

func stringValue(c *gin.Context, key string) string {
	value, ok := c.Get(key)
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func requestIDFrom(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Request-ID"))
}

func traceContextIDs(r *http.Request) (string, string) {
	spanContext := trace.SpanContextFromContext(r.Context())
	if !spanContext.IsValid() {
		return "", ""
	}
	return spanContext.TraceID().String(), spanContext.SpanID().String()
}

func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "req_" + hex.EncodeToString(raw[:])
	}
	return "req_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func approvalConfirmed(r *http.Request) bool {
	value := strings.TrimSpace(r.Header.Get("X-Approval-Confirmed"))
	return strings.EqualFold(value, "true") || value == "1"
}

func (a *API) approvalConfirmed(r *http.Request) bool {
	return a.allowApprovalHeader && approvalConfirmed(r)
}

func pageRequestFromRequest(r *http.Request) (domain.PageRequest, error) {
	values := r.URL.Query()
	page, err := positiveIntQuery(values.Get("page"), "page", 0)
	if err != nil {
		return domain.PageRequest{}, err
	}
	pageSize, err := positiveIntQuery(values.Get("page_size"), "page_size", domain.MaxPageSize)
	if err != nil {
		return domain.PageRequest{}, err
	}
	return domain.PageRequest{
		Page:     page,
		PageSize: pageSize,
		Sort:     strings.TrimSpace(values.Get("sort")),
	}, nil
}

func pageResponseRequest(page, pageSize int, sort string) domain.PageRequest {
	if page <= 0 {
		page = domain.DefaultPage
	}
	if pageSize <= 0 {
		pageSize = domain.DefaultPageSize
	}
	if pageSize > domain.MaxPageSize {
		pageSize = domain.MaxPageSize
	}
	return domain.PageRequest{Page: page, PageSize: pageSize, Sort: strings.TrimSpace(sort)}
}

func positiveIntQuery(raw, name string, max int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.BadRequestCode(domain.ErrorCodeInvalidQueryInteger, name+" must be an integer")
	}
	if value <= 0 {
		return 0, domain.BadRequestCode(domain.ErrorCodeQueryBelowMinimum, name+" must be greater than zero")
	}
	if max > 0 && value > max {
		return 0, domain.BadRequestCode(domain.ErrorCodeQueryAboveMaximum, name+" must be less than or equal to "+strconv.Itoa(max))
	}
	return value, nil
}
