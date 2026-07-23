package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAgentModelCredentialAADIsStableAndRowScoped(t *testing.T) {
	want := "tenant-1\x00agent-model\x00model-1"
	if got := string(AgentModelCredentialAAD(" tenant-1 ", " model-1 ")); got != want {
		t.Fatalf("AgentModelCredentialAAD() = %q, want %q", got, want)
	}
	if string(AgentModelCredentialAAD("tenant-1", "model-2")) == want {
		t.Fatal("AgentModelCredentialAAD must change when the model row changes")
	}
}

func TestExternalToolCredentialAADIsStableAndRowScoped(t *testing.T) {
	want := "tenant-1\x00external-tool-connection\x00tool-1"
	if got := string(ExternalToolCredentialAAD(" tenant-1 ", " tool-1 ")); got != want {
		t.Fatalf("ExternalToolCredentialAAD() = %q, want %q", got, want)
	}
	if string(ExternalToolCredentialAAD("tenant-1", "tool-2")) == want {
		t.Fatal("ExternalToolCredentialAAD must change when the connection row changes")
	}
}

func TestModelConnectionJSONDoesNotExposeCiphertext(t *testing.T) {
	raw, err := json.Marshal(ModelConnection{ID: "model-1", APIKeyCiphertext: "encrypted-secret"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(raw), "encrypted-secret") || strings.Contains(string(raw), "ciphertext") {
		t.Fatalf("model connection JSON exposed ciphertext: %s", raw)
	}
}

func TestAgentV2TimeoutCompatibilityHelpers(t *testing.T) {
	model := ModelConnection{TimeoutMS: 60_001}
	if got, want := model.TimeoutDuration(), 60_001*time.Millisecond; got != want {
		t.Fatalf("ModelConnection.TimeoutDuration() = %s, want %s", got, want)
	}
	if got, want := model.TimeoutSeconds(), 61; got != want {
		t.Fatalf("ModelConnection.TimeoutSeconds() = %d, want %d", got, want)
	}

	revision := AgentRevision{TimeoutMS: 500}
	if got, want := revision.TimeoutSeconds(), 1; got != want {
		t.Fatalf("AgentRevision.TimeoutSeconds() = %d, want %d", got, want)
	}
}

func TestAgentConfirmationStatusAllowsRetryableResetOnlyFromExecuting(t *testing.T) {
	if !AgentConfirmationStatusExecuting.CanTransitionTo(AgentConfirmationStatusPending) {
		t.Fatal("executing confirmation must be able to return to pending after a retryable failure")
	}
	if AgentConfirmationStatusCompleted.CanTransitionTo(AgentConfirmationStatusPending) {
		t.Fatal("completed confirmation must remain terminal")
	}
}
