package service

import (
	"context"
	"sync"
)

type ObjectStore interface {
	PutObject(ctx context.Context, key string, contentType string, data []byte) error
}

type objectDeleter interface {
	DeleteObject(ctx context.Context, key string) error
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
