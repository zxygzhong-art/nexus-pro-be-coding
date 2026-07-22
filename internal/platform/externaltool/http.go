// Package externaltool executes explicitly configured HTTP tool capabilities.
package externaltool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nexus-pro-api/internal/platform/mcpclient"
)

var (
	// ErrUnsupportedMethod indicates that a capability requested a method the
	// executor never permits.
	ErrUnsupportedMethod = errors.New("unsupported external tool HTTP method")
	// ErrInvalidPath indicates that a capability path is unsafe or cross-origin.
	ErrInvalidPath = errors.New("invalid external tool HTTP path")
	// ErrUnsafeTextResponse indicates that a non-JSON response is not safe UTF-8 text.
	ErrUnsafeTextResponse = errors.New("external tool response is not safe text")
	// ErrResponseTooLarge aliases the shared bounded HTTP response sentinel.
	ErrResponseTooLarge = mcpclient.ErrResponseTooLarge
)

// Request describes one preconfigured HTTP capability invocation.
type Request struct {
	Endpoint          string
	Method            string
	Path              string
	Headers           http.Header
	Arguments         map[string]any
	Timeout           time.Duration
	MaxResponseBytes  int64
	EndpointValidator mcpclient.EndpointValidator
}

// Result is the normalized successful HTTP capability response. JSON is set
// for application/json and +json responses; Text is set for other safe text.
type Result struct {
	StatusCode  int
	ContentType string
	JSON        any
	Text        string
}

// ProbeRequest describes a side-effect-free HTTP connection check.
type ProbeRequest struct {
	Endpoint          string
	Headers           http.Header
	Timeout           time.Duration
	MaxResponseBytes  int64
	EndpointValidator mcpclient.EndpointValidator
}

// ProbeResult reports the upstream status observed by a HEAD request.
type ProbeResult struct {
	StatusCode int
}

// StatusError classifies a non-2xx upstream response without exposing its body.
type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("external tool HTTP request returned status %d", e.StatusCode)
}

// Executor is a stateless platform adapter that can be injected behind a
// service-owned interface.
type Executor struct{}

// Call executes one configured HTTP capability.
func (Executor) Call(ctx context.Context, request Request) (Result, error) {
	return Call(ctx, request)
}

// Probe verifies endpoint reachability without invoking a configured business operation.
func (Executor) Probe(ctx context.Context, request ProbeRequest) (ProbeResult, error) {
	return Probe(ctx, request)
}

// Probe sends a bounded HEAD request to the configured endpoint. HTTP status is
// returned to the caller so the service can distinguish auth failures from a
// reachable server that simply does not implement HEAD.
func Probe(ctx context.Context, request ProbeRequest) (ProbeResult, error) {
	if ctx == nil {
		return ProbeResult{}, fmt.Errorf("external tool HTTP probe: nil context")
	}
	client, endpoint, err := mcpclient.NewBoundedHTTPClient(
		ctx,
		request.Endpoint,
		request.Headers,
		request.Timeout,
		request.MaxResponseBytes,
		request.EndpointValidator,
	)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("prepare external tool HTTP probe: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint.String(), nil)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("build external tool HTTP probe: %w", err)
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ProbeResult{}, fmt.Errorf("execute external tool HTTP probe: %w", ctxErr)
		}
		return ProbeResult{}, fmt.Errorf("execute external tool HTTP probe: %w", err)
	}
	defer response.Body.Close()
	return ProbeResult{StatusCode: response.StatusCode}, nil
}

// Call executes one configured HTTP capability through the shared bounded
// outbound HTTP client. It never follows a capability path to another origin.
func Call(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("external tool HTTP call: nil context")
	}
	method, err := normalizeMethod(request.Method)
	if err != nil {
		return Result{}, err
	}
	transportHeaders := request.Headers.Clone()
	if transportHeaders == nil {
		transportHeaders = make(http.Header)
	}
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		// Configured headers are injected by the bounded RoundTripper after the
		// request is built, so pin this value there as well as on the request.
		transportHeaders.Set("Content-Type", "application/json")
	}

	client, endpoint, err := mcpclient.NewBoundedHTTPClient(
		ctx,
		request.Endpoint,
		transportHeaders,
		request.Timeout,
		request.MaxResponseBytes,
		request.EndpointValidator,
	)
	if err != nil {
		return Result{}, fmt.Errorf("prepare external tool HTTP client: %w", err)
	}
	target, err := resolvePath(endpoint, request.Path)
	if err != nil {
		return Result{}, err
	}

	body, err := encodeArguments(method, target, request.Arguments)
	if err != nil {
		return Result{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return Result{}, fmt.Errorf("build external tool HTTP request: %w", err)
	}
	if body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	httpRequest.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.1")

	response, err := client.Do(httpRequest)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, fmt.Errorf("execute external tool HTTP request: %w", ctxErr)
		}
		return Result{}, fmt.Errorf("execute external tool HTTP request: %w", err)
	}
	defer response.Body.Close()

	result := Result{
		StatusCode:  response.StatusCode,
		ContentType: normalizedContentType(response.Header.Get("Content-Type")),
	}
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return result, fmt.Errorf("read external tool HTTP response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return result, &StatusError{StatusCode: response.StatusCode}
	}
	if len(responseBody) == 0 {
		return result, nil
	}
	if isJSONContentType(result.ContentType) {
		value, err := decodeJSON(responseBody)
		if err != nil {
			return result, fmt.Errorf("decode external tool HTTP JSON response: %w", err)
		}
		result.JSON = value
		return result, nil
	}
	if !utf8.Valid(responseBody) || bytes.IndexByte(responseBody, 0) >= 0 {
		return result, ErrUnsafeTextResponse
	}
	result.Text = string(responseBody)
	return result, nil
}

