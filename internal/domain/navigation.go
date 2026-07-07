package domain

// MenuNode 定義 menu node 的資料結構。
type MenuNode struct {
	Key      string     `json:"key"`
	Label    string     `json:"label"`
	Path     string     `json:"path"`
	Icon     string     `json:"icon,omitempty"`
	Children []MenuNode `json:"children,omitempty"`
}
