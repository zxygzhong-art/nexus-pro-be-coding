package service_test

import (
	"strings"
	"testing"

	agentservice "nexus-pro-api/internal/service/agent"
)

// TestRootAgentInstructionRequiresReadableMarkdown protects the user-facing query-result layout contract.
func TestRootAgentInstructionRequiresReadableMarkdown(t *testing.T) {
	instruction := agentservice.RootAgentInstruction("", 0)
	for _, required := range []string{"GitHub Flavored Markdown", "有序列表", "無序列表", "必須實際換行", "不要展示內部 ID"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("agent instruction is missing %q: %s", required, instruction)
		}
	}
}
