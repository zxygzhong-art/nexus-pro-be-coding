package domain_test

import (
	"encoding/json"
	"strings"
	"testing"

	"nexus-pro-api/internal/domain"
)

func TestLeaveTypeCatalogOmitsInternalBalanceRule(t *testing.T) {
	catalog := domain.LeaveTypeCatalog{Items: domain.DefaultLeaveTypes()}
	payload, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(payload)
	for _, forbidden := range []string{"requires_balance", `"unit"`, "paid_ratio"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("leave catalog response exposed removed field %q: %s", forbidden, raw)
		}
	}
}
