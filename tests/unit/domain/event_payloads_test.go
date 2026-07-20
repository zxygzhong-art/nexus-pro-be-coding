package domain_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"nexus-pro-be/internal/domain"
)

// TestOpenFGARelationshipPayloadRoundTrip 驗證 typed payload 經 wire map 來回转換後保持一致。
func TestOpenFGARelationshipPayloadRoundTrip(t *testing.T) {
	want := domain.OpenFGARelationshipPayload{
		Operation:   string(domain.AuthzRelationshipTupleWrite),
		ObjectType:  "hr.employee",
		ObjectID:    "emp-1",
		Relation:    "owner",
		SubjectType: "account",
		SubjectID:   "acct-1",
	}
	wire, err := want.Map()
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.DecodeOpenFGARelationshipPayload(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
}

// TestOpenFGARelationshipPayloadWireKeysMatchProducer 驗證 typed payload 的 wire 鍵與生產端實際寫入的鍵一致。
// 生產端（service.relationshipTuplePayload）寫入 operation/object_type/object_id/relation/subject_type/subject_id。
func TestOpenFGARelationshipPayloadWireKeysMatchProducer(t *testing.T) {
	payload := domain.OpenFGARelationshipPayload{
		Operation:   string(domain.AuthzRelationshipTupleDelete),
		ObjectType:  "hr.employee",
		ObjectID:    "emp-1",
		Relation:    "owner",
		SubjectType: "account",
		SubjectID:   "acct-1",
	}
	wire, err := payload.Map()
	if err != nil {
		t.Fatal(err)
	}
	producer := map[string]any{
		"operation":    "delete",
		"object_type":  "hr.employee",
		"object_id":    "emp-1",
		"relation":     "owner",
		"subject_type": "account",
		"subject_id":   "acct-1",
	}
	if !reflect.DeepEqual(wire, producer) {
		t.Fatalf("wire map = %+v, want producer keys %+v", wire, producer)
	}
	typedJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(typedJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, producer) {
		t.Fatalf("wire JSON %s decodes to %+v, want %+v", typedJSON, decoded, producer)
	}
}

// TestDecodeOpenFGARelationshipPayloadRejectsTypeMismatch 驗證鍵值型別錯誤回傳明確錯誤。
func TestDecodeOpenFGARelationshipPayloadRejectsTypeMismatch(t *testing.T) {
	_, err := domain.DecodeOpenFGARelationshipPayload(map[string]any{
		"object_type":  "hr.employee",
		"object_id":    42,
		"relation":     "owner",
		"subject_type": "account",
		"subject_id":   "acct-1",
	})
	if err == nil || !strings.Contains(err.Error(), "decode openfga relationship payload") {
		t.Fatalf("expected explicit decode error, got %v", err)
	}
}

// TestDecodeOpenFGARelationshipPayloadToleratesNilAndExtraKeys 驗證 nil payload 與未知鍵不報錯,維持向前相容。
func TestDecodeOpenFGARelationshipPayloadToleratesNilAndExtraKeys(t *testing.T) {
	got, err := domain.DecodeOpenFGARelationshipPayload(nil)
	if err != nil {
		t.Fatalf("nil payload should decode to zero value, got %v", err)
	}
	if got != (domain.OpenFGARelationshipPayload{}) {
		t.Fatalf("nil payload should decode to zero value, got %+v", got)
	}
	got, err = domain.DecodeOpenFGARelationshipPayload(map[string]any{
		"object_type":  "hr.employee",
		"object_id":    "emp-1",
		"relation":     "owner",
		"subject_type": "account",
		"subject_id":   "acct-1",
		"future_key":   "ignored",
	})
	if err != nil {
		t.Fatalf("unknown keys must stay forward-compatible, got %v", err)
	}
	if got.ObjectID != "emp-1" {
		t.Fatalf("unexpected decode result: %+v", got)
	}
}

// TestAgentModelSyncPayloadRoundTrip 驗證模型同步 payload 與生產端 wire 格式一致且可來回转換。
func TestAgentModelSyncPayloadRoundTrip(t *testing.T) {
	want := domain.AgentModelSyncPayload{ModelID: "amodel-1"}
	wire, err := want.Map()
	if err != nil {
		t.Fatal(err)
	}
	// 生產端（service.appendAgentModelSyncEvent）寫入 map[string]any{"model_id": modelID}。
	if !reflect.DeepEqual(wire, map[string]any{"model_id": "amodel-1"}) {
		t.Fatalf("wire map = %+v, want producer keys", wire)
	}
	got, err := domain.DecodeAgentModelSyncPayload(wire)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip mismatch: got %+v, want %+v", got, want)
	}
}

// TestDecodeAgentModelSyncPayloadRejectsTypeMismatch 驗證 model_id 型別錯誤回傳明確錯誤。
func TestDecodeAgentModelSyncPayloadRejectsTypeMismatch(t *testing.T) {
	_, err := domain.DecodeAgentModelSyncPayload(map[string]any{"model_id": 123})
	if err == nil || !strings.Contains(err.Error(), "decode agent model sync payload") {
		t.Fatalf("expected explicit decode error, got %v", err)
	}
}
