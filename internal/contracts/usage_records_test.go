package contracts

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestUsageRecordsFromMessagesDropsTranscriptPayloads(t *testing.T) {
	createdAt := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	records := UsageRecordsFromMessages([]Message{{
		ID:        "message",
		TurnID:    "turn",
		Role:      "assistant",
		CreatedAt: createdAt,
		ModelID:   "provider/model",
		Usage:     Usage{InputTokens: 12, OutputTokens: 3, CachedTokens: 1},
		Blocks: []Block{
			{Kind: "text", Text: "transcript body"},
			{Kind: "tool", Tool: &ToolCall{ID: "tool", Output: "tool output"}},
		},
	}})
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.ID != "message" || record.TurnID != "turn" || record.Role != "assistant" ||
		!record.CreatedAt.Equal(createdAt) || record.ModelID != "provider/model" ||
		record.Usage.InputTokens != 12 || record.Usage.OutputTokens != 3 || record.ToolCount != 1 || record.BlockCount != 2 {
		t.Fatalf("record metadata = %+v", record)
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{[]byte("transcript body"), []byte("tool output")} {
		if bytes.Contains(encoded, payload) {
			t.Fatalf("record JSON retained payload %q: %s", payload, encoded)
		}
	}
}

func TestUsageRunsDropsNonUsagePayloads(t *testing.T) {
	runs := UsageRuns([]Run{{
		RequestID:    "request",
		ID:           "run",
		SessionID:    "session",
		Status:       "completed",
		FinishReason: "stop",
		Error:        "sensitive run error",
		DurationMS:   1250,
		Summary:      ActivitySummary{ToolCalls: 9, Commands: 3},
		StartedAt:    time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC),
	}})
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	projected := runs[0]
	if projected.ID != "run" || projected.SessionID != "session" || projected.Status != "completed" || projected.DurationMS != 1250 {
		t.Fatalf("projected run = %+v", projected)
	}
	if projected.RequestID != "" || projected.Error != "" || projected.FinishReason != "" ||
		projected.Summary != (ActivitySummary{}) || !projected.StartedAt.IsZero() || projected.FinishedAt != nil {
		t.Fatalf("non-usage run payload survived: %+v", projected)
	}
	encoded, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("sensitive run error")) {
		t.Fatalf("run JSON retained an error payload: %s", encoded)
	}
}
