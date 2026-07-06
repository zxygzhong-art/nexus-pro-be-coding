package service

import (
	"context"
	"sync"
)

// ObjectStore 定義物件儲存層的行為契約。
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

// StoredObject 定義 stored 物件的資料結構。
type StoredObject struct {
	Key         string
	ContentType string
	Data        []byte
}

// MemoryObjectStore 定義 memory 物件儲存層的資料結構。
type MemoryObjectStore struct {
	mu      sync.RWMutex
	objects map[string]StoredObject
}

// NewMemoryObjectStore 建立 memory 物件儲存層。
func NewMemoryObjectStore() *MemoryObjectStore {
	return &MemoryObjectStore{objects: map[string]StoredObject{}}
}

// PutObject 從儲存層處理 put 物件。
func (s *MemoryObjectStore) PutObject(_ context.Context, key string, contentType string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyData := make([]byte, len(data))
	copy(copyData, data)
	s.objects[key] = StoredObject{Key: key, ContentType: contentType, Data: copyData}
	return nil
}

// Provider 從儲存層處理提供者。
func (s *MemoryObjectStore) Provider() string {
	return "memory"
}

// Bucket 從儲存層處理 bucket。
func (s *MemoryObjectStore) Bucket() string {
	return ""
}

// GetObject 從儲存層取得物件。
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

// DeleteObject 從儲存層刪除物件。
func (s *MemoryObjectStore) DeleteObject(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

// firstObjectStore 取得第一個物件儲存層。
func firstObjectStore(store ObjectStore) ObjectStore {
	if store != nil {
		return store
	}
	return NewMemoryObjectStore()
}

// objectStoreProvider 處理物件儲存層提供者。
func objectStoreProvider(store ObjectStore) string {
	if described, ok := store.(objectStoreDescriptor); ok {
		return described.Provider()
	}
	return ""
}

// objectStoreBucket 處理物件儲存層 bucket。
func objectStoreBucket(store ObjectStore) string {
	if described, ok := store.(objectStoreDescriptor); ok {
		return described.Bucket()
	}
	return ""
}
