package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHookRecordSnapshotClonesAndRoundTripsWithoutBecomingReply(t *testing.T) {
	code := 0
	original := Snapshot{Messages: map[string][]Message{"chat": {{
		ID: "message", Role: "assistant", Blocks: []Block{{ID: "hook", Kind: "hook", Hook: &HookRecord{
			RuleID: "start", Name: "Startup", Event: HookSessionStart, Status: "completed", Details: "context", ExitCode: &code,
		}}},
	}}}}
	cloned := CloneSnapshot(original)
	record := cloned.Messages["chat"][0].Blocks[0].Hook
	record.Name = "changed"
	*record.ExitCode = 1
	source := original.Messages["chat"][0].Blocks[0].Hook
	if source.Name != "Startup" || *source.ExitCode != 0 {
		t.Fatal("hook records alias snapshot-owned values")
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	block := decoded.Messages["chat"][0].Blocks[0]
	if block.Kind != "hook" || block.Text != "" || block.Tool != nil || block.Hook == nil ||
		block.Hook.Details != "context" || block.Hook.ExitCode == nil || *block.Hook.ExitCode != 0 {
		t.Fatalf("lost typed hook record: %+v", block)
	}
	summary := SummarizeActivity([]Block{block})
	if summary.ToolCalls != 0 || summary.Notes != 0 {
		t.Fatalf("host hook counted as model work: %+v", summary)
	}
	if strings.Contains(string(data), `"text":"context"`) {
		t.Fatal("hook details serialized as reply text")
	}
}
