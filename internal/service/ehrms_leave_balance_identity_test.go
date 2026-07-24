package service

import "testing"

func TestEHRMSLeaveBalanceStableIDUsesEmployeeTypeAndYear(t *testing.T) {
	base := LeaveBalance{
		LeaveTypeID:      "lt-annual",
		EntitlementYear:  2026,
		RemainingMinutes: 480,
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
		"leave type":       func(v *LeaveBalance) { v.LeaveTypeID = "lt-carry" },
		"entitlement year": func(v *LeaveBalance) { v.EntitlementYear = 2025 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := base
			mutate(&candidate)
			if got := ehrmsLeaveBalanceStableID("tenant-1", "EMP-1", candidate); got == want {
				t.Fatalf("%s must distinguish annual balances", name)
			}
		})
	}
}

func TestEHRMSEmployeeStableIDIsTenantScoped(t *testing.T) {
	tenantA := ehrmsEmployeeStableID("tenant-a", "EMP-1")
	tenantB := ehrmsEmployeeStableID("tenant-b", "EMP-1")
	if tenantA == tenantB {
		t.Fatalf("same upstream employee number must not collide across tenants: %q", tenantA)
	}
	if got := ehrmsEmployeeStableID(" tenant-a ", " emp-1 "); got != tenantA {
		t.Fatalf("stable employee identity must normalize tenant and employee keys: got %q want %q", got, tenantA)
	}
}
