package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// SFTPGoHTTP stores objects through SFTPGo's user REST API.
type SFTPGoHTTP struct {
	baseURL  string
	root     string
	provider string
	username string
	password string
	client   *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewSFTPGoHTTP creates an object store backed by SFTPGo HTTP/REST.
func NewSFTPGoHTTP(ctx context.Context, opts SFTPGoOptions) (*SFTPGoHTTP, error) {
	baseURL, err := normalizeSFTPGoHTTPBaseURL(opts.Endpoint)
	if err != nil {
		return nil, err
	}
	root, err := cleanSFTPGoRoot(opts.Root)
	if err != nil {
		return nil, err
	}
	username := strings.TrimSpace(opts.Username)
	if username == "" {
		return nil, errors.New("object store username is required")
	}
	if opts.Password == "" {
		return nil, errors.New("object store password is required")
	}
	store := &SFTPGoHTTP{
		baseURL:  baseURL,
		root:     root,
		provider: strings.TrimSpace(opts.Provider),
		username: username,
		password: opts.Password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	if store.provider == "" {
		store.provider = "sftpgo"
	}
	if opts.CreateRoot {
		if err := store.ensureDir(ctx, root); err != nil {
			return nil, err
		}
	}
	return store, nil
}

// PutObject writes an object to SFTPGo over HTTP.
func (s *SFTPGoHTTP) PutObject(ctx context.Context, key string, contentType string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	objectPath, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("path", objectPath)
	query.Set("mkdir_parents", "true")
	body := data
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL("/user/files/upload")+"?"+query.Encode(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.ContentLength = int64(len(body))
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	resp, err := s.doAuthorized(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return sftpgoHTTPStatusError("upload object", resp)
	}
	return nil
}

// DeleteObject removes an object from SFTPGo over HTTP.
func (s *SFTPGoHTTP) DeleteObject(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	objectPath, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	query := url.Values{}
	query.Set("path", objectPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.apiURL("/user/files")+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := s.doAuthorized(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return sftpgoHTTPStatusError("delete object", resp)
	}
	return nil
}

// Provider returns the storage provider name.
func (s *SFTPGoHTTP) Provider() string {
	return s.provider
}

// Bucket returns the configured SFTPGo root path.
func (s *SFTPGoHTTP) Bucket() string {
	return strings.TrimPrefix(s.root, "/")
}

func (s *SFTPGoHTTP) ensureDir(ctx context.Context, dir string) error {
	dir = path.Clean(dir)
	if dir == "." || dir == "/" {
		return nil
	}
	query := url.Values{}
	query.Set("path", dir)
	query.Set("mkdir_parents", "true")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiURL("/user/dirs")+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := s.doAuthorized(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusOK {
		return nil
	}
	return sftpgoHTTPStatusError("create directory", resp)
}

func (s *SFTPGoHTTP) pathForKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("object key is required")
	}
	cleanKey := path.Clean(strings.TrimPrefix(key, "/"))
	if cleanKey == "." || cleanKey == ".." || strings.HasPrefix(cleanKey, "../") || path.IsAbs(cleanKey) {
		return "", errors.New("object key escapes object store root")
	}
	return path.Join(s.root, cleanKey), nil
}

func (s *SFTPGoHTTP) apiURL(apiPath string) string {
	return strings.TrimRight(s.baseURL, "/") + "/api/v2" + apiPath
}

func (s *SFTPGoHTTP) doAuthorized(ctx context.Context, req *http.Request) (*http.Response, error) {
	token, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}
	_ = resp.Body.Close()
	s.invalidateToken()
	token, err = s.token(ctx)
	if err != nil {
		return nil, err
	}
	retry := req.Clone(ctx)
	retry.Header.Set("Authorization", "Bearer "+token)
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		retry.Body = body
	} else if req.Body != nil {
		return nil, errors.New("sftpgo http request body is not replayable after unauthorized")
	}
	return s.client.Do(retry)
}

func (s *SFTPGoHTTP) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accessToken != "" && time.Now().Before(s.expiresAt.Add(-30*time.Second)) {
		return s.accessToken, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiURL("/user/token"), nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(s.username, s.password)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", sftpgoHTTPStatusError("get user token", resp)
	}
	var payload struct {
		AccessToken string    `json:"access_token"`
		ExpiresAt   time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode sftpgo token: %w", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", errors.New("sftpgo token response missing access_token")
	}
	s.accessToken = payload.AccessToken
	if payload.ExpiresAt.IsZero() {
		s.expiresAt = time.Now().Add(5 * time.Minute)
	} else {
		s.expiresAt = payload.ExpiresAt
	}
	return s.accessToken, nil
}

func (s *SFTPGoHTTP) invalidateToken() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accessToken = ""
	s.expiresAt = time.Time{}
}

func normalizeSFTPGoHTTPBaseURL(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("object store endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", errors.New("object store endpoint must use http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("object store endpoint host is required")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func sftpgoHTTPStatusError(action string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("sftpgo %s failed: %s", action, message)
}
