package contracts

import "testing"

func TestActivitySummaryUsesInvocationIdentityAndUniqueEditedPaths(t *testing.T) {
	one := 1
	blocks := []Block{
		{ID: "thought", Kind: "reasoning", Text: "Inspect the failing check"},
		{Kind: "toolCall", Tool: &ToolCall{ID: "exec", Kind: ToolExec, Name: "Bash", Status: "working"}},
		{Kind: "toolResult", Tool: &ToolCall{ID: "exec", Status: "failed", ExitCode: &one}},
		{Kind: "tool", Tool: &ToolCall{ID: "read1", Kind: ToolReadFile, Path: "a.go"}},
		{Kind: "tool", Tool: &ToolCall{ID: "read2", Kind: ToolReadFile, Path: "a.go"}},
		{Kind: "tool", Tool: &ToolCall{ID: "write", Kind: ToolWriteFile, Path: "a.go"}},
		{Kind: "tool", Tool: &ToolCall{ID: "edit", Kind: ToolEditFile, Path: "a.go"}},
		{Kind: "tool", Tool: &ToolCall{ID: "patch", Kind: ToolApplyPatch, Diff: &Change{Path: "b.go"}}},
		{Kind: "tool", Tool: &ToolCall{ID: "glob", Kind: ToolGlob}},
		{Kind: "subagent", Subagent: &Subagent{ID: "child"}},
	}
	summary := SummarizeActivity(blocks)
	if summary.ToolCalls != 8 || summary.Commands != 1 || summary.Reads != 2 || summary.EditedFiles != 2 || summary.Searches != 1 || summary.Failed != 1 || summary.Other != 1 || summary.Subagents != 1 {
		t.Fatalf("incorrect activity counts: %+v", summary)
	}
	want := "Thought process · Ran 1 command · edited 2 files · read 2 files · searched 1 time · called 1 tool · 1 failed"
	if summary.Label() != want {
		t.Fatalf("label=%q, want %q", summary.Label(), want)
	}
}

func TestTurnActivityExcludesFinalReplyAndOtherTurns(t *testing.T) {
	messages := []Message{{TurnID: "a", Role: "user", Blocks: []Block{{Kind: "text", Text: "request"}}}, {TurnID: "a", Role: "assistant", Blocks: []Block{{ID: "r", Kind: "reasoning", Text: "thought"}, {ID: "n", Kind: "text", Text: "intermediate note"}, {ID: "t", Kind: "tool", Tool: &ToolCall{ID: "read", Kind: ToolReadFile}}, {ID: "reply", Kind: "text", Text: "Final answer"}}}, {TurnID: "b", Role: "assistant", Blocks: []Block{{Kind: "tool", Tool: &ToolCall{ID: "other", Kind: ToolExec}}}}}
	activity := TurnActivity(messages, "a")
	if len(activity) != 3 {
		t.Fatalf("working prefix includes final/foreign blocks: %+v", activity)
	}
	summary := SummarizeActivity(activity)
	if summary.Notes != 1 || summary.Commands != 0 || summary.Reads != 1 {
		t.Fatal("summary incorporated final reply or another turn")
	}
	if (ActivitySummary{Thoughts: 1}).Label() != "Thought process" || (ActivitySummary{Notes: 2}).Label() != "Wrote 2 notes" || (ActivitySummary{}).Label() != "" {
		t.Fatal("thought/note-only labels diverged from source")
	}
}
