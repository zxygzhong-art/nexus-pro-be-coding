// Package authz implements the local permission engine: it computes effective
// permissions (direct ∪ group ∪ assumed-role − deny) ∩ boundary, intersects data
// scopes, and returns a Decision. The Authorizer interface (adapters/authorizer)
// lets OpenFGA plug in later behind the same contract.
package authz

// Request is a single authorization query. ResourceID is optional (for future
// relationship-based checks). When PermissionID is set, the request matches that
// exact permission point (used for menu/button gating where many points share the
// same resource_type/action); otherwise it matches by resource_type + action.
type Request struct {
	ApplicationCode string
	ResourceType    string
	ResourceID      string
	Action          string
	PermissionID    string
}

// Decision is a strict superset of the frontend AuthzDecision: the extra fields
// are json-omitempty so existing clients keep working while richer clients can
// consume scope/field-policy/boundary detail.
type Decision struct {
	Allowed            bool              `json:"allowed"`
	Reason             string            `json:"reason"`
	MatchedBy          []string          `json:"matched_by,omitempty"`
	MissingPermissions []string          `json:"missing_permissions,omitempty"`
	Scope              string            `json:"scope,omitempty"`
	Conditions         map[string]any    `json:"conditions,omitempty"`
	FieldPolicies      map[string]string `json:"field_policies,omitempty"`
	AssumedRole        *string           `json:"assumed_role,omitempty"`
	PermissionBoundary *string           `json:"permission_boundary,omitempty"`
	RequiresApproval   bool              `json:"requires_approval"`
}
