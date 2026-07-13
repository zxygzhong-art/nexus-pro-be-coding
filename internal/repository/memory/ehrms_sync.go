package memory

import (
	"context"
	"sort"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/utils"
)

// UpsertEHRMSSyncRun 保存同步運行。
func (s *Store) UpsertEHRMSSyncRun(_ context.Context, value domain.EHRMSSyncRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.ehrmsSyncRuns, value.TenantID, value.ID, copyEHRMSSyncRun(value))
	return nil
}

// UpsertEHRMSSyncRunStep 保存同步步驟。
func (s *Store) UpsertEHRMSSyncRunStep(_ context.Context, value domain.EHRMSSyncRunStep) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	putNested(s.ehrmsSyncRunSteps, value.TenantID, value.ID, copyEHRMSSyncRunStep(value))
	return nil
}

// GetEHRMSSyncRun 取得同步運行。
func (s *Store) GetEHRMSSyncRun(_ context.Context, tenantID, id string) (domain.EHRMSSyncRun, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := getNested(s.ehrmsSyncRuns, tenantID, id)
	return copyEHRMSSyncRun(value), ok, nil
}

// ListEHRMSSyncRuns 分頁列出同步運行。
func (s *Store) ListEHRMSSyncRuns(_ context.Context, tenantID string, page domain.PageRequest) ([]domain.EHRMSSyncRun, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]domain.EHRMSSyncRun, 0, len(s.ehrmsSyncRuns[tenantID]))
	for _, value := range s.ehrmsSyncRuns[tenantID] {
		items = append(items, copyEHRMSSyncRun(value))
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
	total := len(items)
	page = utils.NormalizePageRequest(page)
	start := (page.Page - 1) * page.PageSize
	if start >= total {
		return []domain.EHRMSSyncRun{}, total, nil
	}
	end := start + page.PageSize
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

// ListEHRMSSyncRunSteps 列出同步步驟。
func (s *Store) ListEHRMSSyncRunSteps(_ context.Context, tenantID, runID string) ([]domain.EHRMSSyncRunStep, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]domain.EHRMSSyncRunStep, 0)
	for _, value := range s.ehrmsSyncRunSteps[tenantID] {
		if value.RunID == runID {
			items = append(items, copyEHRMSSyncRunStep(value))
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Sequence == items[j].Sequence {
			return items[i].Attempt < items[j].Attempt
		}
		return items[i].Sequence < items[j].Sequence
	})
	return items, nil
}

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

func copyEHRMSSyncRun(value domain.EHRMSSyncRun) domain.EHRMSSyncRun {
	value.Summary = utils.CopyStringMap(value.Summary)
	return value
}

func copyEHRMSSyncRunStep(value domain.EHRMSSyncRunStep) domain.EHRMSSyncRunStep {
	value.Summary = utils.CopyStringMap(value.Summary)
	return value
}
