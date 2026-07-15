package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPGoOptions defines the SFTPGo-backed object store settings.
type SFTPGoOptions struct {
	Provider            string
	Endpoint            string
	Root                string
	Username            string
	Password            string
	HostKey             string
	InsecureSkipHostKey bool
	CreateRoot          bool
}

// SFTPGo stores objects through SFTPGo's SFTP endpoint.
type SFTPGo struct {
	endpoint string
	root     string
	provider string
	config   *ssh.ClientConfig
}

// ObjectStore is the shared object store surface used by bootstrap.
type ObjectStore interface {
	PutObject(ctx context.Context, key string, contentType string, data []byte) error
	GetObject(ctx context.Context, key string) ([]byte, error)
	DeleteObject(ctx context.Context, key string) error
	Provider() string
	Bucket() string
}

// GetObject reads an object from SFTPGo's SFTP endpoint.
func (s *SFTPGo) GetObject(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	objectPath, err := s.PathForKey(key)
	if err != nil {
		return nil, err
	}
	var data []byte
	err = s.withClient(ctx, func(client *sftp.Client) error {
		file, err := client.Open(objectPath)
		if err != nil {
			return err
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		return err
	})
	return data, err
}

// NewSFTPGoStore creates an SFTPGo-backed object store using HTTP or SFTP.
func NewSFTPGoStore(ctx context.Context, opts SFTPGoOptions) (ObjectStore, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			return NewSFTPGoHTTP(ctx, opts)
		case "sftp":
			return NewSFTPGo(ctx, opts)
		default:
			return nil, errors.New("object store endpoint must use http, https, or sftp")
		}
	}
	return NewSFTPGo(ctx, opts)
}

// NewSFTPGo creates an object store backed by SFTPGo.
func NewSFTPGo(ctx context.Context, opts SFTPGoOptions) (*SFTPGo, error) {
	endpoint, err := NormalizeSFTPGoEndpoint(opts.Endpoint)
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
	config, err := newSFTPGoSSHConfig(username, opts.Password, opts.HostKey, opts.InsecureSkipHostKey)
	if err != nil {
		return nil, err
	}
	store := &SFTPGo{
		endpoint: endpoint,
		root:     root,
		provider: strings.TrimSpace(opts.Provider),
		config:   config,
	}
	if store.provider == "" {
		store.provider = "sftpgo"
	}
	if opts.CreateRoot {
		if err := store.withClient(ctx, func(client *sftp.Client) error {
			return mkdirAllSFTPGo(ctx, client, root)
		}); err != nil {
			return nil, err
		}
	}
	return store, nil
}

// PutObject writes an object to SFTPGo.
func (s *SFTPGo) PutObject(ctx context.Context, key string, _ string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	objectPath, err := s.PathForKey(key)
	if err != nil {
		return err
	}
	return s.withClient(ctx, func(client *sftp.Client) error {
		if err := mkdirAllSFTPGo(ctx, client, path.Dir(objectPath)); err != nil {
			return err
		}
		file, err := client.Create(objectPath)
		if err != nil {
			return err
		}
		defer file.Close()
		if _, err := file.ReadFrom(bytes.NewReader(data)); err != nil {
			return err
		}
		return ctx.Err()
	})
}

// DeleteObject removes an object from SFTPGo.
func (s *SFTPGo) DeleteObject(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	objectPath, err := s.PathForKey(key)
	if err != nil {
		return err
	}
	return s.withClient(ctx, func(client *sftp.Client) error {
		if err := client.Remove(objectPath); err != nil && !isSFTPGoStatus(err, 2) {
			return err
		}
		return ctx.Err()
	})
}

// Provider returns the storage provider name.
func (s *SFTPGo) Provider() string {
	return s.provider
}

// Bucket returns the configured SFTPGo root path.
func (s *SFTPGo) Bucket() string {
	return strings.TrimPrefix(s.root, "/")
}

func (s *SFTPGo) withClient(ctx context.Context, fn func(*sftp.Client) error) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", s.endpoint)
	if err != nil {
		return err
	}
	sshConn, channels, requests, err := ssh.NewClientConn(conn, s.endpoint, s.config)
	if err != nil {
		_ = conn.Close()
		return err
	}
	sshClient := ssh.NewClient(sshConn, channels, requests)
	defer sshClient.Close()
	client, err := sftp.NewClient(sshClient)
	if err != nil {
		return err
	}
	defer client.Close()
	return fn(client)
}

// PathForKey resolves an object key under the configured SFTP root without allowing traversal.
func (s *SFTPGo) PathForKey(key string) (string, error) {
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

// NormalizeSFTPGoEndpoint validates an SFTP endpoint and adds the default SFTPGo port when absent.
func NormalizeSFTPGoEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("object store endpoint is required")
	}
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil {
			return "", err
		}
		if parsed.Scheme != "sftp" {
			return "", errors.New("object store endpoint must use sftp")
		}
		endpoint = parsed.Host
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			endpoint = net.JoinHostPort(endpoint, "2022")
			host, port, err = net.SplitHostPort(endpoint)
		}
		if err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return "", errors.New("object store endpoint host and port are required")
	}
	return net.JoinHostPort(host, port), nil
}

func cleanSFTPGoRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("object store root is required")
	}
	relativeRoot := path.Clean(strings.TrimPrefix(root, "/"))
	if relativeRoot == "." || relativeRoot == ".." || strings.HasPrefix(relativeRoot, "../") || path.IsAbs(relativeRoot) {
		return "", errors.New("object store root is invalid")
	}
	return "/" + relativeRoot, nil
}

func newSFTPGoSSHConfig(username, password, hostKey string, insecureSkipHostKey bool) (*ssh.ClientConfig, error) {
	hostKey = strings.TrimSpace(hostKey)
	var callback ssh.HostKeyCallback
	switch {
	case hostKey != "":
		parsed, _, _, _, err := ssh.ParseAuthorizedKey([]byte(hostKey))
		if err != nil {
			return nil, fmt.Errorf("parse OBJECT_STORE_SFTP_HOST_KEY: %w", err)
		}
		callback = ssh.FixedHostKey(parsed)
	case insecureSkipHostKey:
		callback = ssh.InsecureIgnoreHostKey()
	default:
		return nil, errors.New("OBJECT_STORE_SFTP_HOST_KEY is required unless OBJECT_STORE_SFTP_INSECURE_SKIP_HOST_KEY=true")
	}
	return &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: callback,
	}, nil
}

func mkdirAllSFTPGo(ctx context.Context, client *sftp.Client, dir string) error {
	dir = path.Clean(dir)
	if dir == "." || dir == "/" {
		return ctx.Err()
	}
	parts := strings.Split(strings.TrimPrefix(dir, "/"), "/")
	current := ""
	for _, part := range parts {
		if err := ctx.Err(); err != nil {
			return err
		}
		current = path.Join(current, part)
		if err := client.Mkdir("/" + current); err != nil {
			info, statErr := client.Stat("/" + current)
			if statErr != nil {
				return err
			}
			if !info.IsDir() {
				return fmt.Errorf("object store path %q exists and is not a directory", "/"+current)
			}
		}
	}
	return ctx.Err()
}

func isSFTPGoStatus(err error, codes ...uint32) bool {
	var status *sftp.StatusError
	if !errors.As(err, &status) {
		return false
	}
	for _, code := range codes {
		if status.Code == code {
			return true
		}
	}
	return false
}
