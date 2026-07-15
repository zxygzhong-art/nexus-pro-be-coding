package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nexus-pro-be/internal/domain"
	"nexus-pro-be/internal/repository/memory"
	"nexus-pro-be/internal/service"
)

func TestLegacyAgentFailRunSanitizesHistory(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	svc := service.New(store, service.Options{Now: func() time.Time { return now }})
	ctx := service.RequestContext{TenantID: "tenant-1", AccountID: "acct-1", TraceID: "trace-legacy-runtime"}
	run := service.AgentRun{
		ID: "run-legacy", TenantID: ctx.TenantID, AccountID: ctx.AccountID, Mode: "policy_qa",
		Status: string(service.AgentRunStatusRunning), CreatedAt: now, UpdatedAt: now,
	}
	if err := store.UpsertAgentRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	const rawFailure = "provider token=legacy-secret"
	if err := svc.Agent().FailRun(ctx, run, errors.New(rawFailure)); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListAgentRunsByAccount(context.Background(), ctx.TenantID, ctx.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one legacy run, got %+v", runs)
	}
	persisted := runs[0]
	if persisted.Status != string(domain.AgentRunStatusFailed) ||
		!strings.Contains(persisted.Answer, service.AgentRuntimeFailureMessage) ||
		!strings.Contains(persisted.Answer, "reason_code="+service.AgentRuntimeFailureReasonCode) ||
		!strings.Contains(persisted.Answer, "trace_id="+ctx.TraceID) ||
		strings.Contains(persisted.Answer, rawFailure) {
		t.Fatalf("legacy runtime failure leaked into history: %+v", persisted)
	}
}
