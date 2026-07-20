// verify-admin-access 驗證初始 admin 帳號是否對路由政策目錄中的每一條受保護路由
// 都通過鑑權 check。
//
// 流程：
//  1. 用記憶體 store 走真實的 service.ProvisionTenant 開通流程，產生初始 admin。
//  2. 對 domain.DefaultRoutePolicies 中的每條路由，按照 API 層 authorize 的
//     方式構造 CheckRequest（Resource/Action/RouteMethod/RoutePath），呼叫
//     svc.Authz().Check。
//  3. 同一租戶內再建一個無權限的對照帳號跑同樣的檢查，證明 check 本身
//     不是恆放行的空驗證。
//
// 用法：
//
//	go run ./tools/verify-admin-access [-json <path>]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"nexus-pro-api/internal/domain"
	"nexus-pro-api/internal/repository/memory"
	"nexus-pro-api/internal/service"
)

// routeResult 記錄單條路由的鑑權結果。
type routeResult struct {
	Name       string `json:"name"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	Permission string `json:"permission"`
	RiskLevel  string `json:"risk_level"`
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason,omitempty"`
	Scope      string `json:"scope,omitempty"`
}

// report 是整體驗證報告。
type report struct {
	GeneratedAt        time.Time     `json:"generated_at"`
	TenantID           string        `json:"tenant_id"`
	AdminAccountID     string        `json:"admin_account_id"`
	AdminPermissionSet string        `json:"admin_permission_set_id"`
	TotalRoutes        int           `json:"total_routes"`
	AdminAllowed       int           `json:"admin_allowed"`
	AdminDenied        int           `json:"admin_denied"`
	ControlAllowed     int           `json:"control_allowed"`
	ControlDenied      int           `json:"control_denied"`
	Results            []routeResult `json:"results"`
}

func main() {
	jsonPath := flag.String("json", "", "optional path to write the JSON report")
	flag.Parse()

	ctx := context.Background()
	store := memory.NewStore()
	svc := service.New(store)

	// 1. 走真實開通流程建立初始 admin。
	provision, err := svc.ProvisionTenant(ctx, service.TenantProvisionInput{
		TenantID:        "verify-admin",
		TenantName:      "Verify Admin Tenant",
		AdminEmail:      "admin@verify.example",
		IdentitySubject: "verify-admin-subject",
	})
	if err != nil {
		fatal("provision tenant: %v", err)
	}

	// 確認權限集第一條確實是 *:*:* 通配。
	adminSet, ok, err := store.GetPermissionSet(ctx, provision.TenantID, provision.AdminPermissionSetID)
	if err != nil || !ok {
		fatal("load admin permission set: ok=%v err=%v", ok, err)
	}
	wildcard := false
	for _, p := range adminSet.Permissions {
		if p.Resource == "*" && string(p.Action) == "*" {
			wildcard = true
			break
		}
	}
	if !wildcard {
		fatal("admin permission set %s has no *:* wildcard grant", adminSet.ID)
	}

	// 2. 對照組：同租戶、無任何權限的普通帳號。
	now := time.Now().UTC()
	if err := store.UpsertAccount(ctx, domain.Account{
		ID:          "acct-verify-control",
		TenantID:    provision.TenantID,
		DisplayName: "Control (no permissions)",
		Status:      string(domain.AccountStatusActive),
		CreatedAt:   now,
	}); err != nil {
		fatal("create control account: %v", err)
	}

	adminCtx := domain.RequestContext{TenantID: provision.TenantID, AccountID: provision.AdminAccountID}
	controlCtx := domain.RequestContext{TenantID: provision.TenantID, AccountID: "acct-verify-control"}

	rep := report{
		GeneratedAt:        now,
		TenantID:           provision.TenantID,
		AdminAccountID:     provision.AdminAccountID,
		AdminPermissionSet: provision.AdminPermissionSetID,
	}

	// 3. 對路由政策目錄逐條 check。
	for _, policy := range domain.DefaultRoutePolicies {
		req := domain.CheckRequest{
			Resource:    policyResource(policy),
			Action:      domain.Action(policy.Action),
			RouteMethod: policy.Method,
			RoutePath:   policy.Path,
		}
		adminRes, err := svc.Authz().Check(adminCtx, req)
		if err != nil {
			fatal("admin check %s %s: %v", policy.Method, policy.Path, err)
		}
		controlRes, err := svc.Authz().Check(controlCtx, req)
		if err != nil {
			fatal("control check %s %s: %v", policy.Method, policy.Path, err)
		}

		item := routeResult{
			Name:       policy.Name,
			Method:     policy.Method,
			Path:       policy.Path,
			Permission: string(policy.ApplicationCode) + "." + string(policy.ResourceType) + "." + policy.Action,
			RiskLevel:  string(policy.RiskLevel),
			Allowed:    adminRes.Allowed,
			Scope:      string(firstScope(adminRes.EffectiveScope, adminRes.Scope)),
		}
		if !adminRes.Allowed {
			item.Reason = adminRes.Reason
		}
		rep.Results = append(rep.Results, item)
		rep.TotalRoutes++
		if adminRes.Allowed {
			rep.AdminAllowed++
		} else {
			rep.AdminDenied++
		}
		if controlRes.Allowed {
			rep.ControlAllowed++
		} else {
			rep.ControlDenied++
		}
	}

	printSummary(rep)

	if *jsonPath != "" {
		payload, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			fatal("marshal report: %v", err)
		}
		if err := os.WriteFile(*jsonPath, payload, 0o644); err != nil {
			fatal("write report: %v", err)
		}
		fmt.Printf("\nJSON report written to %s\n", *jsonPath)
	}

	if rep.AdminDenied > 0 {
		os.Exit(1)
	}
}

