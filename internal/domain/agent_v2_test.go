package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCredentialSecretAADIsStableAndRowScoped(t *testing.T) {
	want := "tenant-1\x00credential-secret\x00secret-1"
	if got := string(CredentialSecretAAD(" tenant-1 ", " secret-1 ")); got != want {
		t.Fatalf("CredentialSecretAAD() = %q, want %q", got, want)
	}
	if string(CredentialSecretAAD("tenant-1", "secret-2")) == want {
		t.Fatal("CredentialSecretAAD must change when the secret row changes")
	}
}

func TestCredentialSecretJSONDoesNotExposeCiphertext(t *testing.T) {
	raw, err := json.Marshal(CredentialSecret{ID: "secret-1", Ciphertext: "encrypted-secret"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(raw), "encrypted-secret") || strings.Contains(string(raw), "ciphertext") {
		t.Fatalf("credential secret JSON exposed ciphertext: %s", raw)
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
