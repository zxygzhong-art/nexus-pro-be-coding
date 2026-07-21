package domain

// 選項列表採用遊標分頁，page_size 上限沿用 MaxPageSize。
const (
	DefaultOptionPageSize = 20
)

// OptionQuery 定義輕量選項列表的查詢條件。
type OptionQuery struct {
	Keyword  string
	Cursor   string
	PageSize int
}

// OptionItem 定義選擇器使用的輕量選項。
type OptionItem struct {
	ID    string         `json:"id"`
	Label string         `json:"label"`
	Meta  map[string]any `json:"meta,omitempty"`
}

// OptionPage 定義遊標分頁的選項回應。NextCursor 為空字串表示沒有下一頁。
type OptionPage struct {
	Items      []OptionItem `json:"items"`
	NextCursor string       `json:"next_cursor"`
}
