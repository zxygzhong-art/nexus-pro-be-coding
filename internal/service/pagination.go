package service

import "sort"

func normalizePageRequest(req PageRequest) PageRequest {
	if req.Page <= 0 {
		req.Page = DefaultPage
	}
	if req.PageSize <= 0 {
		req.PageSize = DefaultPageSize
	}
	if req.PageSize > MaxPageSize {
		req.PageSize = MaxPageSize
	}
	if req.Sort == "" {
		req.Sort = "created_at_desc"
	}
	return req
}

func pageResponse[T any](items []T, req PageRequest) PageResponse[T] {
	req = normalizePageRequest(req)
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
	return PageResponse[T]{
		Items:    pageItems,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
		Sort:     req.Sort,
	}
}

func pageResponseFromStore[T any](items []T, total int, req PageRequest) PageResponse[T] {
	req = normalizePageRequest(req)
	if items == nil {
		items = []T{}
	}
	return PageResponse[T]{
		Items:    items,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
		Sort:     req.Sort,
	}
}

func sortSlice[T any](items []T, less func(a, b T) bool) {
	sort.SliceStable(items, func(i, j int) bool {
		return less(items[i], items[j])
	})
}
