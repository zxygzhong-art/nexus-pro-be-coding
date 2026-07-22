package service

import "testing"

func TestEHRMSLeaveBalanceStableIDKeepsOverlappingBucketsDistinct(t *testing.T) {
	base := LeaveBalance{
		LeaveTypeID:          "lt-annual",
		ExternalLeaveCode:    "annual",
		ExternalCategoryCode: "paid",
		EntitlementYear:      2026,
		PeriodStart:          "2026-01-01",
		PeriodEnd:            "2026-12-31",
		CarryExpire:          "2026-03-31",
		RemainingMinutes:     480,
	}
	want := ehrmsLeaveBalanceStableID("tenant-1", "EMP-1", base)
	if got := ehrmsLeaveBalanceStableID(" tenant-1 ", " emp-1 ", base); got != want {
		t.Fatalf("stable identity must normalize tenant and employee keys: got %q want %q", got, want)
	}

	mutatedBalance := base
	mutatedBalance.RemainingMinutes = 60
	if got := ehrmsLeaveBalanceStableID("tenant-1", "EMP-1", mutatedBalance); got != want {
		t.Fatalf("mutable snapshot amounts must not change bucket identity: got %q want %q", got, want)
	}

	mutations := map[string]func(*LeaveBalance){
		"external code":     func(v *LeaveBalance) { v.ExternalLeaveCode = "carry" },
		"external category": func(v *LeaveBalance) { v.ExternalCategoryCode = "carry" },
		"leave type":        func(v *LeaveBalance) { v.LeaveTypeID = "lt-carry" },
		"entitlement year":  func(v *LeaveBalance) { v.EntitlementYear = 2025 },
		"period start":      func(v *LeaveBalance) { v.PeriodStart = "2025-12-01" },
		"period end":        func(v *LeaveBalance) { v.PeriodEnd = "2027-01-31" },
		"carry expiry":      func(v *LeaveBalance) { v.CarryExpire = "2026-06-30" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if got := ehrmsLeaveBalanceStableID("tenant-1", "EMP-1", candidate); got == want {
				t.Fatalf("%s must distinguish overlapping upstream buckets", name)
			}
		})
	}
}
