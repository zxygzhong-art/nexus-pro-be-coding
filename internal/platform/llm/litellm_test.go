package llm

import (
	"testing"

	"google.golang.org/genai"
)

// TestFunctionParametersNormalizesGenAISchemaTypes protects OpenAI-compatible task-agent tools.
func TestFunctionParametersNormalizesGenAISchemaTypes(t *testing.T) {
	declaration := &genai.FunctionDeclaration{
		Name: "sub_agent_1",
		Parameters: &genai.Schema{
			Type: "OBJECT",
			Properties: map[string]*genai.Schema{
				"request": {Type: "STRING"},
			},
			Required: []string{"request"},
		},
	}

	parameters, err := functionParameters(declaration)
	if err != nil {
		t.Fatalf("functionParameters() error = %v", err)
	}
	if got := parameters["type"]; got != "object" {
		t.Fatalf("root schema type = %v, want object", got)
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v, want map", parameters["properties"])
	}
	request, ok := properties["request"].(map[string]any)
	if !ok {
		t.Fatalf("request schema = %#v, want map", properties["request"])
	}
	if got := request["type"]; got != "string" {
		t.Fatalf("request schema type = %v, want string", got)
	}
}