func normalizeMethod(method string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case http.MethodGet, http.MethodDelete, http.MethodPost, http.MethodPut, http.MethodPatch:
		return method, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedMethod, method)
	}
}

func resolvePath(endpoint *url.URL, rawPath string) (*url.URL, error) {
	if endpoint == nil {
		return nil, fmt.Errorf("%w: endpoint is nil", ErrInvalidPath)
	}
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		clone := *endpoint
		return &clone, nil
	}
	reference, err := url.Parse(rawPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}
	if reference.User != nil {
		return nil, fmt.Errorf("%w: embedded credentials are not allowed", ErrInvalidPath)
	}
	if reference.Fragment != "" {
		return nil, fmt.Errorf("%w: fragments are not allowed", ErrInvalidPath)
	}

	var target *url.URL
	if reference.IsAbs() || reference.Host != "" {
		target = reference
	} else if reference.Path != "" && !strings.HasPrefix(reference.Path, "/") {
		// Treat an endpoint path as a base directory even when it has no trailing
		// slash: endpoint /v1 plus path users becomes /v1/users.
		base := *endpoint
		if !strings.HasSuffix(base.Path, "/") {
			base.Path += "/"
		}
		target = base.ResolveReference(reference)
	} else {
		target = endpoint.ResolveReference(reference)
	}
	if !sameOrigin(endpoint, target) {
		return nil, fmt.Errorf("%w: path must remain on the configured origin", ErrInvalidPath)
	}
	if target.User != nil || target.Fragment != "" {
		return nil, fmt.Errorf("%w: credentials and fragments are not allowed", ErrInvalidPath)
	}
	return target, nil
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func encodeArguments(method string, target *url.URL, arguments map[string]any) (io.Reader, error) {
	switch method {
	case http.MethodGet, http.MethodDelete:
		query := target.Query()
		for key, value := range arguments {
			if strings.TrimSpace(key) == "" {
				return nil, fmt.Errorf("encode external tool HTTP query: argument name is required")
			}
			values, err := queryValues(value)
			if err != nil {
				return nil, fmt.Errorf("encode external tool HTTP query argument %q: %w", key, err)
			}
			query.Del(key)
			for _, item := range values {
				query.Add(key, item)
			}
		}
		target.RawQuery = query.Encode()
		return nil, nil
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		if arguments == nil {
			arguments = map[string]any{}
		}
		encoded, err := json.Marshal(arguments)
		if err != nil {
			return nil, fmt.Errorf("encode external tool HTTP JSON arguments: %w", err)
		}
		return bytes.NewReader(encoded), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedMethod, method)
	}
}

func queryValues(value any) ([]string, error) {
	if value == nil {
		return []string{""}, nil
	}
	raw := reflect.ValueOf(value)
	if raw.Kind() == reflect.Slice || raw.Kind() == reflect.Array {
		values := make([]string, 0, raw.Len())
		for index := 0; index < raw.Len(); index++ {
			item, err := scalarQueryValue(raw.Index(index).Interface())
			if err != nil {
				return nil, fmt.Errorf("array item %d: %w", index, err)
			}
			values = append(values, item)
		}
		return values, nil
	}
	item, err := scalarQueryValue(value)
	if err != nil {
		return nil, err
	}
	return []string{item}, nil
}

func scalarQueryValue(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "", nil
	case string:
		return typed, nil
	case bool:
		return strconv.FormatBool(typed), nil
	case json.Number:
		if _, err := strconv.ParseFloat(typed.String(), 64); err != nil {
			return "", fmt.Errorf("invalid JSON number %q", typed)
		}
		return typed.String(), nil
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(reflected.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(reflected.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(reflected.Float(), 'g', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(reflected.Float(), 'g', -1, 64), nil
	default:
		return "", fmt.Errorf("value of type %T is not a scalar", value)
	}
}

func normalizedContentType(value string) string {
	mediaType, _, err := mime.ParseMediaType(value)
	if err == nil {
		return strings.ToLower(mediaType)
	}
	return strings.ToLower(strings.TrimSpace(strings.SplitN(value, ";", 2)[0]))
}

func isJSONContentType(contentType string) bool {
	return contentType == "application/json" || strings.HasSuffix(contentType, "+json")
}

func decodeJSON(body []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}
