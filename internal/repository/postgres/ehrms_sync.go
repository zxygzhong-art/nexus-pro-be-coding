package postgres

import (
	"context"
)

// WithEHRMSSyncLock 使用 session advisory lock 防止同租戶同類型並行同步。
func (s *Store) WithEHRMSSyncLock(ctx context.Context, tenantID, syncType string, fn func() error) (bool, error) {
	if s.pool == nil {
		return true, fn()
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()
	key := tenantID + ":" + syncType
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, key).Scan(&acquired); err != nil || !acquired {
		return acquired, err
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key)
	}()
	return true, fn()
}
