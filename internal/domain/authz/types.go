package authz

type RiskLevel string

const (
	RiskNormal   RiskLevel = "normal"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type RoutePolicy struct {
	Name            string
	Method          string
	Path            string
	ApplicationCode string
	ResourceType    string
	Action          string
	RiskLevel       RiskLevel
}

type RelationshipCheck struct {
	TenantID string
	Subject  string
	Relation string
	Object   string
}
