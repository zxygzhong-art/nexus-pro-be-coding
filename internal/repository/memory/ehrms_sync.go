package memory

import (
	"context"
)

// WithEHRMSSyncLock 對同租戶與同步類型提供非阻塞互斥。
func (s *Store) WithEHRMSSyncLock(ctx context.Context, tenantID, syncType string, fn func() error) (bool, error) {
	key := tenantID + ":" + syncType
	s.mu.Lock()
	if s.ehrmsSyncLocks[key] {
		s.mu.Unlock()
		return false, nil
	}
	s.ehrmsSyncLocks[key] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.ehrmsSyncLocks, key)
		s.mu.Unlock()
	}()
	if err := ctx.Err(); err != nil {
		return true, err
	}
	return true, fn()
}
