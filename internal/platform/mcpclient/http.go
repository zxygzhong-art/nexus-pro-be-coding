package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// EndpointValidator validates an outbound MCP URL. Implementations may be
// called both with the configured hostname and with resolved IP literals.
type EndpointValidator func(context.Context, *url.URL) error

// NewBoundedHTTPClient builds a reusable outbound HTTP client with the same
// network policy as the MCP adapter. Configured headers are copied onto
// same-origin requests only, responses are size-limited, redirects are
// revalidated, and DNS results are pinned through the checked dial path.
//
// The returned URL is parsed and normalized. Callers should resolve relative
// request paths against it and still attach their operation context to each
// request. A zero timeout or response limit selects package defaults.
func NewBoundedHTTPClient(
	ctx context.Context,
	rawEndpoint string,
	headers http.Header,
	timeout time.Duration,
	maxResponseBytes int64,
	validator EndpointValidator,
) (*http.Client, *url.URL, error) {
	if ctx == nil {
		return nil, nil, fmt.Errorf("bounded HTTP client: nil context")
	}
	rawEndpoint = strings.TrimSpace(rawEndpoint)
	if rawEndpoint == "" {
		return nil, nil, fmt.Errorf("bounded HTTP endpoint is required")
	}
	endpoint, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("parse bounded HTTP endpoint: %w", err)
	}
	if err := validateEndpointSyntax(endpoint); err != nil {
		return nil, nil, err
	}
	if timeout < 0 {
		return nil, nil, fmt.Errorf("bounded HTTP timeout must not be negative")
	}
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if maxResponseBytes < 0 {
		return nil, nil, fmt.Errorf("bounded HTTP max response bytes must not be negative")
	}
	if maxResponseBytes == 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	if validator == nil {
		validator = ValidatePublicEndpoint
	}
	validationCtx, cancel := context.WithTimeout(ctx, timeout)
	err = validator(validationCtx, endpoint)
	cancel()
	if err != nil {
		return nil, nil, fmt.Errorf("bounded HTTP endpoint validation: %w", err)
	}

	cfg := Config{
		Headers:           headers.Clone(),
		Timeout:           timeout,
		MaxResponseBytes:  maxResponseBytes,
		EndpointValidator: validator,
	}
	return newHTTPClient(endpoint, cfg), endpoint, nil
}

// ValidatePublicEndpoint is the default SSRF policy. It rejects credentials,
// non-HTTP schemes, and any destination resolving to loopback, private,
// link-local, multicast, unspecified, or otherwise non-global addresses.
func ValidatePublicEndpoint(ctx context.Context, endpoint *url.URL) error {
	if err := validateEndpointSyntax(endpoint); err != nil {
		return err
	}
	ips, err := resolveEndpointIPs(ctx, endpoint.Hostname())
	if err != nil {
		return fmt.Errorf("%w: resolve %q: %v", ErrUnsafeEndpoint, endpoint.Hostname(), err)
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return fmt.Errorf("%w: address %s is not public", ErrUnsafeEndpoint, ip.String())
		}
	}
	return nil
}

// AllowPrivateEndpoints is an explicit opt-out for trusted intranet endpoints
// and tests. URL syntax and the HTTP(S)-only rule are still enforced.
func AllowPrivateEndpoints(_ context.Context, endpoint *url.URL) error {
	return validateEndpointSyntax(endpoint)
}

func validateEndpointSyntax(endpoint *url.URL) error {
	if endpoint == nil {
		return fmt.Errorf("%w: endpoint is nil", ErrUnsafeEndpoint)
	}
	scheme := strings.ToLower(endpoint.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: scheme %q is not http(s)", ErrUnsafeEndpoint, endpoint.Scheme)
	}
	if endpoint.Host == "" || endpoint.Hostname() == "" {
		return fmt.Errorf("%w: endpoint host is required", ErrUnsafeEndpoint)
	}
	if endpoint.User != nil {
		return fmt.Errorf("%w: URL credentials are not allowed", ErrUnsafeEndpoint)
	}
	if endpoint.Fragment != "" {
		return fmt.Errorf("%w: URL fragments are not allowed", ErrUnsafeEndpoint)
	}
	if strings.Contains(endpoint.Hostname(), "%") {
		return fmt.Errorf("%w: scoped IPv6 addresses are not allowed", ErrUnsafeEndpoint)
	}
	if port := endpoint.Port(); port != "" && !validPort(port) {
		return fmt.Errorf("%w: invalid port %q", ErrUnsafeEndpoint, port)
	}
	return nil
}

func resolveEndpointIPs(ctx context.Context, host string) ([]net.IP, error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return nil, fmt.Errorf("empty host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no addresses")
	}
	ips := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if address.IP != nil {
			ips = append(ips, address.IP)
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no usable addresses")
	}
	return ips, nil
}

func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsGlobalUnicast() &&
		!ip.IsPrivate() &&
		!ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}

