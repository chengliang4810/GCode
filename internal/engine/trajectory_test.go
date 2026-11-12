package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestTrajectoryEncodedContentAndTruncation(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	if err := core.Execute(context.Background(), contracts.Command{Kind: "session.create", Name: "Encoding"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := core.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session := snapshot.Sessions[0]
	for _, tc := range []struct {
		name string
		text string
	}{
		{"normal", "中文\n<request>"},
		{"oversized", strings.Repeat("x", trajectoryRecordSize+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := contracts.ModelInvocation{
				SessionID: session.ID,
				Invocation: model.Invocation{
					ID: tc.name, Attempt: 1, StartedAt: time.Unix(1, 0), System: tc.text,
					Response: &model.Result{Message: model.Message{Role: "assistant", Text: "原始回复"}},
				},
			}
			if err := core.writeTrajectory(session, record); err != nil {
				t.Fatal(err)
			}
			if record.System != tc.text || record.Response.Message.Text != "原始回复" || record.Truncated {
				t.Fatal("saving diagnostics mutated the caller's request or response")
			}
			expected := record
			if tc.name == "oversized" {
				expected.Truncated = true
				expected.System = "[request context omitted: record size limit]"
				response := *record.Response
				response.Message = model.Message{Role: "assistant", Text: "[response content omitted: record size limit]"}
				expected.Response = &response
			}
			want, err := json.MarshalIndent(expected, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			folder, err := core.trajectoryDirectory(session, false)
			if err != nil {
				t.Fatal(err)
			}
			defer folder.Close()
			name := fmt.Sprintf("call-%019d-%s-%03d.json", record.StartedAt.UnixNano(), record.ID, record.Attempt)
			got, err := folder.ReadFile(name)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("saved trajectory differs from the expected JSON: %v", err)
			}
		})
	}
}

func TestTrajectoryOwnershipRetentionAndMissingHistory(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	ctx := context.Background()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.create", Name: "Trace"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := core.Snapshot(ctx)
	session := snapshot.Sessions[0]
	empty, err := core.ReadModelTrajectory(ctx, session.ID)
	if err != nil || len(empty.Records) != 0 {
		t.Fatal("history without real diagnostics must remain empty")
	}
	record := core.trajectoryRecorder(session, "turn", "main")
	start := time.Now()
	for i := range trajectoryMaxRecords + 2 {
		record(model.Invocation{ID: "record", Attempt: i + 1, StartedAt: start.Add(time.Duration(i) * time.Millisecond),
			System: strings.Repeat("s", 64), Response: &model.Result{Message: model.Message{Role: "assistant", Text: "answer"}}})
	}
	report, err := core.ReadModelTrajectory(ctx, session.ID)
	if err != nil || len(report.Records) != trajectoryMaxRecords || !report.Truncated || report.Records[0].Attempt != 3 {
		t.Fatalf("trajectory retention lost recent records: records=%d truncated=%v error=%v", len(report.Records), report.Truncated, err)
	}
	if report.Records[0].Error == "" {
		t.Fatal("abandoned call was presented as still running")
	}
	record(model.Invocation{ID: "oversized", Attempt: 1, StartedAt: start.Add(time.Second),
		System: strings.Repeat("x", trajectoryRecordSize+1)})
	bounded, err := core.ReadModelTrajectory(ctx, session.ID)
	if err != nil || !bounded.Truncated || len(bounded.Records) == 0 ||
		!strings.Contains(bounded.Records[len(bounded.Records)-1].System, "omitted") {
		t.Fatal("oversized context was not explicitly bounded")
	}
	if _, err := core.ReadModelTrajectory(ctx, "../another-session"); err == nil {
		t.Fatal("foreign session path accepted")
	}
	folder, err := core.trajectoryDirectory(session, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := folder.WriteFile("user-note.txt", []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	const foreign = "call-0000000000000000001-foreign-001.json"
	if err := folder.WriteFile(foreign, []byte("foreign content"), 0600); err != nil {
		t.Fatal(err)
	}
	defer folder.Close()
	if err := core.Execute(ctx, contracts.Command{Kind: "session.delete", ID: session.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := folder.Stat("user-note.txt"); err != nil {
		t.Fatal("deleting diagnostics removed an unrelated file")
	}
	entries, err := folder.ReadFile("user-note.txt")
	if err != nil || string(entries) != "preserve" {
		t.Fatal("foreign content changed")
	}
	if content, err := folder.ReadFile(foreign); err != nil || string(content) != "foreign content" {
		t.Fatal("a filename pattern was mistaken for ownership")
	}
}
