package objectstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Local 定義本機的資料結構。
type Local struct {
	root string
}

// NewLocal 建立本機。
func NewLocal(root string) (*Local, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("object store root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &Local{root: abs}, nil
}

// PutObject 處理 put 物件。
func (s *Local) PutObject(ctx context.Context, key string, _ string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	copyData := make([]byte, len(data))
	copy(copyData, data)
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.WriteFile(path, copyData, 0o644)
}

// Provider 處理提供者。
func (s *Local) Provider() string {
	return "local"
}

// Bucket 處理 bucket。
func (s *Local) Bucket() string {
	return s.root
}

// DeleteObject 刪除物件。
func (s *Local) DeleteObject(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return ctx.Err()
}

// pathForKey 處理 path for key。
func (s *Local) pathForKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("object key is required")
	}
	cleanKey := filepath.Clean(strings.TrimPrefix(key, "/"))
	if cleanKey == "." || cleanKey == ".." || filepath.IsAbs(cleanKey) || strings.HasPrefix(cleanKey, ".."+string(os.PathSeparator)) {
		return "", errors.New("object key escapes object store root")
	}
	path := filepath.Join(s.root, cleanKey)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if absPath != s.root && !strings.HasPrefix(absPath, s.root+string(os.PathSeparator)) {
		return "", errors.New("object key escapes object store root")
	}
	return absPath, nil
}
