package domain

// MenuNode describes one menu entry returned to the current account.
type MenuNode struct {
	Key      string     `json:"key"`
	Label    string     `json:"label"`
	Path     string     `json:"path"`
	Children []MenuNode `json:"children,omitempty"`
}
