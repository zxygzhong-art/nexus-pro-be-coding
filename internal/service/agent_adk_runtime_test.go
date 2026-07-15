package service

import (
	"strings"
	"testing"
)

// TestRootAgentInstructionRequiresReadableMarkdown protects the user-facing query-result layout contract.
func TestRootAgentInstructionRequiresReadableMarkdown(t *testing.T) {
	instruction := rootAgentInstruction("", 0)
	for _, required := range []string{"GitHub Flavored Markdown", "有序列表", "无序列表", "必须实际换行", "不要展示内部 ID"} {
		if !strings.Contains(instruction, required) {
			t.Fatalf("agent instruction is missing %q: %s", required, instruction)
		}
	}
}
