package authz

import (
	"encoding/json"

	"gorm.io/datatypes"
)

// jsonStrings decodes a JSONB array of strings (e.g. boundary.allowed_permissions).
// Returns nil on empty/invalid input.
func jsonStrings(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
