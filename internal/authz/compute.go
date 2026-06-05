package authz

import (
	"context"
	"sort"
	"strings"

	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/models"
	"git.corp.ikala.tv/nexus-pro/nexus-pro-be/internal/platform/principal"
)

// evaluate implements §7: effective = (direct ∪ group ∪ assumed) − deny, then
// final = effective ∩ boundary; scope = normal ∩ assumed ∩ boundary.
func (e *LocalEngine) evaluate(ctx context.Context, p principal.Principal, req Request) (Decision, error) {
	subjects, session := e.subjectsFor(ctx, p)

	assignments, err := e.src.Assignments(ctx, subjects)
	if err != nil {
		return Decision{}, err
	}

	// Collect permission sets by effect, and accumulate scopes by origin.
	var allowSets, denySets []string
	normalScope, assumedScope := "", ""
	for _, a := range assignments {
		if a.Effect == "deny" {
			denySets = append(denySets, a.PermissionSetID)
			continue
		}
		allowSets = append(allowSets, a.PermissionSetID)
		switch a.SubjectType {
		case "assumable_role":
			assumedScope = unionScope(assumedScope, a.DataScopeType)
		default: // user, group
			normalScope = unionScope(normalScope, a.DataScopeType)
		}
	}

	setPerms, err := e.src.PermissionsForSets(ctx, dedupe(append(append([]string{}, allowSets...), denySets...)))
	if err != nil {
		return Decision{}, err
	}

	// Effective permission map (allow − deny).
	effective := map[string]models.Permission{}
	for _, sid := range allowSets {
		for _, perm := range setPerms[sid] {
			effective[perm.ID] = perm
		}
	}
	for _, sid := range denySets {
		for _, perm := range setPerms[sid] {
			delete(effective, perm.ID)
		}
	}

	// Boundary intersection (when an assumed-role session carries one).
	var boundaryID *string
	boundaryScope := ""
	if session != nil && session.PermissionBoundaryID != "" {
		if b, err := e.src.Boundary(ctx, session.PermissionBoundaryID); err == nil && b != nil {
			patterns := jsonStrings(b.AllowedPermissions)
			for id := range effective {
				if !matchesAny(id, patterns) {
					delete(effective, id)
				}
			}
			boundaryScope = b.ScopeType
			bid := b.ID
			boundaryID = &bid
		}
	}

	// Match the request against the final permission set.
	var matched []string
	requiresApproval := false
	for id, perm := range effective {
		if permissionMatches(perm, req) {
			matched = append(matched, id)
			if perm.HighRisk || perm.RiskLevel == "high" || perm.RiskLevel == "critical" {
				requiresApproval = true
			}
		}
	}
	sort.Strings(matched)

	// Source labels that granted access (for explainability / audit).
	var sources []string
	for _, a := range assignments {
		if a.Effect == "allow" && len(setPerms[a.PermissionSetID]) > 0 {
			sources = append(sources, a.SourceLabel)
		}
	}
	sources = dedupe(sources)
	sort.Strings(sources)

	dec := Decision{
		MatchedBy:        append(matched, sources...),
		RequiresApproval: requiresApproval,
	}
	if session != nil {
		role := session.AssumableRoleID
		dec.AssumedRole = &role
	}
	dec.PermissionBoundary = boundaryID

	if len(matched) == 0 {
		dec.Allowed = false
		dec.Reason = "missing permission"
		dec.MissingPermissions = []string{syntheticPermissionID(req)}
		dec.MatchedBy = sources // no permission matched; keep source context only
		return dec, nil
	}

	// Final data scope: normal ∩ assumed (if session) ∩ boundary.
	scope := normalScope
	if session != nil {
		scope = intersectScope(scope, assumedScope)
	}
	scope = intersectScope(scope, boundaryScope)

	dec.Allowed = true
	dec.Reason = "matched permission set"
	dec.Scope = scope

	// Field policies for the resource type.
	if fps, err := e.src.FieldPolicies(ctx, req.ApplicationCode, req.ResourceType); err == nil && len(fps) > 0 {
		fp := map[string]string{}
		for _, f := range fps {
			fp[f.Field] = f.Effect
		}
		dec.FieldPolicies = fp
	}
	return dec, nil
}

// permissionMatches reports whether perm satisfies the request, honoring "*"
// wildcards on resource/action and an optional application filter.
func permissionMatches(perm models.Permission, req Request) bool {
	// Exact permission-point match (menu/button gating).
	if req.PermissionID != "" {
		return perm.ID == req.PermissionID
	}
	// An empty perm.ApplicationCode applies to any application.
	if req.ApplicationCode != "" && perm.ApplicationCode != "" && perm.ApplicationCode != "*" && perm.ApplicationCode != req.ApplicationCode {
		return false
	}
	if req.ResourceType != "" && perm.ResourceType != "*" && perm.ResourceType != req.ResourceType {
		return false
	}
	if req.Action != "" && perm.Action != "*" && perm.Action != req.Action {
		return false
	}
	return true
}

func syntheticPermissionID(req Request) string {
	if req.PermissionID != "" {
		return req.PermissionID
	}
	parts := []string{}
	for _, s := range []string{req.ApplicationCode, req.ResourceType, req.Action} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ".")
}

// matchesAny reports whether id matches any pattern. A pattern ending in ".*" or
// equal to "*" is a prefix/any wildcard; otherwise it is an exact match.
func matchesAny(id string, patterns []string) bool {
	for _, p := range patterns {
		if p == "*" || p == id {
			return true
		}
		if strings.HasSuffix(p, ".*") && strings.HasPrefix(id, strings.TrimSuffix(p, "*")) {
			return true
		}
	}
	return false
}

func dedupe(in []string) []string {
	seen := map[string]struct{}{}
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
