//go:build !adk

package service

import "fmt"

// NewADKAgentChatRuntime reports that the ADK runtime is unavailable in non-ADK builds.
func NewADKAgentChatRuntime(_ any) (AgentChatRuntime, error) {
	return nil, fmt.Errorf("agent chat ADK runtime is unavailable: build with -tags adk after dependencies are available")
}
