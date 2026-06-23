package objectstore

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOOptions configures an S3-compatible object store, including MinIO.
type MinIOOptions struct {
	Provider        string
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	UseSSL          bool
	CreateBucket    bool
}

// MinIO stores objects through the S3-compatible MinIO client.
type MinIO struct {
	client   *minio.Client
	provider string
	bucket   string
}

// NewMinIO creates an S3-compatible object store adapter.
func NewMinIO(ctx context.Context, opts MinIOOptions) (*MinIO, error) {
	endpoint, secure, err := normalizeMinIOEndpoint(opts.Endpoint, opts.UseSSL)
	if err != nil {
		return nil, err
	}
	bucket := strings.TrimSpace(opts.Bucket)
	if bucket == "" {
		return nil, errors.New("object store bucket is required")
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(strings.TrimSpace(opts.AccessKeyID), opts.SecretAccessKey, ""),
		Secure: secure,
		Region: strings.TrimSpace(opts.Region),
	})
	if err != nil {
		return nil, err
	}
	store := &MinIO{
		client:   client,
		provider: strings.TrimSpace(opts.Provider),
		bucket:   bucket,
	}
	if store.provider == "" {
		store.provider = "minio"
	}
	if opts.CreateBucket {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return nil, err
		}
		if !exists {
			if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: strings.TrimSpace(opts.Region)}); err != nil {
				return nil, err
			}
		}
	}
	return store, nil
}

// PutObject uploads the object bytes under key.
func (s *MinIO) PutObject(ctx context.Context, key string, contentType string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.client.PutObject(ctx, s.bucket, strings.TrimPrefix(key, "/"), bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

// DeleteObject removes an object from the configured bucket.
func (s *MinIO) DeleteObject(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.client.RemoveObject(ctx, s.bucket, strings.TrimPrefix(key, "/"), minio.RemoveObjectOptions{})
}

// Provider identifies this store in import metadata.
func (s *MinIO) Provider() string {
	return s.provider
}

// Bucket returns the configured object bucket.
func (s *MinIO) Bucket() string {
	return s.bucket
}

func normalizeMinIOEndpoint(endpoint string, fallbackSecure bool) (string, bool, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", false, errors.New("object store endpoint is required")
	}
	if !strings.Contains(endpoint, "://") {
		return endpoint, fallbackSecure, nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", false, err
	}
	if parsed.Host == "" {
		return "", false, errors.New("object store endpoint host is required")
	}
	switch parsed.Scheme {
	case "http":
		return parsed.Host, false, nil
	case "https":
		return parsed.Host, true, nil
	default:
		return "", false, errors.New("object store endpoint must use http or https")
	}
}
