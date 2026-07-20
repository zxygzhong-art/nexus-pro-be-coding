package utils

import (
	"sort"

	"nexus-pro-api/internal/domain"
)

// NormalizePageRequest 正規化分頁請求。
func NormalizePageRequest(req domain.PageRequest) domain.PageRequest {
	if req.Page <= 0 {
		req.Page = domain.DefaultPage
	}
	if req.PageSize <= 0 {
		req.PageSize = domain.DefaultPageSize
	}
	if req.PageSize > domain.MaxPageSize {
		req.PageSize = domain.MaxPageSize
	}
	if req.Sort == "" {
		req.Sort = "created_at_desc"
	}
	return req
}

// PageResponse 處理分頁回應。
func PageResponse[T any](items []T, req domain.PageRequest) domain.PageResponse[T] {
	req = NormalizePageRequest(req)
	total := len(items)
	start := (req.Page - 1) * req.PageSize
	if start > total {
		start = total
	}
	end := start + req.PageSize
	if end > total {
		end = total
	}
	pageItems := make([]T, end-start)
	copy(pageItems, items[start:end])
	return domain.PageResponse[T]{
		Items:    pageItems,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
		Sort:     req.Sort,
	}
}

// PageResponseFromStore 處理分頁回應 來源 儲存層。
func PageResponseFromStore[T any](items []T, total int, req domain.PageRequest) domain.PageResponse[T] {
	req = NormalizePageRequest(req)
	if items == nil {
		items = []T{}
	}
	return domain.PageResponse[T]{
		Items:    items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
		Sort:     req.Sort,
	}
}

// SortSlice 排序slice。
func SortSlice[T any](items []T, less func(a, b T) bool) {
	sort.SliceStable(items, func(i, j int) bool {
		return less(items[i], items[j])
	})
}
