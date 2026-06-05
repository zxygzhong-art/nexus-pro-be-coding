package authz

import (
	"context"
	"sort"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
)

// PruneMenus returns the menu items the principal in ctx may see, sorted by
// sort_order. A menu with no required permission is always visible; otherwise the
// principal must hold that exact permission point (mirrors the prototype's
// MenusFor, but matches by permission id so sibling menus sharing resource/action
// are gated independently).
func (e *LocalEngine) PruneMenus(ctx context.Context, applicationCode string) ([]models.MenuItem, error) {
	menus, err := e.src.Menus(ctx, applicationCode)
	if err != nil {
		return nil, err
	}

	out := make([]models.MenuItem, 0, len(menus))
	for _, m := range menus {
		if m.RequiredPermissionID == "" {
			out = append(out, m)
			continue
		}
		dec, err := e.Check(ctx, Request{PermissionID: m.RequiredPermissionID})
		if err != nil {
			return nil, err
		}
		if dec.Allowed {
			out = append(out, m)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out, nil
}
