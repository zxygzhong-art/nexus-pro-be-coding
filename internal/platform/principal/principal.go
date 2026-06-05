// Package principal defines the resolved caller identity for a request.
package principal

// Principal is the authenticated caller: who they are and the security context
// (tenant, group memberships, optional assumed-role session) used for authz.
type Principal struct {
	TenantID             string
	AccountID            string
	Email                string
	Name                 string
	GroupIDs             []string
	AssumedRoleSessionID string
}