func newHTTPClient(endpoint *url.URL, cfg Config) *http.Client {
	dialTimeout := minDuration(cfg.Timeout, 10*time.Second)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Environment proxies can resolve or reach the destination outside the
	// checked dial path, so remote MCP traffic deliberately bypasses them.
	transport.Proxy = nil
	transport.DialContext = (&safeDialer{
		dialer: net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		},
		validator: cfg.EndpointValidator,
		scheme:    endpoint.Scheme,
	}).DialContext
	// Keep the transport guard slightly behind the operation context so callers
	// receive a stable context.DeadlineExceeded classification. The guard still
	// bounds context-free lifecycle requests such as the SDK's graceful DELETE.
	headerTimeoutMargin := cfg.Timeout / 10
	if headerTimeoutMargin < time.Millisecond {
		headerTimeoutMargin = time.Millisecond
	}
	if headerTimeoutMargin > time.Second {
		headerTimeoutMargin = time.Second
	}
	transport.ResponseHeaderTimeout = cfg.Timeout + headerTimeoutMargin
	transport.TLSHandshakeTimeout = dialTimeout

	var roundTripper http.RoundTripper = transport
	roundTripper = &responseLimitRoundTripper{base: roundTripper, maxBytes: cfg.MaxResponseBytes}
	roundTripper = &headerRoundTripper{base: roundTripper, origin: originOf(endpoint), headers: cfg.Headers}
	roundTripper = &validatingRoundTripper{base: roundTripper, validator: cfg.EndpointValidator}

	return &http.Client{
		Transport: roundTripper,
		Timeout:   cfg.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("mcp redirect limit exceeded")
			}
			if err := validateEndpointSyntax(req.URL); err != nil {
				return fmt.Errorf("validate mcp redirect: %w", err)
			}
			if err := cfg.EndpointValidator(req.Context(), req.URL); err != nil {
				return fmt.Errorf("validate mcp redirect: %w", err)
			}
			return nil
		},
	}
}

type safeDialer struct {
	dialer    net.Dialer
	validator EndpointValidator
	scheme    string
}

func (d *safeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse mcp dial address: %w", err)
	}
	ips, err := resolveEndpointIPs(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve mcp dial address %q: %w", host, err)
	}

	// Validate the exact addresses from this resolution and connect directly to
	// one of them. This closes the usual validate-then-resolve DNS rebinding gap.
	for _, ip := range ips {
		resolvedURL := &url.URL{Scheme: d.scheme, Host: net.JoinHostPort(ip.String(), port)}
		if err := d.validator(ctx, resolvedURL); err != nil {
			return nil, fmt.Errorf("validate resolved mcp address: %w", err)
		}
	}

	var dialErrors []error
	for _, ip := range ips {
		conn, err := d.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		dialErrors = append(dialErrors, err)
	}
	return nil, fmt.Errorf("dial mcp endpoint: %w", errors.Join(dialErrors...))
}

type validatingRoundTripper struct {
	base      http.RoundTripper
	validator EndpointValidator
}

func (t *validatingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("mcp request URL is required")
	}
	if err := validateEndpointSyntax(req.URL); err != nil {
		return nil, fmt.Errorf("validate mcp request: %w", err)
	}
	if err := t.validator(req.Context(), req.URL); err != nil {
		return nil, fmt.Errorf("validate mcp request: %w", err)
	}
	return t.base.RoundTrip(req)
}

type headerRoundTripper struct {
	base    http.RoundTripper
	origin  string
	headers http.Header
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(t.headers) == 0 || originOf(req.URL) != t.origin {
		return t.base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	for key, values := range t.headers {
		clone.Header.Del(key)
		for _, value := range values {
			clone.Header.Add(key, value)
		}
	}
	return t.base.RoundTrip(clone)
}

func originOf(endpoint *url.URL) string {
	if endpoint == nil {
		return ""
	}
	scheme := strings.ToLower(endpoint.Scheme)
	host := strings.ToLower(endpoint.Hostname())
	port := endpoint.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

type responseLimitRoundTripper struct {
	base     http.RoundTripper
	maxBytes int64
}

func (t *responseLimitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.ContentLength > t.maxBytes {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: content length %d, limit %d", ErrResponseTooLarge, resp.ContentLength, t.maxBytes)
	}
	resp.Body = &maxBytesReadCloser{body: resp.Body, remaining: t.maxBytes, limit: t.maxBytes}
	return resp, nil
}

type maxBytesReadCloser struct {
	body      io.ReadCloser
	remaining int64
	limit     int64
}

func (r *maxBytesReadCloser) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.body.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("%w: limit %d", ErrResponseTooLarge, r.limit)
		}
		return 0, err
	}

	maxRead := r.remaining + 1
	if int64(len(buffer)) > maxRead {
		buffer = buffer[:maxRead]
	}
	n, err := r.body.Read(buffer)
	if int64(n) > r.remaining {
		allowed := int(r.remaining)
		r.remaining = 0
		if allowed == 0 {
			return 0, fmt.Errorf("%w: limit %d", ErrResponseTooLarge, r.limit)
		}
		return allowed, fmt.Errorf("%w: limit %d", ErrResponseTooLarge, r.limit)
	}
	r.remaining -= int64(n)
	return n, err
}

func (r *maxBytesReadCloser) Close() error { return r.body.Close() }

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func validPort(port string) bool {
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value <= 65535
}
