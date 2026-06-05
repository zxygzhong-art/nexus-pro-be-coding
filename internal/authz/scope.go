package authz

// Data-scope rank: narrower scopes rank lower. union() keeps the strongest
// (widest) of a principal's scopes; intersect() keeps the narrowest when an
// assumed role or boundary constrains it (§7/§10).
var scopeRank = map[string]int{
	"":                   0,
	"own":                1,
	"direct_reports":     2,
	"department":         3,
	"department_subtree": 4,
	"assigned_org_units": 5,
	"tenant":             6,
	"system":             7,
}

func rank(s string) int {
	if r, ok := scopeRank[s]; ok {
		return r
	}
	// custom_condition and unknown scopes are treated as narrow (department-ish).
	return 3
}

// unionScope returns the wider of two scopes.
func unionScope(a, b string) string {
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// intersectScope returns the narrower of two scopes. An empty constraint ("")
// means "no constraint", so it does not shrink the scope.
func intersectScope(scope, constraint string) string {
	if constraint == "" {
		return scope
	}
	if scope == "" {
		return constraint
	}
	if rank(constraint) < rank(scope) {
		return constraint
	}
	return scope
}
