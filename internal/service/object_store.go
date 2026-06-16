package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type ObjectStore interface {
	PutObject(ctx context.Context, key string, contentType string, data []byte) error
}

type StoredObject struct {
	Key         string
	ContentType string
	Data        []byte
}

type MemoryObjectStore struct {
	mu      sync.RWMutex
	objects map[string]StoredObject
}

func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{objects: map[string]StoredObject{}}
}

func (s *MemoryObjectStore) PutObject(_ context.Context, key string, contentType string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyData := make([]byte, len(data))
	copy(copyData, data)
	s.objects[key] = StoredObject{Key: key, ContentType: contentType, Data: copyData}
	return nil
}

func (s *MemoryObjectStore) GetObject(key string) (StoredObject, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	object, ok := s.objects[key]
	if !ok {
		return StoredObject{}, false
	}
	data := make([]byte, len(object.Data))
	copy(data, object.Data)
	object.Data = data
	return object, true
}

type LocalObjectStore struct {
	root string
}

func NewLocalObjectStore(root string) (*LocalObjectStore, error) {
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
	return &LocalObjectStore{root: abs}, nil
}

func (s *LocalObjectStore) PutObject(ctx context.Context, key string, _ string, data []byte) error {
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

func (s *LocalObjectStore) pathForKey(key string) (string, error) {
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

func firstObjectStore(store ObjectStore) ObjectStore {
	if store != nil {
		return store
	}
	return NewMemoryObjectStore()
}
