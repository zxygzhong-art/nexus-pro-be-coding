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

// stringValue 處理字串 value。
func stringValue(c *gin.Context, key string) string {
	value, ok := c.Get(key)
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

// requestIDFrom 處理請求 ID from。
func requestIDFrom(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Request-ID"))
}

// traceContextIDs 處理 trace context IDs。
func traceContextIDs(r *http.Request) (string, string) {
	spanContext := trace.SpanContextFromContext(r.Context())
	if !spanContext.IsValid() {
		return "", ""
	}
	return spanContext.TraceID().String(), spanContext.SpanID().String()
}

// newRequestID 建立請求 ID。
func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "req_" + hex.EncodeToString(raw[:])
	}
	return "req_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

// pageRequestFromRequest 處理分頁請求 來源 請求。
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

// pageResponseRequest 處理分頁回應請求。
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

// positiveIntQuery 處理正數整數查詢。
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