// policyResource 還原 API route binder 傳入的 resource 字串。
func policyResource(policy domain.RoutePolicy) string {
	if policy.ApplicationCode == "" {
		return policy.ResourceType
	}
	return string(policy.ApplicationCode) + "." + string(policy.ResourceType)
}

// firstScope 取有效資料範圍。
func firstScope(values ...domain.Scope) domain.Scope {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// printSummary 輸出人類可讀的驗證摘要。
func printSummary(rep report) {
	fmt.Printf("tenant=%s admin_account=%s permission_set=%s\n", rep.TenantID, rep.AdminAccountID, rep.AdminPermissionSet)
	fmt.Printf("route policies checked : %d\n", rep.TotalRoutes)
	fmt.Printf("admin   allowed/denied : %d/%d\n", rep.AdminAllowed, rep.AdminDenied)
	fmt.Printf("control allowed/denied : %d/%d (no-permission control account)\n", rep.ControlAllowed, rep.ControlDenied)

	riskCount := map[string]int{}
	for _, r := range rep.Results {
		risk := r.RiskLevel
		if risk == "" {
			risk = "normal"
		}
		riskCount[risk]++
	}
	levels := make([]string, 0, len(riskCount))
	for level := range riskCount {
		levels = append(levels, level)
	}
	sort.Strings(levels)
	fmt.Printf("risk level coverage    : ")
	for i, level := range levels {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Printf("%s=%d", level, riskCount[level])
	}
	fmt.Println()

	if rep.AdminDenied > 0 {
		fmt.Println("\nDENIED routes for admin:")
		for _, r := range rep.Results {
			if !r.Allowed {
				fmt.Printf("  %-6s %-55s %-45s reason=%s\n", r.Method, r.Path, r.Permission, r.Reason)
			}
		}
		return
	}
	fmt.Println("\nAll routes allowed for admin. Sample scopes:")
	limit := 5
	for _, r := range rep.Results {
		if limit == 0 {
			break
		}
		fmt.Printf("  %-6s %-55s scope=%s\n", r.Method, r.Path, r.Scope)
		limit--
	}
}

// fatal 輸出錯誤並終止。
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "verify-admin-access: "+format+"\n", args...)
	os.Exit(2)
}
