package service

import (
	"context"
	"sync"
)

// ObjectStore writes binary payloads behind a stable key.
type ObjectStore interface {
	PutObject(ctx context.Context, key string, contentType string, data []byte) error
}

type objectStoreDescriptor interface {
	Provider() string
	Bucket() string
}

type objectDeleter interface {
	DeleteObject(ctx context.Context, key string) error
}

// StoredObject is the in-memory representation of an object-store item.
type StoredObject struct {
	Key         string
	ContentType string
	Data        []byte
}

// MemoryObjectStore is a process-local object store used when no external store is configured.
type MemoryObjectStore struct {
	mu      sync.RWMutex
	objects map[string]StoredObject
}

// NewMemoryObjectStore creates an empty process-local object store.
func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{objects: map[string]StoredObject{}}
}

// PutObject stores a defensive copy of the object bytes.
func (s *MemoryObjectStore) PutObject(_ context.Context, key string, contentType string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyData := make([]byte, len(data))
	copy(copyData, data)
	s.objects[key] = StoredObject{Key: key, ContentType: contentType, Data: copyData}
	return nil
}

// Provider identifies this process-local store in import metadata.
func (s *MemoryObjectStore) Provider() string {
	return "memory"
}

// Bucket returns an empty bucket because memory storage is process-local.
func (s *MemoryObjectStore) Bucket() string {
	return ""
}

// GetObject returns a defensive copy of an object when present.
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

// DeleteObject removes an object by key.
func (s *MemoryObjectStore) DeleteObject(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func firstObjectStore(store ObjectStore) ObjectStore {
	if store != nil {
		return store
	}
	return NewMemoryObjectStore()
}

func objectStoreProvider(store ObjectStore) string {
	if described, ok := store.(objectStoreDescriptor); ok {
		return described.Provider()
	}
	return ""
}

func objectStoreBucket(store ObjectStore) string {
	if described, ok := store.(objectStoreDescriptor); ok {
		return described.Bucket()
	}
	return ""
}
