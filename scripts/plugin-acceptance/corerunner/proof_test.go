package corerunner

import (
	"strings"
	"testing"

	"gcode/internal/contracts"
)

func TestCompletedToolsRebuildsAgentResultFromSubagentBlock(t *testing.T) {
	completed := &contracts.ToolCall{ID: "call_read", Name: "Read", Status: "completed", Output: "file text"}
	running := &contracts.Subagent{ID: "call_bg", SessionID: "bg-session", Status: "running", Summary: ""}
	state := contracts.Snapshot{Messages: map[string][]contracts.Message{
		"parent": {{
			ID: "m1", Role: "assistant",
			Blocks: []contracts.Block{
				{ID: "b1", Kind: "tool", Tool: completed},
				{ID: "b2", Kind: "subagent", Subagent: &contracts.Subagent{
					ID: "call_agent", SessionID: "child-1", Status: "completed",
					Summary: "VERDICT: INCORRECT\n1681 = 41^2",
				}},
				{ID: "b3", Kind: "subagent", Subagent: running},
			},
		}},
	}}
	tools := completedTools(state)
	if len(tools) != 2 {
		t.Fatalf("expected Read plus one finished child, got %d tools", len(tools))
	}
	var agent *contracts.ToolCall
	for i := range tools {
		if tools[i].Name == "Agent" {
			agent = &tools[i]
		}
	}
	if agent == nil {
		t.Fatal("finished Agent call was not collected")
	}
	want := "child_session_id: child-1\nstatus: completed\nVERDICT: INCORRECT\n1681 = 41^2"
	if agent.Output != want {
		t.Fatalf("agent output mismatch:\n got %q\nwant %q", agent.Output, want)
	}
	if agent.ID != "call_agent" || agent.Status != "completed" {
		t.Fatalf("unexpected identity %q status %q", agent.ID, agent.Status)
	}
	for _, tool := range tools {
		if strings.Contains(tool.Output, "bg-session") {
			t.Fatal("in-flight child without a delivered result must stay excluded")
		}
	}
}
